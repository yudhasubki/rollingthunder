package oracle

import (
	"testing"

	"rollingthunder/pkg/database"
)

func TestOracleDependencyReferenceMapsMaterializedView(t *testing.T) {
	reference := oracleDependencyReference(
		"REPORTING",
		"ORDER_TOTALS",
		"MATERIALIZED VIEW",
	)
	if reference.Kind != database.ObjectKindMaterializedView ||
		reference.Schema != "REPORTING" ||
		reference.ID == "" {
		t.Fatalf("reference = %+v", reference)
	}
}

func TestOracleDependencySetDeduplicatesEdges(t *testing.T) {
	selected := database.ObjectReference{
		Kind:   database.ObjectKindView,
		Schema: "APP",
		Name:   "ORDER_VIEW",
	}
	set := newOracleDependencySet()
	dependency := database.ObjectDependency{
		Reference: database.ObjectReference{
			Kind:   database.ObjectKindTable,
			Schema: "APP",
			Name:   "ORDERS",
		},
	}
	set.add(dependency, selected)
	set.add(dependency, selected)
	if got := len(set.sorted()); got != 1 {
		t.Fatalf("dependency count = %d", got)
	}
}
