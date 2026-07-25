package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	"rollingthunder/pkg/database"
)

func sqliteObjectID(reference database.ObjectReference) string {
	parts := []string{
		string(reference.Kind),
		reference.Schema,
		reference.Name,
		reference.ParentSchema,
		reference.ParentName,
	}
	for index, part := range parts {
		parts[index] = base64.RawURLEncoding.EncodeToString([]byte(part))
	}
	return "sqlite:" + strings.Join(parts, ".")
}

func sqliteObjectCanManage(
	kind database.ObjectKind,
	capabilities database.Capabilities,
) bool {
	switch kind {
	case database.ObjectKindView:
		return capabilities.ManageViews
	case database.ObjectKindTrigger:
		return capabilities.ManageTriggers
	case database.ObjectKindIndex:
		return capabilities.ManageIndexes
	case database.ObjectKindTable, database.ObjectKindConstraint:
		return capabilities.AlterTableStructure
	default:
		return false
	}
}

func makeSQLiteObject(
	reference database.ObjectReference,
	description string,
	properties []database.ObjectProperty,
	capabilities database.Capabilities,
) database.DatabaseObject {
	reference.ID = sqliteObjectID(reference)
	allowedActions := make([]database.ObjectChangeAction, 0)
	switch reference.Kind {
	case database.ObjectKindTable:
		allowedActions = append(
			allowedActions,
			database.ObjectChangeRename,
			database.ObjectChangeDrop,
		)
	case database.ObjectKindView, database.ObjectKindTrigger:
		allowedActions = append(
			allowedActions,
			database.ObjectChangeReplace,
			database.ObjectChangeDrop,
		)
	case database.ObjectKindIndex:
		allowedActions = append(allowedActions, database.ObjectChangeDrop)
	}
	return database.DatabaseObject{
		Reference:   reference,
		DisplayName: reference.Name,
		Description: description,
		CanOpenData: reference.Kind == database.ObjectKindTable ||
			reference.Kind == database.ObjectKindView,
		CanManage: sqliteObjectCanManage(reference.Kind, capabilities) ||
			len(allowedActions) > 0,
		AllowedActions: allowedActions,
		Properties:     properties,
	}
}

type sqliteSchemaObjectRow struct {
	Type      string         `db:"type"`
	Name      string         `db:"name"`
	TableName string         `db:"tbl_name"`
	SQL       sql.NullString `db:"sql"`
}

