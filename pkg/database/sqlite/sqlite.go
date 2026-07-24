package sqlite

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"rollingthunder/pkg/database"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

const defaultBusyTimeout = 5 * time.Second

type Config struct {
	Db string
}

type SQLite struct {
	cfg         Config
	ctx         context.Context
	conn        *sqlx.DB
	path        string
	journalMode string
	busyTimeout time.Duration
}

func NewSQLite(ctx context.Context, cfg Config) *SQLite {
	return &SQLite{
		cfg:         cfg,
		ctx:         ctx,
		busyTimeout: defaultBusyTimeout,
	}
}

func sqliteDSN(configuredPath string) (string, string, error) {
	configuredPath = strings.TrimSpace(configuredPath)
	if configuredPath == "" {
		return "", "", fmt.Errorf("SQLite database file is required")
	}
	if configuredPath == ":memory:" {
		name := "rollingthunder-" + uuid.NewString()
		return "file:" + name + "?mode=memory&cache=shared", ":memory:", nil
	}
	if strings.HasPrefix(configuredPath, "file:") {
		return configuredPath, configuredPath, nil
	}
	absolute, err := filepath.Abs(configuredPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve SQLite database path: %w", err)
	}
	parent := filepath.Dir(absolute)
	if info, err := os.Stat(parent); err != nil {
		return "", "", fmt.Errorf("access SQLite database directory: %w", err)
	} else if !info.IsDir() {
		return "", "", fmt.Errorf("SQLite parent path is not a directory")
	}
	dsn := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}).String()
	return dsn, absolute, nil
}

func (s *SQLite) Connect(ctx context.Context) error {
	if ctx == nil {
		ctx = s.ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if s.conn != nil {
		if err := s.Close(); err != nil {
			return fmt.Errorf("close previous SQLite database: %w", err)
		}
	}
	dsn, path, err := sqliteDSN(s.cfg.Db)
	if err != nil {
		return err
	}
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	// SQLite connection-local PRAGMAs and transaction semantics are safest
	// when a Rolling Thunder connection owns one physical SQLite connection.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return err
	}
	conn := sqlx.NewDb(sqlDB, "sqlite")
	timeoutMS := s.busyTimeout.Milliseconds()
	if timeoutMS <= 0 {
		timeoutMS = defaultBusyTimeout.Milliseconds()
	}
	if _, err := conn.ExecContext(
		ctx,
		fmt.Sprintf("PRAGMA busy_timeout = %d", timeoutMS),
	); err != nil {
		_ = conn.Close()
		return fmt.Errorf("configure SQLite busy timeout: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		_ = conn.Close()
		return fmt.Errorf("enable SQLite foreign keys: %w", err)
	}
	var journalMode string
	if err := conn.GetContext(
		ctx,
		&journalMode,
		"PRAGMA journal_mode = WAL",
	); err != nil {
		_ = conn.Close()
		return fmt.Errorf(
			"enable SQLite WAL mode (the file may be read-only or locked): %w",
			err,
		)
	}
	s.conn = conn
	s.path = path
	s.journalMode = strings.ToUpper(journalMode)
	return nil
}

func (s *SQLite) Close() error {
	if s.conn == nil {
		return nil
	}
	err := s.conn.Close()
	s.conn = nil
	return err
}

func (s *SQLite) Ping(ctx context.Context) error {
	if s.conn == nil {
		return fmt.Errorf("SQLite connection is not open")
	}
	return s.conn.PingContext(ctx)
}

func (s *SQLite) ensureConnected() error {
	if s.conn == nil {
		return fmt.Errorf("SQLite database is not open")
	}
	return nil
}

func normalizeSQLiteSchema(schema string) string {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		return "main"
	}
	return schema
}

type sqliteDatabaseRow struct {
	Sequence int            `db:"seq"`
	Name     string         `db:"name"`
	File     sql.NullString `db:"file"`
}

func (s *SQLite) databaseList() ([]sqliteDatabaseRow, error) {
	if err := s.ensureConnected(); err != nil {
		return nil, err
	}
	var rows []sqliteDatabaseRow
	if err := s.conn.Select(&rows, "PRAGMA database_list"); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *SQLite) GetSchemas() ([]string, error) {
	rows, err := s.databaseList()
	if err != nil {
		return nil, err
	}
	schemas := make([]string, 0, len(rows))
	for _, row := range rows {
		schemas = append(schemas, row.Name)
	}
	return schemas, nil
}

