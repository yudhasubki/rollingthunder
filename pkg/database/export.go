package database

import (
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"time"
	"unicode/utf8"
)

type ExportFormat string

const (
	ExportFormatCSV ExportFormat = "csv"
)

type ExportScope string

const (
	ExportScopePage ExportScope = "page"
	ExportScopeAll  ExportScope = "all"
)

type CSVOptions struct {
	Delimiter     string `json:"delimiter"`
	IncludeHeader bool   `json:"includeHeader"`
	NullValue     string `json:"nullValue"`
}

type ExportOptions struct {
	Format ExportFormat `json:"format"`
	CSV    CSVOptions   `json:"csv"`
}

type TableExportRequest struct {
	Table         Table         `json:"table"`
	Scope         ExportScope   `json:"scope"`
	SuggestedName string        `json:"suggestedName"`
	Options       ExportOptions `json:"options"`
}

type RowsExportRequest struct {
	Columns       []string                 `json:"columns"`
	Rows          []map[string]interface{} `json:"rows"`
	SuggestedName string                   `json:"suggestedName"`
	Options       ExportOptions            `json:"options"`
}

type ExportStats struct {
	Rows int64
}

type ExportResult struct {
	Path      string       `json:"path"`
	Rows      int64        `json:"rows"`
	Bytes     int64        `json:"bytes"`
	Cancelled bool         `json:"cancelled"`
	Format    ExportFormat `json:"format"`
}

type RowStream interface {
	Columns() ([]string, error)
	Next() bool
	Values() ([]interface{}, error)
	Err() error
}

type csvSink struct {
	writer    *csv.Writer
	nullValue string
}

func newCSVSink(writer io.Writer, options CSVOptions) (*csvSink, error) {
	delimiter, err := parseCSVDelimiter(options.Delimiter)
	if err != nil {
		return nil, err
	}

	csvWriter := csv.NewWriter(writer)
	csvWriter.Comma = delimiter
	return &csvSink{
		writer:    csvWriter,
		nullValue: options.NullValue,
	}, nil
}

func parseCSVDelimiter(value string) (rune, error) {
	if value == "" {
		return ',', nil
	}
	if utf8.RuneCountInString(value) != 1 {
		return 0, fmt.Errorf("CSV delimiter must be exactly one character")
	}

	delimiter, _ := utf8.DecodeRuneInString(value)
	if delimiter == 0 || delimiter == '"' || delimiter == '\r' || delimiter == '\n' || delimiter == utf8.RuneError {
		return 0, fmt.Errorf("invalid CSV delimiter")
	}
	return delimiter, nil
}

func (s *csvSink) writeHeader(columns []string, include bool) error {
	if !include {
		return nil
	}
	return s.writer.Write(columns)
}

func (s *csvSink) writeValues(values []interface{}) error {
	record := make([]string, len(values))
	for index, value := range values {
		formatted, err := formatCSVValue(value, s.nullValue)
		if err != nil {
			return err
		}
		record[index] = formatted
	}
	return s.writer.Write(record)
}

func (s *csvSink) close() error {
	s.writer.Flush()
	return s.writer.Error()
}

func formatCSVValue(value interface{}, nullValue string) (string, error) {
	if value == nil {
		return nullValue, nil
	}

	switch typed := value.(type) {
	case string:
		return typed, nil
	case []byte:
		if utf8.Valid(typed) {
			return string(typed), nil
		}
		return "base64:" + base64.StdEncoding.EncodeToString(typed), nil
	case time.Time:
		return typed.Format(time.RFC3339Nano), nil
	case bool:
		return strconv.FormatBool(typed), nil
	case json.RawMessage:
		return string(typed), nil
	case fmt.Stringer:
		return typed.String(), nil
	}

	valueOf := reflect.ValueOf(value)
	switch valueOf.Kind() {
	case reflect.Map, reflect.Slice, reflect.Array:
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("encode CSV value: %w", err)
		}
		return string(encoded), nil
	default:
		return fmt.Sprint(value), nil
	}
}

func WriteCSVStream(writer io.Writer, rows RowStream, options CSVOptions) (ExportStats, error) {
	columns, err := rows.Columns()
	if err != nil {
		return ExportStats{}, fmt.Errorf("read export columns: %w", err)
	}

	sink, err := newCSVSink(writer, options)
	if err != nil {
		return ExportStats{}, err
	}
	if err := sink.writeHeader(columns, options.IncludeHeader); err != nil {
		return ExportStats{}, fmt.Errorf("write CSV header: %w", err)
	}

	var count int64
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return ExportStats{}, fmt.Errorf("read export row: %w", err)
		}
		if len(values) != len(columns) {
			return ExportStats{}, fmt.Errorf(
				"export row has %d values for %d columns",
				len(values),
				len(columns),
			)
		}
		if err := sink.writeValues(values); err != nil {
			return ExportStats{}, fmt.Errorf("write CSV row: %w", err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return ExportStats{}, fmt.Errorf("read export rows: %w", err)
	}
	if err := sink.close(); err != nil {
		return ExportStats{}, fmt.Errorf("flush CSV export: %w", err)
	}

	return ExportStats{Rows: count}, nil
}

func WriteCSVRows(
	writer io.Writer,
	columns []string,
	rows []map[string]interface{},
	options CSVOptions,
) (ExportStats, error) {
	if len(columns) == 0 && len(rows) > 0 {
		return ExportStats{}, fmt.Errorf("query result columns are required")
	}

	sink, err := newCSVSink(writer, options)
	if err != nil {
		return ExportStats{}, err
	}
	if err := sink.writeHeader(columns, options.IncludeHeader); err != nil {
		return ExportStats{}, fmt.Errorf("write CSV header: %w", err)
	}

	for _, row := range rows {
		values := make([]interface{}, len(columns))
		for index, column := range columns {
			values[index] = row[column]
		}
		if err := sink.writeValues(values); err != nil {
			return ExportStats{}, fmt.Errorf("write CSV row: %w", err)
		}
	}
	if err := sink.close(); err != nil {
		return ExportStats{}, fmt.Errorf("flush CSV export: %w", err)
	}

	return ExportStats{Rows: int64(len(rows))}, nil
}
