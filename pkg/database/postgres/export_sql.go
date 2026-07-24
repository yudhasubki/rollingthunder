package postgres

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"rollingthunder/pkg/database"
)

func postgresInsertableColumns(columns Columns) Columns {
	insertable := make(Columns, 0, len(columns))
	for _, column := range columns {
		if strings.EqualFold(column.IsGenerated, "ALWAYS") {
			continue
		}
		insertable = append(insertable, column)
	}
	return insertable
}

func postgresSQLLiteralProjection(columns Columns) (string, error) {
	if len(columns) == 0 {
		return "", fmt.Errorf("table has no columns that can be exported as SQL INSERT")
	}

	projection := make([]string, 0, len(columns))
	for _, column := range columns {
		if strings.TrimSpace(column.ColumnName) == "" {
			return "", fmt.Errorf("SQL INSERT export encountered an unnamed column")
		}

		identifier := quotePostgresIdentifier(column.ColumnName)
		projection = append(
			projection,
			fmt.Sprintf(
				"pg_catalog.quote_nullable(%s) AS %s",
				identifier,
				identifier,
			),
		)
	}
	return strings.Join(projection, ", "), nil
}

func postgresHasAlwaysIdentity(columns Columns) bool {
	for _, column := range columns {
		if !strings.EqualFold(column.IsIdentity, "YES") ||
			column.IdentityMode == nil {
			continue
		}
		if strings.EqualFold(*column.IdentityMode, "ALWAYS") {
			return true
		}
	}
	return false
}

type postgresInsertSink struct {
	writer          *bufio.Writer
	statementPrefix string
	batchSize       int
	rowsInBatch     int
	rows            int64
}

func newPostgresInsertSink(
	writer io.Writer,
	table database.Table,
	columns Columns,
	options database.SQLInsertOptions,
) (*postgresInsertSink, error) {
	if len(columns) == 0 {
		return nil, fmt.Errorf("table has no columns that can be exported as SQL INSERT")
	}

	columnNames := make([]string, 0, len(columns))
	for _, column := range columns {
		columnNames = append(columnNames, quotePostgresIdentifier(column.ColumnName))
	}

	prefix := fmt.Sprintf(
		"INSERT INTO %s (%s)",
		quotePostgresQualifiedIdentifier(table.Schema, table.Name),
		strings.Join(columnNames, ", "),
	)
	if postgresHasAlwaysIdentity(columns) {
		prefix += " OVERRIDING SYSTEM VALUE"
	}
	prefix += " VALUES\n"

	return &postgresInsertSink{
		writer:          bufio.NewWriter(writer),
		statementPrefix: prefix,
		batchSize:       options.EffectiveBatchSize(),
	}, nil
}

func (sink *postgresInsertSink) writeString(value string) error {
	written, err := sink.writer.WriteString(value)
	if err != nil {
		return err
	}
	if written != len(value) {
		return io.ErrShortWrite
	}
	return nil
}

func postgresQuotedLiteral(value interface{}) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "NULL", nil
	case string:
		return typed, nil
	case []byte:
		return string(typed), nil
	default:
		return "", fmt.Errorf(
			"quoted SQL value has unexpected type %T",
			value,
		)
	}
}

func (sink *postgresInsertSink) writeValues(values []interface{}) error {
	literals := make([]string, len(values))
	for index, value := range values {
		literal, err := postgresQuotedLiteral(value)
		if err != nil {
			return err
		}
		literals[index] = literal
	}

	if sink.rowsInBatch == sink.batchSize {
		if err := sink.writeString(";\n\n"); err != nil {
			return err
		}
		sink.rowsInBatch = 0
	}
	if sink.rowsInBatch == 0 {
		if err := sink.writeString(sink.statementPrefix); err != nil {
			return err
		}
	} else if err := sink.writeString(",\n"); err != nil {
		return err
	}
	if err := sink.writeString("  (" + strings.Join(literals, ", ") + ")"); err != nil {
		return err
	}

	sink.rowsInBatch++
	sink.rows++
	return nil
}

func (sink *postgresInsertSink) close(includeTransaction bool) error {
	if sink.rowsInBatch > 0 {
		if err := sink.writeString(";\n"); err != nil {
			return err
		}
	} else if sink.rows == 0 {
		if err := sink.writeString("-- No rows matched the export scope.\n"); err != nil {
			return err
		}
	}
	if includeTransaction {
		if err := sink.writeString("\nCOMMIT;\n"); err != nil {
			return err
		}
	}
	return sink.writer.Flush()
}

func writePostgresInsertStream(
	writer io.Writer,
	rows database.RowStream,
	table database.Table,
	columns Columns,
	options database.SQLInsertOptions,
) (database.ExportStats, error) {
	return writePostgresInsertStreamContext(
		context.Background(),
		writer,
		rows,
		table,
		columns,
		options,
	)
}

func writePostgresInsertStreamContext(
	ctx context.Context,
	writer io.Writer,
	rows database.RowStream,
	table database.Table,
	columns Columns,
	options database.SQLInsertOptions,
) (database.ExportStats, error) {
	if err := database.CheckExportContext(ctx); err != nil {
		return database.ExportStats{}, err
	}
	streamColumns, err := rows.Columns()
	if err != nil {
		return database.ExportStats{}, fmt.Errorf("read SQL export columns: %w", err)
	}
	if len(streamColumns) != len(columns) {
		return database.ExportStats{}, fmt.Errorf(
			"SQL export returned %d columns for %d insert columns",
			len(streamColumns),
			len(columns),
		)
	}
	for index, column := range columns {
		if streamColumns[index] != column.ColumnName {
			return database.ExportStats{}, fmt.Errorf(
				"SQL export column %d is %q, expected %q",
				index,
				streamColumns[index],
				column.ColumnName,
			)
		}
	}

	sink, err := newPostgresInsertSink(writer, table, columns, options)
	if err != nil {
		return database.ExportStats{}, err
	}
	if err := sink.writeString(
		"-- Rolling Thunder PostgreSQL INSERT export\n" +
			"-- Generated columns are omitted; sequence state is not modified.\n\n",
	); err != nil {
		return database.ExportStats{}, fmt.Errorf("start SQL export: %w", err)
	}
	if options.IncludeTransaction {
		if err := sink.writeString("BEGIN;\n\n"); err != nil {
			return database.ExportStats{}, fmt.Errorf("start SQL transaction: %w", err)
		}
	}

	for rows.Next() {
		if err := database.CheckExportContext(ctx); err != nil {
			return database.ExportStats{}, err
		}
		values, err := rows.Values()
		if err != nil {
			return database.ExportStats{}, fmt.Errorf("read SQL export row: %w", err)
		}
		if len(values) != len(columns) {
			return database.ExportStats{}, fmt.Errorf(
				"SQL export row has %d values for %d columns",
				len(values),
				len(columns),
			)
		}
		if err := sink.writeValues(values); err != nil {
			return database.ExportStats{}, fmt.Errorf("write SQL export row: %w", err)
		}
		database.ReportExportProgress(ctx, sink.rows)
	}
	if err := rows.Err(); err != nil {
		return database.ExportStats{}, fmt.Errorf("read SQL export rows: %w", err)
	}
	if err := database.CheckExportContext(ctx); err != nil {
		return database.ExportStats{}, err
	}
	if err := sink.close(options.IncludeTransaction); err != nil {
		return database.ExportStats{}, fmt.Errorf("finish SQL export: %w", err)
	}

	return database.ExportStats{Rows: sink.rows}, nil
}
