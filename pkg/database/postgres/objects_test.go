package postgres

import (
	"testing"

	"rollingthunder/pkg/database"
)

func TestParsePostgresObjectID(t *testing.T) {
	catalog, oid, err := parsePostgresObjectID("pg_proc:42")
	if err != nil {
		t.Fatalf("parse object ID: %v", err)
	}
	if catalog != "pg_proc" || oid != 42 {
		t.Fatalf("parsed ID = %s:%d", catalog, oid)
	}

	for _, invalid := range []string{
		"",
		"pg_proc",
		"pg_proc:0",
		"pg_proc:not-a-number",
		"pg_class; DROP TABLE users:1",
		"unknown:1",
	} {
		if _, _, err := parsePostgresObjectID(invalid); err == nil {
			t.Errorf("parsePostgresObjectID(%q) succeeded", invalid)
		}
	}
}

func TestPostgresObjectFromRowNormalizesRoutineAndParent(t *testing.T) {
	description := "Refreshes reporting data"
	object, err := postgresObjectFromRow(postgresObjectRow{
		ID:           "pg_proc:7",
		Kind:         "function",
		Schema:       "reporting",
		Name:         "refresh",
		Signature:    "integer, text",
		ParentSchema: "reporting",
		ParentName:   "jobs",
		Description:  &description,
	}, (&Postgres{}).Capabilities())
	if err != nil {
		t.Fatalf("normalize object: %v", err)
	}
	if object.DisplayName != "refresh(integer, text)" {
		t.Fatalf("display name = %q", object.DisplayName)
	}
	if !object.CanManage || object.CanOpenData {
		t.Fatalf("unexpected routine capabilities: %+v", object)
	}
	if object.Description != description || len(object.Properties) != 2 {
		t.Fatalf("unexpected object metadata: %+v", object)
	}
}

func TestPostgresViewDefinitionUsesCorrectRelationKind(t *testing.T) {
	view := database.DatabaseObject{Reference: database.ObjectReference{
		Kind:   database.ObjectKindView,
		Schema: "public",
		Name:   "active users",
	}}
	got := postgresViewDefinition(view, " SELECT * FROM users; ")
	want := "CREATE OR REPLACE VIEW \"public\".\"active users\" AS\nSELECT * FROM users;\n"
	if got != want {
		t.Fatalf("view definition:\n%s\nwant:\n%s", got, want)
	}

	view.Reference.Kind = database.ObjectKindMaterializedView
	got = postgresViewDefinition(view, "SELECT 1")
	want = "CREATE MATERIALIZED VIEW \"public\".\"active users\" AS\nSELECT 1;\n"
	if got != want {
		t.Fatalf("materialized view definition:\n%s\nwant:\n%s", got, want)
	}
}

func TestDependencyFromPostgresRowPreservesInspectableIdentity(t *testing.T) {
	dependency := dependencyFromPostgresRow(postgresDependencyRow{
		Catalog:     "pg_class",
		OID:         19,
		Type:        "table",
		Schema:      "public",
		Name:        "accounts",
		Identity:    "public.accounts",
		Description: "table public.accounts",
	})
	if dependency.Reference.Kind != database.ObjectKindTable ||
		dependency.Reference.ID != "pg_class:19" ||
		dependency.Reference.QualifiedName() != "public.accounts" {
		t.Fatalf("dependency = %+v", dependency)
	}
}
