package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"rollingthunder/pkg/database"
)

type oracleDependencySet struct {
	items []database.ObjectDependency
	seen  map[string]struct{}
}

func newOracleDependencySet() *oracleDependencySet {
	return &oracleDependencySet{
		items: make([]database.ObjectDependency, 0),
		seen:  make(map[string]struct{}),
	}
}

func oracleDependencyKey(reference database.ObjectReference) string {
	return strings.ToUpper(strings.Join([]string{
		string(reference.Kind),
		reference.Schema,
		reference.Name,
		reference.ParentSchema,
		reference.ParentName,
	}, "\x00"))
}

func (set *oracleDependencySet) add(
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
	key := oracleDependencyKey(reference)
	if _, exists := set.seen[key]; exists {
		return
	}
	set.seen[key] = struct{}{}
	set.items = append(set.items, dependency)
}

func (set *oracleDependencySet) sorted() []database.ObjectDependency {
	sort.SliceStable(set.items, func(left, right int) bool {
		leftReference := set.items[left].Reference
		rightReference := set.items[right].Reference
		return strings.ToUpper(
			leftReference.QualifiedName()+"\x00"+string(leftReference.Kind),
		) < strings.ToUpper(
			rightReference.QualifiedName()+"\x00"+string(rightReference.Kind),
		)
	})
	return set.items
}

func oracleDependencyType(kind database.ObjectKind) (string, error) {
	switch kind {
	case database.ObjectKindTable:
		return "TABLE", nil
	case database.ObjectKindView:
		return "VIEW", nil
	case database.ObjectKindMaterializedView:
		return "MATERIALIZED VIEW", nil
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
		return "", fmt.Errorf(
			"Oracle dependencies are unavailable for %q",
			kind,
		)
	}
}

func oracleDependencyReference(
	schema string,
	name string,
	objectType string,
) database.ObjectReference {
	reference := database.ObjectReference{
		Kind:   oracleObjectKind(objectType),
		Schema: schema,
		Name:   name,
	}
	if reference.Kind != database.ObjectKindUnknown {
		reference.ID = objectID(reference)
	}
	return reference
}

func (o *Oracle) catalogDependencies(
	ctx context.Context,
	selected database.ObjectReference,
	reverse bool,
	target *oracleDependencySet,
) error {
	objectType, err := oracleDependencyType(selected.Kind)
	if err != nil {
		return err
	}
	query := `
		SELECT DISTINCT
			referenced_owner,
			referenced_name,
			referenced_type
		FROM all_dependencies
		WHERE owner = :owner
			AND name = :object_name
			AND type = :object_type
			AND referenced_owner IS NOT NULL
			AND referenced_name IS NOT NULL
			AND referenced_owner NOT IN ('SYS', 'PUBLIC')
		ORDER BY referenced_owner, referenced_type, referenced_name`
	description := "Referenced by Oracle dependency metadata"
	if reverse {
		query = `
			SELECT DISTINCT
				owner,
				name,
				type
			FROM all_dependencies
			WHERE referenced_owner = :owner
				AND referenced_name = :object_name
				AND referenced_type = :object_type
				AND owner NOT IN ('SYS', 'PUBLIC')
			ORDER BY owner, type, name`
		description = "References this object"
	}
	rows, err := o.conn.QueryContext(
		ctx,
		query,
		sql.Named("owner", selected.Schema),
		sql.Named("object_name", selected.Name),
		sql.Named("object_type", objectType),
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var schema, name, dependencyType string
		if err := rows.Scan(&schema, &name, &dependencyType); err != nil {
			return err
		}
		target.add(database.ObjectDependency{
			Reference: oracleDependencyReference(
				schema,
				name,
				dependencyType,
			),
			Description: description + " · " +
				strings.ToLower(dependencyType),
		}, selected)
	}
	return rows.Err()
}

