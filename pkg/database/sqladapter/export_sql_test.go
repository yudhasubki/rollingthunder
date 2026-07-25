package sqladapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"rollingthunder/pkg/database"
)

type insertExportRows struct {
	columns []string
	rows    [][]interface{}
	index   int
}

func (rows *insertExportRows) Columns() ([]string, error) {
	return append([]string(nil), rows.columns...), nil
}

func (rows *insertExportRows) Next() bool {
	return rows.index < len(rows.rows)
}

func (rows *insertExportRows) Values() ([]interface{}, error) {
	if rows.index >= len(rows.rows) {
		return nil, fmt.Errorf("row is unavailable")
	}
	value := append([]interface{}(nil), rows.rows[rows.index]...)
	rows.index++
	return value, nil
}

func (rows *insertExportRows) Err() error {
	return nil
}

func TestWriteInsertExportUsesBoundedBatches(t *testing.T) {
	var output bytes.Buffer
	stats, err := WriteInsertExportContext(
		context.Background(),
		&output,
		&insertExportRows{
			columns: []string{"id", "name"},
			rows: [][]interface{}{
				{1, "Ada"},
				{2, "Grace"},
				{3, "Linus"},
			},
		},
		database.Table{Schema: "dbo", Name: "people"},
		database.Structures{
			{Name: "id"},
			{Name: "name"},
		},
		database.SQLInsertOptions{
			BatchSize:          2,
			IncludeTransaction: true,
		},
		InsertExportDialect{
			EngineLabel: "Test SQL",
			QuoteIdentifier: func(value string) string {
				return "[" + value + "]"
			},
			QuoteQualified: func(schema, name string) string {
				return "[" + schema + "].[" + name + "]"
			},
			Literal: func(
				value interface{},
				_ database.Structure,
			) (string, error) {
				if number, ok, err := SQLNumericLiteral(value); ok {
					return number, err
				}
				return "'" + strings.ReplaceAll(
					fmt.Sprint(value),
					"'",
					"''",
				) + "'", nil
			},
			BeginStatement:   "BEGIN TRANSACTION;",
			CommitStatement:  "COMMIT;",
			MultiRowValues:   true,
			MaximumBatchSize: 2,
		},
	)
	if err != nil {
		t.Fatalf("write INSERT export: %v", err)
	}
	if stats.Rows != 3 {
		t.Fatalf("rows = %d, want 3", stats.Rows)
	}
	sql := output.String()
	if strings.Count(sql, "INSERT INTO [dbo].[people]") != 2 ||
		!strings.Contains(sql, "BEGIN TRANSACTION;") ||
		!strings.Contains(sql, "COMMIT;") {
		t.Fatalf("export SQL = %q", sql)
	}
}

func TestSQLNumericLiteralRejectsNonFiniteValues(t *testing.T) {
	if value, ok, err := SQLNumericLiteral(int64(42)); !ok ||
		err != nil ||
		value != "42" {
		t.Fatalf("integer literal = %q, ok=%t, err=%v", value, ok, err)
	}
	if _, ok, err := SQLNumericLiteral(math.NaN()); !ok || err == nil {
		t.Fatal("NaN was accepted as a SQL numeric literal")
	}
	if _, ok, err := SQLNumericLiteral(json.Number("NaN")); !ok || err == nil {
		t.Fatal("JSON NaN was accepted as a SQL numeric literal")
	}
}
