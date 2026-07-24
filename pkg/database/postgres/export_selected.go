package postgres

import (
	"fmt"

	"rollingthunder/pkg/database"
)

type selectedRowStream struct {
	rows     database.RowStream
	selected map[int]struct{}
	index    int
	matched  int
}

func newSelectedRowStream(
	rows database.RowStream,
	indexes []int,
	pageLimit int,
) (*selectedRowStream, error) {
	if len(indexes) == 0 {
		return nil, fmt.Errorf("selected-row export requires at least one row")
	}

	selected := make(map[int]struct{}, len(indexes))
	for _, index := range indexes {
		if index < 0 {
			return nil, fmt.Errorf("selected row index cannot be negative")
		}
		if pageLimit > 0 && index >= pageLimit {
			return nil, fmt.Errorf(
				"selected row index %d is outside the current page",
				index,
			)
		}
		selected[index] = struct{}{}
	}

	return &selectedRowStream{
		rows:     rows,
		selected: selected,
	}, nil
}

func (rows *selectedRowStream) Columns() ([]string, error) {
	return rows.rows.Columns()
}

func (rows *selectedRowStream) Next() bool {
	if rows.matched == len(rows.selected) {
		return false
	}
	for rows.rows.Next() {
		index := rows.index
		rows.index++
		if _, selected := rows.selected[index]; selected {
			rows.matched++
			return true
		}
	}
	return false
}

func (rows *selectedRowStream) Values() ([]interface{}, error) {
	return rows.rows.Values()
}

func (rows *selectedRowStream) Err() error {
	if err := rows.rows.Err(); err != nil {
		return err
	}
	if rows.matched != len(rows.selected) {
		return fmt.Errorf(
			"only %d of %d selected rows still exist on the current page",
			rows.matched,
			len(rows.selected),
		)
	}
	return nil
}
