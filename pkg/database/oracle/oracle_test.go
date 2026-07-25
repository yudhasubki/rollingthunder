package oracle

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/database/drivertest"
	"rollingthunder/pkg/database/sqladapter"
)

func runOracleDataPumpLiveConformance(
	t *testing.T,
	driver *Oracle,
) {
	t.Helper()
	if os.Getenv("ROLLINGTHUNDER_ORACLE_TEST_DATAPUMP") != "1" {
		return
	}
	const (
		schema = "RT_DP_CONFORMANCE"
		table  = "BACKUP_PROBE"
	)
	ctx := context.Background()
	dropSchema := func() {
		_, _ = driver.conn.ExecContext(
			context.Background(),
			`DROP USER `+schema+` CASCADE`,
		)
	}
	dropSchema()
	t.Cleanup(dropSchema)
	if _, err := driver.conn.ExecContext(
		ctx,
		`CREATE USER `+schema+`
		 IDENTIFIED BY "RollingThunder_DP_2026"`,
	); err != nil {
		t.Fatalf("create Data Pump conformance schema: %v", err)
	}
	if _, err := driver.conn.ExecContext(
		ctx,
		`GRANT CREATE SESSION, CREATE TABLE, UNLIMITED TABLESPACE TO `+schema,
	); err != nil {
		t.Fatalf("grant Data Pump conformance privileges: %v", err)
	}
	if _, err := driver.conn.ExecContext(
		ctx,
		`CREATE TABLE `+schema+`.`+table+` (
			ID NUMBER(10) PRIMARY KEY,
			NAME VARCHAR2(100) NOT NULL
		)`,
	); err != nil {
		t.Fatalf("create Data Pump conformance table: %v", err)
	}
	if _, err := driver.conn.ExecContext(
		ctx,
		`INSERT INTO `+schema+`.`+table+` (ID, NAME)
		 VALUES (1, 'rolling thunder')`,
	); err != nil {
		t.Fatalf("seed Data Pump conformance table: %v", err)
	}

	directories, err := driver.GetBackupDirectories(ctx)
	if err != nil {
		t.Fatalf("GetBackupDirectories() error = %v", err)
	}
	directory := ""
	for _, candidate := range directories {
		if candidate.Name == "DATA_PUMP_DIR" {
			directory = candidate.Name
			break
		}
	}
	if directory == "" {
		t.Fatalf(
			"GetBackupDirectories() = %+v, missing DATA_PUMP_DIR",
			directories,
		)
	}
	var dump bytes.Buffer
	if err := driver.BackupDatabaseToWriter(
		ctx,
		&dump,
		database.BackupRequest{
			ConnectionID: "live-conformance",
			Schema:       schema,
			Directory:    directory,
		},
	); err != nil {
		t.Fatalf("BackupDatabaseToWriter() error = %v", err)
	}
	if dump.Len() == 0 {
		t.Fatal("BackupDatabaseToWriter() returned an empty dump")
	}
	if _, err := driver.conn.ExecContext(
		ctx,
		`DROP TABLE `+schema+`.`+table+` PURGE`,
	); err != nil {
		t.Fatalf("drop Data Pump conformance table: %v", err)
	}
	if err := driver.RestoreDatabaseFromReader(
		ctx,
		bytes.NewReader(dump.Bytes()),
		database.RestorePreviewRequest{
			ConnectionID: "live-conformance",
			Schema:       schema,
			Directory:    directory,
		},
	); err != nil {
		t.Fatalf("RestoreDatabaseFromReader() error = %v", err)
	}
	var rows int
	if err := driver.conn.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM `+schema+`.`+table,
	).Scan(&rows); err != nil {
		t.Fatalf("query restored Data Pump table: %v", err)
	}
	if rows != 1 {
		t.Fatalf("restored Data Pump row count = %d, want 1", rows)
	}
}

func TestOracleCapabilityContract(t *testing.T) {
	driver := NewOracle(context.Background(), Config{})
	drivertest.RunCapabilityContract(t, driver, database.DriverOracle)
	if driver.Capabilities().Dialect.PlaceholderStyle != database.PlaceholderColon {
		t.Fatalf(
			"placeholder style = %q",
			driver.Capabilities().Dialect.PlaceholderStyle,
		)
	}
}

func TestOracleDialectAndTableQuery(t *testing.T) {
	driver := NewOracle(context.Background(), Config{})
	if got := driver.QuoteIdentifier(`odd"name`); got != `"odd""name"` {
		t.Fatalf("QuoteIdentifier() = %q", got)
	}
	if got := driver.Placeholder(3); got != ":3" {
		t.Fatalf("Placeholder() = %q", got)
	}
	if got, err := driver.PaginationClause(25, 50); err != nil ||
		got != "OFFSET 50 ROWS FETCH NEXT 25 ROWS ONLY" {
		t.Fatalf("PaginationClause() = %q, %v", got, err)
	}
	query, args, err := sqladapter.BuildTableSelect(
		database.Table{
			Schema: "APP",
			Name:   "events",
			Limit:  25,
			Filters: []database.Filter{{
				Column: "name", Operator: database.FilterContains, Value: "storm",
			}},
		},
		database.Structures{
			{Name: "id", IsPrimary: true},
			{Name: "name"},
		},
		"*",
		driver.adapterDialect(),
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT * FROM "APP"."events" WHERE TO_CHAR("name") LIKE :1 ORDER BY "id" ASC OFFSET 0 ROWS FETCH NEXT 25 ROWS ONLY`
	if query != want {
		t.Fatalf("query = %q, want %q", query, want)
	}
	if len(args) != 1 || args[0] != "%storm%" {
		t.Fatalf("args = %#v", args)
	}
}

func TestOracleConnectionValidationAndTLSModes(t *testing.T) {
	config, port, err := normalizeConfig(Config{Db: "FREEPDB1"})
	if err != nil {
		t.Fatal(err)
	}
	if config.Host != database.DefaultHost ||
		config.Port != database.DefaultOraclePort ||
		port != 1521 {
		t.Fatalf("normalized config = %+v, port = %d", config, port)
	}
	if _, _, err := normalizeConfig(Config{}); err == nil {
		t.Fatal("missing service name was accepted")
	}
	if _, _, err := normalizeConfig(Config{
		Db: "FREEPDB1", Port: "70000",
	}); err == nil {
		t.Fatal("invalid port was accepted")
	}
	required, err := oracleTLSConfig(Config{
		Host: "oracle.example", SSLMode: "require",
	})
	if err != nil {
		t.Fatal(err)
	}
	if required == nil || !required.InsecureSkipVerify {
		t.Fatalf("require TLS config = %#v", required)
	}
	verified, err := oracleTLSConfig(Config{
		Host: "127.0.0.1", TLSServerName: "oracle.internal", SSLMode: "verify-full",
	})
	if err != nil {
		t.Fatal(err)
	}
	if verified.ServerName != "oracle.internal" ||
		!verified.InsecureSkipVerify ||
		verified.VerifyConnection == nil {
		t.Fatalf("verify-full TLS config = %#v", verified)
	}
}

func TestOracleIndexColumnPrefersFunctionExpression(t *testing.T) {
	column := sql.NullString{String: "SYS_NC00003$", Valid: true}
	expression := sql.NullString{String: `LOWER("EMAIL")`, Valid: true}
	if got := oracleIndexColumn(column, expression); got != `LOWER("EMAIL")` {
		t.Fatalf("oracleIndexColumn() = %q", got)
	}
	if got := oracleIndexColumn(column, sql.NullString{}); got != column.String {
		t.Fatalf("oracleIndexColumn() fallback = %q", got)
	}
	if got := oracleIndexColumn(sql.NullString{}, sql.NullString{}); got != "(expression)" {
		t.Fatalf("oracleIndexColumn() empty fallback = %q", got)
	}
}

func TestOracleColumnMetadataReadsVirtualFlagFromTabCols(t *testing.T) {
	normalized := strings.ToLower(oracleCollectionStructuresQuery)
	if !strings.Contains(normalized, "from all_tab_columns column_object") {
		t.Fatal("column metadata no longer uses the visible-column catalog")
	}
	if !strings.Contains(normalized, "from all_tab_cols column_flags") ||
		!strings.Contains(normalized, "column_flags.virtual_column") {
		t.Fatal("virtual column metadata is not read from all_tab_cols")
	}
	if strings.Contains(normalized, "\n\t\tvirtual_column\n") {
		t.Fatal("column metadata still selects virtual_column from all_tab_columns")
	}
}

func TestOracleDatabaseVersionHasPermissionSafeFallback(t *testing.T) {
	if len(oracleDatabaseVersionQueries) < 3 {
		t.Fatalf("version queries = %v", oracleDatabaseVersionQueries)
	}
	fallback := strings.ToUpper(
		oracleDatabaseVersionQueries[len(oracleDatabaseVersionQueries)-1],
	)
	if !strings.Contains(fallback, "DBMS_DB_VERSION.VERSION") ||
		!strings.Contains(fallback, "FROM DUAL") {
		t.Fatalf("permission-safe version fallback = %q", fallback)
	}
	for _, query := range oracleDatabaseVersionQueries[:2] {
		normalized := strings.ToUpper(query)
		if !strings.Contains(normalized, "MAX(") {
			t.Fatalf("catalog version query can return no rows: %q", query)
		}
	}
}

func TestOracleCatalogViewDDL(t *testing.T) {
	view := database.ObjectReference{
		Kind:   database.ObjectKindView,
		Schema: `APP`,
		Name:   `ACTIVE_ORDERS`,
	}
	got, err := oracleCatalogViewDDL(
		view,
		`SELECT "ID" FROM "APP"."ORDERS";`,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "CREATE OR REPLACE VIEW \"APP\".\"ACTIVE_ORDERS\" AS\n" +
		`SELECT "ID" FROM "APP"."ORDERS";`
	if got != want {
		t.Fatalf("view DDL = %q, want %q", got, want)
	}

	materialized := view
	materialized.Kind = database.ObjectKindMaterializedView
	materialized.Name = "ORDER_TOTALS"
	got, err = oracleCatalogViewDDL(materialized, "SELECT COUNT(*) FROM ORDERS")
	if err != nil {
		t.Fatal(err)
	}
	want = "CREATE MATERIALIZED VIEW \"APP\".\"ORDER_TOTALS\" AS\n" +
		"SELECT COUNT(*) FROM ORDERS;"
	if got != want {
		t.Fatalf("materialized view DDL = %q, want %q", got, want)
	}

	if _, err := oracleCatalogViewDDL(view, " ; "); err == nil {
		t.Fatal("empty catalog view definition was accepted")
	}
}

func TestOracleLiveConformance(t *testing.T) {
	host := os.Getenv("ROLLINGTHUNDER_ORACLE_TEST_HOST")
	if host == "" {
		t.Skip("set ROLLINGTHUNDER_ORACLE_TEST_HOST to run live Oracle conformance")
	}
	config := Config{
		Host:     host,
		Port:     os.Getenv("ROLLINGTHUNDER_ORACLE_TEST_PORT"),
		User:     os.Getenv("ROLLINGTHUNDER_ORACLE_TEST_USER"),
		Password: os.Getenv("ROLLINGTHUNDER_ORACLE_TEST_PASSWORD"),
		Db:       os.Getenv("ROLLINGTHUNDER_ORACLE_TEST_SERVICE"),
		SSLMode:  os.Getenv("ROLLINGTHUNDER_ORACLE_TEST_SSL_MODE"),
	}
	driver := NewOracle(context.Background(), config)
	if err := driver.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() {
		if err := driver.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	schema := driver.currentSchema
	if strings.TrimSpace(schema) == "" {
		t.Fatal("Oracle current schema is empty")
	}
	drivertest.RunLiveContract(t, drivertest.LiveConfig{
		Driver:             driver,
		Schema:             schema,
		IntegerType:        "NUMBER(10)",
		TextType:           "VARCHAR2(255)",
		ExercisePrivileged: os.Getenv("ROLLINGTHUNDER_ORACLE_TEST_PRIVILEGED") == "1",
	})
	runOracleDataPumpLiveConformance(t, driver)
}
