package drivertest

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"rollingthunder/pkg/database"
)

type LiveConfig struct {
	Driver             database.Driver
	Schema             string
	IntegerType        string
	TextType           string
	ExercisePrivileged bool
}

func RunCapabilityContract(
	t *testing.T,
	driver database.CapabilityDriver,
	expectedEngine string,
) {
	t.Helper()
	capabilities := driver.Capabilities()
	if err := capabilities.Validate(); err != nil {
		t.Fatalf("Capabilities().Validate() error = %v", err)
	}
	if capabilities.Engine != expectedEngine {
		t.Fatalf(
			"Capabilities().Engine = %q, want %q",
			capabilities.Engine,
			expectedEngine,
		)
	}
	quoted := driver.QuoteIdentifier(`odd"name`)
	if quoted == "" || quoted == `odd"name` {
		t.Fatalf("QuoteIdentifier() did not quote identifier: %q", quoted)
	}
	if driver.Placeholder(1) == "" || driver.Placeholder(2) == "" {
		t.Fatal("Placeholder() returned an empty placeholder")
	}
	pagination, err := driver.PaginationClause(25, 50)
	if err != nil {
		t.Fatalf("PaginationClause() error = %v", err)
	}
	if !strings.Contains(pagination, "25") ||
		!strings.Contains(pagination, "50") {
		t.Fatalf("PaginationClause() = %q, want limit and offset", pagination)
	}
	if _, err := driver.PaginationClause(-1, 0); err == nil {
		t.Fatal("PaginationClause() accepted a negative limit")
	}
	if _, err := driver.PaginationClause(1, -1); err == nil {
		t.Fatal("PaginationClause() accepted a negative offset")
	}

	requireCapabilityInterface(
		t,
		capabilities.Schemas,
		"schema discovery",
		func() bool {
			_, ok := driver.(database.DriverWithSchema)
			return ok
		},
	)
	requireCapabilityInterface(
		t,
		capabilities.ObjectDefinitions ||
			capabilities.ObjectDependencies,
		"object metadata",
		func() bool {
			_, ok := driver.(database.ObjectDriver)
			return ok
		},
	)
	requireCapabilityInterface(
		t,
		capabilities.ManageViews ||
			capabilities.ManageRoutines ||
			capabilities.ManageTriggers ||
			capabilities.ManageIndexes ||
			capabilities.AlterTableStructure,
		"reviewed object changes",
		func() bool {
			_, ok := driver.(database.ObjectChangeDriver)
			return ok
		},
	)
	requireCapabilityInterface(
		t,
		capabilities.ExplainPlans,
		"query plans",
		func() bool {
			_, ok := driver.(database.ExplainPlanDriver)
			return ok
		},
	)
	requireCapabilityInterface(
		t,
		capabilities.Transactions,
		"transactions",
		func() bool {
			_, ok := driver.(database.TransactionalDriver)
			return ok
		},
	)
	requireCapabilityInterface(
		t,
		capabilities.AtomicTableChanges,
		"atomic table changes",
		func() bool {
			_, ok := driver.(database.TableChangeDriver)
			return ok
		},
	)
	requireCapabilityInterface(
		t,
		capabilities.ActivityMonitor,
		"activity monitoring",
		func() bool {
			_, ok := driver.(database.ActivityDriver)
			return ok
		},
	)
	requireCapabilityInterface(
		t,
		capabilities.ManageSecurity,
		"security management",
		func() bool {
			_, ok := driver.(database.SecurityDriver)
			return ok
		},
	)
}

