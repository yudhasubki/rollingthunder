package oracle

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"rollingthunder/pkg/application"
	"rollingthunder/pkg/database"
	"rollingthunder/pkg/database/sqladapter"

	go_ora "github.com/sijms/go-ora/v2"
	"github.com/sijms/go-ora/v2/configurations"
)

const (
	defaultConnectionTimeout  = 15 * time.Second
	defaultConnectionLifetime = 5 * time.Minute
	defaultMaxIdleConnections = 2
	defaultMaxOpenConnections = 8
)

type Config struct {
	Host           string
	Port           string
	User           string
	Password       string
	Db             string
	SSLMode        string
	SSLRootCert    string
	SSLCert        string
	SSLKey         string
	TLSServerName  string
	ConnectionMode string
	TNSConfigPath  string
	TNSAlias       string
	WalletPath     string
	WalletPassword string
}

type Oracle struct {
	cfg           Config
	ctx           context.Context
	conn          *sql.DB
	currentSchema string
}

func NewOracle(ctx context.Context, cfg Config) *Oracle {
	return &Oracle{cfg: cfg, ctx: ctx}
}

func normalizeConfig(config Config) (Config, int, error) {
	config.ConnectionMode = strings.ToLower(
		strings.TrimSpace(config.ConnectionMode),
	)
	if config.ConnectionMode == "" {
		config.ConnectionMode = "direct"
	}
	config.SSLMode = strings.ToLower(strings.TrimSpace(config.SSLMode))
	if config.SSLMode == "" {
		config.SSLMode = database.DefaultSSLMode
	}
	if config.ConnectionMode == "tns" {
		config.TNSConfigPath = strings.TrimSpace(config.TNSConfigPath)
		config.TNSAlias = strings.TrimSpace(config.TNSAlias)
		if config.TNSConfigPath == "" || config.TNSAlias == "" {
			return Config{}, 0, errors.New(
				"Oracle TNS mode requires a tnsnames.ora file and alias",
			)
		}
		return config, 0, nil
	}
	if config.ConnectionMode != "direct" {
		return Config{}, 0, fmt.Errorf(
			"unsupported Oracle connection mode %q",
			config.ConnectionMode,
		)
	}
	config.Host = strings.TrimSpace(config.Host)
	if config.Host == "" {
		config.Host = database.DefaultHost
	}
	config.Port = strings.TrimSpace(config.Port)
	if config.Port == "" {
		config.Port = database.DefaultOraclePort
	}
	port, err := strconv.Atoi(config.Port)
	if err != nil || port < 1 || port > 65535 {
		return Config{}, 0, fmt.Errorf(
			"Oracle port must be a number from 1 to 65535",
		)
	}
	config.Db = strings.TrimSpace(config.Db)
	if config.Db == "" {
		return Config{}, 0, errors.New("Oracle service name is required")
	}
	return config, port, nil
}

func rootCertificatePool(path string) (*x509.CertPool, error) {
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if strings.TrimSpace(path) == "" {
		return roots, nil
	}
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Oracle CA certificate: %w", err)
	}
	if !roots.AppendCertsFromPEM(pem) {
		return nil, errors.New("Oracle CA certificate contains no valid PEM certificate")
	}
	return roots, nil
}

