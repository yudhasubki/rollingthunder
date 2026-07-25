package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"rollingthunder/pkg/database"
	sqlitedriver "rollingthunder/pkg/database/sqlite"
)

func sqliteMigrationDriver(t *testing.T, path string) *sqlitedriver.SQLite {
	t.Helper()
	driver := sqlitedriver.NewSQLite(
		context.Background(),
		sqlitedriver.Config{Db: path},
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := driver.Connect(ctx); err != nil {
		t.Fatalf("connect SQLite migration fixture: %v", err)
	}
	t.Cleanup(func() { _ = driver.Close() })
	return driver
}

func schemaMigrationService(
	source database.Driver,
	target database.Driver,
) *Service {
	service := NewService()
	service.connections = map[string]*Connection{
		"source": {
			ID:     "source",
			Name:   "Source",
			Driver: source,
			Config: database.Config{Driver: "sqlite"},
		},
		"target": {
			ID:     "target",
			Name:   "Target",
			Driver: target,
			Config: database.Config{Driver: "sqlite"},
		},
	}
	return service
}

func sqliteMigrationRequest(includeDestructive bool) database.SchemaMigrationRequest {
	return database.SchemaMigrationRequest{
		SourceConnectionID: "source",
		SourceSchema:       "main",
		TargetConnectionID: "target",
		TargetSchema:       "main",
		IncludeDestructive: includeDestructive,
	}
}

func TestPreviewAndApplySchemaMigration(t *testing.T) {
	source := sqliteMigrationDriver(
		t,
		filepath.Join(t.TempDir(), "source.sqlite"),
	)
	target := sqliteMigrationDriver(
		t,
		filepath.Join(t.TempDir(), "target.sqlite"),
	)
	if _, err := source.ExecuteQuery(
		context.Background(),
		`CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			email TEXT NOT NULL UNIQUE
		);
		CREATE INDEX users_email_idx ON users(email);`,
		database.QueryOptions{},
	); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	service := schemaMigrationService(source, target)
	request := sqliteMigrationRequest(false)

	preview := service.PreviewSchemaMigration(request)
	if len(preview.Errors) != 0 {
		t.Fatalf("preview errors = %+v", preview.Errors)
	}
	if preview.Data.StatementCount < 1 ||
		preview.Data.Fingerprint == "" ||
		preview.Data.SelectedChanges < 1 {
		t.Fatalf("preview = %+v", preview.Data)
	}

	applied := service.ApplySchemaMigration(database.ApplySchemaMigrationRequest{
		Migration:   request,
		Fingerprint: preview.Data.Fingerprint,
	})
	if len(applied.Errors) != 0 || !applied.Data.Applied {
		t.Fatalf("apply = %+v", applied)
	}

	after := service.PreviewSchemaMigration(request)
	if len(after.Errors) != 0 {
		t.Fatalf("after errors = %+v", after.Errors)
	}
	if after.Data.StatementCount != 0 {
		t.Fatalf("schema still differs: %+v", after.Data)
	}
}

func TestSchemaMigrationRequiresDestructiveOptIn(t *testing.T) {
	source := sqliteMigrationDriver(
		t,
		filepath.Join(t.TempDir(), "source.sqlite"),
	)
	target := sqliteMigrationDriver(
		t,
		filepath.Join(t.TempDir(), "target.sqlite"),
	)
	if _, err := target.ExecuteQuery(
		context.Background(),
		`CREATE TABLE legacy (id INTEGER PRIMARY KEY);`,
		database.QueryOptions{},
	); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	service := schemaMigrationService(source, target)

	safe := service.PreviewSchemaMigration(sqliteMigrationRequest(false))
	if len(safe.Errors) != 0 {
		t.Fatalf("safe preview errors = %+v", safe.Errors)
	}
	if safe.Data.StatementCount != 0 || safe.Data.ManualChanges != 1 {
		t.Fatalf("safe preview = %+v", safe.Data)
	}

	destructive := service.PreviewSchemaMigration(
		sqliteMigrationRequest(true),
	)
	if len(destructive.Errors) != 0 {
		t.Fatalf("destructive preview errors = %+v", destructive.Errors)
	}
	if destructive.Data.StatementCount != 1 || !destructive.Data.Destructive {
		t.Fatalf("destructive preview = %+v", destructive.Data)
	}
}
