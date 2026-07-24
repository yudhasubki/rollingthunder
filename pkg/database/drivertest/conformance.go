package drivertest

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"rollingthunder/pkg/database"
)

type LiveConfig struct {
	Driver      database.Driver
	Schema      string
	IntegerType string
	TextType    string
}

func RunCapabilityContract(
	t *testing.T,
	driver database.CapabilityDriver,
	expectedEngine string,
) {
	t.Helper()
	capabilities := driver.Capabilities()
	if err := capabilities.Validate(); err != nil {
		t.Fatalf("Capabilities().Validate() error = %v", err)
	}
	if capabilities.Engine != expectedEngine {
		t.Fatalf(
			"Capabilities().Engine = %q, want %q",
			capabilities.Engine,
			expectedEngine,
		)
	}
	quoted := driver.QuoteIdentifier(`odd"name`)
	if quoted == "" || quoted == `odd"name` {
		t.Fatalf("QuoteIdentifier() did not quote identifier: %q", quoted)
	}
	if driver.Placeholder(1) == "" || driver.Placeholder(2) == "" {
		t.Fatal("Placeholder() returned an empty placeholder")
	}
	pagination, err := driver.PaginationClause(25, 50)
	if err != nil {
		t.Fatalf("PaginationClause() error = %v", err)
	}
	if !strings.Contains(pagination, "25") ||
		!strings.Contains(pagination, "50") {
		t.Fatalf("PaginationClause() = %q, want limit and offset", pagination)
	}
	if _, err := driver.PaginationClause(-1, 0); err == nil {
		t.Fatal("PaginationClause() accepted a negative limit")
	}
	if _, err := driver.PaginationClause(1, -1); err == nil {
		t.Fatal("PaginationClause() accepted a negative offset")
	}
}

