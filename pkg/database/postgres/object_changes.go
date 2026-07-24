package postgres

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"rollingthunder/pkg/database"
)

var postgresIndexMethodPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func reviewedDDL(value string, kind database.ObjectKind) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("object definition is empty")
	}
	if database.CountSQLStatements(value) != 1 {
		return "", fmt.Errorf("object definition must contain exactly one SQL statement")
	}
	keywords := database.LeadingSQLKeywords(value, 6)
	if len(keywords) == 0 || keywords[0] != "CREATE" {
		return "", fmt.Errorf("object definition must start with CREATE")
	}

	expected := ""
	switch kind {
	case database.ObjectKindView:
		expected = "VIEW"
	case database.ObjectKindMaterializedView:
		expected = "MATERIALIZED"
	case database.ObjectKindFunction:
		expected = "FUNCTION"
	case database.ObjectKindProcedure:
		expected = "PROCEDURE"
	case database.ObjectKindTrigger:
		expected = "TRIGGER"
	default:
		return "", fmt.Errorf("creating %s objects is not supported", kind)
	}
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
	if !strings.HasSuffix(value, ";") {
		value += ";"
	}
	return value, nil
}

func postgresViewBody(
	reference database.ObjectReference,
	body string,
	materialized bool,
	replace bool,
) (string, error) {
	body = strings.TrimSpace(body)
	if database.CountSQLStatements(body) != 1 {
		return "", fmt.Errorf("view query must contain exactly one statement")
	}
	keywords := database.LeadingSQLKeywords(body, 1)
	if len(keywords) == 0 {
		return "", fmt.Errorf("view query is empty")
	}
	switch keywords[0] {
	case "SELECT", "WITH", "VALUES", "TABLE":
	default:
		return reviewedDDL(body, reference.Kind)
	}
	body = strings.TrimSuffix(body, ";")
	qualified := quotePostgresQualifiedIdentifier(reference.Schema, reference.Name)
	if materialized {
		if replace {
			return "", fmt.Errorf(
				"PostgreSQL cannot replace a materialized view in place; drop and recreate it after reviewing dependencies",
			)
		}
		return fmt.Sprintf(
			"CREATE MATERIALIZED VIEW %s AS\n%s;",
			qualified,
			body,
		), nil
	}
	prefix := "CREATE VIEW"
	if replace {
		prefix = "CREATE OR REPLACE VIEW"
	}
	return fmt.Sprintf("%s %s AS\n%s;", prefix, qualified, body), nil
}

func validatePostgresFragment(value, label string) error {
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s contains a NUL byte", label)
	}
	if database.HasTopLevelStatementSeparator(value) {
		return fmt.Errorf("%s must not contain another SQL statement", label)
	}
	return nil
}

func validatePostgresRoutineSignature(signature string) error {
	if err := validatePostgresFragment(signature, "routine signature"); err != nil {
		return err
	}
	for _, value := range signature {
		switch {
		case unicode.IsLetter(value), unicode.IsDigit(value), unicode.IsSpace(value):
		case strings.ContainsRune(`_,."[]()%`, value):
		default:
			return fmt.Errorf("routine signature contains unsupported character %q", value)
		}
	}
	return nil
}

func postgresRoutineName(reference database.ObjectReference) (string, error) {
	if err := validatePostgresRoutineSignature(reference.Signature); err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"%s(%s)",
		quotePostgresQualifiedIdentifier(reference.Schema, reference.Name),
		reference.Signature,
	), nil
}

func postgresParentTable(reference database.ObjectReference) (string, error) {
	parentSchema := reference.ParentSchema
	if parentSchema == "" {
		parentSchema = reference.Schema
	}
	if strings.TrimSpace(parentSchema) == "" ||
		strings.TrimSpace(reference.ParentName) == "" {
		return "", fmt.Errorf("the parent table is required for %s", reference.Kind)
	}
	return quotePostgresQualifiedIdentifier(parentSchema, reference.ParentName), nil
}

func postgresRefreshReference(
	kind database.ObjectKind,
	table database.Table,
) database.ObjectReference {
	return database.ObjectReference{
		Kind:   kind,
		Schema: table.Schema,
		Name:   table.Name,
	}
}

