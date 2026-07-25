package oracle

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"rollingthunder/pkg/database"
)

func objectID(reference database.ObjectReference) string {
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
	return "oracle:" + strings.Join(parts, ".")
}

func oracleObjectKind(value string) database.ObjectKind {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "TABLE":
		return database.ObjectKindTable
	case "VIEW":
		return database.ObjectKindView
	case "MATERIALIZED VIEW":
		return database.ObjectKindMaterializedView
	case "FUNCTION":
		return database.ObjectKindFunction
	case "PROCEDURE":
		return database.ObjectKindProcedure
	case "TRIGGER":
		return database.ObjectKindTrigger
	case "SEQUENCE":
		return database.ObjectKindSequence
	case "TYPE":
		return database.ObjectKindType
	case "INDEX":
		return database.ObjectKindIndex
	default:
		return database.ObjectKindUnknown
	}
}

func oracleObjectMatches(
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

type objectParent struct {
	schema     string
	name       string
	hasEnabled bool
	enabled    bool
}

func (o *Oracle) objectParents(
	ctx context.Context,
	schema string,
) (map[string]objectParent, error) {
	parents := make(map[string]objectParent)
	triggerRows, err := o.conn.QueryContext(ctx, `
		SELECT trigger_name, table_owner, table_name, status
		FROM all_triggers
		WHERE owner = :1`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	for triggerRows.Next() {
		var (
			name   string
			owner  sql.NullString
			table  sql.NullString
			status string
		)
		if err := triggerRows.Scan(
			&name,
			&owner,
			&table,
			&status,
		); err != nil {
			_ = triggerRows.Close()
			return nil, err
		}
		parents["TRIGGER:"+name] = objectParent{
			schema:     owner.String,
			name:       table.String,
			hasEnabled: true,
			enabled:    strings.EqualFold(status, "ENABLED"),
		}
	}
	if err := triggerRows.Close(); err != nil {
		return nil, err
	}
	indexRows, err := o.conn.QueryContext(ctx, `
		SELECT index_name, table_owner, table_name
		FROM all_indexes
		WHERE owner = :1`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer indexRows.Close()
	for indexRows.Next() {
		var name, owner, table string
		if err := indexRows.Scan(&name, &owner, &table); err != nil {
			return nil, err
		}
		parents["INDEX:"+name] = objectParent{schema: owner, name: table}
	}
	return parents, indexRows.Err()
}

func (o *Oracle) relationComments(
	ctx context.Context,
	schema string,
) (map[string]string, error) {
	rows, err := o.conn.QueryContext(ctx, `
		SELECT table_name, comments
		FROM all_tab_comments
		WHERE owner = :1`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	comments := make(map[string]string)
	for rows.Next() {
		var (
			name    string
			comment sql.NullString
		)
		if err := rows.Scan(&name, &comment); err != nil {
			return nil, err
		}
		if comment.Valid {
			comments[name] = comment.String
		}
	}
	return comments, rows.Err()
}

func oracleObjectCanManage(
	kind database.ObjectKind,
	capabilities database.Capabilities,
) bool {
	switch kind {
	case database.ObjectKindTable, database.ObjectKindConstraint:
		return capabilities.AlterTableStructure
	case database.ObjectKindView, database.ObjectKindMaterializedView:
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

func oracleObjectAllowedActions(
	reference database.ObjectReference,
	capabilities database.Capabilities,
) []database.ObjectChangeAction {
	if !oracleObjectCanManage(reference.Kind, capabilities) {
		return nil
	}
	switch reference.Kind {
	case database.ObjectKindTable:
		return []database.ObjectChangeAction{
			database.ObjectChangeRename,
			database.ObjectChangeDrop,
		}
	case database.ObjectKindView:
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
	case database.ObjectKindFunction, database.ObjectKindProcedure:
		return []database.ObjectChangeAction{
			database.ObjectChangeReplace,
			database.ObjectChangeDrop,
		}
	case database.ObjectKindTrigger:
		actions := []database.ObjectChangeAction{
			database.ObjectChangeReplace,
		}
		if capabilities.TriggerToggle {
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
		return []database.ObjectChangeAction{
			database.ObjectChangeRename,
			database.ObjectChangeDrop,
		}
	default:
		return nil
	}
}

func (o *Oracle) ListObjects(
	ctx context.Context,
	filter database.ObjectFilter,
) ([]database.DatabaseObject, error) {
	if err := o.ensureConnected(); err != nil {
		return nil, err
	}
	schema := o.defaultSchema(filter.Schema)
	allowed := kindSet(filter.Kinds)
	capabilities := o.Capabilities()
	parents, err := o.objectParents(ctx, schema)
	if err != nil {
		return nil, err
	}
	comments, err := o.relationComments(ctx, schema)
	if err != nil {
		return nil, err
	}
	rows, err := o.conn.QueryContext(ctx, `
		SELECT object_type, object_name, status, created, last_ddl_time
		FROM all_objects
		WHERE owner = :1
			AND object_type IN (
				'TABLE', 'VIEW', 'MATERIALIZED VIEW', 'FUNCTION',
				'PROCEDURE', 'TRIGGER', 'SEQUENCE', 'TYPE', 'INDEX'
			)
			AND subobject_name IS NULL
		ORDER BY object_type, object_name`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	objects := make([]database.DatabaseObject, 0)
	for rows.Next() {
		var (
			objectType string
			name       string
			status     string
			created    time.Time
			lastDDL    time.Time
		)
		if err := rows.Scan(
			&objectType,
			&name,
			&status,
			&created,
			&lastDDL,
		); err != nil {
			return nil, err
		}
		kind := oracleObjectKind(objectType)
		if kind == database.ObjectKindUnknown {
			continue
		}
		reference := database.ObjectReference{
			Kind:   kind,
			Schema: schema,
			Name:   name,
		}
		parent, hasParent := parents[objectType+":"+name]
		if hasParent {
			reference.ParentSchema = parent.schema
			reference.ParentName = parent.name
		}
		reference.ID = objectID(reference)
		properties := []database.ObjectProperty{
			{Name: "Status", Value: status, Category: "state"},
			{
				Name:     "Created",
				Value:    created.Format(time.RFC3339),
				Category: "lifecycle",
			},
			{
				Name:     "Last DDL",
				Value:    lastDDL.Format(time.RFC3339),
				Category: "lifecycle",
			},
		}
		if reference.ParentName != "" {
			properties = append(properties, database.ObjectProperty{
				Name:     "Parent",
				Value:    reference.ParentSchema + "." + reference.ParentName,
				Category: "identity",
			})
		}
		if hasParent && parent.hasEnabled {
			properties = append(properties, database.ObjectProperty{
				Name:     "Enabled",
				Value:    fmt.Sprintf("%t", parent.enabled),
				Category: "state",
			})
		}
		allowedActions := oracleObjectAllowedActions(
			reference,
			capabilities,
		)
		object := database.DatabaseObject{
			Reference:   reference,
			DisplayName: name,
			Description: comments[name],
			CanOpenData: kind == database.ObjectKindTable ||
				kind == database.ObjectKindView ||
				kind == database.ObjectKindMaterializedView,
			CanManage:      len(allowedActions) > 0,
			AllowedActions: allowedActions,
			Properties:     properties,
		}
		if oracleObjectMatches(object, allowed, filter.Search) {
			objects = append(objects, object)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	constraintRows, err := o.conn.QueryContext(ctx, `
		SELECT constraint_name, table_name, constraint_type, status
		FROM all_constraints
		WHERE owner = :1
			AND constraint_type IN ('P', 'U', 'R', 'C')
			AND (constraint_type <> 'C' OR generated = 'USER NAME')
		ORDER BY constraint_name`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer constraintRows.Close()
	for constraintRows.Next() {
		var name, tableName, constraintType, status string
		if err := constraintRows.Scan(
			&name,
			&tableName,
			&constraintType,
			&status,
		); err != nil {
			return nil, err
		}
		reference := database.ObjectReference{
			Kind:         database.ObjectKindConstraint,
			Schema:       schema,
			Name:         name,
			ParentSchema: schema,
			ParentName:   tableName,
		}
		reference.ID = objectID(reference)
		object := database.DatabaseObject{
			Reference:   reference,
			DisplayName: name,
			Description: constraintType,
			Properties: []database.ObjectProperty{
				{Name: "Constraint type", Value: constraintType, Category: "constraint"},
				{Name: "Status", Value: status, Category: "state"},
				{Name: "Parent", Value: schema + "." + tableName, Category: "identity"},
			},
		}
		object.AllowedActions = oracleObjectAllowedActions(
			reference,
			capabilities,
		)
		object.CanManage = len(object.AllowedActions) > 0
		if oracleObjectMatches(object, allowed, filter.Search) {
			objects = append(objects, object)
		}
	}
	return objects, constraintRows.Err()
}

func metadataType(kind database.ObjectKind) (string, error) {
	switch kind {
	case database.ObjectKindTable:
		return "TABLE", nil
	case database.ObjectKindView:
		return "VIEW", nil
	case database.ObjectKindMaterializedView:
		return "MATERIALIZED_VIEW", nil
	case database.ObjectKindFunction:
		return "FUNCTION", nil
	case database.ObjectKindProcedure:
		return "PROCEDURE", nil
	case database.ObjectKindTrigger:
		return "TRIGGER", nil
	case database.ObjectKindSequence:
		return "SEQUENCE", nil
	case database.ObjectKindType:
		return "TYPE", nil
	case database.ObjectKindIndex:
		return "INDEX", nil
	case database.ObjectKindConstraint:
		return "CONSTRAINT", nil
	default:
		return "", fmt.Errorf("Oracle definition is unavailable for %q", kind)
	}
}

const (
	oracleViewDefinitionQuery = `
		SELECT text
		FROM all_views
		WHERE owner = :1
			AND view_name = :2`
	oracleMaterializedViewDefinitionQuery = `
		SELECT query
		FROM all_mviews
		WHERE owner = :1
			AND mview_name = :2`
)

func oracleCatalogViewDDL(
	reference database.ObjectReference,
	query string,
) (string, error) {
	query = strings.TrimSpace(query)
	query = strings.TrimSuffix(query, ";")
	if query == "" {
		return "", fmt.Errorf(
			"Oracle %s %q has an empty catalog definition",
			reference.Kind,
			reference.QualifiedName(),
		)
	}
	prefix := "CREATE OR REPLACE VIEW "
	if reference.Kind == database.ObjectKindMaterializedView {
		prefix = "CREATE MATERIALIZED VIEW "
	}
	return prefix +
		quoteQualified(reference.Schema, reference.Name) +
		" AS\n" + query + ";", nil
}

func (o *Oracle) catalogViewDefinition(
	ctx context.Context,
	reference database.ObjectReference,
) (string, error) {
	query := oracleViewDefinitionQuery
	if reference.Kind == database.ObjectKindMaterializedView {
		query = oracleMaterializedViewDefinitionQuery
	}
	var definition sql.NullString
	if err := o.conn.QueryRowContext(
		ctx,
		query,
		o.defaultSchema(reference.Schema),
		reference.Name,
	).Scan(&definition); err != nil {
		return "", err
	}
	if !definition.Valid {
		return "", fmt.Errorf(
			"Oracle %s %q has no catalog definition",
			reference.Kind,
			reference.QualifiedName(),
		)
	}
	return oracleCatalogViewDDL(reference, definition.String)
}

func (o *Oracle) objectDefinition(
	ctx context.Context,
	reference database.ObjectReference,
) (string, error) {
	var catalogErr error
	if reference.Kind == database.ObjectKindView ||
		reference.Kind == database.ObjectKindMaterializedView {
		// Oracle Free images can raise ORA-00600 while loading the XDB
		// library behind DBMS_METADATA. The catalog contains the view query
		// directly, so prefer that safe path and retain DBMS_METADATA only as
		// a fallback for unusual catalog visibility failures.
		var definition string
		definition, catalogErr = o.catalogViewDefinition(ctx, reference)
		if catalogErr == nil {
			return definition, nil
		}
	}
	objectType, err := metadataType(reference.Kind)
	if err != nil {
		return "", err
	}
	var definition sql.NullString
	metadataErr := o.conn.QueryRowContext(
		ctx,
		"SELECT DBMS_METADATA.GET_DDL(:1, :2, :3) FROM dual",
		objectType,
		reference.Name,
		o.defaultSchema(reference.Schema),
	).Scan(&definition)
	if metadataErr == nil && definition.Valid &&
		strings.TrimSpace(definition.String) != "" {
		result := strings.TrimSpace(definition.String)
		if !strings.HasSuffix(result, ";") {
			result += ";"
		}
		return result, nil
	}
	if catalogErr != nil {
		if metadataErr != nil {
			return "", fmt.Errorf(
				"read Oracle %s definition from the catalog: %v; "+
					"DBMS_METADATA fallback also failed: %w",
				reference.Kind,
				catalogErr,
				metadataErr,
			)
		}
		return "", catalogErr
	}
	if metadataErr != nil {
		return "", metadataErr
	}
	return "", nil
}

func (o *Oracle) GetObjectDetail(
	ctx context.Context,
	reference database.ObjectReference,
) (database.ObjectDetail, error) {
	if err := o.ensureConnected(); err != nil {
		return database.ObjectDetail{}, err
	}
	if err := reference.Validate(); err != nil {
		return database.ObjectDetail{}, err
	}
	reference.Schema = o.defaultSchema(reference.Schema)
	objects, err := o.ListObjects(ctx, database.ObjectFilter{
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
			"Oracle %s %q was not found",
			reference.Kind,
			reference.QualifiedName(),
		)
	}
	definition, err := o.objectDefinition(ctx, selected.Reference)
	if err != nil {
		return database.ObjectDetail{}, err
	}
	detail := database.ObjectDetail{
		Object:     *selected,
		Definition: definition,
		Properties: selected.Properties,
	}
	if reference.Kind == database.ObjectKindTable ||
		reference.Kind == database.ObjectKindView ||
		reference.Kind == database.ObjectKindMaterializedView {
		detail.Columns, err = o.GetCollectionStructures(database.Table{
			Schema: reference.Schema,
			Name:   reference.Name,
		})
		if err != nil {
			return database.ObjectDetail{}, err
		}
	}
	return detail, nil
}