func (s *SQLite) GetCollections(schema ...string) ([]string, error) {
	if err := s.ensureConnected(); err != nil {
		return nil, err
	}
	target := "main"
	if len(schema) > 0 {
		target = normalizeSQLiteSchema(schema[0])
	}
	query := fmt.Sprintf(`
		SELECT name
		FROM %s.sqlite_schema
		WHERE type = 'table'
		  AND name NOT LIKE 'sqlite_%%'
		ORDER BY name`,
		quoteSQLiteIdentifier(target),
	)
	var tables []string
	if err := s.conn.Select(&tables, query); err != nil {
		return nil, err
	}
	return tables, nil
}

func (s *SQLite) GetDatabaseInfo() (database.Info, error) {
	if err := s.ensureConnected(); err != nil {
		return database.Info{}, err
	}
	var version string
	if err := s.conn.Get(&version, "SELECT sqlite_version()"); err != nil {
		return database.Info{}, err
	}
	return database.Info{
		Engine:   "SQLite",
		Version:  version,
		Database: s.path,
	}, nil
}

func (s *SQLite) CountCollectionData(table database.Table) (int, error) {
	structures, err := s.GetCollectionStructures(table)
	if err != nil {
		return 0, err
	}
	filter, args, err := buildSQLiteFilterClause(table.Filters, structures)
	if err != nil {
		return 0, err
	}
	query := "SELECT COUNT(*) FROM " +
		quoteSQLiteQualifiedIdentifier(
			normalizeSQLiteSchema(table.Schema),
			table.Name,
		) +
		filter
	var count int
	if err := s.conn.Get(&count, query, args...); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *SQLite) tableHasRowID(table database.Table) (bool, error) {
	ddl, err := s.GetTableDDL(table)
	if err != nil {
		return false, err
	}
	return !strings.Contains(strings.ToUpper(ddl), "WITHOUT ROWID"), nil
}