func (p *Postgres) buildPostgresCreateObject(
	request database.ObjectChangeRequest,
) (database.ObjectChangePlan, error) {
	reference := request.Reference
	var statements []string
	var err error
	replace := request.Action == database.ObjectChangeReplace

	switch reference.Kind {
	case database.ObjectKindView:
		var statement string
		statement, err = postgresViewBody(reference, request.Definition, false, replace)
		statements = []string{statement}
	case database.ObjectKindMaterializedView:
		var statement string
		statement, err = postgresViewBody(reference, request.Definition, true, replace)
		statements = []string{statement}
	case database.ObjectKindFunction,
		database.ObjectKindProcedure:
		var statement string
		statement, err = reviewedDDL(request.Definition, reference.Kind)
		statements = []string{statement}
	case database.ObjectKindTrigger:
		var statement string
		statement, err = reviewedDDL(request.Definition, reference.Kind)
		statements = []string{statement}
		if err == nil && replace {
			var drop string
			drop, err = postgresDropStatement(reference, false)
			if err == nil {
				statements = []string{drop, statement}
			}
		}
	default:
		return database.ObjectChangePlan{}, fmt.Errorf(
			"creating %s objects is not supported by the structural editor",
			reference.Kind,
		)
	}
	if err != nil {
		return database.ObjectChangePlan{}, err
	}

	verb := "Create"
	if replace {
		verb = "Create or replace"
	}
	return database.ObjectChangePlan{
		Summary:       fmt.Sprintf("%s %s %s", verb, reference.Kind, reference.QualifiedName()),
		Statements:    statements,
		Transactional: true,
		Refresh:       []database.ObjectReference{reference},
	}, nil
}

func postgresRenameStatement(
	reference database.ObjectReference,
	newName string,
) (string, error) {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return "", fmt.Errorf("new object name is required")
	}
	qualified := quotePostgresQualifiedIdentifier(reference.Schema, reference.Name)
	quotedNewName := quotePostgresIdentifier(newName)

	switch reference.Kind {
	case database.ObjectKindTable:
		return fmt.Sprintf("ALTER TABLE %s RENAME TO %s;", qualified, quotedNewName), nil
	case database.ObjectKindView:
		return fmt.Sprintf("ALTER VIEW %s RENAME TO %s;", qualified, quotedNewName), nil
	case database.ObjectKindMaterializedView:
		return fmt.Sprintf(
			"ALTER MATERIALIZED VIEW %s RENAME TO %s;",
			qualified,
			quotedNewName,
		), nil
	case database.ObjectKindSequence:
		return fmt.Sprintf("ALTER SEQUENCE %s RENAME TO %s;", qualified, quotedNewName), nil
	case database.ObjectKindType, database.ObjectKindEnum:
		return fmt.Sprintf("ALTER TYPE %s RENAME TO %s;", qualified, quotedNewName), nil
	case database.ObjectKindDomain:
		return fmt.Sprintf("ALTER DOMAIN %s RENAME TO %s;", qualified, quotedNewName), nil
	case database.ObjectKindIndex:
		return fmt.Sprintf("ALTER INDEX %s RENAME TO %s;", qualified, quotedNewName), nil
	case database.ObjectKindFunction, database.ObjectKindProcedure:
		routine, err := postgresRoutineName(reference)
		if err != nil {
			return "", err
		}
		keyword := strings.ToUpper(string(reference.Kind))
		return fmt.Sprintf("ALTER %s %s RENAME TO %s;", keyword, routine, quotedNewName), nil
	case database.ObjectKindTrigger:
		parent, err := postgresParentTable(reference)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(
			"ALTER TRIGGER %s ON %s RENAME TO %s;",
			quotePostgresIdentifier(reference.Name),
			parent,
			quotedNewName,
		), nil
	case database.ObjectKindConstraint:
		parent, err := postgresParentTable(reference)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(
			"ALTER TABLE %s RENAME CONSTRAINT %s TO %s;",
			parent,
			quotePostgresIdentifier(reference.Name),
			quotedNewName,
		), nil
	default:
		return "", fmt.Errorf("renaming %s objects is not supported", reference.Kind)
	}
}

func postgresDropStatement(
	reference database.ObjectReference,
	cascade bool,
) (string, error) {
	suffix := ""
	if cascade {
		suffix = " CASCADE"
	}
	qualified := quotePostgresQualifiedIdentifier(reference.Schema, reference.Name)

	switch reference.Kind {
	case database.ObjectKindTable:
		return fmt.Sprintf("DROP TABLE IF EXISTS %s%s;", qualified, suffix), nil
	case database.ObjectKindView:
		return fmt.Sprintf("DROP VIEW IF EXISTS %s%s;", qualified, suffix), nil
	case database.ObjectKindMaterializedView:
		return fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s%s;", qualified, suffix), nil
	case database.ObjectKindSequence:
		return fmt.Sprintf("DROP SEQUENCE IF EXISTS %s%s;", qualified, suffix), nil
	case database.ObjectKindType, database.ObjectKindEnum:
		return fmt.Sprintf("DROP TYPE IF EXISTS %s%s;", qualified, suffix), nil
	case database.ObjectKindDomain:
		return fmt.Sprintf("DROP DOMAIN IF EXISTS %s%s;", qualified, suffix), nil
	case database.ObjectKindIndex:
		return fmt.Sprintf("DROP INDEX IF EXISTS %s%s;", qualified, suffix), nil
	case database.ObjectKindFunction, database.ObjectKindProcedure:
		routine, err := postgresRoutineName(reference)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(
			"DROP %s IF EXISTS %s%s;",
			strings.ToUpper(string(reference.Kind)),
			routine,
			suffix,
		), nil
	case database.ObjectKindTrigger:
		parent, err := postgresParentTable(reference)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(
			"DROP TRIGGER IF EXISTS %s ON %s%s;",
			quotePostgresIdentifier(reference.Name),
			parent,
			suffix,
		), nil
	case database.ObjectKindConstraint:
		parent, err := postgresParentTable(reference)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(
			"ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s%s;",
			parent,
			quotePostgresIdentifier(reference.Name),
			suffix,
		), nil
	default:
		return "", fmt.Errorf("dropping %s objects is not supported", reference.Kind)
	}
}

