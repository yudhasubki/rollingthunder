package sqlserver

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"rollingthunder/pkg/database"
)

func objectID(prefix string, values ...string) string {
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = base64.RawURLEncoding.EncodeToString([]byte(value))
	}
	return "sqlserver:" + prefix + ":" + strings.Join(parts, ".")
}

func objectKind(value string) database.ObjectKind {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "U":
		return database.ObjectKindTable
	case "V":
		return database.ObjectKindView
	case "P", "PC":
		return database.ObjectKindProcedure
	case "FN", "FS", "FT", "IF", "TF":
		return database.ObjectKindFunction
	case "TR", "TA":
		return database.ObjectKindTrigger
	case "SO":
		return database.ObjectKindSequence
	case "PK", "UQ", "F", "C", "D":
		return database.ObjectKindConstraint
	default:
		return database.ObjectKindUnknown
	}
}

func kindSet(kinds []database.ObjectKind) map[database.ObjectKind]struct{} {
	if len(kinds) == 0 {
		return nil
	}
	result := make(map[database.ObjectKind]struct{}, len(kinds))
	for _, kind := range kinds {
		result[kind] = struct{}{}
	}
	return result
}

func objectMatches(
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

func sqlServerObjectCanManage(
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

func sqlServerObjectAllowedActions(
	reference database.ObjectReference,
	capabilities database.Capabilities,
) []database.ObjectChangeAction {
	if !sqlServerObjectCanManage(reference.Kind, capabilities) {
		return nil
	}
	switch reference.Kind {
	case database.ObjectKindTable:
		return []database.ObjectChangeAction{
			database.ObjectChangeRename,
			database.ObjectChangeDrop,
		}
	case database.ObjectKindView,
		database.ObjectKindFunction,
		database.ObjectKindProcedure:
		return []database.ObjectChangeAction{
			database.ObjectChangeReplace,
			database.ObjectChangeRename,
			database.ObjectChangeDrop,
		}
	case database.ObjectKindTrigger:
		actions := []database.ObjectChangeAction{
			database.ObjectChangeReplace,
			database.ObjectChangeRename,
		}
		if capabilities.TriggerToggle && reference.ParentName != "" {
			actions = append(
				actions,
				database.ObjectChangeEnable,
				database.ObjectChangeDisable,
			)
		}
		return append(actions, database.ObjectChangeDrop)
	case database.ObjectKindIndex:
		return []database.ObjectChangeAction{
			database.ObjectChangeRename,
			database.ObjectChangeDrop,
		}
	case database.ObjectKindConstraint:
		return []database.ObjectChangeAction{database.ObjectChangeDrop}
	default:
		return nil
	}
}

func (s *SQLServer) ListObjects(
	ctx context.Context,
	filter database.ObjectFilter,
) ([]database.DatabaseObject, error) {
	if err := s.ensureConnected(); err != nil {
		return nil, err
	}
	schema := s.defaultSchema(filter.Schema)
	allowed := kindSet(filter.Kinds)
	capabilities := s.Capabilities()
	rows, err := s.conn.QueryContext(ctx, `
		SELECT
			object_value.object_id,
			object_value.type,
			object_value.type_desc,
			object_value.name,
			parent_schema.name,
			parent_object.name,
			object_value.create_date,
			object_value.modify_date,
			CONVERT(nvarchar(max), description.value),
			trigger_value.is_disabled
		FROM sys.objects object_value
		JOIN sys.schemas object_schema
			ON object_schema.schema_id = object_value.schema_id
		LEFT JOIN sys.objects parent_object
			ON parent_object.object_id = object_value.parent_object_id
		LEFT JOIN sys.schemas parent_schema
			ON parent_schema.schema_id = parent_object.schema_id
		LEFT JOIN sys.extended_properties description
			ON description.major_id = object_value.object_id
			AND description.minor_id = 0
			AND description.name = 'MS_Description'
			AND description.class = 1
		LEFT JOIN sys.triggers trigger_value
			ON trigger_value.object_id = object_value.object_id
		WHERE object_schema.name = @p1
			AND object_value.is_ms_shipped = 0
			AND object_value.type IN (
				'U', 'V', 'P', 'PC', 'FN', 'FS', 'FT', 'IF', 'TF',
				'TR', 'TA', 'SO', 'PK', 'UQ', 'F', 'C', 'D'
			)
		ORDER BY object_value.type_desc, object_value.name`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	objects := make([]database.DatabaseObject, 0)
	for rows.Next() {
		var (
			id           int64
			objectType   string
			typeDesc     string
			name         string
			parentSchema sql.NullString
			parentName   sql.NullString
			created      time.Time
			modified     time.Time
			description  sql.NullString
			triggerOff   sql.NullBool
		)
		if err := rows.Scan(
			&id,
			&objectType,
			&typeDesc,
			&name,
			&parentSchema,
			&parentName,
			&created,
			&modified,
			&description,
			&triggerOff,
		); err != nil {
			return nil, err
		}
		kind := objectKind(objectType)
		if kind == database.ObjectKindUnknown {
			continue
		}
		reference := database.ObjectReference{
			ID:     objectID("object", strconv.FormatInt(id, 10)),
			Kind:   kind,
			Schema: schema,
			Name:   name,
		}
		if parentName.Valid {
			reference.ParentSchema = parentSchema.String
			reference.ParentName = parentName.String
		}
		properties := []database.ObjectProperty{
			{Name: "Object type", Value: typeDesc, Category: "identity"},
			{Name: "Created", Value: created.Format(time.RFC3339), Category: "lifecycle"},
			{Name: "Modified", Value: modified.Format(time.RFC3339), Category: "lifecycle"},
		}
		if reference.ParentName != "" {
			properties = append(properties, database.ObjectProperty{
				Name:     "Parent",
				Value:    reference.ParentSchema + "." + reference.ParentName,
				Category: "identity",
			})
		}
		if kind == database.ObjectKindTrigger && triggerOff.Valid {
			properties = append(properties, database.ObjectProperty{
				Name:     "Enabled",
				Value:    strconv.FormatBool(!triggerOff.Bool),
				Category: "state",
			})
		}
		allowedActions := sqlServerObjectAllowedActions(
			reference,
			capabilities,
		)
		object := database.DatabaseObject{
			Reference:   reference,
			DisplayName: name,
			Description: description.String,
			CanOpenData: kind == database.ObjectKindTable ||
				kind == database.ObjectKindView,
			CanManage:      len(allowedActions) > 0,
			AllowedActions: allowedActions,
			Properties:     properties,
		}
		if objectMatches(object, allowed, filter.Search) {
			objects = append(objects, object)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	typeRows, err := s.conn.QueryContext(ctx, `
		SELECT type_object.user_type_id, type_object.name, base_type.name
		FROM sys.types type_object
		JOIN sys.schemas type_schema
			ON type_schema.schema_id = type_object.schema_id
		JOIN sys.types base_type
			ON base_type.user_type_id = type_object.system_type_id
			AND base_type.system_type_id = type_object.system_type_id
		WHERE type_schema.name = @p1
			AND type_object.is_user_defined = 1
		ORDER BY type_object.name`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	for typeRows.Next() {
		var id int64
		var name, baseType string
		if err := typeRows.Scan(&id, &name, &baseType); err != nil {
			_ = typeRows.Close()
			return nil, err
		}
		object := database.DatabaseObject{
			Reference: database.ObjectReference{
				ID:     objectID("type", strconv.FormatInt(id, 10)),
				Kind:   database.ObjectKindType,
				Schema: schema,
				Name:   name,
			},
			DisplayName: name,
			Description: "Alias type based on " + baseType,
			CanManage:   false,
			Properties: []database.ObjectProperty{
				{Name: "Base type", Value: baseType, Category: "type"},
			},
		}
		if objectMatches(object, allowed, filter.Search) {
			objects = append(objects, object)
		}
	}
	if err := typeRows.Close(); err != nil {
		return nil, err
	}

	indexRows, err := s.conn.QueryContext(ctx, `
		SELECT
			index_object.index_id,
			index_object.name,
			table_object.name,
			index_object.is_unique,
			index_object.type_desc
		FROM sys.indexes index_object
		JOIN sys.tables table_object
			ON table_object.object_id = index_object.object_id
		JOIN sys.schemas table_schema
			ON table_schema.schema_id = table_object.schema_id
		WHERE table_schema.name = @p1
			AND index_object.index_id > 0
			AND index_object.name IS NOT NULL
			AND index_object.is_hypothetical = 0
		ORDER BY index_object.name`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer indexRows.Close()
	for indexRows.Next() {
		var id int64
		var name, parentName, typeDesc string
		var unique bool
		if err := indexRows.Scan(
			&id,
			&name,
			&parentName,
			&unique,
			&typeDesc,
		); err != nil {
			return nil, err
		}
		object := database.DatabaseObject{
			Reference: database.ObjectReference{
				ID:           objectID("index", parentName, strconv.FormatInt(id, 10)),
				Kind:         database.ObjectKindIndex,
				Schema:       schema,
				Name:         name,
				ParentSchema: schema,
				ParentName:   parentName,
			},
			DisplayName: name,
			Properties: []database.ObjectProperty{
				{Name: "Parent", Value: schema + "." + parentName, Category: "identity"},
				{Name: "Type", Value: typeDesc, Category: "index"},
				{Name: "Unique", Value: strconv.FormatBool(unique), Category: "index"},
			},
		}
		object.AllowedActions = sqlServerObjectAllowedActions(
			object.Reference,
			capabilities,
		)
		object.CanManage = len(object.AllowedActions) > 0
		if objectMatches(object, allowed, filter.Search) {
			objects = append(objects, object)
		}
	}
	return objects, indexRows.Err()
}

func (s *SQLServer) objectDefinition(
	reference database.ObjectReference,
) (string, error) {
	switch reference.Kind {
	case database.ObjectKindTable:
		return s.GetTableDDL(database.Table{
			Schema: reference.Schema,
			Name:   reference.Name,
		})
	case database.ObjectKindIndex:
		return s.indexDefinition(reference)
	case database.ObjectKindType:
		var (
			baseType  string
			maxLength int64
			precision int64
			scale     int64
			nullable  bool
		)
		if err := s.conn.QueryRow(`
			SELECT
				base_type.name,
				type_object.max_length,
				type_object.precision,
				type_object.scale,
				type_object.is_nullable
			FROM sys.types type_object
			JOIN sys.schemas type_schema
				ON type_schema.schema_id = type_object.schema_id
			JOIN sys.types base_type
				ON base_type.user_type_id = type_object.system_type_id
				AND base_type.system_type_id = type_object.system_type_id
			WHERE type_schema.name = @p1 AND type_object.name = @p2`,
			reference.Schema,
			reference.Name,
		).Scan(
			&baseType,
			&maxLength,
			&precision,
			&scale,
			&nullable,
		); err != nil {
			return "", err
		}
		renderedType := displayType(columnMetadata{
			systemTypeName: baseType,
			maxLength:      maxLength,
			precision:      precision,
			scale:          scale,
		})
		nullability := " NOT NULL"
		if nullable {
			nullability = " NULL"
		}
		return "CREATE TYPE " + quoteQualified(reference.Schema, reference.Name) +
			" FROM " + renderedType + nullability + ";", nil
	case database.ObjectKindSequence:
		var (
			dataType         string
			start, increment string
			minimum, maximum string
			cycling, cached  bool
			cacheSize        sql.NullInt64
		)
		if err := s.conn.QueryRow(`
			SELECT
				TYPE_NAME(sequence_object.user_type_id),
				CONVERT(nvarchar(100), sequence_object.start_value),
				CONVERT(nvarchar(100), sequence_object.increment),
				CONVERT(nvarchar(100), sequence_object.minimum_value),
				CONVERT(nvarchar(100), sequence_object.maximum_value),
				sequence_object.is_cycling,
				sequence_object.is_cached,
				sequence_object.cache_size
			FROM sys.sequences sequence_object
			JOIN sys.schemas sequence_schema
				ON sequence_schema.schema_id = sequence_object.schema_id
			WHERE sequence_schema.name = @p1 AND sequence_object.name = @p2`,
			reference.Schema,
			reference.Name,
		).Scan(
			&dataType,
			&start,
			&increment,
			&minimum,
			&maximum,
			&cycling,
			&cached,
			&cacheSize,
		); err != nil {
			return "", err
		}
		cycle := "NO CYCLE"
		if cycling {
			cycle = "CYCLE"
		}
		cache := "NO CACHE"
		if cached && cacheSize.Valid {
			cache = fmt.Sprintf("CACHE %d", cacheSize.Int64)
		}
		return fmt.Sprintf(
			"CREATE SEQUENCE %s AS %s START WITH %s INCREMENT BY %s MINVALUE %s MAXVALUE %s %s %s;",
			quoteQualified(reference.Schema, reference.Name),
			dataType,
			start,
			increment,
			minimum,
			maximum,
			cycle,
			cache,
		), nil
	default:
		var definition sql.NullString
		qualified := quoteQualified(reference.Schema, reference.Name)
		if err := s.conn.QueryRow(
			"SELECT OBJECT_DEFINITION(OBJECT_ID(@p1))",
			qualified,
		).Scan(&definition); err != nil {
			return "", err
		}
		if !definition.Valid {
			return "", nil
		}
		result := strings.TrimSpace(definition.String)
		if result != "" && !strings.HasSuffix(result, ";") {
			result += ";"
		}
		return result, nil
	}
}

type sqlServerIndexMetadata struct {
	unique           bool
	primary          bool
	uniqueConstraint bool
	typeDescription  string
	filter           sql.NullString
	column           sql.NullString
	included         bool
	descending       bool
}

func formatSQLServerIndexDefinition(
	reference database.ObjectReference,
	rows []sqlServerIndexMetadata,
) (string, error) {
	if len(rows) == 0 {
		return "", fmt.Errorf("SQL Server index %q was not found", reference.Name)
	}
	metadata := rows[0]
	indexType := strings.ToUpper(
		strings.ReplaceAll(metadata.typeDescription, "_", " "),
	)
	keys := make([]string, 0, len(rows))
	includes := make([]string, 0, len(rows))
	for _, row := range rows {
		if !row.column.Valid || strings.TrimSpace(row.column.String) == "" {
			continue
		}
		column := quoteIdentifier(row.column.String)
		if row.included {
			includes = append(includes, column)
			continue
		}
		if strings.Contains(indexType, "COLUMNSTORE") {
			keys = append(keys, column)
			continue
		}
		if row.descending {
			column += " DESC"
		} else {
			column += " ASC"
		}
		keys = append(keys, column)
	}
	table := quoteQualified(reference.ParentSchema, reference.ParentName)
	name := quoteIdentifier(reference.Name)
	switch indexType {
	case "CLUSTERED", "NONCLUSTERED":
		if len(keys) == 0 {
			return "", fmt.Errorf(
				"SQL Server %s index %q has no key columns",
				indexType,
				reference.Name,
			)
		}
	case "CLUSTERED COLUMNSTORE":
		return "CREATE CLUSTERED COLUMNSTORE INDEX " + name +
			" ON " + table + ";", nil
	case "NONCLUSTERED COLUMNSTORE":
		if len(keys) == 0 {
			return "", fmt.Errorf(
				"SQL Server columnstore index %q has no columns",
				reference.Name,
			)
		}
		return "CREATE NONCLUSTERED COLUMNSTORE INDEX " + name +
			" ON " + table + " (" + strings.Join(keys, ", ") + ");", nil
	default:
		return fmt.Sprintf(
			"-- Rolling Thunder cannot safely reconstruct this %s index yet.\n-- Index: %s on %s",
			indexType,
			name,
			table,
		), nil
	}

	if metadata.primary || metadata.uniqueConstraint {
		constraintType := "UNIQUE"
		if metadata.primary {
			constraintType = "PRIMARY KEY"
		}
		return "ALTER TABLE " + table + " ADD CONSTRAINT " + name + " " +
			constraintType + " " + indexType + " (" +
			strings.Join(keys, ", ") + ");", nil
	}
	unique := ""
	if metadata.unique {
		unique = "UNIQUE "
	}
	statement := "CREATE " + unique + indexType + " INDEX " + name +
		" ON " + table + " (" + strings.Join(keys, ", ") + ")"
	if len(includes) > 0 {
		statement += " INCLUDE (" + strings.Join(includes, ", ") + ")"
	}
	if metadata.filter.Valid &&
		strings.TrimSpace(metadata.filter.String) != "" {
		statement += " WHERE " + strings.TrimSpace(metadata.filter.String)
	}
	return statement + ";", nil
}

func (s *SQLServer) indexDefinition(
	reference database.ObjectReference,
) (string, error) {
	rows, err := s.conn.Query(`
		SELECT
			index_object.is_unique,
			index_object.is_primary_key,
			index_object.is_unique_constraint,
			index_object.type_desc,
			index_object.filter_definition,
			column_object.name,
			CONVERT(bit, COALESCE(index_column.is_included_column, 0)),
			CONVERT(bit, COALESCE(index_column.is_descending_key, 0))
		FROM sys.indexes index_object
		JOIN sys.tables table_object
			ON table_object.object_id = index_object.object_id
		JOIN sys.schemas table_schema
			ON table_schema.schema_id = table_object.schema_id
		LEFT JOIN sys.index_columns index_column
			ON index_column.object_id = index_object.object_id
			AND index_column.index_id = index_object.index_id
		LEFT JOIN sys.columns column_object
			ON column_object.object_id = index_column.object_id
			AND column_object.column_id = index_column.column_id
		WHERE table_schema.name = @p1
			AND table_object.name = @p2
			AND index_object.name = @p3
		ORDER BY
			index_column.is_included_column,
			index_column.key_ordinal,
			index_column.index_column_id`,
		reference.ParentSchema,
		reference.ParentName,
		reference.Name,
	)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	metadata := make([]sqlServerIndexMetadata, 0)
	for rows.Next() {
		var row sqlServerIndexMetadata
		if err := rows.Scan(
			&row.unique,
			&row.primary,
			&row.uniqueConstraint,
			&row.typeDescription,
			&row.filter,
			&row.column,
			&row.included,
			&row.descending,
		); err != nil {
			return "", err
		}
		metadata = append(metadata, row)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return formatSQLServerIndexDefinition(reference, metadata)
}

func (s *SQLServer) GetObjectDetail(
	ctx context.Context,
	reference database.ObjectReference,
) (database.ObjectDetail, error) {
	if err := s.ensureConnected(); err != nil {
		return database.ObjectDetail{}, err
	}
	if err := reference.Validate(); err != nil {
		return database.ObjectDetail{}, err
	}
	reference.Schema = s.defaultSchema(reference.Schema)
	objects, err := s.ListObjects(ctx, database.ObjectFilter{
		Schema: reference.Schema,
		Kinds:  []database.ObjectKind{reference.Kind},
	})
	if err != nil {
		return database.ObjectDetail{}, err
	}
	var selected *database.DatabaseObject
	for index := range objects {
		candidate := &objects[index]
		if strings.EqualFold(candidate.Reference.Name, reference.Name) &&
			strings.EqualFold(
				candidate.Reference.ParentName,
				reference.ParentName,
			) {
			selected = candidate
			break
		}
	}
	if selected == nil {
		return database.ObjectDetail{}, fmt.Errorf(
			"SQL Server %s %q was not found",
			reference.Kind,
			reference.QualifiedName(),
		)
	}
	definition, err := s.objectDefinition(selected.Reference)
	if err != nil {
		return database.ObjectDetail{}, err
	}
	detail := database.ObjectDetail{
		Object:     *selected,
		Definition: definition,
		Properties: selected.Properties,
	}
	if reference.Kind == database.ObjectKindTable ||
		reference.Kind == database.ObjectKindView {
		detail.Columns, err = s.GetCollectionStructures(database.Table{
			Schema: reference.Schema,
			Name:   reference.Name,
		})
		if err != nil {
			return database.ObjectDetail{}, err
		}
	}
	return detail, nil
}