func (s *SQLite) GetCollectionData(
	table database.Table,
) (database.Structures, []map[string]interface{}, error) {
	if table.Limit < 0 {
		return nil, nil, fmt.Errorf("table limit cannot be negative")
	}
	if table.Offset < 0 {
		return nil, nil, fmt.Errorf("table offset cannot be negative")
	}
	table.Schema = normalizeSQLiteSchema(table.Schema)
	structures, err := s.GetCollectionStructures(table)
	if err != nil {
		return nil, nil, err
	}
	hasRowID, err := s.tableHasRowID(table)
	if err != nil {
		return nil, nil, err
	}
	filter, args, err := buildSQLiteFilterClause(table.Filters, structures)
	if err != nil {
		return nil, nil, err
	}
	order, err := buildSQLiteOrderClause(table.Sorts, structures, hasRowID)
	if err != nil {
		return nil, nil, err
	}
	pagination, err := s.PaginationClause(table.Limit, table.Offset)
	if err != nil {
		return nil, nil, err
	}
	query := "SELECT * FROM " +
		quoteSQLiteQualifiedIdentifier(table.Schema, table.Name) +
		filter + order + " " + pagination
	rows, err := s.conn.Queryx(query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	results := make([]map[string]interface{}, 0)
	for rows.Next() {
		row := make(map[string]interface{})
		if err := rows.MapScan(row); err != nil {
			return nil, nil, fmt.Errorf("scan SQLite row: %w", err)
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("read SQLite rows: %w", err)
	}
	return structures, results, nil
}

func sqliteMutationColumns(
	values map[string]interface{},
	structures database.Structures,
) ([]string, []interface{}, error) {
	available := make(map[string]database.Structure, len(structures))
	for _, structure := range structures {
		available[structure.Name] = structure
	}
	for column := range values {
		if column == "_isNew" || strings.HasPrefix(column, "temp_") ||
			strings.HasPrefix(column, "_rt") {
			continue
		}
		if _, exists := available[column]; !exists {
			return nil, nil, fmt.Errorf("unknown table column %q", column)
		}
	}
	columns := make([]string, 0, len(values))
	args := make([]interface{}, 0, len(values))
	for _, structure := range structures {
		value, exists := values[structure.Name]
		if !exists || structure.IsGenerated {
			continue
		}
		if value == nil && (structure.IsAutoInc || structure.Default != nil) {
			continue
		}
		columns = append(columns, structure.Name)
		args = append(args, value)
	}
	return columns, args, nil
}

func (s *SQLite) InsertRow(
	table database.Table,
	values map[string]interface{},
) error {
	if len(values) == 0 {
		return fmt.Errorf("no data to insert")
	}
	table.Schema = normalizeSQLiteSchema(table.Schema)
	structures, err := s.GetCollectionStructures(table)
	if err != nil {
		return err
	}
	columns, args, err := sqliteMutationColumns(values, structures)
	if err != nil {
		return err
	}
	target := quoteSQLiteQualifiedIdentifier(table.Schema, table.Name)
	query := "INSERT INTO " + target + " DEFAULT VALUES"
	if len(columns) > 0 {
		quoted := make([]string, len(columns))
		placeholders := make([]string, len(columns))
		for index, column := range columns {
			quoted[index] = quoteSQLiteIdentifier(column)
			placeholders[index] = "?"
		}
		query = fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES (%s)",
			target,
			strings.Join(quoted, ", "),
			strings.Join(placeholders, ", "),
		)
	}
	_, err = s.conn.Exec(query, args...)
	return err
}

func (s *SQLite) UpdateRow(
	table database.Table,
	values map[string]interface{},
	primaryKey string,
) error {
	primaryKey = strings.TrimSpace(primaryKey)
	if primaryKey == "" {
		return fmt.Errorf("a primary key is required for row updates")
	}
	primaryValue, exists := values[primaryKey]
	if !exists {
		return fmt.Errorf("primary key %q not found in data", primaryKey)
	}
	table.Schema = normalizeSQLiteSchema(table.Schema)
	structures, err := s.GetCollectionStructures(table)
	if err != nil {
		return err
	}
	columns, args, err := sqliteMutationColumns(values, structures)
	if err != nil {
		return err
	}
	assignments := make([]string, 0, len(columns))
	filteredArgs := make([]interface{}, 0, len(columns))
	for index, column := range columns {
		if column == primaryKey {
			continue
		}
		assignments = append(
			assignments,
			quoteSQLiteIdentifier(column)+" = ?",
		)
		filteredArgs = append(filteredArgs, args[index])
	}
	if len(assignments) == 0 {
		return fmt.Errorf("no mutable columns to update")
	}
	filteredArgs = append(filteredArgs, primaryValue)
	result, err := s.conn.Exec(
		fmt.Sprintf(
			"UPDATE %s SET %s WHERE %s = ?",
			quoteSQLiteQualifiedIdentifier(table.Schema, table.Name),
			strings.Join(assignments, ", "),
			quoteSQLiteIdentifier(primaryKey),
		),
		filteredArgs...,
	)
	if err != nil {
		return err
	}
	return requireOneSQLiteRow(result, "row update")
}

func (s *SQLite) DeleteRow(
	table database.Table,
	primaryKey string,
	primaryValue interface{},
) error {
	primaryKey = strings.TrimSpace(primaryKey)
	if primaryKey == "" {
		return fmt.Errorf("a primary key is required for row deletion")
	}
	result, err := s.conn.Exec(
		fmt.Sprintf(
			"DELETE FROM %s WHERE %s = ?",
			quoteSQLiteQualifiedIdentifier(
				normalizeSQLiteSchema(table.Schema),
				table.Name,
			),
			quoteSQLiteIdentifier(primaryKey),
		),
		primaryValue,
	)
	if err != nil {
		return err
	}
	return requireOneSQLiteRow(result, "row deletion")
}

func requireOneSQLiteRow(result sql.Result, action string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf(
			"%s affected %d rows instead of exactly one",
			action,
			affected,
		)
	}
	return nil
}

type sqliteMappedRows interface {
	Next() bool
	Columns() ([]string, error)
	MapScan(map[string]interface{}) error
	Err() error
}

