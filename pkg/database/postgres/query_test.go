package postgres

import (
	"errors"
	"testing"

	"rollingthunder/pkg/database"
)

type fakeMappedRows struct {
	rows        []map[string]interface{}
	index       int
	nextCalls   int
	scanCalls   int
	scanError   error
	resultError error
}

func (f *fakeMappedRows) Next() bool {
	f.nextCalls++
	if f.index >= len(f.rows) {
		return false
	}
	f.index++
	return true
}

func (f *fakeMappedRows) MapScan(dest map[string]interface{}) error {
	f.scanCalls++
	if f.scanError != nil {
		return f.scanError
	}
	for key, value := range f.rows[f.index-1] {
		dest[key] = value
	}
	return nil
}

func (f *fakeMappedRows) Err() error {
	return f.resultError
}

func TestCollectQueryResultsStopsAfterLimitAndDetectsMoreRows(t *testing.T) {
	rows := &fakeMappedRows{
		rows: []map[string]interface{}{
			{"id": 1},
			{"id": 2},
			{"id": 3},
		},
	}

	result, err := collectQueryResults(rows, 2)
	if err != nil {
		t.Fatalf("collect query results: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("row count = %d, want 2", len(result.Rows))
	}
	if !result.Truncated {
		t.Fatal("expected result to be marked as truncated")
	}
	if result.RowLimit != 2 {
		t.Fatalf("row limit = %d, want 2", result.RowLimit)
	}
	if rows.scanCalls != 2 {
		t.Fatalf("scan calls = %d, want 2", rows.scanCalls)
	}
	if rows.nextCalls != 3 {
		t.Fatalf("next calls = %d, want 3 to detect the extra row", rows.nextCalls)
	}
}

func TestCollectQueryResultsDoesNotMarkExactLimitAsTruncated(t *testing.T) {
	rows := &fakeMappedRows{
		rows: []map[string]interface{}{
			{"id": 1},
			{"id": 2},
		},
	}

	result, err := collectQueryResults(rows, 2)
	if err != nil {
		t.Fatalf("collect query results: %v", err)
	}
	if result.Truncated {
		t.Fatal("did not expect an exact-limit result to be marked as truncated")
	}
	if len(result.Rows) != 2 {
		t.Fatalf("row count = %d, want 2", len(result.Rows))
	}
}

func TestCollectQueryResultsSupportsUnlimitedInternalUse(t *testing.T) {
	rows := &fakeMappedRows{
		rows: []map[string]interface{}{
			{"id": 1},
			{"id": 2},
			{"id": 3},
		},
	}

	result, err := collectQueryResults(rows, 0)
	if err != nil {
		t.Fatalf("collect query results: %v", err)
	}
	if result.Truncated {
		t.Fatal("unlimited collection must not be marked as truncated")
	}
	if len(result.Rows) != 3 {
		t.Fatalf("row count = %d, want 3", len(result.Rows))
	}
}

func TestCollectQueryResultsReturnsScanAndIterationErrors(t *testing.T) {
	scanFailure := errors.New("scan failed")
	if _, err := collectQueryResults(
		&fakeMappedRows{
			rows:      []map[string]interface{}{{"id": 1}},
			scanError: scanFailure,
		},
		10,
	); !errors.Is(err, scanFailure) {
		t.Fatalf("expected scan error, got %v", err)
	}

	iterationFailure := errors.New("iteration failed")
	if _, err := collectQueryResults(
		&fakeMappedRows{resultError: iterationFailure},
		10,
	); !errors.Is(err, iterationFailure) {
		t.Fatalf("expected iteration error, got %v", err)
	}
}

func TestBuildPostgresExportQueryPreservesFilterSortAndPage(t *testing.T) {
	query, err := buildPostgresExportQuery(
		database.Table{
			Schema: "public",
			Name:   "orders",
			Offset: 200,
			Limit:  100,
			Filter: `"status" = 'open'`,
			Sorts: []database.Sort{
				{
					Column:    "created_at",
					Direction: database.SortDescending,
					Nulls:     database.NullsLast,
				},
			},
		},
		database.ExportScopePage,
		database.Structures{
			{Name: "id", IsPrimary: true},
			{Name: "created_at"},
		},
	)
	if err != nil {
		t.Fatalf("build export query: %v", err)
	}

	const expected = `SELECT * FROM "public"."orders" WHERE "status" = 'open' ORDER BY "created_at" DESC NULLS LAST, "id" ASC NULLS LAST LIMIT 100 OFFSET 200`
	if query != expected {
		t.Fatalf("query = %q, want %q", query, expected)
	}
}

func TestBuildPostgresExportQueryOmitsPaginationForAllRows(t *testing.T) {
	query, err := buildPostgresExportQuery(
		database.Table{
			Schema: "odd\"schema",
			Name:   "order items",
			Offset: 400,
			Limit:  100,
		},
		database.ExportScopeAll,
		database.Structures{{Name: "name"}},
	)
	if err != nil {
		t.Fatalf("build export query: %v", err)
	}

	const expected = `SELECT * FROM "odd""schema"."order items" ORDER BY tableoid ASC, ctid ASC`
	if query != expected {
		t.Fatalf("query = %q, want %q", query, expected)
	}
}

func TestBuildPostgresExportQueryRejectsInvalidScope(t *testing.T) {
	if _, err := buildPostgresExportQuery(
		database.Table{Schema: "public", Name: "orders", Limit: 100},
		database.ExportScope("unknown"),
		database.Structures{{Name: "id", IsPrimary: true}},
	); err == nil {
		t.Fatal("expected unsupported-scope error")
	}
}

func TestBuildPostgresExportQueryUsesPageBoundsForSelectedRows(t *testing.T) {
	query, err := buildPostgresExportQuery(
		database.Table{
			Schema: "public",
			Name:   "orders",
			Offset: 200,
			Limit:  100,
		},
		database.ExportScopeSelected,
		database.Structures{{Name: "id", IsPrimary: true}},
	)
	if err != nil {
		t.Fatalf("build selected-row export query: %v", err)
	}

	const expected = `SELECT * FROM "public"."orders" ORDER BY "id" ASC NULLS LAST LIMIT 100 OFFSET 200`
	if query != expected {
		t.Fatalf("query = %q, want %q", query, expected)
	}
}
