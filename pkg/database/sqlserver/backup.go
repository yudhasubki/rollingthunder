package sqlserver

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"rollingthunder/pkg/database"
)

const (
	maxSQLServerBackupPathLength = 2048
	sqlServerRestoreRecoveryTime = 2 * time.Minute
)

type sqlServerBackupHeader struct {
	database      string
	bytes         int64
	position      int
	finishedAt    string
	backupSetID   string
	checkpointLSN string
	databaseLSN   string
}

func normalizeSQLServerBackupPath(value string) (string, error) {
	path := strings.TrimSpace(value)
	if path == "" {
		return "", errors.New("SQL Server backup path is required")
	}
	if len(path) > maxSQLServerBackupPathLength {
		return "", fmt.Errorf(
			"SQL Server backup path exceeds %d characters",
			maxSQLServerBackupPathLength,
		)
	}
	for _, character := range path {
		if unicode.IsControl(character) {
			return "", errors.New(
				"SQL Server backup path cannot contain control characters",
			)
		}
	}
	if strings.ContainsAny(path, "*?") {
		return "", errors.New(
			"SQL Server backup path cannot contain wildcard characters",
		)
	}
	if !strings.HasSuffix(strings.ToLower(path), ".bak") {
		return "", errors.New("SQL Server native backups must use a .bak file")
	}
	absolute := strings.HasPrefix(path, "/") ||
		strings.HasPrefix(path, `\\`) ||
		strings.HasPrefix(path, "//") ||
		(len(path) >= 3 &&
			((path[0] >= 'a' && path[0] <= 'z') ||
				(path[0] >= 'A' && path[0] <= 'Z')) &&
			path[1] == ':' &&
			(path[2] == '\\' || path[2] == '/'))
	if !absolute {
		return "", errors.New(
			"SQL Server backup path must be absolute on the database server",
		)
	}
	return path, nil
}

