package db

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"

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

func jsonExportOptions(pretty bool) database.ExportOptions {
	return database.ExportOptions{
		Format: database.ExportFormatJSON,
		JSON: database.JSONOptions{
			Pretty: pretty,
		},
	}
}

func sqlExportOptions(batchSize int, includeTransaction bool) database.ExportOptions {
	return database.ExportOptions{
		Format: database.ExportFormatSQL,
		SQL: database.SQLInsertOptions{
			BatchSize:          batchSize,
			IncludeTransaction: includeTransaction,
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

func TestExportCanBeCancelledWhileChoosingDestination(t *testing.T) {
	service := NewService()
	service.Start(context.Background())
	dialogStarted := make(chan struct{})
	service.saveDialog = func(
		ctx context.Context,
		_ wailsruntime.SaveDialogOptions,
	) (string, error) {
		close(dialogStarted)
		<-ctx.Done()
		return "", ctx.Err()
	}

	outcome := make(chan response.BaseResponse[database.ExportResult], 1)
	go func() {
		outcome <- service.ExportQueryResults(database.RowsExportRequest{
			Columns:      []string{"id"},
			Rows:         []map[string]interface{}{{"id": 1}},
			JobID:        "cancel-preparing-export",
			ExpectedRows: 1,
			Options:      csvExportOptions(),
		})
	}()

	select {
	case <-dialogStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("save dialog did not open")
	}

	progress := service.GetExportProgress("cancel-preparing-export")
	if len(progress.Errors) != 0 || progress.Data.Status != exportStatusPreparing {
		t.Fatalf("unexpected preparing progress: %+v", progress)
	}
	if cancelled := service.CancelExport("cancel-preparing-export"); len(cancelled.Errors) != 0 {
		t.Fatalf("cancel preparing export: %+v", cancelled.Errors)
	}

	select {
	case result := <-outcome:
		if len(result.Errors) != 0 || !result.Data.Cancelled {
			t.Fatalf("cancelled preparing export result: %+v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("preparing export did not stop")
	}
}

func TestExportQueryResultsWritesFormatAwareJSONFile(t *testing.T) {
	service := NewService()
	service.Start(context.Background())
	targetWithoutExtension := filepath.Join(t.TempDir(), "query-results")
	var dialogOptions wailsruntime.SaveDialogOptions
	service.saveDialog = func(
		_ context.Context,
		options wailsruntime.SaveDialogOptions,
	) (string, error) {
		dialogOptions = options
		return targetWithoutExtension, nil
	}

	response := service.ExportQueryResults(database.RowsExportRequest{
		Columns: []string{"id", "metadata"},
		Rows: []map[string]interface{}{
			{"id": 1, "metadata": map[string]interface{}{"active": true}},
		},
		SuggestedName: "query-results.csv",
		Options:       jsonExportOptions(true),
	})
	if len(response.Errors) != 0 {
		t.Fatalf("export JSON returned errors: %+v", response.Errors)
	}
	if response.Data.Path != targetWithoutExtension+".json" {
		t.Fatalf("path = %q, want JSON extension", response.Data.Path)
	}
	if response.Data.Format != database.ExportFormatJSON {
		t.Fatalf("format = %q, want JSON", response.Data.Format)
	}
	if dialogOptions.Title != "Export JSON" {
		t.Fatalf("dialog title = %q", dialogOptions.Title)
	}
	if dialogOptions.DefaultFilename != "query-results.json" {
		t.Fatalf("default filename = %q", dialogOptions.DefaultFilename)
	}
	if len(dialogOptions.Filters) != 1 || dialogOptions.Filters[0].Pattern != "*.json" {
		t.Fatalf("dialog filters = %+v", dialogOptions.Filters)
	}

	content, err := os.ReadFile(response.Data.Path)
	if err != nil {
		t.Fatalf("read JSON export: %v", err)
	}
	if !json.Valid(content) {
		t.Fatalf("invalid JSON export: %s", content)
	}
	if !strings.Contains(string(content), "\n    \"metadata\"") {
		t.Fatalf("expected pretty JSON output: %s", content)
	}
}

func TestEnsureExportExtensionMatchesSelectedFormat(t *testing.T) {
	tests := map[string]struct {
		path   string
		format database.ExportFormat
		want   string
	}{
		"append csv":  {path: "orders", format: database.ExportFormatCSV, want: "orders.csv"},
		"keep csv":    {path: "orders.CSV", format: database.ExportFormatCSV, want: "orders.CSV"},
		"replace csv": {path: "orders.csv", format: database.ExportFormatJSON, want: "orders.json"},
		"replace json with sql": {
			path:   "orders.json",
			format: database.ExportFormatSQL,
			want:   "orders.sql",
		},
		"replace text": {
			path:   "orders.txt",
			format: database.ExportFormatJSON,
			want:   "orders.json",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := ensureExportExtension(test.path, test.format); got != test.want {
				t.Fatalf("ensureExportExtension(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}
}

func TestExportTableDataWritesFormatAwareSQLFile(t *testing.T) {
	driver := &routingTestDriver{
		name:          "alpha",
		exportContent: "INSERT INTO \"public\".\"orders\" (\"id\") VALUES\n  (1);\n",
		exportRows:    1,
	}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": driver},
		"alpha",
	)
	service.Start(context.Background())
	targetWithoutExtension := filepath.Join(t.TempDir(), "orders")
	var dialogOptions wailsruntime.SaveDialogOptions
	service.saveDialog = func(
		_ context.Context,
		options wailsruntime.SaveDialogOptions,
	) (string, error) {
		dialogOptions = options
		return targetWithoutExtension, nil
	}

	response := service.ExportTableData("alpha", database.TableExportRequest{
		Table:         database.Table{Schema: "public", Name: "orders", Limit: 100},
		Scope:         database.ExportScopeAll,
		SuggestedName: "orders.json",
		Options:       sqlExportOptions(500, false),
	})
	if len(response.Errors) != 0 {
		t.Fatalf("export SQL returned errors: %+v", response.Errors)
	}
	if response.Data.Path != targetWithoutExtension+".sql" {
		t.Fatalf("path = %q, want SQL extension", response.Data.Path)
	}
	if response.Data.Format != database.ExportFormatSQL {
		t.Fatalf("format = %q, want SQL", response.Data.Format)
	}
	if dialogOptions.Title != "Export SQL" {
		t.Fatalf("dialog title = %q", dialogOptions.Title)
	}
	if dialogOptions.DefaultFilename != "orders.sql" {
		t.Fatalf("default filename = %q", dialogOptions.DefaultFilename)
	}
	if len(dialogOptions.Filters) != 1 || dialogOptions.Filters[0].Pattern != "*.sql" {
		t.Fatalf("dialog filters = %+v", dialogOptions.Filters)
	}

	content, err := os.ReadFile(response.Data.Path)
	if err != nil {
		t.Fatalf("read SQL export: %v", err)
	}
	if string(content) != driver.exportContent {
		t.Fatalf("unexpected SQL content: %q", content)
	}

	driver.mu.Lock()
	request := driver.exportRequest
	driver.mu.Unlock()
	if request.Options.Format != database.ExportFormatSQL ||
		request.Options.SQL.BatchSize != 500 ||
		request.Options.SQL.IncludeTransaction {
		t.Fatalf("driver export request = %+v", request.Options)
	}
}

func TestExportQueryResultsRejectsSQLBeforeOpeningSaveDialog(t *testing.T) {
	service := NewService()
	service.Start(context.Background())
	dialogOpened := false
	service.saveDialog = func(
		context.Context,
		wailsruntime.SaveDialogOptions,
	) (string, error) {
		dialogOpened = true
		return filepath.Join(t.TempDir(), "query.sql"), nil
	}

	response := service.ExportQueryResults(database.RowsExportRequest{
		Columns: []string{"id"},
		Rows:    []map[string]interface{}{{"id": 1}},
		Options: sqlExportOptions(100, true),
	})
	if len(response.Errors) != 1 ||
		!strings.Contains(response.Errors[0].Detail, "table source") {
		t.Fatalf("expected table-source error, got %+v", response.Errors)
	}
	if dialogOpened {
		t.Fatal("save dialog opened for unsupported query-result SQL export")
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

func TestRunningExportReportsProgressAndCanBeCancelled(t *testing.T) {
	driver := &routingTestDriver{
		name:          "alpha",
		exportContent: strings.Repeat("partial export\n", 32),
		exportRows:    25,
		exportStarted: make(chan struct{}),
		exportRelease: make(chan struct{}),
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

	type exportOutcome struct {
		cancelled bool
		errors    int
	}
	outcome := make(chan exportOutcome, 1)
	go func() {
		response := service.ExportTableData("alpha", database.TableExportRequest{
			Table:         database.Table{Schema: "public", Name: "orders", Limit: 100},
			Scope:         database.ExportScopeAll,
			JobID:         "cancel-export-test",
			ExpectedRows:  100,
			SuggestedName: "orders.csv",
			Options:       csvExportOptions(),
		})
		outcome <- exportOutcome{
			cancelled: response.Data.Cancelled,
			errors:    len(response.Errors),
		}
	}()

	select {
	case <-driver.exportStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("export did not start")
	}

	progress := service.GetExportProgress("cancel-export-test")
	if len(progress.Errors) != 0 {
		t.Fatalf("get export progress returned errors: %+v", progress.Errors)
	}
	if progress.Data.Status != exportStatusRunning ||
		progress.Data.Rows != 25 ||
		progress.Data.Bytes <= 0 ||
		progress.Data.TotalRows != 100 ||
		!progress.Data.Cancellable {
		t.Fatalf("unexpected export progress: %+v", progress.Data)
	}

	cancelled := service.CancelExport("cancel-export-test")
	if len(cancelled.Errors) != 0 || !cancelled.Data {
		t.Fatalf("cancel export response: %+v", cancelled)
	}

	select {
	case result := <-outcome:
		if result.errors != 0 || !result.cancelled {
			t.Fatalf("cancelled export outcome: %+v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled export did not finish")
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read destination after cancellation: %v", err)
	}
	if string(content) != "previous export" {
		t.Fatalf("cancelled export replaced destination: %q", content)
	}
	if response := service.GetExportProgress("cancel-export-test"); len(response.Errors) != 1 {
		t.Fatalf("completed export job was not cleaned up: %+v", response)
	}
}

func TestReplaceExportFileRejectsDirectoryDestination(t *testing.T) {
	root := t.TempDir()
	tempPath := filepath.Join(root, "export.tmp")
	if err := os.WriteFile(tempPath, []byte("new export"), 0o600); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(root, "destination.csv")
	if err := os.Mkdir(targetPath, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := replaceExportFile(tempPath, targetPath); err == nil {
		t.Fatal("replaceExportFile() unexpectedly replaced a directory")
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("directory destination was modified")
	}
}
