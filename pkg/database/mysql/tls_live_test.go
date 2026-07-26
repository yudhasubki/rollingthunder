package mysql

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

const mysqlTLSConnectTimeout = 10 * time.Second

var mysqlTLSUserPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

type mysqlTLSState struct {
	Cipher      string
	CurrentUser string
}

func requiredMySQLTLSEnvironment(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		if os.Getenv("ROLLINGTHUNDER_MYSQL_TEST_REQUIRE_TLS") == "1" {
			t.Fatalf("required MySQL TLS test variable %s is empty", name)
		}
		t.Skip("MySQL TLS live test environment is not configured")
	}
	return value
}

func mysqlTLSLiveConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		Host: requiredMySQLTLSEnvironment(
			t,
			"ROLLINGTHUNDER_MYSQL_TEST_HOST",
		),
		Port: requiredMySQLTLSEnvironment(
			t,
			"ROLLINGTHUNDER_MYSQL_TEST_PORT",
		),
		User: requiredMySQLTLSEnvironment(
			t,
			"ROLLINGTHUNDER_MYSQL_TEST_USER",
		),
		Password: os.Getenv("ROLLINGTHUNDER_MYSQL_TEST_PASSWORD"),
		Db: requiredMySQLTLSEnvironment(
			t,
			"ROLLINGTHUNDER_MYSQL_TEST_DATABASE",
		),
	}
}

func connectMySQLTLS(config Config) (*MySQL, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		mysqlTLSConnectTimeout,
	)
	defer cancel()
	driver := NewMySQL(context.Background(), config)
	if err := driver.Connect(ctx); err != nil {
		return nil, err
	}
	return driver, nil
}

func readMySQLTLSState(t *testing.T, driver *MySQL) mysqlTLSState {
	t.Helper()
	ctx, cancel := context.WithTimeout(
		context.Background(),
		mysqlTLSConnectTimeout,
	)
	defer cancel()
	var (
		statusName string
		state      mysqlTLSState
	)
	if err := driver.conn.QueryRowxContext(
		ctx,
		"SHOW SESSION STATUS LIKE 'Ssl_cipher'",
	).Scan(&statusName, &state.Cipher); err != nil {
		t.Fatalf("read MySQL TLS cipher: %v", err)
	}
	if !strings.EqualFold(statusName, "Ssl_cipher") ||
		strings.TrimSpace(state.Cipher) == "" {
		t.Fatalf("MySQL session is not encrypted: %+v", state)
	}
	if err := driver.conn.GetContext(
		ctx,
		&state.CurrentUser,
		"SELECT CURRENT_USER()",
	); err != nil {
		t.Fatalf("read MySQL TLS session user: %v", err)
	}
	return state
}

func assertMySQLTLSConnection(
	t *testing.T,
	config Config,
) mysqlTLSState {
	t.Helper()
	driver, err := connectMySQLTLS(config)
	if err != nil {
		t.Fatalf("connect to MySQL over TLS: %v", err)
	}
	defer func() {
		if err := driver.Close(); err != nil {
			t.Errorf("close MySQL TLS connection: %v", err)
		}
	}()
	return readMySQLTLSState(t, driver)
}

func assertMySQLTLSConnectionFails(t *testing.T, config Config) {
	t.Helper()
	driver, err := connectMySQLTLS(config)
	if err != nil {
		return
	}
	_ = driver.Close()
	t.Fatal("MySQL accepted a TLS connection that must be rejected")
}

