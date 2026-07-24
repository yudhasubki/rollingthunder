package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/database/drivertest"
)

func TestSQLiteCapabilityContract(t *testing.T) {
	driver := NewSQLite(context.Background(), Config{Db: ":memory:"})
	drivertest.RunCapabilityContract(t, driver, "sqlite")
}

func TestSQLiteLiveConformance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conformance.sqlite3")
	driver := NewSQLite(context.Background(), Config{Db: path})
	if err := driver.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() {
		if err := driver.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	drivertest.RunLiveContract(t, drivertest.LiveConfig{
		Driver:      driver,
		Schema:      "main",
		IntegerType: "INTEGER",
		TextType:    "TEXT",
	})
}

func TestSQLiteAttachedDatabaseAndGeneratedMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.sqlite3")
	attached := filepath.Join(t.TempDir(), "analytics.sqlite3")
	driver := NewSQLite(context.Background(), Config{Db: path})
	if err := driver.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = driver.Close() })

	if _, err := driver.conn.Exec(
		"ATTACH DATABASE ? AS analytics",
		attached,
	); err != nil {
		t.Fatalf("ATTACH DATABASE error = %v", err)
	}
	if _, err := driver.conn.Exec(`
		CREATE TABLE main.metrics (
			id INTEGER PRIMARY KEY,
			raw INTEGER NOT NULL,
			doubled INTEGER GENERATED ALWAYS AS (raw * 2) STORED
		)`); err != nil {
		t.Fatalf("CREATE TABLE error = %v", err)
	}
	if _, err := driver.conn.Exec(
		"CREATE TABLE analytics.events (id INTEGER PRIMARY KEY)",
	); err != nil {
		t.Fatalf("CREATE attached table error = %v", err)
	}

	schemas, err := driver.GetSchemas()
	if err != nil {
		t.Fatalf("GetSchemas() error = %v", err)
	}
	if !containsSQLite(schemas, "main") || !containsSQLite(schemas, "analytics") {
		t.Fatalf("GetSchemas() = %v", schemas)
	}
	structures, err := driver.GetCollectionStructures(
		table("main", "metrics"),
	)
	if err != nil {
		t.Fatalf("GetCollectionStructures() error = %v", err)
	}
	if len(structures) != 3 {
		t.Fatalf("structures = %+v", structures)
	}
	if !structures[0].IsRowID || !structures[0].IsAutoInc {
		t.Fatalf("integer primary key rowid metadata = %+v", structures[0])
	}
	if !structures[2].IsGenerated || structures[2].Generation != "STORED" {
		t.Fatalf("generated metadata = %+v", structures[2])
	}
}

func table(schema, name string) database.Table {
	return database.Table{Schema: schema, Name: name}
}

func containsSQLite(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
