package postgres

import (
	"fmt"

	"rollingthunder/pkg/database"
)

var postgresDialect = database.Dialect{
	Name:                 "postgresql",
	IdentifierOpen:       `"`,
	IdentifierClose:      `"`,
	PlaceholderStyle:     database.PlaceholderDollar,
	PaginationStyle:      database.PaginationLimitOffset,
	SupportsNullOrdering: true,
}

func (p *Postgres) Capabilities() database.Capabilities {
	return database.Capabilities{
		Engine:              "postgres",
		DisplayName:         "PostgreSQL",
		Dialect:             postgresDialect,
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
		Domains:             true,
		Constraints:         true,
		Extensions:          true,
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
		Upsert:              true,
		ManageSecurity:      true,
		ActivityMonitor:     true,
		SSHConnections:      true,
	}
}

func (p *Postgres) QuoteIdentifier(identifier string) string {
	return quotePostgresIdentifier(identifier)
}

func (p *Postgres) Placeholder(position int) string {
	if position < 1 {
		position = 1
	}
	return fmt.Sprintf("$%d", position)
}

func (p *Postgres) PaginationClause(limit, offset int) (string, error) {
	if limit < 0 {
		return "", fmt.Errorf("pagination limit cannot be negative")
	}
	if offset < 0 {
		return "", fmt.Errorf("pagination offset cannot be negative")
	}
	return fmt.Sprintf("LIMIT %d OFFSET %d", limit, offset), nil
}
