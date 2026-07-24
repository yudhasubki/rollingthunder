package mysql

import (
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

var mysqlDialect = database.Dialect{
	Name:                 "mysql",
	IdentifierOpen:       "`",
	IdentifierClose:      "`",
	PlaceholderStyle:     database.PlaceholderQuestion,
	PaginationStyle:      database.PaginationLimitOffset,
	SupportsNullOrdering: false,
}

func (m *MySQL) Capabilities() database.Capabilities {
	return database.Capabilities{
		Engine:              "mysql",
		DisplayName:         "MySQL / MariaDB",
		Dialect:             mysqlDialect,
		Schemas:             false,
		Databases:           true,
		Tables:              true,
		Views:               true,
		Functions:           true,
		Procedures:          true,
		Triggers:            true,
		Constraints:         true,
		ObjectDefinitions:   true,
		ObjectDependencies:  true,
		ManageViews:         true,
		ManageRoutines:      true,
		ManageTriggers:      true,
		TriggerToggle:       false,
		ManageIndexes:       true,
		AlterTableStructure: true,
		ExplainPlans:        true,
		Transactions:        true,
		TransactionalDDL:    false,
		AtomicTableChanges:  true,
		SQLInsertExport:     true,
		GeneratedColumns:    true,
		Upsert:              true,
	}
}

func quoteMySQLIdentifier(identifier string) string {
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}

func quoteMySQLQualifiedIdentifier(databaseName, name string) string {
	if strings.TrimSpace(databaseName) == "" {
		return quoteMySQLIdentifier(name)
	}
	return quoteMySQLIdentifier(databaseName) + "." + quoteMySQLIdentifier(name)
}

func (m *MySQL) QuoteIdentifier(identifier string) string {
	return quoteMySQLIdentifier(identifier)
}

func (m *MySQL) Placeholder(_ int) string {
	return "?"
}

func (m *MySQL) PaginationClause(limit, offset int) (string, error) {
	if limit < 0 {
		return "", fmt.Errorf("pagination limit cannot be negative")
	}
	if offset < 0 {
		return "", fmt.Errorf("pagination offset cannot be negative")
	}
	return fmt.Sprintf("LIMIT %d OFFSET %d", limit, offset), nil
}

type mysqlQuery struct {
	SQL  string
	Args []interface{}
}

func buildMySQLFilterClause(
	filters []database.Filter,
	structures database.Structures,
) (string, []interface{}, error) {
	if len(filters) == 0 {
		return "", nil, nil
	}

	availableColumns := make(map[string]struct{}, len(structures))
	for _, structure := range structures {
		availableColumns[structure.Name] = struct{}{}
	}

	conditions := make([]string, 0, len(filters))
	args := make([]interface{}, 0, len(filters))
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
		quoted := quoteMySQLIdentifier(column)

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
				"CAST("+quoted+" AS CHAR) LIKE ?",
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

func buildMySQLOrderClause(
	sorts []database.Sort,
	structures database.Structures,
) (string, error) {
	availableColumns := make(map[string]database.Structure, len(structures))
	for _, structure := range structures {
		availableColumns[structure.Name] = structure
	}

	parts := make([]string, 0, len(sorts)*2+len(structures))
	seen := make(map[string]struct{}, len(sorts))
	for _, sort := range sorts {
		column := strings.TrimSpace(sort.Column)
		if column == "" {
			return "", fmt.Errorf("sort column cannot be empty")
		}
		if _, exists := availableColumns[column]; !exists {
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

		quoted := quoteMySQLIdentifier(column)
		nullDirection := "ASC"
		if nulls == database.NullsFirst {
			nullDirection = "DESC"
		}
		parts = append(
			parts,
			fmt.Sprintf("(%s IS NULL) %s", quoted, nullDirection),
			fmt.Sprintf("%s %s", quoted, strings.ToUpper(string(direction))),
		)
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
			fmt.Sprintf(
				"(%s IS NULL) ASC",
				quoteMySQLIdentifier(structure.Name),
			),
			quoteMySQLIdentifier(structure.Name)+" ASC",
		)
		seen[structure.Name] = struct{}{}
	}

	// InnoDB does not expose a safe physical row identifier. A complete
	// column ordering is the most stable fallback available for offset
	// pagination on a table without a primary key.
	if len(parts) == 0 {
		for _, structure := range structures {
			parts = append(
				parts,
				fmt.Sprintf(
					"(%s IS NULL) ASC",
					quoteMySQLIdentifier(structure.Name),
				),
				quoteMySQLIdentifier(structure.Name)+" ASC",
			)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("table has no columns available for stable ordering")
	}
	return " ORDER BY " + strings.Join(parts, ", "), nil
}
