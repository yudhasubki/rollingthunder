package database

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"time"
	"unicode/utf8"
)

type ExportFormat string

const (
	ExportFormatCSV  ExportFormat = "csv"
	ExportFormatJSON ExportFormat = "json"
	ExportFormatSQL  ExportFormat = "sql"
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

type JSONOptions struct {
	Pretty bool `json:"pretty"`
}

const (
	DefaultSQLInsertBatchSize = 100
	MaxSQLInsertBatchSize     = 10000
)

type SQLInsertOptions struct {
	BatchSize          int  `json:"batchSize"`
	IncludeTransaction bool `json:"includeTransaction"`
}

func (options SQLInsertOptions) EffectiveBatchSize() int {
	if options.BatchSize == 0 {
		return DefaultSQLInsertBatchSize
	}
	return options.BatchSize
}

type ExportOptions struct {
	Format ExportFormat     `json:"format"`
	CSV    CSVOptions       `json:"csv"`
	JSON   JSONOptions      `json:"json"`
	SQL    SQLInsertOptions `json:"sql"`
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

func ValidateExportOptions(options ExportOptions) error {
	switch options.Format {
	case ExportFormatCSV:
		_, err := parseCSVDelimiter(options.CSV.Delimiter)
		return err
	case ExportFormatJSON:
		return nil
	case ExportFormatSQL:
		if options.SQL.BatchSize < 0 || options.SQL.BatchSize > MaxSQLInsertBatchSize {
			return fmt.Errorf(
				"SQL INSERT batch size must be 0 (default) or between 1 and %d",
				MaxSQLInsertBatchSize,
			)
		}
		return nil
	default:
		return fmt.Errorf("unsupported export format %q", options.Format)
	}
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

type orderedJSONRow struct {
	columns []string
	values  []interface{}
}

func (r orderedJSONRow) MarshalJSON() ([]byte, error) {
	if len(r.values) != len(r.columns) {
		return nil, fmt.Errorf(
			"export row has %d values for %d columns",
			len(r.values),
			len(r.columns),
		)
	}

	var output bytes.Buffer
	output.WriteByte('{')
	for index, column := range r.columns {
		if index > 0 {
			output.WriteByte(',')
		}

		encodedColumn, err := json.Marshal(column)
		if err != nil {
			return nil, fmt.Errorf("encode JSON column: %w", err)
		}
		output.Write(encodedColumn)
		output.WriteByte(':')

		value, err := normalizeJSONValue(r.values[index])
		if err != nil {
			return nil, err
		}
		encodedValue, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode JSON value for %q: %w", column, err)
		}
		output.Write(encodedValue)
	}
	output.WriteByte('}')
	return output.Bytes(), nil
}

func normalizeJSONValue(value interface{}) (interface{}, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case json.RawMessage:
		trimmed := bytes.TrimSpace(typed)
		if len(trimmed) == 0 {
			return nil, nil
		}
		if !json.Valid(trimmed) {
			return nil, fmt.Errorf("encode JSON value: invalid raw JSON")
		}
		return json.RawMessage(bytes.Clone(trimmed)), nil
	case []byte:
		if !utf8.Valid(typed) {
			return "base64:" + base64.StdEncoding.EncodeToString(typed), nil
		}

		trimmed := bytes.TrimSpace(typed)
		if len(trimmed) > 0 &&
			(trimmed[0] == '{' || trimmed[0] == '[') &&
			json.Valid(trimmed) {
			return json.RawMessage(bytes.Clone(trimmed)), nil
		}
		return string(typed), nil
	case time.Time:
		return typed.Format(time.RFC3339Nano), nil
	case float32:
		value := float64(typed)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Sprint(typed), nil
		}
		return typed, nil
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return fmt.Sprint(typed), nil
		}
		return typed, nil
	case json.Number:
		return typed, nil
	case map[string]interface{}:
		normalized := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			value, err := normalizeJSONValue(item)
			if err != nil {
				return nil, err
			}
			normalized[key] = value
		}
		return normalized, nil
	case []interface{}:
		normalized := make([]interface{}, len(typed))
		for index, item := range typed {
			value, err := normalizeJSONValue(item)
			if err != nil {
				return nil, err
			}
			normalized[index] = value
		}
		return normalized, nil
	case fmt.Stringer:
		return typed.String(), nil
	default:
		return value, nil
	}
}

type jsonSink struct {
	writer *bufio.Writer
	pretty bool
	rows   int64
}

func newJSONSink(writer io.Writer, options JSONOptions) (*jsonSink, error) {
	sink := &jsonSink{
		writer: bufio.NewWriter(writer),
		pretty: options.Pretty,
	}
	if err := sink.writeString("["); err != nil {
		return nil, err
	}
	return sink, nil
}

