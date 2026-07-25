package sqlserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"rollingthunder/pkg/application"
	"rollingthunder/pkg/database"
	"rollingthunder/pkg/database/sqladapter"

	_ "github.com/microsoft/go-mssqldb"
)

const (
	defaultConnectionTimeout  = 15 * time.Second
	defaultConnectionLifetime = 5 * time.Minute
	defaultMaxIdleConnections = 2
	defaultMaxOpenConnections = 8
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

type SQLServer struct {
	cfg           Config
	ctx           context.Context
	conn          *sql.DB
	currentSchema string
}

func NewSQLServer(ctx context.Context, cfg Config) *SQLServer {
	return &SQLServer{cfg: cfg, ctx: ctx}
}

func normalizeConfig(config Config) (Config, error) {
	config.Host = strings.TrimSpace(config.Host)
	if config.Host == "" {
		config.Host = database.DefaultHost
	}
	config.Port = strings.TrimSpace(config.Port)
	if config.Port == "" {
		config.Port = database.DefaultSQLServerPort
	}
	port, err := strconv.Atoi(config.Port)
	if err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf(
			"SQL Server port must be a number from 1 to 65535",
		)
	}
	config.Db = strings.TrimSpace(config.Db)
	if config.Db == "" {
		return Config{}, errors.New("SQL Server database name is required")
	}
	config.SSLMode = strings.ToLower(strings.TrimSpace(config.SSLMode))
	if config.SSLMode == "" {
		config.SSLMode = database.DefaultSSLMode
	}
	if config.SSLCert != "" || config.SSLKey != "" {
		return Config{}, errors.New(
			"SQL Server does not use TLS client certificate authentication",
		)
	}
	if config.SSLRootCert != "" {
		if info, statErr := os.Stat(config.SSLRootCert); statErr != nil {
			return Config{}, fmt.Errorf(
				"access SQL Server CA certificate: %w",
				statErr,
			)
		} else if info.IsDir() {
			return Config{}, errors.New(
				"SQL Server CA certificate path must point to a PEM file",
			)
		}
	}
	return config, nil
}

func buildConnectionURL(config Config) (string, error) {
	config, err := normalizeConfig(config)
	if err != nil {
		return "", err
	}
	query := url.Values{
		"app name":           []string{application.DatabaseClientName},
		"connection timeout": []string{strconv.Itoa(int(defaultConnectionTimeout.Seconds()))},
		"database":           []string{config.Db},
		"dial timeout":       []string{strconv.Itoa(int(defaultConnectionTimeout.Seconds()))},
	}
	switch config.SSLMode {
	case "disable":
		query.Set("encrypt", "disable")
	case "require":
		query.Set("encrypt", "true")
		query.Set("TrustServerCertificate", "true")
	case "verify-ca", "verify-full":
		query.Set("encrypt", "true")
		query.Set("TrustServerCertificate", "false")
		serverName := strings.TrimSpace(config.TLSServerName)
		if serverName == "" {
			serverName = config.Host
		}
		query.Set("hostNameInCertificate", serverName)
		if config.SSLRootCert != "" {
			query.Set("certificate", config.SSLRootCert)
		}
	default:
		return "", fmt.Errorf("unsupported SQL Server TLS mode %q", config.SSLMode)
	}
	connectionURL := &url.URL{
		Scheme:   "sqlserver",
		Host:     net.JoinHostPort(config.Host, config.Port),
		RawQuery: query.Encode(),
	}
	if config.User != "" || config.Password != "" {
		connectionURL.User = url.UserPassword(config.User, config.Password)
	}
	return connectionURL.String(), nil
}

func (s *SQLServer) Connect(ctx context.Context) error {
	if ctx == nil {
		ctx = s.ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if s.conn != nil {
		if err := s.Close(); err != nil {
			return fmt.Errorf("close previous SQL Server connection: %w", err)
		}
	}
	dsn, err := buildConnectionURL(s.cfg)
	if err != nil {
		return err
	}
	connection, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return fmt.Errorf("initialize SQL Server connection: %w", err)
	}
	connection.SetMaxIdleConns(defaultMaxIdleConnections)
	connection.SetMaxOpenConns(defaultMaxOpenConnections)
	connection.SetConnMaxLifetime(defaultConnectionLifetime)
	if err := connection.PingContext(ctx); err != nil {
		_ = connection.Close()
		return fmt.Errorf("connect to SQL Server: %w", err)
	}
	var schema string
	if err := connection.QueryRowContext(
		ctx,
		"SELECT COALESCE(SCHEMA_NAME(), 'dbo')",
	).Scan(&schema); err != nil {
		_ = connection.Close()
		return fmt.Errorf("read SQL Server default schema: %w", err)
	}
	s.conn = connection
	s.ctx = context.WithoutCancel(ctx)
	s.currentSchema = schema
	return nil
}

func (s *SQLServer) Close() error {
	if s.conn == nil {
		return nil
	}
	err := s.conn.Close()
	s.conn = nil
	s.currentSchema = ""
	return err
}

func (s *SQLServer) Ping(ctx context.Context) error {
	if err := s.ensureConnected(); err != nil {
		return err
	}
	return s.conn.PingContext(ctx)
}

