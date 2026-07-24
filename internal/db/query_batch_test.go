package db

import (
	"context"
	"path/filepath"
	"testing"

	"rollingthunder/pkg/database"
)

func TestSQLiteQueryBatchVariablesAndExplain(t *testing.T) {
	service := NewService()
	service.Start(context.Background())
	connected := service.Connect(ConnectRequest{
		Driver: "sqlite",
		Config: database.Config{
			Name:   "batch-test",
			Driver: "sqlite",
			Db:     filepath.Join(t.TempDir(), "batch.sqlite3"),
		},
	})
	if len(connected.Errors) > 0 {
		t.Fatalf("Connect() errors = %+v", connected.Errors)
	}
	connectionID := connected.Data.ConnectionID
	t.Cleanup(func() {
		_ = service.DisconnectConnection(connectionID)
	})

	result := service.ExecuteQuery(database.QueryRequest{
		ConnectionID: connectionID,
		Query: `
			CREATE TABLE main.events (id INTEGER PRIMARY KEY, name TEXT);
			INSERT INTO main.events (id, name) VALUES ({{id}}, {{name}});
			SELECT id, name FROM main.events;
		`,
		Variables: []database.QueryVariable{
			{Name: "id", Value: 7, Type: "number"},
			{Name: "name", Value: "storm", Type: "text"},
		},
	})
	if len(result.Errors) > 0 {
		t.Fatalf("ExecuteQuery() errors = %+v", result.Errors)
	}
	if result.Data.StatementCount != 3 || len(result.Data.ResultSets) != 3 {
		t.Fatalf("batch result = %+v", result.Data)
	}
	rows := result.Data.ResultSets[2].Rows
	if len(rows) != 1 || rows[0]["name"] != "storm" {
		t.Fatalf("select rows = %+v", rows)
	}
	if len(result.Data.ResultSets[2].Columns) != 2 {
		t.Fatalf("select columns = %+v", result.Data.ResultSets[2].Columns)
	}

	plan := service.ExplainQuery(database.QueryRequest{
		ConnectionID: connectionID,
		Query:        "SELECT * FROM main.events WHERE id = {{id}}",
		Variables: []database.QueryVariable{
			{Name: "id", Value: 7, Type: "number"},
		},
	})
	if len(plan.Errors) > 0 {
		t.Fatalf("ExplainQuery() errors = %+v", plan.Errors)
	}
	if plan.Data.Engine != "SQLite" || len(plan.Data.Roots) == 0 {
		t.Fatalf("explain plan = %+v", plan.Data)
	}
}
