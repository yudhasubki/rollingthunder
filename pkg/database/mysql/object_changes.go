package mysql

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"rollingthunder/pkg/database"
)

var mysqlIndexMethodPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

func reviewedMySQLDDL(
	value string,
	kind database.ObjectKind,
) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("object definition is empty")
	}
	compoundObject := kind == database.ObjectKindFunction ||
		kind == database.ObjectKindProcedure ||
		kind == database.ObjectKindTrigger
	if !compoundObject && database.CountSQLStatements(value) != 1 {
		return "", fmt.Errorf("object definition must contain exactly one SQL statement")
	}
	if compoundObject &&
		strings.Contains(strings.ToUpper(value), "DELIMITER") {
		return "", fmt.Errorf(
			"DELIMITER is a command-line client directive; remove it before previewing the stored object",
		)
	}
	keywords := database.LeadingSQLKeywords(value, 16)
	if len(keywords) == 0 || keywords[0] != "CREATE" {
		return "", fmt.Errorf("object definition must start with CREATE")
	}
	expected := strings.ToUpper(string(kind))
	found := false
	for _, keyword := range keywords {
		if keyword == expected {
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf(
			"object definition does not create a %s",
			strings.ReplaceAll(string(kind), "_", " "),
		)
	}
	if compoundObject &&
		strings.Contains(strings.ToUpper(value), "BEGIN") {
		upper := strings.ToUpper(strings.TrimSuffix(value, ";"))
		if !strings.HasSuffix(strings.TrimSpace(upper), "END") {
			return "", fmt.Errorf(
				"compound MySQL routine or trigger definition must end with END",
			)
		}
	}
	if !strings.HasSuffix(value, ";") {
		value += ";"
	}
	return value, nil
}

func mysqlViewStatement(
	reference database.ObjectReference,
	body string,
	replace bool,
) (string, error) {
	body = strings.TrimSpace(body)
	if database.CountSQLStatements(body) != 1 {
		return "", fmt.Errorf("view definition must contain exactly one statement")
	}
	keywords := database.LeadingSQLKeywords(body, 2)
	if len(keywords) == 0 {
		return "", fmt.Errorf("view definition is empty")
	}
	if keywords[0] == "CREATE" {
		statement, err := reviewedMySQLDDL(body, database.ObjectKindView)
		if err != nil {
			return "", err
		}
		if replace {
			upper := strings.ToUpper(statement)
			index := strings.Index(upper, "CREATE ")
			if index >= 0 && !strings.Contains(upper[:min(len(upper), 32)], "OR REPLACE") {
				statement = statement[:index] + "CREATE OR REPLACE " +
					statement[index+len("CREATE "):]
			}
		}
		return statement, nil
	}
	switch keywords[0] {
	case "SELECT", "WITH", "VALUES", "TABLE":
	default:
		return "", fmt.Errorf(
			"view body must be a SELECT, WITH, VALUES, or TABLE statement",
		)
	}
	body = strings.TrimSuffix(body, ";")
	prefix := "CREATE VIEW"
	if replace {
		prefix = "CREATE OR REPLACE VIEW"
	}
	return fmt.Sprintf(
		"%s %s AS\n%s;",
		prefix,
		quoteMySQLQualifiedIdentifier(reference.Schema, reference.Name),
		body,
	), nil
}

func mysqlParentTable(
	reference database.ObjectReference,
) (string, error) {
	schema := reference.ParentSchema
	if schema == "" {
		schema = reference.Schema
	}
	if strings.TrimSpace(schema) == "" ||
		strings.TrimSpace(reference.ParentName) == "" {
		return "", fmt.Errorf("parent table is required for %s", reference.Kind)
	}
	return quoteMySQLQualifiedIdentifier(schema, reference.ParentName), nil
}

