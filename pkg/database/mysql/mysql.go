package mysql

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"rollingthunder/pkg/application"
	"rollingthunder/pkg/database"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

type Config struct {
	Host          string
	Port          string
	User          string
	Password      string
	Db            string
	SSLMode       string
	SSLRootCert   string
	SSLCert       string
	SSLKey        string
	TLSServerName string
}

type MySQL struct {
	cfg    Config
	ctx    context.Context
	conn   *sqlx.DB
	engine string
}

const (
	defaultConnectionTimeout  = 15 * time.Second
	defaultConnectionLifetime = 5 * time.Minute
	defaultMaxIdleConnections = 2
	defaultMaxOpenConnections = 8
)

func NewMySQL(ctx context.Context, cfg Config) *MySQL {
	return &MySQL{
		cfg:    cfg,
		ctx:    ctx,
		engine: "MySQL",
	}
}

func (m *MySQL) Connect(ctx context.Context) error {
	if ctx == nil {
		ctx = m.ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if m.conn != nil {
		if err := m.Close(); err != nil {
			return fmt.Errorf("close previous MySQL connection: %w", err)
		}
	}

	driverConfig, err := buildMySQLDriverConfig(m.cfg)
	if err != nil {
		return err
	}

	connector, err := mysqldriver.NewConnector(driverConfig)
	if err != nil {
		return fmt.Errorf("configure MySQL connection: %w", err)
	}
	sqlDB := sql.OpenDB(connector)
	sqlDB.SetConnMaxLifetime(defaultConnectionLifetime)
	sqlDB.SetMaxIdleConns(defaultMaxIdleConnections)
	sqlDB.SetMaxOpenConns(defaultMaxOpenConnections)
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return err
	}

	m.conn = sqlx.NewDb(sqlDB, "mysql")
	var versionComment string
	if err := m.conn.GetContext(ctx, &versionComment, "SELECT @@version_comment"); err == nil &&
		strings.Contains(strings.ToLower(versionComment), "mariadb") {
		m.engine = "MariaDB"
	} else {
		m.engine = "MySQL"
	}
	m.ctx = context.WithoutCancel(ctx)
	return nil
}

func buildMySQLDriverConfig(cfg Config) (*mysqldriver.Config, error) {
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		host = database.DefaultHost
	}
	port := strings.TrimSpace(cfg.Port)
	if port == "" {
		port = database.DefaultMySQLPort
	}

	driverConfig := mysqldriver.NewConfig()
	driverConfig.User = cfg.User
	driverConfig.Passwd = cfg.Password
	driverConfig.Net = "tcp"
	driverConfig.Addr = net.JoinHostPort(host, port)
	driverConfig.DBName = strings.TrimSpace(cfg.Db)
	driverConfig.ParseTime = true
	driverConfig.ConnectionAttributes = "program_name:" + application.DatabaseClientName
	driverConfig.Loc = time.UTC
	driverConfig.Timeout = defaultConnectionTimeout
	driverConfig.ReadTimeout = 0
	driverConfig.WriteTimeout = 0
	driverConfig.MultiStatements = false
	driverConfig.InterpolateParams = false
	driverConfig.ClientFoundRows = true
	if err := driverConfig.Apply(
		mysqldriver.Charset("utf8mb4", "utf8mb4_unicode_ci"),
	); err != nil {
		return nil, fmt.Errorf("configure MySQL connection charset: %w", err)
	}

	tlsServerName := strings.TrimSpace(cfg.TLSServerName)
	if tlsServerName == "" {
		tlsServerName = host
	}
	tlsConfig, err := buildMySQLTLSConfig(cfg, tlsServerName)
	if err != nil {
		return nil, err
	}
	driverConfig.TLS = tlsConfig

	return driverConfig, nil
}

