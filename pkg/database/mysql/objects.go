package mysql

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	"rollingthunder/pkg/database"
)

func mysqlObjectID(reference database.ObjectReference) string {
	parts := []string{
		string(reference.Kind),
		reference.Schema,
		reference.Name,
		reference.Signature,
		reference.ParentSchema,
		reference.ParentName,
	}
	for index, part := range parts {
		parts[index] = base64.RawURLEncoding.EncodeToString([]byte(part))
	}
	return "mysql:" + strings.Join(parts, ".")
}

func mysqlObjectCanManage(
	kind database.ObjectKind,
	capabilities database.Capabilities,
) bool {
	switch kind {
	case database.ObjectKindTable, database.ObjectKindConstraint:
		return capabilities.AlterTableStructure
	case database.ObjectKindView:
		return capabilities.ManageViews
	case database.ObjectKindFunction, database.ObjectKindProcedure:
		return capabilities.ManageRoutines
	case database.ObjectKindTrigger:
		return capabilities.ManageTriggers
	case database.ObjectKindIndex:
		return capabilities.ManageIndexes
	default:
		return false
	}
}

func makeMySQLObject(
	reference database.ObjectReference,
	description string,
	properties []database.ObjectProperty,
	capabilities database.Capabilities,
) database.DatabaseObject {
	reference.ID = mysqlObjectID(reference)
	displayName := reference.Name
	if reference.Signature != "" {
		displayName += "(" + reference.Signature + ")"
	}
	allowedActions := make([]database.ObjectChangeAction, 0)
	switch reference.Kind {
	case database.ObjectKindTable:
		allowedActions = append(
			allowedActions,
			database.ObjectChangeRename,
			database.ObjectChangeDrop,
		)
	case database.ObjectKindView:
		allowedActions = append(
			allowedActions,
			database.ObjectChangeReplace,
			database.ObjectChangeRename,
			database.ObjectChangeDrop,
		)
	case database.ObjectKindFunction, database.ObjectKindProcedure,
		database.ObjectKindTrigger:
		allowedActions = append(
			allowedActions,
			database.ObjectChangeReplace,
			database.ObjectChangeDrop,
		)
	case database.ObjectKindIndex:
		allowedActions = append(
			allowedActions,
			database.ObjectChangeRename,
			database.ObjectChangeDrop,
		)
	}
	canManage := mysqlObjectCanManage(reference.Kind, capabilities) &&
		len(allowedActions) > 0
	return database.DatabaseObject{
		Reference:   reference,
		DisplayName: displayName,
		Description: description,
		CanOpenData: reference.Kind == database.ObjectKindTable ||
			reference.Kind == database.ObjectKindView,
		CanManage:      canManage,
		AllowedActions: allowedActions,
		Properties:     properties,
	}
}

func mysqlObjectKindSet(
	kinds []database.ObjectKind,
) map[database.ObjectKind]struct{} {
	if len(kinds) == 0 {
		return nil
	}
	result := make(map[database.ObjectKind]struct{}, len(kinds))
	for _, kind := range kinds {
		result[kind] = struct{}{}
	}
	return result
}

func mysqlObjectMatches(
	object database.DatabaseObject,
	allowed map[database.ObjectKind]struct{},
	search string,
) bool {
	if len(allowed) > 0 {
		if _, exists := allowed[object.Reference.Kind]; !exists {
			return false
		}
	}
	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		object.Reference.Name,
		object.Reference.ParentName,
		object.Reference.Signature,
		object.Description,
	}, " "))
	return strings.Contains(haystack, search)
}

type mysqlRelationObjectRow struct {
	Schema    string         `db:"rt_table_schema"`
	Name      string         `db:"rt_table_name"`
	TableType string         `db:"rt_table_type"`
	Engine    sql.NullString `db:"rt_engine"`
	Rows      sql.NullInt64  `db:"rt_table_rows"`
	Comment   string         `db:"rt_table_comment"`
}