func RunLiveContract(t *testing.T, config LiveConfig) {
	t.Helper()
	ctx := context.Background()
	driver := config.Driver
	schema := config.Schema
	tableName := "rt_conformance_rows"
	table := database.Table{Schema: schema, Name: tableName}
	_ = driver.DropTable(table)
	t.Cleanup(func() {
		_ = driver.DropTable(table)
	})

	if err := driver.CreateTable(table, []database.ColumnDefinition{
		{
			Name:       "id",
			Type:       config.IntegerType,
			Nullable:   false,
			PrimaryKey: true,
		},
		{
			Name:     "name",
			Type:     config.TextType,
			Nullable: false,
		},
		{
			Name:     "score",
			Type:     config.IntegerType,
			Nullable: true,
		},
	}); err != nil {
		t.Fatalf("CreateTable() error = %v", err)
	}

	collections, err := driver.GetCollections(schema)
	if err != nil {
		t.Fatalf("GetCollections() error = %v", err)
	}
	if !contains(collections, tableName) {
		t.Fatalf("GetCollections() = %v, missing %q", collections, tableName)
	}
	structures, err := driver.GetCollectionStructures(table)
	if err != nil {
		t.Fatalf("GetCollectionStructures() error = %v", err)
	}
	if len(structures) != 3 || !structures[0].IsPrimary {
		t.Fatalf("GetCollectionStructures() = %+v, want three columns and PK", structures)
	}

	for _, row := range []map[string]interface{}{
		{"id": 1, "name": "alpha", "score": 10},
		{"id": 2, "name": "beta", "score": 20},
	} {
		if err := driver.InsertRow(table, row); err != nil {
			t.Fatalf("InsertRow() error = %v", err)
		}
	}
	count, err := driver.CountCollectionData(table)
	if err != nil || count != 2 {
		t.Fatalf("CountCollectionData() = %d, %v, want 2", count, err)
	}

	page := table
	page.Limit = 1
	page.Filters = []database.Filter{{
		Column: "name", Operator: database.FilterContains, Value: "a",
	}}
	page.Sorts = []database.Sort{{
		Column: "score", Direction: database.SortDescending, Nulls: database.NullsLast,
	}}
	pageStructures, rows, err := driver.GetCollectionData(page)
	if err != nil {
		t.Fatalf("GetCollectionData() error = %v", err)
	}
	if len(pageStructures) != 3 || len(rows) != 1 ||
		fmt.Sprint(rows[0]["name"]) != "beta" {
		t.Fatalf("GetCollectionData() structures=%d rows=%v", len(pageStructures), rows)
	}

	changeDriver, ok := driver.(database.TableChangeDriver)
	if !ok {
		t.Fatal("driver does not implement TableChangeDriver")
	}
	result, err := changeDriver.ApplyTableChanges(ctx, database.TableChangeSet{
		Table: table,
		Added: []map[string]interface{}{
			{"id": 3, "name": "gamma", "score": 30},
		},
		Updated: []database.RowUpdate{{
			Original:       map[string]interface{}{"id": 1, "name": "alpha", "score": 10},
			Values:         map[string]interface{}{"id": 1, "name": "alpha-updated", "score": 11},
			ChangedColumns: []string{"name", "score"},
		}},
		Deleted: []map[string]interface{}{
			{"id": 2, "name": "beta", "score": 20},
		},
	})
	if err != nil {
		t.Fatalf("ApplyTableChanges() error = %v", err)
	}
	if result.Inserted != 1 || result.Updated != 1 || result.Deleted != 1 {
		t.Fatalf("ApplyTableChanges() result = %+v", result)
	}

	transactional, ok := driver.(database.TransactionalDriver)
	if !ok {
		t.Fatal("driver does not implement TransactionalDriver")
	}
	transaction, err := transactional.BeginTransaction(ctx)
	if err != nil {
		t.Fatalf("BeginTransaction() error = %v", err)
	}
	_, err = transaction.ExecuteQuery(
		ctx,
		fmt.Sprintf(
			"INSERT INTO %s (%s, %s, %s) VALUES (99, 'rollback', 99)",
			qualified(driver, schema, tableName),
			driver.QuoteIdentifier("id"),
			driver.QuoteIdentifier("name"),
			driver.QuoteIdentifier("score"),
		),
		database.QueryOptions{MaxRows: 10},
	)
	if err != nil {
		_ = transaction.Rollback()
		t.Fatalf("transaction ExecuteQuery() error = %v", err)
	}
	if err := transaction.Rollback(); err != nil {
		t.Fatalf("transaction Rollback() error = %v", err)
	}
	count, err = driver.CountCollectionData(table)
	if err != nil || count != 2 {
		t.Fatalf("count after rollback = %d, %v, want 2", count, err)
	}

	objectDriver, ok := driver.(database.ObjectDriver)
	if !ok {
		t.Fatal("driver does not implement ObjectDriver")
	}
	objects, err := objectDriver.ListObjects(ctx, database.ObjectFilter{
		Schema: schema,
		Kinds:  []database.ObjectKind{database.ObjectKindTable},
		Search: tableName,
	})
	if err != nil {
		t.Fatalf("ListObjects() error = %v", err)
	}
	var reference database.ObjectReference
	for _, object := range objects {
		if object.Reference.Name == tableName {
			reference = object.Reference
			break
		}
	}
	if reference.Name == "" {
		t.Fatalf("ListObjects() = %+v, missing conformance table", objects)
	}
	detail, err := objectDriver.GetObjectDetail(ctx, reference)
	if err != nil {
		t.Fatalf("GetObjectDetail() error = %v", err)
	}
	if detail.Definition == "" || len(detail.Columns) != 3 {
		t.Fatalf("GetObjectDetail() = %+v", detail)
	}

	var exported bytes.Buffer
	_, err = driver.ExportTable(ctx, database.TableExportRequest{
		Table: database.Table{
			Schema: schema,
			Name:   tableName,
			Limit:  100,
		},
		Scope: database.ExportScopeAll,
		Options: database.ExportOptions{
			Format: database.ExportFormatCSV,
			CSV: database.CSVOptions{
				IncludeHeader: true,
				Encoding:      database.CSVEncodingUTF8,
			},
		},
	}, &exported)
	if err != nil {
		t.Fatalf("ExportTable() error = %v", err)
	}
	if !strings.Contains(exported.String(), "alpha-updated") ||
		!strings.Contains(exported.String(), "gamma") {
		t.Fatalf("ExportTable() output = %q", exported.String())
	}
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func qualified(
	driver database.CapabilityDriver,
	schema string,
	name string,
) string {
	if schema == "" {
		return driver.QuoteIdentifier(name)
	}
	return driver.QuoteIdentifier(schema) + "." + driver.QuoteIdentifier(name)
}
