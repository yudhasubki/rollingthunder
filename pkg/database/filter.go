package database

import (
	"fmt"
	"strings"
)

type FilterOperator string

const (
	FilterEqual        FilterOperator = "eq"
	FilterNotEqual     FilterOperator = "neq"
	FilterGreaterThan  FilterOperator = "gt"
	FilterLessThan     FilterOperator = "lt"
	FilterGreaterEqual FilterOperator = "gte"
	FilterLessEqual    FilterOperator = "lte"
	FilterContains     FilterOperator = "contains"
	FilterIsNull       FilterOperator = "is_null"
	FilterIsNotNull    FilterOperator = "is_not_null"
)

type Filter struct {
	Column   string
	Operator FilterOperator
	Value    interface{}
}

func (filter Filter) Validate() error {
	if strings.TrimSpace(filter.Column) == "" {
		return fmt.Errorf("filter column cannot be empty")
	}

	switch filter.Operator {
	case FilterEqual,
		FilterNotEqual,
		FilterGreaterThan,
		FilterLessThan,
		FilterGreaterEqual,
		FilterLessEqual,
		FilterContains:
		if filter.Value == nil {
			return fmt.Errorf(
				"filter %q requires a value",
				filter.Operator,
			)
		}
	case FilterIsNull, FilterIsNotNull:
	default:
		return fmt.Errorf("unsupported filter operator %q", filter.Operator)
	}

	return nil
}