type mysqlRoutineObjectRow struct {
	Schema       string         `db:"rt_routine_schema"`
	Name         string         `db:"rt_routine_name"`
	Type         string         `db:"rt_routine_type"`
	ReturnType   sql.NullString `db:"rt_data_type"`
	SecurityType string         `db:"rt_security_type"`
	SQLData      string         `db:"rt_sql_data_access"`
	Comment      string         `db:"rt_routine_comment"`
}

type mysqlTriggerObjectRow struct {
	Schema       string `db:"rt_trigger_schema"`
	Name         string `db:"rt_trigger_name"`
	ParentSchema string `db:"rt_event_object_schema"`
	ParentName   string `db:"rt_event_object_table"`
	Event        string `db:"rt_event_manipulation"`
	Timing       string `db:"rt_action_timing"`
}

type mysqlConstraintObjectRow struct {
	Schema       string `db:"rt_constraint_schema"`
	Name         string `db:"rt_constraint_name"`
	ParentSchema string `db:"rt_table_schema"`
	ParentName   string `db:"rt_table_name"`
	Type         string `db:"rt_constraint_type"`
}

type mysqlIndexObjectRow struct {
	Schema     string `db:"rt_table_schema"`
	Name       string `db:"rt_index_name"`
	ParentName string `db:"rt_table_name"`
	NonUnique  int    `db:"rt_non_unique"`
	IndexType  string `db:"rt_index_type"`
}