func sqlServerDirectoryFromFile(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	index := strings.LastIndexAny(path, `/\`)
	if index < 0 {
		return ""
	}
	if index == 0 {
		return path[:1]
	}
	return path[:index]
}

func sqlServerSupportsDiskBackup(engineEdition int) bool {
	switch engineEdition {
	case 5, 6, 8, 11:
		return false
	default:
		return true
	}
}

func (s *SQLServer) GetBackupDirectories(
	ctx context.Context,
) ([]database.BackupDirectory, error) {
	if err := s.ensureConnected(); err != nil {
		return nil, err
	}
	var engineEdition int
	var defaultPath sql.NullString
	if err := s.conn.QueryRowContext(
		ctx,
		`SELECT
			CONVERT(int, SERVERPROPERTY('EngineEdition')),
			CONVERT(nvarchar(4000), SERVERPROPERTY('InstanceDefaultBackupPath'))`,
	).Scan(&engineEdition, &defaultPath); err != nil {
		return nil, fmt.Errorf("read SQL Server backup configuration: %w", err)
	}
	if !sqlServerSupportsDiskBackup(engineEdition) {
		return nil, errors.New(
			"this SQL Server deployment does not expose native DISK backup paths; Azure-managed deployments require a separate URL-based workflow",
		)
	}
	path := strings.TrimSpace(defaultPath.String)
	if path == "" {
		var masterFile string
		if err := s.conn.QueryRowContext(
			ctx,
			`SELECT TOP (1) physical_name
			 FROM master.sys.master_files
			 WHERE database_id = DB_ID(N'master') AND file_id = 1`,
		).Scan(&masterFile); err != nil {
			return nil, fmt.Errorf(
				"read SQL Server default data directory: %w",
				err,
			)
		}
		path = sqlServerDirectoryFromFile(masterFile)
	}
	if path == "" {
		return []database.BackupDirectory{}, nil
	}
	return []database.BackupDirectory{{
		Name: "Default backup directory",
		Path: path,
	}}, nil
}

func sqlServerMetadataText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []byte:
		return strings.TrimSpace(string(typed))
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func sqlServerMetadataInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int:
		return int64(typed), nil
	case int8:
		return int64(typed), nil
	case int16:
		return int64(typed), nil
	case int32:
		return int64(typed), nil
	case int64:
		return typed, nil
	case uint:
		return int64(typed), nil
	case uint8:
		return int64(typed), nil
	case uint16:
		return int64(typed), nil
	case uint32:
		return int64(typed), nil
	case uint64:
		if typed > uint64(^uint64(0)>>1) {
			return 0, errors.New("numeric metadata exceeds int64")
		}
		return int64(typed), nil
	case float32:
		return int64(typed), nil
	case float64:
		return int64(typed), nil
	}
	text := sqlServerMetadataText(value)
	if text == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse numeric backup metadata %q: %w", text, err)
	}
	return parsed, nil
}

func sqlServerBackupIdentity(
	path string,
	header sqlServerBackupHeader,
) string {
	hash := sha256.New()
	for _, value := range []string{
		path,
		header.database,
		strconv.FormatInt(header.bytes, 10),
		strconv.Itoa(header.position),
		header.finishedAt,
		header.backupSetID,
		header.checkpointLSN,
		header.databaseLSN,
	} {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func scanSQLServerBackupHeader(rows *sql.Rows) (sqlServerBackupHeader, error) {
	columns, err := rows.Columns()
	if err != nil {
		return sqlServerBackupHeader{}, err
	}
	columnIndex := make(map[string]int, len(columns))
	for index, column := range columns {
		columnIndex[strings.ToLower(column)] = index
	}
	required := []string{"databasename", "backuptype", "position"}
	for _, column := range required {
		if _, exists := columnIndex[column]; !exists {
			return sqlServerBackupHeader{}, fmt.Errorf(
				"SQL Server backup header does not contain %s",
				column,
			)
		}
	}

	var selected sqlServerBackupHeader
	found := false
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return sqlServerBackupHeader{}, err
		}
		backupType, err := sqlServerMetadataInt64(
			values[columnIndex["backuptype"]],
		)
		if err != nil {
			return sqlServerBackupHeader{}, err
		}
		if backupType != 1 {
			continue
		}
		position, err := sqlServerMetadataInt64(
			values[columnIndex["position"]],
		)
		if err != nil || position < 1 {
			if err == nil {
				err = errors.New("backup-set position must be positive")
			}
			return sqlServerBackupHeader{}, err
		}
		value := func(name string) any {
			index, exists := columnIndex[name]
			if !exists {
				return nil
			}
			return values[index]
		}
		size, err := sqlServerMetadataInt64(value("compressedbackupsize"))
		if err != nil {
			return sqlServerBackupHeader{}, err
		}
		if size <= 0 {
			size, err = sqlServerMetadataInt64(value("backupsize"))
			if err != nil {
				return sqlServerBackupHeader{}, err
			}
		}
		header := sqlServerBackupHeader{
			database:      sqlServerMetadataText(value("databasename")),
			bytes:         size,
			position:      int(position),
			finishedAt:    sqlServerMetadataText(value("backupfinishdate")),
			backupSetID:   sqlServerMetadataText(value("backupsetguid")),
			checkpointLSN: sqlServerMetadataText(value("checkpointlsn")),
			databaseLSN:   sqlServerMetadataText(value("databasebackuplsn")),
		}
		if header.database == "" {
			return sqlServerBackupHeader{}, errors.New(
				"SQL Server backup header does not identify a database",
			)
		}
		if !found ||
			header.finishedAt > selected.finishedAt ||
			(header.finishedAt == selected.finishedAt &&
				header.position > selected.position) {
			selected = header
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		return sqlServerBackupHeader{}, err
	}
	if !found {
		return sqlServerBackupHeader{}, errors.New(
			"the server file does not contain a full SQL Server database backup",
		)
	}
	return selected, nil
}

func inspectSQLServerBackup(
	ctx context.Context,
	connection *sql.DB,
	path string,
) (database.ServerBackupMetadata, error) {
	normalized, err := normalizeSQLServerBackupPath(path)
	if err != nil {
		return database.ServerBackupMetadata{}, err
	}
	rows, err := connection.QueryContext(
		ctx,
		"RESTORE HEADERONLY FROM DISK = @p1",
		normalized,
	)
	if err != nil {
		return database.ServerBackupMetadata{}, fmt.Errorf(
			"read SQL Server backup header: %w",
			err,
		)
	}
	header, scanErr := scanSQLServerBackupHeader(rows)
	closeErr := rows.Close()
	if scanErr != nil {
		return database.ServerBackupMetadata{}, scanErr
	}
	if closeErr != nil {
		return database.ServerBackupMetadata{}, closeErr
	}
	verifyStatement := fmt.Sprintf(
		"RESTORE VERIFYONLY FROM DISK = @p1 WITH FILE = %d, CHECKSUM",
		header.position,
	)
	if _, err := connection.ExecContext(ctx, verifyStatement, normalized); err != nil {
		return database.ServerBackupMetadata{}, fmt.Errorf(
			"verify SQL Server backup: %w",
			err,
		)
	}
	return database.ServerBackupMetadata{
		Path:       normalized,
		Database:   header.database,
		Bytes:      header.bytes,
		Position:   header.position,
		FinishedAt: header.finishedAt,
		Identity:   sqlServerBackupIdentity(normalized, header),
	}, nil
}

func (s *SQLServer) InspectServerBackup(
	ctx context.Context,
	path string,
) (database.ServerBackupMetadata, error) {
	if err := s.ensureConnected(); err != nil {
		return database.ServerBackupMetadata{}, err
	}
	return inspectSQLServerBackup(ctx, s.conn, path)
}

func (s *SQLServer) BackupDatabaseToServer(
	ctx context.Context,
	request database.BackupRequest,
) (database.ServerBackupMetadata, error) {
	if err := s.ensureConnected(); err != nil {
		return database.ServerBackupMetadata{}, err
	}
	path, err := normalizeSQLServerBackupPath(request.ServerPath)
	if err != nil {
		return database.ServerBackupMetadata{}, err
	}
	if request.SchemaOnly || request.DataOnly ||
		strings.TrimSpace(request.Schema) != "" {
		return database.ServerBackupMetadata{}, errors.New(
			"SQL Server native backup supports the complete database only",
		)
	}
	statement := fmt.Sprintf(
		"BACKUP DATABASE %s TO DISK = @p1 "+
			"WITH COPY_ONLY, INIT, CHECKSUM, STATS = 10",
		quoteIdentifier(s.cfg.Db),
	)
	if _, err := s.conn.ExecContext(ctx, statement, path); err != nil {
		return database.ServerBackupMetadata{}, fmt.Errorf(
			"create SQL Server native backup: %w",
			err,
		)
	}
	return inspectSQLServerBackup(ctx, s.conn, path)
}

func sqlServerSystemDatabase(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "master", "model", "msdb", "tempdb":
		return true
	default:
		return false
	}
}

func (s *SQLServer) reconnectAfterRestore(
	ctx context.Context,
	original Config,
) error {
	s.cfg = original
	return s.Connect(ctx)
}

func (s *SQLServer) RestoreDatabaseFromServer(
	ctx context.Context,
	path string,
) error {
	if err := s.ensureConnected(); err != nil {
		return err
	}
	if sqlServerSystemDatabase(s.cfg.Db) {
		return fmt.Errorf(
			"Rolling Thunder does not restore the SQL Server system database %q",
			s.cfg.Db,
		)
	}
	metadata, err := inspectSQLServerBackup(ctx, s.conn, path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(metadata.Database, s.cfg.Db) {
		return fmt.Errorf(
			"backup database %q does not match target database %q",
			metadata.Database,
			s.cfg.Db,
		)
	}

	original := s.cfg
	adminConfig := original
	adminConfig.Db = "master"
	admin, _, err := openSQLServerConnection(ctx, adminConfig)
	if err != nil {
		return fmt.Errorf("open SQL Server restore connection: %w", err)
	}
	defer admin.Close()
	if err := s.Close(); err != nil {
		return fmt.Errorf("close target SQL Server connection pool: %w", err)
	}

	target := quoteIdentifier(original.Db)
	recoveryCtx, recoveryCancel := context.WithTimeout(
		context.Background(),
		sqlServerRestoreRecoveryTime,
	)
	defer recoveryCancel()
	reconnect := func() error {
		return s.reconnectAfterRestore(recoveryCtx, original)
	}
	restoreAccess := func() error {
		_, recoveryErr := admin.ExecContext(
			recoveryCtx,
			"RESTORE DATABASE "+target+" WITH RECOVERY",
		)
		_, multiUserErr := admin.ExecContext(
			recoveryCtx,
			"ALTER DATABASE "+target+" SET MULTI_USER",
		)
		return errors.Join(recoveryErr, multiUserErr)
	}

	if _, err := admin.ExecContext(
		ctx,
		"ALTER DATABASE "+target+
			" SET SINGLE_USER WITH ROLLBACK IMMEDIATE",
	); err != nil {
		reconnectErr := reconnect()
		return fmt.Errorf(
			"prepare SQL Server database restore: %w (reconnect: %v)",
			err,
			reconnectErr,
		)
	}
	restoreStatement := fmt.Sprintf(
		"RESTORE DATABASE %s FROM DISK = @p1 "+
			"WITH FILE = %d, REPLACE, RECOVERY, CHECKSUM, STATS = 10",
		target,
		metadata.Position,
	)
	_, restoreErr := admin.ExecContext(ctx, restoreStatement, metadata.Path)
	if restoreErr != nil {
		recoveryErr := restoreAccess()
		reconnectErr := reconnect()
		return fmt.Errorf(
			"restore SQL Server database: %w (recovery: %v; reconnect: %v)",
			restoreErr,
			recoveryErr,
			reconnectErr,
		)
	}
	_, multiUserErr := admin.ExecContext(
		recoveryCtx,
		"ALTER DATABASE "+target+" SET MULTI_USER",
	)
	reconnectErr := reconnect()
	if multiUserErr != nil || reconnectErr != nil {
		return fmt.Errorf(
			"SQL Server restore completed but connection recovery failed (multi-user: %v; reconnect: %v)",
			multiUserErr,
			reconnectErr,
		)
	}
	return nil
}

var _ database.ServerSideBackupDriver = (*SQLServer)(nil)
