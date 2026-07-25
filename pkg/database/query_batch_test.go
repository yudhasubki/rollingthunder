package database

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type queryTestDialect struct {
	style PlaceholderStyle
}

func (dialect queryTestDialect) Capabilities() Capabilities {
	return Capabilities{
		Engine:      "test",
		DisplayName: "Test",
		Dialect: Dialect{
			Name:             "test",
			IdentifierOpen:   `"`,
			IdentifierClose:  `"`,
			PlaceholderStyle: dialect.style,
			PaginationStyle:  PaginationLimitOffset,
		},
	}
}

func (dialect queryTestDialect) QuoteIdentifier(identifier string) string {
	return `"` + identifier + `"`
}

func (dialect queryTestDialect) Placeholder(position int) string {
	if dialect.style == PlaceholderDollar {
		return "$" + strconv.Itoa(position)
	}
	return "?"
}

func (dialect queryTestDialect) PaginationClause(_, _ int) (string, error) {
	return "", nil
}

func TestSplitSQLStatementsPreservesQuotedBodies(t *testing.T) {
	query := `
		SELECT ';' AS separator;
		-- ignored ;
		SELECT $$body;still body$$;
		SELECT "semi;column" FROM [semi;table];
	`
	statements, err := SplitSQLStatements(query)
	if err != nil {
		t.Fatalf("SplitSQLStatements() error = %v", err)
	}
	if len(statements) != 3 {
		t.Fatalf("statements = %#v", statements)
	}
}

func TestSplitSQLStatementsKeepsRoutineDefinitionsIntact(t *testing.T) {
	query := `CREATE TRIGGER audit_insert AFTER INSERT ON users
		BEGIN
			INSERT INTO audit_log(message) VALUES ('created');
			UPDATE counters SET value = value + 1;
		END;`
	statements, err := SplitSQLStatements(query)
	if err != nil {
		t.Fatalf("SplitSQLStatements() error = %v", err)
	}
	if len(statements) != 1 || statements[0] == "" {
		t.Fatalf("statements = %#v", statements)
	}
}

func TestSplitSQLStatementsKeepsOracleAndTSQLBlocksIntact(t *testing.T) {
	for _, query := range []string{
		`BEGIN
			INSERT INTO audit_log(message) VALUES (q'[created; safely]');
			UPDATE counters SET value = value + 1;
		END;`,
		`DECLARE @value int = 1;
			SELECT @value;
			SET @value = @value + 1;`,
		`BEGIN TRY
			SELECT 1;
		END TRY
		BEGIN CATCH
			THROW;
		END CATCH;`,
		`CREATE OR REPLACE PACKAGE report_api AS
			PROCEDURE refresh_report;
			FUNCTION status RETURN VARCHAR2;
		END report_api;`,
	} {
		statements, err := SplitSQLStatements(query)
		if err != nil {
			t.Fatalf("SplitSQLStatements() error = %v", err)
		}
		if len(statements) != 1 || statements[0] == "" {
			t.Fatalf("block was split: %#v", statements)
		}
	}
}

func TestBindQueryVariablesUsesDriverPlaceholders(t *testing.T) {
	query := `
		SELECT {{tenant_id}}, '{{ignored}}'
		FROM members
		WHERE tenant_id = {{tenant_id}}
		  AND enabled = {{enabled}}
		-- {{also_ignored}}
	`
	bound, args, err := BindQueryVariables(
		query,
		queryTestDialect{style: PlaceholderDollar},
		[]QueryVariable{
			{Name: "tenant_id", Value: 42, Type: "number"},
			{Name: "enabled", Value: "true", Type: "boolean"},
		},
	)
	if err != nil {
		t.Fatalf("BindQueryVariables() error = %v", err)
	}
	if want := []interface{}{42, 42, true}; !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	if bound == query || !containsAll(bound, "$1", "$2", "$3", "'{{ignored}}'") {
		t.Fatalf("bound query = %q", bound)
	}
}

func TestBindQueryVariablesRejectsMissingValues(t *testing.T) {
	_, _, err := BindQueryVariables(
		"SELECT {{missing}}",
		queryTestDialect{style: PlaceholderQuestion},
		nil,
	)
	if err == nil {
		t.Fatal("BindQueryVariables() accepted a missing value")
	}
}

func TestBindQueryVariablesIgnoresOracleAlternativeQuotes(t *testing.T) {
	query := `SELECT q'[{{ignored}}; DROP TABLE users]' AS value, {{actual}} FROM dual`
	bound, args, err := BindQueryVariables(
		query,
		queryTestDialect{style: PlaceholderDollar},
		[]QueryVariable{{Name: "actual", Value: 7}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 1 || args[0] != 7 ||
		!strings.Contains(bound, "q'[{{ignored}}; DROP TABLE users]'") ||
		!strings.Contains(bound, "$1") {
		t.Fatalf("bound = %q, args = %#v", bound, args)
	}
}

func containsAll(value string, expected ...string) bool {
	for _, item := range expected {
		if !strings.Contains(value, item) {
			return false
		}
	}
	return true
}
