package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/database/drivertest"
)

func TestSQLiteCapabilityContract(t *testing.T) {
	driver := NewSQLite(context.Background(), Config{Db: ":memory:"})
	drivertest.RunCapabilityContract(t, driver, "sqlite")
}

func TestSQLiteKeepsDurableOperationContextAfterConnectAttemptEnds(t *testing.T) {
	connectContext, cancelConnect := context.WithCancel(context.Background())
	driver := NewSQLite(connectContext, Config{Db: ":memory:"})
	if err := driver.Connect(connectContext); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = driver.Close() })
	cancelConnect()

	if err := driver.CreateTable(
		table("main", "durable_context"),
		[]database.ColumnDefinition{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "name", Type: "TEXT", Nullable: true},
		},
	); err != nil {
		t.Fatalf("CreateTable() after connect cancellation error = %v", err)
	}
	if _, err := driver.ApplyTableChanges(
		nil,
		database.TableChangeSet{
			Table: table("main", "durable_context"),
			Added: []map[string]interface{}{{
				"id": int64(1), "name": "storm",
			}},
		},
	); err != nil {
		t.Fatalf("ApplyTableChanges() after connect cancellation error = %v", err)
	}
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

func TestSQLiteViewDetailIncludesStructureAndDependency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "views.sqlite3")
	driver := NewSQLite(context.Background(), Config{Db: path})
	if err := driver.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = driver.Close() })
	if _, err := driver.conn.Exec(`
		CREATE TABLE main.orders (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL
		);
		CREATE VIEW main.order_names AS
		SELECT id, name FROM main.orders;
	`); err != nil {
		t.Fatalf("create view fixture: %v", err)
	}

	structures, err := driver.GetCollectionStructures(
		table("main", "order_names"),
	)
	if err != nil {
		t.Fatalf("GetCollectionStructures(view) error = %v", err)
	}
	if len(structures) != 2 ||
		structures[0].Name != "id" ||
		structures[1].Name != "name" {
		t.Fatalf("view structures = %+v", structures)
	}

	detail, err := driver.GetObjectDetail(
		context.Background(),
		database.ObjectReference{
			Kind:   database.ObjectKindView,
			Schema: "main",
			Name:   "order_names",
		},
	)
	if err != nil {
		t.Fatalf("GetObjectDetail(view) error = %v", err)
	}
	if len(detail.Columns) != 2 ||
		len(detail.Dependencies) != 1 ||
		detail.Dependencies[0].Reference.Name != "orders" {
		t.Fatalf("view detail = %+v", detail)
	}
}

func TestSQLiteSQLReferenceDetectionUsesIdentifierBoundaries(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want bool
	}{
		{
			name: "unquoted qualified identifier before punctuation",
			sql:  "SELECT * FROM main.orders;",
			want: true,
		},
		{
			name: "quoted identifier",
			sql:  `SELECT * FROM "main"."orders"`,
			want: true,
		},
		{
			name: "bracket identifier",
			sql:  "SELECT * FROM [main].[orders]",
			want: true,
		},
		{
			name: "string literal is not an identifier",
			sql:  "SELECT 'orders'",
			want: false,
		},
		{
			name: "comment is not an identifier",
			sql:  "SELECT 1 -- orders\n",
			want: false,
		},
		{
			name: "identifier substring",
			sql:  "SELECT * FROM main.orders_archive",
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := sqliteSQLReferencesName(test.sql, "orders"); got != test.want {
				t.Fatalf(
					"sqliteSQLReferencesName(%q, orders) = %t, want %t",
					test.sql,
					got,
					test.want,
				)
			}
		})
	}
}

func TestSQLiteReviewedAddAndDropColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "structure.sqlite3")
	driver := NewSQLite(context.Background(), Config{Db: path})
	if err := driver.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = driver.Close() })
	if _, err := driver.conn.Exec(
		`CREATE TABLE main.orders (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`,
	); err != nil {
		t.Fatalf("create fixture table: %v", err)
	}

	add, err := driver.BuildObjectChange(
		context.Background(),
		database.ObjectChangeRequest{
			Action: database.ObjectChangeAddColumn,
			AddColumn: &database.AddColumnChange{
				Table: table("main", "orders"),
				Column: database.ColumnDefinition{
					Name:     "status",
					Type:     "TEXT",
					Nullable: false,
					Default:  "'pending'",
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("BuildObjectChange(add) error = %v", err)
	}
	if err := driver.ApplyObjectChange(context.Background(), add); err != nil {
		t.Fatalf("ApplyObjectChange(add) error = %v", err)
	}
	structures, err := driver.GetCollectionStructures(table("main", "orders"))
	if err != nil || len(structures) != 3 || structures[2].Name != "status" {
		t.Fatalf("structures after add = %+v, %v", structures, err)
	}

	drop, err := driver.BuildObjectChange(
		context.Background(),
		database.ObjectChangeRequest{
			Action: database.ObjectChangeDropColumn,
			DropColumn: &database.DropColumnChange{
				Table: table("main", "orders"),
				Name:  "status",
			},
		},
	)
	if err != nil {
		t.Fatalf("BuildObjectChange(drop) error = %v", err)
	}
	if err := driver.ApplyObjectChange(context.Background(), drop); err != nil {
		t.Fatalf("ApplyObjectChange(drop) error = %v", err)
	}
	structures, err = driver.GetCollectionStructures(table("main", "orders"))
	if err != nil || len(structures) != 2 {
		t.Fatalf("structures after drop = %+v, %v", structures, err)
	}
}

func TestSQLiteReviewedTriggerLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "triggers.sqlite3")
	driver := NewSQLite(context.Background(), Config{Db: path})
	if err := driver.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = driver.Close() })
	if _, err := driver.conn.Exec(`
		CREATE TABLE main.events (
			id INTEGER PRIMARY KEY,
			touched INTEGER NOT NULL DEFAULT 0
		);
	`); err != nil {
		t.Fatalf("create trigger fixture table: %v", err)
	}
	ctx := context.Background()
	reference := database.ObjectReference{
		Kind:         database.ObjectKindTrigger,
		Schema:       "main",
		Name:         "touch_inserted_event",
		ParentSchema: "main",
		ParentName:   "events",
	}
	create, err := driver.BuildObjectChange(
		ctx,
		database.ObjectChangeRequest{
			Action:    database.ObjectChangeCreate,
			Reference: reference,
			Definition: `CREATE TRIGGER main.touch_inserted_event
				AFTER INSERT ON events
				BEGIN
					UPDATE events SET touched = 1 WHERE id = NEW.id;
				END`,
		},
	)
	if err != nil {
		t.Fatalf("BuildObjectChange(create trigger) error = %v", err)
	}
	if err := driver.ApplyObjectChange(ctx, create); err != nil {
		t.Fatalf("ApplyObjectChange(create trigger) error = %v", err)
	}
	if _, err := driver.conn.Exec(
		"INSERT INTO main.events(id) VALUES (1)",
	); err != nil {
		t.Fatalf("insert trigger fixture row: %v", err)
	}
	var touched int
	if err := driver.conn.Get(
		&touched,
		"SELECT touched FROM main.events WHERE id = 1",
	); err != nil {
		t.Fatalf("read trigger fixture row: %v", err)
	}
	if touched != 1 {
		t.Fatalf("trigger result = %d, want 1", touched)
	}

	objects, err := driver.ListObjects(ctx, database.ObjectFilter{
		Schema: "main",
		Kinds:  []database.ObjectKind{database.ObjectKindTrigger},
		Search: reference.Name,
	})
	if err != nil {
		t.Fatalf("ListObjects(trigger) error = %v", err)
	}
	if len(objects) != 1 {
		t.Fatalf("trigger objects = %+v", objects)
	}
	detail, err := driver.GetObjectDetail(ctx, objects[0].Reference)
	if err != nil {
		t.Fatalf("GetObjectDetail(trigger) error = %v", err)
	}
	if detail.Definition == "" ||
		detail.Object.Reference.ParentName != "events" {
		t.Fatalf("trigger detail = %+v", detail)
	}

	drop, err := driver.BuildObjectChange(
		ctx,
		database.ObjectChangeRequest{
			Action:    database.ObjectChangeDrop,
			Reference: objects[0].Reference,
		},
	)
	if err != nil {
		t.Fatalf("BuildObjectChange(drop trigger) error = %v", err)
	}
	if err := driver.ApplyObjectChange(ctx, drop); err != nil {
		t.Fatalf("ApplyObjectChange(drop trigger) error = %v", err)
	}
}

func TestSQLiteOnlineBackupAndRestore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "online.sqlite3")
	driver := NewSQLite(context.Background(), Config{Db: path})
	if err := driver.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = driver.Close() })
	ctx := context.Background()
	if _, err := driver.conn.ExecContext(
		ctx,
		`CREATE TABLE backup_items (id INTEGER PRIMARY KEY, label TEXT NOT NULL);
		 INSERT INTO backup_items(label) VALUES ('before');`,
	); err != nil {
		t.Fatalf("seed backup database: %v", err)
	}

	backupPath := filepath.Join(t.TempDir(), "snapshot.sqlite3")
	// The service creates a same-directory temporary file before handing it
	// to the online backup driver, so cover an existing empty destination.
	if err := os.WriteFile(backupPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := driver.BackupDatabase(ctx, backupPath); err != nil {
		t.Fatalf("backup database: %v", err)
	}
	if _, err := driver.conn.ExecContext(
		ctx,
		`DELETE FROM backup_items;
		 INSERT INTO backup_items(label) VALUES ('after');`,
	); err != nil {
		t.Fatalf("mutate database: %v", err)
	}
	if err := driver.RestoreDatabase(ctx, backupPath); err != nil {
		t.Fatalf("restore database: %v", err)
	}

	var label string
	if err := driver.conn.GetContext(
		ctx,
		&label,
		"SELECT label FROM backup_items",
	); err != nil {
		t.Fatalf("read restored row: %v", err)
	}
	if label != "before" {
		t.Fatalf("restored label = %q, want before", label)
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