func (m *MySQL) ListObjects(
	ctx context.Context,
	filter database.ObjectFilter,
) ([]database.DatabaseObject, error) {
	if err := m.ensureConnected(); err != nil {
		return nil, err
	}
	databaseName := m.defaultDatabase(filter.Schema)
	if databaseName == "" {
		return nil, fmt.Errorf("database name is required to list MySQL objects")
	}
	capabilities := m.Capabilities()
	allowed := mysqlObjectKindSet(filter.Kinds)
	objects := make([]database.DatabaseObject, 0)

	var relations []mysqlRelationObjectRow
	if err := m.conn.SelectContext(ctx, &relations, `
		SELECT
			table_schema AS rt_table_schema,
			table_name AS rt_table_name,
			table_type AS rt_table_type,
			engine AS rt_engine,
			table_rows AS rt_table_rows,
			table_comment AS rt_table_comment
		FROM information_schema.tables
		WHERE table_schema = ?
		ORDER BY table_name`, databaseName); err != nil {
		return nil, err
	}
	for _, row := range relations {
		kind := database.ObjectKindTable
		if strings.EqualFold(row.TableType, "VIEW") ||
			strings.EqualFold(row.TableType, "SYSTEM VIEW") {
			kind = database.ObjectKindView
		}
		properties := []database.ObjectProperty{
			{Name: "Table type", Value: row.TableType, Category: "storage"},
		}
		if row.Engine.Valid {
			properties = append(properties, database.ObjectProperty{
				Name: "Engine", Value: row.Engine.String, Category: "storage",
			})
		}
		if row.Rows.Valid {
			properties = append(properties, database.ObjectProperty{
				Name:     "Estimated rows",
				Value:    fmt.Sprintf("%d", row.Rows.Int64),
				Category: "storage",
			})
		}
		object := makeMySQLObject(database.ObjectReference{
			Kind:   kind,
			Schema: row.Schema,
			Name:   row.Name,
		}, row.Comment, properties, capabilities)
		if mysqlObjectMatches(object, allowed, filter.Search) {
			objects = append(objects, object)
		}
	}

	var routines []mysqlRoutineObjectRow
	if err := m.conn.SelectContext(ctx, &routines, `
		SELECT
			routine_schema AS rt_routine_schema,
			routine_name AS rt_routine_name,
			routine_type AS rt_routine_type,
			data_type AS rt_data_type,
			security_type AS rt_security_type,
			sql_data_access AS rt_sql_data_access,
			routine_comment AS rt_routine_comment
		FROM information_schema.routines
		WHERE routine_schema = ?
		ORDER BY routine_type, routine_name`, databaseName); err != nil {
		return nil, err
	}
	for _, row := range routines {
		kind := database.ObjectKindFunction
		if strings.EqualFold(row.Type, "PROCEDURE") {
			kind = database.ObjectKindProcedure
		}
		properties := []database.ObjectProperty{
			{Name: "Security", Value: row.SecurityType, Category: "routine"},
			{Name: "SQL data access", Value: row.SQLData, Category: "routine"},
		}
		if row.ReturnType.Valid && row.ReturnType.String != "" {
			properties = append(properties, database.ObjectProperty{
				Name: "Returns", Value: row.ReturnType.String, Category: "routine",
			})
		}
		object := makeMySQLObject(database.ObjectReference{
			Kind:   kind,
			Schema: row.Schema,
			Name:   row.Name,
		}, row.Comment, properties, capabilities)
		if mysqlObjectMatches(object, allowed, filter.Search) {
			objects = append(objects, object)
		}
	}

	var triggers []mysqlTriggerObjectRow
	if err := m.conn.SelectContext(ctx, &triggers, `
		SELECT
			trigger_schema AS rt_trigger_schema,
			trigger_name AS rt_trigger_name,
			event_object_schema AS rt_event_object_schema,
			event_object_table AS rt_event_object_table,
			event_manipulation AS rt_event_manipulation,
			action_timing AS rt_action_timing
		FROM information_schema.triggers
		WHERE trigger_schema = ?
		ORDER BY event_object_table, trigger_name`, databaseName); err != nil {
		return nil, err
	}
	for _, row := range triggers {
		object := makeMySQLObject(database.ObjectReference{
			Kind:         database.ObjectKindTrigger,
			Schema:       row.Schema,
			Name:         row.Name,
			ParentSchema: row.ParentSchema,
			ParentName:   row.ParentName,
		}, "", []database.ObjectProperty{
			{Name: "Timing", Value: row.Timing, Category: "trigger"},
			{Name: "Event", Value: row.Event, Category: "trigger"},
			{
				Name:     "Parent",
				Value:    row.ParentSchema + "." + row.ParentName,
				Category: "identity",
			},
			{Name: "Enabled", Value: "true", Category: "trigger"},
		}, capabilities)
		if mysqlObjectMatches(object, allowed, filter.Search) {
			objects = append(objects, object)
		}
	}

	var constraints []mysqlConstraintObjectRow
	if err := m.conn.SelectContext(ctx, &constraints, `
		SELECT
			constraint_schema AS rt_constraint_schema,
			constraint_name AS rt_constraint_name,
			table_schema AS rt_table_schema,
			table_name AS rt_table_name,
			constraint_type AS rt_constraint_type
		FROM information_schema.table_constraints
		WHERE table_schema = ?
		ORDER BY table_name, constraint_name`, databaseName); err != nil {
		return nil, err
	}
	for _, row := range constraints {
		object := makeMySQLObject(database.ObjectReference{
			Kind:         database.ObjectKindConstraint,
			Schema:       row.Schema,
			Name:         row.Name,
			ParentSchema: row.ParentSchema,
			ParentName:   row.ParentName,
		}, row.Type, []database.ObjectProperty{
			{Name: "Constraint type", Value: row.Type, Category: "constraint"},
			{
				Name:     "Parent",
				Value:    row.ParentSchema + "." + row.ParentName,
				Category: "identity",
			},
		}, capabilities)
		if mysqlObjectMatches(object, allowed, filter.Search) {
			objects = append(objects, object)
		}
	}

	var indexes []mysqlIndexObjectRow
	if err := m.conn.SelectContext(ctx, &indexes, `
		SELECT
			table_schema AS rt_table_schema,
			table_name AS rt_table_name,
			index_name AS rt_index_name,
			MIN(non_unique) AS rt_non_unique,
			MAX(index_type) AS rt_index_type
		FROM information_schema.statistics
		WHERE table_schema = ?
		GROUP BY table_schema, table_name, index_name
		ORDER BY table_name, index_name`, databaseName); err != nil {
		return nil, err
	}
	for _, row := range indexes {
		object := makeMySQLObject(database.ObjectReference{
			Kind:         database.ObjectKindIndex,
			Schema:       row.Schema,
			Name:         row.Name,
			ParentSchema: row.Schema,
			ParentName:   row.ParentName,
		}, "", []database.ObjectProperty{
			{Name: "Method", Value: row.IndexType, Category: "index"},
			{
				Name:     "Unique",
				Value:    fmt.Sprintf("%t", row.NonUnique == 0),
				Category: "index",
			},
			{
				Name:     "Parent",
				Value:    row.Schema + "." + row.ParentName,
				Category: "identity",
			},
		}, capabilities)
		if mysqlObjectMatches(object, allowed, filter.Search) {
			objects = append(objects, object)
		}
	}

	sort.SliceStable(objects, func(left, right int) bool {
		if objects[left].Reference.Kind != objects[right].Reference.Kind {
			return objects[left].Reference.Kind < objects[right].Reference.Kind
		}
		if objects[left].Reference.ParentName != objects[right].Reference.ParentName {
			return objects[left].Reference.ParentName <
				objects[right].Reference.ParentName
		}
		return objects[left].DisplayName < objects[right].DisplayName
	})
	return objects, nil
}

