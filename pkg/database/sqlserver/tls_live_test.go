package sqlserver

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"rollingthunder/pkg/database"
)

const (
	sqlServerTLSConnectTimeout = 20 * time.Second
	sqlServerTDS74Version      = "74000004"
	sqlServerTDS80Version      = "08000000"
)

type sqlServerTLSState struct {
	Encrypted       string
	Transport       string
	Protocol        string
	ProtocolVersion string
	AuthScheme      string
	SessionUser     string
}

func requiredSQLServerTLSEnvironment(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		if os.Getenv("ROLLINGTHUNDER_SQLSERVER_TEST_REQUIRE_TLS") == "1" {
			t.Fatalf("required SQL Server TLS test variable %s is empty", name)
		}
		t.Skip("SQL Server TLS live test environment is not configured")
	}
	return value
}

func sqlServerTLSLiveConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		Host: requiredSQLServerTLSEnvironment(
			t,
			"ROLLINGTHUNDER_SQLSERVER_TEST_HOST",
		),
		Port: requiredSQLServerTLSEnvironment(
			t,
			"ROLLINGTHUNDER_SQLSERVER_TEST_PORT",
		),
		User: requiredSQLServerTLSEnvironment(
			t,
			"ROLLINGTHUNDER_SQLSERVER_TEST_USER",
		),
		Password: os.Getenv("ROLLINGTHUNDER_SQLSERVER_TEST_PASSWORD"),
		Db: requiredSQLServerTLSEnvironment(
			t,
			"ROLLINGTHUNDER_SQLSERVER_TEST_DATABASE",
		),
		AuthMode: database.SQLServerAuthSQL,
	}
}

func connectSQLServerTLS(config Config) (*SQLServer, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		sqlServerTLSConnectTimeout,
	)
	defer cancel()
	driver := NewSQLServer(context.Background(), config)
	if err := driver.Connect(ctx); err != nil {
		return nil, err
	}
	return driver, nil
}

func readSQLServerTLSState(
	t *testing.T,
	driver *SQLServer,
) sqlServerTLSState {
	t.Helper()
	ctx, cancel := context.WithTimeout(
		context.Background(),
		sqlServerTLSConnectTimeout,
	)
	defer cancel()
	var state sqlServerTLSState
	if err := driver.conn.QueryRowContext(
		ctx,
		`SELECT
			CONVERT(nvarchar(16), connection_value.encrypt_option),
			CONVERT(nvarchar(32), connection_value.net_transport),
			CONVERT(nvarchar(32), connection_value.protocol_type),
			CONVERT(
				char(8),
				CONVERT(varbinary(4), connection_value.protocol_version),
				2
			),
			CONVERT(nvarchar(32), connection_value.auth_scheme),
			CONVERT(nvarchar(128), SUSER_SNAME())
		 FROM sys.dm_exec_connections AS connection_value
		 WHERE connection_value.session_id = @@SPID`,
	).Scan(
		&state.Encrypted,
		&state.Transport,
		&state.Protocol,
		&state.ProtocolVersion,
		&state.AuthScheme,
		&state.SessionUser,
	); err != nil {
		t.Fatalf("read SQL Server TLS session state: %v", err)
	}
	if !strings.EqualFold(strings.TrimSpace(state.Encrypted), "TRUE") {
		t.Fatalf("SQL Server session is not encrypted: %+v", state)
	}
	if !strings.EqualFold(strings.TrimSpace(state.Transport), "TCP") ||
		!strings.EqualFold(strings.TrimSpace(state.Protocol), "TSQL") ||
		len(strings.TrimSpace(state.ProtocolVersion)) != 8 ||
		strings.TrimSpace(state.AuthScheme) == "" ||
		strings.TrimSpace(state.SessionUser) == "" {
		t.Fatalf("SQL Server TLS session metadata is incomplete: %+v", state)
	}
	return state
}

func assertSQLServerTLSConnection(
	t *testing.T,
	config Config,
) sqlServerTLSState {
	t.Helper()
	driver, err := connectSQLServerTLS(config)
	if err != nil {
		t.Fatalf("connect to SQL Server over TLS: %v", err)
	}
	defer func() {
		if err := driver.Close(); err != nil {
			t.Errorf("close SQL Server TLS connection: %v", err)
		}
	}()
	return readSQLServerTLSState(t, driver)
}