func (o *Oracle) foreignKeyDependencies(
	ctx context.Context,
	selected database.ObjectReference,
	reverse bool,
	target *oracleDependencySet,
) error {
	query := `
		SELECT DISTINCT
			referenced_constraint.owner,
			referenced_constraint.table_name,
			foreign_key.constraint_name
		FROM all_constraints foreign_key
		JOIN all_constraints referenced_constraint
			ON referenced_constraint.owner = foreign_key.r_owner
			AND referenced_constraint.constraint_name =
				foreign_key.r_constraint_name
		WHERE foreign_key.owner = :1
			AND foreign_key.table_name = :2
			AND foreign_key.constraint_type = 'R'
		ORDER BY
			referenced_constraint.owner,
			referenced_constraint.table_name,
			foreign_key.constraint_name`
	description := "Referenced by foreign key"
	if reverse {
		query = `
			SELECT DISTINCT
				foreign_key.owner,
				foreign_key.table_name,
				foreign_key.constraint_name
			FROM all_constraints foreign_key
			JOIN all_constraints referenced_constraint
				ON referenced_constraint.owner = foreign_key.r_owner
				AND referenced_constraint.constraint_name =
					foreign_key.r_constraint_name
			WHERE referenced_constraint.owner = :1
				AND referenced_constraint.table_name = :2
				AND foreign_key.constraint_type = 'R'
			ORDER BY
				foreign_key.owner,
				foreign_key.table_name,
				foreign_key.constraint_name`
		description = "References this table through foreign key"
	}
	rows, err := o.conn.QueryContext(
		ctx,
		query,
		selected.Schema,
		selected.Name,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var schema, name, constraint string
		if err := rows.Scan(&schema, &name, &constraint); err != nil {
			return err
		}
		reference := database.ObjectReference{
			Kind:   database.ObjectKindTable,
			Schema: schema,
			Name:   name,
		}
		reference.ID = objectID(reference)
		target.add(database.ObjectDependency{
			Reference:   reference,
			Description: description + " " + constraint,
		}, selected)
	}
	return rows.Err()
}

func (o *Oracle) foreignKeyConstraintDependency(
	ctx context.Context,
	selected database.ObjectReference,
	target *oracleDependencySet,
) error {
	var schema, table, constraint string
	err := o.conn.QueryRowContext(ctx, `
		SELECT
			referenced_constraint.owner,
			referenced_constraint.table_name,
			foreign_key.constraint_name
		FROM all_constraints foreign_key
		JOIN all_constraints referenced_constraint
			ON referenced_constraint.owner = foreign_key.r_owner
			AND referenced_constraint.constraint_name =
				foreign_key.r_constraint_name
		WHERE foreign_key.owner = :1
			AND foreign_key.constraint_name = :2
			AND foreign_key.constraint_type = 'R'`,
		selected.Schema,
		selected.Name,
	).Scan(&schema, &table, &constraint)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	reference := database.ObjectReference{
		Kind:   database.ObjectKindTable,
		Schema: schema,
		Name:   table,
	}
	reference.ID = objectID(reference)
	target.add(database.ObjectDependency{
		Reference:   reference,
		Description: "Referenced by foreign key " + constraint,
	}, selected)
	return nil
}

func (o *Oracle) oracleParentDependency(
	ctx context.Context,
	selected database.ObjectReference,
	target *oracleDependencySet,
) error {
	if selected.ParentName != "" {
		reference := database.ObjectReference{
			Kind:   database.ObjectKindTable,
			Schema: selected.ParentSchema,
			Name:   selected.ParentName,
		}
		reference.ID = objectID(reference)
		target.add(database.ObjectDependency{
			Reference:   reference,
			Description: "Defined on table",
		}, selected)
		return nil
	}
	if selected.Kind != database.ObjectKindConstraint {
		return nil
	}
	var tableName string
	if err := o.conn.QueryRowContext(
		ctx,
		`SELECT table_name
		 FROM all_constraints
		 WHERE owner = :1 AND constraint_name = :2`,
		selected.Schema,
		selected.Name,
	).Scan(&tableName); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	reference := database.ObjectReference{
		Kind:   database.ObjectKindTable,
		Schema: selected.Schema,
		Name:   tableName,
	}
	reference.ID = objectID(reference)
	target.add(database.ObjectDependency{
		Reference:   reference,
		Description: "Defined on table",
	}, selected)
	return nil
}

