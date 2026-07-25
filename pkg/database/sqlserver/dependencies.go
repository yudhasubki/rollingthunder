package sqlserver

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"rollingthunder/pkg/database"
)

type sqlServerDependencySet struct {
	items []database.ObjectDependency
	seen  map[string]struct{}
}

func newSQLServerDependencySet() *sqlServerDependencySet {
	return &sqlServerDependencySet{
		items: make([]database.ObjectDependency, 0),
		seen:  make(map[string]struct{}),
	}
}

func sqlServerDependencyKey(reference database.ObjectReference) string {
	return strings.ToLower(strings.Join([]string{
		string(reference.Kind),
		reference.Schema,
		reference.Name,
		reference.ParentSchema,
		reference.ParentName,
	}, "\x00"))
}

func (set *sqlServerDependencySet) add(
	dependency database.ObjectDependency,
	selected database.ObjectReference,
) {
	reference := dependency.Reference
	if strings.TrimSpace(reference.Name) == "" {
		return
	}
	if strings.EqualFold(reference.Schema, selected.Schema) &&
		strings.EqualFold(reference.Name, selected.Name) &&
		reference.Kind == selected.Kind &&
		strings.EqualFold(reference.ParentName, selected.ParentName) {
		return
	}
	key := sqlServerDependencyKey(reference)
	if _, exists := set.seen[key]; exists {
		return
	}
	set.seen[key] = struct{}{}
	set.items = append(set.items, dependency)
}

func (set *sqlServerDependencySet) sorted() []database.ObjectDependency {
	sort.SliceStable(set.items, func(left, right int) bool {
		leftReference := set.items[left].Reference
		rightReference := set.items[right].Reference
		leftName := strings.ToLower(
			leftReference.QualifiedName() + "\x00" +
				string(leftReference.Kind),
		)
		rightName := strings.ToLower(
			rightReference.QualifiedName() + "\x00" +
				string(rightReference.Kind),
		)
		return leftName < rightName
	})
	return set.items
}

func (s *SQLServer) sqlServerCatalogObjectID(
	ctx context.Context,
	reference database.ObjectReference,
) (int64, error) {
	schema := s.defaultSchema(reference.Schema)
	name := reference.Name
	if reference.Kind == database.ObjectKindIndex {
		name = reference.ParentName
	}
	var id int64
	if err := s.conn.QueryRowContext(
		ctx,
		`SELECT object_value.object_id
		 FROM sys.objects object_value
		 JOIN sys.schemas schema_value
			ON schema_value.schema_id = object_value.schema_id
		 WHERE schema_value.name = @p1
			AND object_value.name = @p2`,
		schema,
		name,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf(
			"resolve SQL Server dependency object %s.%s: %w",
			schema,
			name,
			err,
		)
	}
	return id, nil
}

type sqlServerExpressionDependencyRow struct {
	id           sql.NullInt64
	schema       string
	name         string
	objectType   string
	parentSchema string
	parentName   string
	class        string
}

const sqlServerDependenciesQuery = `
	SELECT DISTINCT
		dependency_value.referenced_id,
		COALESCE(
			referenced_schema.name,
			dependency_value.referenced_schema_name,
			N''
		),
		COALESCE(
			referenced_object.name,
			dependency_value.referenced_entity_name,
			N''
		),
		COALESCE(referenced_object.type, N''),
		COALESCE(parent_schema.name, N''),
		COALESCE(parent_object.name, N''),
		COALESCE(dependency_value.referenced_class_desc, N'OBJECT')
	FROM sys.sql_expression_dependencies dependency_value
	LEFT JOIN sys.objects referenced_object
		ON referenced_object.object_id = dependency_value.referenced_id
	LEFT JOIN sys.schemas referenced_schema
		ON referenced_schema.schema_id = referenced_object.schema_id
	LEFT JOIN sys.objects parent_object
		ON parent_object.object_id = referenced_object.parent_object_id
	LEFT JOIN sys.schemas parent_schema
		ON parent_schema.schema_id = parent_object.schema_id
	WHERE dependency_value.referencing_id = @p1`

const sqlServerDependentsQuery = `
	SELECT DISTINCT
		dependency_value.referencing_id,
		COALESCE(referencing_schema.name, N''),
		COALESCE(referencing_object.name, N''),
		COALESCE(referencing_object.type, N''),
		COALESCE(parent_schema.name, N''),
		COALESCE(parent_object.name, N''),
		COALESCE(dependency_value.referenced_class_desc, N'OBJECT')
	FROM sys.sql_expression_dependencies dependency_value
	JOIN sys.objects referencing_object
		ON referencing_object.object_id = dependency_value.referencing_id
	JOIN sys.schemas referencing_schema
		ON referencing_schema.schema_id = referencing_object.schema_id
	LEFT JOIN sys.objects parent_object
		ON parent_object.object_id = referencing_object.parent_object_id
	LEFT JOIN sys.schemas parent_schema
		ON parent_schema.schema_id = parent_object.schema_id
	WHERE dependency_value.referenced_id = @p1`