func sqliteAllowedKinds(
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

func sqliteObjectMatches(
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
	return strings.Contains(strings.ToLower(strings.Join([]string{
		object.Reference.Name,
		object.Reference.ParentName,
		object.Description,
	}, " ")), search)
}

func sqliteKind(value string) database.ObjectKind {
	switch strings.ToLower(value) {
	case "table":
		return database.ObjectKindTable
	case "view":
		return database.ObjectKindView
	case "trigger":
		return database.ObjectKindTrigger
	case "index":
		return database.ObjectKindIndex
	default:
		return database.ObjectKindUnknown
	}
}

func (s *SQLite) objectsForSchema(
	ctx context.Context,
	schema string,
	allowed map[database.ObjectKind]struct{},
	search string,
) ([]database.DatabaseObject, error) {
	query := fmt.Sprintf(`
		SELECT type, name, tbl_name, sql
		FROM %s.sqlite_schema
		WHERE type IN ('table', 'view', 'trigger', 'index')
		  AND name NOT LIKE 'sqlite_%%'
		  AND sql IS NOT NULL
		ORDER BY type, name`,
		quoteSQLiteIdentifier(schema),
	)
	var rows []sqliteSchemaObjectRow
	if err := s.conn.SelectContext(ctx, &rows, query); err != nil {
		return nil, err
	}
	capabilities := s.Capabilities()
	objects := make([]database.DatabaseObject, 0, len(rows))
	tableNames := make([]string, 0)
	for _, row := range rows {
		kind := sqliteKind(row.Type)
		if kind == database.ObjectKindUnknown {
			continue
		}
		reference := database.ObjectReference{
			Kind:   kind,
			Schema: schema,
			Name:   row.Name,
		}
		if kind == database.ObjectKindTrigger ||
			kind == database.ObjectKindIndex {
			reference.ParentSchema = schema
			reference.ParentName = row.TableName
		}
		properties := []database.ObjectProperty{
			{Name: "Schema", Value: schema, Category: "identity"},
			{Name: "Catalog type", Value: row.Type, Category: "storage"},
		}
		if reference.ParentName != "" {
			properties = append(properties, database.ObjectProperty{
				Name:     "Parent",
				Value:    schema + "." + reference.ParentName,
				Category: "identity",
			})
		}
		if kind == database.ObjectKindTrigger {
			properties = append(properties, database.ObjectProperty{
				Name: "Enabled", Value: "true", Category: "trigger",
			})
		}
		object := makeSQLiteObject(
			reference,
			"",
			properties,
			capabilities,
		)
		if sqliteObjectMatches(object, allowed, search) {
			objects = append(objects, object)
		}
		if kind == database.ObjectKindTable {
			tableNames = append(tableNames, row.Name)
		}
	}

	for _, tableName := range tableNames {
		table := database.Table{Schema: schema, Name: tableName}
		structures, err := s.GetCollectionStructures(table)
		if err != nil {
			return nil, err
		}
		primaryColumns := make([]string, 0)
		for _, structure := range structures {
			if structure.IsPrimary {
				primaryColumns = append(primaryColumns, structure.Name)
			}
		}
		if len(primaryColumns) > 0 {
			name := "pk_" + tableName
			object := makeSQLiteObject(database.ObjectReference{
				Kind:         database.ObjectKindConstraint,
				Schema:       schema,
				Name:         name,
				ParentSchema: schema,
				ParentName:   tableName,
			}, "PRIMARY KEY", []database.ObjectProperty{
				{Name: "Constraint type", Value: "PRIMARY KEY", Category: "constraint"},
				{Name: "Columns", Value: strings.Join(primaryColumns, ", "), Category: "constraint"},
			}, capabilities)
			if sqliteObjectMatches(object, allowed, search) {
				objects = append(objects, object)
			}
		}
		foreignKeys, err := s.sqliteForeignKeys(table)
		if err != nil {
			return nil, err
		}
		grouped := make(map[int][]sqliteForeignKeyRow)
		for _, foreignKey := range foreignKeys {
			grouped[foreignKey.ID] = append(grouped[foreignKey.ID], foreignKey)
		}
		for id, group := range grouped {
			name := fmt.Sprintf("fk_%s_%d", tableName, id)
			object := makeSQLiteObject(database.ObjectReference{
				Kind:         database.ObjectKindConstraint,
				Schema:       schema,
				Name:         name,
				ParentSchema: schema,
				ParentName:   tableName,
			}, "FOREIGN KEY", []database.ObjectProperty{
				{Name: "Constraint type", Value: "FOREIGN KEY", Category: "constraint"},
				{Name: "References", Value: group[0].ForeignTable, Category: "constraint"},
				{Name: "On update", Value: group[0].OnUpdate, Category: "constraint"},
				{Name: "On delete", Value: group[0].OnDelete, Category: "constraint"},
			}, capabilities)
			if sqliteObjectMatches(object, allowed, search) {
				objects = append(objects, object)
			}
		}
	}
	return objects, nil
}

func (s *SQLite) ListObjects(
	ctx context.Context,
	filter database.ObjectFilter,
) ([]database.DatabaseObject, error) {
	if err := s.ensureConnected(); err != nil {
		return nil, err
	}
	schemas := make([]string, 0)
	if strings.TrimSpace(filter.Schema) != "" {
		schemas = append(schemas, normalizeSQLiteSchema(filter.Schema))
	} else {
		var err error
		schemas, err = s.GetSchemas()
		if err != nil {
			return nil, err
		}
	}
	allowed := sqliteAllowedKinds(filter.Kinds)
	objects := make([]database.DatabaseObject, 0)
	for _, schema := range schemas {
		schemaObjects, err := s.objectsForSchema(
			ctx,
			schema,
			allowed,
			filter.Search,
		)
		if err != nil {
			return nil, err
		}
		objects = append(objects, schemaObjects...)
	}
	sort.SliceStable(objects, func(left, right int) bool {
		if objects[left].Reference.Schema != objects[right].Reference.Schema {
			return objects[left].Reference.Schema <
				objects[right].Reference.Schema
		}
		if objects[left].Reference.Kind != objects[right].Reference.Kind {
			return objects[left].Reference.Kind < objects[right].Reference.Kind
		}
		if objects[left].Reference.ParentName != objects[right].Reference.ParentName {
			return objects[left].Reference.ParentName <
				objects[right].Reference.ParentName
		}
		return objects[left].Reference.Name < objects[right].Reference.Name
	})
	return objects, nil
}

func (s *SQLite) resolveObject(
	ctx context.Context,
	reference database.ObjectReference,
) (database.DatabaseObject, error) {
	objects, err := s.ListObjects(ctx, database.ObjectFilter{
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
		if candidate.Kind == reference.Kind &&
			candidate.Name == reference.Name &&
			(reference.ParentName == "" ||
				candidate.ParentName == reference.ParentName) {
			return object, nil
		}
	}
	return database.DatabaseObject{}, fmt.Errorf(
		"SQLite object %s was not found",
		reference.QualifiedName(),
	)
}

func (s *SQLite) schemaObjectDefinition(
	ctx context.Context,
	reference database.ObjectReference,
) (string, error) {
	schema := normalizeSQLiteSchema(reference.Schema)
	query := fmt.Sprintf(
		"SELECT sql FROM %s.sqlite_schema WHERE type = ? AND name = ?",
		quoteSQLiteIdentifier(schema),
	)
	catalogType := string(reference.Kind)
	var definition sql.NullString
	if err := s.conn.GetContext(
		ctx,
		&definition,
		query,
		catalogType,
		reference.Name,
	); err != nil {
		return "", err
	}
	if !definition.Valid || strings.TrimSpace(definition.String) == "" {
		return "", fmt.Errorf("SQLite object has no stored DDL")
	}
	ddl := strings.TrimSpace(definition.String)
	if !strings.HasSuffix(ddl, ";") {
		ddl += ";"
	}
	return ddl, nil
}

func parseSQLiteForeignConstraintID(
	name string,
	table string,
) (int, error) {
	prefix := "fk_" + table + "_"
	if !strings.HasPrefix(name, prefix) {
		return 0, fmt.Errorf("invalid SQLite foreign-key identity")
	}
	var id int
	if _, err := fmt.Sscanf(strings.TrimPrefix(name, prefix), "%d", &id); err != nil {
		return 0, fmt.Errorf("invalid SQLite foreign-key identity: %w", err)
	}
	return id, nil
}

func (s *SQLite) constraintDefinition(
	reference database.ObjectReference,
) (string, error) {
	table := database.Table{
		Schema: normalizeSQLiteSchema(reference.ParentSchema),
		Name:   reference.ParentName,
	}
	if table.Schema == "main" && reference.ParentSchema == "" {
		table.Schema = normalizeSQLiteSchema(reference.Schema)
	}
	if reference.Name == "pk_"+reference.ParentName {
		structures, err := s.GetCollectionStructures(table)
		if err != nil {
			return "", err
		}
		columns := make([]string, 0)
		for _, structure := range structures {
			if structure.IsPrimary {
				columns = append(columns, quoteSQLiteIdentifier(structure.Name))
			}
		}
		if len(columns) == 0 {
			return "", fmt.Errorf("primary-key columns were not found")
		}
		return fmt.Sprintf(
			"-- SQLite stores this constraint in the table definition.\n"+
				"PRIMARY KEY (%s)",
			strings.Join(columns, ", "),
		), nil
	}
	id, err := parseSQLiteForeignConstraintID(
		reference.Name,
		reference.ParentName,
	)
	if err != nil {
		return "", err
	}
	foreignKeys, err := s.sqliteForeignKeys(table)
	if err != nil {
		return "", err
	}
	group := make([]sqliteForeignKeyRow, 0)
	for _, foreignKey := range foreignKeys {
		if foreignKey.ID == id {
			group = append(group, foreignKey)
		}
	}
	if len(group) == 0 {
		return "", fmt.Errorf("foreign-key metadata was not found")
	}
	local := make([]string, len(group))
	foreign := make([]string, len(group))
	for index, item := range group {
		local[index] = quoteSQLiteIdentifier(item.Column)
		if item.ForeignCol.Valid {
			foreign[index] = quoteSQLiteIdentifier(item.ForeignCol.String)
		}
	}
	return fmt.Sprintf(
		"-- SQLite stores this constraint in the table definition.\n"+
			"FOREIGN KEY (%s) REFERENCES %s (%s) ON UPDATE %s ON DELETE %s",
		strings.Join(local, ", "),
		quoteSQLiteIdentifier(group[0].ForeignTable),
		strings.Join(foreign, ", "),
		group[0].OnUpdate,
		group[0].OnDelete,
	), nil
}

func (s *SQLite) objectDefinition(
	ctx context.Context,
	object database.DatabaseObject,
) (string, []database.ObjectProperty, database.Structures, error) {
	reference := object.Reference
	properties := append([]database.ObjectProperty(nil), object.Properties...)
	for _, row := range func() []sqliteDatabaseRow {
		rows, _ := s.databaseList()
		return rows
	}() {
		if row.Name == reference.Schema && row.File.Valid {
			properties = append(properties, database.ObjectProperty{
				Name: "File", Value: row.File.String, Category: "storage",
			})
			break
		}
	}

	switch reference.Kind {
	case database.ObjectKindTable:
		definition, err := s.GetTableDDL(database.Table{
			Schema: reference.Schema,
			Name:   reference.Name,
		})
		if err != nil {
			return "", nil, nil, err
		}
		columns, err := s.GetCollectionStructures(database.Table{
			Schema: reference.Schema,
			Name:   reference.Name,
		})
		return definition, properties, columns, err
	case database.ObjectKindView:
		definition, err := s.schemaObjectDefinition(ctx, reference)
		if err != nil {
			return "", nil, nil, err
		}
		columns, err := s.GetCollectionStructures(database.Table{
			Schema: reference.Schema,
			Name:   reference.Name,
		})
		return definition, properties, columns, err
	case database.ObjectKindTrigger, database.ObjectKindIndex:
		definition, err := s.schemaObjectDefinition(ctx, reference)
		return definition, properties, nil, err
	case database.ObjectKindConstraint:
		definition, err := s.constraintDefinition(reference)
		return definition, properties, nil, err
	default:
		return "", nil, nil, fmt.Errorf(
			"definition is not available for SQLite %s objects",
			reference.Kind,
		)
	}
}

func sqliteObjectReference(
	kind database.ObjectKind,
	schema string,
	name string,
) database.ObjectReference {
	reference := database.ObjectReference{
		Kind:   kind,
		Schema: schema,
		Name:   name,
	}
	reference.ID = sqliteObjectID(reference)
	return reference
}

func sqliteSQLReferencesName(sqlText, name string) bool {
	return database.SQLReferencesIdentifier(sqlText, name)
}

func (s *SQLite) objectDependencies(
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
		parent := sqliteObjectReference(
			database.ObjectKindTable,
			parentSchema,
			reference.ParentName,
		)
		add(&dependencies, seenDependencies, database.ObjectDependency{
			Reference: parent, Description: "Parent table",
		})
	}

	if reference.Kind == database.ObjectKindTable ||
		reference.Kind == database.ObjectKindView {
		foreignKeys, err := s.sqliteForeignKeys(database.Table{
			Schema: reference.Schema,
			Name:   reference.Name,
		})
		if err == nil {
			for _, foreignKey := range foreignKeys {
				target := sqliteObjectReference(
					database.ObjectKindTable,
					reference.Schema,
					foreignKey.ForeignTable,
				)
				add(&dependencies, seenDependencies, database.ObjectDependency{
					Reference:   target,
					Description: "Referenced by foreign key",
				})
			}
		}
		tables, err := s.GetCollections(reference.Schema)
		if err != nil {
			return nil, nil, err
		}
		for _, tableName := range tables {
			if tableName == reference.Name {
				continue
			}
			foreignKeys, err := s.sqliteForeignKeys(database.Table{
				Schema: reference.Schema,
				Name:   tableName,
			})
			if err != nil {
				return nil, nil, err
			}
			for _, foreignKey := range foreignKeys {
				if foreignKey.ForeignTable != reference.Name {
					continue
				}
				source := sqliteObjectReference(
					database.ObjectKindTable,
					reference.Schema,
					tableName,
				)
				add(&dependents, seenDependents, database.ObjectDependency{
					Reference:   source,
					Description: "References this table by foreign key",
				})
			}
		}
	}

	schema := normalizeSQLiteSchema(reference.Schema)
	query := fmt.Sprintf(`
		SELECT type, name, tbl_name, sql
		FROM %s.sqlite_schema
		WHERE type IN ('table', 'view', 'trigger')
		  AND sql IS NOT NULL
		  AND name NOT LIKE 'sqlite_%%'`,
		quoteSQLiteIdentifier(schema),
	)
	var rows []sqliteSchemaObjectRow
	if err := s.conn.SelectContext(ctx, &rows, query); err != nil {
		return nil, nil, err
	}
	if reference.Kind == database.ObjectKindView ||
		reference.Kind == database.ObjectKindTrigger {
		definition, err := s.schemaObjectDefinition(ctx, reference)
		if err == nil {
			for _, candidate := range rows {
				if candidate.Name == reference.Name ||
					!candidate.SQL.Valid ||
					!sqliteSQLReferencesName(definition, candidate.Name) {
					continue
				}
				kind := sqliteKind(candidate.Type)
				if kind != database.ObjectKindTable &&
					kind != database.ObjectKindView {
					continue
				}
				target := sqliteObjectReference(kind, schema, candidate.Name)
				add(&dependencies, seenDependencies, database.ObjectDependency{
					Reference:   target,
					Description: "Referenced by stored SQL definition",
				})
			}
		}
	}
	for _, candidate := range rows {
		if candidate.Name == reference.Name ||
			!candidate.SQL.Valid ||
			!sqliteSQLReferencesName(candidate.SQL.String, reference.Name) {
			continue
		}
		kind := sqliteKind(candidate.Type)
		if kind != database.ObjectKindView &&
			kind != database.ObjectKindTrigger {
			continue
		}
		target := sqliteObjectReference(kind, schema, candidate.Name)
		add(&dependents, seenDependents, database.ObjectDependency{
			Reference:   target,
			Description: "Stored SQL definition references this object",
		})
	}
	return dependencies, dependents, nil
}

func sortSQLiteProperties(properties []database.ObjectProperty) {
	sort.SliceStable(properties, func(left, right int) bool {
		if properties[left].Category != properties[right].Category {
			return properties[left].Category < properties[right].Category
		}
		return properties[left].Name < properties[right].Name
	})
}

func (s *SQLite) GetObjectDetail(
	ctx context.Context,
	reference database.ObjectReference,
) (database.ObjectDetail, error) {
	object, err := s.resolveObject(ctx, reference)
	if err != nil {
		return database.ObjectDetail{}, err
	}
	definition, properties, columns, err := s.objectDefinition(ctx, object)
	if err != nil {
		return database.ObjectDetail{}, err
	}
	dependencies, dependents, err := s.objectDependencies(ctx, object)
	if err != nil {
		return database.ObjectDetail{}, err
	}
	sortSQLiteProperties(properties)
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

var _ database.ObjectDriver = (*SQLite)(nil)
