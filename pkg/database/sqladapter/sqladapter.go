package sqladapter

import (
	"context"
	"database/sql"
	"fmt"

	"rollingthunder/pkg/database"
)

type QueryRunner interface {
	QueryContext(
		context.Context,
		string,
		...interface{},
	) (*sql.Rows, error)
}

func scanCurrentRow(
	rows *sql.Rows,
	columns []string,
) (map[string]interface{}, error) {
	values := make([]interface{}, len(columns))
	destinations := make([]interface{}, len(columns))
	for index := range values {
		destinations[index] = &values[index]
	}
	if err := rows.Scan(destinations...); err != nil {
		return nil, err
	}
	result := make(map[string]interface{}, len(columns))
	for index, column := range columns {
		value := values[index]
		if raw, ok := value.(sql.RawBytes); ok {
			value = append([]byte(nil), raw...)
		}
		result[column] = value
	}
	return result, nil
}

func ExecuteQuery(
	ctx context.Context,
	runner QueryRunner,
	query string,
	options database.QueryOptions,
) (database.QueryResult, error) {
	rows, err := runner.QueryContext(ctx, query, options.Args...)
	if err != nil {
		return database.QueryResult{}, err
	}
	defer rows.Close()

	result := database.QueryResult{
		Rows:       make([]map[string]interface{}, 0),
		ResultSets: make([]database.QueryResultSet, 0),
		RowLimit:   options.MaxRows,
	}
	resultIndex := 0
	for {
		columns, columnErr := rows.Columns()
		if columnErr != nil {
			return database.QueryResult{}, fmt.Errorf(
				"read query result columns: %w",
				columnErr,
			)
		}
		current := database.QueryResultSet{
			Index:    resultIndex,
			Columns:  append([]string(nil), columns...),
			Rows:     make([]map[string]interface{}, 0),
			RowLimit: options.MaxRows,
		}
		for rows.Next() {
			if options.MaxRows > 0 && len(current.Rows) >= options.MaxRows {
				current.Truncated = true
				break
			}
			row, scanErr := scanCurrentRow(rows, columns)
			if scanErr != nil {
				return database.QueryResult{}, fmt.Errorf(
					"scan query result row: %w",
					scanErr,
				)
			}
			current.Rows = append(current.Rows, row)
		}
		if rowErr := rows.Err(); rowErr != nil {
			return database.QueryResult{}, fmt.Errorf(
				"read query result rows: %w",
				rowErr,
			)
		}
		result.ResultSets = append(result.ResultSets, current)
		if resultIndex == 0 {
			result.Columns = append([]string(nil), current.Columns...)
			result.Rows = append(result.Rows, current.Rows...)
			result.Truncated = current.Truncated
		}
		resultIndex++
		if !rows.NextResultSet() {
			break
		}
	}
	result.StatementCount = len(result.ResultSets)
	return result, nil
}

type Rows struct {
	rows    *sql.Rows
	columns []string
	values  []interface{}
	err     error
}

func NewRows(rows *sql.Rows) *Rows {
	return &Rows{rows: rows}
}

func (rows *Rows) Columns() ([]string, error) {
	if rows.columns != nil || rows.err != nil {
		return append([]string(nil), rows.columns...), rows.err
	}
	rows.columns, rows.err = rows.rows.Columns()
	return append([]string(nil), rows.columns...), rows.err
}

func (rows *Rows) Next() bool {
	return rows.rows.Next()
}

func (rows *Rows) Values() ([]interface{}, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	values := make([]interface{}, len(columns))
	destinations := make([]interface{}, len(columns))
	for index := range values {
		destinations[index] = &values[index]
	}
	if err := rows.rows.Scan(destinations...); err != nil {
		rows.err = err
		return nil, err
	}
	for index, value := range values {
		if raw, ok := value.(sql.RawBytes); ok {
			values[index] = append([]byte(nil), raw...)
		}
	}
	rows.values = values
	return append([]interface{}(nil), values...), nil
}

func (rows *Rows) Err() error {
	if rows.err != nil {
		return rows.err
	}
	return rows.rows.Err()
}

type SelectedRows struct {
	source   database.RowStream
	selected map[int]struct{}
	index    int
	current  []interface{}
	err      error
}

func NewSelectedRows(
	source database.RowStream,
	indexes []int,
	pageSize int,
) (*SelectedRows, error) {
	if pageSize <= 0 {
		return nil, fmt.Errorf("selected export requires a positive page size")
	}
	if len(indexes) == 0 {
		return nil, fmt.Errorf(
			"selected export requires at least one row",
		)
	}
	selected := make(map[int]struct{}, len(indexes))
	for _, index := range indexes {
		if index < 0 || index >= pageSize {
			return nil, fmt.Errorf("selected row index %d is outside the current page", index)
		}
		selected[index] = struct{}{}
	}
	return &SelectedRows{source: source, selected: selected}, nil
}

func (rows *SelectedRows) Columns() ([]string, error) {
	return rows.source.Columns()
}

func (rows *SelectedRows) Next() bool {
	for rows.source.Next() {
		values, err := rows.source.Values()
		if err != nil {
			rows.err = err
			return false
		}
		index := rows.index
		rows.index++
		if _, selected := rows.selected[index]; !selected {
			continue
		}
		rows.current = values
		return true
	}
	return false
}

func (rows *SelectedRows) Values() ([]interface{}, error) {
	if rows.err != nil {
		return nil, rows.err
	}
	return append([]interface{}(nil), rows.current...), nil
}

func (rows *SelectedRows) Err() error {
	if rows.err != nil {
		return rows.err
	}
	return rows.source.Err()
}
