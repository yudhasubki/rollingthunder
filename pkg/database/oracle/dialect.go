package oracle

import (
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/database/sqladapter"
)

var oracleDialect = database.Dialect{
	Name:                 "oracle",
	IdentifierOpen:       `"`,
	IdentifierClose:      `"`,
	PlaceholderStyle:     database.PlaceholderColon,
	PaginationStyle:      database.PaginationOffsetFetch,
	SupportsNullOrdering: true,
}

func (o *Oracle) Capabilities() database.Capabilities {
	return database.Capabilities{
		Engine:              database.DriverOracle,
		DisplayName:         "Oracle Database",
		Dialect:             oracleDialect,
		Schemas:             true,
		Databases:           false,
		Tables:              true,
		Views:               true,
		MaterializedViews:   true,
		Functions:           true,
		Procedures:          true,
		Triggers:            true,
		Sequences:           true,
		CustomTypes:         true,
		Constraints:         true,
		ObjectDefinitions:   true,
		ObjectDependencies:  false,
		ManageViews:         true,
		ManageRoutines:      true,
		ManageTriggers:      true,
		TriggerToggle:       true,
		ManageIndexes:       true,
		AlterTableStructure: true,
		ExplainPlans:        true,
		Transactions:        true,
		TransactionalDDL:    false,
		AtomicTableChanges:  true,
		SQLInsertExport:     true,
		GeneratedColumns:    true,
		Upsert:              false,
		ManageSecurity:      false,
		ActivityMonitor:     false,
		SSHConnections:      true,
	}
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func quoteQualified(schema, name string) string {
	if strings.TrimSpace(schema) == "" {
		return quoteIdentifier(name)
	}
	return quoteIdentifier(schema) + "." + quoteIdentifier(name)
}

func (o *Oracle) QuoteIdentifier(identifier string) string {
	return quoteIdentifier(identifier)
}

func (o *Oracle) Placeholder(position int) string {
	if position < 1 {
		position = 1
	}
	return fmt.Sprintf(":%d", position)
}

func (o *Oracle) PaginationClause(limit, offset int) (string, error) {
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

func (o *Oracle) adapterDialect() sqladapter.Dialect {
	return sqladapter.Dialect{
		QuoteIdentifier:            quoteIdentifier,
		QuoteQualified:             quoteQualified,
		Placeholder:                o.Placeholder,
		Pagination:                 o.PaginationClause,
		RequiresOrderForPagination: true,
		PaginationFallbackOrder:    " ORDER BY ROWID",
		SupportsNullOrdering:       true,
		TextExpression: func(identifier string) string {
			return "TO_CHAR(" + identifier + ")"
		},
		InsertExport: oracleInsertExportDialect(),
	}
}
