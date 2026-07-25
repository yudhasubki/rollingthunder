package oracle

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"rollingthunder/pkg/database"
)

func TestBuildOracleViewAndRoutineChanges(t *testing.T) {
	driver := &Oracle{}
	view, err := driver.BuildObjectChange(
		context.Background(),
		database.ObjectChangeRequest{
			Action: database.ObjectChangeCreate,
			Reference: database.ObjectReference{
				Kind:   database.ObjectKindView,
				Schema: "REPORTING",
				Name:   "ACTIVE USERS",
			},
			Definition: `SELECT * FROM "APP"."USERS" WHERE "ACTIVE" = 1`,
		},
	)
	if err != nil {
		t.Fatalf("build view: %v", err)
	}
	want := "CREATE VIEW \"REPORTING\".\"ACTIVE USERS\" AS\n" +
		`SELECT * FROM "APP"."USERS" WHERE "ACTIVE" = 1;`
	if len(view.Statements) != 1 || view.Statements[0] != want {
		t.Fatalf("view SQL = %#v, want %q", view.Statements, want)
	}
	if view.Transactional {
		t.Fatal("Oracle DDL plan was incorrectly marked transactional")
	}

	routine, err := driver.BuildObjectChange(
		context.Background(),
		database.ObjectChangeRequest{
			Action: database.ObjectChangeReplace,
			Reference: database.ObjectReference{
				Kind:   database.ObjectKindFunction,
				Schema: "APP",
				Name:   "ANSWER",
			},
			Definition: `
				CREATE FUNCTION "APP"."ANSWER"
				RETURN NUMBER
				AS
				BEGIN
					RETURN 42;
				END;
			`,
		},
	)
	if err != nil {
		t.Fatalf("build function: %v", err)
	}
	if !strings.HasPrefix(routine.Statements[0], "CREATE OR REPLACE FUNCTION") {
		t.Fatalf("replace function SQL = %q", routine.Statements[0])
	}
	if _, err := reviewedOracleDDL(
		routine.Statements[0]+` DROP TABLE "APP"."ORDERS";`,
		database.ObjectKindFunction,
		true,
	); err == nil {
		t.Fatal("statement appended after an Oracle routine was accepted")
	}
}

func TestBuildOracleIndexAndColumnChanges(t *testing.T) {
	index, err := buildOracleIndexChange(database.IndexChange{
		Table:   database.Table{Schema: "APP", Name: "ORDERS"},
		Name:    "ORDERS_CUSTOMER_IDX",
		Columns: []string{"CUSTOMER_ID", "CREATED_AT"},
		Unique:  true,
		Method:  "btree",
	})
	if err != nil {
		t.Fatalf("build index: %v", err)
	}
	want := `CREATE UNIQUE INDEX "APP"."ORDERS_CUSTOMER_IDX" ON "APP"."ORDERS" ("CUSTOMER_ID", "CREATED_AT");`
	if index != want {
		t.Fatalf("index SQL = %q, want %q", index, want)
	}
	if _, err := buildOracleIndexChange(database.IndexChange{
		Table:   database.Table{Schema: "APP", Name: "ORDERS"},
		Name:    "UNSUPPORTED",
		Columns: []string{"ID"},
		Where:   "DELETED_AT IS NULL",
	}); err == nil {
		t.Fatal("partial Oracle index was accepted")
	}

	nullable := false
	defaultValue := "'pending'"
	statements, err := buildOracleColumnChange(database.ColumnChange{
		Table:    database.Table{Schema: "APP", Name: "ORDERS"},
		Name:     "STATUS",
		NewName:  "STATE",
		DataType: "VARCHAR2(32)",
		Nullable: &nullable,
		Default:  &defaultValue,
	})
	if err != nil {
		t.Fatalf("build column change: %v", err)
	}
	if len(statements) != 4 {
		t.Fatalf("column statement count = %d: %#v", len(statements), statements)
	}
	if !strings.Contains(statements[1], `"STATE" VARCHAR2(32)`) {
		t.Fatalf("renamed column was not used: %q", statements[1])
	}
}

func TestReviewedOracleDDLRequiresTheRequestedObjectKind(t *testing.T) {
	if _, err := reviewedOracleDDL(
		"CREATE TABLE FUNCTION_LOG (ID NUMBER);",
		database.ObjectKindFunction,
		false,
	); err == nil {
		t.Fatal("table DDL was accepted as a function definition")
	}
	definition, err := reviewedOracleDDL(
		`CREATE OR REPLACE FORCE EDITIONABLE VIEW "APP"."ACTIVE_USERS" AS SELECT 1 AS "ID" FROM dual`,
		database.ObjectKindView,
		true,
	)
	if err != nil {
		t.Fatalf("valid Oracle view DDL was rejected: %v", err)
	}
	if !strings.HasPrefix(definition, "CREATE OR REPLACE FORCE EDITIONABLE VIEW") {
		t.Fatalf("definition = %q", definition)
	}
}

