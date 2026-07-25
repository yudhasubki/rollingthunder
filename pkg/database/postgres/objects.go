package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"rollingthunder/pkg/database"
)

type postgresObjectRow struct {
	ID           string  `db:"id"`
	Kind         string  `db:"kind"`
	Schema       string  `db:"schema_name"`
	Name         string  `db:"object_name"`
	Signature    string  `db:"signature"`
	ParentSchema string  `db:"parent_schema"`
	ParentName   string  `db:"parent_name"`
	Description  *string `db:"description"`
}

const postgresObjectsQuery = `
WITH database_objects AS (
	SELECT
		'pg_class:' || relation.oid::text AS id,
		CASE relation.relkind
			WHEN 'r' THEN 'table'
			WHEN 'p' THEN 'table'
			WHEN 'v' THEN 'view'
			WHEN 'm' THEN 'materialized_view'
			WHEN 'S' THEN 'sequence'
		END AS kind,
		namespace.nspname AS schema_name,
		relation.relname AS object_name,
		''::text AS signature,
		''::text AS parent_schema,
		''::text AS parent_name,
		obj_description(relation.oid, 'pg_class') AS description
	FROM pg_class relation
	JOIN pg_namespace namespace ON namespace.oid = relation.relnamespace
	WHERE relation.relkind IN ('r', 'p', 'v', 'm', 'S')
		AND namespace.nspname NOT LIKE 'pg\_%' ESCAPE '\'
		AND namespace.nspname <> 'information_schema'

	UNION ALL

	SELECT
		'pg_proc:' || routine.oid::text,
		CASE routine.prokind WHEN 'p' THEN 'procedure' ELSE 'function' END,
		namespace.nspname,
		routine.proname,
		pg_get_function_identity_arguments(routine.oid),
		'',
		'',
		obj_description(routine.oid, 'pg_proc')
	FROM pg_proc routine
	JOIN pg_namespace namespace ON namespace.oid = routine.pronamespace
	WHERE routine.prokind IN ('f', 'p', 'w')
		AND namespace.nspname NOT LIKE 'pg\_%' ESCAPE '\'
		AND namespace.nspname <> 'information_schema'

	UNION ALL

	SELECT
		'pg_trigger:' || trigger.oid::text,
		'trigger',
		namespace.nspname,
		trigger.tgname,
		'',
		namespace.nspname,
		parent.relname,
		obj_description(trigger.oid, 'pg_trigger')
	FROM pg_trigger trigger
	JOIN pg_class parent ON parent.oid = trigger.tgrelid
	JOIN pg_namespace namespace ON namespace.oid = parent.relnamespace
	WHERE NOT trigger.tgisinternal
		AND namespace.nspname NOT LIKE 'pg\_%' ESCAPE '\'
		AND namespace.nspname <> 'information_schema'

	UNION ALL

	SELECT
		'pg_type:' || object_type.oid::text,
		CASE object_type.typtype
			WHEN 'e' THEN 'enum'
			WHEN 'd' THEN 'domain'
			ELSE 'type'
		END,
		namespace.nspname,
		object_type.typname,
		'',
		'',
		'',
		obj_description(object_type.oid, 'pg_type')
	FROM pg_type object_type
	JOIN pg_namespace namespace ON namespace.oid = object_type.typnamespace
	LEFT JOIN pg_class type_relation ON type_relation.oid = object_type.typrelid
	WHERE object_type.typtype IN ('e', 'd', 'c')
		AND (object_type.typtype <> 'c' OR type_relation.relkind = 'c')
		AND object_type.typname NOT LIKE '\_%' ESCAPE '\'
		AND namespace.nspname NOT LIKE 'pg\_%' ESCAPE '\'
		AND namespace.nspname <> 'information_schema'

	UNION ALL

	SELECT
		'pg_constraint:' || constraint_object.oid::text,
		'constraint',
		namespace.nspname,
		constraint_object.conname,
		'',
		namespace.nspname,
		COALESCE(parent.relname, ''),
		obj_description(constraint_object.oid, 'pg_constraint')
	FROM pg_constraint constraint_object
	JOIN pg_namespace namespace ON namespace.oid = constraint_object.connamespace
	LEFT JOIN pg_class parent ON parent.oid = constraint_object.conrelid
	WHERE namespace.nspname NOT LIKE 'pg\_%' ESCAPE '\'
		AND namespace.nspname <> 'information_schema'

	UNION ALL

	SELECT
		'pg_extension:' || extension_object.oid::text,
		'extension',
		COALESCE(namespace.nspname, ''),
		extension_object.extname,
		extension_object.extversion,
		'',
		'',
		obj_description(extension_object.oid, 'pg_extension')
	FROM pg_extension extension_object
	LEFT JOIN pg_namespace namespace ON namespace.oid = extension_object.extnamespace

	UNION ALL

	SELECT
		'pg_class:' || index_object.oid::text,
		'index',
		namespace.nspname,
		index_object.relname,
		'',
		namespace.nspname,
		parent.relname,
		obj_description(index_object.oid, 'pg_class')
	FROM pg_class index_object
	JOIN pg_index index_metadata ON index_metadata.indexrelid = index_object.oid
	JOIN pg_class parent ON parent.oid = index_metadata.indrelid
	JOIN pg_namespace namespace ON namespace.oid = index_object.relnamespace
	WHERE index_object.relkind = 'i'
		AND namespace.nspname NOT LIKE 'pg\_%' ESCAPE '\'
		AND namespace.nspname <> 'information_schema'
)
SELECT
	id,
	kind,
	schema_name,
	object_name,
	signature,
	parent_schema,
	parent_name,
	description
FROM database_objects
WHERE ($1 = '' OR schema_name = $1)
	AND (
		$2 = ''
		OR object_name ILIKE '%' || $2 || '%'
		OR parent_name ILIKE '%' || $2 || '%'
		OR signature ILIKE '%' || $2 || '%'
	)
ORDER BY kind, object_name, signature`