func RunLiveContract(t *testing.T, config LiveConfig) {
	t.Helper()
	ctx := context.Background()
	driver := config.Driver
	schema := config.Schema
	tableName := "rt_conformance_rows"
	table := database.Table{Schema: schema, Name: tableName}
	relationTable := database.Table{
		Schema: schema,
		Name:   "rt_conformance_links",
	}
	viewReference := database.ObjectReference{
		Kind:   database.ObjectKindView,
		Schema: schema,
		Name:   "rt_conformance_view",
	}
	if objectChangeDriver, ok := driver.(database.ObjectChangeDriver); ok {
		if plan, err := objectChangeDriver.BuildObjectChange(
			ctx,
			database.ObjectChangeRequest{
				Action:    database.ObjectChangeDrop,
				Reference: viewReference,
			},
		); err == nil {
			_ = objectChangeDriver.ApplyObjectChange(ctx, plan)
		}
	}
	_ = driver.DropTable(relationTable)
	_ = driver.DropTable(table)
	t.Cleanup(func() {
		_ = driver.DropTable(relationTable)
		_ = driver.DropTable(table)
	})

	healthDriver, ok := driver.(database.HealthDriver)
	if !ok {
		t.Fatal("driver does not implement HealthDriver")
	}
	if err := healthDriver.Ping(ctx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}

	info, err := driver.GetDatabaseInfo()
	if err != nil {
		t.Fatalf("GetDatabaseInfo() error = %v", err)
	}
	if strings.TrimSpace(info.Engine) == "" ||
		strings.TrimSpace(info.Version) == "" ||
		strings.TrimSpace(info.Database) == "" {
		t.Fatalf("GetDatabaseInfo() = %+v, want complete database metadata", info)
	}

	dataTypes := driver.GetDataTypes()
	if len(dataTypes) == 0 {
		t.Fatal("GetDataTypes() returned no data types")
	}
	for index, dataType := range dataTypes {
		if strings.TrimSpace(dataType.Name) == "" {
			t.Fatalf("GetDataTypes()[%d] has an empty name: %+v", index, dataType)
		}
	}

	if schemaDriver, supported := driver.(database.DriverWithSchema); supported {
		schemas, err := schemaDriver.GetSchemas()
		if err != nil {
			t.Fatalf("GetSchemas() error = %v", err)
		}
		if !containsFold(schemas, schema) {
			t.Fatalf("GetSchemas() = %v, missing %q", schemas, schema)
		}
	}

	if err := driver.CreateTable(table, []database.ColumnDefinition{
		{
			Name:       "id",
			Type:       config.IntegerType,
			Nullable:   false,
			PrimaryKey: true,
		},
		{
			Name:     "name",
			Type:     config.TextType,
			Nullable: false,
		},
		{
			Name:     "score",
			Type:     config.IntegerType,
			Nullable: true,
		},
	}); err != nil {
		t.Fatalf("CreateTable() error = %v", err)
	}

	collections, err := driver.GetCollections(schema)
	if err != nil {
		t.Fatalf("GetCollections() error = %v", err)
	}
	if !contains(collections, tableName) {
		t.Fatalf("GetCollections() = %v, missing %q", collections, tableName)
	}
	structures, err := driver.GetCollectionStructures(table)
	if err != nil {
		t.Fatalf("GetCollectionStructures() error = %v", err)
	}
	if len(structures) != 3 || !structures[0].IsPrimary {
		t.Fatalf("GetCollectionStructures() = %+v, want three columns and PK", structures)
	}

	if _, err := driver.GetIndices(table); err != nil {
		t.Fatalf("GetIndices() error = %v", err)
	}
	ddl, err := driver.GetTableDDL(table)
	if err != nil {
		t.Fatalf("GetTableDDL() error = %v", err)
	}
	if strings.TrimSpace(ddl) == "" {
		t.Fatal("GetTableDDL() returned an empty definition")
	}

	_, err = driver.ExecuteQuery(
		ctx,
		fmt.Sprintf(
			"CREATE TABLE %s (%s %s NOT NULL PRIMARY KEY, %s %s NOT NULL, CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s))",
			qualified(driver, schema, relationTable.Name),
			driver.QuoteIdentifier("id"),
			config.IntegerType,
			driver.QuoteIdentifier("row_id"),
			config.IntegerType,
			driver.QuoteIdentifier("rt_conformance_links_fk"),
			driver.QuoteIdentifier("row_id"),
			driver.QuoteIdentifier(tableName),
			driver.QuoteIdentifier("id"),
		),
		database.QueryOptions{MaxRows: 10},
	)
	if err != nil {
		t.Fatalf("create foreign-key fixture error = %v", err)
	}
	relationStructures, err := driver.GetCollectionStructures(relationTable)
	if err != nil {
		t.Fatalf("GetCollectionStructures(foreign key) error = %v", err)
	}
	if len(relationStructures) != 2 || !relationStructures[0].IsPrimary {
		t.Fatalf(
			"foreign-key fixture structures = %+v, want two columns and PK",
			relationStructures,
		)
	}
	foreignColumn := findColumn(relationStructures, "row_id")
	if foreignColumn == nil ||
		foreignColumn.ForeignSchema == nil ||
		!strings.EqualFold(*foreignColumn.ForeignSchema, schema) ||
		foreignColumn.ForeignTable == nil ||
		!strings.EqualFold(*foreignColumn.ForeignTable, tableName) ||
		foreignColumn.ForeignColumn == nil ||
		!strings.EqualFold(*foreignColumn.ForeignColumn, "id") {
		t.Fatalf(
			"foreign-key metadata = %+v, want %s(id)",
			foreignColumn,
			tableName,
		)
	}

	for _, row := range []map[string]interface{}{
		{"id": 1, "name": "alpha", "score": 10},
		{"id": 2, "name": "beta", "score": 20},
	} {
		if err := driver.InsertRow(table, row); err != nil {
			t.Fatalf("InsertRow() error = %v", err)
		}
	}
	count, err := driver.CountCollectionData(table)
	if err != nil || count != 2 {
		t.Fatalf("CountCollectionData() = %d, %v, want 2", count, err)
	}

	if err := driver.UpdateRow(table, map[string]interface{}{
		"id": 1, "name": "alpha-direct", "score": 11,
	}, "id"); err != nil {
		t.Fatalf("UpdateRow() error = %v", err)
	}
	directQuery, err := driver.ExecuteQuery(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM %s WHERE %s = 1",
			driver.QuoteIdentifier("name"),
			qualified(driver, schema, tableName),
			driver.QuoteIdentifier("id"),
		),
		database.QueryOptions{MaxRows: 10},
	)
	if err != nil {
		t.Fatalf("ExecuteQuery() error = %v", err)
	}
	if len(directQuery.Rows) != 1 ||
		fmt.Sprint(rowValueFold(directQuery.Rows[0], "name")) != "alpha-direct" {
		t.Fatalf("ExecuteQuery() rows = %v, want directly updated row", directQuery.Rows)
	}
	if err := driver.DeleteRow(table, "id", 2); err != nil {
		t.Fatalf("DeleteRow() error = %v", err)
	}
	count, err = driver.CountCollectionData(table)
	if err != nil || count != 1 {
		t.Fatalf("count after DeleteRow() = %d, %v, want 1", count, err)
	}
	if err := driver.InsertRow(
		table,
		map[string]interface{}{"id": 2, "name": "beta", "score": 20},
	); err != nil {
		t.Fatalf("InsertRow() after direct delete error = %v", err)
	}

	page := table
	page.Limit = 1
	page.Filters = []database.Filter{{
		Column: "name", Operator: database.FilterContains, Value: "a",
	}}
	page.Sorts = []database.Sort{{
		Column: "score", Direction: database.SortDescending, Nulls: database.NullsLast,
	}}
	pageStructures, rows, err := driver.GetCollectionData(page)
	if err != nil {
		t.Fatalf("GetCollectionData() error = %v", err)
	}
	if len(pageStructures) != 3 || len(rows) != 1 ||
		fmt.Sprint(rows[0]["name"]) != "beta" {
		t.Fatalf("GetCollectionData() structures=%d rows=%v", len(pageStructures), rows)
	}

	changeDriver, ok := driver.(database.TableChangeDriver)
	if !ok {
		t.Fatal("driver does not implement TableChangeDriver")
	}
	result, err := changeDriver.ApplyTableChanges(ctx, database.TableChangeSet{
		Table: table,
		Added: []map[string]interface{}{
			{"id": 3, "name": "gamma", "score": 30},
		},
		Updated: []database.RowUpdate{{
			Original:       map[string]interface{}{"id": 1, "name": "alpha-direct", "score": 11},
			Values:         map[string]interface{}{"id": 1, "name": "alpha-updated", "score": 12},
			ChangedColumns: []string{"name", "score"},
		}},
		Deleted: []map[string]interface{}{
			{"id": 2, "name": "beta", "score": 20},
		},
	})
	if err != nil {
		t.Fatalf("ApplyTableChanges() error = %v", err)
	}
	if result.Inserted != 1 || result.Updated != 1 || result.Deleted != 1 {
		t.Fatalf("ApplyTableChanges() result = %+v", result)
	}

	transactional, ok := driver.(database.TransactionalDriver)
	if !ok {
		t.Fatal("driver does not implement TransactionalDriver")
	}
	transaction, err := transactional.BeginTransaction(ctx)
	if err != nil {
		t.Fatalf("BeginTransaction() error = %v", err)
	}
	_, err = transaction.ExecuteQuery(
		ctx,
		fmt.Sprintf(
			"INSERT INTO %s (%s, %s, %s) VALUES (99, 'rollback', 99)",
			qualified(driver, schema, tableName),
			driver.QuoteIdentifier("id"),
			driver.QuoteIdentifier("name"),
			driver.QuoteIdentifier("score"),
		),
		database.QueryOptions{MaxRows: 10},
	)
	if err != nil {
		_ = transaction.Rollback()
		t.Fatalf("transaction ExecuteQuery() error = %v", err)
	}
	if err := transaction.Rollback(); err != nil {
		t.Fatalf("transaction Rollback() error = %v", err)
	}
	count, err = driver.CountCollectionData(table)
	if err != nil || count != 2 {
		t.Fatalf("count after rollback = %d, %v, want 2", count, err)
	}

	transaction, err = transactional.BeginTransaction(ctx)
	if err != nil {
		t.Fatalf("BeginTransaction() for commit error = %v", err)
	}
	_, err = transaction.ExecuteQuery(
		ctx,
		fmt.Sprintf(
			"INSERT INTO %s (%s, %s, %s) VALUES (98, 'commit', 98)",
			qualified(driver, schema, tableName),
			driver.QuoteIdentifier("id"),
			driver.QuoteIdentifier("name"),
			driver.QuoteIdentifier("score"),
		),
		database.QueryOptions{MaxRows: 10},
	)
	if err != nil {
		_ = transaction.Rollback()
		t.Fatalf("transaction ExecuteQuery() before commit error = %v", err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatalf("transaction Commit() error = %v", err)
	}
	count, err = driver.CountCollectionData(table)
	if err != nil || count != 3 {
		t.Fatalf("count after commit = %d, %v, want 3", count, err)
	}
	if err := driver.DeleteRow(table, "id", 98); err != nil {
		t.Fatalf("DeleteRow() after commit error = %v", err)
	}

	limitedQuery, err := driver.ExecuteQuery(
		ctx,
		fmt.Sprintf(
			"SELECT * FROM %s ORDER BY %s",
			qualified(driver, schema, tableName),
			driver.QuoteIdentifier("id"),
		),
		database.QueryOptions{MaxRows: 1},
	)
	if err != nil {
		t.Fatalf("ExecuteQuery() with row limit error = %v", err)
	}
	if len(limitedQuery.Rows) != 1 ||
		!limitedQuery.Truncated ||
		limitedQuery.RowLimit != 1 {
		t.Fatalf(
			"limited ExecuteQuery() = rows:%d truncated:%t limit:%d, want 1/true/1",
			len(limitedQuery.Rows),
			limitedQuery.Truncated,
			limitedQuery.RowLimit,
		)
	}

	capabilities := driver.Capabilities()
	if capabilities.ExplainPlans {
		explainDriver, ok := driver.(database.ExplainPlanDriver)
		if !ok {
			t.Fatal("driver advertises explain plans without ExplainPlanDriver")
		}
		plan, err := explainDriver.ExplainQuery(
			ctx,
			fmt.Sprintf(
				"SELECT * FROM %s WHERE %s = 1",
				qualified(driver, schema, tableName),
				driver.QuoteIdentifier("id"),
			),
		)
		if err != nil {
			t.Fatalf("ExplainQuery() error = %v", err)
		}
		if strings.TrimSpace(plan.Engine) == "" ||
			strings.TrimSpace(plan.Summary) == "" ||
			len(plan.Roots) == 0 {
			t.Fatalf("ExplainQuery() = %+v, want a populated plan", plan)
		}
	}

	if capabilities.AlterTableStructure || capabilities.ManageIndexes {
		objectChangeDriver, ok := driver.(database.ObjectChangeDriver)
		if !ok {
			t.Fatal("driver advertises structural changes without ObjectChangeDriver")
		}
		if capabilities.AlterTableStructure {
			addPlan, err := objectChangeDriver.BuildObjectChange(
				ctx,
				database.ObjectChangeRequest{
					Action: database.ObjectChangeAddColumn,
					AddColumn: &database.AddColumnChange{
						Table: table,
						Column: database.ColumnDefinition{
							Name:     "reviewed_note",
							Type:     config.TextType,
							Nullable: true,
						},
					},
				},
			)
			if err != nil {
				t.Fatalf("BuildObjectChange(add column) error = %v", err)
			}
			if err := addPlan.Validate(); err != nil {
				t.Fatalf("add-column plan validation error = %v", err)
			}
			if err := objectChangeDriver.ApplyObjectChange(ctx, addPlan); err != nil {
				t.Fatalf("ApplyObjectChange(add column) error = %v", err)
			}
			structures, err = driver.GetCollectionStructures(table)
			if err != nil {
				t.Fatalf("structures after add column error = %v", err)
			}
			if !hasColumn(structures, "reviewed_note") {
				t.Fatalf("structures after add column = %+v", structures)
			}

			dropPlan, err := objectChangeDriver.BuildObjectChange(
				ctx,
				database.ObjectChangeRequest{
					Action: database.ObjectChangeDropColumn,
					DropColumn: &database.DropColumnChange{
						Table: table,
						Name:  "reviewed_note",
					},
				},
			)
			if err != nil {
				t.Fatalf("BuildObjectChange(drop column) error = %v", err)
			}
			if err := dropPlan.Validate(); err != nil {
				t.Fatalf("drop-column plan validation error = %v", err)
			}
			if err := objectChangeDriver.ApplyObjectChange(ctx, dropPlan); err != nil {
				t.Fatalf("ApplyObjectChange(drop column) error = %v", err)
			}
			structures, err = driver.GetCollectionStructures(table)
			if err != nil {
				t.Fatalf("structures after drop column error = %v", err)
			}
			if hasColumn(structures, "reviewed_note") {
				t.Fatalf("structures still contain dropped column: %+v", structures)
			}
		}

		if capabilities.ManageIndexes {
			indexName := "rt_conformance_name_idx"
			indexPlan, err := objectChangeDriver.BuildObjectChange(
				ctx,
				database.ObjectChangeRequest{
					Action: database.ObjectChangeCreateIndex,
					Index: &database.IndexChange{
						Table:   table,
						Name:    indexName,
						Columns: []string{"name"},
					},
				},
			)
			if err != nil {
				t.Fatalf("BuildObjectChange(create index) error = %v", err)
			}
			if err := indexPlan.Validate(); err != nil {
				t.Fatalf("create-index plan validation error = %v", err)
			}
			if err := objectChangeDriver.ApplyObjectChange(ctx, indexPlan); err != nil {
				t.Fatalf("ApplyObjectChange(create index) error = %v", err)
			}
			indices, err := driver.GetIndices(table)
			if err != nil {
				t.Fatalf("GetIndices() after create error = %v", err)
			}
			if !hasIndex(indices, indexName, "name") {
				t.Fatalf("GetIndices() = %+v, missing reviewed index", indices)
			}
		}

		if capabilities.ManageViews {
			objectDriver, ok := driver.(database.ObjectDriver)
			if !ok {
				t.Fatal("driver advertises managed views without ObjectDriver")
			}
			createView, err := objectChangeDriver.BuildObjectChange(
				ctx,
				database.ObjectChangeRequest{
					Action:    database.ObjectChangeCreate,
					Reference: viewReference,
					Definition: fmt.Sprintf(
						"SELECT %s, %s FROM %s",
						driver.QuoteIdentifier("id"),
						driver.QuoteIdentifier("name"),
						qualified(driver, schema, tableName),
					),
				},
			)
			if err != nil {
				t.Fatalf("BuildObjectChange(create view) error = %v", err)
			}
			if err := createView.Validate(); err != nil {
				t.Fatalf("create-view plan validation error = %v", err)
			}
			if err := objectChangeDriver.ApplyObjectChange(ctx, createView); err != nil {
				t.Fatalf("ApplyObjectChange(create view) error = %v", err)
			}
			dropView, err := objectChangeDriver.BuildObjectChange(
				ctx,
				database.ObjectChangeRequest{
					Action:    database.ObjectChangeDrop,
					Reference: viewReference,
				},
			)
			if err != nil {
				t.Fatalf("BuildObjectChange(drop view) error = %v", err)
			}
			t.Cleanup(func() {
				_ = objectChangeDriver.ApplyObjectChange(
					context.Background(),
					dropView,
				)
			})

			viewObjects, err := objectDriver.ListObjects(
				ctx,
				database.ObjectFilter{
					Schema: schema,
					Kinds:  []database.ObjectKind{database.ObjectKindView},
					Search: viewReference.Name,
				},
			)
			if err != nil {
				t.Fatalf("ListObjects(view) error = %v", err)
			}
			actualView := findReference(viewObjects, viewReference.Name)
			if actualView.Name == "" {
				t.Fatalf("ListObjects(view) = %+v, missing reviewed view", viewObjects)
			}
			viewDetail, err := objectDriver.GetObjectDetail(ctx, actualView)
			if err != nil {
				t.Fatalf("GetObjectDetail(view) error = %v", err)
			}
			if strings.TrimSpace(viewDetail.Definition) == "" {
				t.Fatalf("GetObjectDetail(view) = %+v", viewDetail)
			}
			if capabilities.ObjectDependencies &&
				!hasDependency(viewDetail.Dependencies, tableName) {
				t.Fatalf(
					"view dependencies = %+v, missing table %q",
					viewDetail.Dependencies,
					tableName,
				)
			}
			if err := objectChangeDriver.ApplyObjectChange(ctx, dropView); err != nil {
				t.Fatalf("ApplyObjectChange(drop view) error = %v", err)
			}
		}
	}

	objectDriver, ok := driver.(database.ObjectDriver)
	if !ok {
		t.Fatal("driver does not implement ObjectDriver")
	}
	objects, err := objectDriver.ListObjects(ctx, database.ObjectFilter{
		Schema: schema,
		Kinds:  []database.ObjectKind{database.ObjectKindTable},
		Search: tableName,
	})
	if err != nil {
		t.Fatalf("ListObjects() error = %v", err)
	}
	var reference database.ObjectReference
	for _, object := range objects {
		if object.Reference.Name == tableName {
			reference = object.Reference
			break
		}
	}
	if reference.Name == "" {
		t.Fatalf("ListObjects() = %+v, missing conformance table", objects)
	}
	detail, err := objectDriver.GetObjectDetail(ctx, reference)
	if err != nil {
		t.Fatalf("GetObjectDetail() error = %v", err)
	}
	if detail.Definition == "" || len(detail.Columns) != 3 {
		t.Fatalf("GetObjectDetail() = %+v", detail)
	}

	if capabilities.ActivityMonitor {
		activityDriver, ok := driver.(database.ActivityDriver)
		if !ok {
			t.Fatal("driver advertises activity monitoring without ActivityDriver")
		}
		activity, err := activityDriver.GetDatabaseActivity(ctx)
		if err != nil {
			t.Fatalf("GetDatabaseActivity() error = %v", err)
		}
		if !activity.Supported ||
			strings.TrimSpace(activity.Engine) == "" ||
			activity.CapturedAt.IsZero() {
			t.Fatalf("GetDatabaseActivity() = %+v", activity)
		}
		if err := activityDriver.CancelDatabaseSession(
			ctx,
			"invalid-session-id",
			false,
		); err == nil {
			t.Fatal("CancelDatabaseSession() accepted an invalid session ID")
		}
	}

	if capabilities.ManageSecurity {
		securityDriver, ok := driver.(database.SecurityDriver)
		if !ok {
			t.Fatal("driver advertises security management without SecurityDriver")
		}
		overview, err := securityDriver.GetSecurityOverview(ctx, "", "")
		if err != nil {
			t.Fatalf("GetSecurityOverview() error = %v", err)
		}
		if !overview.Supported ||
			strings.TrimSpace(overview.Engine) == "" ||
			strings.TrimSpace(overview.CurrentUser) == "" {
			t.Fatalf("GetSecurityOverview() = %+v", overview)
		}
		securityPlan, err := securityDriver.BuildSecurityChange(
			ctx,
			database.SecurityChangeRequest{
				Action: database.SecurityCreatePrincipal,
				Principal: database.PrincipalOptions{
					Name:    "rt_conformance_planned_role",
					Kind:    database.PrincipalRole,
					Inherit: true,
				},
			},
		)
		if err != nil {
			t.Fatalf("BuildSecurityChange() error = %v", err)
		}
		if err := securityPlan.Validate(); err != nil {
			t.Fatalf("security plan validation error = %v", err)
		}
		if config.ExercisePrivileged {
			dropSecurityPlan, err := securityDriver.BuildSecurityChange(
				ctx,
				database.SecurityChangeRequest{
					Action: database.SecurityDropPrincipal,
					Principal: database.PrincipalOptions{
						Name: "rt_conformance_planned_role",
						Kind: database.PrincipalRole,
					},
				},
			)
			if err != nil {
				t.Fatalf("BuildSecurityChange(drop role) error = %v", err)
			}
			_ = securityDriver.ApplySecurityChange(ctx, dropSecurityPlan)
			t.Cleanup(func() {
				_ = securityDriver.ApplySecurityChange(
					context.Background(),
					dropSecurityPlan,
				)
			})
			if err := securityDriver.ApplySecurityChange(ctx, securityPlan); err != nil {
				t.Fatalf("ApplySecurityChange(create role) error = %v", err)
			}
			createdOverview, err := securityDriver.GetSecurityOverview(ctx, "", "")
			if err != nil {
				t.Fatalf("security overview after role creation error = %v", err)
			}
			if !hasPrincipal(
				createdOverview.Principals,
				"rt_conformance_planned_role",
			) {
				t.Fatalf(
					"security principals = %+v, missing conformance role",
					createdOverview.Principals,
				)
			}
			if err := securityDriver.ApplySecurityChange(
				ctx,
				dropSecurityPlan,
			); err != nil {
				t.Fatalf("ApplySecurityChange(drop role) error = %v", err)
			}
		}
	}

	var exported bytes.Buffer
	stats, err := driver.ExportTable(ctx, database.TableExportRequest{
		Table: database.Table{
			Schema: schema,
			Name:   tableName,
			Limit:  100,
		},
		Scope: database.ExportScopeAll,
		Options: database.ExportOptions{
			Format: database.ExportFormatCSV,
			CSV: database.CSVOptions{
				IncludeHeader: true,
				Encoding:      database.CSVEncodingUTF8,
			},
		},
	}, &exported)
	if err != nil {
		t.Fatalf("ExportTable() error = %v", err)
	}
	if stats.Rows != 2 {
		t.Fatalf("ExportTable() rows = %d, want 2", stats.Rows)
	}
	if !strings.Contains(exported.String(), "alpha-updated") ||
		!strings.Contains(exported.String(), "gamma") {
		t.Fatalf("ExportTable() output = %q", exported.String())
	}

	var jsonExport bytes.Buffer
	jsonStats, err := driver.ExportTable(ctx, database.TableExportRequest{
		Table: database.Table{Schema: schema, Name: tableName, Limit: 100},
		Scope: database.ExportScopeAll,
		Options: database.ExportOptions{
			Format: database.ExportFormatJSON,
			JSON:   database.JSONOptions{Pretty: true},
		},
	}, &jsonExport)
	if err != nil {
		t.Fatalf("ExportTable(JSON) error = %v", err)
	}
	if jsonStats.Rows != 2 ||
		!strings.Contains(jsonExport.String(), "alpha-updated") ||
		!strings.Contains(jsonExport.String(), "gamma") {
		t.Fatalf(
			"ExportTable(JSON) rows=%d output=%q",
			jsonStats.Rows,
			jsonExport.String(),
		)
	}

	if capabilities.SQLInsertExport {
		var sqlExport bytes.Buffer
		sqlStats, err := driver.ExportTable(ctx, database.TableExportRequest{
			Table: database.Table{Schema: schema, Name: tableName, Limit: 100},
			Scope: database.ExportScopeAll,
			Options: database.ExportOptions{
				Format: database.ExportFormatSQL,
				SQL: database.SQLInsertOptions{
					BatchSize: 2,
				},
			},
		}, &sqlExport)
		if err != nil {
			t.Fatalf("ExportTable(SQL) error = %v", err)
		}
		if sqlStats.Rows != 2 ||
			!strings.Contains(strings.ToUpper(sqlExport.String()), "INSERT") ||
			!strings.Contains(sqlExport.String(), tableName) {
			t.Fatalf(
				"ExportTable(SQL) rows=%d output=%q",
				sqlStats.Rows,
				sqlExport.String(),
			)
		}
	}

	if err := driver.DropTable(relationTable); err != nil {
		t.Fatalf("DropTable(foreign-key fixture) error = %v", err)
	}
	if err := driver.TruncateTable(table); err != nil {
		t.Fatalf("TruncateTable() error = %v", err)
	}
	count, err = driver.CountCollectionData(table)
	if err != nil || count != 0 {
		t.Fatalf("count after TruncateTable() = %d, %v, want 0", count, err)
	}
	if err := driver.DropTable(table); err != nil {
		t.Fatalf("DropTable() error = %v", err)
	}
	collections, err = driver.GetCollections(schema)
	if err != nil {
		t.Fatalf("GetCollections() after DropTable error = %v", err)
	}
	if containsFold(collections, tableName) {
		t.Fatalf("GetCollections() still contains dropped table %q", tableName)
	}
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func containsFold(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(value, expected) {
			return true
		}
	}
	return false
}

