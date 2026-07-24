package postgres

import (
	"reflect"
	"testing"

	"rollingthunder/pkg/database"
)

func TestBuildPostgresInsertMutationOmitsGeneratedDefaults(t *testing.T) {
	defaultValue := "nextval('events_id_seq'::regclass)"
	mutation, err := buildPostgresInsertMutation(
		database.Table{Schema: "public", Name: "events"},
		map[string]interface{}{
			"id":    nil,
			"label": "created",
		},
		database.Structures{
			{
				Name:      "id",
				IsPrimary: true,
				IsAutoInc: true,
				Default:   &defaultValue,
			},
			{Name: "label"},
		},
	)
	if err != nil {
		t.Fatalf("build insert mutation: %v", err)
	}

	const expected = `INSERT INTO "public"."events" ("label") VALUES ($1)`
	if mutation.SQL != expected {
		t.Fatalf("SQL = %q, want %q", mutation.SQL, expected)
	}
	if !reflect.DeepEqual(mutation.Args, []interface{}{"created"}) {
		t.Fatalf("args = %#v", mutation.Args)
	}
}

func TestBuildPostgresUpdateMutationUsesOriginalCompositeKey(t *testing.T) {
	mutation, err := buildPostgresUpdateMutation(
		database.Table{Schema: "tenant", Name: "memberships"},
		database.RowUpdate{
			Original: map[string]interface{}{
				"tenant_id": 4,
				"user_id":   9,
				"role":      "viewer",
			},
			Values: map[string]interface{}{
				"tenant_id": 4,
				"user_id":   9,
				"role":      "admin",
			},
			ChangedColumns: []string{"role"},
		},
		database.Structures{
			{Name: "tenant_id", IsPrimary: true},
			{Name: "user_id", IsPrimary: true},
			{Name: "role"},
		},
		[]string{"tenant_id", "user_id"},
	)
	if err != nil {
		t.Fatalf("build update mutation: %v", err)
	}

	const expected = `UPDATE "tenant"."memberships" SET "role" = $1 WHERE "tenant_id" = $2 AND "user_id" = $3`
	if mutation.SQL != expected {
		t.Fatalf("SQL = %q, want %q", mutation.SQL, expected)
	}
	if !reflect.DeepEqual(
		mutation.Args,
		[]interface{}{"admin", 4, 9},
	) {
		t.Fatalf("args = %#v", mutation.Args)
	}
}

func TestBuildPostgresDeleteMutationUsesEveryPrimaryKey(t *testing.T) {
	mutation, err := buildPostgresDeleteMutation(
		database.Table{Schema: "tenant", Name: "memberships"},
		map[string]interface{}{"tenant_id": 4, "user_id": 9},
		[]string{"tenant_id", "user_id"},
	)
	if err != nil {
		t.Fatalf("build delete mutation: %v", err)
	}

	const expected = `DELETE FROM "tenant"."memberships" WHERE "tenant_id" = $1 AND "user_id" = $2`
	if mutation.SQL != expected {
		t.Fatalf("SQL = %q, want %q", mutation.SQL, expected)
	}
	if !reflect.DeepEqual(mutation.Args, []interface{}{4, 9}) {
		t.Fatalf("args = %#v", mutation.Args)
	}
}

func TestPostgresMutationsRejectUnsafeRowIdentity(t *testing.T) {
	_, updateErr := buildPostgresUpdateMutation(
		database.Table{Schema: "public", Name: "logs"},
		database.RowUpdate{
			Original:       map[string]interface{}{"message": "before"},
			Values:         map[string]interface{}{"message": "after"},
			ChangedColumns: []string{"message"},
		},
		database.Structures{{Name: "message"}},
		nil,
	)
	if updateErr == nil {
		t.Fatal("expected update without primary key to fail")
	}

	_, deleteErr := buildPostgresDeleteMutation(
		database.Table{Schema: "public", Name: "logs"},
		map[string]interface{}{"message": "before"},
		nil,
	)
	if deleteErr == nil {
		t.Fatal("expected delete without primary key to fail")
	}
}