func buildMySQLTLSConfig(cfg Config, host string) (*tls.Config, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.SSLMode))
	if mode == "" || mode == "disable" {
		if cfg.SSLRootCert != "" || cfg.SSLCert != "" || cfg.SSLKey != "" {
			return nil, fmt.Errorf(
				"TLS certificate paths require an SSL mode other than disable",
			)
		}
		return nil, nil
	}
	if mode != "require" && mode != "verify-ca" && mode != "verify-full" {
		return nil, fmt.Errorf("unsupported MySQL SSL mode %q", cfg.SSLMode)
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: host,
	}
	if mode == "require" {
		// This mode guarantees encryption but intentionally does not
		// authenticate the server certificate.
		tlsConfig.InsecureSkipVerify = true //nolint:gosec
	}

	if strings.TrimSpace(cfg.SSLRootCert) != "" {
		pem, err := os.ReadFile(cfg.SSLRootCert)
		if err != nil {
			return nil, fmt.Errorf("read MySQL CA certificate: %w", err)
		}
		roots, err := x509.SystemCertPool()
		if err != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("MySQL CA certificate contains no valid certificates")
		}
		tlsConfig.RootCAs = roots
	} else if mode == "verify-ca" || mode == "verify-full" {
		return nil, fmt.Errorf("%s requires a CA certificate path", mode)
	}

	certPath := strings.TrimSpace(cfg.SSLCert)
	keyPath := strings.TrimSpace(cfg.SSLKey)
	if (certPath == "") != (keyPath == "") {
		return nil, fmt.Errorf(
			"MySQL client certificate and key must be provided together",
		)
	}
	if certPath != "" {
		certificate, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("load MySQL client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}

	if mode == "verify-ca" {
		tlsConfig.InsecureSkipVerify = true //nolint:gosec
		tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return fmt.Errorf("MySQL server did not provide a certificate")
			}
			intermediates := x509.NewCertPool()
			for _, certificate := range state.PeerCertificates[1:] {
				intermediates.AddCert(certificate)
			}
			_, err := state.PeerCertificates[0].Verify(x509.VerifyOptions{
				Roots:         tlsConfig.RootCAs,
				Intermediates: intermediates,
			})
			return err
		}
	}
	return tlsConfig, nil
}

func (m *MySQL) Close() error {
	if m.conn == nil {
		return nil
	}
	err := m.conn.Close()
	m.conn = nil
	return err
}

func (m *MySQL) Ping(ctx context.Context) error {
	if m.conn == nil {
		return fmt.Errorf("MySQL connection is not open")
	}
	return m.conn.PingContext(ctx)
}

func (m *MySQL) ensureConnected() error {
	if m.conn == nil {
		return fmt.Errorf("MySQL connection is not open")
	}
	return nil
}

func (m *MySQL) defaultDatabase(value string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(m.cfg.Db)
}

func (m *MySQL) GetSchemas() ([]string, error) {
	if err := m.ensureConnected(); err != nil {
		return nil, err
	}
	var databases []string
	const query = `
		SELECT schema_name
		FROM information_schema.schemata
		WHERE schema_name NOT IN (
			'information_schema',
			'mysql',
			'performance_schema',
			'sys'
		)
		ORDER BY schema_name`
	if err := m.conn.Select(&databases, query); err != nil {
		return nil, err
	}
	return databases, nil
}

func (m *MySQL) GetCollections(schema ...string) ([]string, error) {
	if err := m.ensureConnected(); err != nil {
		return nil, err
	}
	databaseName := ""
	if len(schema) > 0 {
		databaseName = schema[0]
	}
	databaseName = m.defaultDatabase(databaseName)
	if databaseName == "" {
		return nil, fmt.Errorf("database name is required")
	}

	var tables []string
	const query = `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = ?
		  AND table_type = 'BASE TABLE'
		ORDER BY table_name`
	if err := m.conn.Select(&tables, query, databaseName); err != nil {
		return nil, err
	}
	return tables, nil
}

func (m *MySQL) GetDatabaseInfo() (database.Info, error) {
	if err := m.ensureConnected(); err != nil {
		return database.Info{}, err
	}
	var version string
	if err := m.conn.Get(&version, "SELECT VERSION()"); err != nil {
		return database.Info{}, err
	}
	var current sql.NullString
	if err := m.conn.Get(&current, "SELECT DATABASE()"); err != nil {
		return database.Info{}, err
	}
	databaseName := m.cfg.Db
	if current.Valid {
		databaseName = current.String
	}
	return database.Info{
		Engine:   m.engine,
		Version:  version,
		Database: databaseName,
	}, nil
}

func normalizeMySQLValue(value interface{}) interface{} {
	bytes, ok := value.([]byte)
	if !ok {
		return value
	}
	if utf8.Valid(bytes) {
		return string(bytes)
	}
	return bytes
}

