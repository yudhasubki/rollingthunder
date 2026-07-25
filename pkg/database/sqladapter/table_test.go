package sqladapter

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"rollingthunder/pkg/database"
)

type captureExecutor struct {
	query string
	args  []interface{}
}

func (executor *captureExecutor) Exec(
	query string,
	args ...interface{},
) (sql.Result, error) {
	executor.query = query
	executor.args = append([]interface{}(nil), args...)
	return captureResult(1), nil
}

type captureResult int64

func (result captureResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result captureResult) RowsAffected() (int64, error) {
	return int64(result), nil
}

func testDialect() Dialect {
	return Dialect{
		QuoteIdentifier: func(value string) string {
			return "[" + value + "]"
		},
		QuoteQualified: func(schema, name string) string {
			return "[" + schema + "].[" + name + "]"
		},
		Placeholder: func(position int) string {
			return fmt.Sprintf("@p%d", position)
		},
	}
}

func TestInsertRowWithStructuresOmitsGeneratedColumns(t *testing.T) {
	executor := &captureExecutor{}
	err := InsertRowWithStructures(
		executor,
		database.Table{Schema: "dbo", Name: "events"},
		map[string]interface{}{
			"id":         nil,
			"name":       "storm",
			"normalized": "STORM",
		},
		database.Structures{
			{Name: "id", IsAutoInc: true},
			{Name: "name"},
			{Name: "normalized", IsGenerated: true},
		},
		testDialect(),
	)
	if err != nil {
		t.Fatalf("insert row: %v", err)
	}
	if executor.query !=
		"INSERT INTO [dbo].[events] ([name]) VALUES (@p1)" ||
		len(executor.args) != 1 ||
		executor.args[0] != "storm" {
		t.Fatalf(
			"query=%q args=%#v",
			executor.query,
			executor.args,
		)
	}
}

func TestInsertRowWithStructuresKeepsExplicitIdentityValue(t *testing.T) {
	executor := &captureExecutor{}
	err := InsertRowWithStructures(
		executor,
		database.Table{Schema: "dbo", Name: "events"},
		map[string]interface{}{
			"id":   int64(42),
			"name": "storm",
		},
		database.Structures{
			{
				Name:        "id",
				IsAutoInc:   true,
				IsGenerated: true,
				Generation:  "IDENTITY(1,1)",
			},
			{Name: "name"},
		},
		testDialect(),
	)
	if err != nil {
		t.Fatalf("insert row: %v", err)
	}
	if executor.query !=
		"INSERT INTO [dbo].[events] ([id], [name]) VALUES (@p1, @p2)" ||
		len(executor.args) != 2 ||
		executor.args[0] != int64(42) {
		t.Fatalf("query=%q args=%#v", executor.query, executor.args)
	}
	if !hasExplicitIdentityValues(
		[]map[string]interface{}{{"ID": int64(42)}},
		database.Structures{{Name: "id", IsAutoInc: true}},
	) {
		t.Fatal("explicit identity value was not detected")
	}
}

func TestInsertRowWithStructuresKeepsRegularNilID(t *testing.T) {
	executor := &captureExecutor{}
	err := InsertRowWithStructures(
		executor,
		database.Table{Schema: "dbo", Name: "events"},
		map[string]interface{}{
			"id":   nil,
			"name": "storm",
		},
		database.Structures{
			{Name: "ID"},
			{Name: "NAME"},
		},
		testDialect(),
	)
	if err != nil {
		t.Fatalf("insert row: %v", err)
	}
	if executor.query !=
		"INSERT INTO [dbo].[events] ([ID], [NAME]) VALUES (@p1, @p2)" ||
		len(executor.args) != 2 ||
		executor.args[0] != nil ||
		executor.args[1] != "storm" {
		t.Fatalf(
			"query=%q args=%#v",
			executor.query,
			executor.args,
		)
	}
}

func TestUpdateRowWithStructuresResolvesColumnCasing(t *testing.T) {
	executor := &captureExecutor{}
	err := UpdateRowWithStructures(
		executor,
		database.Table{Schema: "dbo", Name: "events"},
		map[string]interface{}{
			"id":   1,
			"name": "thunder",
		},
		"id",
		database.Structures{
			{Name: "ID", IsPrimary: true},
			{Name: "NAME"},
		},
		testDialect(),
	)
	if err != nil {
		t.Fatalf("update row: %v", err)
	}
	if executor.query !=
		"UPDATE [dbo].[events] SET [NAME] = @p1 WHERE [ID] = @p2" ||
		len(executor.args) != 2 ||
		executor.args[0] != "thunder" ||
		executor.args[1] != 1 {
		t.Fatalf(
			"query=%q args=%#v",
			executor.query,
			executor.args,
		)
	}
}

func TestUpdateRowWithStructuresRejectsUnknownColumns(t *testing.T) {
	executor := &captureExecutor{}
	err := UpdateRowWithStructures(
		executor,
		database.Table{Schema: "dbo", Name: "events"},
		map[string]interface{}{
			"id":      1,
			"missing": "value",
		},
		"id",
		database.Structures{
			{Name: "id", IsPrimary: true},
			{Name: "name"},
		},
		testDialect(),
	)
	if err == nil || !strings.Contains(err.Error(), "unknown update column") {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestSelectedRowsRequiresASelection(t *testing.T) {
	_, err := NewSelectedRows(
		&insertExportRows{},
		nil,
		100,
	)
	if err == nil {
		t.Fatal("empty selected-row export was accepted")
	}
}