func objectKindSet(kinds []database.ObjectKind) map[database.ObjectKind]struct{} {
	if len(kinds) == 0 {
		return nil
	}
	result := make(map[database.ObjectKind]struct{}, len(kinds))
	for _, kind := range kinds {
		result[kind] = struct{}{}
	}
	return result
}

func postgresObjectCanManage(
	kind database.ObjectKind,
	capabilities database.Capabilities,
) bool {
	switch kind {
	case database.ObjectKindView, database.ObjectKindMaterializedView:
		return capabilities.ManageViews
	case database.ObjectKindFunction, database.ObjectKindProcedure:
		return capabilities.ManageRoutines
	case database.ObjectKindTrigger:
		return capabilities.ManageTriggers
	case database.ObjectKindIndex:
		return capabilities.ManageIndexes
	case database.ObjectKindConstraint, database.ObjectKindTable:
		return capabilities.AlterTableStructure
	default:
		return false
	}
}

func postgresObjectAllowedActions(
	kind database.ObjectKind,
	capabilities database.Capabilities,
) []database.ObjectChangeAction {
	if !postgresObjectCanManage(kind, capabilities) {
		return nil
	}
	switch kind {
	case database.ObjectKindTable:
		return []database.ObjectChangeAction{
			database.ObjectChangeRename,
			database.ObjectChangeDrop,
		}
	case database.ObjectKindView,
		database.ObjectKindFunction, database.ObjectKindProcedure:
		return []database.ObjectChangeAction{
			database.ObjectChangeReplace,
			database.ObjectChangeRename,
			database.ObjectChangeDrop,
		}
	case database.ObjectKindMaterializedView:
		return []database.ObjectChangeAction{
			database.ObjectChangeRename,
			database.ObjectChangeDrop,
		}
	case database.ObjectKindTrigger:
		return []database.ObjectChangeAction{
			database.ObjectChangeReplace,
			database.ObjectChangeRename,
			database.ObjectChangeEnable,
			database.ObjectChangeDisable,
			database.ObjectChangeDrop,
		}
	case database.ObjectKindIndex, database.ObjectKindConstraint:
		return []database.ObjectChangeAction{
			database.ObjectChangeRename,
			database.ObjectChangeDrop,
		}
	default:
		return nil
	}
}

