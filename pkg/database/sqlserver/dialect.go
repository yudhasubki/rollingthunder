package sqlserver

import (
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/database/sqladapter"
)

var sqlServerDialect = database.Dialect{
	Name:                 "sqlserver",
	IdentifierOpen:       "[",
	IdentifierClose:      "]",
	PlaceholderStyle:     database.PlaceholderAt,
	PaginationStyle:      database.PaginationOffsetFetch,
	SupportsNullOrdering: false,
}

func (s *SQLServer) Capabilities() database.Capabilities {
	return database.Capabilities{
		Engine:              database.DriverSQLServer,
		DisplayName:         "Microsoft SQL Server",
		Dialect:             sqlServerDialect,
		Schemas:             true,
		Databases:           false,
		Tables:              true,
		Views:               true,
		Functions:           true,
		Procedures:          true,
		Triggers:            true,
		Sequences:           true,
		CustomTypes:         true,
		Constraints:         true,
		ObjectDefinitions:   true,
		ObjectDependencies:  true,
		ManageViews:         true,
		ManageRoutines:      true,
		ManageTriggers:      true,
		TriggerToggle:       true,
		ManageIndexes:       true,
		AlterTableStructure: true,
		ExplainPlans:        true,
		Transactions:        true,
		TransactionalDDL:    true,
		AtomicTableChanges:  true,
		SQLInsertExport:     true,
		GeneratedColumns:    true,
		Upsert:              false,
		ManageSecurity:      true,
		ActivityMonitor:     true,
		SSHConnections:      true,
	}
}

func quoteIdentifier(identifier string) string {
	return "[" + strings.ReplaceAll(identifier, "]", "]]") + "]"
}

func quoteQualified(schema, name string) string {
	if strings.TrimSpace(schema) == "" {
		return quoteIdentifier(name)
	}
	return quoteIdentifier(schema) + "." + quoteIdentifier(name)
}

func (s *SQLServer) QuoteIdentifier(identifier string) string {
	return quoteIdentifier(identifier)
}

func (s *SQLServer) Placeholder(position int) string {
	if position < 1 {
		position = 1
	}
	return fmt.Sprintf("@p%d", position)
}

func (s *SQLServer) PaginationClause(limit, offset int) (string, error) {
	if limit < 0 {
		return "", fmt.Errorf("pagination limit cannot be negative")
	}
	if offset < 0 {
		return "", fmt.Errorf("pagination offset cannot be negative")
	}
	return fmt.Sprintf(
		"OFFSET %d ROWS FETCH NEXT %d ROWS ONLY",
		offset,
		limit,
	), nil
}

func (s *SQLServer) adapterDialect() sqladapter.Dialect {
	return sqladapter.Dialect{
		QuoteIdentifier:            quoteIdentifier,
		QuoteQualified:             quoteQualified,
		Placeholder:                s.Placeholder,
		Pagination:                 s.PaginationClause,
		RequiresOrderForPagination: true,
		PaginationFallbackOrder:    " ORDER BY (SELECT NULL)",
		SupportsNullOrdering:       false,
		TextExpression: func(identifier string) string {
			return "CONVERT(nvarchar(max), " + identifier + ")"
		},
		NullOrderExpression: func(
			identifier string,
			position database.NullsPosition,
		) string {
			nullRank, valueRank := 1, 0
			if position == database.NullsFirst {
				nullRank, valueRank = 0, 1
			}
			return fmt.Sprintf(
				"CASE WHEN %s IS NULL THEN %d ELSE %d END ASC",
				identifier,
				nullRank,
				valueRank,
			)
		},
		InsertExport: sqlServerInsertExportDialect(),
		IdentityInsertStatements: func(table database.Table) (string, string) {
			target := quoteQualified(table.Schema, table.Name)
			return "SET IDENTITY_INSERT " + target + " ON",
				"SET IDENTITY_INSERT " + target + " OFF"
		},
	}
}
