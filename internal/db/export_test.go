package db

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rollingthunder/pkg/database"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func csvExportOptions() database.ExportOptions {
	return database.ExportOptions{
		Format: database.ExportFormatCSV,
		CSV: database.CSVOptions{
			Delimiter:     ",",
			IncludeHeader: true,
			NullValue:     "NULL",
		},
	}
}

func TestExportQueryResultsWritesChosenCSVFile(t *testing.T) {
	service := NewService()
	service.Start(context.Background())
	targetWithoutExtension := filepath.Join(t.TempDir(), "query-results")
	service.saveDialog = func(
		context.Context,
		wailsruntime.SaveDialogOptions,
	) (string, error) {
		return targetWithoutExtension, nil
	}

	response := service.ExportQueryResults(database.RowsExportRequest{
		Columns: []string{"id", "name"},
		Rows: []map[string]interface{}{
			{"id": 1, "name": "Alpha"},
			{"id": 2, "name": nil},
		},
		SuggestedName: "query-results.csv",
		Options:       csvExportOptions(),
	})
	if len(response.Errors) != 0 {
		t.Fatalf("export query results returned errors: %+v", response.Errors)
	}
	if response.Data.Cancelled {
		t.Fatal("export was unexpectedly cancelled")
	}
	if response.Data.Path != targetWithoutExtension+".csv" {
		t.Fatalf("path = %q, want %q", response.Data.Path, targetWithoutExtension+".csv")
	}
	if response.Data.Rows != 2 {
		t.Fatalf("row count = %d, want 2", response.Data.Rows)
	}
	if response.Data.Bytes <= 0 {
		t.Fatalf("byte count = %d, want a positive value", response.Data.Bytes)
	}

	content, err := os.ReadFile(response.Data.Path)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if string(content) != "id,name\n1,Alpha\n2,NULL\n" {
		t.Fatalf("unexpected export content: %q", content)
	}
}

func TestExportQueryResultsHandlesDialogCancellation(t *testing.T) {
	service := NewService()
	service.Start(context.Background())
	service.saveDialog = func(
		context.Context,
		wailsruntime.SaveDialogOptions,
	) (string, error) {
		return "", nil
	}

	response := service.ExportQueryResults(database.RowsExportRequest{
		Columns: []string{"id"},
		Rows:    []map[string]interface{}{{"id": 1}},
		Options: csvExportOptions(),
	})
	if len(response.Errors) != 0 {
		t.Fatalf("cancelled export returned errors: %+v", response.Errors)
	}
	if !response.Data.Cancelled {
		t.Fatal("expected cancelled export result")
	}
}

func TestExportTableDataUsesOwningConnection(t *testing.T) {
	alpha := &routingTestDriver{
		name:          "alpha",
		exportContent: "source\nalpha\n",
		exportRows:    1,
	}
	bravo := &routingTestDriver{
		name:          "bravo",
		exportContent: "source\nbravo\n",
		exportRows:    1,
	}
	service := newRoutingTestService(
		map[string]*routingTestDriver{
			"alpha": alpha,
			"bravo": bravo,
		},
		"bravo",
	)
	service.Start(context.Background())
	target := filepath.Join(t.TempDir(), "orders.csv")
	service.saveDialog = func(
		context.Context,
		wailsruntime.SaveDialogOptions,
	) (string, error) {
		return target, nil
	}

	response := service.ExportTableData("alpha", database.TableExportRequest{
		Table: database.Table{
			Schema: "public",
			Name:   "orders",
			Limit:  100,
		},
		Scope:   database.ExportScopePage,
		Options: csvExportOptions(),
	})
	if len(response.Errors) != 0 {
		t.Fatalf("export table returned errors: %+v", response.Errors)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read table export: %v", err)
	}
	if string(content) != "source\nalpha\n" {
		t.Fatalf("export used the wrong connection: %q", content)
	}
	if alpha.exportCallCount() != 1 || bravo.exportCallCount() != 0 {
		t.Fatalf(
			"export calls alpha=%d bravo=%d, want alpha=1 bravo=0",
			alpha.exportCallCount(),
			bravo.exportCallCount(),
		)
	}
}

func TestFailedExportPreservesExistingDestination(t *testing.T) {
	driver := &routingTestDriver{
		name:          "alpha",
		exportContent: "partial",
		exportErr:     errors.New("database stream failed"),
	}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": driver},
		"alpha",
	)
	service.Start(context.Background())
	target := filepath.Join(t.TempDir(), "orders.csv")
	if err := os.WriteFile(target, []byte("previous export"), 0o600); err != nil {
		t.Fatalf("write existing export: %v", err)
	}
	service.saveDialog = func(
		context.Context,
		wailsruntime.SaveDialogOptions,
	) (string, error) {
		return target, nil
	}

	response := service.ExportTableData("alpha", database.TableExportRequest{
		Table:   database.Table{Schema: "public", Name: "orders", Limit: 100},
		Scope:   database.ExportScopePage,
		Options: csvExportOptions(),
	})
	if len(response.Errors) != 1 ||
		!strings.Contains(response.Errors[0].Detail, "database stream failed") {
		t.Fatalf("expected stream error, got %+v", response.Errors)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read preserved export: %v", err)
	}
	if string(content) != "previous export" {
		t.Fatalf("existing export was overwritten after failure: %q", content)
	}
}