func postgresObjectFromRow(
	row postgresObjectRow,
	capabilities database.Capabilities,
) (database.DatabaseObject, error) {
	kind := database.ObjectKind(row.Kind)
	if !kind.Valid() || kind == database.ObjectKindUnknown {
		return database.DatabaseObject{}, fmt.Errorf(
			"unsupported PostgreSQL object kind %q",
			row.Kind,
		)
	}

	displayName := row.Name
	if row.Signature != "" &&
		(kind == database.ObjectKindFunction ||
			kind == database.ObjectKindProcedure) {
		displayName += "(" + row.Signature + ")"
	}

	properties := make([]database.ObjectProperty, 0, 2)
	if row.Signature != "" {
		properties = append(properties, database.ObjectProperty{
			Name:     "Signature",
			Value:    row.Signature,
			Category: "identity",
		})
	}
	if row.ParentName != "" {
		parent := row.ParentName
		if row.ParentSchema != "" {
			parent = row.ParentSchema + "." + row.ParentName
		}
		properties = append(properties, database.ObjectProperty{
			Name:     "Parent",
			Value:    parent,
			Category: "identity",
		})
	}

	description := ""
	if row.Description != nil {
		description = *row.Description
	}
	return database.DatabaseObject{
		Reference: database.ObjectReference{
			ID:           row.ID,
			Kind:         kind,
			Schema:       row.Schema,
			Name:         row.Name,
			Signature:    row.Signature,
			ParentSchema: row.ParentSchema,
			ParentName:   row.ParentName,
		},
		DisplayName: displayName,
		Description: description,
		CanOpenData: kind == database.ObjectKindTable ||
			kind == database.ObjectKindView ||
			kind == database.ObjectKindMaterializedView,
		CanManage:      postgresObjectCanManage(kind, capabilities),
		AllowedActions: postgresObjectAllowedActions(kind, capabilities),
		Properties:     properties,
	}, nil
}

func (p *Postgres) ListObjects(
	ctx context.Context,
	filter database.ObjectFilter,
) ([]database.DatabaseObject, error) {
	rows := make([]postgresObjectRow, 0)
	if err := p.conn.SelectContext(
		ctx,
		&rows,
		postgresObjectsQuery,
		strings.TrimSpace(filter.Schema),
		strings.TrimSpace(filter.Search),
	); err != nil {
		return nil, err
	}

	allowedKinds := objectKindSet(filter.Kinds)
	capabilities := p.Capabilities()
	objects := make([]database.DatabaseObject, 0, len(rows))
	for _, row := range rows {
		object, err := postgresObjectFromRow(row, capabilities)
		if err != nil {
			return nil, err
		}
		if len(allowedKinds) > 0 {
			if _, allowed := allowedKinds[object.Reference.Kind]; !allowed {
				continue
			}
		}
		objects = append(objects, object)
	}
	return objects, nil
}

var postgresObjectCatalogs = map[string]struct{}{
	"pg_class":      {},
	"pg_proc":       {},
	"pg_trigger":    {},
	"pg_type":       {},
	"pg_constraint": {},
	"pg_extension":  {},
}

func parsePostgresObjectID(value string) (string, uint64, error) {
	catalog, oidText, found := strings.Cut(strings.TrimSpace(value), ":")
	if !found {
		return "", 0, fmt.Errorf("invalid PostgreSQL object ID")
	}
	if _, allowed := postgresObjectCatalogs[catalog]; !allowed {
		return "", 0, fmt.Errorf("unsupported PostgreSQL object catalog %q", catalog)
	}
	oid, err := strconv.ParseUint(oidText, 10, 32)
	if err != nil || oid == 0 {
		return "", 0, fmt.Errorf("invalid PostgreSQL object OID %q", oidText)
	}
	return catalog, oid, nil
}

func (p *Postgres) resolveObject(
	ctx context.Context,
	reference database.ObjectReference,
) (database.DatabaseObject, string, uint64, error) {
	if reference.ID != "" {
		catalog, oid, err := parsePostgresObjectID(reference.ID)
		if err != nil {
			return database.DatabaseObject{}, "", 0, err
		}
		objects, err := p.ListObjects(ctx, database.ObjectFilter{
			Schema: reference.Schema,
		})
		if err != nil {
			return database.DatabaseObject{}, "", 0, err
		}
		for _, object := range objects {
			if object.Reference.ID == reference.ID {
				return object, catalog, oid, nil
			}
		}
		return database.DatabaseObject{}, "", 0, fmt.Errorf(
			"database object %q was not found",
			reference.ID,
		)
	}

	objects, err := p.ListObjects(ctx, database.ObjectFilter{
		Schema: reference.Schema,
		Kinds:  []database.ObjectKind{reference.Kind},
		Search: reference.Name,
	})
	if err != nil {
		return database.DatabaseObject{}, "", 0, err
	}
	for _, object := range objects {
		candidate := object.Reference
		if candidate.Name != reference.Name {
			continue
		}
		if reference.Signature != "" &&
			candidate.Signature != reference.Signature {
			continue
		}
		catalog, oid, err := parsePostgresObjectID(candidate.ID)
		return object, catalog, oid, err
	}
	return database.DatabaseObject{}, "", 0, fmt.Errorf(
		"database object %s was not found",
		reference.QualifiedName(),
	)
}