func mysqlDropStatement(
	reference database.ObjectReference,
) (string, error) {
	qualified := quoteMySQLQualifiedIdentifier(reference.Schema, reference.Name)
	switch reference.Kind {
	case database.ObjectKindTable:
		return "DROP TABLE IF EXISTS " + qualified + ";", nil
	case database.ObjectKindView:
		return "DROP VIEW IF EXISTS " + qualified + ";", nil
	case database.ObjectKindFunction:
		return "DROP FUNCTION IF EXISTS " + qualified + ";", nil
	case database.ObjectKindProcedure:
		return "DROP PROCEDURE IF EXISTS " + qualified + ";", nil
	case database.ObjectKindTrigger:
		return "DROP TRIGGER IF EXISTS " + qualified + ";", nil
	case database.ObjectKindIndex:
		parent, err := mysqlParentTable(reference)
		if err != nil {
			return "", err
		}
		if strings.EqualFold(reference.Name, "PRIMARY") {
			return "ALTER TABLE " + parent + " DROP PRIMARY KEY;", nil
		}
		return fmt.Sprintf(
			"ALTER TABLE %s DROP INDEX %s;",
			parent,
			quoteMySQLIdentifier(reference.Name),
		), nil
	default:
		return "", fmt.Errorf("dropping MySQL %s objects is not supported", reference.Kind)
	}
}

func mysqlRenameStatement(
	reference database.ObjectReference,
	newName string,
) (string, error) {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return "", fmt.Errorf("new object name is required")
	}
	switch reference.Kind {
	case database.ObjectKindTable, database.ObjectKindView:
		return fmt.Sprintf(
			"RENAME TABLE %s TO %s;",
			quoteMySQLQualifiedIdentifier(reference.Schema, reference.Name),
			quoteMySQLQualifiedIdentifier(reference.Schema, newName),
		), nil
	case database.ObjectKindIndex:
		parent, err := mysqlParentTable(reference)
		if err != nil {
			return "", err
		}
		if strings.EqualFold(reference.Name, "PRIMARY") {
			return "", fmt.Errorf("the MySQL PRIMARY index cannot be renamed")
		}
		return fmt.Sprintf(
			"ALTER TABLE %s RENAME INDEX %s TO %s;",
			parent,
			quoteMySQLIdentifier(reference.Name),
			quoteMySQLIdentifier(newName),
		), nil
	default:
		return "", fmt.Errorf(
			"MySQL cannot rename %s objects in place; recreate the object with the new name",
			reference.Kind,
		)
	}
}

func buildMySQLIndexChange(change database.IndexChange) (string, error) {
	if strings.TrimSpace(change.Table.Name) == "" ||
		strings.TrimSpace(change.Name) == "" {
		return "", fmt.Errorf("index table and name are required")
	}
	if strings.TrimSpace(change.Where) != "" {
		return "", fmt.Errorf("MySQL does not support partial-index predicates")
	}
	method := strings.ToUpper(strings.TrimSpace(change.Method))
	if method == "" {
		method = "BTREE"
	}
	if !mysqlIndexMethodPattern.MatchString(method) {
		return "", fmt.Errorf("invalid MySQL index method %q", change.Method)
	}
	switch method {
	case "BTREE", "HASH":
	default:
		return "", fmt.Errorf(
			"MySQL structural editor supports BTREE or HASH index methods",
		)
	}
	columns := make([]string, 0, len(change.Columns))
	seen := make(map[string]struct{}, len(change.Columns))
	for _, column := range change.Columns {
		column = strings.TrimSpace(column)
		if column == "" {
			return "", fmt.Errorf("index column cannot be empty")
		}
		if _, duplicate := seen[column]; duplicate {
			continue
		}
		seen[column] = struct{}{}
		columns = append(columns, quoteMySQLIdentifier(column))
	}
	if len(columns) == 0 {
		return "", fmt.Errorf("at least one index column is required")
	}
	unique := ""
	if change.Unique {
		unique = "UNIQUE "
	}
	return fmt.Sprintf(
		"CREATE %sINDEX %s ON %s (%s) USING %s;",
		unique,
		quoteMySQLIdentifier(change.Name),
		quoteMySQLQualifiedIdentifier(change.Table.Schema, change.Table.Name),
		strings.Join(columns, ", "),
		method,
	), nil
}