func sqlServerDependencyReference(
	row sqlServerExpressionDependencyRow,
) database.ObjectReference {
	kind := objectKind(row.objectType)
	reference := database.ObjectReference{
		Kind:         kind,
		Schema:       row.schema,
		Name:         row.name,
		ParentSchema: row.parentSchema,
		ParentName:   row.parentName,
	}
	if row.id.Valid && kind != database.ObjectKindUnknown {
		reference.ID = objectID(
			"object",
			strconv.FormatInt(row.id.Int64, 10),
		)
	}
	return reference
}

func (s *SQLServer) expressionDependencies(
	ctx context.Context,
	catalogID int64,
	selected database.ObjectReference,
	reverse bool,
	target *sqlServerDependencySet,
) error {
	query := sqlServerDependenciesQuery
	description := "Referenced by SQL expression"
	if reverse {
		query = sqlServerDependentsQuery
		description = "References this object in a SQL expression"
	}
	rows, err := s.conn.QueryContext(ctx, query, catalogID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row sqlServerExpressionDependencyRow
		if err := rows.Scan(
			&row.id,
			&row.schema,
			&row.name,
			&row.objectType,
			&row.parentSchema,
			&row.parentName,
			&row.class,
		); err != nil {
			return err
		}
		reference := sqlServerDependencyReference(row)
		target.add(database.ObjectDependency{
			Reference: reference,
			Description: description + " · " +
				strings.ToLower(strings.ReplaceAll(row.class, "_", " ")),
		}, selected)
	}
	return rows.Err()
}

func (s *SQLServer) foreignKeyDependencies(
	ctx context.Context,
	catalogID int64,
	selected database.ObjectReference,
	reverse bool,
	target *sqlServerDependencySet,
) error {
	column := "foreign_key.parent_object_id"
	targetColumn := "foreign_key.referenced_object_id"
	description := "Referenced by foreign key"
	if reverse {
		column = "foreign_key.referenced_object_id"
		targetColumn = "foreign_key.parent_object_id"
		description = "References this table through foreign key"
	}
	query := fmt.Sprintf(`
		SELECT
			target_schema.name,
			target_table.name,
			foreign_key.name,
			target_table.object_id
		FROM sys.foreign_keys foreign_key
		JOIN sys.tables target_table
			ON target_table.object_id = %s
		JOIN sys.schemas target_schema
			ON target_schema.schema_id = target_table.schema_id
		WHERE %s = @p1
		ORDER BY target_schema.name, target_table.name, foreign_key.name`,
		targetColumn,
		column,
	)
	rows, err := s.conn.QueryContext(ctx, query, catalogID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var schema, name, constraint string
		var id int64
		if err := rows.Scan(&schema, &name, &constraint, &id); err != nil {
			return err
		}
		target.add(database.ObjectDependency{
			Reference: database.ObjectReference{
				ID:     objectID("object", strconv.FormatInt(id, 10)),
				Kind:   database.ObjectKindTable,
				Schema: schema,
				Name:   name,
			},
			Description: description + " " + constraint,
		}, selected)
	}
	return rows.Err()
}

func (s *SQLServer) foreignKeyConstraintDependency(
	ctx context.Context,
	catalogID int64,
	selected database.ObjectReference,
	target *sqlServerDependencySet,
) error {
	var id int64
	var schema, name, constraint string
	err := s.conn.QueryRowContext(ctx, `
		SELECT
			referenced_table.object_id,
			referenced_schema.name,
			referenced_table.name,
			foreign_key.name
		FROM sys.foreign_keys foreign_key
		JOIN sys.tables referenced_table
			ON referenced_table.object_id = foreign_key.referenced_object_id
		JOIN sys.schemas referenced_schema
			ON referenced_schema.schema_id = referenced_table.schema_id
		WHERE foreign_key.object_id = @p1`,
		catalogID,
	).Scan(&id, &schema, &name, &constraint)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	target.add(database.ObjectDependency{
		Reference: database.ObjectReference{
			ID:     objectID("object", strconv.FormatInt(id, 10)),
			Kind:   database.ObjectKindTable,
			Schema: schema,
			Name:   name,
		},
		Description: "Referenced by foreign key " + constraint,
	}, selected)
	return nil
}

