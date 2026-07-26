package oracle

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/database/drivertest"

	"github.com/google/uuid"
)

const (
	oracleApplicationQuota = "64M"
	oracleLongRunningQuery = `SELECT COUNT(*)
		FROM all_objects first_object
		CROSS JOIN all_objects second_object
		WHERE DBMS_RANDOM.VALUE(0, 1) >= 0`
)

type oracleCoreOnlyDriver struct {
	*Oracle
}

func (driver *oracleCoreOnlyDriver) Capabilities() database.Capabilities {
	capabilities := driver.Oracle.Capabilities()
	// Administration is intentionally tested through the privileged contract.
	// An ordinary application account should need only object-owner privileges
	// for the core data and schema workflows.
	capabilities.ActivityMonitor = false
	capabilities.ManageSecurity = false
	return capabilities
}

func oracleLiveConfig(t *testing.T) Config {
	t.Helper()
	host := strings.TrimSpace(os.Getenv("ROLLINGTHUNDER_ORACLE_TEST_HOST"))
	if host == "" {
		t.Skip("set ROLLINGTHUNDER_ORACLE_TEST_HOST to run live Oracle conformance")
	}
	value := func(name, fallback string) string {
		if configured := strings.TrimSpace(os.Getenv(name)); configured != "" {
			return configured
		}
		return fallback
	}
	return Config{
		Host:     host,
		Port:     value("ROLLINGTHUNDER_ORACLE_TEST_PORT", database.DefaultOraclePort),
		User:     value("ROLLINGTHUNDER_ORACLE_TEST_USER", "system"),
		Password: os.Getenv("ROLLINGTHUNDER_ORACLE_TEST_PASSWORD"),
		Db:       value("ROLLINGTHUNDER_ORACLE_TEST_SERVICE", "FREEPDB1"),
		SSLMode:  value("ROLLINGTHUNDER_ORACLE_TEST_SSL_MODE", "disable"),
	}
}

func connectOracleLive(t *testing.T, config Config) *Oracle {
	t.Helper()
	driver := NewOracle(context.Background(), config)
	connectCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := driver.Connect(connectCtx); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() {
		if err := driver.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return driver
}

func oracleTestName(prefix string) string {
	suffix := strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))
	return prefix + "_" + suffix[:12]
}

func createOracleApplicationDriver(
	t *testing.T,
	admin *Oracle,
) (*Oracle, string) {
	t.Helper()
	ctx := context.Background()
	username := oracleTestName("RT_APP")
	password := "Rt" + strings.TrimPrefix(username, "RT_APP_") + "_9x"
	quotedUser := quoteIdentifier(username)
	dropUser := func() {
		_, _ = admin.conn.ExecContext(
			context.Background(),
			"DROP USER "+quotedUser+" CASCADE",
		)
	}
	dropUser()
	t.Cleanup(dropUser)
	if _, err := admin.conn.ExecContext(
		ctx,
		"CREATE USER "+quotedUser+
			" IDENTIFIED BY "+quoteIdentifier(password),
	); err != nil {
		t.Fatalf("create least-privilege Oracle application user: %v", err)
	}
	var tablespace string
	if err := admin.conn.QueryRowContext(
		ctx,
		`SELECT default_tablespace
		 FROM dba_users
		 WHERE username = :1`,
		username,
	).Scan(&tablespace); err != nil {
		t.Fatalf("read Oracle application tablespace: %v", err)
	}
	if strings.TrimSpace(tablespace) == "" {
		t.Fatal("Oracle application tablespace is empty")
	}
	if _, err := admin.conn.ExecContext(
		ctx,
		"ALTER USER "+quotedUser+" QUOTA "+oracleApplicationQuota+
			" ON "+quoteIdentifier(tablespace),
	); err != nil {
		t.Fatalf("grant Oracle application tablespace quota: %v", err)
	}
	if _, err := admin.conn.ExecContext(
		ctx,
		"GRANT CREATE SESSION, CREATE TABLE, CREATE VIEW, CREATE SEQUENCE, "+
			"CREATE PROCEDURE, CREATE TRIGGER TO "+quotedUser,
	); err != nil {
		t.Fatalf("grant least-privilege Oracle application capabilities: %v", err)
	}
	config := admin.cfg
	config.User = username
	config.Password = password
	config.ConnectionMode = "direct"
	config.TNSConfigPath = ""
	config.TNSAlias = ""
	config.WalletPath = ""
	config.WalletPassword = ""
	config.SSLRootCert = ""
	config.SSLCert = ""
	config.SSLKey = ""
	config.TLSServerName = ""
	driver := connectOracleLive(t, config)
	if !strings.EqualFold(driver.currentSchema, username) {
		t.Fatalf(
			"least-privilege current schema = %q, want %q",
			driver.currentSchema,
			username,
		)
	}
	return driver, username
}