func (m *MySQL) buildMySQLColumnChange(
	change database.ColumnChange,
) ([]string, error) {
	change.Table.Schema = m.defaultDatabase(change.Table.Schema)
	structures, err := m.GetCollectionStructures(change.Table)
	if err != nil {
		return nil, err
	}
	var current *database.Structure
	for index := range structures {
		if structures[index].Name == change.Name {
			current = &structures[index]
			break
		}
	}
	if current == nil {
		return nil, fmt.Errorf("column %q was not found", change.Name)
	}
	if strings.TrimSpace(change.Using) != "" {
		return nil, fmt.Errorf(
			"MySQL does not support a PostgreSQL-style USING conversion expression",
		)
	}

	table := quoteMySQLQualifiedIdentifier(change.Table.Schema, change.Table.Name)
	activeName := strings.TrimSpace(change.Name)
	statements := make([]string, 0, 2)
	if strings.TrimSpace(change.NewName) != "" &&
		strings.TrimSpace(change.NewName) != activeName {
		statements = append(statements, fmt.Sprintf(
			"ALTER TABLE %s RENAME COLUMN %s TO %s;",
			table,
			quoteMySQLIdentifier(activeName),
			quoteMySQLIdentifier(change.NewName),
		))
		activeName = strings.TrimSpace(change.NewName)
	}

	needsModify := strings.TrimSpace(change.DataType) != "" ||
		change.Nullable != nil ||
		change.Default != nil ||
		change.DropDefault
	if needsModify {
		dataType := strings.TrimSpace(change.DataType)
		if dataType == "" {
			dataType = current.DataType
		}
		if err := database.ValidateDDLFragment(dataType, "column data type"); err != nil {
			return nil, err
		}
		nullable := current.Nullable
		if change.Nullable != nil {
			nullable = *change.Nullable
		}
		definition := fmt.Sprintf(
			"ALTER TABLE %s MODIFY COLUMN %s %s",
			table,
			quoteMySQLIdentifier(activeName),
			dataType,
		)
		if nullable {
			definition += " NULL"
		} else {
			definition += " NOT NULL"
		}
		switch {
		case change.Default != nil:
			if err := database.ValidateDDLFragment(
				*change.Default,
				"column default",
			); err != nil {
				return nil, err
			}
			definition += " DEFAULT " + strings.TrimSpace(*change.Default)
		case change.DropDefault:
		case current.Default != nil:
			definition += " DEFAULT " + *current.Default
		}
		if current.IsAutoInc {
			definition += " AUTO_INCREMENT"
		}
		statements = append(statements, definition+";")
	}
	if len(statements) == 0 {
		return nil, fmt.Errorf("column change has no effective alterations")
	}
	return statements, nil
}

func buildMySQLAddColumn(
	change database.AddColumnChange,
) (string, error) {
	column := change.Column
	if err := database.ValidateDDLFragment(column.Type, "column data type"); err != nil {
		return "", err
	}
	if err := database.ValidateDDLFragment(column.Default, "column default"); err != nil {
		return "", err
	}
	statement := fmt.Sprintf(
		"ALTER TABLE %s ADD COLUMN %s %s",
		quoteMySQLQualifiedIdentifier(change.Table.Schema, change.Table.Name),
		quoteMySQLIdentifier(strings.TrimSpace(column.Name)),
		strings.TrimSpace(column.Type),
	)
	if column.Nullable {
		statement += " NULL"
	} else {
		statement += " NOT NULL"
	}
	if strings.TrimSpace(column.Default) != "" {
		statement += " DEFAULT " + strings.TrimSpace(column.Default)
	}
	if column.Unique {
		statement += " UNIQUE"
	}
	if column.PrimaryKey {
		statement += " PRIMARY KEY"
	}
	switch {
	case change.First:
		statement += " FIRST"
	case strings.TrimSpace(change.After) != "":
		statement += " AFTER " + quoteMySQLIdentifier(strings.TrimSpace(change.After))
	}
	return statement + ";", nil
}