func quotePostgresLiteral(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return "E'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func postgresViewDefinition(
	object database.DatabaseObject,
	body string,
) string {
	qualifiedName := quotePostgresQualifiedIdentifier(
		object.Reference.Schema,
		object.Reference.Name,
	)
	body = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(body), ";"))
	if object.Reference.Kind == database.ObjectKindMaterializedView {
		return fmt.Sprintf(
			"CREATE MATERIALIZED VIEW %s AS\n%s;\n",
			qualifiedName,
			body,
		)
	}
	return fmt.Sprintf(
		"CREATE OR REPLACE VIEW %s AS\n%s;\n",
		qualifiedName,
		body,
	)
}

func (p *Postgres) objectDefinition(
	ctx context.Context,
	object database.DatabaseObject,
	oid uint64,
) (string, []database.ObjectProperty, database.Structures, error) {
	reference := object.Reference
	properties := append([]database.ObjectProperty(nil), object.Properties...)

	switch reference.Kind {
	case database.ObjectKindTable:
		ddl, err := p.GetTableDDL(database.Table{
			Schema: reference.Schema,
			Name:   reference.Name,
		})
		if err != nil {
			return "", nil, nil, err
		}
		columns, err := p.GetCollectionStructures(database.Table{
			Schema: reference.Schema,
			Name:   reference.Name,
		})
		return ddl, properties, columns, err

	case database.ObjectKindView, database.ObjectKindMaterializedView:
		var body string
		if err := p.conn.GetContext(
			ctx,
			&body,
			"SELECT pg_get_viewdef($1::oid, true)",
			oid,
		); err != nil {
			return "", nil, nil, err
		}
		columns, err := p.GetCollectionStructures(database.Table{
			Schema: reference.Schema,
			Name:   reference.Name,
		})
		return postgresViewDefinition(object, body), properties, columns, err

	case database.ObjectKindFunction, database.ObjectKindProcedure:
		var definition string
		err := p.conn.GetContext(
			ctx,
			&definition,
			"SELECT pg_get_functiondef($1::oid)",
			oid,
		)
		return definition, properties, nil, err

	case database.ObjectKindTrigger:
		type triggerMetadata struct {
			Definition string `db:"definition"`
			Enabled    string `db:"enabled"`
		}
		var metadata triggerMetadata
		err := p.conn.GetContext(
			ctx,
			&metadata,
			`SELECT
				pg_get_triggerdef($1::oid, true) AS definition,
				tgenabled::text AS enabled
			FROM pg_trigger
			WHERE oid = $1::oid`,
			oid,
		)
		if err == nil && metadata.Definition != "" {
			metadata.Definition += ";\n"
		}
		properties = append(properties, database.ObjectProperty{
			Name:     "Enabled",
			Value:    strconv.FormatBool(metadata.Enabled != "D"),
			Category: "trigger",
		})
		return metadata.Definition, properties, nil, err

	case database.ObjectKindIndex:
		var definition string
		err := p.conn.GetContext(
			ctx,
			&definition,
			"SELECT pg_get_indexdef($1::oid, 0, true)",
			oid,
		)
		if err == nil && definition != "" {
			definition += ";\n"
		}
		return definition, properties, nil, err

	case database.ObjectKindConstraint:
		var definition string
		err := p.conn.GetContext(
			ctx,
			&definition,
			"SELECT pg_get_constraintdef($1::oid, true)",
			oid,
		)
		if err != nil {
			return "", nil, nil, err
		}
		parentSchema := reference.ParentSchema
		if parentSchema == "" {
			parentSchema = reference.Schema
		}
		if reference.ParentName != "" {
			definition = fmt.Sprintf(
				"ALTER TABLE %s ADD CONSTRAINT %s %s;\n",
				quotePostgresQualifiedIdentifier(
					parentSchema,
					reference.ParentName,
				),
				quotePostgresIdentifier(reference.Name),
				definition,
			)
		}
		return definition, properties, nil, nil

	case database.ObjectKindSequence:
		type sequenceMetadata struct {
			DataType  string `db:"data_type"`
			Start     int64  `db:"start_value"`
			Minimum   int64  `db:"min_value"`
			Maximum   int64  `db:"max_value"`
			Increment int64  `db:"increment_by"`
			Cache     int64  `db:"cache_size"`
			Cycle     bool   `db:"cycle"`
		}
		var metadata sequenceMetadata
		err := p.conn.GetContext(ctx, &metadata, `
			SELECT
				format_type(sequence.seqtypid, NULL) AS data_type,
				sequence.seqstart AS start_value,
				sequence.seqmin AS min_value,
				sequence.seqmax AS max_value,
				sequence.seqincrement AS increment_by,
				sequence.seqcache AS cache_size,
				sequence.seqcycle AS cycle
			FROM pg_sequence sequence
			WHERE sequence.seqrelid = $1::oid`, oid)
		if err != nil {
			return "", nil, nil, err
		}
		properties = append(properties,
			database.ObjectProperty{Name: "Data type", Value: metadata.DataType, Category: "sequence"},
			database.ObjectProperty{Name: "Start", Value: strconv.FormatInt(metadata.Start, 10), Category: "sequence"},
			database.ObjectProperty{Name: "Increment", Value: strconv.FormatInt(metadata.Increment, 10), Category: "sequence"},
			database.ObjectProperty{Name: "Minimum", Value: strconv.FormatInt(metadata.Minimum, 10), Category: "sequence"},
			database.ObjectProperty{Name: "Maximum", Value: strconv.FormatInt(metadata.Maximum, 10), Category: "sequence"},
			database.ObjectProperty{Name: "Cache", Value: strconv.FormatInt(metadata.Cache, 10), Category: "sequence"},
			database.ObjectProperty{Name: "Cycle", Value: strconv.FormatBool(metadata.Cycle), Category: "sequence"},
		)
		cycle := "NO CYCLE"
		if metadata.Cycle {
			cycle = "CYCLE"
		}
		definition := fmt.Sprintf(
			"CREATE SEQUENCE %s\n  AS %s\n  INCREMENT BY %d\n  MINVALUE %d\n  MAXVALUE %d\n  START WITH %d\n  CACHE %d\n  %s;\n",
			quotePostgresQualifiedIdentifier(reference.Schema, reference.Name),
			metadata.DataType,
			metadata.Increment,
			metadata.Minimum,
			metadata.Maximum,
			metadata.Start,
			metadata.Cache,
			cycle,
		)
		return definition, properties, nil, nil

	case database.ObjectKindEnum:
		var values []string
		if err := p.conn.SelectContext(ctx, &values, `
			SELECT enumlabel
			FROM pg_enum
			WHERE enumtypid = $1::oid
			ORDER BY enumsortorder`, oid); err != nil {
			return "", nil, nil, err
		}
		quotedValues := make([]string, len(values))
		for index, value := range values {
			quotedValues[index] = quotePostgresLiteral(value)
		}
		properties = append(properties, database.ObjectProperty{
			Name:     "Values",
			Value:    strings.Join(values, ", "),
			Category: "type",
		})
		definition := fmt.Sprintf(
			"CREATE TYPE %s AS ENUM (\n  %s\n);\n",
			quotePostgresQualifiedIdentifier(reference.Schema, reference.Name),
			strings.Join(quotedValues, ",\n  "),
		)
		return definition, properties, nil, nil

	case database.ObjectKindDomain:
		type domainMetadata struct {
			BaseType string         `db:"base_type"`
			NotNull  bool           `db:"not_null"`
			Default  sql.NullString `db:"default_value"`
		}
		var metadata domainMetadata
		if err := p.conn.GetContext(ctx, &metadata, `
			SELECT
				format_type(type_object.typbasetype, type_object.typtypmod) AS base_type,
				type_object.typnotnull AS not_null,
				pg_get_expr(type_object.typdefaultbin, 0) AS default_value
			FROM pg_type type_object
			WHERE type_object.oid = $1::oid`, oid); err != nil {
			return "", nil, nil, err
		}
		var constraints []string
		if err := p.conn.SelectContext(ctx, &constraints, `
			SELECT pg_get_constraintdef(constraint_object.oid, true)
			FROM pg_constraint constraint_object
			WHERE constraint_object.contypid = $1::oid
			ORDER BY constraint_object.conname`, oid); err != nil {
			return "", nil, nil, err
		}
		parts := []string{
			fmt.Sprintf(
				"CREATE DOMAIN %s AS %s",
				quotePostgresQualifiedIdentifier(reference.Schema, reference.Name),
				metadata.BaseType,
			),
		}
		if metadata.Default.Valid {
			parts = append(parts, "  DEFAULT "+metadata.Default.String)
		}
		if metadata.NotNull {
			parts = append(parts, "  NOT NULL")
		}
		for _, constraint := range constraints {
			parts = append(parts, "  "+constraint)
		}
		properties = append(properties, database.ObjectProperty{
			Name: "Base type", Value: metadata.BaseType, Category: "type",
		})
		return strings.Join(parts, "\n") + ";\n", properties, nil, nil

	case database.ObjectKindType:
		type compositeAttribute struct {
			Name string `db:"attribute_name"`
			Type string `db:"attribute_type"`
		}
		var attributes []compositeAttribute
		if err := p.conn.SelectContext(ctx, &attributes, `
			SELECT
				attribute.attname AS attribute_name,
				format_type(attribute.atttypid, attribute.atttypmod) AS attribute_type
			FROM pg_type type_object
			JOIN pg_attribute attribute
				ON attribute.attrelid = type_object.typrelid
			WHERE type_object.oid = $1::oid
				AND attribute.attnum > 0
				AND NOT attribute.attisdropped
			ORDER BY attribute.attnum`, oid); err != nil {
			return "", nil, nil, err
		}
		definitions := make([]string, len(attributes))
		for index, attribute := range attributes {
			definitions[index] = fmt.Sprintf(
				"  %s %s",
				quotePostgresIdentifier(attribute.Name),
				attribute.Type,
			)
		}
		definition := fmt.Sprintf(
			"CREATE TYPE %s AS (\n%s\n);\n",
			quotePostgresQualifiedIdentifier(reference.Schema, reference.Name),
			strings.Join(definitions, ",\n"),
		)
		return definition, properties, nil, nil

	case database.ObjectKindExtension:
		type extensionMetadata struct {
			Version     string `db:"version"`
			Schema      string `db:"schema_name"`
			Relocatable bool   `db:"relocatable"`
		}
		var metadata extensionMetadata
		if err := p.conn.GetContext(ctx, &metadata, `
			SELECT
				extension_object.extversion AS version,
				namespace.nspname AS schema_name,
				extension_object.extrelocatable AS relocatable
			FROM pg_extension extension_object
			JOIN pg_namespace namespace
				ON namespace.oid = extension_object.extnamespace
			WHERE extension_object.oid = $1::oid`, oid); err != nil {
			return "", nil, nil, err
		}
		properties = append(properties,
			database.ObjectProperty{Name: "Version", Value: metadata.Version, Category: "extension"},
			database.ObjectProperty{Name: "Schema", Value: metadata.Schema, Category: "extension"},
			database.ObjectProperty{Name: "Relocatable", Value: strconv.FormatBool(metadata.Relocatable), Category: "extension"},
		)
		definition := fmt.Sprintf(
			"CREATE EXTENSION IF NOT EXISTS %s WITH SCHEMA %s VERSION %s;\n",
			quotePostgresIdentifier(reference.Name),
			quotePostgresIdentifier(metadata.Schema),
			quotePostgresLiteral(metadata.Version),
		)
		return definition, properties, nil, nil
	default:
		return "", nil, nil, fmt.Errorf(
			"definition is not available for %s objects",
			reference.Kind,
		)
	}
}

