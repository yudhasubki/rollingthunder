package database

import (
	"fmt"
	"strings"
)

type PlaceholderStyle string

const (
	PlaceholderDollar   PlaceholderStyle = "dollar"
	PlaceholderQuestion PlaceholderStyle = "question"
)

type PaginationStyle string

const (
	PaginationLimitOffset PaginationStyle = "limit_offset"
)

// Dialect describes the SQL primitives that must be owned by a database
// driver. Keeping these rules out of the UI prevents identifiers and
// placeholders from being assembled with the wrong engine syntax.
type Dialect struct {
	Name                 string           `json:"name"`
	IdentifierOpen       string           `json:"identifierOpen"`
	IdentifierClose      string           `json:"identifierClose"`
	PlaceholderStyle     PlaceholderStyle `json:"placeholderStyle"`
	PaginationStyle      PaginationStyle  `json:"paginationStyle"`
	SupportsNullOrdering bool             `json:"supportsNullOrdering"`
}

// Capabilities is the feature contract advertised by a connected driver.
// A false value means the UI must not expose the corresponding workflow.
type Capabilities struct {
	Engine              string  `json:"engine"`
	DisplayName         string  `json:"displayName"`
	Dialect             Dialect `json:"dialect"`
	Schemas             bool    `json:"schemas"`
	Databases           bool    `json:"databases"`
	Tables              bool    `json:"tables"`
	Views               bool    `json:"views"`
	MaterializedViews   bool    `json:"materializedViews"`
	Functions           bool    `json:"functions"`
	Procedures          bool    `json:"procedures"`
	Triggers            bool    `json:"triggers"`
	Sequences           bool    `json:"sequences"`
	CustomTypes         bool    `json:"customTypes"`
	Domains             bool    `json:"domains"`
	Constraints         bool    `json:"constraints"`
	Extensions          bool    `json:"extensions"`
	ObjectDefinitions   bool    `json:"objectDefinitions"`
	ObjectDependencies  bool    `json:"objectDependencies"`
	ManageViews         bool    `json:"manageViews"`
	ManageRoutines      bool    `json:"manageRoutines"`
	ManageTriggers      bool    `json:"manageTriggers"`
	TriggerToggle       bool    `json:"triggerToggle"`
	ManageIndexes       bool    `json:"manageIndexes"`
	AlterTableStructure bool    `json:"alterTableStructure"`
	ExplainPlans        bool    `json:"explainPlans"`
	Transactions        bool    `json:"transactions"`
	TransactionalDDL    bool    `json:"transactionalDDL"`
	AtomicTableChanges  bool    `json:"atomicTableChanges"`
	SQLInsertExport     bool    `json:"sqlInsertExport"`
	FileDatabase        bool    `json:"fileDatabase"`
	AttachedDatabases   bool    `json:"attachedDatabases"`
	GeneratedColumns    bool    `json:"generatedColumns"`
	Upsert              bool    `json:"upsert"`
}

func (capabilities Capabilities) Validate() error {
	if strings.TrimSpace(capabilities.Engine) == "" {
		return fmt.Errorf("capability engine is required")
	}
	if strings.TrimSpace(capabilities.DisplayName) == "" {
		return fmt.Errorf("capability display name is required")
	}
	if strings.TrimSpace(capabilities.Dialect.Name) == "" {
		return fmt.Errorf("dialect name is required")
	}
	if capabilities.Dialect.IdentifierOpen == "" ||
		capabilities.Dialect.IdentifierClose == "" {
		return fmt.Errorf("dialect identifier quotes are required")
	}
	switch capabilities.Dialect.PlaceholderStyle {
	case PlaceholderDollar, PlaceholderQuestion:
	default:
		return fmt.Errorf(
			"unsupported placeholder style %q",
			capabilities.Dialect.PlaceholderStyle,
		)
	}
	switch capabilities.Dialect.PaginationStyle {
	case PaginationLimitOffset:
	default:
		return fmt.Errorf(
			"unsupported pagination style %q",
			capabilities.Dialect.PaginationStyle,
		)
	}
	if capabilities.MaterializedViews && !capabilities.Views {
		return fmt.Errorf("materialized views require view support")
	}
	if capabilities.ManageViews && !capabilities.Views {
		return fmt.Errorf("view management requires view support")
	}
	if capabilities.ManageRoutines &&
		!capabilities.Functions &&
		!capabilities.Procedures {
		return fmt.Errorf("routine management requires functions or procedures")
	}
	if capabilities.ManageTriggers && !capabilities.Triggers {
		return fmt.Errorf("trigger management requires trigger support")
	}
	if capabilities.TriggerToggle && !capabilities.ManageTriggers {
		return fmt.Errorf("trigger toggling requires trigger management support")
	}
	if capabilities.AttachedDatabases && !capabilities.Databases {
		return fmt.Errorf("attached databases require database support")
	}
	return nil
}

// CapabilityDriver is deliberately required by Driver. It is separated here
// so conformance tests can exercise the static dialect contract directly.
type CapabilityDriver interface {
	Capabilities() Capabilities
	QuoteIdentifier(identifier string) string
	Placeholder(position int) string
	PaginationClause(limit, offset int) (string, error)
}