func (m *MySQL) resolveObject(
	ctx context.Context,
	reference database.ObjectReference,
) (database.DatabaseObject, error) {
	objects, err := m.ListObjects(ctx, database.ObjectFilter{
		Schema: reference.Schema,
		Kinds: func() []database.ObjectKind {
			if reference.Kind == database.ObjectKindUnknown {
				return nil
			}
			return []database.ObjectKind{reference.Kind}
		}(),
	})
	if err != nil {
		return database.DatabaseObject{}, err
	}
	for _, object := range objects {
		candidate := object.Reference
		if reference.ID != "" && candidate.ID == reference.ID {
			return object, nil
		}
		if candidate.Kind != reference.Kind ||
			candidate.Name != reference.Name {
			continue
		}
		if reference.ParentName != "" &&
			candidate.ParentName != reference.ParentName {
			continue
		}
		return object, nil
	}
	return database.DatabaseObject{}, fmt.Errorf(
		"MySQL object %s was not found",
		reference.QualifiedName(),
	)
}

func (m *MySQL) showCreate(
	ctx context.Context,
	keyword string,
	reference database.ObjectReference,
) (string, error) {
	query := fmt.Sprintf(
		"SHOW CREATE %s %s",
		keyword,
		quoteMySQLQualifiedIdentifier(reference.Schema, reference.Name),
	)
	rows, err := m.conn.QueryxContext(ctx, query)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", fmt.Errorf("%s %s was not found", keyword, reference.Name)
	}
	values := make(map[string]interface{})
	if err := rows.MapScan(values); err != nil {
		return "", err
	}
	for key, value := range values {
		if strings.HasPrefix(strings.ToLower(key), "create ") {
			definition := fmt.Sprint(normalizeMySQLValue(value))
			if !strings.HasSuffix(strings.TrimSpace(definition), ";") {
				definition += ";"
			}
			return definition, nil
		}
	}
	return "", fmt.Errorf("SHOW CREATE %s returned no definition", keyword)
}