func normalizeMySQLRow(row map[string]interface{}) {
	for key, value := range row {
		row[key] = normalizeMySQLValue(value)
	}
}

func (m *MySQL) CountCollectionData(table database.Table) (int, error) {
	structures, err := m.GetCollectionStructures(table)
	if err != nil {
		return 0, err
	}
	filterClause, args, err := buildMySQLFilterClause(table.Filters, structures)
	if err != nil {
		return 0, err
	}
	var count int
	query := "SELECT COUNT(*) FROM " +
		quoteMySQLQualifiedIdentifier(m.defaultDatabase(table.Schema), table.Name) +
		filterClause
	if err := m.conn.Get(&count, query, args...); err != nil {
		return 0, err
	}
	return count, nil
}

func (m *MySQL) GetCollectionData(
	table database.Table,
) (database.Structures, []map[string]interface{}, error) {
	if table.Limit < 0 {
		return nil, nil, fmt.Errorf("table limit cannot be negative")
	}
	if table.Offset < 0 {
		return nil, nil, fmt.Errorf("table offset cannot be negative")
	}
	structures, err := m.GetCollectionStructures(table)
	if err != nil {
		return nil, nil, err
	}
	filterClause, args, err := buildMySQLFilterClause(table.Filters, structures)
	if err != nil {
		return nil, nil, err
	}
	orderClause, err := buildMySQLOrderClause(table.Sorts, structures)
	if err != nil {
		return nil, nil, err
	}
	pagination, err := m.PaginationClause(table.Limit, table.Offset)
	if err != nil {
		return nil, nil, err
	}
	query := "SELECT * FROM " +
		quoteMySQLQualifiedIdentifier(m.defaultDatabase(table.Schema), table.Name) +
		filterClause + orderClause + " " + pagination

	rows, err := m.conn.Queryx(query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	results := make([]map[string]interface{}, 0)
	for rows.Next() {
		row := make(map[string]interface{})
		if err := rows.MapScan(row); err != nil {
			return nil, nil, fmt.Errorf("scan MySQL row: %w", err)
		}
		normalizeMySQLRow(row)
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("read MySQL rows: %w", err)
	}
	return structures, results, nil
}

func sortedMySQLMutationColumns(
	data map[string]interface{},
	structures database.Structures,
) ([]string, []interface{}, error) {
	available := make(map[string]database.Structure, len(structures))
	for _, structure := range structures {
		available[structure.Name] = structure
	}
	for column := range data {
		if column == "_isNew" || strings.HasPrefix(column, "temp_") ||
			strings.HasPrefix(column, "_rt") {
			continue
		}
		if _, exists := available[column]; !exists {
			return nil, nil, fmt.Errorf("unknown table column %q", column)
		}
	}
	columns := make([]string, 0, len(data))
	values := make([]interface{}, 0, len(data))
	for _, structure := range structures {
		value, exists := data[structure.Name]
		if !exists {
			continue
		}
		if value == nil && (structure.IsAutoInc || structure.Default != nil) {
			continue
		}
		columns = append(columns, structure.Name)
		values = append(values, value)
	}
	return columns, values, nil
}

func (m *MySQL) InsertRow(
	table database.Table,
	data map[string]interface{},
) error {
	if len(data) == 0 {
		return fmt.Errorf("no data to insert")
	}
	structures, err := m.GetCollectionStructures(table)
	if err != nil {
		return err
	}
	columns, values, err := sortedMySQLMutationColumns(data, structures)
	if err != nil {
		return err
	}
	target := quoteMySQLQualifiedIdentifier(
		m.defaultDatabase(table.Schema),
		table.Name,
	)
	query := "INSERT INTO " + target + " () VALUES ()"
	if len(columns) > 0 {
		quoted := make([]string, len(columns))
		placeholders := make([]string, len(columns))
		for index, column := range columns {
			quoted[index] = quoteMySQLIdentifier(column)
			placeholders[index] = "?"
		}
		query = fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES (%s)",
			target,
			strings.Join(quoted, ", "),
			strings.Join(placeholders, ", "),
		)
	}
	_, err = m.conn.Exec(query, values...)
	return err
}

