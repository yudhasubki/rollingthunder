package postgres

import (
	"context"
	"strings"
	"testing"

	"rollingthunder/pkg/database"
)

func TestBuildPostgresViewChanges(t *testing.T) {
	driver := &Postgres{}
	create, err := driver.BuildObjectChange(context.Background(), database.ObjectChangeRequest{
		Action: database.ObjectChangeCreate,
		Reference: database.ObjectReference{
			Kind:   database.ObjectKindView,
			Schema: "reporting",
			Name:   "active users",
		},
		Definition: "SELECT * FROM public.users WHERE active",
	})
	if err != nil {
		t.Fatalf("build create view: %v", err)
	}
	want := "CREATE VIEW \"reporting\".\"active users\" AS\nSELECT * FROM public.users WHERE active;"
	if len(create.Statements) != 1 || create.Statements[0] != want {
		t.Fatalf("create view SQL = %#v", create.Statements)
	}

	replace, err := driver.BuildObjectChange(context.Background(), database.ObjectChangeRequest{
		Action: database.ObjectChangeReplace,
		Reference: database.ObjectReference{
			Kind:   database.ObjectKindView,
			Schema: "reporting",
			Name:   "active_users",
		},
		Definition: "WITH users AS (SELECT * FROM public.users) SELECT * FROM users",
	})
	if err != nil {
		t.Fatalf("build replace view: %v", err)
	}
	if !strings.HasPrefix(replace.Statements[0], "CREATE OR REPLACE VIEW") {
		t.Fatalf("replace view SQL = %q", replace.Statements[0])
	}
}

func TestReviewedPostgresRoutineAllowsDollarQuotedBodyOnly(t *testing.T) {
	definition := `
		CREATE OR REPLACE FUNCTION public.answer()
		RETURNS integer
		LANGUAGE plpgsql
		AS $body$
		BEGIN
			RETURN 42;
		END;
		$body$;
	`
	got, err := reviewedDDL(definition, database.ObjectKindFunction)
	if err != nil {
		t.Fatalf("review routine DDL: %v", err)
	}
	if !strings.HasSuffix(got, ";") {
		t.Fatalf("reviewed DDL has no terminator: %q", got)
	}
	if _, err := reviewedDDL(
		definition+" DROP TABLE public.users;",
		database.ObjectKindFunction,
	); err == nil {
		t.Fatal("multiple top-level routine statements were accepted")
	}
}

func TestBuildPostgresRenameAndTriggerChanges(t *testing.T) {
	routine, err := postgresRenameStatement(database.ObjectReference{
		Kind:      database.ObjectKindFunction,
		Schema:    "public",
		Name:      "calculate",
		Signature: "numeric, numeric(10, 2)",
	}, "calculate_total")
	if err != nil {
		t.Fatalf("rename routine: %v", err)
	}
	want := `ALTER FUNCTION "public"."calculate"(numeric, numeric(10, 2)) RENAME TO "calculate_total";`
	if routine != want {
		t.Fatalf("routine rename = %q, want %q", routine, want)
	}

	driver := &Postgres{}
	trigger, err := driver.BuildObjectChange(context.Background(), database.ObjectChangeRequest{
		Action: database.ObjectChangeDisable,
		Reference: database.ObjectReference{
			Kind:         database.ObjectKindTrigger,
			Schema:       "audit",
			Name:         "capture_update",
			ParentSchema: "public",
			ParentName:   "orders",
		},
	})
	if err != nil {
		t.Fatalf("disable trigger: %v", err)
	}
	want = `ALTER TABLE "public"."orders" DISABLE TRIGGER "capture_update";`
	if trigger.Statements[0] != want {
		t.Fatalf("trigger SQL = %q, want %q", trigger.Statements[0], want)
	}
}

func TestBuildPostgresIndexChangeValidatesMethodAndPredicate(t *testing.T) {
	statement, err := buildPostgresIndexChange(database.IndexChange{
		Table:   database.Table{Schema: "public", Name: "orders"},
		Name:    "orders_customer_created_idx",
		Columns: []string{"customer_id", "created_at"},
		Unique:  true,
		Method:  "btree",
		Where:   "deleted_at IS NULL",
	})
	if err != nil {
		t.Fatalf("build index: %v", err)
	}
	want := `CREATE UNIQUE INDEX "orders_customer_created_idx" ON "public"."orders" USING btree ("customer_id", "created_at") WHERE deleted_at IS NULL;`
	if statement != want {
		t.Fatalf("index SQL = %q, want %q", statement, want)
	}

	if _, err := buildPostgresIndexChange(database.IndexChange{
		Table:   database.Table{Schema: "public", Name: "orders"},
		Name:    "unsafe",
		Columns: []string{"id"},
		Method:  "btree; DROP TABLE orders",
	}); err == nil {
		t.Fatal("unsafe index method was accepted")
	}
	if _, err := buildPostgresIndexChange(database.IndexChange{
		Table:   database.Table{Schema: "public", Name: "orders"},
		Name:    "unsafe",
		Columns: []string{"id"},
		Where:   "true; DROP TABLE orders",
	}); err == nil {
		t.Fatal("multi-statement index predicate was accepted")
	}
}

func TestBuildPostgresColumnChangeProducesReviewedSequence(t *testing.T) {
	nullable := false
	defaultValue := "now()"
	statements, err := buildPostgresColumnChange(database.ColumnChange{
		Table:    database.Table{Schema: "public", Name: "orders"},
		Name:     "created",
		NewName:  "created_at",
		DataType: "timestamptz",
		Using:    "created AT TIME ZONE 'UTC'",
		Nullable: &nullable,
		Default:  &defaultValue,
	})
	if err != nil {
		t.Fatalf("build column change: %v", err)
	}
	if len(statements) != 4 {
		t.Fatalf("statement count = %d, want 4: %#v", len(statements), statements)
	}
	if !strings.Contains(statements[1], `"created_at" TYPE timestamptz`) {
		t.Fatalf("type statement did not use renamed column: %q", statements[1])
	}
	if !strings.Contains(statements[2], "SET NOT NULL") {
		t.Fatalf("nullability statement = %q", statements[2])
	}
}

func TestBuildPostgresConstraintChangeRejectsArbitrarySQL(t *testing.T) {
	_, _, err := buildPostgresConstraintChange(
		database.ObjectChangeAddConstraint,
		database.ConstraintChange{
			Table:      database.Table{Schema: "public", Name: "orders"},
			Name:       "unsafe",
			Definition: "DROP TABLE users",
		},
		false,
	)
	if err == nil {
		t.Fatal("arbitrary constraint SQL was accepted")
	}

	statement, destructive, err := buildPostgresConstraintChange(
		database.ObjectChangeAddConstraint,
		database.ConstraintChange{
			Table:      database.Table{Schema: "public", Name: "orders"},
			Name:       "orders_amount_positive",
			Definition: "CHECK (amount > 0)",
		},
		false,
	)
	if err != nil || destructive {
		t.Fatalf("build check constraint = %q, destructive=%v, err=%v", statement, destructive, err)
	}
}
