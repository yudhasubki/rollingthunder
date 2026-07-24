package postgres

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"rollingthunder/pkg/database"
)

type fakeExportRows struct {
	columns     []string
	rows        [][]interface{}
	index       int
	columnsErr  error
	valuesErrAt int
	resultErr   error
}

func (rows *fakeExportRows) Columns() ([]string, error) {
	return rows.columns, rows.columnsErr
}

func (rows *fakeExportRows) Next() bool {
	return rows.index < len(rows.rows)
}

func (rows *fakeExportRows) Values() ([]interface{}, error) {
	if rows.valuesErrAt > 0 && rows.index+1 == rows.valuesErrAt {
		rows.index++
		return nil, errors.New("scan failed")
	}
	row := rows.rows[rows.index]
	rows.index++
	return row, nil
}

func (rows *fakeExportRows) Err() error {
	return rows.resultErr
}

func TestPostgresSQLLiteralProjectionSkipsGeneratedColumns(t *testing.T) {
	generationMode := "ALWAYS"
	columns := Columns{
		{ColumnName: "id", IsIdentity: "YES", IdentityMode: &generationMode},
		{ColumnName: `display"name`, IsGenerated: "ALWAYS"},
		{ColumnName: "payload"},
	}

	insertable := postgresInsertableColumns(columns)
	if len(insertable) != 2 ||
		insertable[0].ColumnName != "id" ||
		insertable[1].ColumnName != "payload" {
		t.Fatalf("insertable columns = %+v", insertable)
	}

	projection, err := postgresSQLLiteralProjection(insertable)
	if err != nil {
		t.Fatalf("build SQL literal projection: %v", err)
	}
	const expected = `pg_catalog.quote_nullable("id") AS "id", pg_catalog.quote_nullable("payload") AS "payload"`
	if projection != expected {
		t.Fatalf("projection = %q, want %q", projection, expected)
	}
}

func TestWritePostgresInsertStreamBatchesRowsAndOverridesIdentity(t *testing.T) {
	generationMode := "ALWAYS"
	columns := Columns{
		{ColumnName: "id", IsIdentity: "YES", IdentityMode: &generationMode},
		{ColumnName: "name"},
		{ColumnName: "metadata"},
	}
	rows := &fakeExportRows{
		columns: []string{"id", "name", "metadata"},
		rows: [][]interface{}{
			{[]byte("1"), "'Alpha'", `'{"active":true}'`},
			{"2", "NULL", `'{"active":false}'`},
			{"3", "'Bravo'", "NULL"},
		},
	}
	var output bytes.Buffer

	stats, err := writePostgresInsertStream(
		&output,
		rows,
		database.Table{Schema: `odd"schema`, Name: "order items"},
		columns,
		database.SQLInsertOptions{
			BatchSize:          2,
			IncludeTransaction: true,
		},
	)
	if err != nil {
		t.Fatalf("write PostgreSQL INSERT stream: %v", err)
	}
	if stats.Rows != 3 {
		t.Fatalf("row count = %d, want 3", stats.Rows)
	}

	const expected = `-- Rolling Thunder PostgreSQL INSERT export
-- Generated columns are omitted; sequence state is not modified.

BEGIN;

INSERT INTO "odd""schema"."order items" ("id", "name", "metadata") OVERRIDING SYSTEM VALUE VALUES
  (1, 'Alpha', '{"active":true}'),
  (2, NULL, '{"active":false}');

INSERT INTO "odd""schema"."order items" ("id", "name", "metadata") OVERRIDING SYSTEM VALUE VALUES
  (3, 'Bravo', NULL);

COMMIT;
`
	if output.String() != expected {
		t.Fatalf("SQL output:\n%s\nwant:\n%s", output.String(), expected)
	}
}

func TestWritePostgresInsertStreamSupportsEmptyNonTransactionExport(t *testing.T) {
	var output bytes.Buffer
	stats, err := writePostgresInsertStream(
		&output,
		&fakeExportRows{columns: []string{"id"}},
		database.Table{Schema: "public", Name: "orders"},
		Columns{{ColumnName: "id"}},
		database.SQLInsertOptions{},
	)
	if err != nil {
		t.Fatalf("write empty PostgreSQL INSERT stream: %v", err)
	}
	if stats.Rows != 0 {
		t.Fatalf("row count = %d, want 0", stats.Rows)
	}
	if !strings.Contains(output.String(), "-- No rows matched the export scope.\n") {
		t.Fatalf("missing empty-export marker: %q", output.String())
	}
	if strings.Contains(output.String(), "BEGIN;") ||
		strings.Contains(output.String(), "COMMIT;") {
		t.Fatalf("unexpected transaction wrapper: %q", output.String())
	}
}

func TestWritePostgresInsertStreamRejectsInvalidStreamValues(t *testing.T) {
	_, err := writePostgresInsertStream(
		&bytes.Buffer{},
		&fakeExportRows{
			columns: []string{"id"},
			rows:    [][]interface{}{{42}},
		},
		database.Table{Schema: "public", Name: "orders"},
		Columns{{ColumnName: "id"}},
		database.SQLInsertOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), "unexpected type int") {
		t.Fatalf("expected quoted-value type error, got %v", err)
	}
}

func TestWritePostgresInsertStreamPropagatesStreamErrors(t *testing.T) {
	columns := Columns{{ColumnName: "id"}}
	table := database.Table{Schema: "public", Name: "orders"}

	_, err := writePostgresInsertStream(
		&bytes.Buffer{},
		&fakeExportRows{
			columns:     []string{"id"},
			rows:        [][]interface{}{{"1"}},
			valuesErrAt: 1,
		},
		table,
		columns,
		database.SQLInsertOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), "scan failed") {
		t.Fatalf("expected scan error, got %v", err)
	}

	_, err = writePostgresInsertStream(
		&bytes.Buffer{},
		&fakeExportRows{
			columns:   []string{"id"},
			resultErr: errors.New("iteration failed"),
		},
		table,
		columns,
		database.SQLInsertOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), "iteration failed") {
		t.Fatalf("expected iteration error, got %v", err)
	}
}

func TestWritePostgresInsertStreamRejectsColumnMismatch(t *testing.T) {
	_, err := writePostgresInsertStream(
		&bytes.Buffer{},
		&fakeExportRows{columns: []string{"wrong_column"}},
		database.Table{Schema: "public", Name: "orders"},
		Columns{{ColumnName: "id"}},
		database.SQLInsertOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), `expected "id"`) {
		t.Fatalf("expected column mismatch error, got %v", err)
	}
}

func TestBuildPostgresExportQueryUsesSQLLiteralProjection(t *testing.T) {
	query, err := buildPostgresExportQueryWithProjection(
		database.Table{
			Schema: "public",
			Name:   "orders",
			Limit:  100,
			Filter: `"status" = 'open'`,
		},
		database.ExportScopePage,
		database.Structures{{Name: "id", IsPrimary: true}},
		`pg_catalog.quote_nullable("id") AS "id"`,
	)
	if err != nil {
		t.Fatalf("build projected export query: %v", err)
	}
	const expected = `SELECT pg_catalog.quote_nullable("id") AS "id" FROM "public"."orders" WHERE "status" = 'open' ORDER BY "id" ASC NULLS LAST LIMIT 100 OFFSET 0`
	if query != expected {
		t.Fatalf("query = %q, want %q", query, expected)
	}
}
