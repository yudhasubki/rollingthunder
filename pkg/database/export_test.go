package database

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"math"
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

func TestWriteJSONRowsPreservesColumnOrderAndNativeValues(t *testing.T) {
	timestamp := time.Date(2026, time.July, 24, 9, 30, 15, 123, time.UTC)
	columns := []string{
		"id",
		"metadata",
		"created_at",
		"nullable",
		"text_bytes",
		"binary",
		"infinity",
	}
	var output bytes.Buffer

	stats, err := WriteJSONRows(
		&output,
		columns,
		[]map[string]interface{}{
			{
				"id":         42,
				"metadata":   []byte(`{"enabled":true}`),
				"created_at": timestamp,
				"nullable":   nil,
				"text_bytes": []byte("thunder"),
				"binary":     []byte{0xff, 0x00},
				"infinity":   math.Inf(1),
			},
		},
		JSONOptions{Pretty: true},
	)
	if err != nil {
		t.Fatalf("write JSON rows: %v", err)
	}
	if stats.Rows != 1 {
		t.Fatalf("row count = %d, want 1", stats.Rows)
	}
	if !json.Valid(output.Bytes()) {
		t.Fatalf("generated invalid JSON: %s", output.String())
	}

	var decoded []map[string]interface{}
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("decode generated JSON: %v", err)
	}
	if len(decoded) != 1 {
		t.Fatalf("decoded row count = %d, want 1", len(decoded))
	}
	row := decoded[0]
	if row["id"] != float64(42) {
		t.Fatalf("id = %#v, want 42", row["id"])
	}
	metadata, ok := row["metadata"].(map[string]interface{})
	if !ok || metadata["enabled"] != true {
		t.Fatalf("metadata = %#v, want embedded JSON object", row["metadata"])
	}
	if row["created_at"] != "2026-07-24T09:30:15.000000123Z" {
		t.Fatalf("created_at = %#v", row["created_at"])
	}
	if row["nullable"] != nil {
		t.Fatalf("nullable = %#v, want nil", row["nullable"])
	}
	if row["text_bytes"] != "thunder" {
		t.Fatalf("text_bytes = %#v, want thunder", row["text_bytes"])
	}
	if row["binary"] != "base64:/wA=" {
		t.Fatalf("binary = %#v, want base64 marker", row["binary"])
	}
	if row["infinity"] != "+Inf" {
		t.Fatalf("infinity = %#v, want +Inf", row["infinity"])
	}

	previousIndex := -1
	for _, column := range columns {
		index := strings.Index(output.String(), `"`+column+`"`)
		if index <= previousIndex {
			t.Fatalf("column %q is not in source order: %s", column, output.String())
		}
		previousIndex = index
	}
	if !strings.Contains(output.String(), "\n    \"metadata\"") {
		t.Fatalf("pretty JSON indentation is missing: %s", output.String())
	}
}

func TestWriteJSONRowsSupportsCompactAndEmptyDocuments(t *testing.T) {
	var compact bytes.Buffer
	if _, err := WriteJSONRows(
		&compact,
		[]string{"id", "name"},
		[]map[string]interface{}{{"id": 1, "name": "Alpha"}},
		JSONOptions{},
	); err != nil {
		t.Fatalf("write compact JSON: %v", err)
	}
	if compact.String() != "[{\"id\":1,\"name\":\"Alpha\"}]\n" {
		t.Fatalf("compact JSON = %q", compact.String())
	}

	var empty bytes.Buffer
	if _, err := WriteJSONRows(&empty, nil, nil, JSONOptions{Pretty: true}); err != nil {
		t.Fatalf("write empty JSON: %v", err)
	}
	if empty.String() != "[]\n" {
		t.Fatalf("empty JSON = %q, want []", empty.String())
	}
}

func TestWriteJSONStreamPropagatesRowErrors(t *testing.T) {
	if _, err := WriteJSONStream(
		&bytes.Buffer{},
		&failingRowStream{},
		JSONOptions{},
	); err == nil || !strings.Contains(err.Error(), "scan failed") {
		t.Fatalf("expected row scan error, got %v", err)
	}
}

type sliceRowStream struct {
	columns []string
	rows    [][]interface{}
	index   int
}

func (s *sliceRowStream) Columns() ([]string, error) {
	return s.columns, nil
}

func (s *sliceRowStream) Next() bool {
	return s.index < len(s.rows)
}

func (s *sliceRowStream) Values() ([]interface{}, error) {
	row := s.rows[s.index]
	s.index++
	return row, nil
}

func (s *sliceRowStream) Err() error {
	return nil
}

func TestWriteJSONStreamWritesRowsIncrementally(t *testing.T) {
	var output bytes.Buffer
	stats, err := WriteJSONStream(
		&output,
		&sliceRowStream{
			columns: []string{"id", "name"},
			rows: [][]interface{}{
				{1, "Alpha"},
				{2, "Bravo"},
			},
		},
		JSONOptions{},
	)
	if err != nil {
		t.Fatalf("write JSON stream: %v", err)
	}
	if stats.Rows != 2 {
		t.Fatalf("row count = %d, want 2", stats.Rows)
	}
	if output.String() != `[{"id":1,"name":"Alpha"},{"id":2,"name":"Bravo"}]`+"\n" {
		t.Fatalf("streamed JSON = %q", output.String())
	}
}

func TestValidateExportOptionsRejectsUnknownFormats(t *testing.T) {
	if err := ValidateExportOptions(ExportOptions{Format: ExportFormat("xml")}); err == nil {
		t.Fatal("expected unsupported-format error")
	}
}