func liveRowValue(row map[string]interface{}, name string) interface{} {
	for key, value := range row {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return nil
}

func liveText(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func runOracleLeastPrivilegeConformance(
	t *testing.T,
	admin *Oracle,
) {
	t.Helper()
	driver, schema := createOracleApplicationDriver(t, admin)
	drivertest.RunLiveContract(t, drivertest.LiveConfig{
		Driver:      &oracleCoreOnlyDriver{Oracle: driver},
		Schema:      schema,
		IntegerType: "NUMBER(10)",
		TextType:    "VARCHAR2(255)",
	})
}

func runOracleEdgeTypeConformance(
	t *testing.T,
	admin *Oracle,
) {
	t.Helper()
	driver, schema := createOracleApplicationDriver(t, admin)
	table := database.Table{Schema: schema, Name: "RT Edge Types"}
	_ = driver.DropTable(table)
	t.Cleanup(func() {
		_ = driver.DropTable(table)
	})
	if err := driver.CreateTable(table, []database.ColumnDefinition{
		{Name: "ID", Type: "NUMBER(10)", PrimaryKey: true, Nullable: false},
		{Name: "UNICODE_TEXT", Type: "NVARCHAR2(100)", Nullable: false},
		{Name: "NULL_TEXT", Type: "VARCHAR2(100)", Nullable: true},
		{Name: "CLOB_VALUE", Type: "CLOB", Nullable: false},
		{Name: "BLOB_VALUE", Type: "BLOB", Nullable: false},
		{Name: "RAW_VALUE", Type: "RAW(16)", Nullable: false},
		{Name: "TIMESTAMP_VALUE", Type: "TIMESTAMP(6)", Nullable: false},
		{
			Name:     "TIMESTAMP_TZ_VALUE",
			Type:     "TIMESTAMP(6) WITH TIME ZONE",
			Nullable: false,
		},
		{Name: "Quoted Value", Type: "VARCHAR2(100)", Nullable: false},
	}); err != nil {
		t.Fatalf("create Oracle edge-type table: %v", err)
	}
	unicodeValue := "Rolling Thunder 雷鳴 🌩"
	clobValue := strings.Repeat("rolling thunder 雷鳴 ", 400)
	blobValue := []byte{0x00, 0x01, 0x7f, 0x80, 0xfe, 0xff}
	rawValue := []byte{
		0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77,
		0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
	}
	timestampValue := time.Date(
		2026,
		time.July,
		26,
		12,
		34,
		56,
		123456000,
		time.UTC,
	)
	timestampTZValue := time.Date(
		2026,
		time.July,
		26,
		21,
		34,
		56,
		654321000,
		time.FixedZone("JST", 9*60*60),
	)
	if err := driver.InsertRow(table, map[string]interface{}{
		"ID":                 1,
		"UNICODE_TEXT":       unicodeValue,
		"NULL_TEXT":          nil,
		"CLOB_VALUE":         clobValue,
		"BLOB_VALUE":         blobValue,
		"RAW_VALUE":          rawValue,
		"TIMESTAMP_VALUE":    timestampValue,
		"TIMESTAMP_TZ_VALUE": timestampTZValue,
		"Quoted Value":       "mixed-case identifier",
	}); err != nil {
		t.Fatalf("insert Oracle edge-type row: %v", err)
	}
	structures, rows, err := driver.GetCollectionData(database.Table{
		Schema: schema,
		Name:   table.Name,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("read Oracle edge-type table: %v", err)
	}
	if len(structures) != 9 || len(rows) != 1 {
		t.Fatalf(
			"Oracle edge-type table returned %d structures and %d rows",
			len(structures),
			len(rows),
		)
	}
	result, err := driver.ExecuteQuery(
		context.Background(),
		`SELECT
			"UNICODE_TEXT" AS unicode_text,
			CASE WHEN "NULL_TEXT" IS NULL THEN 1 ELSE 0 END AS null_ok,
			DBMS_LOB.GETLENGTH("CLOB_VALUE") AS clob_length,
			RAWTOHEX(DBMS_LOB.SUBSTR("BLOB_VALUE", 2000, 1)) AS blob_hex,
			RAWTOHEX("RAW_VALUE") AS raw_hex,
			TO_CHAR(
				"TIMESTAMP_VALUE",
				'YYYY-MM-DD"T"HH24:MI:SS.FF6'
			) AS timestamp_text,
			TO_CHAR(
				"TIMESTAMP_TZ_VALUE",
				'YYYY-MM-DD"T"HH24:MI:SS.FF6TZH:TZM'
			) AS timestamp_tz_text,
			"Quoted Value" AS quoted_value
		 FROM `+quoteQualified(schema, table.Name)+`
		 WHERE "ID" = 1`,
		database.QueryOptions{MaxRows: 10},
	)
	if err != nil {
		t.Fatalf("query canonical Oracle edge-type values: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("canonical Oracle edge-type rows = %d, want 1", len(result.Rows))
	}
	row := result.Rows[0]
	if got := liveText(liveRowValue(row, "unicode_text")); got != unicodeValue {
		t.Fatalf("Oracle Unicode round trip = %q, want %q", got, unicodeValue)
	}
	if got := liveText(liveRowValue(row, "null_ok")); got != "1" {
		t.Fatalf("Oracle NULL round trip marker = %q, want 1", got)
	}
	clobLength, err := strconv.Atoi(
		liveText(liveRowValue(row, "clob_length")),
	)
	if err != nil || clobLength != utf8.RuneCountInString(clobValue) {
		t.Fatalf(
			"Oracle CLOB length = %d, %v, want %d",
			clobLength,
			err,
			utf8.RuneCountInString(clobValue),
		)
	}
	if got := strings.ToUpper(liveText(liveRowValue(row, "blob_hex"))); got != "00017F80FEFF" {
		t.Fatalf("Oracle BLOB round trip = %q", got)
	}
	if got := strings.ToUpper(liveText(liveRowValue(row, "raw_hex"))); got != "00112233445566778899AABBCCDDEEFF" {
		t.Fatalf("Oracle RAW round trip = %q", got)
	}
	if got := liveText(liveRowValue(row, "timestamp_text")); got != "2026-07-26T12:34:56.123456" {
		t.Fatalf("Oracle TIMESTAMP round trip = %q", got)
	}
	if got := liveText(liveRowValue(row, "timestamp_tz_text")); got != "2026-07-26T21:34:56.654321+09:00" {
		t.Fatalf("Oracle TIMESTAMP WITH TIME ZONE round trip = %q", got)
	}
	if got := liveText(liveRowValue(row, "quoted_value")); got != "mixed-case identifier" {
		t.Fatalf("Oracle quoted identifier round trip = %q", got)
	}
}

func assertOracleContextInterruption(
	t *testing.T,
	driver *Oracle,
	cancelExplicitly bool,
) {
	t.Helper()
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	if cancelExplicitly {
		ctx, cancel = context.WithCancel(context.Background())
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), 250*time.Millisecond)
	}
	defer cancel()
	result := make(chan error, 1)
	started := time.Now()
	go func() {
		_, err := driver.ExecuteQuery(
			ctx,
			oracleLongRunningQuery,
			database.QueryOptions{MaxRows: 1},
		)
		result <- err
	}()
	if cancelExplicitly {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("Oracle long-running statement ignored context interruption")
		}
		if ctx.Err() == nil {
			t.Fatalf(
				"Oracle statement failed before context interruption: %v",
				err,
			)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("Oracle context interruption did not stop the statement promptly")
	}
	if elapsed := time.Since(started); elapsed >= 4*time.Second {
		t.Fatalf("Oracle context interruption took %s", elapsed)
	}
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()
	if err := driver.Ping(pingCtx); err != nil {
		t.Fatalf("Oracle connection did not recover after interruption: %v", err)
	}
}

func runOracleConnectionResilienceConformance(
	t *testing.T,
	config Config,
) {
	t.Helper()
	driver := connectOracleLive(t, config)
	previousPool := driver.conn
	reconnectCtx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()
	if err := driver.Connect(reconnectCtx); err != nil {
		t.Fatalf("Oracle reconnect failed: %v", err)
	}
	if err := previousPool.Ping(); err == nil {
		t.Fatal("Oracle reconnect left the replaced connection pool open")
	}
	if err := driver.Ping(context.Background()); err != nil {
		t.Fatalf("Oracle ping after reconnect failed: %v", err)
	}
	t.Run("deadline", func(t *testing.T) {
		assertOracleContextInterruption(t, driver, false)
	})
	t.Run("explicit_cancel", func(t *testing.T) {
		assertOracleContextInterruption(t, driver, true)
	})
}

func runOracleTNSConformance(
	t *testing.T,
	config Config,
) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tnsnames.ora")
	descriptor := fmt.Sprintf(
		`RT_STABLE =
		  (DESCRIPTION =
		    (ADDRESS = (PROTOCOL = TCP)(HOST = %s)(PORT = %s))
		    (CONNECT_DATA = (SERVICE_NAME = %s))
		  )
		`,
		config.Host,
		config.Port,
		config.Db,
	)
	if err := os.WriteFile(path, []byte(descriptor), 0o600); err != nil {
		t.Fatalf("write live Oracle TNS fixture: %v", err)
	}
	tnsConfig := config
	tnsConfig.ConnectionMode = "tns"
	tnsConfig.TNSConfigPath = path
	tnsConfig.TNSAlias = "RT_STABLE"
	driver := connectOracleLive(t, tnsConfig)
	result, err := driver.ExecuteQuery(
		context.Background(),
		"SELECT SYS_CONTEXT('USERENV', 'SERVICE_NAME') AS service_name FROM dual",
		database.QueryOptions{MaxRows: 1},
	)
	if err != nil || len(result.Rows) != 1 {
		t.Fatalf("Oracle live TNS query = %+v, %v", result, err)
	}
	if strings.TrimSpace(liveText(
		liveRowValue(result.Rows[0], "service_name"),
	)) == "" {
		t.Fatalf("Oracle live TNS service metadata = %+v", result.Rows[0])
	}
}

func requiredSecureOracleEnvironment(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		if os.Getenv("ROLLINGTHUNDER_ORACLE_TEST_REQUIRE_SECURE") == "1" {
			t.Fatalf("required secure Oracle test variable %s is empty", name)
		}
		t.Skip("secure Oracle listener is not configured")
	}
	return value
}