func (m *MySQL) UpdateRow(
	table database.Table,
	data map[string]interface{},
	primaryKey string,
) error {
	primaryKey = strings.TrimSpace(primaryKey)
	if primaryKey == "" {
		return fmt.Errorf("a primary key is required for row updates")
	}
	primaryValue, exists := data[primaryKey]
	if !exists {
		return fmt.Errorf("primary key %q not found in data", primaryKey)
	}
	structures, err := m.GetCollectionStructures(table)
	if err != nil {
		return err
	}
	columns, values, err := sortedMySQLMutationColumns(data, structures)
	if err != nil {
		return err
	}
	assignments := make([]string, 0, len(columns))
	args := make([]interface{}, 0, len(columns))
	for index, column := range columns {
		if column == primaryKey {
			continue
		}
		assignments = append(
			assignments,
			quoteMySQLIdentifier(column)+" = ?",
		)
		args = append(args, values[index])
	}
	if len(assignments) == 0 {
		return fmt.Errorf("no mutable columns to update")
	}
	args = append(args, primaryValue)
	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE %s = ?",
		quoteMySQLQualifiedIdentifier(m.defaultDatabase(table.Schema), table.Name),
		strings.Join(assignments, ", "),
		quoteMySQLIdentifier(primaryKey),
	)
	result, err := m.conn.Exec(query, args...)
	if err != nil {
		return err
	}
	return requireOneMySQLRow(result, "row update")
}

func (m *MySQL) DeleteRow(
	table database.Table,
	primaryKey string,
	primaryValue interface{},
) error {
	primaryKey = strings.TrimSpace(primaryKey)
	if primaryKey == "" {
		return fmt.Errorf("a primary key is required for row deletion")
	}
	query := fmt.Sprintf(
		"DELETE FROM %s WHERE %s = ?",
		quoteMySQLQualifiedIdentifier(m.defaultDatabase(table.Schema), table.Name),
		quoteMySQLIdentifier(primaryKey),
	)
	result, err := m.conn.Exec(query, primaryValue)
	if err != nil {
		return err
	}
	return requireOneMySQLRow(result, "row deletion")
}

func requireOneMySQLRow(result sql.Result, action string) error {
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

type mysqlMappedRows interface {
	Next() bool
	Columns() ([]string, error)
	MapScan(map[string]interface{}) error
	Err() error
}

func collectMySQLQueryResults(
	rows mysqlMappedRows,
	maxRows int,
) (database.QueryResult, error) {
	columns, err := rows.Columns()
	if err != nil {
		return database.QueryResult{}, fmt.Errorf("read MySQL query columns: %w", err)
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
			return database.QueryResult{}, fmt.Errorf("scan MySQL query row: %w", err)
		}
		normalizeMySQLRow(row)
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return database.QueryResult{}, fmt.Errorf("read MySQL query rows: %w", err)
	}
	return result, nil
}

type mysqlQueryRunner interface {
	QueryxContext(context.Context, string, ...interface{}) (*sqlx.Rows, error)
}

func executeMySQLQuery(
	ctx context.Context,
	runner mysqlQueryRunner,
	query string,
	options database.QueryOptions,
) (database.QueryResult, error) {
	rows, err := runner.QueryxContext(ctx, query, options.Args...)
	if err != nil {
		return database.QueryResult{}, err
	}
	defer rows.Close()
	return collectMySQLQueryResults(rows, options.MaxRows)
}

func (m *MySQL) ExecuteQuery(
	ctx context.Context,
	query string,
	options database.QueryOptions,
) (database.QueryResult, error) {
	return executeMySQLQuery(ctx, m.conn, query, options)
}

type mysqlTransaction struct {
	tx *sqlx.Tx
}

func (transaction *mysqlTransaction) ExecuteQuery(
	ctx context.Context,
	query string,
	options database.QueryOptions,
) (database.QueryResult, error) {
	return executeMySQLQuery(ctx, transaction.tx, query, options)
}

func (transaction *mysqlTransaction) Commit() error {
	return transaction.tx.Commit()
}

func (transaction *mysqlTransaction) Rollback() error {
	return transaction.tx.Rollback()
}

func (m *MySQL) BeginTransaction(
	ctx context.Context,
) (database.Transaction, error) {
	transaction, err := m.conn.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &mysqlTransaction{tx: transaction}, nil
}

type mysqlExportRows struct {
	rows *sqlx.Rows
}

func (rows *mysqlExportRows) Columns() ([]string, error) {
	return rows.rows.Columns()
}

