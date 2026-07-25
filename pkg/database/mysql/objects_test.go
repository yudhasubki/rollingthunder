package mysql

import (
	"testing"

	"rollingthunder/pkg/database"
)

func TestInferMySQLViewDependenciesSupportsMariaDBFallback(t *testing.T) {
	dependencies := inferMySQLViewDependencies(
		"select `orders`.`id` from `rolling`.`orders`",
		[]mysqlViewCandidateRow{
			{Schema: "rolling", Name: "orders", Type: "BASE TABLE"},
			{Schema: "rolling", Name: "orders_archive", Type: "BASE TABLE"},
			{Schema: "rolling", Name: "active_orders", Type: "VIEW"},
		},
	)
	if len(dependencies) != 1 {
		t.Fatalf("dependencies = %+v", dependencies)
	}
	reference := dependencies[0].Reference
	if reference.Kind != database.ObjectKindTable ||
		reference.Schema != "rolling" ||
		reference.Name != "orders" {
		t.Fatalf("dependency reference = %+v", reference)
	}
}

func TestMySQLViewCandidateKind(t *testing.T) {
	if kind := mysqlViewCandidateKind("SYSTEM VIEW"); kind != database.ObjectKindView {
		t.Fatalf("SYSTEM VIEW kind = %q", kind)
	}
	if kind := mysqlViewCandidateKind("BASE TABLE"); kind != database.ObjectKindTable {
		t.Fatalf("BASE TABLE kind = %q", kind)
	}
}
