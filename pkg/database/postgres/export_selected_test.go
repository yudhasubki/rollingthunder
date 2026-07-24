package postgres

import (
	"errors"
	"testing"
)

type advancingExportRows struct {
	columns []string
	rows    [][]interface{}
	next    int
	current int
	err     error
}

func (rows *advancingExportRows) Columns() ([]string, error) {
	return rows.columns, nil
}

func (rows *advancingExportRows) Next() bool {
	if rows.next >= len(rows.rows) {
		return false
	}
	rows.current = rows.next
	rows.next++
	return true
}

func (rows *advancingExportRows) Values() ([]interface{}, error) {
	return rows.rows[rows.current], nil
}

func (rows *advancingExportRows) Err() error {
	return rows.err
}

func TestSelectedRowStreamKeepsRequestedPageRowsInSourceOrder(t *testing.T) {
	base := &advancingExportRows{
		columns: []string{"id"},
		rows: [][]interface{}{
			{1},
			{2},
			{3},
			{4},
		},
	}
	rows, err := newSelectedRowStream(base, []int{3, 1, 1}, 100)
	if err != nil {
		t.Fatalf("create selected-row stream: %v", err)
	}

	var values []int
	for rows.Next() {
		row, err := rows.Values()
		if err != nil {
			t.Fatalf("read selected row: %v", err)
		}
		values = append(values, row[0].(int))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("selected-row stream error: %v", err)
	}
	if len(values) != 2 || values[0] != 2 || values[1] != 4 {
		t.Fatalf("selected values = %v, want [2 4]", values)
	}
}

func TestSelectedRowStreamValidatesIndexesAndForwardsErrors(t *testing.T) {
	base := &advancingExportRows{columns: []string{"id"}}
	if _, err := newSelectedRowStream(base, nil, 100); err == nil {
		t.Fatal("expected empty-selection error")
	}
	if _, err := newSelectedRowStream(base, []int{-1}, 100); err == nil {
		t.Fatal("expected negative-index error")
	}
	if _, err := newSelectedRowStream(base, []int{100}, 100); err == nil {
		t.Fatal("expected out-of-page error")
	}

	iterationErr := errors.New("iteration failed")
	rows, err := newSelectedRowStream(
		&advancingExportRows{
			columns: []string{"id"},
			err:     iterationErr,
		},
		[]int{0},
		100,
	)
	if err != nil {
		t.Fatalf("create selected stream: %v", err)
	}
	if rows.Next() {
		t.Fatal("unexpected selected row")
	}
	if !errors.Is(rows.Err(), iterationErr) {
		t.Fatalf("expected forwarded iteration error, got %v", rows.Err())
	}
}

func TestSelectedRowStreamRejectsRowsThatDisappearedFromThePage(t *testing.T) {
	rows, err := newSelectedRowStream(
		&advancingExportRows{
			columns: []string{"id"},
			rows:    [][]interface{}{{1}},
		},
		[]int{1},
		100,
	)
	if err != nil {
		t.Fatalf("create selected stream: %v", err)
	}
	if rows.Next() {
		t.Fatal("unexpected selected row")
	}
	if err := rows.Err(); err == nil {
		t.Fatal("expected missing selected-row error")
	}
}