func collectSQLiteQueryResults(
	rows sqliteMappedRows,
	maxRows int,
) (database.QueryResult, error) {
	columns, err := rows.Columns()
	if err != nil {
		return database.QueryResult{}, fmt.Errorf("read SQLite query columns: %w", err)
	}
	result := database.QueryResult{
		Rows:     make([]map[string]interface{}, 0),
		RowLimit: maxRows,
		Columns:  columns,
	}
	for rows.Next() {
		if maxRows > 0 && len(result.Rows) >= maxRows {
			result.Truncated = true
			break
		}
		row := make(map[string]interface{})
		if err := rows.MapScan(row); err != nil {
			return database.QueryResult{}, fmt.Errorf("scan SQLite query row: %w", err)
		}
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return database.QueryResult{}, fmt.Errorf("read SQLite query rows: %w", err)
	}
	return result, nil
}

type sqliteQueryRunner interface {
	QueryxContext(context.Context, string, ...interface{}) (*sqlx.Rows, error)
}

func executeSQLiteQuery(
	ctx context.Context,
	runner sqliteQueryRunner,
	query string,
	options database.QueryOptions,
) (database.QueryResult, error) {
	rows, err := runner.QueryxContext(ctx, query, options.Args...)
	if err != nil {
		return database.QueryResult{}, err
	}
	defer rows.Close()
	return collectSQLiteQueryResults(rows, options.MaxRows)
}

func (s *SQLite) ExecuteQuery(
	ctx context.Context,
	query string,
	options database.QueryOptions,
) (database.QueryResult, error) {
	return executeSQLiteQuery(ctx, s.conn, query, options)
}

type sqliteTransaction struct {
	tx *sqlx.Tx
}

func (transaction *sqliteTransaction) ExecuteQuery(
	ctx context.Context,
	query string,
	options database.QueryOptions,
) (database.QueryResult, error) {
	return executeSQLiteQuery(ctx, transaction.tx, query, options)
}

func (transaction *sqliteTransaction) Commit() error {
	return transaction.tx.Commit()
}

func (transaction *sqliteTransaction) Rollback() error {
	return transaction.tx.Rollback()
}

func (s *SQLite) BeginTransaction(
	ctx context.Context,
) (database.Transaction, error) {
	transaction, err := s.conn.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &sqliteTransaction{tx: transaction}, nil
}

type sqliteExportRows struct {
	rows *sqlx.Rows
}

func (rows *sqliteExportRows) Columns() ([]string, error) {
	return rows.rows.Columns()
}

func (rows *sqliteExportRows) Next() bool {
	return rows.rows.Next()
}

func (rows *sqliteExportRows) Values() ([]interface{}, error) {
	return rows.rows.SliceScan()
}

func (rows *sqliteExportRows) Err() error {
	return rows.rows.Err()
}

type selectedSQLiteRows struct {
	source       database.RowStream
	selected     map[int]struct{}
	currentIndex int
	current      []interface{}
	err          error
}

func newSelectedSQLiteRows(
	source database.RowStream,
	indexes []int,
	pageLimit int,
) (database.RowStream, error) {
	selected := make(map[int]struct{}, len(indexes))
	for _, index := range indexes {
		if index < 0 || index >= pageLimit {
			return nil, fmt.Errorf("selected row index %d is outside the current page", index)
		}
		selected[index] = struct{}{}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("select at least one row to export")
	}
	return &selectedSQLiteRows{
		source:       source,
		selected:     selected,
		currentIndex: -1,
	}, nil
}

func (rows *selectedSQLiteRows) Columns() ([]string, error) {
	return rows.source.Columns()
}

func (rows *selectedSQLiteRows) Next() bool {
	for rows.source.Next() {
		rows.currentIndex++
		values, err := rows.source.Values()
		if err != nil {
			rows.err = err
			rows.current = nil
			return false
		}
		if _, selected := rows.selected[rows.currentIndex]; selected {
			rows.current = values
			return true
		}
	}
	rows.current = nil
	return false
}

func (rows *selectedSQLiteRows) Values() ([]interface{}, error) {
	if rows.err != nil {
		return nil, rows.err
	}
	if rows.current == nil {
		return nil, fmt.Errorf("selected export row is unavailable")
	}
	return rows.current, nil
}

func (rows *selectedSQLiteRows) Err() error {
	if rows.err != nil {
		return rows.err
	}
	return rows.source.Err()
}