func TestBuildOracleDropAndTriggerChanges(t *testing.T) {
	driver := &Oracle{}
	drop, err := driver.BuildObjectChange(
		context.Background(),
		database.ObjectChangeRequest{
			Action: database.ObjectChangeDrop,
			Reference: database.ObjectReference{
				Kind:   database.ObjectKindTable,
				Schema: "APP",
				Name:   "ORDERS",
			},
			Cascade: true,
		},
	)
	if err != nil {
		t.Fatalf("build drop: %v", err)
	}
	if !drop.Destructive ||
		drop.Statements[0] !=
			`DROP TABLE "APP"."ORDERS" CASCADE CONSTRAINTS;` {
		t.Fatalf("drop plan = %+v", drop)
	}

	toggle, err := driver.BuildObjectChange(
		context.Background(),
		database.ObjectChangeRequest{
			Action: database.ObjectChangeDisable,
			Reference: database.ObjectReference{
				Kind:   database.ObjectKindTrigger,
				Schema: "APP",
				Name:   "ORDERS_AUDIT",
			},
		},
	)
	if err != nil {
		t.Fatalf("build trigger toggle: %v", err)
	}
	if toggle.Statements[0] !=
		`ALTER TRIGGER "APP"."ORDERS_AUDIT" DISABLE;` {
		t.Fatalf("trigger SQL = %q", toggle.Statements[0])
	}
}

func TestExecutableOracleDDLHandlesTerminators(t *testing.T) {
	if got := executableOracleDDL(
		`ALTER TABLE "APP"."ORDERS" ADD "STATE" VARCHAR2(20);`,
	); strings.HasSuffix(got, ";") {
		t.Fatalf("plain Oracle SQL retained client terminator: %q", got)
	}
	plsql := `CREATE OR REPLACE FUNCTION "APP"."ANSWER" RETURN NUMBER AS BEGIN RETURN 42; END;`
	if got := executableOracleDDL(plsql); got != plsql {
		t.Fatalf("PL/SQL terminator changed: %q", got)
	}
}

func TestBuildOracleExplainPlan(t *testing.T) {
	plan, err := buildOracleExplainPlan([]oracleExplainRow{
		{
			id:          0,
			operation:   "SELECT STATEMENT",
			cost:        sql.NullFloat64{Float64: 4, Valid: true},
			cardinality: sql.NullFloat64{Float64: 3, Valid: true},
		},
		{
			id:          1,
			parentID:    sql.NullInt64{Int64: 0, Valid: true},
			operation:   "TABLE ACCESS",
			options:     sql.NullString{String: "FULL", Valid: true},
			objectOwner: sql.NullString{String: "APP", Valid: true},
			objectName:  sql.NullString{String: "USERS", Valid: true},
			cost:        sql.NullFloat64{Float64: 4, Valid: true},
			cardinality: sql.NullFloat64{Float64: 3, Valid: true},
			filter:      sql.NullString{String: `"ACTIVE"=1`, Valid: true},
		},
	})
	if err != nil {
		t.Fatalf("build explain plan: %v", err)
	}
	if len(plan.Roots) != 1 || len(plan.Roots[0].Children) != 1 {
		t.Fatalf("plan tree = %+v", plan.Roots)
	}
	child := plan.Roots[0].Children[0]
	if child.Relation != "APP.USERS" ||
		child.Details["Filter predicates"] != `"ACTIVE"=1` {
		t.Fatalf("child node = %+v", child)
	}
}

func TestOracleSQLInsertLiterals(t *testing.T) {
	text, err := oracleSQLLiteral(
		"thunder's",
		database.Structure{Name: "NAME", DataType: "VARCHAR2(255)"},
	)
	if err != nil || text != "'thunder''s'" {
		t.Fatalf("text literal = %q, err=%v", text, err)
	}
	raw, err := oracleSQLLiteral(
		[]byte{0x00, 0xff},
		database.Structure{Name: "PAYLOAD", DataType: "RAW(2)"},
	)
	if err != nil || raw != "HEXTORAW('00FF')" {
		t.Fatalf("RAW literal = %q, err=%v", raw, err)
	}
}
