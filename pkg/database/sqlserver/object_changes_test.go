package sqlserver

import (
	"context"
	"strings"
	"testing"

	"rollingthunder/pkg/database"
)

func TestBuildSQLServerViewAndRoutineChanges(t *testing.T) {
	driver := &SQLServer{}
	view, err := driver.BuildObjectChange(
		context.Background(),
		database.ObjectChangeRequest{
			Action: database.ObjectChangeCreate,
			Reference: database.ObjectReference{
				Kind:   database.ObjectKindView,
				Schema: "reporting",
				Name:   "active users",
			},
			Definition: "SELECT * FROM dbo.users WHERE active = 1",
		},
	)
	if err != nil {
		t.Fatalf("build view: %v", err)
	}
	want := "CREATE VIEW [reporting].[active users] AS\n" +
		"SELECT * FROM dbo.users WHERE active = 1;"
	if len(view.Statements) != 1 || view.Statements[0] != want {
		t.Fatalf("view SQL = %#v, want %q", view.Statements, want)
	}

	routine, err := driver.BuildObjectChange(
		context.Background(),
		database.ObjectChangeRequest{
			Action: database.ObjectChangeReplace,
			Reference: database.ObjectReference{
				Kind:   database.ObjectKindProcedure,
				Schema: "dbo",
				Name:   "refresh_report",
			},
			Definition: `
				CREATE PROCEDURE [dbo].[refresh_report]
				AS
				BEGIN
					SELECT 1;
				END;
			`,
		},
	)
	if err != nil {
		t.Fatalf("build routine: %v", err)
	}
	if !strings.HasPrefix(routine.Statements[0], "CREATE OR ALTER PROCEDURE") {
		t.Fatalf("replace routine SQL = %q", routine.Statements[0])
	}
}

func TestBuildSQLServerIndexChange(t *testing.T) {
	statement, err := buildIndexChange(database.IndexChange{
		Table:   database.Table{Schema: "dbo", Name: "orders"},
		Name:    "orders_customer_idx",
		Columns: []string{"customer_id", "created_at"},
		Unique:  true,
		Method:  "nonclustered",
		Where:   "deleted_at IS NULL",
	})
	if err != nil {
		t.Fatalf("build index: %v", err)
	}
	want := "CREATE UNIQUE NONCLUSTERED INDEX [orders_customer_idx] " +
		"ON [dbo].[orders] ([customer_id], [created_at]) " +
		"WHERE deleted_at IS NULL;"
	if statement != want {
		t.Fatalf("index SQL = %q, want %q", statement, want)
	}
	if _, err := buildIndexChange(database.IndexChange{
		Table:   database.Table{Schema: "dbo", Name: "orders"},
		Name:    "unsafe",
		Columns: []string{"id"},
		Method:  "nonclustered; DROP TABLE orders",
	}); err == nil {
		t.Fatal("unsafe index type was accepted")
	}
}

func TestReviewedSQLServerDDLRequiresTheRequestedObjectKind(t *testing.T) {
	if _, err := reviewedDDL(
		"CREATE TABLE procedure_log (id int);",
		database.ObjectKindProcedure,
		false,
	); err == nil {
		t.Fatal("table DDL was accepted as a procedure definition")
	}
	if _, err := reviewedDDL(
		"-- comment\nCREATE PROCEDURE dbo.refresh_report AS SELECT 1;",
		database.ObjectKindProcedure,
		false,
	); err == nil {
		t.Fatal("a definition with content before CREATE was accepted")
	}
}

func TestSQLServerRenameQuotesMultipartNames(t *testing.T) {
	statement, err := renameStatement(database.ObjectReference{
		Kind:   database.ObjectKindView,
		Schema: "reporting.archive",
		Name:   "active.users",
	}, "recent.users")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(
		statement,
		"N'[reporting.archive].[active.users]'",
	) {
		t.Fatalf("rename SQL = %q", statement)
	}
}