func oracleTLSConfig(config Config) (*tls.Config, error) {
	if config.SSLMode == "disable" {
		return nil, nil
	}
	serverName := strings.TrimSpace(config.TLSServerName)
	if serverName == "" {
		serverName = config.Host
	}
	roots, err := rootCertificatePool(config.SSLRootCert)
	if err != nil {
		return nil, err
	}
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: serverName,
	}
	if config.SSLCert != "" || config.SSLKey != "" {
		if config.SSLCert == "" || config.SSLKey == "" {
			return nil, errors.New(
				"Oracle TLS client certificate and key must be provided together",
			)
		}
		certificate, loadErr := tls.LoadX509KeyPair(
			config.SSLCert,
			config.SSLKey,
		)
		if loadErr != nil {
			return nil, fmt.Errorf("load Oracle TLS client certificate: %w", loadErr)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	switch config.SSLMode {
	case "require":
		// Explicitly requested encryption without identity verification.
		tlsConfig.InsecureSkipVerify = true //nolint:gosec
	case "verify-ca":
		tlsConfig.InsecureSkipVerify = true //nolint:gosec
		tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return errors.New("Oracle TLS server returned no certificate")
			}
			intermediates := x509.NewCertPool()
			for _, certificate := range state.PeerCertificates[1:] {
				intermediates.AddCert(certificate)
			}
			_, verifyErr := state.PeerCertificates[0].Verify(x509.VerifyOptions{
				Roots:         roots,
				Intermediates: intermediates,
				KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			})
			return verifyErr
		}
	case "verify-full":
		if strings.TrimSpace(config.TLSServerName) != "" {
			// go-ora rewrites tls.Config.ServerName to the active network
			// endpoint. A tunnel endpoint is local, so preserve verification
			// against the reviewed database hostname explicitly.
			expectedName := serverName
			tlsConfig.InsecureSkipVerify = true //nolint:gosec
			tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
				if len(state.PeerCertificates) == 0 {
					return errors.New(
						"Oracle TLS server returned no certificate",
					)
				}
				intermediates := x509.NewCertPool()
				for _, certificate := range state.PeerCertificates[1:] {
					intermediates.AddCert(certificate)
				}
				_, verifyErr := state.PeerCertificates[0].Verify(
					x509.VerifyOptions{
						Roots:         roots,
						Intermediates: intermediates,
						DNSName:       expectedName,
						KeyUsages: []x509.ExtKeyUsage{
							x509.ExtKeyUsageServerAuth,
						},
					},
				)
				return verifyErr
			}
		}
	default:
		return nil, fmt.Errorf("unsupported Oracle TLS mode %q", config.SSLMode)
	}
	return tlsConfig, nil
}

func oracleWalletDirectory(config Config) (string, error) {
	walletPath := strings.TrimSpace(config.WalletPath)
	if walletPath == "" {
		return "", nil
	}
	if config.SSLMode == "verify-ca" {
		return "", errors.New(
			"Oracle Wallet supports require or verify-full TLS modes; CA-only verification is not supported by the Oracle driver",
		)
	}
	if config.SSLRootCert != "" || config.SSLCert != "" || config.SSLKey != "" {
		return "", errors.New(
			"Oracle Wallet cannot be combined with separate TLS certificate paths",
		)
	}
	info, err := InspectWalletDirectory(walletPath)
	if err != nil {
		return "", err
	}
	if config.WalletPassword != "" && !info.HasEWallet {
		return "", errors.New(
			"Oracle Wallet password requires ewallet.p12 in the selected directory",
		)
	}
	if config.WalletPassword == "" && !info.HasAutoLogin {
		return "", errors.New(
			"enter the Oracle Wallet password for ewallet.p12 or add cwallet.sso for auto-login",
		)
	}
	return info.Path, nil
}