type postgresDependencyRow struct {
	Catalog     string `db:"catalog"`
	OID         uint64 `db:"object_oid"`
	Type        string `db:"object_type"`
	Schema      string `db:"schema_name"`
	Name        string `db:"object_name"`
	Identity    string `db:"identity"`
	Description string `db:"description"`
}

func postgresKindFromIdentity(value string) database.ObjectKind {
	lower := strings.ToLower(value)
	switch {
	case strings.Contains(lower, "materialized view"):
		return database.ObjectKindMaterializedView
	case strings.Contains(lower, "view"):
		return database.ObjectKindView
	case strings.Contains(lower, "table"):
		return database.ObjectKindTable
	case strings.Contains(lower, "procedure"):
		return database.ObjectKindProcedure
	case strings.Contains(lower, "function"), strings.Contains(lower, "aggregate"):
		return database.ObjectKindFunction
	case strings.Contains(lower, "trigger"):
		return database.ObjectKindTrigger
	case strings.Contains(lower, "sequence"):
		return database.ObjectKindSequence
	case strings.Contains(lower, "domain"):
		return database.ObjectKindDomain
	case strings.Contains(lower, "type"):
		return database.ObjectKindType
	case strings.Contains(lower, "constraint"):
		return database.ObjectKindConstraint
	case strings.Contains(lower, "extension"):
		return database.ObjectKindExtension
	case strings.Contains(lower, "index"):
		return database.ObjectKindIndex
	default:
		return database.ObjectKindUnknown
	}
}