func hasColumn(structures database.Structures, expected string) bool {
	for _, structure := range structures {
		if strings.EqualFold(structure.Name, expected) {
			return true
		}
	}
	return false
}

func findColumn(
	structures database.Structures,
	expected string,
) *database.Structure {
	for index := range structures {
		if strings.EqualFold(structures[index].Name, expected) {
			return &structures[index]
		}
	}
	return nil
}

func hasIndex(
	indices database.Indices,
	expectedName string,
	expectedColumn string,
) bool {
	for _, index := range indices {
		if !strings.EqualFold(index.Name, expectedName) {
			continue
		}
		return containsFold(index.Columns, expectedColumn)
	}
	return false
}

func rowValueFold(row map[string]interface{}, expected string) interface{} {
	for name, value := range row {
		if strings.EqualFold(name, expected) {
			return value
		}
	}
	return nil
}

func findReference(
	objects []database.DatabaseObject,
	expected string,
) database.ObjectReference {
	for _, object := range objects {
		if strings.EqualFold(object.Reference.Name, expected) {
			return object.Reference
		}
	}
	return database.ObjectReference{}
}

func hasDependency(
	dependencies []database.ObjectDependency,
	expected string,
) bool {
	for _, dependency := range dependencies {
		if strings.EqualFold(dependency.Reference.Name, expected) {
			return true
		}
	}
	return false
}

func hasPrincipal(
	principals []database.DatabasePrincipal,
	expected string,
) bool {
	for _, principal := range principals {
		if strings.EqualFold(principal.Name, expected) {
			return true
		}
	}
	return false
}

func requireCapabilityInterface(
	t *testing.T,
	enabled bool,
	name string,
	implemented func() bool,
) {
	t.Helper()
	if enabled && !implemented() {
		t.Fatalf("driver advertises %s without implementing its interface", name)
	}
}

func qualified(
	driver database.CapabilityDriver,
	schema string,
	name string,
) string {
	if schema == "" {
		return driver.QuoteIdentifier(name)
	}
	return driver.QuoteIdentifier(schema) + "." + driver.QuoteIdentifier(name)
}
