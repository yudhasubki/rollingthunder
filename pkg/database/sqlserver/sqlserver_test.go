package sqlserver

import (
	"context"
	"net/url"
	"os"
	"testing"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/database/drivertest"
	"rollingthunder/pkg/database/sqladapter"
)

func TestSQLServerCapabilityContract(t *testing.T) {
	driver := NewSQLServer(context.Background(), Config{})
	drivertest.RunCapabilityContract(t, driver, database.DriverSQLServer)
	if driver.Capabilities().Dialect.PlaceholderStyle != database.PlaceholderAt {
		t.Fatalf(
			"placeholder style = %q",
			driver.Capabilities().Dialect.PlaceholderStyle,
		)
	}
}

func TestSQLServerDialectAndTableQuery(t *testing.T) {
	driver := NewSQLServer(context.Background(), Config{})
	if got := driver.QuoteIdentifier(`odd]name`); got != `[odd]]name]` {
		t.Fatalf("QuoteIdentifier() = %q", got)
	}
	if got := driver.Placeholder(3); got != "@p3" {
		t.Fatalf("Placeholder() = %q", got)
	}
	query, args, err := sqladapter.BuildTableSelect(
		database.Table{
			Schema: "dbo",
			Name:   "events",
			Limit:  25,
			Filters: []database.Filter{{
				Column: "name", Operator: database.FilterContains, Value: "storm",
			}},
			Sorts: []database.Sort{{
				Column:    "created_at",
				Direction: database.SortDescending,
				Nulls:     database.NullsLast,
			}},
		},
		database.Structures{
			{Name: "id", IsPrimary: true},
			{Name: "name"},
			{Name: "created_at"},
		},
		"*",
		driver.adapterDialect(),
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT * FROM [dbo].[events] WHERE CONVERT(nvarchar(max), [name]) LIKE @p1" +
		" ORDER BY CASE WHEN [created_at] IS NULL THEN 1 ELSE 0 END ASC, [created_at] DESC" +
		" OFFSET 0 ROWS FETCH NEXT 25 ROWS ONLY"
	if query != want {
		t.Fatalf("query = %q, want %q", query, want)
	}
	if len(args) != 1 || args[0] != "%storm%" {
		t.Fatalf("args = %#v", args)
	}
	enable, disable := driver.adapterDialect().IdentityInsertStatements(
		database.Table{Schema: "odd]schema", Name: "event rows"},
	)
	if enable != "SET IDENTITY_INSERT [odd]]schema].[event rows] ON" ||
		disable != "SET IDENTITY_INSERT [odd]]schema].[event rows] OFF" {
		t.Fatalf("identity statements = %q / %q", enable, disable)
	}
}

func TestSQLServerConnectionURL(t *testing.T) {
	dsn, err := buildConnectionURL(Config{
		Host:          "127.0.0.1",
		Port:          "1433",
		User:          "sa@example",
		Password:      "s:e/c?ret",
		Db:            "rolling",
		SSLMode:       "verify-full",
		TLSServerName: "database.internal",
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Host != "127.0.0.1:1433" {
		t.Fatalf("host = %q", parsed.Host)
	}
	if parsed.Query().Get("database") != "rolling" ||
		parsed.Query().Get("encrypt") != "true" ||
		parsed.Query().Get("TrustServerCertificate") != "false" ||
		parsed.Query().Get("hostNameInCertificate") != "database.internal" {
		t.Fatalf("query = %#v", parsed.Query())
	}
	password, _ := parsed.User.Password()
	if parsed.User.Username() != "sa@example" || password != "s:e/c?ret" {
		t.Fatalf("credentials did not round-trip through URL encoding")
	}
	required, err := buildConnectionURL(Config{
		Db: "master", SSLMode: "require",
	})
	if err != nil {
		t.Fatal(err)
	}
	requiredURL, _ := url.Parse(required)
	if requiredURL.Query().Get("TrustServerCertificate") != "true" {
		t.Fatalf("require URL = %q", required)
	}
	if _, err := buildConnectionURL(Config{
		Db: "master", SSLMode: "disable", SSLCert: "client.pem",
	}); err == nil {
		t.Fatal("SQL Server accepted a TLS client certificate")
	}
}

func TestBuildKeyConstraintDefinitionsPreservesOrderAndShape(t *testing.T) {
	definitions := buildKeyConstraintDefinitions([]keyConstraintColumn{
		{
			name:      "PK_orders",
			kind:      "PK",
			algorithm: "CLUSTERED",
			column:    "tenant_id",
			position:  1,
		},
		{
			name:       "PK_orders",
			kind:       "PK",
			algorithm:  "CLUSTERED",
			column:     "order_id",
			descending: true,
			position:   2,
		},
		{
			name:      "UQ_orders_reference",
			kind:      "UQ",
			algorithm: "NONCLUSTERED",
			column:    "reference",
			position:  1,
		},
	})
	if len(definitions) != 2 {
		t.Fatalf("definitions = %#v", definitions)
	}
	if definitions[0] !=
		"CONSTRAINT [PK_orders] PRIMARY KEY CLUSTERED ([tenant_id] ASC, [order_id] DESC)" {
		t.Fatalf("primary definition = %q", definitions[0])
	}
	if definitions[1] !=
		"CONSTRAINT [UQ_orders_reference] UNIQUE NONCLUSTERED ([reference] ASC)" {
		t.Fatalf("unique definition = %q", definitions[1])
	}
}

func TestBuildForeignKeyDefinitionsPreservesCompositeRelationship(t *testing.T) {
	definitions := buildForeignKeyDefinitions([]foreignKeyMetadata{
		{
			name:          "FK_order_items_orders",
			column:        "tenant_id",
			foreignSchema: "sales",
			foreignTable:  "orders",
			foreignColumn: "tenant_id",
			position:      1,
			deleteAction:  "CASCADE",
			updateAction:  "SET_NULL",
			notReplicated: true,
		},
		{
			name:          "FK_order_items_orders",
			column:        "order_id",
			foreignSchema: "sales",
			foreignTable:  "orders",
			foreignColumn: "id",
			position:      2,
			deleteAction:  "CASCADE",
			updateAction:  "SET_NULL",
			notReplicated: true,
		},
	})
	if len(definitions) != 1 {
		t.Fatalf("definitions = %#v", definitions)
	}
	want := "CONSTRAINT [FK_order_items_orders] FOREIGN KEY " +
		"([tenant_id], [order_id]) REFERENCES [sales].[orders] " +
		"([tenant_id], [id]) ON DELETE CASCADE ON UPDATE SET NULL " +
		"NOT FOR REPLICATION"
	if definitions[0] != want {
		t.Fatalf("foreign key definition = %q, want %q", definitions[0], want)
	}
}

func TestSQLServerLiveConformance(t *testing.T) {
	host := os.Getenv("ROLLINGTHUNDER_SQLSERVER_TEST_HOST")
	if host == "" {
		t.Skip("set ROLLINGTHUNDER_SQLSERVER_TEST_HOST to run live SQL Server conformance")
	}
	config := Config{
		Host:     host,
		Port:     os.Getenv("ROLLINGTHUNDER_SQLSERVER_TEST_PORT"),
		User:     os.Getenv("ROLLINGTHUNDER_SQLSERVER_TEST_USER"),
		Password: os.Getenv("ROLLINGTHUNDER_SQLSERVER_TEST_PASSWORD"),
		Db:       os.Getenv("ROLLINGTHUNDER_SQLSERVER_TEST_DATABASE"),
		SSLMode:  os.Getenv("ROLLINGTHUNDER_SQLSERVER_TEST_SSL_MODE"),
	}
	driver := NewSQLServer(context.Background(), config)
	if err := driver.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = driver.Close() })
	drivertest.RunLiveContract(t, drivertest.LiveConfig{
		Driver:      driver,
		Schema:      driver.currentSchema,
		IntegerType: "INT",
		TextType:    "NVARCHAR(255)",
	})
}