func (rows *mysqlExportRows) Next() bool {
	return rows.rows.Next()
}

func (rows *mysqlExportRows) Values() ([]interface{}, error) {
	return rows.rows.SliceScan()
}

func (rows *mysqlExportRows) Err() error {
	return rows.rows.Err()
}

type selectedMySQLRows struct {
	source   database.RowStream
	selected map[int]struct{}
	index    int
	current  []interface{}
}

func newSelectedMySQLRows(
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
	return &selectedMySQLRows{
		source:   source,
		selected: selected,
		index:    -1,
	}, nil
}

func (rows *selectedMySQLRows) Columns() ([]string, error) {
	return rows.source.Columns()
}

func (rows *selectedMySQLRows) Next() bool {
	for rows.source.Next() {
		rows.index++
		values, err := rows.source.Values()
		if err != nil {
			rows.current = nil
			return false
		}
		if _, selected := rows.selected[rows.index]; selected {
			rows.current = values
			return true
		}
	}
	rows.current = nil
	return false
}

func (rows *selectedMySQLRows) Values() ([]interface{}, error) {
	if rows.current == nil {
		return nil, fmt.Errorf("selected export row is unavailable")
	}
	return rows.current, nil
}

func (rows *selectedMySQLRows) Err() error {
	return rows.source.Err()
}

func buildMySQLExportQuery(
	table database.Table,
	scope database.ExportScope,
	structures database.Structures,
	projection string,
) (mysqlQuery, error) {
	if strings.TrimSpace(table.Name) == "" {
		return mysqlQuery{}, fmt.Errorf("table name is required")
	}
	if strings.TrimSpace(projection) == "" {
		return mysqlQuery{}, fmt.Errorf("export projection is required")
	}
	if table.Offset < 0 {
		return mysqlQuery{}, fmt.Errorf("table offset cannot be negative")
	}
	filterClause, args, err := buildMySQLFilterClause(table.Filters, structures)
	if err != nil {
		return mysqlQuery{}, err
	}
	orderClause, err := buildMySQLOrderClause(table.Sorts, structures)
	if err != nil {
		return mysqlQuery{}, err
	}
	query := fmt.Sprintf(
		"SELECT %s FROM %s%s%s",
		projection,
		quoteMySQLQualifiedIdentifier(table.Schema, table.Name),
		filterClause,
		orderClause,
	)
	switch scope {
	case database.ExportScopePage, database.ExportScopeSelected:
		if table.Limit <= 0 {
			return mysqlQuery{}, fmt.Errorf(
				"page export requires a positive table limit",
			)
		}
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", table.Limit, table.Offset)
	case database.ExportScopeAll:
	default:
		return mysqlQuery{}, fmt.Errorf("unsupported table export scope %q", scope)
	}
	return mysqlQuery{SQL: query, Args: args}, nil
}