func buildConnector(config Config) (*go_ora.OracleConnector, error) {
	normalized, port, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	options := map[string]string{
		"CONNECTION TIMEOUT": strconv.Itoa(int(defaultConnectionTimeout.Seconds())),
		"PROGRAM":            application.DatabaseClientName,
	}
	descriptor := ""
	if normalized.ConnectionMode == "tns" {
		descriptor, err = resolveTNSAlias(
			normalized.TNSConfigPath,
			normalized.TNSAlias,
		)
		if err != nil {
			return nil, err
		}
		servers, extractErr := configurations.ExtractServers(descriptor)
		if extractErr != nil || len(servers) == 0 {
			if extractErr == nil {
				extractErr = errors.New(
					"TNS descriptor contains no supported ADDRESS",
				)
			}
			return nil, fmt.Errorf(
				"inspect Oracle TNS descriptor: %w",
				extractErr,
			)
		}
		normalized.Host = servers[0].Addr
		compactDescriptor := strings.Join(
			strings.Fields(strings.ToUpper(descriptor)),
			"",
		)
		if strings.Contains(compactDescriptor, "(PROTOCOL=TCPS)") &&
			normalized.SSLMode == "disable" {
			return nil, errors.New(
				"the selected TNS alias uses TCPS; choose a TLS verification mode",
			)
		}
	}
	walletPath, err := oracleWalletDirectory(normalized)
	if err != nil {
		return nil, err
	}
	var tlsConfig *tls.Config
	if walletPath == "" {
		tlsConfig, err = oracleTLSConfig(normalized)
		if err != nil {
			return nil, err
		}
	} else {
		options["WALLET"] = walletPath
		if normalized.WalletPassword != "" {
			options["WALLET PASSWORD"] = normalized.WalletPassword
		}
	}
	if normalized.SSLMode != "disable" {
		options["SSL"] = "true"
		options["SSL VERIFY"] = strconv.FormatBool(normalized.SSLMode != "require")
	}
	dsn := ""
	if normalized.ConnectionMode == "tns" {
		dsn = go_ora.BuildJDBC(
			normalized.User,
			normalized.Password,
			descriptor,
			options,
		)
	} else {
		dsn = go_ora.BuildUrl(
			normalized.Host,
			port,
			normalized.Db,
			normalized.User,
			normalized.Password,
			options,
		)
	}
	connector, ok := go_ora.NewConnector(dsn).(*go_ora.OracleConnector)
	if !ok {
		return nil, errors.New("Oracle driver returned an incompatible connector")
	}
	if tlsConfig != nil {
		connector.WithTLSConfig(tlsConfig)
	}
	return connector, nil
}

