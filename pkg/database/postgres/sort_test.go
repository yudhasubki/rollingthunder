package postgres

import (
	"rollingthunder/pkg/database"
	"testing"
)

func TestBuildPostgresOrderClauseUsesPrimaryKeyByDefault(t *testing.T) {
	t.Parallel()

	structures := database.Structures{
		{Name: "id", IsPrimary: true},
		{Name: "name"},
	}

	clause, err := buildPostgresOrderClause(nil, structures)
	if err != nil {
		t.Fatalf("build order clause: %v", err)
	}

	const expected = ` ORDER BY "id" ASC NULLS LAST`
	if clause != expected {
		t.Fatalf("expected %q, got %q", expected, clause)
	}
}

func TestBuildPostgresOrderClauseAddsStableTieBreaker(t *testing.T) {
	t.Parallel()

	structures := database.Structures{
		{Name: "id", IsPrimary: true},
		{Name: "created_at"},
	}
	sorts := []database.Sort{
		{
			Column:    "created_at",
			Direction: database.SortDescending,
			Nulls:     database.NullsLast,
		},
	}

	clause, err := buildPostgresOrderClause(sorts, structures)
	if err != nil {
		t.Fatalf("build order clause: %v", err)
	}

	const expected = ` ORDER BY "created_at" DESC NULLS LAST, "id" ASC NULLS LAST`
	if clause != expected {
		t.Fatalf("expected %q, got %q", expected, clause)
	}
}

func TestBuildPostgresOrderClauseSupportsMultipleSorts(t *testing.T) {
	t.Parallel()

	structures := database.Structures{
		{Name: "tenant_id", IsPrimary: true},
		{Name: "id", IsPrimary: true},
		{Name: `display"name`},
	}
	sorts := []database.Sort{
		{Column: `display"name`, Direction: database.SortAscending, Nulls: database.NullsFirst},
		{Column: "tenant_id", Direction: database.SortDescending, Nulls: database.NullsLast},
	}

	clause, err := buildPostgresOrderClause(sorts, structures)
	if err != nil {
		t.Fatalf("build order clause: %v", err)
	}

	const expected = ` ORDER BY "display""name" ASC NULLS FIRST, "tenant_id" DESC NULLS LAST, "id" ASC NULLS LAST`
	if clause != expected {
		t.Fatalf("expected %q, got %q", expected, clause)
	}
}

func TestBuildPostgresOrderClauseFallsBackToCtid(t *testing.T) {
	t.Parallel()

	structures := database.Structures{{Name: "name"}}
	sorts := []database.Sort{{Column: "name", Direction: database.SortAscending}}

	clause, err := buildPostgresOrderClause(sorts, structures)
	if err != nil {
		t.Fatalf("build order clause: %v", err)
	}

	const expected = ` ORDER BY "name" ASC NULLS LAST, tableoid ASC, ctid ASC`
	if clause != expected {
		t.Fatalf("expected %q, got %q", expected, clause)
	}
}

func TestBuildPostgresOrderClauseRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	structures := database.Structures{{Name: "id", IsPrimary: true}}
	testCases := []struct {
		name string
		sort database.Sort
	}{
		{
			name: "unknown column",
			sort: database.Sort{Column: "missing", Direction: database.SortAscending},
		},
		{
			name: "invalid direction",
			sort: database.Sort{Column: "id", Direction: database.SortDirection("sideways")},
		},
		{
			name: "invalid null position",
			sort: database.Sort{
				Column:    "id",
				Direction: database.SortAscending,
				Nulls:     database.NullsPosition("middle"),
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := buildPostgresOrderClause([]database.Sort{testCase.sort}, structures); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestQuotePostgresQualifiedIdentifier(t *testing.T) {
	t.Parallel()

	got := quotePostgresQualifiedIdentifier(`odd"schema`, `order item`)
	const expected = `"odd""schema"."order item"`
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}