func TestBuildSQLServerDropColumnRemovesDefaultFirst(t *testing.T) {
	driver := &SQLServer{}
	plan, err := driver.BuildObjectChange(
		context.Background(),
		database.ObjectChangeRequest{
			Action: database.ObjectChangeDropColumn,
			DropColumn: &database.DropColumnChange{
				Table: database.Table{Schema: "dbo", Name: "orders"},
				Name:  "status",
			},
		},
	)
	if err != nil {
		t.Fatalf("build drop column: %v", err)
	}
	if !plan.Destructive || len(plan.Statements) != 2 {
		t.Fatalf("drop plan = %+v", plan)
	}
	if !strings.Contains(plan.Statements[0], "sys.default_constraints") {
		t.Fatalf("default constraint cleanup missing: %q", plan.Statements[0])
	}
	if plan.Statements[1] !=
		"ALTER TABLE [dbo].[orders] DROP COLUMN [status];" {
		t.Fatalf("drop SQL = %q", plan.Statements[1])
	}
}

func TestBuildSQLServerTriggerToggle(t *testing.T) {
	driver := &SQLServer{}
	plan, err := driver.BuildObjectChange(
		context.Background(),
		database.ObjectChangeRequest{
			Action: database.ObjectChangeDisable,
			Reference: database.ObjectReference{
				Kind:         database.ObjectKindTrigger,
				Schema:       "dbo",
				Name:         "orders_audit",
				ParentSchema: "dbo",
				ParentName:   "orders",
			},
		},
	)
	if err != nil {
		t.Fatalf("build trigger toggle: %v", err)
	}
	want := "DISABLE TRIGGER [orders_audit] ON [dbo].[orders];"
	if len(plan.Statements) != 1 || plan.Statements[0] != want {
		t.Fatalf("trigger SQL = %#v, want %q", plan.Statements, want)
	}
}

func TestParseSQLServerShowplan(t *testing.T) {
	document := `
		<ShowPlanXML>
			<BatchSequence>
				<Batch>
					<Statements>
						<StmtSimple StatementText="SELECT * FROM dbo.users">
							<QueryPlan>
								<RelOp NodeId="0" PhysicalOp="Nested Loops" LogicalOp="Inner Join"
									EstimateRows="2" EstimatedTotalSubtreeCost="0.1">
									<NestedLoops>
										<RelOp NodeId="1" PhysicalOp="Index Seek" LogicalOp="Index Seek"
											EstimateRows="2" EstimatedTotalSubtreeCost="0.04">
											<IndexScan>
												<Object Schema="[dbo]" Table="[users]" Index="[users_pk]" />
												<SeekPredicates>
													<ScalarOperator ScalarString="[dbo].[users].[id]=(1)" />
												</SeekPredicates>
											</IndexScan>
										</RelOp>
									</NestedLoops>
								</RelOp>
							</QueryPlan>
						</StmtSimple>
					</Statements>
				</Batch>
			</BatchSequence>
		</ShowPlanXML>`
	roots, summary, err := parseSQLServerShowplan(document)
	if err != nil {
		t.Fatalf("parse showplan: %v", err)
	}
	if summary != "SELECT * FROM dbo.users" ||
		len(roots) != 1 ||
		len(roots[0].Children) != 1 {
		t.Fatalf("showplan summary=%q roots=%+v", summary, roots)
	}
	child := roots[0].Children[0]
	if child.ParentID != roots[0].ID ||
		child.Relation != "dbo.users" ||
		child.Details["Index"] != "users_pk" {
		t.Fatalf("child node = %+v", child)
	}
}

func TestSQLServerSQLInsertLiterals(t *testing.T) {
	text, err := sqlServerSQLLiteral(
		"thunder's",
		database.Structure{Name: "name", DataType: "nvarchar(255)"},
	)
	if err != nil || text != "N'thunder''s'" {
		t.Fatalf("text literal = %q, err=%v", text, err)
	}
	binary, err := sqlServerSQLLiteral(
		[]byte{0x00, 0xff},
		database.Structure{Name: "payload", DataType: "varbinary(max)"},
	)
	if err != nil || binary != "0x00FF" {
		t.Fatalf("binary literal = %q, err=%v", binary, err)
	}
}
