package sqlserver

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

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
		User: "sa", Db: "master", SSLMode: "require",
	})
	if err != nil {
		t.Fatal(err)
	}
	requiredURL, _ := url.Parse(required)
	if requiredURL.Query().Get("TrustServerCertificate") != "true" {
		t.Fatalf("require URL = %q", required)
	}
	strict, err := buildConnectionURL(Config{
		Host:          "127.0.0.1",
		User:          "sa",
		Db:            "master",
		SSLMode:       "strict",
		TLSServerName: "database.internal",
	})
	if err != nil {
		t.Fatal(err)
	}
	strictURL, _ := url.Parse(strict)
	if strictURL.Query().Get("encrypt") != "strict" ||
		strictURL.Query().Get("tlsmin") != "1.2" ||
		strictURL.Query().Get("TrustServerCertificate") != "false" ||
		strictURL.Query().Get("hostNameInCertificate") !=
			"database.internal" {
		t.Fatalf("strict URL = %q", strict)
	}
	if _, err := buildConnectionURL(Config{
		User: "sa", Db: "master", SSLMode: "disable", SSLCert: "client.pem",
	}); err == nil {
		t.Fatal("SQL Server accepted a TLS client certificate")
	}
}

func TestSQLServerAuthenticationConnectionURLs(t *testing.T) {
	tests := []struct {
		name       string
		config     Config
		driverName string
		fedAuth    string
		username   string
	}{
		{
			name: "integrated",
			config: Config{
				Db:       "rolling",
				SSLMode:  "require",
				AuthMode: database.SQLServerAuthIntegrated,
			},
			driverName: "sqlserver",
		},
		{
			name: "entra default",
			config: Config{
				Db:       "rolling",
				SSLMode:  "verify-full",
				AuthMode: database.SQLServerAuthEntraDefault,
			},
			driverName: "azuresql",
			fedAuth:    "ActiveDirectoryDefault",
		},
		{
			name: "entra password",
			config: Config{
				Db:            "rolling",
				User:          "user@example.com",
				Password:      "secret",
				SSLMode:       "verify-full",
				AuthMode:      database.SQLServerAuthEntraPassword,
				EntraClientID: "application-id",
			},
			driverName: "azuresql",
			fedAuth:    "ActiveDirectoryPassword",
			username:   "user@example.com",
		},
		{
			name: "service principal",
			config: Config{
				Db:            "rolling",
				Password:      "client-secret",
				SSLMode:       "verify-full",
				AuthMode:      database.SQLServerAuthEntraServicePrincipal,
				EntraClientID: "client-id",
				EntraTenantID: "tenant-id",
			},
			driverName: "azuresql",
			fedAuth:    "ActiveDirectoryServicePrincipal",
			username:   "client-id@tenant-id",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn, err := buildConnectionURL(test.config)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := url.Parse(dsn)
			if err != nil {
				t.Fatal(err)
			}
			if got := sqlServerDriverName(test.config); got != test.driverName {
				t.Fatalf("driver = %q", got)
			}
			if got := parsed.Query().Get("fedauth"); got != test.fedAuth {
				t.Fatalf("fedauth = %q", got)
			}
			if test.name == "integrated" &&
				parsed.Query().Get("Integrated Security") != "sspi" {
				t.Fatalf("query = %#v", parsed.Query())
			}
			if test.username != "" && parsed.User.Username() != test.username {
				t.Fatalf("username = %q", parsed.User.Username())
			}
		})
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

func applySQLServerSecurityConformanceChange(
	t *testing.T,
	ctx context.Context,
	driver *SQLServer,
	request database.SecurityChangeRequest,
) {
	t.Helper()
	plan, err := driver.BuildSecurityChange(ctx, request)
	if err != nil {
		t.Fatalf("BuildSecurityChange(%s) error = %v", request.Action, err)
	}
	if err := driver.ApplySecurityChange(ctx, plan); err != nil {
		t.Fatalf("ApplySecurityChange(%s) error = %v", request.Action, err)
	}
}

func runSQLServerSecurityConformance(
	t *testing.T,
	driver *SQLServer,
) {
	t.Helper()
	const (
		loginName = "rt_server_login_conformance"
		roleName  = "rt_server_role_conformance"
		userName  = "rt_database_user_conformance"
		tableName = "rt_security_target_conformance"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cleanupStatements := []string{
		`IF OBJECT_ID(N'dbo.rt_security_target_conformance', N'U') IS NOT NULL
			DROP TABLE [dbo].[rt_security_target_conformance];`,
		`IF USER_ID(N'rt_database_user_conformance') IS NOT NULL
			DROP USER [rt_database_user_conformance];`,
		`IF EXISTS (
				SELECT 1 FROM sys.server_principals
				WHERE type = 'R' AND name = N'rt_server_role_conformance'
			)
			IF IS_SRVROLEMEMBER(
				N'rt_server_role_conformance',
				N'rt_server_login_conformance'
			) = 1
				ALTER SERVER ROLE [rt_server_role_conformance]
					DROP MEMBER [rt_server_login_conformance];`,
		`IF EXISTS (
				SELECT 1 FROM sys.server_principals
				WHERE type = 'R' AND name = N'rt_server_role_conformance'
			)
			DROP SERVER ROLE [rt_server_role_conformance];`,
		`IF SUSER_ID(N'rt_server_login_conformance') IS NOT NULL
			DROP LOGIN [rt_server_login_conformance];`,
	}
	cleanup := func(cleanupContext context.Context) {
		for _, statement := range cleanupStatements {
			_, _ = driver.conn.ExecContext(cleanupContext, statement)
		}
	}
	cleanup(ctx)
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(
			context.Background(),
			time.Minute,
		)
		defer cleanupCancel()
		cleanup(cleanupContext)
	})

	applySQLServerSecurityConformanceChange(
		t,
		ctx,
		driver,
		database.SecurityChangeRequest{
			Action: database.SecurityCreatePrincipal,
			Principal: database.PrincipalOptions{
				Name:     loginName,
				Kind:     database.PrincipalLogin,
				Password: "RollingThunder_Conformance_2026!",
				CanLogin: true,
			},
		},
	)
	applySQLServerSecurityConformanceChange(
		t,
		ctx,
		driver,
		database.SecurityChangeRequest{
			Action: database.SecurityCreatePrincipal,
			Principal: database.PrincipalOptions{
				Name:  roleName,
				Kind:  database.PrincipalRole,
				Scope: "server",
			},
		},
	)
	applySQLServerSecurityConformanceChange(
		t,
		ctx,
		driver,
		database.SecurityChangeRequest{
			Action: database.SecurityGrantRole,
			Grant: database.GrantOptions{
				Grantee:    loginName,
				Role:       roleName,
				ObjectType: "server",
			},
		},
	)
	applySQLServerSecurityConformanceChange(
		t,
		ctx,
		driver,
		database.SecurityChangeRequest{
			Action: database.SecurityGrantPrivilege,
			Grant: database.GrantOptions{
				Grantee:    loginName,
				ObjectType: "server",
				Privilege:  "VIEW ANY DATABASE",
			},
		},
	)
	applySQLServerSecurityConformanceChange(
		t,
		ctx,
		driver,
		database.SecurityChangeRequest{
			Action: database.SecurityCreatePrincipal,
			Principal: database.PrincipalOptions{
				Name:     userName,
				Kind:     database.PrincipalUser,
				Login:    loginName,
				CanLogin: true,
			},
		},
	)
	if _, err := driver.conn.ExecContext(
		ctx,
		"CREATE TABLE [dbo].["+tableName+"] ([id] INT NOT NULL);",
	); err != nil {
		t.Fatalf("create SQL Server security target: %v", err)
	}
	applySQLServerSecurityConformanceChange(
		t,
		ctx,
		driver,
		database.SecurityChangeRequest{
			Action: database.SecurityGrantPrivilege,
			Grant: database.GrantOptions{
				Grantee:    userName,
				ObjectType: "table",
				Schema:     "dbo",
				Object:     tableName,
				Privilege:  "SELECT",
			},
		},
	)

	serverOverview, err := driver.GetSecurityOverview(
		ctx,
		loginName,
		"server",
	)
	if err != nil {
		t.Fatalf("GetSecurityOverview(server) error = %v", err)
	}
	foundServerRole := false
	foundServerPermission := false
	for _, grant := range serverOverview.Grants {
		foundServerRole = foundServerRole ||
			(grant.Role == roleName && grant.Privilege == "MEMBER")
		foundServerPermission = foundServerPermission ||
			(grant.ObjectType == "server" &&
				grant.Privilege == "VIEW ANY DATABASE")
	}
	if !foundServerRole || !foundServerPermission {
		t.Fatalf("server grants = %+v", serverOverview.Grants)
	}
	databaseOverview, err := driver.GetSecurityOverview(
		ctx,
		userName,
		"database",
	)
	if err != nil {
		t.Fatalf("GetSecurityOverview(database) error = %v", err)
	}
	foundTableGrant := false
	for _, grant := range databaseOverview.Grants {
		if grant.ObjectType == "table" &&
			grant.Schema == "dbo" &&
			grant.Object == tableName &&
			grant.Privilege == "SELECT" {
			foundTableGrant = true
			break
		}
	}
	if !foundTableGrant {
		t.Fatalf("database grants = %+v", databaseOverview.Grants)
	}

	changes := []database.SecurityChangeRequest{
		{
			Action: database.SecurityRevokePrivilege,
			Grant: database.GrantOptions{
				Grantee:    userName,
				ObjectType: "table",
				Schema:     "dbo",
				Object:     tableName,
				Privilege:  "SELECT",
			},
		},
		{
			Action: database.SecurityRevokePrivilege,
			Grant: database.GrantOptions{
				Grantee:    loginName,
				ObjectType: "server",
				Privilege:  "VIEW ANY DATABASE",
			},
		},
		{
			Action: database.SecurityRevokeRole,
			Grant: database.GrantOptions{
				Grantee:    loginName,
				Role:       roleName,
				ObjectType: "server",
			},
		},
		{
			Action: database.SecurityDropPrincipal,
			Principal: database.PrincipalOptions{
				Name: userName,
				Kind: database.PrincipalUser,
			},
		},
		{
			Action: database.SecurityDropPrincipal,
			Principal: database.PrincipalOptions{
				Name:  roleName,
				Kind:  database.PrincipalRole,
				Scope: "server",
			},
		},
		{
			Action: database.SecurityDropPrincipal,
			Principal: database.PrincipalOptions{
				Name: loginName,
				Kind: database.PrincipalLogin,
			},
		},
	}
	for _, change := range changes {
		applySQLServerSecurityConformanceChange(t, ctx, driver, change)
	}
	if _, err := driver.conn.ExecContext(
		ctx,
		"DROP TABLE [dbo].["+tableName+"];",
	); err != nil {
		t.Fatalf("drop SQL Server security target: %v", err)
	}
}