func buildSQLiteExportQuery(
	table database.Table,
	scope database.ExportScope,
	structures database.Structures,
	projection string,
	hasRowID bool,
) (sqliteQuery, error) {
	if strings.TrimSpace(table.Name) == "" {
		return sqliteQuery{}, fmt.Errorf("table name is required")
	}
	if table.Offset < 0 {
		return sqliteQuery{}, fmt.Errorf("table offset cannot be negative")
	}
	filter, args, err := buildSQLiteFilterClause(table.Filters, structures)
	if err != nil {
		return sqliteQuery{}, err
	}
	order, err := buildSQLiteOrderClause(table.Sorts, structures, hasRowID)
	if err != nil {
		return sqliteQuery{}, err
	}
	query := fmt.Sprintf(
		"SELECT %s FROM %s%s%s",
		projection,
		quoteSQLiteQualifiedIdentifier(table.Schema, table.Name),
		filter,
		order,
	)
	switch scope {
	case database.ExportScopePage, database.ExportScopeSelected:
		if table.Limit <= 0 {
			return sqliteQuery{}, fmt.Errorf("page export requires a positive table limit")
		}
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", table.Limit, table.Offset)
	case database.ExportScopeAll:
	default:
		return sqliteQuery{}, fmt.Errorf("unsupported table export scope %q", scope)
	}
	return sqliteQuery{SQL: query, Args: args}, nil
}

func (s *SQLite) ExportTable(
	ctx context.Context,
	request database.TableExportRequest,
	writer io.Writer,
) (database.ExportStats, error) {
	if err := database.ValidateExportOptions(request.Options); err != nil {
		return database.ExportStats{}, err
	}
	request.Table.Schema = normalizeSQLiteSchema(request.Table.Schema)
	structures, err := s.GetCollectionStructures(request.Table)
	if err != nil {
		return database.ExportStats{}, err
	}
	hasRowID, err := s.tableHasRowID(request.Table)
	if err != nil {
		return database.ExportStats{}, err
	}
	projection := "*"
	insertColumns := make(database.Structures, 0, len(structures))
	if request.Options.Format == database.ExportFormatSQL {
		parts := make([]string, 0, len(structures))
		for _, column := range structures {
			if column.IsGenerated {
				continue
			}
			insertColumns = append(insertColumns, column)
			quoted := quoteSQLiteIdentifier(column.Name)
			parts = append(parts, "quote("+quoted+") AS "+quoted)
		}
		if len(parts) == 0 {
			return database.ExportStats{}, fmt.Errorf(
				"table has no columns that can be exported as INSERT statements",
			)
		}
		projection = strings.Join(parts, ", ")
	}
	query, err := buildSQLiteExportQuery(
		request.Table,
		request.Scope,
		structures,
		projection,
		hasRowID,
	)
	if err != nil {
		return database.ExportStats{}, err
	}
	rows, err := s.conn.QueryxContext(ctx, query.SQL, query.Args...)
	if err != nil {
		return database.ExportStats{}, err
	}
	defer rows.Close()
	var stream database.RowStream = &sqliteExportRows{rows: rows}
	if request.Scope == database.ExportScopeSelected {
		stream, err = newSelectedSQLiteRows(
			stream,
			request.SelectedRowIndexes,
			request.Table.Limit,
		)
		if err != nil {
			return database.ExportStats{}, err
		}
	}
	if request.Options.Format == database.ExportFormatSQL {
		return writeSQLiteInsertStream(
			ctx,
			writer,
			stream,
			request.Table,
			insertColumns,
			request.Options.SQL,
		)
	}
	return database.WriteExportStreamContext(ctx, writer, stream, request.Options)
}

type sqliteInsertSink struct {
	writer      *bufio.Writer
	prefix      string
	batchSize   int
	rowsInBatch int
	rows        int64
}

func newSQLiteInsertSink(
	writer io.Writer,
	table database.Table,
	columns database.Structures,
	options database.SQLInsertOptions,
) *sqliteInsertSink {
	names := make([]string, len(columns))
	for index, column := range columns {
		names[index] = quoteSQLiteIdentifier(column.Name)
	}
	return &sqliteInsertSink{
		writer: bufio.NewWriter(writer),
		prefix: fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES\n",
			quoteSQLiteQualifiedIdentifier(table.Schema, table.Name),
			strings.Join(names, ", "),
		),
		batchSize: options.EffectiveBatchSize(),
	}
}

func (sink *sqliteInsertSink) write(value string) error {
	written, err := sink.writer.WriteString(value)
	if err != nil {
		return err
	}
	if written != len(value) {
		return io.ErrShortWrite
	}
	return nil
}

