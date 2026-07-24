package postgres

import (
	"errors"
	"testing"
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