func dependencyFromPostgresRow(row postgresDependencyRow) database.ObjectDependency {
	kind := postgresKindFromIdentity(row.Type)
	id := ""
	if _, allowed := postgresObjectCatalogs[row.Catalog]; allowed && row.OID > 0 {
		id = fmt.Sprintf("%s:%d", row.Catalog, row.OID)
	}
	name := row.Name
	if name == "" {
		name = row.Identity
	}
	return database.ObjectDependency{
		Reference: database.ObjectReference{
			ID:        id,
			Kind:      kind,
			Schema:    row.Schema,
			Name:      name,
			Signature: row.Identity,
		},
		Description: row.Description,
	}
}

func (p *Postgres) objectDependencies(
	ctx context.Context,
	catalog string,
	oid uint64,
	dependents bool,
) ([]database.ObjectDependency, error) {
	classColumn := "dependency.refclassid"
	objectColumn := "dependency.refobjid"
	whereClass := "dependency.classid"
	whereObject := "dependency.objid"
	if dependents {
		classColumn = "dependency.classid"
		objectColumn = "dependency.objid"
		whereClass = "dependency.refclassid"
		whereObject = "dependency.refobjid"
	}
	query := fmt.Sprintf(`
		SELECT
			%s::regclass::text AS catalog,
			%s::oid::bigint AS object_oid,
			identified.type AS object_type,
			COALESCE(identified.schema, '') AS schema_name,
			COALESCE(identified.name, '') AS object_name,
			COALESCE(identified.identity, '') AS identity,
			pg_describe_object(%s, %s, 0) AS description
		FROM pg_depend dependency
		CROSS JOIN LATERAL pg_identify_object(%s, %s, 0) identified
		WHERE %s = $1::regclass
			AND %s = $2::oid
			AND dependency.deptype IN ('n', 'a')
			AND %s <> $1::regclass
		ORDER BY identified.type, identified.schema, identified.name
		LIMIT 200`,
		classColumn,
		objectColumn,
		classColumn,
		objectColumn,
		classColumn,
		objectColumn,
		whereClass,
		whereObject,
		classColumn,
	)

	var rows []postgresDependencyRow
	if err := p.conn.SelectContext(ctx, &rows, query, catalog, oid); err != nil {
		return nil, err
	}
	result := make([]database.ObjectDependency, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		dependency := dependencyFromPostgresRow(row)
		key := dependency.Reference.ID + ":" + dependency.Description
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, dependency)
	}
	return result, nil
}

func sortObjectProperties(properties []database.ObjectProperty) {
	sort.SliceStable(properties, func(left, right int) bool {
		if properties[left].Category != properties[right].Category {
			return properties[left].Category < properties[right].Category
		}
		return properties[left].Name < properties[right].Name
	})
}

func (p *Postgres) GetObjectDetail(
	ctx context.Context,
	reference database.ObjectReference,
) (database.ObjectDetail, error) {
	object, catalog, oid, err := p.resolveObject(ctx, reference)
	if err != nil {
		return database.ObjectDetail{}, err
	}
	definition, properties, columns, err := p.objectDefinition(
		ctx,
		object,
		oid,
	)
	if err != nil {
		return database.ObjectDetail{}, err
	}
	dependencies, err := p.objectDependencies(ctx, catalog, oid, false)
	if err != nil {
		return database.ObjectDetail{}, err
	}
	dependents, err := p.objectDependencies(ctx, catalog, oid, true)
	if err != nil {
		return database.ObjectDetail{}, err
	}
	sortObjectProperties(properties)

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
