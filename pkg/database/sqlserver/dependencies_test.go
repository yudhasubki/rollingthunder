package sqlserver

import (
	"database/sql"
	"testing"

	"rollingthunder/pkg/database"
)

func TestSQLServerDependencySetDeduplicatesCatalogEdges(t *testing.T) {
	selected := database.ObjectReference{
		Kind:   database.ObjectKindView,
		Schema: "dbo",
		Name:   "order_summary",
	}
	set := newSQLServerDependencySet()
	dependency := database.ObjectDependency{
		Reference: database.ObjectReference{
			Kind:   database.ObjectKindTable,
			Schema: "dbo",
			Name:   "orders",
		},
		Description: "Referenced by SQL expression",
	}
	set.add(dependency, selected)
	set.add(dependency, selected)
	if got := len(set.sorted()); got != 1 {
		t.Fatalf("dependency count = %d", got)
	}
}

func TestSQLServerDependencyReferenceMapsRoutineKinds(t *testing.T) {
	reference := sqlServerDependencyReference(sqlServerExpressionDependencyRow{
		id:         sql.NullInt64{Int64: 42, Valid: true},
		schema:     "dbo",
		name:       "calculate_total",
		objectType: "FN",
	})
	if reference.Kind != database.ObjectKindFunction ||
		reference.ID != "sqlserver:object:NDI" {
		t.Fatalf("reference = %+v", reference)
	}
}