func buildMySQLDropColumn(change database.DropColumnChange) string {
	return fmt.Sprintf(
		"ALTER TABLE %s DROP COLUMN %s;",
		quoteMySQLQualifiedIdentifier(change.Table.Schema, change.Table.Name),
		quoteMySQLIdentifier(strings.TrimSpace(change.Name)),
	)
}

func validateMySQLConstraintDefinition(value string) error {
	value = strings.TrimSpace(value)
	if err := database.ValidateDDLFragment(value, "constraint definition"); err != nil {
		return err
	}
	keywords := database.LeadingSQLKeywords(value, 2)
	if len(keywords) == 0 {
		return fmt.Errorf("constraint definition is empty")
	}
	switch keywords[0] {
	case "PRIMARY", "UNIQUE", "FOREIGN", "CHECK":
		return nil
	default:
		return fmt.Errorf(
			"constraint must start with PRIMARY KEY, UNIQUE, FOREIGN KEY, or CHECK",
		)
	}
}

func (m *MySQL) mysqlConstraintType(
	ctx context.Context,
	change database.ConstraintChange,
) (string, error) {
	schema := m.defaultDatabase(change.Table.Schema)
	var constraintType string
	err := m.conn.GetContext(ctx, &constraintType, `
		SELECT constraint_type
		FROM information_schema.table_constraints
		WHERE table_schema = ?
		  AND table_name = ?
		  AND constraint_name = ?`,
		schema,
		change.Table.Name,
		change.Name,
	)
	if err != nil {
		return "", err
	}
	return strings.ToUpper(constraintType), nil
}

func (m *MySQL) buildMySQLConstraintChange(
	ctx context.Context,
	action database.ObjectChangeAction,
	change database.ConstraintChange,
) (string, bool, error) {
	change.Table.Schema = m.defaultDatabase(change.Table.Schema)
	table := quoteMySQLQualifiedIdentifier(change.Table.Schema, change.Table.Name)
	name := quoteMySQLIdentifier(change.Name)
	if action == database.ObjectChangeAddConstraint {
		if err := validateMySQLConstraintDefinition(change.Definition); err != nil {
			return "", false, err
		}
		return fmt.Sprintf(
			"ALTER TABLE %s ADD CONSTRAINT %s %s;",
			table,
			name,
			strings.TrimSpace(change.Definition),
		), false, nil
	}

	constraintType, err := m.mysqlConstraintType(ctx, change)
	if err != nil {
		return "", false, err
	}
	switch constraintType {
	case "PRIMARY KEY":
		return "ALTER TABLE " + table + " DROP PRIMARY KEY;", true, nil
	case "FOREIGN KEY":
		return fmt.Sprintf(
			"ALTER TABLE %s DROP FOREIGN KEY %s;",
			table,
			name,
		), true, nil
	case "UNIQUE":
		return fmt.Sprintf(
			"ALTER TABLE %s DROP INDEX %s;",
			table,
			name,
		), true, nil
	case "CHECK":
		return fmt.Sprintf(
			"ALTER TABLE %s DROP CHECK %s;",
			table,
			name,
		), true, nil
	default:
		return "", false, fmt.Errorf(
			"unsupported MySQL constraint type %q",
			constraintType,
		)
	}
}

func mysqlRefreshReference(
	kind database.ObjectKind,
	table database.Table,
) database.ObjectReference {
	return database.ObjectReference{
		Kind:   kind,
		Schema: table.Schema,
		Name:   table.Name,
	}
}

