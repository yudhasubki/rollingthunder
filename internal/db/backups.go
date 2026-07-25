package db

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"rollingthunder/pkg/application"
	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"

	"github.com/google/uuid"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type executableLookup func(string) (string, error)
type commandFactory func(context.Context, string, ...string) *exec.Cmd

var defaultExecutableLookup executableLookup = exec.LookPath
var defaultCommandFactory commandFactory = exec.CommandContext

type restoreFileGrant struct {
	selection  database.RestoreFileSelection
	path       string
	connection string
	modified   time.Time
}

type maintenanceJob struct {
	id        string
	kind      string
	cancel    context.CancelFunc
	startedAt time.Time
	status    atomic.Value
	cancelled atomic.Bool
}

func newMaintenanceJob(
	id string,
	kind string,
	cancel context.CancelFunc,
) *maintenanceJob {
	job := &maintenanceJob{
		id:        id,
		kind:      kind,
		cancel:    cancel,
		startedAt: time.Now(),
	}
	job.status.Store("running")
	return job
}

func (job *maintenanceJob) progress() database.MaintenanceProgress {
	status, _ := job.status.Load().(string)
	return database.MaintenanceProgress{
		JobID:       job.id,
		Kind:        job.kind,
		Status:      status,
		ElapsedMS:   time.Since(job.startedAt).Milliseconds(),
		Cancellable: status == "running",
	}
}

func (s *Service) startMaintenanceJob(
	requestedID string,
	kind string,
) (context.Context, *maintenanceJob, error) {
	parent := s.ctx
	if parent == nil {
		parent = context.Background()
	}
	jobID := strings.TrimSpace(requestedID)
	if jobID == "" {
		jobID = uuid.NewString()
	}
	if len(jobID) > 128 {
		return nil, nil, fmt.Errorf("maintenance job ID is too long")
	}
	ctx, cancel := context.WithCancel(parent)
	job := newMaintenanceJob(jobID, kind, cancel)
	s.maintenanceMu.Lock()
	if _, exists := s.maintenanceJobs[jobID]; exists {
		s.maintenanceMu.Unlock()
		cancel()
		return nil, nil, fmt.Errorf("maintenance job %q is already running", jobID)
	}
	s.maintenanceJobs[jobID] = job
	s.maintenanceMu.Unlock()
	return ctx, job, nil
}

func (s *Service) finishMaintenanceJob(job *maintenanceJob) {
	if job == nil {
		return
	}
	job.cancel()
	s.maintenanceMu.Lock()
	if s.maintenanceJobs[job.id] == job {
		delete(s.maintenanceJobs, job.id)
	}
	s.maintenanceMu.Unlock()
}

func (s *Service) GetMaintenanceProgress(
	jobID string,
) response.BaseResponse[database.MaintenanceProgress] {
	s.maintenanceMu.RLock()
	job := s.maintenanceJobs[strings.TrimSpace(jobID)]
	s.maintenanceMu.RUnlock()
	if job == nil {
		return serviceError[database.MaintenanceProgress](
			"maintenance job is not running",
		)
	}
	return response.BaseResponse[database.MaintenanceProgress]{
		Data: job.progress(),
	}
}

func (s *Service) CancelMaintenance(
	jobID string,
) response.BaseResponse[bool] {
	s.maintenanceMu.RLock()
	job := s.maintenanceJobs[strings.TrimSpace(jobID)]
	s.maintenanceMu.RUnlock()
	if job == nil {
		return serviceError[bool]("maintenance job is not running")
	}
	job.cancelled.Store(true)
	job.status.Store("cancelling")
	job.cancel()
	return response.BaseResponse[bool]{Data: true}
}

func lookupExecutable(
	lookup executableLookup,
	names ...string,
) (string, string) {
	for _, name := range names {
		path, err := lookup(name)
		if err == nil && strings.TrimSpace(path) != "" {
			return path, filepath.Base(path)
		}
	}
	return "", ""
}