func (o *Oracle) tableChildDependencies(
	ctx context.Context,
	selected database.ObjectReference,
	target *oracleDependencySet,
) error {
	rows, err := o.conn.QueryContext(ctx, `
		SELECT object_type, object_name
		FROM all_objects
		WHERE owner = :1
			AND object_type IN ('INDEX', 'TRIGGER')
			AND object_name IN (
				SELECT index_name
				FROM all_indexes
				WHERE table_owner = :2 AND table_name = :3
				UNION ALL
				SELECT trigger_name
				FROM all_triggers
				WHERE table_owner = :4 AND table_name = :5
			)
		ORDER BY object_type, object_name`,
		selected.Schema,
		selected.Schema,
		selected.Name,
		selected.Schema,
		selected.Name,
	)
	if err != nil {
		return err
	}
	for rows.Next() {
		var objectType, name string
		if err := rows.Scan(&objectType, &name); err != nil {
			_ = rows.Close()
			return err
		}
		reference := database.ObjectReference{
			Kind:         oracleObjectKind(objectType),
			Schema:       selected.Schema,
			Name:         name,
			ParentSchema: selected.Schema,
			ParentName:   selected.Name,
		}
		reference.ID = objectID(reference)
		label := strings.ToLower(objectType)
		if label != "" {
			label = strings.ToUpper(label[:1]) + label[1:]
		}
		target.add(database.ObjectDependency{
			Reference:   reference,
			Description: label + " defined on this table",
		}, selected)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	constraintRows, err := o.conn.QueryContext(ctx, `
		SELECT constraint_name
		FROM all_constraints
		WHERE owner = :1
			AND table_name = :2
			AND constraint_type IN ('P', 'U', 'R', 'C')
			AND (constraint_type <> 'C' OR generated = 'USER NAME')
		ORDER BY constraint_name`,
		selected.Schema,
		selected.Name,
	)
	if err != nil {
		return err
	}
	defer constraintRows.Close()
	for constraintRows.Next() {
		var name string
		if err := constraintRows.Scan(&name); err != nil {
			return err
		}
		reference := database.ObjectReference{
			Kind:         database.ObjectKindConstraint,
			Schema:       selected.Schema,
			Name:         name,
			ParentSchema: selected.Schema,
			ParentName:   selected.Name,
		}
		reference.ID = objectID(reference)
		target.add(database.ObjectDependency{
			Reference:   reference,
			Description: "Constraint defined on this table",
		}, selected)
	}
	return constraintRows.Err()
}

func (o *Oracle) objectDependencies(
	ctx context.Context,
	reference database.ObjectReference,
) ([]database.ObjectDependency, []database.ObjectDependency, error) {
	reference.Schema = o.defaultSchema(reference.Schema)
	dependencies := newOracleDependencySet()
	dependents := newOracleDependencySet()

	if err := o.oracleParentDependency(
		ctx,
		reference,
		dependencies,
	); err != nil {
		return nil, nil, err
	}
	if reference.Kind != database.ObjectKindIndex &&
		reference.Kind != database.ObjectKindConstraint {
		if err := o.catalogDependencies(
			ctx,
			reference,
			false,
			dependencies,
		); err != nil {
			return nil, nil, err
		}
		if err := o.catalogDependencies(
			ctx,
			reference,
			true,
			dependents,
		); err != nil {
			return nil, nil, err
		}
	}
	if reference.Kind == database.ObjectKindTable {
		if err := o.foreignKeyDependencies(
			ctx,
			reference,
			false,
			dependencies,
		); err != nil {
			return nil, nil, err
		}
		if err := o.foreignKeyDependencies(
			ctx,
			reference,
			true,
			dependents,
		); err != nil {
			return nil, nil, err
		}
		if err := o.tableChildDependencies(
			ctx,
			reference,
			dependents,
		); err != nil {
			return nil, nil, err
		}
	}
	if reference.Kind == database.ObjectKindConstraint {
		if err := o.foreignKeyConstraintDependency(
			ctx,
			reference,
			dependencies,
		); err != nil {
			return nil, nil, err
		}
	}
	return dependencies.sorted(), dependents.sorted(), nil
}