func (m *MySQL) ExportTable(
	ctx context.Context,
	request database.TableExportRequest,
	writer io.Writer,
) (database.ExportStats, error) {
	if err := database.ValidateExportOptions(request.Options); err != nil {
		return database.ExportStats{}, err
	}
	request.Table.Schema = m.defaultDatabase(request.Table.Schema)
	structures, err := m.GetCollectionStructures(request.Table)
	if err != nil {
		return database.ExportStats{}, err
	}

	projection := "*"
	insertColumns := make(database.Structures, 0, len(structures))
	if request.Options.Format == database.ExportFormatSQL {
		parts := make([]string, 0, len(structures))
		for _, column := range structures {
			if column.IsAutoInc ||
				(column.Default != nil &&
					strings.Contains(strings.ToUpper(*column.Default), "GENERATED")) {
				continue
			}
			insertColumns = append(insertColumns, column)
			quoted := quoteMySQLIdentifier(column.Name)
			parts = append(parts, "QUOTE("+quoted+") AS "+quoted)
		}
		if len(parts) == 0 {
			return database.ExportStats{}, fmt.Errorf(
				"table has no columns that can be exported as INSERT statements",
			)
		}
		projection = strings.Join(parts, ", ")
	}

	query, err := buildMySQLExportQuery(
		request.Table,
		request.Scope,
		structures,
		projection,
	)
	if err != nil {
		return database.ExportStats{}, err
	}
	rows, err := m.conn.QueryxContext(ctx, query.SQL, query.Args...)
	if err != nil {
		return database.ExportStats{}, err
	}
	defer rows.Close()

	var stream database.RowStream = &mysqlExportRows{rows: rows}
	if request.Scope == database.ExportScopeSelected {
		stream, err = newSelectedMySQLRows(
			stream,
			request.SelectedRowIndexes,
			request.Table.Limit,
		)
		if err != nil {
			return database.ExportStats{}, err
		}
	}
	if request.Options.Format == database.ExportFormatSQL {
		return writeMySQLInsertStream(
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

type mysqlInsertSink struct {
	writer          *bufio.Writer
	prefix          string
	batchSize       int
	rowsInBatch     int
	rows            int64
	includeUpsert   bool
	upsertStatement string
}

func newMySQLInsertSink(
	writer io.Writer,
	table database.Table,
	columns database.Structures,
	options database.SQLInsertOptions,
) *mysqlInsertSink {
	names := make([]string, len(columns))
	updates := make([]string, 0, len(columns))
	for index, column := range columns {
		names[index] = quoteMySQLIdentifier(column.Name)
		if !column.IsPrimary && !column.IsAutoInc {
			updates = append(
				updates,
				fmt.Sprintf(
					"%s = VALUES(%s)",
					quoteMySQLIdentifier(column.Name),
					quoteMySQLIdentifier(column.Name),
				),
			)
		}
	}
	upsert := ""
	if len(updates) > 0 {
		upsert = "\nON DUPLICATE KEY UPDATE " + strings.Join(updates, ", ")
	}
	return &mysqlInsertSink{
		writer: bufio.NewWriter(writer),
		prefix: fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES\n",
			quoteMySQLQualifiedIdentifier(table.Schema, table.Name),
			strings.Join(names, ", "),
		),
		batchSize:       options.EffectiveBatchSize(),
		includeUpsert:   options.Upsert && upsert != "",
		upsertStatement: upsert,
	}
}

func (sink *mysqlInsertSink) write(value string) error {
	written, err := sink.writer.WriteString(value)
	if err != nil {
		return err
	}
	if written != len(value) {
		return io.ErrShortWrite
	}
	return nil
}

func mysqlQuotedValue(value interface{}) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "NULL", nil
	case string:
		return typed, nil
	case []byte:
		if utf8.Valid(typed) {
			return string(typed), nil
		}
		return "_binary 0x" + hex.EncodeToString(typed), nil
	default:
		return "", fmt.Errorf("MySQL QUOTE returned unexpected value %T", value)
	}
}