func requiredSecureOraclePassword(t *testing.T) string {
	t.Helper()
	path := requiredSecureOracleEnvironment(
		t,
		"ROLLINGTHUNDER_ORACLE_TEST_WALLET_PASSWORD_FILE",
	)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("inspect secure Oracle Wallet password file: %v", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf(
			"secure Oracle Wallet password file mode = %s, want a private regular file",
			info.Mode(),
		)
	}
	value, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read secure Oracle Wallet password file: %v", err)
	}
	password := strings.TrimSpace(string(value))
	if password == "" {
		t.Fatal("secure Oracle Wallet password file is empty")
	}
	return password
}

func assertSecureOracleConnection(
	t *testing.T,
	config Config,
) {
	t.Helper()
	driver := connectOracleLive(t, config)
	result, err := driver.ExecuteQuery(
		context.Background(),
		"SELECT 1 AS secure_probe FROM dual",
		database.QueryOptions{MaxRows: 1},
	)
	if err != nil || len(result.Rows) != 1 ||
		liveText(liveRowValue(result.Rows[0], "secure_probe")) != "1" {
		t.Fatalf("secure Oracle probe = %+v, %v", result, err)
	}
}

func runOracleSecureConnectivityConformance(
	t *testing.T,
	base Config,
) {
	t.Helper()
	tlsPort := requiredSecureOracleEnvironment(
		t,
		"ROLLINGTHUNDER_ORACLE_TEST_TLS_PORT",
	)
	serverName := requiredSecureOracleEnvironment(
		t,
		"ROLLINGTHUNDER_ORACLE_TEST_TLS_SERVER_NAME",
	)
	rootCert := requiredSecureOracleEnvironment(
		t,
		"ROLLINGTHUNDER_ORACLE_TEST_TLS_ROOT_CERT",
	)
	walletPath := requiredSecureOracleEnvironment(
		t,
		"ROLLINGTHUNDER_ORACLE_TEST_WALLET_PATH",
	)
	walletPassword := requiredSecureOraclePassword(t)

	t.Run("tls_require", func(t *testing.T) {
		config := base
		config.Port = tlsPort
		config.SSLMode = "require"
		assertSecureOracleConnection(t, config)
	})
	t.Run("tls_verify_ca", func(t *testing.T) {
		config := base
		config.Port = tlsPort
		config.SSLMode = "verify-ca"
		config.SSLRootCert = rootCert
		assertSecureOracleConnection(t, config)
	})
	t.Run("tls_verify_full_tunnel_name", func(t *testing.T) {
		config := base
		config.Port = tlsPort
		config.SSLMode = "verify-full"
		config.SSLRootCert = rootCert
		config.TLSServerName = serverName
		assertSecureOracleConnection(t, config)
	})
	t.Run("tls_rejects_wrong_server_name", func(t *testing.T) {
		config := base
		config.Port = tlsPort
		config.SSLMode = "verify-full"
		config.SSLRootCert = rootCert
		config.TLSServerName = "wrong-host.invalid"
		driver := NewOracle(context.Background(), config)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := driver.Connect(ctx); err == nil {
			_ = driver.Close()
			t.Fatal("Oracle verify-full accepted the wrong TLS server name")
		}
	})
	t.Run("wallet_auto_login", func(t *testing.T) {
		config := base
		config.Host = serverName
		config.Port = tlsPort
		config.SSLMode = "verify-full"
		config.WalletPath = walletPath
		assertSecureOracleConnection(t, config)
	})
	t.Run("wallet_password", func(t *testing.T) {
		config := base
		config.Host = serverName
		config.Port = tlsPort
		config.SSLMode = "verify-full"
		config.WalletPath = walletPath
		config.WalletPassword = walletPassword
		assertSecureOracleConnection(t, config)
	})
	t.Run("tns_tcps_wallet", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "tnsnames.ora")
		descriptor := fmt.Sprintf(
			`RT_STABLE_TCPS =
			  (DESCRIPTION =
			    (ADDRESS = (PROTOCOL = TCPS)(HOST = %s)(PORT = %s))
			    (CONNECT_DATA = (SERVICE_NAME = %s))
			  )
			`,
			serverName,
			tlsPort,
			base.Db,
		)
		if err := os.WriteFile(path, []byte(descriptor), 0o600); err != nil {
			t.Fatalf("write live Oracle TCPS TNS fixture: %v", err)
		}
		config := base
		config.ConnectionMode = "tns"
		config.TNSConfigPath = path
		config.TNSAlias = "RT_STABLE_TCPS"
		config.SSLMode = "verify-full"
		config.WalletPath = walletPath
		config.Host = serverName
		assertSecureOracleConnection(t, config)
	})
}

func TestOracleConnectHonorsContextDeadline(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skip("sandbox does not permit a loopback listener")
		}
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- connection
		}
	}()
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	driver := NewOracle(context.Background(), Config{
		Host:     "127.0.0.1",
		Port:     port,
		User:     "rolling",
		Password: "not-a-secret",
		Db:       "FREEPDB1",
		SSLMode:  "disable",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()
	connectErr := driver.Connect(ctx)
	elapsed := time.Since(started)
	select {
	case connection := <-accepted:
		_ = connection.Close()
	default:
	}
	if connectErr == nil {
		_ = driver.Close()
		t.Fatal("Oracle connection to a non-responsive endpoint succeeded")
	}
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("Oracle connection context = %v, want deadline exceeded", ctx.Err())
	}
	if elapsed >= 2*time.Second {
		t.Fatalf("Oracle connection deadline took %s, want less than 2s", elapsed)
	}
}
