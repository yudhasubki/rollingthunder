package db

import (
	"context"
	"fmt"
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

func TestExplainRejectsTransactionControlStatements(t *testing.T) {
	service := NewService()
	for _, query := range []string{
		"BEGIN",
		"START TRANSACTION",
		"COMMIT",
		"ROLLBACK",
		"SAVEPOINT before_update",
		"RELEASE SAVEPOINT before_update",
		"ABORT",
		"END",
	} {
		result := service.ExplainQuery(database.QueryRequest{Query: query})
		if len(result.Errors) != 1 {
			t.Fatalf("ExplainQuery(%q) errors = %+v", query, result.Errors)
		}
		if result.Errors[0].Code != errorCodeInvalidRequest {
			t.Fatalf(
				"ExplainQuery(%q) code = %q, want %q",
				query,
				result.Errors[0].Code,
				errorCodeInvalidRequest,
			)
		}
	}
}

func TestSQLiteRollbackRemovesWritesAndExplainRemainsAvailable(t *testing.T) {
	service := NewService()
	service.Start(context.Background())
	connected := service.Connect(ConnectRequest{
		Driver: "sqlite",
		Config: database.Config{
			Name:   "rollback-test",
			Driver: "sqlite",
			Db:     filepath.Join(t.TempDir(), "rollback.sqlite3"),
		},
	})
	if len(connected.Errors) > 0 {
		t.Fatalf("Connect() errors = %+v", connected.Errors)
	}
	connectionID := connected.Data.ConnectionID
	t.Cleanup(func() {
		_ = service.DisconnectConnection(connectionID)
	})

	created := service.ExecuteQuery(database.QueryRequest{
		ConnectionID: connectionID,
		Query:        "CREATE TABLE main.events (id INTEGER PRIMARY KEY, name TEXT)",
	})
	if len(created.Errors) > 0 {
		t.Fatalf("create table errors = %+v", created.Errors)
	}

	const transactionID = "rollback-and-explain"
	begin := service.BeginTransaction(connectionID, transactionID)
	if len(begin.Errors) > 0 {
		t.Fatalf("BeginTransaction() errors = %+v", begin.Errors)
	}
	inserted := service.ExecuteQuery(database.QueryRequest{
		ConnectionID:  connectionID,
		TransactionID: transactionID,
		Query:         "INSERT INTO main.events (id, name) VALUES (1, 'temporary')",
	})
	if len(inserted.Errors) > 0 {
		t.Fatalf("transaction insert errors = %+v", inserted.Errors)
	}
	rolledBack := service.RollbackTransaction(transactionID)
	if len(rolledBack.Errors) > 0 || rolledBack.Data.State != "rolled_back" {
		t.Fatalf("RollbackTransaction() = %+v", rolledBack)
	}

	count := service.ExecuteQuery(database.QueryRequest{
		ConnectionID: connectionID,
		Query:        "SELECT COUNT(*) AS total FROM main.events",
	})
	if len(count.Errors) > 0 {
		t.Fatalf("count query errors = %+v", count.Errors)
	}
	if len(count.Data.Rows) != 1 || fmt.Sprint(count.Data.Rows[0]["total"]) != "0" {
		t.Fatalf("rows after rollback = %+v", count.Data.Rows)
	}

	plan := service.ExplainQuery(database.QueryRequest{
		ConnectionID: connectionID,
		Query:        "SELECT * FROM main.events WHERE id = 1",
	})
	if len(plan.Errors) > 0 || len(plan.Data.Roots) == 0 {
		t.Fatalf("ExplainQuery() after rollback = %+v", plan)
	}
}
