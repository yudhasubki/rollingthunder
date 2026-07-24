package sqlite

import (
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

var sqliteDialect = database.Dialect{
	Name:                 "sqlite",
	IdentifierOpen:       `"`,
	IdentifierClose:      `"`,
	PlaceholderStyle:     database.PlaceholderQuestion,
	PaginationStyle:      database.PaginationLimitOffset,
	SupportsNullOrdering: true,
}

func (s *SQLite) Capabilities() database.Capabilities {
	return database.Capabilities{
		Engine:              "sqlite",
		DisplayName:         "SQLite",
		Dialect:             sqliteDialect,
		Schemas:             false,
		Databases:           true,
		Tables:              true,
		Views:               true,
		Triggers:            true,
		Constraints:         true,
		ObjectDefinitions:   true,
		ObjectDependencies:  true,
		ManageViews:         true,
		ManageTriggers:      true,
		TriggerToggle:       false,
		ManageIndexes:       true,
		AlterTableStructure: false,
		ExplainPlans:        true,
		Transactions:        true,
		TransactionalDDL:    true,
		AtomicTableChanges:  true,
		SQLInsertExport:     true,
		FileDatabase:        true,
		AttachedDatabases:   true,
		GeneratedColumns:    true,
		Upsert:              true,
	}
}

func quoteSQLiteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func quoteSQLiteQualifiedIdentifier(schema, name string) string {
	if strings.TrimSpace(schema) == "" {
		return quoteSQLiteIdentifier(name)
	}
	return quoteSQLiteIdentifier(schema) + "." + quoteSQLiteIdentifier(name)
}

func quoteSQLiteLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func (s *SQLite) QuoteIdentifier(identifier string) string {
	return quoteSQLiteIdentifier(identifier)
}

func (s *SQLite) Placeholder(_ int) string {
	return "?"
}

func (s *SQLite) PaginationClause(limit, offset int) (string, error) {
	if limit < 0 {
		return "", fmt.Errorf("pagination limit cannot be negative")
	}
	if offset < 0 {
		return "", fmt.Errorf("pagination offset cannot be negative")
	}
	return fmt.Sprintf("LIMIT %d OFFSET %d", limit, offset), nil
}

type sqliteQuery struct {
	SQL  string
	Args []interface{}
}

func buildSQLiteFilterClause(
	filters []database.Filter,
	structures database.Structures,
) (string, []interface{}, error) {
	if len(filters) == 0 {
		return "", nil, nil
	}
	available := make(map[string]struct{}, len(structures))
	for _, structure := range structures {
		available[structure.Name] = struct{}{}
	}
	conditions := make([]string, 0, len(filters))
	args := make([]interface{}, 0, len(filters))
	for _, filter := range filters {
		if err := filter.Validate(); err != nil {
			return "", nil, err
		}
		column := strings.TrimSpace(filter.Column)
		if _, exists := available[column]; !exists {
			return "", nil, fmt.Errorf(
				"cannot filter by unknown column %q",
				column,
			)
		}
		quoted := quoteSQLiteIdentifier(column)
		switch filter.Operator {
		case database.FilterEqual:
			conditions = append(conditions, quoted+" = ?")
			args = append(args, filter.Value)
		case database.FilterNotEqual:
			conditions = append(conditions, quoted+" <> ?")
			args = append(args, filter.Value)
		case database.FilterGreaterThan:
			conditions = append(conditions, quoted+" > ?")
			args = append(args, filter.Value)
		case database.FilterLessThan:
			conditions = append(conditions, quoted+" < ?")
			args = append(args, filter.Value)
		case database.FilterGreaterEqual:
			conditions = append(conditions, quoted+" >= ?")
			args = append(args, filter.Value)
		case database.FilterLessEqual:
			conditions = append(conditions, quoted+" <= ?")
			args = append(args, filter.Value)
		case database.FilterContains:
			conditions = append(
				conditions,
				"CAST("+quoted+" AS TEXT) LIKE ? COLLATE NOCASE",
			)
			args = append(args, "%"+fmt.Sprint(filter.Value)+"%")
		case database.FilterIsNull:
			conditions = append(conditions, quoted+" IS NULL")
		case database.FilterIsNotNull:
			conditions = append(conditions, quoted+" IS NOT NULL")
		}
	}
	return " WHERE " + strings.Join(conditions, " AND "), args, nil
}

func buildSQLiteOrderClause(
	sorts []database.Sort,
	structures database.Structures,
	hasRowID bool,
) (string, error) {
	available := make(map[string]database.Structure, len(structures))
	for _, structure := range structures {
		available[structure.Name] = structure
	}
	parts := make([]string, 0, len(sorts)+len(structures)+1)
	seen := make(map[string]struct{}, len(sorts))
	for _, sort := range sorts {
		column := strings.TrimSpace(sort.Column)
		if column == "" {
			return "", fmt.Errorf("sort column cannot be empty")
		}
		if _, exists := available[column]; !exists {
			return "", fmt.Errorf("cannot sort by unknown column %q", column)
		}
		if _, duplicate := seen[column]; duplicate {
			continue
		}
		direction := database.SortDirection(strings.ToLower(string(sort.Direction)))
		if direction == "" {
			direction = database.SortAscending
		}
		if direction != database.SortAscending &&
			direction != database.SortDescending {
			return "", fmt.Errorf(
				"invalid sort direction %q for column %q",
				sort.Direction,
				column,
			)
		}
		nulls := database.NullsPosition(strings.ToLower(string(sort.Nulls)))
		if nulls == "" {
			nulls = database.NullsLast
		}
		if nulls != database.NullsFirst && nulls != database.NullsLast {
			return "", fmt.Errorf(
				"invalid null position %q for column %q",
				sort.Nulls,
				column,
			)
		}
		parts = append(parts, fmt.Sprintf(
			"%s %s NULLS %s",
			quoteSQLiteIdentifier(column),
			strings.ToUpper(string(direction)),
			strings.ToUpper(string(nulls)),
		))
		seen[column] = struct{}{}
	}
	for _, structure := range structures {
		if !structure.IsPrimary {
			continue
		}
		if _, exists := seen[structure.Name]; exists {
			continue
		}
		parts = append(
			parts,
			quoteSQLiteIdentifier(structure.Name)+" ASC NULLS LAST",
		)
		seen[structure.Name] = struct{}{}
	}
	if hasRowID {
		parts = append(parts, "_rowid_ ASC")
	} else if len(parts) == 0 {
		for _, structure := range structures {
			parts = append(
				parts,
				quoteSQLiteIdentifier(structure.Name)+" ASC NULLS LAST",
			)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("table has no columns available for stable ordering")
	}
	return " ORDER BY " + strings.Join(parts, ", "), nil
}