func (m *MySQL) BuildObjectChange(
	ctx context.Context,
	request database.ObjectChangeRequest,
) (database.ObjectChangePlan, error) {
	if err := request.Validate(); err != nil {
		return database.ObjectChangePlan{}, err
	}
	if ctx == nil {
		ctx = m.ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if request.Cascade {
		return database.ObjectChangePlan{}, fmt.Errorf(
			"MySQL does not support CASCADE for this reviewed object change",
		)
	}
	request.Reference.Schema = m.defaultDatabase(request.Reference.Schema)

	switch request.Action {
	case database.ObjectChangeCreate, database.ObjectChangeReplace:
		replace := request.Action == database.ObjectChangeReplace
		var statements []string
		var warnings []string
		switch request.Reference.Kind {
		case database.ObjectKindView:
			statement, err := mysqlViewStatement(
				request.Reference,
				request.Definition,
				replace,
			)
			if err != nil {
				return database.ObjectChangePlan{}, err
			}
			statements = []string{statement}
		case database.ObjectKindFunction,
			database.ObjectKindProcedure,
			database.ObjectKindTrigger:
			statement, err := reviewedMySQLDDL(
				request.Definition,
				request.Reference.Kind,
			)
			if err != nil {
				return database.ObjectChangePlan{}, err
			}
			if replace {
				drop, err := mysqlDropStatement(request.Reference)
				if err != nil {
					return database.ObjectChangePlan{}, err
				}
				statements = []string{drop, statement}
				warnings = append(
					warnings,
					"MySQL replaces this object with DROP followed by CREATE; DDL auto-commits and cannot be rolled back as one unit.",
				)
			} else {
				statements = []string{statement}
			}
		default:
			return database.ObjectChangePlan{}, fmt.Errorf(
				"creating MySQL %s objects is not supported by the structural editor",
				request.Reference.Kind,
			)
		}
		verb := "Create"
		if replace {
			verb = "Replace"
		}
		return database.ObjectChangePlan{
			Summary: fmt.Sprintf(
				"%s %s %s",
				verb,
				request.Reference.Kind,
				request.Reference.QualifiedName(),
			),
			Statements:    statements,
			Transactional: false,
			Warnings:      warnings,
			Refresh:       []database.ObjectReference{request.Reference},
		}, nil

	case database.ObjectChangeRename:
		statement, err := mysqlRenameStatement(request.Reference, request.NewName)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		refresh := request.Reference
		refresh.ID = ""
		refresh.Name = strings.TrimSpace(request.NewName)
		return database.ObjectChangePlan{
			Summary: fmt.Sprintf(
				"Rename %s %s to %s",
				request.Reference.Kind,
				request.Reference.QualifiedName(),
				refresh.Name,
			),
			Statements:    []string{statement},
			Transactional: false,
			Warnings: []string{
				"MySQL DDL auto-commits; the rename cannot be rolled back by Rolling Thunder.",
			},
			Refresh: []database.ObjectReference{refresh},
		}, nil

	case database.ObjectChangeDrop:
		statement, err := mysqlDropStatement(request.Reference)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		return database.ObjectChangePlan{
			Summary: fmt.Sprintf(
				"Drop %s %s",
				request.Reference.Kind,
				request.Reference.QualifiedName(),
			),
			Statements:    []string{statement},
			Destructive:   true,
			Transactional: false,
			Warnings: []string{
				"Dropping an object is permanent.",
				"MySQL DDL auto-commits and cannot be rolled back by Rolling Thunder.",
			},
			Refresh: []database.ObjectReference{request.Reference},
		}, nil

	case database.ObjectChangeEnable, database.ObjectChangeDisable:
		return database.ObjectChangePlan{}, fmt.Errorf(
			"MySQL does not support enabling or disabling a trigger without dropping it",
		)

	case database.ObjectChangeCreateIndex:
		request.Index.Table.Schema = m.defaultDatabase(request.Index.Table.Schema)
		statement, err := buildMySQLIndexChange(*request.Index)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		return database.ObjectChangePlan{
			Summary: fmt.Sprintf(
				"Create index %s on %s",
				request.Index.Name,
				request.Index.Table.Name,
			),
			Statements:    []string{statement},
			Transactional: false,
			Warnings: []string{
				"Creating an index may lock or rebuild the table, depending on the server version and storage engine.",
			},
			Refresh: []database.ObjectReference{
				mysqlRefreshReference(database.ObjectKindTable, request.Index.Table),
			},
		}, nil

	case database.ObjectChangeAddColumn:
		request.AddColumn.Table.Schema = m.defaultDatabase(
			request.AddColumn.Table.Schema,
		)
		statement, err := buildMySQLAddColumn(*request.AddColumn)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		return database.ObjectChangePlan{
			Summary: fmt.Sprintf(
				"Add column %s to %s",
				request.AddColumn.Column.Name,
				request.AddColumn.Table.Name,
			),
			Statements:    []string{statement},
			Transactional: false,
			Warnings: []string{
				"Adding a column can rebuild and lock the table.",
				"MySQL DDL auto-commits and cannot be rolled back by Rolling Thunder.",
			},
			Refresh: []database.ObjectReference{
				mysqlRefreshReference(
					database.ObjectKindTable,
					request.AddColumn.Table,
				),
			},
		}, nil

	case database.ObjectChangeAlterColumn:
		request.Column.Table.Schema = m.defaultDatabase(request.Column.Table.Schema)
		statements, err := m.buildMySQLColumnChange(*request.Column)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		return database.ObjectChangePlan{
			Summary: fmt.Sprintf(
				"Alter column %s on %s",
				request.Column.Name,
				request.Column.Table.Name,
			),
			Statements:    statements,
			Transactional: false,
			Warnings: []string{
				"Changing a column can rebuild and lock the table.",
				"MySQL DDL auto-commits; multiple statements may be partially applied if a later statement fails.",
			},
			Refresh: []database.ObjectReference{
				mysqlRefreshReference(database.ObjectKindTable, request.Column.Table),
			},
		}, nil

	case database.ObjectChangeDropColumn:
		request.DropColumn.Table.Schema = m.defaultDatabase(
			request.DropColumn.Table.Schema,
		)
		return database.ObjectChangePlan{
			Summary: fmt.Sprintf(
				"Drop column %s from %s",
				request.DropColumn.Name,
				request.DropColumn.Table.Name,
			),
			Statements: []string{
				buildMySQLDropColumn(*request.DropColumn),
			},
			Destructive:   true,
			Transactional: false,
			Warnings: []string{
				"Dropping a column permanently removes its data.",
				"MySQL DDL auto-commits and cannot be rolled back by Rolling Thunder.",
			},
			Refresh: []database.ObjectReference{
				mysqlRefreshReference(
					database.ObjectKindTable,
					request.DropColumn.Table,
				),
			},
		}, nil

	case database.ObjectChangeAddConstraint,
		database.ObjectChangeDropConstraint:
		request.Constraint.Table.Schema = m.defaultDatabase(
			request.Constraint.Table.Schema,
		)
		statement, destructive, err := m.buildMySQLConstraintChange(
			ctx,
			request.Action,
			*request.Constraint,
		)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		verb := "Add"
		if destructive {
			verb = "Drop"
		}
		return database.ObjectChangePlan{
			Summary: fmt.Sprintf(
				"%s constraint %s on %s",
				verb,
				request.Constraint.Name,
				request.Constraint.Table.Name,
			),
			Statements:    []string{statement},
			Destructive:   destructive,
			Transactional: false,
			Warnings: []string{
				"MySQL DDL auto-commits and cannot be rolled back by Rolling Thunder.",
			},
			Refresh: []database.ObjectReference{
				mysqlRefreshReference(
					database.ObjectKindTable,
					request.Constraint.Table,
				),
			},
		}, nil
	default:
		return database.ObjectChangePlan{}, fmt.Errorf(
			"unsupported MySQL object change %q",
			request.Action,
		)
	}
}

func (m *MySQL) ApplyObjectChange(
	ctx context.Context,
	plan database.ObjectChangePlan,
) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	if plan.Transactional {
		return fmt.Errorf(
			"MySQL structural plans must be marked non-transactional",
		)
	}
	for index, statement := range plan.Statements {
		if _, err := m.conn.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf(
				"execute structural statement %d of %d: %w",
				index+1,
				len(plan.Statements),
				err,
			)
		}
	}
	return nil
}

var _ database.ObjectChangeDriver = (*MySQL)(nil)
