package postgres

import (
	"testing"

	"rollingthunder/pkg/database"
)

func TestPostgresCapabilityContract(t *testing.T) {
	driver := &Postgres{}
	capabilities := driver.Capabilities()
	if err := capabilities.Validate(); err != nil {
		t.Fatalf("capabilities are invalid: %v", err)
	}
	if capabilities.Engine != "postgres" {
		t.Fatalf("engine = %q, want postgres", capabilities.Engine)
	}
	if !capabilities.MaterializedViews ||
		!capabilities.Functions ||
		!capabilities.TransactionalDDL {
		t.Fatalf("missing PostgreSQL capabilities: %+v", capabilities)
	}
	if capabilities.Dialect.PlaceholderStyle != database.PlaceholderDollar {
		t.Fatalf(
			"placeholder style = %q, want dollar",
			capabilities.Dialect.PlaceholderStyle,
		)
	}
}

func TestPostgresDialectPrimitives(t *testing.T) {
	driver := &Postgres{}
	if got := driver.QuoteIdentifier(`odd"name`); got != `"odd""name"` {
		t.Fatalf("quoted identifier = %q", got)
	}
	if got := driver.Placeholder(3); got != "$3" {
		t.Fatalf("placeholder = %q, want $3", got)
	}
	if got, err := driver.PaginationClause(50, 100); err != nil ||
		got != "LIMIT 50 OFFSET 100" {
		t.Fatalf("pagination = %q, %v", got, err)
	}
	if _, err := driver.PaginationClause(-1, 0); err == nil {
		t.Fatal("negative pagination limit was accepted")
	}
}