func assertSQLServerTLSConnectionFails(t *testing.T, config Config) {
	t.Helper()
	driver, err := connectSQLServerTLS(config)
	if err != nil {
		return
	}
	_ = driver.Close()
	t.Fatal("SQL Server accepted a TLS connection that must be rejected")
}

func assertSQLServerTDSVersion(
	t *testing.T,
	state sqlServerTLSState,
	want string,
) {
	t.Helper()
	if !strings.EqualFold(strings.TrimSpace(state.ProtocolVersion), want) {
		t.Fatalf(
			"connection negotiated TDS protocol %s, want %s",
			state.ProtocolVersion,
			want,
		)
	}
}

func TestSQLServerTLSLiveConformance(t *testing.T) {
	base := sqlServerTLSLiveConfig(t)
	rootCert := requiredSQLServerTLSEnvironment(
		t,
		"ROLLINGTHUNDER_SQLSERVER_TEST_TLS_ROOT_CERT",
	)
	wrongRootCert := requiredSQLServerTLSEnvironment(
		t,
		"ROLLINGTHUNDER_SQLSERVER_TEST_TLS_WRONG_ROOT_CERT",
	)
	serverName := requiredSQLServerTLSEnvironment(
		t,
		"ROLLINGTHUNDER_SQLSERVER_TEST_TLS_SERVER_NAME",
	)

	t.Run("plaintext_rejected", func(t *testing.T) {
		config := base
		config.SSLMode = "disable"
		assertSQLServerTLSConnectionFails(t, config)
	})
	t.Run("require_encrypts_session", func(t *testing.T) {
		config := base
		config.SSLMode = "require"
		state := assertSQLServerTLSConnection(t, config)
		assertSQLServerTDSVersion(t, state, sqlServerTDS74Version)
	})
	t.Run("verify_ca", func(t *testing.T) {
		config := base
		config.SSLMode = "verify-ca"
		config.SSLRootCert = rootCert
		config.TLSServerName = serverName
		state := assertSQLServerTLSConnection(t, config)
		assertSQLServerTDSVersion(t, state, sqlServerTDS74Version)
	})
	t.Run("verify_full", func(t *testing.T) {
		config := base
		config.SSLMode = "verify-full"
		config.SSLRootCert = rootCert
		config.TLSServerName = serverName
		state := assertSQLServerTLSConnection(t, config)
		assertSQLServerTDSVersion(t, state, sqlServerTDS74Version)
	})
	t.Run("verify_full_wrong_ca_rejected", func(t *testing.T) {
		config := base
		config.SSLMode = "verify-full"
		config.SSLRootCert = wrongRootCert
		config.TLSServerName = serverName
		assertSQLServerTLSConnectionFails(t, config)
	})
	t.Run("verify_full_wrong_server_name_rejected", func(t *testing.T) {
		config := base
		config.SSLMode = "verify-full"
		config.SSLRootCert = rootCert
		config.TLSServerName = "wrong-host.invalid"
		assertSQLServerTLSConnectionFails(t, config)
	})
	t.Run("strict", func(t *testing.T) {
		if os.Getenv("ROLLINGTHUNDER_SQLSERVER_TEST_TDS8_STRICT") != "1" {
			t.Skip(
				"TDS 8.0 Strict live fixture is not enabled for this SQL Server platform",
			)
		}

		// Establishing a valid Strict connection first prevents the negative
		// cases from passing merely because the server rejects every TDS 8.0
		// handshake.
		config := base
		config.SSLMode = "strict"
		config.SSLRootCert = rootCert
		config.TLSServerName = serverName
		state := assertSQLServerTLSConnection(t, config)
		assertSQLServerTDSVersion(t, state, sqlServerTDS80Version)

		t.Run("wrong_ca_rejected", func(t *testing.T) {
			config := base
			config.SSLMode = "strict"
			config.SSLRootCert = wrongRootCert
			config.TLSServerName = serverName
			assertSQLServerTLSConnectionFails(t, config)
		})
		t.Run("wrong_server_name_rejected", func(t *testing.T) {
			config := base
			config.SSLMode = "strict"
			config.SSLRootCert = rootCert
			config.TLSServerName = "wrong-host.invalid"
			assertSQLServerTLSConnectionFails(t, config)
		})
	})
}
