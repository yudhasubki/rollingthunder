package postgres

import (
	"fmt"
	"rollingthunder/pkg/database"
	"strings"
)

func quotePostgresIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func quotePostgresQualifiedIdentifier(schema, name string) string {
	return quotePostgresIdentifier(schema) + "." + quotePostgresIdentifier(name)
}

func buildPostgresOrderClause(
	sorts []database.Sort,
	structures database.Structures,
) (string, error) {
	availableColumns := make(map[string]database.Structure, len(structures))
	for _, structure := range structures {
		availableColumns[structure.Name] = structure
	}

	parts := make([]string, 0, len(sorts)+1)
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
		if direction != database.SortAscending && direction != database.SortDescending {
			return "", fmt.Errorf("invalid sort direction %q for column %q", sort.Direction, column)
		}

		nulls := database.NullsPosition(strings.ToLower(string(sort.Nulls)))
		if nulls == "" {
			nulls = database.NullsLast
		}
		if nulls != database.NullsFirst && nulls != database.NullsLast {
			return "", fmt.Errorf("invalid null position %q for column %q", sort.Nulls, column)
		}

		parts = append(
			parts,
			fmt.Sprintf(
				"%s %s NULLS %s",
				quotePostgresIdentifier(column),
				strings.ToUpper(string(direction)),
				strings.ToUpper(string(nulls)),
			),
		)
		seen[column] = struct{}{}
	}

	for _, structure := range structures {
		if !structure.IsPrimary {
			continue
		}
		if _, alreadySorted := seen[structure.Name]; alreadySorted {
			continue
		}
		parts = append(
			parts,
			fmt.Sprintf("%s ASC NULLS LAST", quotePostgresIdentifier(structure.Name)),
		)
		seen[structure.Name] = struct{}{}
	}

	// PostgreSQL tables without a primary key still need deterministic ordering
	// for offset pagination. tableoid + ctid is unique within the current table
	// snapshot, including inherited/partitioned storage, and is used only as an
	// internal tie-breaker.
	if len(parts) == 0 || !hasPrimaryKey(structures) {
		parts = append(parts, "tableoid ASC", "ctid ASC")
	}

	return " ORDER BY " + strings.Join(parts, ", "), nil
}

func hasPrimaryKey(structures database.Structures) bool {
	for _, structure := range structures {
		if structure.IsPrimary {
			return true
		}
	}
	return false
}