func backupCapabilitiesFor(
	lookup executableLookup,
	engine string,
) database.BackupCapabilities {
	switch engine {
	case "sqlite":
		return database.BackupCapabilities{
			Available:     true,
			Engine:        engine,
			Format:        database.BackupFormatSQLiteNative,
			Extension:     ".sqlite3",
			BackupTool:    "Built in",
			RestoreTool:   "Built in",
			RestoreReady:  true,
			BuiltIn:       true,
			SupportsScope: false,
		}
	case "postgres":
		_, backupTool := lookupExecutable(lookup, "pg_dump")
		_, restoreTool := lookupExecutable(lookup, "pg_restore")
		available := backupTool != ""
		message := ""
		if !available {
			message = "Install PostgreSQL client tools so pg_dump is available in PATH."
		} else if restoreTool == "" {
			message = "Backups are available, but pg_restore is required to restore them."
		}
		return database.BackupCapabilities{
			Available:     available,
			Engine:        engine,
			Format:        database.BackupFormatPostgresCustom,
			Extension:     ".dump",
			BackupTool:    backupTool,
			RestoreTool:   restoreTool,
			RestoreReady:  restoreTool != "",
			Message:       message,
			SupportsScope: true,
		}
	case "mysql":
		_, backupTool := lookupExecutable(
			lookup,
			"mysqldump",
			"mariadb-dump",
		)
		_, restoreTool := lookupExecutable(lookup, "mysql", "mariadb")
		available := backupTool != ""
		message := ""
		if !available {
			message = "Install MySQL or MariaDB client tools so mysqldump is available in PATH."
		} else if restoreTool == "" {
			message = "Backups are available, but the mysql or mariadb client is required to restore them."
		}
		return database.BackupCapabilities{
			Available:     available,
			Engine:        engine,
			Format:        database.BackupFormatMySQLSQL,
			Extension:     ".sql",
			BackupTool:    backupTool,
			RestoreTool:   restoreTool,
			RestoreReady:  restoreTool != "",
			Message:       message,
			SupportsScope: true,
		}
	case "oracle":
		return database.BackupCapabilities{
			Available:         true,
			Engine:            engine,
			Format:            database.BackupFormatOracleDataPump,
			Extension:         ".dmp",
			BackupTool:        "DBMS_DATAPUMP",
			RestoreTool:       "DBMS_DATAPUMP",
			RestoreReady:      true,
			BuiltIn:           true,
			SupportsScope:     false,
			RequiresDirectory: true,
			Directories:       []database.BackupDirectory{},
			Message: "Oracle Data Pump stages encrypted-at-rest responsibility " +
				"with the configured database server directory.",
		}
	default:
		return database.BackupCapabilities{
			Engine:  engine,
			Message: "This database engine does not expose a backup workflow.",
		}
	}
}

func (s *Service) backupCapabilitiesForConnection(
	ctx context.Context,
	connection *Connection,
) database.BackupCapabilities {
	capabilities := backupCapabilitiesFor(
		s.lookPath,
		connection.Driver.Capabilities().Engine,
	)
	if !capabilities.RequiresDirectory {
		return capabilities
	}
	driver, ok := connection.Driver.(database.StreamingBackupDriver)
	if !ok {
		capabilities.Available = false
		capabilities.RestoreReady = false
		capabilities.Message =
			"The Oracle driver does not expose its native Data Pump workflow."
		return capabilities
	}
	directories, err := driver.GetBackupDirectories(ctx)
	if err != nil {
		capabilities.Available = false
		capabilities.RestoreReady = false
		capabilities.Message = "Could not read Oracle Data Pump directories: " +
			err.Error()
		return capabilities
	}
	capabilities.Directories = directories
	if len(directories) == 0 {
		capabilities.Available = false
		capabilities.RestoreReady = false
		capabilities.Message =
			"Grant READ and WRITE on an Oracle DIRECTORY object before using Data Pump."
	}
	return capabilities
}

func (s *Service) GetBackupCapabilities(
	connectionID string,
) response.BaseResponse[database.BackupCapabilities] {
	connection, release, err := s.pinnedConnection(connectionID)
	if err != nil {
		return serviceError[database.BackupCapabilities](err.Error())
	}
	defer release()
	ctx, cancel := s.structuralChangeContext()
	defer cancel()
	return response.BaseResponse[database.BackupCapabilities]{
		Data: s.backupCapabilitiesForConnection(ctx, connection),
	}
}

func backupDialogConfiguration(
	capabilities database.BackupCapabilities,
	databaseName string,
) (wailsruntime.SaveDialogOptions, error) {
	name := sanitizeSuggestedFilename(databaseName, "database")
	name = strings.TrimSuffix(name, filepath.Ext(name)) + capabilities.Extension
	var displayName string
	switch capabilities.Format {
	case database.BackupFormatSQLiteNative:
		displayName = "SQLite backups (*.sqlite3)"
	case database.BackupFormatPostgresCustom:
		displayName = "PostgreSQL custom backups (*.dump)"
	case database.BackupFormatMySQLSQL:
		displayName = "MySQL / MariaDB backups (*.sql)"
	case database.BackupFormatOracleDataPump:
		displayName = "Oracle Data Pump backups (*.dmp)"
	default:
		return wailsruntime.SaveDialogOptions{}, fmt.Errorf(
			"unsupported backup format %q",
			capabilities.Format,
		)
	}
	return wailsruntime.SaveDialogOptions{
		Title:           "Back up database",
		DefaultFilename: name,
		Filters: []wailsruntime.FileFilter{{
			DisplayName: displayName,
			Pattern:     "*" + capabilities.Extension,
		}},
		CanCreateDirectories: true,
	}, nil
}

