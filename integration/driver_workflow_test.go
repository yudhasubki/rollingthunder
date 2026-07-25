package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"rollingthunder/internal/db"
	"rollingthunder/pkg/database"

	"github.com/google/uuid"
)

func environment(name string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func exerciseDriverWorkflow(
	t *testing.T,
	driverName string,
	config database.Config,
	schema string,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	driver, err := db.NewDriver(ctx, driverName, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.Connect(ctx); err != nil {
		t.Fatalf("connect %s: %v", driverName, err)
	}
	t.Cleanup(func() {
		_ = driver.Close()
	})
	if healthDriver, ok := driver.(database.HealthDriver); ok {
		if err := healthDriver.Ping(ctx); err != nil {
			t.Fatalf("ping %s: %v", driverName, err)
		}
	}

	tableName := "rt_integration_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	table := database.Table{Schema: schema, Name: tableName}
	textType := "TEXT"
	switch driver.Capabilities().Engine {
	case database.DriverOracle:
		textType = "VARCHAR2(255)"
	case database.DriverSQLServer:
		textType = "NVARCHAR(255)"
	}
	if err := driver.CreateTable(table, []database.ColumnDefinition{
		{
			Name:       "id",
			Type:       "INTEGER",
			PrimaryKey: true,
			Nullable:   false,
		},
		{
			Name:     "name",
			Type:     textType,
			Nullable: false,
		},
	}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	dropped := false
	t.Cleanup(func() {
		if !dropped {
			_ = driver.DropTable(table)
		}
	})

	if err := driver.InsertRow(table, map[string]interface{}{
		"id":   1,
		"name": "storm",
	}); err != nil {
		t.Fatalf("insert row: %v", err)
	}
	if count, err := driver.CountCollectionData(table); err != nil || count != 1 {
		t.Fatalf("count rows = %d, %v", count, err)
	}
	structures, err := driver.GetCollectionStructures(table)
	if err != nil || len(structures) != 2 {
		t.Fatalf("structures = %+v, %v", structures, err)
	}
	query := fmt.Sprintf(
		"SELECT %s, %s FROM %s.%s WHERE %s = %s",
		driver.QuoteIdentifier("id"),
		driver.QuoteIdentifier("name"),
		driver.QuoteIdentifier(schema),
		driver.QuoteIdentifier(tableName),
		driver.QuoteIdentifier("id"),
		driver.Placeholder(1),
	)
	result, err := driver.ExecuteQuery(ctx, query, database.QueryOptions{
		MaxRows: 10,
		Args:    []interface{}{1},
	})
	if err != nil || len(result.Rows) != 1 || result.Rows[0]["name"] != "storm" {
		t.Fatalf("query result = %+v, %v", result, err)
	}

	changeDriver, ok := driver.(database.TableChangeDriver)
	if !ok {
		t.Fatalf("%s does not implement atomic table changes", driverName)
	}
	changed, err := changeDriver.ApplyTableChanges(ctx, database.TableChangeSet{
		Table: table,
		Updated: []database.RowUpdate{
			{
				Original:       map[string]interface{}{"id": 1, "name": "storm"},
				Values:         map[string]interface{}{"id": 1, "name": "thunder"},
				ChangedColumns: []string{"name"},
			},
		},
	})
	if err != nil || changed.Updated != 1 {
		t.Fatalf("atomic update = %+v, %v", changed, err)
	}

	var exported bytes.Buffer
	stats, err := driver.ExportTable(ctx, database.TableExportRequest{
		Table: table,
		Scope: database.ExportScopeAll,
		Options: database.ExportOptions{
			Format: database.ExportFormatCSV,
			CSV: database.CSVOptions{
				Delimiter:     ",",
				IncludeHeader: true,
				Encoding:      database.CSVEncodingUTF8,
			},
		},
	}, &exported)
	if err != nil || stats.Rows != 1 ||
		!strings.Contains(exported.String(), "thunder") {
		t.Fatalf("export = %q, %+v, %v", exported.String(), stats, err)
	}

	deleted, err := changeDriver.ApplyTableChanges(ctx, database.TableChangeSet{
		Table:   table,
		Deleted: []map[string]interface{}{{"id": 1, "name": "thunder"}},
	})
	if err != nil || deleted.Deleted != 1 {
		t.Fatalf("atomic delete = %+v, %v", deleted, err)
	}
	if err := driver.DropTable(table); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	dropped = true
	collections, err := driver.GetCollections(schema)
	if err != nil {
		t.Fatalf("collections after drop: %v", err)
	}
	for _, collection := range collections {
		if collection == tableName {
			t.Fatalf("dropped table %q is still listed", tableName)
		}
	}
}

func TestSQLiteDriverWorkflow(t *testing.T) {
	exerciseDriverWorkflow(t, "sqlite", database.Config{
		Driver: "sqlite",
		Db:     filepath.Join(t.TempDir(), "integration.sqlite3"),
	}, "main")
}

func TestPostgreSQLDriverWorkflow(t *testing.T) {
	if os.Getenv("RT_INTEGRATION_POSTGRES") != "1" {
		t.Skip("set RT_INTEGRATION_POSTGRES=1 to run PostgreSQL integration")
	}
	exerciseDriverWorkflow(t, "postgres", database.Config{
		Driver:   "postgres",
		Host:     environment("RT_DATABASE_HOST", "127.0.0.1"),
		Port:     environment("RT_DATABASE_PORT", "5432"),
		User:     environment("RT_DATABASE_USER", "rolling"),
		Password: environment("RT_DATABASE_PASSWORD", "rolling"),
		Db:       environment("RT_DATABASE_NAME", "rolling"),
		SSLMode:  "disable",
	}, "public")
}

func TestMySQLCompatibleDriverWorkflow(t *testing.T) {
	if os.Getenv("RT_INTEGRATION_MYSQL") != "1" {
		t.Skip("set RT_INTEGRATION_MYSQL=1 to run MySQL/MariaDB integration")
	}
	port := environment("RT_DATABASE_PORT", "3306")
	if _, err := strconv.Atoi(port); err != nil {
		t.Fatalf("invalid RT_DATABASE_PORT: %v", err)
	}
	driverName := environment("RT_DATABASE_DRIVER", "mysql")
	exerciseDriverWorkflow(t, driverName, database.Config{
		Driver:   driverName,
		Host:     environment("RT_DATABASE_HOST", "127.0.0.1"),
		Port:     port,
		User:     environment("RT_DATABASE_USER", "root"),
		Password: environment("RT_DATABASE_PASSWORD", "rolling"),
		Db:       environment("RT_DATABASE_NAME", "rolling"),
		SSLMode:  "disable",
	}, environment("RT_DATABASE_NAME", "rolling"))
}

func TestOracleDriverWorkflow(t *testing.T) {
	if os.Getenv("RT_INTEGRATION_ORACLE") != "1" {
		t.Skip("set RT_INTEGRATION_ORACLE=1 to run Oracle integration")
	}
	exerciseDriverWorkflow(t, database.DriverOracle, database.Config{
		Driver:   database.DriverOracle,
		Host:     environment("RT_DATABASE_HOST", "127.0.0.1"),
		Port:     environment("RT_DATABASE_PORT", "1521"),
		User:     environment("RT_DATABASE_USER", "system"),
		Password: environment("RT_DATABASE_PASSWORD", "RollingThunder_2026"),
		Db:       environment("RT_DATABASE_NAME", "FREEPDB1"),
		SSLMode:  environment("RT_DATABASE_SSL_MODE", "disable"),
	}, strings.ToUpper(environment("RT_DATABASE_SCHEMA", "SYSTEM")))
}

func TestSQLServerDriverWorkflow(t *testing.T) {
	if os.Getenv("RT_INTEGRATION_SQLSERVER") != "1" {
		t.Skip("set RT_INTEGRATION_SQLSERVER=1 to run SQL Server integration")
	}
	exerciseDriverWorkflow(t, database.DriverSQLServer, database.Config{
		Driver:   database.DriverSQLServer,
		Host:     environment("RT_DATABASE_HOST", "127.0.0.1"),
		Port:     environment("RT_DATABASE_PORT", "1433"),
		User:     environment("RT_DATABASE_USER", "sa"),
		Password: environment("RT_DATABASE_PASSWORD", "RollingThunder_2026!"),
		Db:       environment("RT_DATABASE_NAME", "master"),
		SSLMode:  environment("RT_DATABASE_SSL_MODE", "disable"),
	}, environment("RT_DATABASE_SCHEMA", "dbo"))
}