func (o *Oracle) Connect(ctx context.Context) error {
	if ctx == nil {
		ctx = o.ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if o.conn != nil {
		if err := o.Close(); err != nil {
			return fmt.Errorf("close previous Oracle connection: %w", err)
		}
	}
	connector, err := buildConnector(o.cfg)
	if err != nil {
		return err
	}
	connection := sql.OpenDB(connector)
	connection.SetMaxIdleConns(defaultMaxIdleConnections)
	connection.SetMaxOpenConns(defaultMaxOpenConnections)
	connection.SetConnMaxLifetime(defaultConnectionLifetime)
	if err := connection.PingContext(ctx); err != nil {
		_ = connection.Close()
		return fmt.Errorf("connect to Oracle Database: %w", err)
	}
	var schema string
	if err := connection.QueryRowContext(
		ctx,
		"SELECT SYS_CONTEXT('USERENV', 'CURRENT_SCHEMA') FROM dual",
	).Scan(&schema); err != nil {
		_ = connection.Close()
		return fmt.Errorf("read Oracle current schema: %w", err)
	}
	o.conn = connection
	o.ctx = context.WithoutCancel(ctx)
	o.currentSchema = schema
	return nil
}

func (o *Oracle) Close() error {
	if o.conn == nil {
		return nil
	}
	err := o.conn.Close()
	o.conn = nil
	o.currentSchema = ""
	return err
}

func (o *Oracle) Ping(ctx context.Context) error {
	if err := o.ensureConnected(); err != nil {
		return err
	}
	return o.conn.PingContext(ctx)
}

func (o *Oracle) ensureConnected() error {
	if o.conn == nil {
		return errors.New("Oracle connection is not open")
	}
	return nil
}

func (o *Oracle) defaultSchema(schema string) string {
	if strings.TrimSpace(schema) != "" {
		return schema
	}
	if o.currentSchema != "" {
		return o.currentSchema
	}
	return strings.ToUpper(strings.TrimSpace(o.cfg.User))
}

func (o *Oracle) CountCollectionData(table database.Table) (int, error) {
	if err := o.ensureConnected(); err != nil {
		return 0, err
	}
	table.Schema = o.defaultSchema(table.Schema)
	structures, err := o.GetCollectionStructures(table)
	if err != nil {
		return 0, err
	}
	return sqladapter.CountTable(o.conn, table, structures, o.adapterDialect())
}

func (o *Oracle) GetCollectionData(
	table database.Table,
) (database.Structures, []map[string]interface{}, error) {
	if err := o.ensureConnected(); err != nil {
		return nil, nil, err
	}
	table.Schema = o.defaultSchema(table.Schema)
	structures, err := o.GetCollectionStructures(table)
	if err != nil {
		return nil, nil, err
	}
	rows, err := sqladapter.GetTableData(
		o.ctx,
		o.conn,
		table,
		structures,
		o.adapterDialect(),
	)
	return structures, rows, err
}

func (o *Oracle) InsertRow(
	table database.Table,
	data map[string]interface{},
) error {
	if err := o.ensureConnected(); err != nil {
		return err
	}
	table.Schema = o.defaultSchema(table.Schema)
	structures, err := o.GetCollectionStructures(table)
	if err != nil {
		return err
	}
	return sqladapter.InsertRowWithStructures(
		o.conn,
		table,
		data,
		structures,
		o.adapterDialect(),
	)
}

func (o *Oracle) UpdateRow(
	table database.Table,
	data map[string]interface{},
	primaryKey string,
) error {
	if err := o.ensureConnected(); err != nil {
		return err
	}
	table.Schema = o.defaultSchema(table.Schema)
	structures, err := o.GetCollectionStructures(table)
	if err != nil {
		return err
	}
	return sqladapter.UpdateRowWithStructures(
		o.conn,
		table,
		data,
		primaryKey,
		structures,
		o.adapterDialect(),
	)
}

func (o *Oracle) DeleteRow(
	table database.Table,
	primaryKey string,
	primaryValue interface{},
) error {
	if err := o.ensureConnected(); err != nil {
		return err
	}
	table.Schema = o.defaultSchema(table.Schema)
	return sqladapter.DeleteRow(
		o.conn,
		table,
		primaryKey,
		primaryValue,
		o.adapterDialect(),
	)
}

func (o *Oracle) ExecuteQuery(
	ctx context.Context,
	query string,
	options database.QueryOptions,
) (database.QueryResult, error) {
	if err := o.ensureConnected(); err != nil {
		return database.QueryResult{}, err
	}
	return sqladapter.ExecuteQuery(ctx, o.conn, query, options)
}

type oracleTransaction struct {
	tx *sql.Tx
}

func (transaction *oracleTransaction) ExecuteQuery(
	ctx context.Context,
	query string,
	options database.QueryOptions,
) (database.QueryResult, error) {
	return sqladapter.ExecuteQuery(ctx, transaction.tx, query, options)
}

func (transaction *oracleTransaction) Commit() error {
	return transaction.tx.Commit()
}

func (transaction *oracleTransaction) Rollback() error {
	return transaction.tx.Rollback()
}

func (o *Oracle) BeginTransaction(
	ctx context.Context,
) (database.Transaction, error) {
	if err := o.ensureConnected(); err != nil {
		return nil, err
	}
	transaction, err := o.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &oracleTransaction{tx: transaction}, nil
}

func (o *Oracle) ApplyTableChanges(
	ctx context.Context,
	changes database.TableChangeSet,
) (database.TableChangeResult, error) {
	if err := o.ensureConnected(); err != nil {
		return database.TableChangeResult{}, err
	}
	changes.Table.Schema = o.defaultSchema(changes.Table.Schema)
	structures, err := o.GetCollectionStructures(changes.Table)
	if err != nil {
		return database.TableChangeResult{}, err
	}
	return sqladapter.ApplyTableChanges(
		ctx,
		o.conn,
		changes,
		structures,
		o.adapterDialect(),
	)
}

func (o *Oracle) ExportTable(
	ctx context.Context,
	request database.TableExportRequest,
	writer io.Writer,
) (database.ExportStats, error) {
	if err := o.ensureConnected(); err != nil {
		return database.ExportStats{}, err
	}
	request.Table.Schema = o.defaultSchema(request.Table.Schema)
	structures, err := o.GetCollectionStructures(request.Table)
	if err != nil {
		return database.ExportStats{}, err
	}
	return sqladapter.ExportTable(
		ctx,
		o.conn,
		request,
		structures,
		o.adapterDialect(),
		writer,
	)
}