func (s *SQLServer) ensureConnected() error {
	if s.conn == nil {
		return errors.New("SQL Server connection is not open")
	}
	return nil
}

func (s *SQLServer) defaultSchema(schema string) string {
	if strings.TrimSpace(schema) != "" {
		return schema
	}
	if s.currentSchema != "" {
		return s.currentSchema
	}
	return "dbo"
}

func (s *SQLServer) CountCollectionData(table database.Table) (int, error) {
	if err := s.ensureConnected(); err != nil {
		return 0, err
	}
	table.Schema = s.defaultSchema(table.Schema)
	structures, err := s.GetCollectionStructures(table)
	if err != nil {
		return 0, err
	}
	return sqladapter.CountTable(s.conn, table, structures, s.adapterDialect())
}

func (s *SQLServer) GetCollectionData(
	table database.Table,
) (database.Structures, []map[string]interface{}, error) {
	if err := s.ensureConnected(); err != nil {
		return nil, nil, err
	}
	table.Schema = s.defaultSchema(table.Schema)
	structures, err := s.GetCollectionStructures(table)
	if err != nil {
		return nil, nil, err
	}
	rows, err := sqladapter.GetTableData(
		s.ctx,
		s.conn,
		table,
		structures,
		s.adapterDialect(),
	)
	return structures, rows, err
}

func (s *SQLServer) InsertRow(
	table database.Table,
	data map[string]interface{},
) error {
	if err := s.ensureConnected(); err != nil {
		return err
	}
	table.Schema = s.defaultSchema(table.Schema)
	structures, err := s.GetCollectionStructures(table)
	if err != nil {
		return err
	}
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	_, err = sqladapter.ApplyTableChanges(
		ctx,
		s.conn,
		database.TableChangeSet{
			Table: table,
			Added: []map[string]interface{}{data},
		},
		structures,
		s.adapterDialect(),
	)
	return err
}

func (s *SQLServer) UpdateRow(
	table database.Table,
	data map[string]interface{},
	primaryKey string,
) error {
	if err := s.ensureConnected(); err != nil {
		return err
	}
	table.Schema = s.defaultSchema(table.Schema)
	structures, err := s.GetCollectionStructures(table)
	if err != nil {
		return err
	}
	return sqladapter.UpdateRowWithStructures(
		s.conn,
		table,
		data,
		primaryKey,
		structures,
		s.adapterDialect(),
	)
}

func (s *SQLServer) DeleteRow(
	table database.Table,
	primaryKey string,
	primaryValue interface{},
) error {
	if err := s.ensureConnected(); err != nil {
		return err
	}
	table.Schema = s.defaultSchema(table.Schema)
	return sqladapter.DeleteRow(
		s.conn,
		table,
		primaryKey,
		primaryValue,
		s.adapterDialect(),
	)
}

func (s *SQLServer) ExecuteQuery(
	ctx context.Context,
	query string,
	options database.QueryOptions,
) (database.QueryResult, error) {
	if err := s.ensureConnected(); err != nil {
		return database.QueryResult{}, err
	}
	return sqladapter.ExecuteQuery(ctx, s.conn, query, options)
}

type sqlServerTransaction struct {
	tx *sql.Tx
}

func (transaction *sqlServerTransaction) ExecuteQuery(
	ctx context.Context,
	query string,
	options database.QueryOptions,
) (database.QueryResult, error) {
	return sqladapter.ExecuteQuery(ctx, transaction.tx, query, options)
}

func (transaction *sqlServerTransaction) Commit() error {
	return transaction.tx.Commit()
}

func (transaction *sqlServerTransaction) Rollback() error {
	return transaction.tx.Rollback()
}

func (s *SQLServer) BeginTransaction(
	ctx context.Context,
) (database.Transaction, error) {
	if err := s.ensureConnected(); err != nil {
		return nil, err
	}
	transaction, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &sqlServerTransaction{tx: transaction}, nil
}

func (s *SQLServer) ApplyTableChanges(
	ctx context.Context,
	changes database.TableChangeSet,
) (database.TableChangeResult, error) {
	if err := s.ensureConnected(); err != nil {
		return database.TableChangeResult{}, err
	}
	changes.Table.Schema = s.defaultSchema(changes.Table.Schema)
	structures, err := s.GetCollectionStructures(changes.Table)
	if err != nil {
		return database.TableChangeResult{}, err
	}
	return sqladapter.ApplyTableChanges(
		ctx,
		s.conn,
		changes,
		structures,
		s.adapterDialect(),
	)
}

func (s *SQLServer) ExportTable(
	ctx context.Context,
	request database.TableExportRequest,
	writer io.Writer,
) (database.ExportStats, error) {
	if err := s.ensureConnected(); err != nil {
		return database.ExportStats{}, err
	}
	request.Table.Schema = s.defaultSchema(request.Table.Schema)
	structures, err := s.GetCollectionStructures(request.Table)
	if err != nil {
		return database.ExportStats{}, err
	}
	return sqladapter.ExportTable(
		ctx,
		s.conn,
		request,
		structures,
		s.adapterDialect(),
		writer,
	)
}