func (s *jsonSink) writeString(value string) error {
	written, err := s.writer.WriteString(value)
	if err != nil {
		return err
	}
	if written != len(value) {
		return io.ErrShortWrite
	}
	return nil
}

func (s *jsonSink) writeBytes(value []byte) error {
	written, err := s.writer.Write(value)
	if err != nil {
		return err
	}
	if written != len(value) {
		return io.ErrShortWrite
	}
	return nil
}

func (s *jsonSink) writeValues(columns []string, values []interface{}) error {
	row := orderedJSONRow{columns: columns, values: values}
	var (
		encoded []byte
		err     error
	)
	if s.pretty {
		encoded, err = json.MarshalIndent(row, "", "  ")
	} else {
		encoded, err = json.Marshal(row)
	}
	if err != nil {
		return err
	}

	if s.rows > 0 {
		if s.pretty {
			if err := s.writeString(",\n"); err != nil {
				return err
			}
		} else if err := s.writeString(","); err != nil {
			return err
		}
	} else if s.pretty {
		if err := s.writeString("\n"); err != nil {
			return err
		}
	}

	if s.pretty {
		encoded = bytes.ReplaceAll(encoded, []byte("\n"), []byte("\n  "))
		if err := s.writeString("  "); err != nil {
			return err
		}
	}
	if err := s.writeBytes(encoded); err != nil {
		return err
	}
	s.rows++
	return nil
}

func (s *jsonSink) close() error {
	if s.pretty && s.rows > 0 {
		if err := s.writeString("\n"); err != nil {
			return err
		}
	}
	if err := s.writeString("]\n"); err != nil {
		return err
	}
	return s.writer.Flush()
}

func WriteJSONStream(writer io.Writer, rows RowStream, options JSONOptions) (ExportStats, error) {
	columns, err := rows.Columns()
	if err != nil {
		return ExportStats{}, fmt.Errorf("read export columns: %w", err)
	}

	sink, err := newJSONSink(writer, options)
	if err != nil {
		return ExportStats{}, fmt.Errorf("start JSON export: %w", err)
	}
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return ExportStats{}, fmt.Errorf("read export row: %w", err)
		}
		if err := sink.writeValues(columns, values); err != nil {
			return ExportStats{}, fmt.Errorf("write JSON row: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return ExportStats{}, fmt.Errorf("read export rows: %w", err)
	}
	if err := sink.close(); err != nil {
		return ExportStats{}, fmt.Errorf("finish JSON export: %w", err)
	}
	return ExportStats{Rows: sink.rows}, nil
}

func WriteJSONRows(
	writer io.Writer,
	columns []string,
	rows []map[string]interface{},
	options JSONOptions,
) (ExportStats, error) {
	if len(columns) == 0 && len(rows) > 0 {
		return ExportStats{}, fmt.Errorf("query result columns are required")
	}

	sink, err := newJSONSink(writer, options)
	if err != nil {
		return ExportStats{}, fmt.Errorf("start JSON export: %w", err)
	}
	for _, row := range rows {
		values := make([]interface{}, len(columns))
		for index, column := range columns {
			values[index] = row[column]
		}
		if err := sink.writeValues(columns, values); err != nil {
			return ExportStats{}, fmt.Errorf("write JSON row: %w", err)
		}
	}
	if err := sink.close(); err != nil {
		return ExportStats{}, fmt.Errorf("finish JSON export: %w", err)
	}
	return ExportStats{Rows: sink.rows}, nil
}

func WriteExportStream(
	writer io.Writer,
	rows RowStream,
	options ExportOptions,
) (ExportStats, error) {
	if err := ValidateExportOptions(options); err != nil {
		return ExportStats{}, err
	}

	switch options.Format {
	case ExportFormatCSV:
		return WriteCSVStream(writer, rows, options.CSV)
	case ExportFormatJSON:
		return WriteJSONStream(writer, rows, options.JSON)
	case ExportFormatSQL:
		return ExportStats{}, fmt.Errorf(
			"SQL INSERT export requires a driver-specific table serializer",
		)
	default:
		return ExportStats{}, fmt.Errorf("unsupported export format %q", options.Format)
	}
}

func WriteExportRows(
	writer io.Writer,
	columns []string,
	rows []map[string]interface{},
	options ExportOptions,
) (ExportStats, error) {
	if err := ValidateExportOptions(options); err != nil {
		return ExportStats{}, err
	}

	switch options.Format {
	case ExportFormatCSV:
		return WriteCSVRows(writer, columns, rows, options.CSV)
	case ExportFormatJSON:
		return WriteJSONRows(writer, columns, rows, options.JSON)
	case ExportFormatSQL:
		return ExportStats{}, fmt.Errorf("SQL INSERT export requires a table source")
	default:
		return ExportStats{}, fmt.Errorf("unsupported export format %q", options.Format)
	}
}