func (m *MySQL) constraintDefinition(
	ctx context.Context,
	reference database.ObjectReference,
) (string, error) {
	parentSchema := reference.ParentSchema
	if parentSchema == "" {
		parentSchema = reference.Schema
	}
	var constraintType string
	if err := m.conn.GetContext(ctx, &constraintType, `
		SELECT constraint_type
		FROM information_schema.table_constraints
		WHERE constraint_schema = ?
		  AND constraint_name = ?
		  AND table_schema = ?
		  AND table_name = ?`,
		reference.Schema,
		reference.Name,
		parentSchema,
		reference.ParentName,
	); err != nil {
		return "", err
	}

	var definition string
	switch strings.ToUpper(constraintType) {
	case "PRIMARY KEY", "UNIQUE":
		var columns []string
		if err := m.conn.SelectContext(ctx, &columns, `
			SELECT column_name
			FROM information_schema.key_column_usage
			WHERE constraint_schema = ?
			  AND constraint_name = ?
			  AND table_schema = ?
			  AND table_name = ?
			ORDER BY ordinal_position`,
			reference.Schema,
			reference.Name,
			parentSchema,
			reference.ParentName,
		); err != nil {
			return "", err
		}
		quoted := make([]string, len(columns))
		for index, column := range columns {
			quoted[index] = quoteMySQLIdentifier(column)
		}
		definition = strings.ToUpper(constraintType) +
			" (" + strings.Join(quoted, ", ") + ")"

	case "FOREIGN KEY":
		type foreignRow struct {
			Column        string `db:"rt_column_name"`
			ForeignSchema string `db:"rt_referenced_table_schema"`
			ForeignTable  string `db:"rt_referenced_table_name"`
			ForeignColumn string `db:"rt_referenced_column_name"`
		}
		var rows []foreignRow
		if err := m.conn.SelectContext(ctx, &rows, `
			SELECT
				column_name AS rt_column_name,
				referenced_table_schema AS rt_referenced_table_schema,
				referenced_table_name AS rt_referenced_table_name,
				referenced_column_name AS rt_referenced_column_name
			FROM information_schema.key_column_usage
			WHERE constraint_schema = ?
			  AND constraint_name = ?
			  AND table_schema = ?
			  AND table_name = ?
			ORDER BY ordinal_position`,
			reference.Schema,
			reference.Name,
			parentSchema,
			reference.ParentName,
		); err != nil {
			return "", err
		}
		if len(rows) == 0 {
			return "", fmt.Errorf("foreign-key columns were not found")
		}
		local := make([]string, len(rows))
		foreign := make([]string, len(rows))
		for index, row := range rows {
			local[index] = quoteMySQLIdentifier(row.Column)
			foreign[index] = quoteMySQLIdentifier(row.ForeignColumn)
		}
		definition = fmt.Sprintf(
			"FOREIGN KEY (%s) REFERENCES %s (%s)",
			strings.Join(local, ", "),
			quoteMySQLQualifiedIdentifier(
				rows[0].ForeignSchema,
				rows[0].ForeignTable,
			),
			strings.Join(foreign, ", "),
		)
		type rules struct {
			UpdateRule string `db:"rt_update_rule"`
			DeleteRule string `db:"rt_delete_rule"`
		}
		var referentialRules rules
		if err := m.conn.GetContext(ctx, &referentialRules, `
			SELECT
				update_rule AS rt_update_rule,
				delete_rule AS rt_delete_rule
			FROM information_schema.referential_constraints
			WHERE constraint_schema = ?
			  AND constraint_name = ?`,
			reference.Schema,
			reference.Name,
		); err == nil {
			definition += " ON UPDATE " + referentialRules.UpdateRule +
				" ON DELETE " + referentialRules.DeleteRule
		}

	case "CHECK":
		if err := m.conn.GetContext(ctx, &definition, `
			SELECT check_clause
			FROM information_schema.check_constraints
			WHERE constraint_schema = ? AND constraint_name = ?`,
			reference.Schema,
			reference.Name,
		); err != nil {
			return "", err
		}
		definition = "CHECK (" + strings.TrimSpace(definition) + ")"
	default:
		return "", fmt.Errorf(
			"unsupported MySQL constraint type %q",
			constraintType,
		)
	}
	return fmt.Sprintf(
		"ALTER TABLE %s ADD CONSTRAINT %s %s;",
		quoteMySQLQualifiedIdentifier(parentSchema, reference.ParentName),
		quoteMySQLIdentifier(reference.Name),
		definition,
	), nil
}

func (m *MySQL) indexDefinition(
	reference database.ObjectReference,
) (string, error) {
	parentSchema := reference.ParentSchema
	if parentSchema == "" {
		parentSchema = reference.Schema
	}
	indices, err := m.GetIndices(database.Table{
		Schema: parentSchema,
		Name:   reference.ParentName,
	})
	if err != nil {
		return "", err
	}
	for _, index := range indices {
		if index.Name != reference.Name {
			continue
		}
		columns := make([]string, len(index.Columns))
		for position, column := range index.Columns {
			columns[position] = quoteMySQLIdentifier(column)
		}
		unique := ""
		if index.IsUnique {
			unique = "UNIQUE "
		}
		if index.IsPrimary {
			return fmt.Sprintf(
				"ALTER TABLE %s ADD PRIMARY KEY (%s);",
				quoteMySQLQualifiedIdentifier(parentSchema, reference.ParentName),
				strings.Join(columns, ", "),
			), nil
		}
		return fmt.Sprintf(
			"CREATE %sINDEX %s ON %s (%s) USING %s;",
			unique,
			quoteMySQLIdentifier(index.Name),
			quoteMySQLQualifiedIdentifier(parentSchema, reference.ParentName),
			strings.Join(columns, ", "),
			index.Algorithm,
		), nil
	}
	return "", fmt.Errorf("index %q was not found", reference.Name)
}

