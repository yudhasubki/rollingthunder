package sqladapter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"

	"rollingthunder/pkg/application"
	"rollingthunder/pkg/database"
)

type InsertExportDialect struct {
	EngineLabel      string
	QuoteIdentifier  func(string) string
	QuoteQualified   func(string, string) string
	Literal          func(interface{}, database.Structure) (string, error)
	BeginStatement   string
	CommitStatement  string
	MultiRowValues   bool
	MaximumBatchSize int
}

func SQLNumericLiteral(value interface{}) (string, bool, error) {
	if number, ok := value.(json.Number); ok {
		parsed, err := number.Float64()
		if err != nil {
			return "", true, fmt.Errorf("invalid JSON number %q", number)
		}
		if math.IsInf(parsed, 0) || math.IsNaN(parsed) {
			return "", true, fmt.Errorf(
				"non-finite JSON numbers cannot be exported as SQL",
			)
		}
		return string(number), true, nil
	}
	valueOf := reflect.ValueOf(value)
	if !valueOf.IsValid() {
		return "", false, nil
	}
	switch valueOf.Kind() {
	case reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64:
		return strconv.FormatInt(valueOf.Int(), 10), true, nil
	case reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64:
		return strconv.FormatUint(valueOf.Uint(), 10), true, nil
	case reflect.Float32, reflect.Float64:
		number := valueOf.Float()
		if math.IsInf(number, 0) || math.IsNaN(number) {
			return "", true, fmt.Errorf(
				"non-finite floating-point values cannot be exported as SQL",
			)
		}
		return strconv.FormatFloat(
			number,
			'g',
			-1,
			valueOf.Type().Bits(),
		), true, nil
	default:
		return "", false, nil
	}
}

func (dialect InsertExportDialect) validate() error {
	if strings.TrimSpace(dialect.EngineLabel) == "" {
		return fmt.Errorf("SQL INSERT export engine label is required")
	}
	if dialect.QuoteIdentifier == nil ||
		dialect.QuoteQualified == nil ||
		dialect.Literal == nil {
		return fmt.Errorf("SQL INSERT export dialect is incomplete")
	}
	return nil
}

type insertExportSink struct {
	writer      *bufio.Writer
	prefix      string
	dialect     InsertExportDialect
	columns     database.Structures
	batchSize   int
	rowsInBatch int
	rows        int64
}

func newInsertExportSink(
	writer io.Writer,
	table database.Table,
	columns database.Structures,
	options database.SQLInsertOptions,
	dialect InsertExportDialect,
) (*insertExportSink, error) {
	if err := dialect.validate(); err != nil {
		return nil, err
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf(
			"table has no columns that can be exported as SQL INSERT",
		)
	}
	if options.Upsert {
		return nil, fmt.Errorf(
			"%s SQL INSERT export does not support generated upsert statements",
			dialect.EngineLabel,
		)
	}
	names := make([]string, 0, len(columns))
	for _, column := range columns {
		names = append(
			names,
			dialect.QuoteIdentifier(column.Name),
		)
	}
	batchSize := options.EffectiveBatchSize()
	if !dialect.MultiRowValues {
		batchSize = 1
	}
	if dialect.MaximumBatchSize > 0 &&
		batchSize > dialect.MaximumBatchSize {
		batchSize = dialect.MaximumBatchSize
	}
	return &insertExportSink{
		writer: bufio.NewWriter(writer),
		prefix: "INSERT INTO " +
			dialect.QuoteQualified(table.Schema, table.Name) +
			" (" + strings.Join(names, ", ") + ") VALUES\n",
		dialect:   dialect,
		columns:   columns,
		batchSize: batchSize,
	}, nil
}

func (sink *insertExportSink) write(value string) error {
	written, err := sink.writer.WriteString(value)
	if err != nil {
		return err
	}
	if written != len(value) {
		return io.ErrShortWrite
	}
	return nil
}