func (sink *mysqlInsertSink) writeValues(values []interface{}) error {
	literals := make([]string, len(values))
	for index, value := range values {
		literal, err := mysqlQuotedValue(value)
		if err != nil {
			return err
		}
		literals[index] = literal
	}
	if sink.rowsInBatch == sink.batchSize {
		if sink.includeUpsert {
			if err := sink.write(sink.upsertStatement); err != nil {
				return err
			}
		}
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

func writeMySQLInsertStream(
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
	sink := newMySQLInsertSink(writer, table, columns, options)
	if err := sink.write(
		"-- " + application.Name + " MySQL / MariaDB INSERT export\n" +
			"-- Generated and auto-increment columns are omitted.\n\n",
	); err != nil {
		return database.ExportStats{}, err
	}
	if options.IncludeTransaction {
		if err := sink.write("START TRANSACTION;\n\n"); err != nil {
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

func (m *MySQL) CreateTable(
	table database.Table,
	columns []database.ColumnDefinition,
) error {
	if strings.TrimSpace(table.Name) == "" {
		return fmt.Errorf("table name is required")
	}
	if len(columns) == 0 {
		return fmt.Errorf("at least one column is required")
	}
	databaseName := m.defaultDatabase(table.Schema)
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
		if err := database.ValidateDDLFragment(
			dataType,
			"column data type",
		); err != nil {
			return err
		}
		definition := quoteMySQLIdentifier(name) + " " + dataType
		if !column.Nullable {
			definition += " NOT NULL"
		}
		if strings.TrimSpace(column.Default) != "" {
			if err := database.ValidateDDLFragment(
				column.Default,
				"column default",
			); err != nil {
				return err
			}
			definition += " DEFAULT " + strings.TrimSpace(column.Default)
		}
		if column.Unique {
			definition += " UNIQUE"
		}
		if column.PrimaryKey {
			primaryKeys = append(primaryKeys, quoteMySQLIdentifier(name))
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
	query := fmt.Sprintf(
		"CREATE TABLE %s (%s)",
		quoteMySQLQualifiedIdentifier(databaseName, table.Name),
		strings.Join(definitions, ", "),
	)
	_, err := m.conn.Exec(query)
	return err
}

func (m *MySQL) DropTable(table database.Table) error {
	if strings.TrimSpace(table.Name) == "" {
		return fmt.Errorf("table name is required")
	}
	_, err := m.conn.Exec(
		"DROP TABLE IF EXISTS " +
			quoteMySQLQualifiedIdentifier(m.defaultDatabase(table.Schema), table.Name),
	)
	return err
}

func (m *MySQL) TruncateTable(table database.Table) error {
	if strings.TrimSpace(table.Name) == "" {
		return fmt.Errorf("table name is required")
	}
	_, err := m.conn.Exec(
		"TRUNCATE TABLE " +
			quoteMySQLQualifiedIdentifier(m.defaultDatabase(table.Schema), table.Name),
	)
	return err
}

func (m *MySQL) GetTableDDL(table database.Table) (string, error) {
	if strings.TrimSpace(table.Name) == "" {
		return "", fmt.Errorf("table name is required")
	}
	rows, err := m.conn.Queryx(
		"SHOW CREATE TABLE " +
			quoteMySQLQualifiedIdentifier(m.defaultDatabase(table.Schema), table.Name),
	)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", fmt.Errorf("table %q was not found", table.Name)
	}
	values, err := rows.SliceScan()
	if err != nil {
		return "", err
	}
	if len(values) < 2 {
		return "", fmt.Errorf("SHOW CREATE TABLE returned no definition")
	}
	definition := fmt.Sprint(normalizeMySQLValue(values[1]))
	if !strings.HasSuffix(strings.TrimSpace(definition), ";") {
		definition += ";"
	}
	return definition, rows.Err()
}

func (m *MySQL) GetDataTypes() []database.DataType {
	return []database.DataType{
		{Name: "tinyint", Category: "Numeric", Description: "1-byte integer"},
		{Name: "smallint", Category: "Numeric", Description: "2-byte integer"},
		{Name: "int", Category: "Numeric", Description: "4-byte integer"},
		{Name: "bigint", Category: "Numeric", Description: "8-byte integer"},
		{Name: "decimal", Category: "Numeric", Description: "Exact fixed-point number"},
		{Name: "float", Category: "Numeric", Description: "Single-precision number"},
		{Name: "double", Category: "Numeric", Description: "Double-precision number"},
		{Name: "char", Category: "Character", Description: "Fixed-length text"},
		{Name: "varchar", Category: "Character", Description: "Variable-length text"},
		{Name: "text", Category: "Character", Description: "Long text"},
		{Name: "binary", Category: "Binary", Description: "Fixed-length bytes"},
		{Name: "varbinary", Category: "Binary", Description: "Variable-length bytes"},
		{Name: "blob", Category: "Binary", Description: "Binary large object"},
		{Name: "date", Category: "Date/Time", Description: "Calendar date"},
		{Name: "time", Category: "Date/Time", Description: "Time of day or duration"},
		{Name: "datetime", Category: "Date/Time", Description: "Date and time"},
		{Name: "timestamp", Category: "Date/Time", Description: "UTC-backed timestamp"},
		{Name: "json", Category: "JSON", Description: "JSON document"},
		{Name: "enum", Category: "Enum", Description: "One value from a declared set"},
		{Name: "set", Category: "Enum", Description: "Multiple values from a declared set"},
		{Name: "boolean", Category: "Boolean", Description: "Boolean alias for tinyint(1)"},
	}
}

func parseMySQLInt(value string) (int, error) {
	number, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q: %w", value, err)
	}
	return number, nil
}

var _ database.Driver = (*MySQL)(nil)
var _ database.DriverWithSchema = (*MySQL)(nil)
var _ database.TransactionalDriver = (*MySQL)(nil)

// Keep these compile-time references close to the connection code. They
// protect accidental removal of cancellation and error semantics while the
// engine implementation evolves.
var (
	_ = errors.Is
	_ = parseMySQLInt
)