func (m *MySQL) objectDefinition(
	ctx context.Context,
	object database.DatabaseObject,
) (string, []database.ObjectProperty, database.Structures, error) {
	reference := object.Reference
	properties := append([]database.ObjectProperty(nil), object.Properties...)
	switch reference.Kind {
	case database.ObjectKindTable:
		definition, err := m.GetTableDDL(database.Table{
			Schema: reference.Schema,
			Name:   reference.Name,
		})
		if err != nil {
			return "", nil, nil, err
		}
		columns, err := m.GetCollectionStructures(database.Table{
			Schema: reference.Schema,
			Name:   reference.Name,
		})
		return definition, properties, columns, err
	case database.ObjectKindView:
		definition, err := m.showCreate(ctx, "VIEW", reference)
		if err != nil {
			return "", nil, nil, err
		}
		columns, err := m.GetCollectionStructures(database.Table{
			Schema: reference.Schema,
			Name:   reference.Name,
		})
		return definition, properties, columns, err
	case database.ObjectKindFunction:
		definition, err := m.showCreate(ctx, "FUNCTION", reference)
		return definition, properties, nil, err
	case database.ObjectKindProcedure:
		definition, err := m.showCreate(ctx, "PROCEDURE", reference)
		return definition, properties, nil, err
	case database.ObjectKindTrigger:
		definition, err := m.showCreate(ctx, "TRIGGER", reference)
		return definition, properties, nil, err
	case database.ObjectKindConstraint:
		definition, err := m.constraintDefinition(ctx, reference)
		return definition, properties, nil, err
	case database.ObjectKindIndex:
		definition, err := m.indexDefinition(reference)
		return definition, properties, nil, err
	default:
		return "", nil, nil, fmt.Errorf(
			"definition is not available for MySQL %s objects",
			reference.Kind,
		)
	}
}

func mysqlTableDependency(
	kind database.ObjectKind,
	schema string,
	name string,
	description string,
) database.ObjectDependency {
	reference := database.ObjectReference{
		Kind:   kind,
		Schema: schema,
		Name:   name,
	}
	reference.ID = mysqlObjectID(reference)
	return database.ObjectDependency{
		Reference:   reference,
		Description: description,
	}
}

type mysqlViewCandidateRow struct {
	Schema string `db:"rt_table_schema"`
	Name   string `db:"rt_table_name"`
	Type   string `db:"rt_table_type"`
}

func mysqlViewCandidateKind(value string) database.ObjectKind {
	if strings.Contains(strings.ToUpper(strings.TrimSpace(value)), "VIEW") {
		return database.ObjectKindView
	}
	return database.ObjectKindTable
}

func inferMySQLViewDependencies(
	definition string,
	candidates []mysqlViewCandidateRow,
) []database.ObjectDependency {
	result := make([]database.ObjectDependency, 0)
	for _, candidate := range candidates {
		if !database.SQLReferencesIdentifier(definition, candidate.Name) {
			continue
		}
		result = append(result, mysqlTableDependency(
			mysqlViewCandidateKind(candidate.Type),
			candidate.Schema,
			candidate.Name,
			"Inferred from the stored view definition",
		))
	}
	return result
}

