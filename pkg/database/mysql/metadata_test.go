package mysql

import (
	"context"
	"database/sql"
	"testing"

	"rollingthunder/pkg/database"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestInformationSchemaAliasesAreCaseStable(t *testing.T) {
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	statements := []string{
		`ATTACH DATABASE ':memory:' AS information_schema`,
		`CREATE TABLE information_schema.columns (
			COLUMN_NAME TEXT,
			DATA_TYPE TEXT,
			COLUMN_TYPE TEXT,
			IS_NULLABLE TEXT,
			CHARACTER_MAXIMUM_LENGTH INTEGER,
			COLUMN_DEFAULT TEXT,
			EXTRA TEXT,
			COLUMN_KEY TEXT,
			GENERATION_EXPRESSION TEXT,
			COLUMN_COMMENT TEXT,
			TABLE_SCHEMA TEXT,
			TABLE_NAME TEXT,
			ORDINAL_POSITION INTEGER
		)`,
		`CREATE TABLE information_schema.key_column_usage (
			COLUMN_NAME TEXT,
			CONSTRAINT_NAME TEXT,
			REFERENCED_TABLE_SCHEMA TEXT,
			REFERENCED_TABLE_NAME TEXT,
			REFERENCED_COLUMN_NAME TEXT,
			TABLE_SCHEMA TEXT,
			TABLE_NAME TEXT,
			ORDINAL_POSITION INTEGER
		)`,
		`CREATE TABLE information_schema.statistics (
			INDEX_NAME TEXT,
			COLUMN_NAME TEXT,
			EXPRESSION TEXT,
			SEQ_IN_INDEX INTEGER,
			NON_UNIQUE INTEGER,
			INDEX_TYPE TEXT,
			TABLE_SCHEMA TEXT,
			TABLE_NAME TEXT
		)`,
		`CREATE TABLE information_schema.tables (
			TABLE_SCHEMA TEXT,
			TABLE_NAME TEXT,
			TABLE_TYPE TEXT,
			ENGINE TEXT,
			TABLE_ROWS INTEGER,
			TABLE_COMMENT TEXT
		)`,
		`CREATE TABLE information_schema.routines (
			ROUTINE_SCHEMA TEXT,
			ROUTINE_NAME TEXT,
			ROUTINE_TYPE TEXT,
			DATA_TYPE TEXT,
			SECURITY_TYPE TEXT,
			SQL_DATA_ACCESS TEXT,
			ROUTINE_COMMENT TEXT
		)`,
		`CREATE TABLE information_schema.triggers (
			TRIGGER_SCHEMA TEXT,
			TRIGGER_NAME TEXT,
			EVENT_OBJECT_SCHEMA TEXT,
			EVENT_OBJECT_TABLE TEXT,
			EVENT_MANIPULATION TEXT,
			ACTION_TIMING TEXT
		)`,
		`CREATE TABLE information_schema.table_constraints (
			CONSTRAINT_SCHEMA TEXT,
			CONSTRAINT_NAME TEXT,
			TABLE_SCHEMA TEXT,
			TABLE_NAME TEXT,
			CONSTRAINT_TYPE TEXT
		)`,
		`INSERT INTO information_schema.columns VALUES (
			'id', 'int', 'int', 'NO', NULL, NULL, 'auto_increment',
			'PRI', NULL, '', 'rolling', 'storms', 1
		)`,
		`INSERT INTO information_schema.statistics VALUES (
			'PRIMARY', 'id', NULL, 1, 0, 'BTREE', 'rolling', 'storms'
		)`,
		`INSERT INTO information_schema.tables VALUES (
			'rolling', 'storms', 'BASE TABLE', 'InnoDB', 1, ''
		)`,
		`INSERT INTO information_schema.table_constraints VALUES (
			'rolling', 'PRIMARY', 'rolling', 'storms', 'PRIMARY KEY'
		)`,
	}
	for _, statement := range statements {
		if _, err := sqlDB.Exec(statement); err != nil {
			t.Fatalf("prepare information_schema fixture: %v", err)
		}
	}

	driver := &MySQL{
		cfg:  Config{Db: "rolling"},
		conn: sqlx.NewDb(sqlDB, "sqlite"),
	}
	table := database.Table{Schema: "rolling", Name: "storms"}

	structures, err := driver.GetCollectionStructures(table)
	if err != nil {
		t.Fatalf("GetCollectionStructures() error = %v", err)
	}
	if len(structures) != 1 || structures[0].Name != "id" ||
		!structures[0].IsPrimary {
		t.Fatalf("GetCollectionStructures() = %+v", structures)
	}

	indices, err := driver.GetIndices(table)
	if err != nil {
		t.Fatalf("GetIndices() error = %v", err)
	}
	if len(indices) != 1 || indices[0].Name != "PRIMARY" ||
		!indices[0].IsPrimary {
		t.Fatalf("GetIndices() = %+v", indices)
	}

	objects, err := driver.ListObjects(
		context.Background(),
		database.ObjectFilter{Schema: "rolling"},
	)
	if err != nil {
		t.Fatalf("ListObjects() error = %v", err)
	}
	foundTable := false
	for _, object := range objects {
		if object.Reference.Kind == database.ObjectKindTable &&
			object.Reference.Name == "storms" {
			foundTable = true
			break
		}
	}
	if !foundTable {
		t.Fatalf("ListObjects() = %+v, missing storms table", objects)
	}
}
