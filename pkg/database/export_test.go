package database

import (
	"bytes"
	"encoding/csv"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWriteCSVRowsPreservesColumnOrderAndFormatsValues(t *testing.T) {
	timestamp := time.Date(2026, time.July, 24, 9, 30, 15, 123, time.UTC)
	var output bytes.Buffer

	stats, err := WriteCSVRows(
		&output,
		[]string{"id", "name", "metadata", "created_at", "nullable", "binary"},
		[]map[string]interface{}{
			{
				"id":         42,
				"name":       "Thunder, \"north\"",
				"metadata":   map[string]interface{}{"enabled": true},
				"created_at": timestamp,
				"nullable":   nil,
				"binary":     []byte{0xff, 0x00},
			},
		},
		CSVOptions{
			Delimiter:     ";",
			IncludeHeader: true,
			NullValue:     "NULL",
		},
	)
	if err != nil {
		t.Fatalf("write CSV rows: %v", err)
	}
	if stats.Rows != 1 {
		t.Fatalf("row count = %d, want 1", stats.Rows)
	}

	reader := csv.NewReader(strings.NewReader(output.String()))
	reader.Comma = ';'
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("read generated CSV: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("record count = %d, want 2", len(records))
	}

	expectedHeader := []string{"id", "name", "metadata", "created_at", "nullable", "binary"}
	for index, expected := range expectedHeader {
		if records[0][index] != expected {
			t.Fatalf("header %d = %q, want %q", index, records[0][index], expected)
		}
	}
	expectedRow := []string{
		"42",
		"Thunder, \"north\"",
		`{"enabled":true}`,
		"2026-07-24T09:30:15.000000123Z",
		"NULL",
		"base64:/wA=",
	}
	for index, expected := range expectedRow {
		if records[1][index] != expected {
			t.Fatalf("value %d = %q, want %q", index, records[1][index], expected)
		}
	}
}

func TestWriteCSVRowsValidatesInput(t *testing.T) {
	if _, err := WriteCSVRows(
		&bytes.Buffer{},
		nil,
		[]map[string]interface{}{{"id": 1}},
		CSVOptions{},
	); err == nil {
		t.Fatal("expected missing-column validation error")
	}

	if _, err := WriteCSVRows(
		&bytes.Buffer{},
		[]string{"id"},
		nil,
		CSVOptions{Delimiter: "||"},
	); err == nil {
		t.Fatal("expected invalid-delimiter validation error")
	}
}

type failingRowStream struct {
	next bool
}

func (s *failingRowStream) Columns() ([]string, error) {
	return []string{"id"}, nil
}

func (s *failingRowStream) Next() bool {
	if s.next {
		return false
	}
	s.next = true
	return true
}

func (s *failingRowStream) Values() ([]interface{}, error) {
	return nil, errors.New("scan failed")
}

func (s *failingRowStream) Err() error {
	return nil
}

func TestWriteCSVStreamPropagatesRowErrors(t *testing.T) {
	if _, err := WriteCSVStream(
		&bytes.Buffer{},
		&failingRowStream{},
		CSVOptions{IncludeHeader: true},
	); err == nil || !strings.Contains(err.Error(), "scan failed") {
		t.Fatalf("expected row scan error, got %v", err)
	}
}