func sqliteQuotedValue(value interface{}) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "NULL", nil
	case string:
		return typed, nil
	case []byte:
		if utf8.Valid(typed) {
			return string(typed), nil
		}
		return "X'" + fmt.Sprintf("%x", typed) + "'", nil
	default:
		return "", fmt.Errorf("SQLite quote() returned unexpected value %T", value)
	}
}

func (sink *sqliteInsertSink) writeValues(values []interface{}) error {
	literals := make([]string, len(values))
	for index, value := range values {
		literal, err := sqliteQuotedValue(value)
		if err != nil {
			return err
		}
		literals[index] = literal
	}
	if sink.rowsInBatch == sink.batchSize {
		if err := sink.write(";\n\n"); err != nil {
			return err
		}
		sink.rowsInBatch = 0
	}
	if sink.rowsInBatch == 0 {
		if err := sink.write(sink.prefix); err != nil {
			return err
		}
	} else if err := sink.write(",\n"); err != nil {
		return err
	}
	if err := sink.write("  (" + strings.Join(literals, ", ") + ")"); err != nil {
		return err
	}
	sink.rowsInBatch++
	sink.rows++
	return nil
}

func writeSQLiteInsertStream(
	ctx context.Context,
	writer io.Writer,
	rows database.RowStream,
	table database.Table,
	columns database.Structures,
	options database.SQLInsertOptions,
) (database.ExportStats, error) {
	streamColumns, err := rows.Columns()
	if err != nil {
		return database.ExportStats{}, err
	}
	if len(streamColumns) != len(columns) {
		return database.ExportStats{}, fmt.Errorf(
			"SQL export returned %d columns for %d insert columns",
			len(streamColumns),
			len(columns),
		)
	}
	sink := newSQLiteInsertSink(writer, table, columns, options)
	if err := sink.write(
		"-- Rolling Thunder SQLite INSERT export\n" +
			"-- Generated columns are omitted.\n\n",
	); err != nil {
		return database.ExportStats{}, err
	}
	if options.IncludeTransaction {
		if err := sink.write("BEGIN TRANSACTION;\n\n"); err != nil {
			return database.ExportStats{}, err
		}
	}
	for rows.Next() {
		if err := database.CheckExportContext(ctx); err != nil {
			return database.ExportStats{}, err
		}
		values, err := rows.Values()
		if err != nil {
			return database.ExportStats{}, err
		}
		if err := sink.writeValues(values); err != nil {
			return database.ExportStats{}, err
		}
		database.ReportExportProgress(ctx, sink.rows)
	}
	if err := rows.Err(); err != nil {
		return database.ExportStats{}, err
	}
	if sink.rowsInBatch > 0 {
		if err := sink.write(";\n"); err != nil {
			return database.ExportStats{}, err
		}
	} else if err := sink.write("-- No rows matched the export scope.\n"); err != nil {
		return database.ExportStats{}, err
	}
	if options.IncludeTransaction {
		if err := sink.write("\nCOMMIT;\n"); err != nil {
			return database.ExportStats{}, err
		}
	}
	if err := sink.writer.Flush(); err != nil {
		return database.ExportStats{}, err
	}
	return database.ExportStats{Rows: sink.rows}, nil
}

func validateSQLiteFragment(value, label string) error {
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s contains a NUL byte", label)
	}
	if database.HasTopLevelStatementSeparator(value) {
		return fmt.Errorf("%s must not contain another SQL statement", label)
	}
	return nil
}

