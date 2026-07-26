package postgres

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

const postgresTLSConnectTimeout = 10 * time.Second

var postgresTLSRolePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

type postgresTLSState struct {
	Encrypted bool   `db:"ssl"`
	Version   string `db:"version"`
	Cipher    string `db:"cipher"`
	ClientDN  string `db:"client_dn"`
}

func requiredPostgresTLSEnvironment(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		if os.Getenv("ROLLINGTHUNDER_POSTGRES_TEST_REQUIRE_TLS") == "1" {
			t.Fatalf("required PostgreSQL TLS test variable %s is empty", name)
		}
		t.Skip("PostgreSQL TLS live test environment is not configured")
	}
	return value
}

func postgresTLSLiveConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		Host: requiredPostgresTLSEnvironment(
			t,
			"ROLLINGTHUNDER_POSTGRES_TEST_HOST",
		),
		Port: requiredPostgresTLSEnvironment(
			t,
			"ROLLINGTHUNDER_POSTGRES_TEST_PORT",
		),
		User: requiredPostgresTLSEnvironment(
			t,
			"ROLLINGTHUNDER_POSTGRES_TEST_USER",
		),
		Password: os.Getenv("ROLLINGTHUNDER_POSTGRES_TEST_PASSWORD"),
		Db: requiredPostgresTLSEnvironment(
			t,
			"ROLLINGTHUNDER_POSTGRES_TEST_DATABASE",
		),
	}
}

func connectPostgresTLS(
	config Config,
) (*Postgres, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		postgresTLSConnectTimeout,
	)
	defer cancel()
	driver := NewPostgres(context.Background(), config)
	if err := driver.Connect(ctx); err != nil {
		return nil, err
	}
	return driver, nil
}

func readPostgresTLSState(
	t *testing.T,
	driver *Postgres,
) postgresTLSState {
	t.Helper()
	ctx, cancel := context.WithTimeout(
		context.Background(),
		postgresTLSConnectTimeout,
	)
	defer cancel()
	var state postgresTLSState
	if err := driver.conn.GetContext(
		ctx,
		&state,
		`SELECT
			ssl,
			COALESCE(version, '') AS version,
			COALESCE(cipher, '') AS cipher,
			COALESCE(client_dn, '') AS client_dn
		 FROM pg_stat_ssl
		 WHERE pid = pg_backend_pid()`,
	); err != nil {
		t.Fatalf("read PostgreSQL TLS session state: %v", err)
	}
	if !state.Encrypted ||
		strings.TrimSpace(state.Version) == "" ||
		strings.TrimSpace(state.Cipher) == "" {
		t.Fatalf("PostgreSQL session is not encrypted: %+v", state)
	}
	return state
}

func assertPostgresTLSConnection(
	t *testing.T,
	config Config,
) postgresTLSState {
	t.Helper()
	driver, err := connectPostgresTLS(config)
	if err != nil {
		t.Fatalf("connect to PostgreSQL over TLS: %v", err)
	}
	defer func() {
		if err := driver.Close(); err != nil {
			t.Errorf("close PostgreSQL TLS connection: %v", err)
		}
	}()
	return readPostgresTLSState(t, driver)
}

func assertPostgresTLSConnectionFails(
	t *testing.T,
	config Config,
) {
	t.Helper()
	driver, err := connectPostgresTLS(config)
	if err != nil {
		return
	}
	_ = driver.Close()
	t.Fatal("PostgreSQL accepted a TLS connection that must be rejected")
}