func (m *MySQL) inferredViewDependencies(
	ctx context.Context,
	reference database.ObjectReference,
) ([]database.ObjectDependency, error) {
	var definition sql.NullString
	if err := m.conn.GetContext(ctx, &definition, `
		SELECT view_definition
		FROM information_schema.views
		WHERE table_schema = ? AND table_name = ?`,
		reference.Schema,
		reference.Name,
	); err != nil || !definition.Valid {
		return nil, nil
	}
	var candidates []mysqlViewCandidateRow
	if err := m.conn.SelectContext(ctx, &candidates, `
		SELECT
			table_schema AS rt_table_schema,
			table_name AS rt_table_name,
			table_type AS rt_table_type
		FROM information_schema.tables
		WHERE table_schema = ? AND table_name <> ?
		ORDER BY table_name`,
		reference.Schema,
		reference.Name,
	); err != nil {
		return nil, err
	}
	return inferMySQLViewDependencies(definition.String, candidates), nil
}

func (m *MySQL) inferredViewDependents(
	ctx context.Context,
	reference database.ObjectReference,
) ([]database.ObjectDependency, error) {
	type viewDefinitionRow struct {
		Schema     string         `db:"rt_view_schema"`
		Name       string         `db:"rt_view_name"`
		Definition sql.NullString `db:"rt_view_definition"`
	}
	var views []viewDefinitionRow
	if err := m.conn.SelectContext(ctx, &views, `
		SELECT
			table_schema AS rt_view_schema,
			table_name AS rt_view_name,
			view_definition AS rt_view_definition
		FROM information_schema.views
		WHERE table_schema = ? AND table_name <> ?
		ORDER BY table_name`,
		reference.Schema,
		reference.Name,
	); err != nil {
		return nil, err
	}
	result := make([]database.ObjectDependency, 0)
	for _, view := range views {
		if !view.Definition.Valid ||
			!database.SQLReferencesIdentifier(
				view.Definition.String,
				reference.Name,
			) {
			continue
		}
		result = append(result, mysqlTableDependency(
			database.ObjectKindView,
			view.Schema,
			view.Name,
			"Inferred from the stored view definition",
		))
	}
	return result, nil
}

