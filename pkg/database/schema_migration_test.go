package database

import "testing"

func TestSchemaMigrationRequestValidation(t *testing.T) {
	valid := SchemaMigrationRequest{
		SourceConnectionID: "source",
		SourceSchema:       "public",
		TargetConnectionID: "target",
		TargetSchema:       "public",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}

	same := valid
	same.TargetConnectionID = same.SourceConnectionID
	if err := same.Validate(); err == nil {
		t.Fatal("same source and target should be rejected")
	}
}

func TestSchemaMigrationFingerprintIncludesSelection(t *testing.T) {
	request := SchemaMigrationRequest{
		SourceConnectionID: "source",
		SourceSchema:       "public",
		TargetConnectionID: "target",
		TargetSchema:       "public",
	}
	change := SchemaMigrationChange{
		ID:         "create_table:public.users",
		Selected:   true,
		Supported:  true,
		Statements: []string{`CREATE TABLE "users" ("id" integer);`},
	}
	first := SchemaMigrationFingerprint(request, "postgres", []SchemaMigrationChange{change})
	change.Selected = false
	second := SchemaMigrationFingerprint(request, "postgres", []SchemaMigrationChange{change})
	if first == second {
		t.Fatal("selection change should invalidate the reviewed fingerprint")
	}
}