func buildPostgresIndexChange(change database.IndexChange) (string, error) {
	method := strings.TrimSpace(change.Method)
	if method == "" {
		method = "btree"
	}
	if !postgresIndexMethodPattern.MatchString(method) {
		return "", fmt.Errorf("invalid PostgreSQL index method %q", method)
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
		columns = append(columns, quotePostgresIdentifier(column))
	}
	if len(columns) == 0 {
		return "", fmt.Errorf("at least one index column is required")
	}
	if err := validatePostgresFragment(change.Where, "partial-index predicate"); err != nil {
		return "", err
	}
	unique := ""
	if change.Unique {
		unique = "UNIQUE "
	}
	statement := fmt.Sprintf(
		"CREATE %sINDEX %s ON %s USING %s (%s)",
		unique,
		quotePostgresIdentifier(change.Name),
		quotePostgresQualifiedIdentifier(change.Table.Schema, change.Table.Name),
		method,
		strings.Join(columns, ", "),
	)
	if strings.TrimSpace(change.Where) != "" {
		statement += " WHERE " + strings.TrimSpace(change.Where)
	}
	return statement + ";", nil
}

func buildPostgresColumnChange(
	change database.ColumnChange,
) ([]string, error) {
	table := quotePostgresQualifiedIdentifier(change.Table.Schema, change.Table.Name)
	columnName := strings.TrimSpace(change.Name)
	activeName := columnName
	statements := make([]string, 0, 5)

	if strings.TrimSpace(change.NewName) != "" &&
		strings.TrimSpace(change.NewName) != columnName {
		statements = append(statements, fmt.Sprintf(
			"ALTER TABLE %s RENAME COLUMN %s TO %s;",
			table,
			quotePostgresIdentifier(columnName),
			quotePostgresIdentifier(change.NewName),
		))
		activeName = strings.TrimSpace(change.NewName)
	}
	quotedColumn := quotePostgresIdentifier(activeName)

	if strings.TrimSpace(change.DataType) != "" {
		if err := validatePostgresFragment(change.DataType, "column data type"); err != nil {
			return nil, err
		}
		if err := validatePostgresFragment(change.Using, "column conversion expression"); err != nil {
			return nil, err
		}
		statement := fmt.Sprintf(
			"ALTER TABLE %s ALTER COLUMN %s TYPE %s",
			table,
			quotedColumn,
			strings.TrimSpace(change.DataType),
		)
		if strings.TrimSpace(change.Using) != "" {
			statement += " USING " + strings.TrimSpace(change.Using)
		}
		statements = append(statements, statement+";")
	}
	if change.Nullable != nil {
		action := "SET NOT NULL"
		if *change.Nullable {
			action = "DROP NOT NULL"
		}
		statements = append(statements, fmt.Sprintf(
			"ALTER TABLE %s ALTER COLUMN %s %s;",
			table,
			quotedColumn,
			action,
		))
	}
	if change.Default != nil {
		if err := validatePostgresFragment(*change.Default, "column default"); err != nil {
			return nil, err
		}
		statements = append(statements, fmt.Sprintf(
			"ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;",
			table,
			quotedColumn,
			strings.TrimSpace(*change.Default),
		))
	} else if change.DropDefault {
		statements = append(statements, fmt.Sprintf(
			"ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;",
			table,
			quotedColumn,
		))
	}
	if len(statements) == 0 {
		return nil, fmt.Errorf("the column change does not contain any effective alterations")
	}
	return statements, nil
}

