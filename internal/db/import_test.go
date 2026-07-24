package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"rollingthunder/pkg/database"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func newSQLiteImportService(t *testing.T) (*Service, string) {
	t.Helper()
	service := NewService()
	service.Start(context.Background())
	connected := service.Connect(ConnectRequest{
		Driver: "sqlite",
		Config: database.Config{
			Name:   "import-test",
			Driver: "sqlite",
			Db:     filepath.Join(t.TempDir(), "import.sqlite3"),
		},
	})
	if len(connected.Errors) > 0 {
		t.Fatalf("Connect() errors = %+v", connected.Errors)
	}
	t.Cleanup(func() {
		_ = service.DisconnectConnection(connected.Data.ConnectionID)
	})
	return service, connected.Data.ConnectionID
}

func selectImportTestFile(
	t *testing.T,
	service *Service,
	path string,
) database.ImportFileSelection {
	t.Helper()
	service.importOpenDialog = func(
		context.Context,
		wailsruntime.OpenDialogOptions,
	) (string, error) {
		return path, nil
	}
	selected := service.ChooseImportFile()
	if len(selected.Errors) > 0 {
		t.Fatalf("ChooseImportFile() errors = %+v", selected.Errors)
	}
	if selected.Data.Token == "" {
		t.Fatal("ChooseImportFile() returned no token")
	}
	return selected.Data
}

func TestCSVImportPreviewAndCreateTable(t *testing.T) {
	service, connectionID := newSQLiteImportService(t)
	source := filepath.Join(t.TempDir(), "people.csv")
	if err := os.WriteFile(
		source,
		[]byte("id,name,active\n1,Ada,true\n2,Linus,false\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	selected := selectImportTestFile(t, service, source)
	options := database.ImportOptions{
		Format:      "csv",
		Delimiter:   ",",
		Header:      true,
		EmptyAsNull: true,
	}
	preview := service.InspectImportFile(database.ImportPreviewRequest{
		Token:   selected.Token,
		Options: options,
	})
	if len(preview.Errors) > 0 {
		t.Fatalf("InspectImportFile() errors = %+v", preview.Errors)
	}
	if preview.Data.Sampled != 2 || len(preview.Data.Columns) != 3 {
		t.Fatalf("preview = %+v", preview.Data)
	}
	if preview.Data.Columns[0].InferredType != "integer" {
		t.Fatalf("id type = %q", preview.Data.Columns[0].InferredType)
	}

	imported := service.ImportData(database.ImportRequest{
		ConnectionID: connectionID,
		Token:        selected.Token,
		Options:      options,
		Schema:       "main",
		Table:        "people",
		CreateTable:  true,
		Columns:      preview.Data.Columns,
	})
	if len(imported.Errors) > 0 {
		t.Fatalf("ImportData() errors = %+v", imported.Errors)
	}
	if imported.Data.RowsInserted != 2 || !imported.Data.TableCreated {
		t.Fatalf("import result = %+v", imported.Data)
	}

	result := service.ExecuteQuery(database.QueryRequest{
		ConnectionID: connectionID,
		Query:        "SELECT id, name, active FROM main.people ORDER BY id",
	})
	if len(result.Errors) > 0 {
		t.Fatalf("select errors = %+v", result.Errors)
	}
	if len(result.Data.Rows) != 2 || result.Data.Rows[1]["name"] != "Linus" {
		t.Fatalf("rows = %+v", result.Data.Rows)
	}
}

func TestJSONLinesImportIntoExistingTable(t *testing.T) {
	service, connectionID := newSQLiteImportService(t)
	created := service.ExecuteQuery(database.QueryRequest{
		ConnectionID: connectionID,
		Query:        "CREATE TABLE main.events (id INTEGER NOT NULL, payload TEXT)",
	})
	if len(created.Errors) > 0 {
		t.Fatalf("create errors = %+v", created.Errors)
	}
	source := filepath.Join(t.TempDir(), "events.ndjson")
	if err := os.WriteFile(
		source,
		[]byte("{\"id\":1,\"payload\":{\"kind\":\"rain\"}}\n{\"id\":2,\"payload\":[\"wind\"]}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	selected := selectImportTestFile(t, service, source)
	options := database.ImportOptions{Format: "json", EmptyAsNull: true}
	preview := service.InspectImportFile(database.ImportPreviewRequest{
		Token:   selected.Token,
		Options: options,
	})
	if len(preview.Errors) > 0 {
		t.Fatalf("preview errors = %+v", preview.Errors)
	}
	imported := service.ImportData(database.ImportRequest{
		ConnectionID: connectionID,
		Token:        selected.Token,
		Options:      options,
		Schema:       "main",
		Table:        "events",
		Columns:      preview.Data.Columns,
	})
	if len(imported.Errors) > 0 {
		t.Fatalf("import errors = %+v", imported.Errors)
	}
	if imported.Data.RowsInserted != 2 {
		t.Fatalf("rows inserted = %d", imported.Data.RowsInserted)
	}
	result := service.ExecuteQuery(database.QueryRequest{
		ConnectionID: connectionID,
		Query:        "SELECT payload FROM main.events ORDER BY id",
	})
	if len(result.Errors) > 0 || len(result.Data.Rows) != 2 {
		t.Fatalf("select = %+v", result)
	}
	if result.Data.Rows[0]["payload"] != `{"kind":"rain"}` {
		t.Fatalf("normalized JSON payload = %#v", result.Data.Rows[0]["payload"])
	}
}

func TestImportRollsBackAllRowsWhenAValueFailsCoercion(t *testing.T) {
	service, connectionID := newSQLiteImportService(t)
	created := service.ExecuteQuery(database.QueryRequest{
		ConnectionID: connectionID,
		Query:        "CREATE TABLE main.numbers (value INTEGER NOT NULL)",
	})
	if len(created.Errors) > 0 {
		t.Fatalf("create errors = %+v", created.Errors)
	}
	source := filepath.Join(t.TempDir(), "numbers.csv")
	if err := os.WriteFile(source, []byte("value\n1\nnot-a-number\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	selected := selectImportTestFile(t, service, source)
	options := database.ImportOptions{Format: "csv", Delimiter: ",", Header: true}
	imported := service.ImportData(database.ImportRequest{
		ConnectionID: connectionID,
		Token:        selected.Token,
		Options:      options,
		Schema:       "main",
		Table:        "numbers",
		Columns: []database.ImportColumn{
			{
				SourceName:   "value",
				TargetName:   "value",
				InferredType: "integer",
				Included:     true,
			},
		},
	})
	if len(imported.Errors) == 0 {
		t.Fatal("ImportData() unexpectedly succeeded")
	}
	count := service.ExecuteQuery(database.QueryRequest{
		ConnectionID: connectionID,
		Query:        "SELECT COUNT(*) AS count FROM main.numbers",
	})
	if len(count.Errors) > 0 || count.Data.Rows[0]["count"] != int64(0) {
		t.Fatalf("rolled-back count = %+v", count)
	}
}
