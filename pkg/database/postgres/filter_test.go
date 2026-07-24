package postgres

import (
	"reflect"
	"testing"

	"rollingthunder/pkg/database"
)

func TestBuildPostgresFilterClauseUsesTypedParameters(t *testing.T) {
	clause, args, err := buildPostgresFilterClause(
		[]database.Filter{
			{
				Column:   "status",
				Operator: database.FilterEqual,
				Value:    "open' OR true --",
			},
			{
				Column:   "customer name",
				Operator: database.FilterContains,
				Value:    "Ada",
			},
			{
				Column:   "deleted_at",
				Operator: database.FilterIsNull,
			},
		},
		database.Structures{
			{Name: "status"},
			{Name: "customer name"},
			{Name: "deleted_at"},
		},
		3,
	)
	if err != nil {
		t.Fatalf("build filter clause: %v", err)
	}

	const expected = ` WHERE "status" = $3 AND "customer name"::text ILIKE $4 AND "deleted_at" IS NULL`
	if clause != expected {
		t.Fatalf("clause = %q, want %q", clause, expected)
	}
	wantArgs := []interface{}{"open' OR true --", "%Ada%"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestBuildPostgresFilterClauseRejectsUnknownColumns(t *testing.T) {
	_, _, err := buildPostgresFilterClause(
		[]database.Filter{
			{
				Column:   "missing",
				Operator: database.FilterEqual,
				Value:    "value",
			},
		},
		database.Structures{{Name: "id"}},
		1,
	)
	if err == nil {
		t.Fatal("expected unknown-column error")
	}
}

func TestBuildPostgresFilterClauseRejectsInvalidOperators(t *testing.T) {
	_, _, err := buildPostgresFilterClause(
		[]database.Filter{
			{
				Column:   "id",
				Operator: database.FilterOperator("raw_sql"),
				Value:    "1 OR true",
			},
		},
		database.Structures{{Name: "id"}},
		1,
	)
	if err == nil {
		t.Fatal("expected invalid-operator error")
	}
}