func (sink *insertExportSink) writeValues(
	values []interface{},
) error {
	if len(values) != len(sink.columns) {
		return fmt.Errorf(
			"SQL export row has %d values for %d columns",
			len(values),
			len(sink.columns),
		)
	}
	literals := make([]string, len(values))
	for index, value := range values {
		literal, err := sink.dialect.Literal(
			value,
			sink.columns[index],
		)
		if err != nil {
			return fmt.Errorf(
				"format column %q: %w",
				sink.columns[index].Name,
				err,
			)
		}
		literals[index] = literal
	}
	if sink.rowsInBatch == sink.batchSize {
		if err := sink.write(";\n\n"); err != nil {
			return err
		}
		sink.rowsInBatch = 0
	}
	if sink.rowsInBatch == 0 {
		if err := sink.write(sink.prefix); err != nil {
			return err
		}
	} else if err := sink.write(",\n"); err != nil {
		return err
	}
	if err := sink.write(
		"  (" + strings.Join(literals, ", ") + ")",
	); err != nil {
		return err
	}
	sink.rowsInBatch++
	sink.rows++
	return nil
}

func WriteInsertExportContext(
	ctx context.Context,
	writer io.Writer,
	rows database.RowStream,
	table database.Table,
	columns database.Structures,
	options database.SQLInsertOptions,
	dialect InsertExportDialect,
) (database.ExportStats, error) {
	if err := database.CheckExportContext(ctx); err != nil {
		return database.ExportStats{}, err
	}
	streamColumns, err := rows.Columns()
	if err != nil {
		return database.ExportStats{}, fmt.Errorf(
			"read SQL export columns: %w",
			err,
		)
	}
	if len(streamColumns) != len(columns) {
		return database.ExportStats{}, fmt.Errorf(
			"SQL export returned %d columns for %d insert columns",
			len(streamColumns),
			len(columns),
		)
	}
	for index, column := range columns {
		if !strings.EqualFold(streamColumns[index], column.Name) {
			return database.ExportStats{}, fmt.Errorf(
				"SQL export column %d is %q, expected %q",
				index,
				streamColumns[index],
				column.Name,
			)
		}
	}
	sink, err := newInsertExportSink(
		writer,
		table,
		columns,
		options,
		dialect,
	)
	if err != nil {
		return database.ExportStats{}, err
	}
	if err := sink.write(
		"-- " + application.Name + " " + dialect.EngineLabel +
			" INSERT export\n" +
			"-- Generated and identity columns are omitted.\n\n",
	); err != nil {
		return database.ExportStats{}, err
	}
	if options.IncludeTransaction &&
		strings.TrimSpace(dialect.BeginStatement) != "" {
		if err := sink.write(
			strings.TrimSpace(dialect.BeginStatement) + "\n\n",
		); err != nil {
			return database.ExportStats{}, err
		}
	}
	for rows.Next() {
		if err := database.CheckExportContext(ctx); err != nil {
			return database.ExportStats{}, err
		}
		values, err := rows.Values()
		if err != nil {
			return database.ExportStats{}, fmt.Errorf(
				"read SQL export row: %w",
				err,
			)
		}
		if err := sink.writeValues(values); err != nil {
			return database.ExportStats{}, fmt.Errorf(
				"write SQL export row: %w",
				err,
			)
		}
		database.ReportExportProgress(ctx, sink.rows)
	}
	if err := rows.Err(); err != nil {
		return database.ExportStats{}, fmt.Errorf(
			"read SQL export rows: %w",
			err,
		)
	}
	if sink.rowsInBatch > 0 {
		if err := sink.write(";\n"); err != nil {
			return database.ExportStats{}, err
		}
	} else if err := sink.write(
		"-- No rows matched the export scope.\n",
	); err != nil {
		return database.ExportStats{}, err
	}
	if options.IncludeTransaction &&
		strings.TrimSpace(dialect.CommitStatement) != "" {
		if err := sink.write(
			"\n" + strings.TrimSpace(dialect.CommitStatement) + "\n",
		); err != nil {
			return database.ExportStats{}, err
		}
	}
	if err := database.CheckExportContext(ctx); err != nil {
		return database.ExportStats{}, err
	}
	if err := sink.writer.Flush(); err != nil {
		return database.ExportStats{}, err
	}
	return database.ExportStats{Rows: sink.rows}, nil
}