func commandEnvironment(config database.Config) []string {
	blocked := map[string]struct{}{
		"PGPASSWORD":    {},
		"PGPASSFILE":    {},
		"PGHOSTADDR":    {},
		"PGSSLMODE":     {},
		"PGSSLROOTCERT": {},
		"PGSSLCERT":     {},
		"PGSSLKEY":      {},
	}
	environment := make([]string, 0, len(os.Environ())+5)
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if _, sensitive := blocked[name]; !sensitive {
			environment = append(environment, value)
		}
	}
	if config.SSLMode != "" {
		environment = append(environment, "PGSSLMODE="+config.SSLMode)
	}
	if config.SSLRootCert != "" {
		environment = append(environment, "PGSSLROOTCERT="+config.SSLRootCert)
	}
	if config.SSLCert != "" {
		environment = append(environment, "PGSSLCERT="+config.SSLCert)
	}
	if config.SSLKey != "" {
		environment = append(environment, "PGSSLKEY="+config.SSLKey)
	}
	if config.TLSServerName != "" && config.Host != "" {
		// libpq uses hostaddr for the network route while retaining host for
		// verify-full certificate checks and password-file matching.
		environment = append(environment, "PGHOSTADDR="+config.Host)
	}
	return environment
}

func postgresPasswordFileValue(value string) (string, error) {
	if strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("PostgreSQL password-file values cannot contain line breaks")
	}
	return strings.NewReplacer(`\`, `\\`, `:`, `\:`).Replace(value), nil
}

func postgresCommandEnvironment(
	config database.Config,
) ([]string, func(), error) {
	environment := commandEnvironment(config)
	if config.Password == "" {
		return environment, func() {}, nil
	}
	fields := []string{
		strings.TrimSpace(config.Host),
		strings.TrimSpace(config.Port),
		strings.TrimSpace(config.Db),
		strings.TrimSpace(config.User),
		config.Password,
	}
	if serverName := strings.TrimSpace(config.TLSServerName); serverName != "" {
		fields[0] = serverName
	}
	for index := range fields {
		if index < len(fields)-1 && fields[index] == "" {
			fields[index] = "*"
		}
		escaped, err := postgresPasswordFileValue(fields[index])
		if err != nil {
			return nil, nil, err
		}
		fields[index] = escaped
	}
	file, err := os.CreateTemp("", "."+application.Identifier+"-pgpass-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create temporary PostgreSQL password file: %w", err)
	}
	path := file.Name()
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(path)
	}
	if err := file.Chmod(0o600); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("secure temporary PostgreSQL password file: %w", err)
	}
	if _, err := io.WriteString(file, strings.Join(fields, ":")+"\n"); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("write temporary PostgreSQL password file: %w", err)
	}
	if err := file.Sync(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("sync temporary PostgreSQL password file: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("close temporary PostgreSQL password file: %w", err)
	}
	return append(environment, "PGPASSFILE="+path), cleanup, nil
}

func postgresConnectionArguments(config database.Config) []string {
	args := make([]string, 0, 4)
	host := strings.TrimSpace(config.Host)
	if strings.TrimSpace(config.TLSServerName) != "" {
		host = strings.TrimSpace(config.TLSServerName)
	}
	if host != "" {
		args = append(args, "--host="+host)
	}
	if strings.TrimSpace(config.Port) != "" {
		args = append(args, "--port="+config.Port)
	}
	if strings.TrimSpace(config.User) != "" {
		args = append(args, "--username="+config.User)
	}
	return args
}

func mysqlOptionValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, "\r", `\r`)
	value = strings.ReplaceAll(value, "\t", `\t`)
	return `"` + value + `"`
}

func writeMySQLDefaults(config database.Config) (string, error) {
	file, err := os.CreateTemp("", "."+application.Identifier+"-mysql-client-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return "", err
	}
	var content strings.Builder
	content.WriteString("[client]\n")
	sslMode := strings.ToLower(strings.TrimSpace(config.SSLMode))
	sslModeOption := ""
	switch sslMode {
	case "", "disable":
		sslModeOption = "DISABLED"
	case "require":
		sslModeOption = "REQUIRED"
	case "verify-ca":
		sslModeOption = "VERIFY_CA"
	case "verify-full":
		sslModeOption = "VERIFY_IDENTITY"
	}
	options := [][2]string{
		{"host", config.Host},
		{"port", config.Port},
		{"user", config.User},
		{"password", config.Password},
		{"ssl-mode", sslModeOption},
		{"ssl-ca", config.SSLRootCert},
		{"ssl-cert", config.SSLCert},
		{"ssl-key", config.SSLKey},
	}
	for _, option := range options {
		if strings.TrimSpace(option[1]) == "" {
			continue
		}
		content.WriteString(option[0])
		content.WriteByte('=')
		content.WriteString(mysqlOptionValue(option[1]))
		content.WriteByte('\n')
	}
	if _, err := io.WriteString(file, content.String()); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	remove = false
	return path, nil
}

func runMaintenanceCommand(
	ctx context.Context,
	factory commandFactory,
	name string,
	args []string,
	environment []string,
	stdin io.Reader,
) error {
	command := factory(ctx, name, args...)
	command.Env = environment
	command.Stdin = stdin
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return context.Canceled
		}
		detail := strings.TrimSpace(stderr.String())
		if len(detail) > 4000 {
			detail = detail[len(detail)-4000:]
		}
		if detail != "" {
			return fmt.Errorf("%s: %w", detail, err)
		}
		return err
	}
	return nil
}

func (s *Service) createBackup(
	ctx context.Context,
	connection *Connection,
	tempPath string,
	request database.BackupRequest,
	capabilities database.BackupCapabilities,
) error {
	config := connection.effectiveConfig()
	switch capabilities.Format {
	case database.BackupFormatSQLiteNative:
		driver, ok := connection.Driver.(database.NativeBackupDriver)
		if !ok {
			return fmt.Errorf("SQLite driver does not support native online backup")
		}
		return driver.BackupDatabase(ctx, tempPath)

	case database.BackupFormatPostgresCustom:
		tool, _ := lookupExecutable(s.lookPath, "pg_dump")
		if tool == "" {
			return fmt.Errorf("pg_dump is not available")
		}
		args := postgresConnectionArguments(config)
		args = append(
			args,
			"--format=custom",
			"--no-owner",
			"--no-privileges",
			"--file="+tempPath,
		)
		if request.Schema != "" {
			args = append(args, "--schema="+request.Schema)
		}
		if request.SchemaOnly {
			args = append(args, "--schema-only")
		}
		if request.DataOnly {
			args = append(args, "--data-only")
		}
		args = append(args, config.Db)
		environment, cleanup, err := postgresCommandEnvironment(config)
		if err != nil {
			return err
		}
		defer cleanup()
		return runMaintenanceCommand(
			ctx,
			s.commandContext,
			tool,
			args,
			environment,
			nil,
		)

	case database.BackupFormatMySQLSQL:
		tool, _ := lookupExecutable(s.lookPath, "mysqldump", "mariadb-dump")
		if tool == "" {
			return fmt.Errorf("mysqldump or mariadb-dump is not available")
		}
		defaults, err := writeMySQLDefaults(config)
		if err != nil {
			return fmt.Errorf("create temporary MySQL client settings: %w", err)
		}
		defer os.Remove(defaults)
		args := []string{
			"--defaults-extra-file=" + defaults,
			"--single-transaction",
			"--quick",
			"--routines",
			"--triggers",
			"--events",
			"--hex-blob",
			"--add-drop-table",
			"--result-file=" + tempPath,
		}
		if request.SchemaOnly {
			args = append(args, "--no-data")
		}
		if request.DataOnly {
			args = append(args, "--no-create-info")
		}
		args = append(args, config.Db)
		return runMaintenanceCommand(
			ctx,
			s.commandContext,
			tool,
			args,
			os.Environ(),
			nil,
		)
	case database.BackupFormatOracleDataPump:
		driver, ok := connection.Driver.(database.StreamingBackupDriver)
		if !ok {
			return fmt.Errorf(
				"Oracle driver does not expose native Data Pump backup support",
			)
		}
		target, err := os.OpenFile(
			tempPath,
			os.O_WRONLY|os.O_TRUNC,
			0o600,
		)
		if err != nil {
			return err
		}
		backupErr := driver.BackupDatabaseToWriter(ctx, target, request)
		closeErr := target.Close()
		if backupErr != nil {
			return backupErr
		}
		return closeErr
	default:
		return fmt.Errorf("unsupported backup format %q", capabilities.Format)
	}
}

func (s *Service) BackupDatabase(
	request database.BackupRequest,
) response.BaseResponse[database.BackupResult] {
	if err := request.Validate(); err != nil {
		return serviceErrorWithCode[database.BackupResult](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Invalid backup request",
			err.Error(),
			"Choose one backup scope and try again.",
		)
	}
	connection, release, err := s.pinnedConnection(request.ConnectionID)
	if err != nil {
		return serviceError[database.BackupResult](err.Error())
	}
	defer release()
	capabilityCtx, capabilityCancel := s.structuralChangeContext()
	capabilities := s.backupCapabilitiesForConnection(
		capabilityCtx,
		connection,
	)
	capabilityCancel()
	if !capabilities.Available {
		return serviceErrorWithCode[database.BackupResult](
			http.StatusNotImplemented,
			errorCodeBackupUnavailable,
			"Backup tooling unavailable",
			capabilities.Message,
			"Install the required database client tools and restart Rolling Thunder.",
		)
	}
	dialog, err := backupDialogConfiguration(
		capabilities,
		connection.Config.Db,
	)
	if err != nil {
		return serviceError[database.BackupResult](err.Error())
	}
	parent := s.ctx
	if parent == nil {
		parent = context.Background()
	}
	targetPath, err := s.saveDialog(parent, dialog)
	if err != nil {
		return serviceErrorWithCode[database.BackupResult](
			http.StatusInternalServerError,
			errorCodeBackupFailed,
			"Could not choose backup destination",
			err.Error(),
			"Check file permissions and try again.",
		)
	}
	if strings.TrimSpace(targetPath) == "" {
		return response.BaseResponse[database.BackupResult]{
			Data: database.BackupResult{
				Format:    capabilities.Format,
				Cancelled: true,
			},
		}
	}
	if !strings.EqualFold(filepath.Ext(targetPath), capabilities.Extension) {
		targetPath = strings.TrimSuffix(
			targetPath,
			filepath.Ext(targetPath),
		) + capabilities.Extension
	}
	ctx, job, err := s.startMaintenanceJob(request.JobID, "backup")
	if err != nil {
		return serviceError[database.BackupResult](err.Error())
	}
	defer s.finishMaintenanceJob(job)

	tempFile, err := os.CreateTemp(
		filepath.Dir(targetPath),
		"."+application.Identifier+"-database-backup-*",
	)
	if err != nil {
		return serviceError[database.BackupResult](err.Error())
	}
	tempPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return serviceError[database.BackupResult](err.Error())
	}
	defer os.Remove(tempPath)

	err = s.createBackup(ctx, connection, tempPath, request, capabilities)
	if err != nil {
		if errors.Is(err, context.Canceled) || job.cancelled.Load() {
			return response.BaseResponse[database.BackupResult]{
				Data: database.BackupResult{
					Format:    capabilities.Format,
					Cancelled: true,
				},
			}
		}
		return serviceErrorWithCode[database.BackupResult](
			http.StatusBadRequest,
			errorCodeBackupFailed,
			"Database backup failed",
			err.Error(),
			"The destination was not replaced. Check database client tooling and permissions.",
		)
	}
	info, err := os.Stat(tempPath)
	if err != nil {
		return serviceError[database.BackupResult](err.Error())
	}
	if info.Size() == 0 {
		return serviceErrorWithCode[database.BackupResult](
			http.StatusInternalServerError,
			errorCodeBackupFailed,
			"Database backup is empty",
			"The backup tool produced a zero-byte file.",
			"The destination was not replaced. Check the database client output.",
		)
	}
	if err := replaceExportFile(tempPath, targetPath); err != nil {
		return serviceError[database.BackupResult](err.Error())
	}
	return response.BaseResponse[database.BackupResult]{
		Data: database.BackupResult{
			Path:   targetPath,
			Bytes:  info.Size(),
			Format: capabilities.Format,
		},
	}
}

func restoreFileFilters(engine string) []wailsruntime.FileFilter {
	switch engine {
	case "sqlite":
		return []wailsruntime.FileFilter{{
			DisplayName: "SQLite backups (*.sqlite, *.sqlite3, *.db)",
			Pattern:     "*.sqlite;*.sqlite3;*.db",
		}}
	case "postgres":
		return []wailsruntime.FileFilter{{
			DisplayName: "PostgreSQL custom backups (*.dump, *.backup)",
			Pattern:     "*.dump;*.backup",
		}}
	case "mysql":
		return []wailsruntime.FileFilter{{
			DisplayName: "MySQL / MariaDB SQL backups (*.sql)",
			Pattern:     "*.sql",
		}}
	case "oracle":
		return []wailsruntime.FileFilter{{
			DisplayName: "Oracle Data Pump backups (*.dmp)",
			Pattern:     "*.dmp",
		}}
	default:
		return nil
	}
}

func restoreFormatFor(engine string, path string) (database.BackupFormat, error) {
	extension := strings.ToLower(filepath.Ext(path))
	switch engine {
	case "sqlite":
		switch extension {
		case ".sqlite", ".sqlite3", ".db":
			return database.BackupFormatSQLiteNative, nil
		}
	case "postgres":
		switch extension {
		case ".dump", ".backup":
			return database.BackupFormatPostgresCustom, nil
		}
	case "mysql":
		if extension == ".sql" {
			return database.BackupFormatMySQLSQL, nil
		}
	case "oracle":
		if extension == ".dmp" {
			return database.BackupFormatOracleDataPump, nil
		}
	}
	return "", fmt.Errorf("selected file does not match the %s restore format", engine)
}

func (s *Service) ChooseRestoreFile(
	connectionID string,
) response.BaseResponse[database.RestoreFileSelection] {
	connection, release, err := s.pinnedConnection(connectionID)
	if err != nil {
		return serviceError[database.RestoreFileSelection](err.Error())
	}
	engine := connection.Driver.Capabilities().Engine
	release()
	parent := s.ctx
	if parent == nil {
		parent = context.Background()
	}
	path, err := s.restoreOpenDialog(parent, wailsruntime.OpenDialogOptions{
		Title:                "Choose database backup",
		Filters:              restoreFileFilters(engine),
		CanCreateDirectories: false,
		ResolvesAliases:      true,
	})
	if err != nil {
		return serviceError[database.RestoreFileSelection](err.Error())
	}
	if strings.TrimSpace(path) == "" {
		return response.BaseResponse[database.RestoreFileSelection]{}
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return serviceError[database.RestoreFileSelection](err.Error())
	}
	if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
		absolute = resolved
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return serviceError[database.RestoreFileSelection](err.Error())
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return serviceErrorWithCode[database.RestoreFileSelection](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Invalid backup file",
			"The selected backup must be a non-empty regular file.",
			"Choose a backup created for the selected database engine.",
		)
	}
	format, err := restoreFormatFor(engine, absolute)
	if err != nil {
		return serviceErrorWithCode[database.RestoreFileSelection](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Unsupported backup file",
			err.Error(),
			"Choose a backup created for the selected database engine.",
		)
	}
	selection := database.RestoreFileSelection{
		Token:  uuid.NewString(),
		Name:   filepath.Base(absolute),
		Size:   info.Size(),
		Format: format,
	}
	s.restoreFileMu.Lock()
	s.restoreFiles[selection.Token] = restoreFileGrant{
		selection:  selection,
		path:       filepath.Clean(absolute),
		connection: connectionID,
		modified:   info.ModTime(),
	}
	s.restoreFileMu.Unlock()
	return response.BaseResponse[database.RestoreFileSelection]{
		Data: selection,
	}
}

func (s *Service) restoreFile(
	connectionID string,
	token string,
) (restoreFileGrant, error) {
	s.restoreFileMu.RLock()
	grant, exists := s.restoreFiles[strings.TrimSpace(token)]
	s.restoreFileMu.RUnlock()
	if !exists || grant.connection != connectionID {
		return restoreFileGrant{}, fmt.Errorf(
			"the restore file token is invalid or belongs to another connection",
		)
	}
	return grant, nil
}

func hashRestoreFile(
	ctx context.Context,
	grant restoreFileGrant,
) (string, error) {
	file, err := os.Open(grant.path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	buffer := make([]byte, 1024*1024)
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		count, readErr := file.Read(buffer)
		if count > 0 {
			_, _ = hash.Write(buffer[:count])
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func validateRestoreHeader(grant restoreFileGrant) error {
	file, err := os.Open(grant.path)
	if err != nil {
		return err
	}
	defer file.Close()
	header := make([]byte, 16)
	count, err := io.ReadFull(file, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	}
	header = header[:count]
	switch grant.selection.Format {
	case database.BackupFormatSQLiteNative:
		if !bytes.HasPrefix(header, []byte("SQLite format 3\x00")) {
			return fmt.Errorf("file does not contain a SQLite database header")
		}
	case database.BackupFormatPostgresCustom:
		if !bytes.HasPrefix(header, []byte("PGDMP")) {
			return fmt.Errorf("file is not a PostgreSQL custom-format backup")
		}
	case database.BackupFormatMySQLSQL:
		if bytes.IndexByte(header, 0) >= 0 {
			return fmt.Errorf("SQL restore file contains binary data")
		}
	case database.BackupFormatOracleDataPump:
		// Oracle Data Pump headers vary by database release. The native import
		// API performs authoritative format and compatibility validation.
	}
	return nil
}

func restoreFingerprint(
	connectionID string,
	engine string,
	databaseName string,
	request database.RestorePreviewRequest,
	grant restoreFileGrant,
	fileHash string,
) string {
	hash := sha256.New()
	values := []string{
		connectionID,
		engine,
		databaseName,
		request.Schema,
		request.Directory,
		string(grant.selection.Format),
		grant.path,
		strconv.FormatInt(grant.selection.Size, 10),
		grant.modified.UTC().Format(time.RFC3339Nano),
		fileHash,
	}
	for _, value := range values {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (s *Service) buildRestorePreview(
	ctx context.Context,
	request database.RestorePreviewRequest,
) (database.RestorePreview, restoreFileGrant, error) {
	if strings.TrimSpace(request.ConnectionID) == "" ||
		strings.TrimSpace(request.Token) == "" {
		return database.RestorePreview{}, restoreFileGrant{},
			fmt.Errorf("connection and restore file are required")
	}
	grant, err := s.restoreFile(request.ConnectionID, request.Token)
	if err != nil {
		return database.RestorePreview{}, restoreFileGrant{}, err
	}
	info, err := os.Stat(grant.path)
	if err != nil {
		return database.RestorePreview{}, restoreFileGrant{}, err
	}
	if !info.Mode().IsRegular() ||
		info.Size() != grant.selection.Size ||
		!info.ModTime().Equal(grant.modified) {
		return database.RestorePreview{}, restoreFileGrant{},
			fmt.Errorf("the selected backup file changed; choose it again")
	}
	if err := validateRestoreHeader(grant); err != nil {
		return database.RestorePreview{}, restoreFileGrant{}, err
	}
	connection, release, err := s.pinnedConnection(request.ConnectionID)
	if err != nil {
		return database.RestorePreview{}, restoreFileGrant{}, err
	}
	engine := connection.Driver.Capabilities().Engine
	databaseName := connection.Config.Db
	transactional := engine == "sqlite"
	capabilities := s.backupCapabilitiesForConnection(ctx, connection)
	release()
	if grant.selection.Format != capabilities.Format {
		return database.RestorePreview{}, restoreFileGrant{}, fmt.Errorf(
			"backup format %s cannot be restored into %s",
			grant.selection.Format,
			engine,
		)
	}
	if !capabilities.RestoreReady {
		return database.RestorePreview{}, restoreFileGrant{},
			fmt.Errorf("%s", capabilities.Message)
	}
	if capabilities.RequiresDirectory {
		selectedDirectory := strings.TrimSpace(request.Directory)
		found := false
		for _, directory := range capabilities.Directories {
			if strings.EqualFold(directory.Name, selectedDirectory) {
				request.Directory = directory.Name
				found = true
				break
			}
		}
		if !found {
			return database.RestorePreview{}, restoreFileGrant{},
				fmt.Errorf(
					"select an accessible Oracle Data Pump directory",
				)
		}
	}
	fileHash, err := hashRestoreFile(ctx, grant)
	if err != nil {
		return database.RestorePreview{}, restoreFileGrant{}, err
	}
	warnings := []string{
		"Restore replaces existing objects and data in the selected database.",
		"Keep an independent backup until the restored database has been verified.",
	}
	if !transactional {
		warnings = append(
			warnings,
			"The external restore tool may commit multiple DDL steps; cancellation or failure can leave a partial restore.",
		)
	}
	if engine == "oracle" {
		warnings = append(
			warnings,
			"Oracle Data Pump stages the dump in the selected server directory and removes the temporary copy after the job.",
			"Data Pump import auto-commits object changes and can leave a partial schema when cancelled or rejected by the server.",
		)
	}
	return database.RestorePreview{
		ConnectionID:  request.ConnectionID,
		Database:      databaseName,
		Engine:        engine,
		File:          grant.selection.Name,
		Size:          grant.selection.Size,
		Format:        grant.selection.Format,
		Schema:        request.Schema,
		Directory:     request.Directory,
		Destructive:   true,
		Transactional: transactional,
		Warnings:      warnings,
		Fingerprint: restoreFingerprint(
			request.ConnectionID,
			engine,
			databaseName,
			request,
			grant,
			fileHash,
		),
	}, grant, nil
}

func (s *Service) PreviewDatabaseRestore(
	request database.RestorePreviewRequest,
) response.BaseResponse[database.RestorePreview] {
	parent := s.ctx
	if parent == nil {
		parent = context.Background()
	}
	preview, _, err := s.buildRestorePreview(parent, request)
	if err != nil {
		return serviceErrorWithCode[database.RestorePreview](
			http.StatusBadRequest,
			errorCodeRestoreFailed,
			"Could not prepare restore",
			err.Error(),
			"Choose the backup again and verify the required client tool is installed.",
		)
	}
	return response.BaseResponse[database.RestorePreview]{Data: preview}
}

func (s *Service) restoreSQLite(
	ctx context.Context,
	driver database.NativeBackupDriver,
	sourcePath string,
) error {
	rollbackFile, err := os.CreateTemp(
		"",
		"."+application.Identifier+"-sqlite-rollback-*.sqlite3",
	)
	if err != nil {
		return fmt.Errorf("prepare SQLite restore rollback: %w", err)
	}
	rollbackPath := rollbackFile.Name()
	if err := rollbackFile.Close(); err != nil {
		_ = os.Remove(rollbackPath)
		return err
	}
	defer os.Remove(rollbackPath)
	if err := driver.BackupDatabase(ctx, rollbackPath); err != nil {
		return fmt.Errorf("create SQLite restore rollback: %w", err)
	}
	if err := driver.RestoreDatabase(ctx, sourcePath); err != nil {
		rollbackCtx, cancel := context.WithTimeout(
			context.Background(),
			restoreRollbackTimeout,
		)
		defer cancel()
		if rollbackErr := driver.RestoreDatabase(rollbackCtx, rollbackPath); rollbackErr != nil {
			return fmt.Errorf(
				"restore SQLite backup: %w; automatic rollback also failed: %v",
				err,
				rollbackErr,
			)
		}
		return fmt.Errorf("restore SQLite backup: %w; previous database was restored", err)
	}
	return nil
}

func (s *Service) runRestore(
	ctx context.Context,
	connection *Connection,
	request database.RestorePreviewRequest,
	grant restoreFileGrant,
) error {
	config := connection.effectiveConfig()
	switch grant.selection.Format {
	case database.BackupFormatSQLiteNative:
		driver, ok := connection.Driver.(database.NativeBackupDriver)
		if !ok {
			return fmt.Errorf("SQLite driver does not expose online restore support")
		}
		return s.restoreSQLite(ctx, driver, grant.path)

	case database.BackupFormatPostgresCustom:
		tool, _ := lookupExecutable(s.lookPath, "pg_restore")
		if tool == "" {
			return fmt.Errorf("pg_restore is not available")
		}
		args := postgresConnectionArguments(config)
		args = append(
			args,
			"--clean",
			"--if-exists",
			"--no-owner",
			"--no-privileges",
			"--exit-on-error",
			"--dbname="+config.Db,
		)
		if request.Schema != "" {
			args = append(args, "--schema="+request.Schema)
		}
		args = append(args, grant.path)
		environment, cleanup, err := postgresCommandEnvironment(config)
		if err != nil {
			return err
		}
		defer cleanup()
		return runMaintenanceCommand(
			ctx,
			s.commandContext,
			tool,
			args,
			environment,
			nil,
		)

	case database.BackupFormatMySQLSQL:
		tool, _ := lookupExecutable(s.lookPath, "mysql", "mariadb")
		if tool == "" {
			return fmt.Errorf("mysql or mariadb client is not available")
		}
		defaults, err := writeMySQLDefaults(config)
		if err != nil {
			return fmt.Errorf("create temporary MySQL client settings: %w", err)
		}
		defer os.Remove(defaults)
		source, err := os.Open(grant.path)
		if err != nil {
			return err
		}
		defer source.Close()
		args := []string{
			"--defaults-extra-file=" + defaults,
			"--binary-mode",
			"--database=" + config.Db,
		}
		return runMaintenanceCommand(
			ctx,
			s.commandContext,
			tool,
			args,
			os.Environ(),
			source,
		)
	case database.BackupFormatOracleDataPump:
		driver, ok := connection.Driver.(database.StreamingBackupDriver)
		if !ok {
			return fmt.Errorf(
				"Oracle driver does not expose native Data Pump restore support",
			)
		}
		source, err := os.Open(grant.path)
		if err != nil {
			return err
		}
		defer source.Close()
		return driver.RestoreDatabaseFromReader(ctx, source, request)
	default:
		return fmt.Errorf("unsupported restore format %q", grant.selection.Format)
	}
}

func (s *Service) ApplyDatabaseRestore(
	request database.ApplyRestoreRequest,
) response.BaseResponse[database.RestoreResult] {
	if strings.TrimSpace(request.Fingerprint) == "" {
		return serviceErrorWithCode[database.RestoreResult](
			http.StatusConflict,
			errorCodeRestoreReview,
			"Restore review required",
			"The selected backup has not been reviewed.",
			"Preview the restore and confirm the target database first.",
		)
	}
	ctx, job, err := s.startMaintenanceJob(request.JobID, "restore")
	if err != nil {
		return serviceError[database.RestoreResult](err.Error())
	}
	defer s.finishMaintenanceJob(job)
	preview, grant, err := s.buildRestorePreview(ctx, request.Restore)
	if err != nil {
		return serviceErrorWithCode[database.RestoreResult](
			http.StatusBadRequest,
			errorCodeRestoreFailed,
			"Could not refresh restore preview",
			err.Error(),
			"Choose the backup again and review a fresh restore preview.",
		)
	}
	if !reviewedFingerprintMatches(request.Fingerprint, preview.Fingerprint) {
		return serviceErrorWithCode[database.RestoreResult](
			http.StatusConflict,
			errorCodeRestoreReview,
			"Restore preview changed",
			"The backup file or target database changed after review.",
			"Review the refreshed restore preview before applying it.",
		)
	}
	connection, release, err := s.writePinnedConnection(
		request.Restore.ConnectionID,
	)
	if err != nil {
		if err == errConnectionReadOnly {
			return readOnlyConnectionError[database.RestoreResult]()
		}
		return serviceError[database.RestoreResult](err.Error())
	}
	defer release()
	if err := s.runRestore(ctx, connection, request.Restore, grant); err != nil {
		if errors.Is(err, context.Canceled) || job.cancelled.Load() {
			return response.BaseResponse[database.RestoreResult]{
				Data: database.RestoreResult{
					Fingerprint: preview.Fingerprint,
					Cancelled:   true,
				},
			}
		}
		return serviceErrorWithCode[database.RestoreResult](
			http.StatusBadRequest,
			errorCodeRestoreFailed,
			"Database restore failed",
			err.Error(),
			"Inspect the database before retrying; external restores can be partially applied.",
		)
	}
	s.restoreFileMu.Lock()
	delete(s.restoreFiles, request.Restore.Token)
	s.restoreFileMu.Unlock()
	return response.BaseResponse[database.RestoreResult]{
		Data: database.RestoreResult{
			Restored:    true,
			Fingerprint: preview.Fingerprint,
		},
	}
}