func (s *SQLServer) tableChildDependencies(
	ctx context.Context,
	catalogID int64,
	selected database.ObjectReference,
	target *sqlServerDependencySet,
) error {
	rows, err := s.conn.QueryContext(ctx, `
		SELECT
			child_object.object_id,
			child_object.type,
			child_object.name,
			child_schema.name
		FROM sys.objects child_object
		JOIN sys.schemas child_schema
			ON child_schema.schema_id = child_object.schema_id
		WHERE child_object.parent_object_id = @p1
			AND child_object.is_ms_shipped = 0
			AND child_object.type IN ('TR', 'TA', 'PK', 'UQ', 'F', 'C', 'D')
		ORDER BY child_object.type, child_object.name`,
		catalogID,
	)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var objectType, name, schema string
		if err := rows.Scan(&id, &objectType, &name, &schema); err != nil {
			_ = rows.Close()
			return err
		}
		kind := objectKind(objectType)
		target.add(database.ObjectDependency{
			Reference: database.ObjectReference{
				ID:           objectID("object", strconv.FormatInt(id, 10)),
				Kind:         kind,
				Schema:       schema,
				Name:         name,
				ParentSchema: selected.Schema,
				ParentName:   selected.Name,
			},
			Description: "Defined on this table",
		}, selected)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	indexRows, err := s.conn.QueryContext(ctx, `
		SELECT index_value.index_id, index_value.name
		FROM sys.indexes index_value
		WHERE index_value.object_id = @p1
			AND index_value.index_id > 0
			AND index_value.name IS NOT NULL
			AND index_value.is_hypothetical = 0
		ORDER BY index_value.name`,
		catalogID,
	)
	if err != nil {
		return err
	}
	defer indexRows.Close()
	for indexRows.Next() {
		var id int64
		var name string
		if err := indexRows.Scan(&id, &name); err != nil {
			return err
		}
		target.add(database.ObjectDependency{
			Reference: database.ObjectReference{
				ID: objectID(
					"index",
					selected.Name,
					strconv.FormatInt(id, 10),
				),
				Kind:         database.ObjectKindIndex,
				Schema:       selected.Schema,
				Name:         name,
				ParentSchema: selected.Schema,
				ParentName:   selected.Name,
			},
			Description: "Index defined on this table",
		}, selected)
	}
	return indexRows.Err()
}

func (s *SQLServer) objectDependencies(
	ctx context.Context,
	reference database.ObjectReference,
) ([]database.ObjectDependency, []database.ObjectDependency, error) {
	reference.Schema = s.defaultSchema(reference.Schema)
	id, err := s.sqlServerCatalogObjectID(ctx, reference)
	if err != nil {
		return nil, nil, err
	}
	dependencies := newSQLServerDependencySet()
	dependents := newSQLServerDependencySet()

	if reference.ParentName != "" {
		var parentID int64
		if err := s.conn.QueryRowContext(
			ctx,
			`SELECT table_value.object_id
			 FROM sys.tables table_value
			 JOIN sys.schemas schema_value
				ON schema_value.schema_id = table_value.schema_id
			 WHERE schema_value.name = @p1 AND table_value.name = @p2`,
			reference.ParentSchema,
			reference.ParentName,
		).Scan(&parentID); err == nil {
			dependencies.add(database.ObjectDependency{
				Reference: database.ObjectReference{
					ID:     objectID("object", strconv.FormatInt(parentID, 10)),
					Kind:   database.ObjectKindTable,
					Schema: reference.ParentSchema,
					Name:   reference.ParentName,
				},
				Description: "Defined on table",
			}, reference)
		}
	}

	if reference.Kind != database.ObjectKindIndex {
		if err := s.expressionDependencies(
			ctx,
			id,
			reference,
			false,
			dependencies,
		); err != nil {
			return nil, nil, err
		}
		if err := s.expressionDependencies(
			ctx,
			id,
			reference,
			true,
			dependents,
		); err != nil {
			return nil, nil, err
		}
	}

	if reference.Kind == database.ObjectKindTable {
		if err := s.foreignKeyDependencies(
			ctx,
			id,
			reference,
			false,
			dependencies,
		); err != nil {
			return nil, nil, err
		}
	}
	if reference.Kind == database.ObjectKindConstraint {
		if err := s.foreignKeyConstraintDependency(
			ctx,
			id,
			reference,
			dependencies,
		); err != nil {
			return nil, nil, err
		}
	}
	if reference.Kind == database.ObjectKindTable {
		if err := s.foreignKeyDependencies(
			ctx,
			id,
			reference,
			true,
			dependents,
		); err != nil {
			return nil, nil, err
		}
		if err := s.tableChildDependencies(
			ctx,
			id,
			reference,
			dependents,
		); err != nil {
			return nil, nil, err
		}
	}
	return dependencies.sorted(), dependents.sorted(), nil
}