func buildPostgresConstraintChange(
	action database.ObjectChangeAction,
	change database.ConstraintChange,
	cascade bool,
) (string, bool, error) {
	table := quotePostgresQualifiedIdentifier(change.Table.Schema, change.Table.Name)
	name := quotePostgresIdentifier(change.Name)
	if action == database.ObjectChangeDropConstraint {
		suffix := ""
		if cascade {
			suffix = " CASCADE"
		}
		return fmt.Sprintf(
			"ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s%s;",
			table,
			name,
			suffix,
		), true, nil
	}

	definition := strings.TrimSpace(change.Definition)
	if err := validatePostgresFragment(definition, "constraint definition"); err != nil {
		return "", false, err
	}
	keywords := database.LeadingSQLKeywords(definition, 2)
	if len(keywords) == 0 {
		return "", false, fmt.Errorf("constraint definition is empty")
	}
	switch keywords[0] {
	case "PRIMARY", "UNIQUE", "FOREIGN", "CHECK", "EXCLUDE":
	default:
		return "", false, fmt.Errorf(
			"constraint definition must start with PRIMARY KEY, UNIQUE, FOREIGN KEY, CHECK, or EXCLUDE",
		)
	}
	return fmt.Sprintf(
		"ALTER TABLE %s ADD CONSTRAINT %s %s;",
		table,
		name,
		definition,
	), false, nil
}

func (p *Postgres) BuildObjectChange(
	_ context.Context,
	request database.ObjectChangeRequest,
) (database.ObjectChangePlan, error) {
	if err := request.Validate(); err != nil {
		return database.ObjectChangePlan{}, err
	}

	switch request.Action {
	case database.ObjectChangeCreate, database.ObjectChangeReplace:
		return p.buildPostgresCreateObject(request)

	case database.ObjectChangeRename:
		statement, err := postgresRenameStatement(request.Reference, request.NewName)
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
			Transactional: true,
			Refresh:       []database.ObjectReference{refresh},
		}, nil

	case database.ObjectChangeDrop:
		statement, err := postgresDropStatement(request.Reference, request.Cascade)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		warnings := []string{"Dropping an object is permanent and can invalidate dependent objects."}
		if request.Cascade {
			warnings = append(
				warnings,
				"CASCADE will also remove dependent database objects.",
			)
		}
		return database.ObjectChangePlan{
			Summary: fmt.Sprintf(
				"Drop %s %s",
				request.Reference.Kind,
				request.Reference.QualifiedName(),
			),
			Statements:    []string{statement},
			Destructive:   true,
			Transactional: true,
			Warnings:      warnings,
			Refresh:       []database.ObjectReference{request.Reference},
		}, nil

	case database.ObjectChangeEnable, database.ObjectChangeDisable:
		parent, err := postgresParentTable(request.Reference)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		action := strings.ToUpper(string(request.Action))
		statement := fmt.Sprintf(
			"ALTER TABLE %s %s TRIGGER %s;",
			parent,
			action,
			quotePostgresIdentifier(request.Reference.Name),
		)
		return database.ObjectChangePlan{
			Summary: fmt.Sprintf(
				"%s trigger %s",
				strings.ToUpper(action[:1])+strings.ToLower(action[1:]),
				request.Reference.QualifiedName(),
			),
			Statements:    []string{statement},
			Transactional: true,
			Refresh:       []database.ObjectReference{request.Reference},
		}, nil

	case database.ObjectChangeCreateIndex:
		statement, err := buildPostgresIndexChange(*request.Index)
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
			Transactional: true,
			Refresh: []database.ObjectReference{
				postgresRefreshReference(database.ObjectKindTable, request.Index.Table),
			},
		}, nil

	case database.ObjectChangeAlterColumn:
		statements, err := buildPostgresColumnChange(*request.Column)
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
			Transactional: true,
			Warnings: []string{
				"Changing a column type or nullability can fail when existing rows are incompatible.",
			},
			Refresh: []database.ObjectReference{
				postgresRefreshReference(database.ObjectKindTable, request.Column.Table),
			},
		}, nil

	case database.ObjectChangeAddConstraint, database.ObjectChangeDropConstraint:
		statement, destructive, err := buildPostgresConstraintChange(
			request.Action,
			*request.Constraint,
			request.Cascade,
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
			Transactional: true,
			Warnings: func() []string {
				if destructive {
					return []string{"Dropping a constraint removes its data-integrity protection."}
				}
				return []string{"Existing rows must satisfy the new constraint."}
			}(),
			Refresh: []database.ObjectReference{
				postgresRefreshReference(
					database.ObjectKindTable,
					request.Constraint.Table,
				),
			},
		}, nil
	default:
		return database.ObjectChangePlan{}, fmt.Errorf(
			"unsupported PostgreSQL object change %q",
			request.Action,
		)
	}
}

func (p *Postgres) ApplyObjectChange(
	ctx context.Context,
	plan database.ObjectChangePlan,
) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	transaction, err := p.conn.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin structural change transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = transaction.Rollback()
		}
	}()

	for index, statement := range plan.Statements {
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf(
				"execute structural statement %d of %d: %w",
				index+1,
				len(plan.Statements),
				err,
			)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit structural change: %w", err)
	}
	committed = true
	return nil
}
