package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rollingthunder/pkg/database"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestSQLiteEndToEndConnectionEditExportAndDestructiveActions(t *testing.T) {
	service := NewService()
	service.Start(context.Background())
	connected := service.Connect(ConnectRequest{
		Driver: "sqlite",
		Config: database.Config{
			Name:   "E2E SQLite",
			Driver: "sqlite",
			Db:     filepath.Join(t.TempDir(), "e2e.sqlite3"),
		},
		ProfileID: "e2e-profile",
	})
	if len(connected.Errors) > 0 || !connected.Data.Connected {
		t.Fatalf("Connect() = %+v", connected)
	}
	connectionID := connected.Data.ConnectionID
	t.Cleanup(func() {
		_ = service.DisconnectConnection(connectionID)
	})
	active := service.GetActiveConnections()
	if len(active.Data) != 1 || active.Data[0].ProfileID != "e2e-profile" {
		t.Fatalf("active connections = %+v", active.Data)
	}

	table := database.Table{Schema: "main", Name: "accounts"}
	created := service.CreateTable(connectionID, table, []database.ColumnDefinition{
		{
			Name:       "id",
			Type:       "INTEGER",
			PrimaryKey: true,
			Nullable:   false,
		},
		{
			Name:     "name",
			Type:     "TEXT",
			Nullable: false,
		},
	})
	if len(created.Errors) > 0 || !created.Data {
		t.Fatalf("CreateTable() = %+v", created)
	}
	inserted := service.ApplyTableChanges(connectionID, database.TableChangeSet{
		Table: table,
		Added: []map[string]interface{}{
			{"id": 1, "name": "Ada"},
			{"id": 2, "name": "Linus"},
		},
	})
	if len(inserted.Errors) > 0 || inserted.Data.Inserted != 2 {
		t.Fatalf("insert changes = %+v", inserted)
	}
	updated := service.ApplyTableChanges(connectionID, database.TableChangeSet{
		Table: table,
		Updated: []database.RowUpdate{
			{
				Original:       map[string]interface{}{"id": 2, "name": "Linus"},
				Values:         map[string]interface{}{"id": 2, "name": "Grace"},
				ChangedColumns: []string{"name"},
			},
		},
	})
	if len(updated.Errors) > 0 || updated.Data.Updated != 1 {
		t.Fatalf("update changes = %+v", updated)
	}

	exportPath := filepath.Join(t.TempDir(), "accounts")
	service.saveDialog = func(
		context.Context,
		wailsruntime.SaveDialogOptions,
	) (string, error) {
		return exportPath, nil
	}
	exported := service.ExportTableData(connectionID, database.TableExportRequest{
		Table:         table,
		Scope:         database.ExportScopeAll,
		ExpectedRows:  2,
		SuggestedName: "accounts.csv",
		Options: database.ExportOptions{
			Format: database.ExportFormatCSV,
			CSV: database.CSVOptions{
				Delimiter:     ",",
				IncludeHeader: true,
				Encoding:      database.CSVEncodingUTF8,
			},
		},
	})
	if len(exported.Errors) > 0 || exported.Data.Rows != 2 {
		t.Fatalf("ExportTableData() = %+v", exported)
	}
	content, err := os.ReadFile(exported.Data.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "Grace") {
		t.Fatalf("export content = %q", content)
	}

	deleted := service.ApplyTableChanges(connectionID, database.TableChangeSet{
		Table:   table,
		Deleted: []map[string]interface{}{{"id": 1, "name": "Ada"}},
	})
	if len(deleted.Errors) > 0 || deleted.Data.Deleted != 1 {
		t.Fatalf("delete changes = %+v", deleted)
	}
	truncated := service.TruncateTable(connectionID, table)
	if len(truncated.Errors) > 0 || !truncated.Data {
		t.Fatalf("TruncateTable() = %+v", truncated)
	}
	if count := service.CountCollectionData(connectionID, table); len(count.Errors) > 0 ||
		count.Data != 0 {
		t.Fatalf("count after truncate = %+v", count)
	}
	dropped := service.DropTable(connectionID, table)
	if len(dropped.Errors) > 0 || !dropped.Data {
		t.Fatalf("DropTable() = %+v", dropped)
	}
	tables := service.GetCollections(connectionID, []string{"main"})
	if len(tables.Errors) > 0 {
		t.Fatalf("GetCollections() = %+v", tables)
	}
	for _, name := range tables.Data {
		if name == table.Name {
			t.Fatalf("dropped table still listed: %+v", tables.Data)
		}
	}
}