func TestPostgresTLSLiveConformance(t *testing.T) {
	base := postgresTLSLiveConfig(t)
	rootCert := requiredPostgresTLSEnvironment(
		t,
		"ROLLINGTHUNDER_POSTGRES_TEST_TLS_ROOT_CERT",
	)
	wrongRootCert := requiredPostgresTLSEnvironment(
		t,
		"ROLLINGTHUNDER_POSTGRES_TEST_TLS_WRONG_ROOT_CERT",
	)
	clientCert := requiredPostgresTLSEnvironment(
		t,
		"ROLLINGTHUNDER_POSTGRES_TEST_TLS_CLIENT_CERT",
	)
	clientKey := requiredPostgresTLSEnvironment(
		t,
		"ROLLINGTHUNDER_POSTGRES_TEST_TLS_CLIENT_KEY",
	)
	clientRole := requiredPostgresTLSEnvironment(
		t,
		"ROLLINGTHUNDER_POSTGRES_TEST_TLS_CLIENT_ROLE",
	)
	serverName := requiredPostgresTLSEnvironment(
		t,
		"ROLLINGTHUNDER_POSTGRES_TEST_TLS_SERVER_NAME",
	)

	t.Run("plaintext_rejected", func(t *testing.T) {
		config := base
		config.SSLMode = "disable"
		assertPostgresTLSConnectionFails(t, config)
	})
	t.Run("require_encrypts_session", func(t *testing.T) {
		config := base
		config.SSLMode = "require"
		assertPostgresTLSConnection(t, config)
	})
	t.Run("verify_ca", func(t *testing.T) {
		config := base
		config.SSLMode = "verify-ca"
		config.SSLRootCert = rootCert
		assertPostgresTLSConnection(t, config)
	})
	t.Run("verify_full", func(t *testing.T) {
		config := base
		config.SSLMode = "verify-full"
		config.SSLRootCert = rootCert
		config.TLSServerName = serverName
		assertPostgresTLSConnection(t, config)
	})
	t.Run("wrong_ca_rejected", func(t *testing.T) {
		config := base
		config.SSLMode = "verify-ca"
		config.SSLRootCert = wrongRootCert
		assertPostgresTLSConnectionFails(t, config)
	})
	t.Run("wrong_server_name_rejected", func(t *testing.T) {
		config := base
		config.SSLMode = "verify-full"
		config.SSLRootCert = rootCert
		config.TLSServerName = "wrong-host.invalid"
		assertPostgresTLSConnectionFails(t, config)
	})
	t.Run("client_certificate_required", func(t *testing.T) {
		if !postgresTLSRolePattern.MatchString(clientRole) {
			t.Fatalf("unsafe PostgreSQL TLS client role %q", clientRole)
		}
		adminConfig := base
		adminConfig.SSLMode = "verify-full"
		adminConfig.SSLRootCert = rootCert
		adminConfig.TLSServerName = serverName
		admin, err := connectPostgresTLS(adminConfig)
		if err != nil {
			t.Fatalf("connect PostgreSQL TLS test administrator: %v", err)
		}
		defer func() {
			if err := admin.Close(); err != nil {
				t.Errorf("close PostgreSQL TLS test administrator: %v", err)
			}
		}()

		quotedRole := quotePostgresIdentifier(clientRole)
		dropRole := func() {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				postgresTLSConnectTimeout,
			)
			defer cancel()
			_, _ = admin.conn.ExecContext(
				ctx,
				"DROP ROLE IF EXISTS "+quotedRole,
			)
		}
		dropRole()
		defer dropRole()

		ctx, cancel := context.WithTimeout(
			context.Background(),
			postgresTLSConnectTimeout,
		)
		defer cancel()
		if _, err := admin.conn.ExecContext(
			ctx,
			"CREATE ROLE "+quotedRole+" LOGIN",
		); err != nil {
			t.Fatalf("create PostgreSQL certificate role: %v", err)
		}

		clientConfig := base
		clientConfig.User = clientRole
		clientConfig.Password = ""
		clientConfig.SSLMode = "verify-full"
		clientConfig.SSLRootCert = rootCert
		clientConfig.TLSServerName = serverName
		assertPostgresTLSConnectionFails(t, clientConfig)

		clientConfig.SSLCert = clientCert
		clientConfig.SSLKey = clientKey
		state := assertPostgresTLSConnection(t, clientConfig)
		if !strings.Contains(
			strings.ToLower(state.ClientDN),
			strings.ToLower("CN="+clientRole),
		) {
			t.Fatalf(
				"PostgreSQL client certificate DN = %q, want CN=%s",
				state.ClientDN,
				clientRole,
			)
		}
	})
}