func TestSQLServerLiveConformance(t *testing.T) {
	host := os.Getenv("ROLLINGTHUNDER_SQLSERVER_TEST_HOST")
	if host == "" {
		t.Skip("set ROLLINGTHUNDER_SQLSERVER_TEST_HOST to run live SQL Server conformance")
	}
	config := Config{
		Host:        host,
		Port:        os.Getenv("ROLLINGTHUNDER_SQLSERVER_TEST_PORT"),
		User:        os.Getenv("ROLLINGTHUNDER_SQLSERVER_TEST_USER"),
		Password:    os.Getenv("ROLLINGTHUNDER_SQLSERVER_TEST_PASSWORD"),
		Db:          os.Getenv("ROLLINGTHUNDER_SQLSERVER_TEST_DATABASE"),
		SSLMode:     os.Getenv("ROLLINGTHUNDER_SQLSERVER_TEST_SSL_MODE"),
		SSLRootCert: os.Getenv("ROLLINGTHUNDER_SQLSERVER_TEST_SSL_ROOT_CERT"),
		TLSServerName: os.Getenv(
			"ROLLINGTHUNDER_SQLSERVER_TEST_TLS_SERVER_NAME",
		),
	}
	driver := NewSQLServer(context.Background(), config)
	if err := driver.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() {
		if err := driver.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	drivertest.RunLiveContract(t, drivertest.LiveConfig{
		Driver:             driver,
		Schema:             driver.currentSchema,
		IntegerType:        "INT",
		TextType:           "NVARCHAR(255)",
		ExercisePrivileged: os.Getenv("ROLLINGTHUNDER_TEST_PRIVILEGED") == "1",
	})
	if os.Getenv("ROLLINGTHUNDER_TEST_PRIVILEGED") == "1" {
		t.Run("server security parity", func(t *testing.T) {
			runSQLServerSecurityConformance(t, driver)
		})
	}

	if os.Getenv("ROLLINGTHUNDER_SQLSERVER_TEST_BACKUP") != "1" {
		return
	}
	t.Run("native backup and restore", func(t *testing.T) {
		const databaseName = "rt_native_backup_conformance"
		path := os.Getenv("ROLLINGTHUNDER_SQLSERVER_TEST_BACKUP_PATH")
		if path == "" {
			path = "/var/opt/mssql/data/rt_native_backup_conformance.bak"
		}
		dropStatement := `
			IF DB_ID(N'rt_native_backup_conformance') IS NOT NULL
			BEGIN
				ALTER DATABASE [rt_native_backup_conformance]
					SET SINGLE_USER WITH ROLLBACK IMMEDIATE;
				DROP DATABASE [rt_native_backup_conformance];
			END`
		_, _ = driver.conn.ExecContext(context.Background(), dropStatement)
		if _, err := driver.conn.ExecContext(
			context.Background(),
			"CREATE DATABASE "+quoteIdentifier(databaseName),
		); err != nil {
			t.Fatalf("create native-backup database: %v", err)
		}
		t.Cleanup(func() {
			_, _ = driver.conn.ExecContext(context.Background(), dropStatement)
		})

		backupConfig := config
		backupConfig.Db = databaseName
		backupDriver := NewSQLServer(context.Background(), backupConfig)
		if err := backupDriver.Connect(context.Background()); err != nil {
			t.Fatalf("connect native-backup database: %v", err)
		}
		t.Cleanup(func() {
			_ = backupDriver.Close()
		})
		if _, err := backupDriver.conn.ExecContext(
			context.Background(),
			`CREATE TABLE dbo.restore_marker (
				id INT NOT NULL PRIMARY KEY,
				value NVARCHAR(64) NOT NULL
			);
			INSERT INTO dbo.restore_marker (id, value)
			VALUES (1, N'before-backup');`,
		); err != nil {
			t.Fatalf("prepare native-backup marker: %v", err)
		}

		maintenanceCtx, cancel := context.WithTimeout(
			context.Background(),
			5*time.Minute,
		)
		defer cancel()
		metadata, err := backupDriver.BackupDatabaseToServer(
			maintenanceCtx,
			database.BackupRequest{ServerPath: path},
		)
		if err != nil {
			t.Fatalf("BackupDatabaseToServer() error = %v", err)
		}
		if metadata.Database != databaseName ||
			metadata.Bytes <= 0 ||
			metadata.Identity == "" {
			t.Fatalf("backup metadata = %+v", metadata)
		}
		if _, err := backupDriver.conn.ExecContext(
			maintenanceCtx,
			"UPDATE dbo.restore_marker SET value = N'after-backup' WHERE id = 1",
		); err != nil {
			t.Fatalf("mutate native-backup marker: %v", err)
		}
		if err := backupDriver.RestoreDatabaseFromServer(
			maintenanceCtx,
			path,
		); err != nil {
			t.Fatalf("RestoreDatabaseFromServer() error = %v", err)
		}
		var marker string
		if err := backupDriver.conn.QueryRowContext(
			maintenanceCtx,
			"SELECT value FROM dbo.restore_marker WHERE id = 1",
		).Scan(&marker); err != nil {
			t.Fatalf("read restored marker: %v", err)
		}
		if marker != "before-backup" {
			t.Fatalf("restored marker = %q", marker)
		}
	})
}