func (m *MySQL) objectDependencies(
	ctx context.Context,
	object database.DatabaseObject,
) ([]database.ObjectDependency, []database.ObjectDependency, error) {
	reference := object.Reference
	dependencies := make([]database.ObjectDependency, 0)
	dependents := make([]database.ObjectDependency, 0)
	seenDependencies := make(map[string]struct{})
	seenDependents := make(map[string]struct{})
	add := func(
		target *[]database.ObjectDependency,
		seen map[string]struct{},
		dependency database.ObjectDependency,
	) {
		key := string(dependency.Reference.Kind) + ":" +
			dependency.Reference.QualifiedName()
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		*target = append(*target, dependency)
	}

	parentSchema := reference.ParentSchema
	if parentSchema == "" {
		parentSchema = reference.Schema
	}
	if reference.ParentName != "" {
		add(
			&dependencies,
			seenDependencies,
			mysqlTableDependency(
				database.ObjectKindTable,
				parentSchema,
				reference.ParentName,
				"Parent table",
			),
		)
	}

	type usageRow struct {
		Schema string `db:"rt_table_schema"`
		Name   string `db:"rt_table_name"`
	}
	if reference.Kind == database.ObjectKindView {
		var rows []usageRow
		usageErr := m.conn.SelectContext(ctx, &rows, `
			SELECT
				table_schema AS rt_table_schema,
				table_name AS rt_table_name
			FROM information_schema.view_table_usage
			WHERE view_schema = ? AND view_name = ?`,
			reference.Schema,
			reference.Name,
		)
		if usageErr == nil {
			for _, row := range rows {
				add(
					&dependencies,
					seenDependencies,
					mysqlTableDependency(
						database.ObjectKindTable,
						row.Schema,
						row.Name,
						"Referenced by view definition",
					),
				)
			}
		}
		if usageErr != nil {
			inferred, err := m.inferredViewDependencies(ctx, reference)
			if err != nil {
				return nil, nil, err
			}
			for _, dependency := range inferred {
				add(&dependencies, seenDependencies, dependency)
			}
		}
	}
	if reference.Kind == database.ObjectKindFunction ||
		reference.Kind == database.ObjectKindProcedure {
		var rows []usageRow
		if err := m.conn.SelectContext(ctx, &rows, `
			SELECT
				table_schema AS rt_table_schema,
				table_name AS rt_table_name
			FROM information_schema.routine_table_usage
			WHERE specific_schema = ? AND specific_name = ?`,
			reference.Schema,
			reference.Name,
		); err == nil {
			for _, row := range rows {
				add(
					&dependencies,
					seenDependencies,
					mysqlTableDependency(
						database.ObjectKindTable,
						row.Schema,
						row.Name,
						"Referenced by routine",
					),
				)
			}
		}
	}
	if reference.Kind == database.ObjectKindTable ||
		reference.Kind == database.ObjectKindView {
		var outbound []usageRow
		if err := m.conn.SelectContext(ctx, &outbound, `
			SELECT DISTINCT
				referenced_table_schema AS rt_table_schema,
				referenced_table_name AS rt_table_name
			FROM information_schema.key_column_usage
			WHERE table_schema = ?
			  AND table_name = ?
			  AND referenced_table_name IS NOT NULL`,
			reference.Schema,
			reference.Name,
		); err != nil {
			return nil, nil, err
		}
		for _, row := range outbound {
			add(
				&dependencies,
				seenDependencies,
				mysqlTableDependency(
					database.ObjectKindTable,
					row.Schema,
					row.Name,
					"Referenced by foreign key",
				),
			)
		}

		var inbound []usageRow
		if err := m.conn.SelectContext(ctx, &inbound, `
			SELECT DISTINCT
				table_schema AS rt_table_schema,
				table_name AS rt_table_name
			FROM information_schema.key_column_usage
			WHERE referenced_table_schema = ?
			  AND referenced_table_name = ?`,
			reference.Schema,
			reference.Name,
		); err != nil {
			return nil, nil, err
		}
		for _, row := range inbound {
			add(
				&dependents,
				seenDependents,
				mysqlTableDependency(
					database.ObjectKindTable,
					row.Schema,
					row.Name,
					"References this object by foreign key",
				),
			)
		}

		type viewUsageRow struct {
			Schema string `db:"rt_view_schema"`
			Name   string `db:"rt_view_name"`
		}
		var views []viewUsageRow
		usageErr := m.conn.SelectContext(ctx, &views, `
			SELECT
				view_schema AS rt_view_schema,
				view_name AS rt_view_name
			FROM information_schema.view_table_usage
			WHERE table_schema = ? AND table_name = ?`,
			reference.Schema,
			reference.Name,
		)
		if usageErr == nil {
			for _, row := range views {
				add(
					&dependents,
					seenDependents,
					mysqlTableDependency(
						database.ObjectKindView,
						row.Schema,
						row.Name,
						"View definition references this object",
					),
				)
			}
		}
		if usageErr != nil {
			inferred, err := m.inferredViewDependents(ctx, reference)
			if err != nil {
				return nil, nil, err
			}
			for _, dependency := range inferred {
				add(&dependents, seenDependents, dependency)
			}
		}
	}
	return dependencies, dependents, nil
}

func sortMySQLProperties(properties []database.ObjectProperty) {
	sort.SliceStable(properties, func(left, right int) bool {
		if properties[left].Category != properties[right].Category {
			return properties[left].Category < properties[right].Category
		}
		return properties[left].Name < properties[right].Name
	})
}

func (m *MySQL) GetObjectDetail(
	ctx context.Context,
	reference database.ObjectReference,
) (database.ObjectDetail, error) {
	object, err := m.resolveObject(ctx, reference)
	if err != nil {
		return database.ObjectDetail{}, err
	}
	definition, properties, columns, err := m.objectDefinition(ctx, object)
	if err != nil {
		return database.ObjectDetail{}, err
	}
	dependencies, dependents, err := m.objectDependencies(ctx, object)
	if err != nil {
		return database.ObjectDetail{}, err
	}
	sortMySQLProperties(properties)
	return database.ObjectDetail{
		Object:       object,
		Definition:   definition,
		Comment:      object.Description,
		Properties:   properties,
		Columns:      columns,
		Dependencies: dependencies,
		Dependents:   dependents,
	}, nil
}

var _ database.ObjectDriver = (*MySQL)(nil)