func TestMySQLTLSLiveConformance(t *testing.T) {
	base := mysqlTLSLiveConfig(t)
	rootCert := requiredMySQLTLSEnvironment(
		t,
		"ROLLINGTHUNDER_MYSQL_TEST_TLS_ROOT_CERT",
	)
	wrongRootCert := requiredMySQLTLSEnvironment(
		t,
		"ROLLINGTHUNDER_MYSQL_TEST_TLS_WRONG_ROOT_CERT",
	)
	clientCert := requiredMySQLTLSEnvironment(
		t,
		"ROLLINGTHUNDER_MYSQL_TEST_TLS_CLIENT_CERT",
	)
	clientKey := requiredMySQLTLSEnvironment(
		t,
		"ROLLINGTHUNDER_MYSQL_TEST_TLS_CLIENT_KEY",
	)
	clientUser := requiredMySQLTLSEnvironment(
		t,
		"ROLLINGTHUNDER_MYSQL_TEST_TLS_CLIENT_USER",
	)
	serverName := requiredMySQLTLSEnvironment(
		t,
		"ROLLINGTHUNDER_MYSQL_TEST_TLS_SERVER_NAME",
	)

	t.Run("plaintext_rejected", func(t *testing.T) {
		config := base
		config.SSLMode = "disable"
		assertMySQLTLSConnectionFails(t, config)
	})
	t.Run("require_encrypts_session", func(t *testing.T) {
		config := base
		config.SSLMode = "require"
		assertMySQLTLSConnection(t, config)
	})
	t.Run("verify_ca", func(t *testing.T) {
		config := base
		config.SSLMode = "verify-ca"
		config.SSLRootCert = rootCert
		assertMySQLTLSConnection(t, config)
	})
	t.Run("verify_full", func(t *testing.T) {
		config := base
		config.SSLMode = "verify-full"
		config.SSLRootCert = rootCert
		config.TLSServerName = serverName
		assertMySQLTLSConnection(t, config)
	})
	t.Run("wrong_ca_rejected", func(t *testing.T) {
		config := base
		config.SSLMode = "verify-ca"
		config.SSLRootCert = wrongRootCert
		assertMySQLTLSConnectionFails(t, config)
	})
	t.Run("wrong_server_name_rejected", func(t *testing.T) {
		config := base
		config.SSLMode = "verify-full"
		config.SSLRootCert = rootCert
		config.TLSServerName = "wrong-host.invalid"
		assertMySQLTLSConnectionFails(t, config)
	})
	t.Run("client_certificate_required", func(t *testing.T) {
		if !mysqlTLSUserPattern.MatchString(clientUser) {
			t.Fatalf("unsafe MySQL TLS client user %q", clientUser)
		}
		adminConfig := base
		adminConfig.SSLMode = "verify-full"
		adminConfig.SSLRootCert = rootCert
		adminConfig.TLSServerName = serverName
		admin, err := connectMySQLTLS(adminConfig)
		if err != nil {
			t.Fatalf("connect MySQL TLS test administrator: %v", err)
		}
		defer func() {
			if err := admin.Close(); err != nil {
				t.Errorf("close MySQL TLS test administrator: %v", err)
			}
		}()

		account := quoteMySQLLiteral(clientUser) + "@'%'"
		dropUser := func() {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				mysqlTLSConnectTimeout,
			)
			defer cancel()
			_, _ = admin.conn.ExecContext(
				ctx,
				"DROP USER IF EXISTS "+account,
			)
		}
		dropUser()
		defer dropUser()

		ctx, cancel := context.WithTimeout(
			context.Background(),
			mysqlTLSConnectTimeout,
		)
		defer cancel()
		if _, err := admin.conn.ExecContext(
			ctx,
			"CREATE USER "+account+" REQUIRE X509",
		); err != nil {
			t.Fatalf("create MySQL certificate user: %v", err)
		}
		if _, err := admin.conn.ExecContext(
			ctx,
			"GRANT SELECT ON "+
				quoteMySQLIdentifier(base.Db)+".* TO "+account,
		); err != nil {
			t.Fatalf("grant MySQL certificate user access: %v", err)
		}

		clientConfig := base
		clientConfig.User = clientUser
		clientConfig.Password = ""
		clientConfig.SSLMode = "verify-full"
		clientConfig.SSLRootCert = rootCert
		clientConfig.TLSServerName = serverName
		assertMySQLTLSConnectionFails(t, clientConfig)

		clientConfig.SSLCert = clientCert
		clientConfig.SSLKey = clientKey
		state := assertMySQLTLSConnection(t, clientConfig)
		if !strings.HasPrefix(
			strings.ToLower(state.CurrentUser),
			strings.ToLower(clientUser)+"@",
		) {
			t.Fatalf(
				"MySQL certificate session user = %q, want %s",
				state.CurrentUser,
				clientUser,
			)
		}
	})
}
