package postgres

import (
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

type postgresQuery struct {
	SQL  string
	Args []interface{}
}

func buildPostgresFilterClause(
	filters []database.Filter,
	structures database.Structures,
	startPlaceholder int,
) (string, []interface{}, error) {
	if len(filters) == 0 {
		return "", nil, nil
	}
	if startPlaceholder < 1 {
		startPlaceholder = 1
	}

	availableColumns := make(map[string]struct{}, len(structures))
	for _, structure := range structures {
		availableColumns[structure.Name] = struct{}{}
	}

	conditions := make([]string, 0, len(filters))
	args := make([]interface{}, 0, len(filters))
	placeholder := startPlaceholder

	for _, filter := range filters {
		if err := filter.Validate(); err != nil {
			return "", nil, err
		}

		column := strings.TrimSpace(filter.Column)
		if _, exists := availableColumns[column]; !exists {
			return "", nil, fmt.Errorf(
				"cannot filter by unknown column %q",
				column,
			)
		}
		quotedColumn := quotePostgresIdentifier(column)

		switch filter.Operator {
		case database.FilterEqual:
			conditions = append(
				conditions,
				fmt.Sprintf("%s = $%d", quotedColumn, placeholder),
			)
			args = append(args, filter.Value)
			placeholder++
		case database.FilterNotEqual:
			conditions = append(
				conditions,
				fmt.Sprintf("%s <> $%d", quotedColumn, placeholder),
			)
			args = append(args, filter.Value)
			placeholder++
		case database.FilterGreaterThan:
			conditions = append(
				conditions,
				fmt.Sprintf("%s > $%d", quotedColumn, placeholder),
			)
			args = append(args, filter.Value)
			placeholder++
		case database.FilterLessThan:
			conditions = append(
				conditions,
				fmt.Sprintf("%s < $%d", quotedColumn, placeholder),
			)
			args = append(args, filter.Value)
			placeholder++
		case database.FilterGreaterEqual:
			conditions = append(
				conditions,
				fmt.Sprintf("%s >= $%d", quotedColumn, placeholder),
			)
			args = append(args, filter.Value)
			placeholder++
		case database.FilterLessEqual:
			conditions = append(
				conditions,
				fmt.Sprintf("%s <= $%d", quotedColumn, placeholder),
			)
			args = append(args, filter.Value)
			placeholder++
		case database.FilterContains:
			conditions = append(
				conditions,
				fmt.Sprintf("%s::text ILIKE $%d", quotedColumn, placeholder),
			)
			args = append(args, "%"+fmt.Sprint(filter.Value)+"%")
			placeholder++
		case database.FilterIsNull:
			conditions = append(conditions, quotedColumn+" IS NULL")
		case database.FilterIsNotNull:
			conditions = append(conditions, quotedColumn+" IS NOT NULL")
		}
	}

	return " WHERE " + strings.Join(conditions, " AND "), args, nil
}