func (s *SQLite) CreateTable(
	table database.Table,
	columns []database.ColumnDefinition,
) error {
	if strings.TrimSpace(table.Name) == "" {
		return fmt.Errorf("table name is required")
	}
	if len(columns) == 0 {
		return fmt.Errorf("at least one column is required")
	}
	table.Schema = normalizeSQLiteSchema(table.Schema)
	definitions := make([]string, 0, len(columns)+1)
	primaryKeys := make([]string, 0)
	for _, column := range columns {
		name := strings.TrimSpace(column.Name)
		dataType := strings.TrimSpace(column.Type)
		if name == "" {
			continue
		}
		if dataType == "" {
			return fmt.Errorf("data type is required for column %q", name)
		}
		if err := validateSQLiteFragment(dataType, "column data type"); err != nil {
			return err
		}
		definition := quoteSQLiteIdentifier(name) + " " + dataType
		if !column.Nullable {
			definition += " NOT NULL"
		}
		if strings.TrimSpace(column.Default) != "" {
			if err := validateSQLiteFragment(column.Default, "column default"); err != nil {
				return err
			}
			definition += " DEFAULT " + strings.TrimSpace(column.Default)
		}
		if column.Unique {
			definition += " UNIQUE"
		}
		if column.PrimaryKey {
			primaryKeys = append(primaryKeys, quoteSQLiteIdentifier(name))
		}
		definitions = append(definitions, definition)
	}
	if len(definitions) == 0 {
		return fmt.Errorf("at least one named column is required")
	}
	if len(primaryKeys) > 0 {
		definitions = append(
			definitions,
			"PRIMARY KEY ("+strings.Join(primaryKeys, ", ")+")",
		)
	}
	_, err := s.conn.Exec(fmt.Sprintf(
		"CREATE TABLE %s (%s)",
		quoteSQLiteQualifiedIdentifier(table.Schema, table.Name),
		strings.Join(definitions, ", "),
	))
	return err
}

func (s *SQLite) DropTable(table database.Table) error {
	if strings.TrimSpace(table.Name) == "" {
		return fmt.Errorf("table name is required")
	}
	_, err := s.conn.Exec(
		"DROP TABLE IF EXISTS " +
			quoteSQLiteQualifiedIdentifier(
				normalizeSQLiteSchema(table.Schema),
				table.Name,
			),
	)
	return err
}

func (s *SQLite) TruncateTable(table database.Table) error {
	if strings.TrimSpace(table.Name) == "" {
		return fmt.Errorf("table name is required")
	}
	table.Schema = normalizeSQLiteSchema(table.Schema)
	transaction, err := s.conn.Beginx()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = transaction.Rollback()
		}
	}()
	if _, err := transaction.Exec(
		"DELETE FROM " +
			quoteSQLiteQualifiedIdentifier(table.Schema, table.Name),
	); err != nil {
		return err
	}
	_, _ = transaction.Exec(
		fmt.Sprintf(
			"DELETE FROM %s.sqlite_sequence WHERE name = ?",
			quoteSQLiteIdentifier(table.Schema),
		),
		table.Name,
	)
	if err := transaction.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *SQLite) GetTableDDL(table database.Table) (string, error) {
	if strings.TrimSpace(table.Name) == "" {
		return "", fmt.Errorf("table name is required")
	}
	table.Schema = normalizeSQLiteSchema(table.Schema)
	var definition sql.NullString
	query := fmt.Sprintf(
		"SELECT sql FROM %s.sqlite_schema WHERE type = 'table' AND name = ?",
		quoteSQLiteIdentifier(table.Schema),
	)
	if err := s.conn.Get(&definition, query, table.Name); err != nil {
		return "", err
	}
	if !definition.Valid || strings.TrimSpace(definition.String) == "" {
		return "", fmt.Errorf("SQLite table %q has no stored DDL", table.Name)
	}
	ddl := strings.TrimSpace(definition.String)
	if !strings.HasSuffix(ddl, ";") {
		ddl += ";"
	}
	return ddl, nil
}

func (s *SQLite) GetDataTypes() []database.DataType {
	return []database.DataType{
		{Name: "INTEGER", Category: "Integer", Description: "Integer affinity; may alias rowid as PRIMARY KEY"},
		{Name: "REAL", Category: "Numeric", Description: "Floating-point affinity"},
		{Name: "NUMERIC", Category: "Numeric", Description: "Numeric affinity"},
		{Name: "TEXT", Category: "Text", Description: "Text affinity"},
		{Name: "BLOB", Category: "Binary", Description: "No type conversion affinity"},
		{Name: "BOOLEAN", Category: "Boolean", Description: "Stored using SQLite numeric affinity"},
		{Name: "DATE", Category: "Date/Time", Description: "Application-defined date representation"},
		{Name: "DATETIME", Category: "Date/Time", Description: "Application-defined timestamp representation"},
		{Name: "JSON", Category: "JSON", Description: "Text JSON, queryable with JSON functions"},
	}
}

var _ database.Driver = (*SQLite)(nil)
var _ database.DriverWithSchema = (*SQLite)(nil)
var _ database.TransactionalDriver = (*SQLite)(nil)
