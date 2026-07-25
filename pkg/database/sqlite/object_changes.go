package sqlite

import (
	"context"
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

func reviewedSQLiteDDL(
	value string,
	kind database.ObjectKind,
) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("object definition is empty")
	}
	if strings.ContainsRune(value, '\x00') {
		return "", fmt.Errorf("object definition contains a NUL byte")
	}
	if kind != database.ObjectKindTrigger &&
		database.CountSQLStatements(value) != 1 {
		return "", fmt.Errorf("object definition must contain exactly one SQL statement")
	}
	keywords := database.LeadingSQLKeywords(value, 12)
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
	if kind == database.ObjectKindTrigger {
		upper := strings.ToUpper(strings.TrimSuffix(value, ";"))
		if !strings.HasSuffix(strings.TrimSpace(upper), "END") {
			return "", fmt.Errorf(
				"SQLite trigger definition must end with END",
			)
		}
	}
	if !strings.HasSuffix(value, ";") {
		value += ";"
	}
	return value, nil
}

func sqliteViewStatement(
	reference database.ObjectReference,
	body string,
) (string, error) {
	body = strings.TrimSpace(body)
	keywords := database.LeadingSQLKeywords(body, 2)
	if len(keywords) == 0 {
		return "", fmt.Errorf("view definition is empty")
	}
	if keywords[0] == "CREATE" {
		return reviewedSQLiteDDL(body, database.ObjectKindView)
	}
	if database.CountSQLStatements(body) != 1 {
		return "", fmt.Errorf("view body must contain exactly one statement")
	}
	switch keywords[0] {
	case "SELECT", "WITH", "VALUES":
	default:
		return "", fmt.Errorf("view body must be a SELECT, WITH, or VALUES statement")
	}
	return fmt.Sprintf(
		"CREATE VIEW %s AS\n%s;",
		quoteSQLiteQualifiedIdentifier(reference.Schema, reference.Name),
		strings.TrimSuffix(body, ";"),
	), nil
}

func sqliteDropStatement(
	reference database.ObjectReference,
) (string, error) {
	qualified := quoteSQLiteQualifiedIdentifier(
		normalizeSQLiteSchema(reference.Schema),
		reference.Name,
	)
	switch reference.Kind {
	case database.ObjectKindTable:
		return "DROP TABLE IF EXISTS " + qualified + ";", nil
	case database.ObjectKindView:
		return "DROP VIEW IF EXISTS " + qualified + ";", nil
	case database.ObjectKindTrigger:
		return "DROP TRIGGER IF EXISTS " + qualified + ";", nil
	case database.ObjectKindIndex:
		return "DROP INDEX IF EXISTS " + qualified + ";", nil
	default:
		return "", fmt.Errorf(
			"dropping SQLite %s objects is not supported without rebuilding the table",
			reference.Kind,
		)
	}
}

func buildSQLiteIndexChange(
	change database.IndexChange,
) (string, error) {
	if strings.TrimSpace(change.Table.Name) == "" ||
		strings.TrimSpace(change.Name) == "" {
		return "", fmt.Errorf("index table and name are required")
	}
	method := strings.ToUpper(strings.TrimSpace(change.Method))
	if method != "" && method != "BTREE" && method != "B-TREE" {
		return "", fmt.Errorf("SQLite indexes use the built-in B-tree implementation")
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
		columns = append(columns, quoteSQLiteIdentifier(column))
	}
	if len(columns) == 0 {
		return "", fmt.Errorf("at least one index column is required")
	}
	if err := validateSQLiteFragment(change.Where, "partial-index predicate"); err != nil {
		return "", err
	}
	unique := ""
	if change.Unique {
		unique = "UNIQUE "
	}
	statement := fmt.Sprintf(
		"CREATE %sINDEX %s ON %s (%s)",
		unique,
		quoteSQLiteQualifiedIdentifier(
			normalizeSQLiteSchema(change.Table.Schema),
			change.Name,
		),
		quoteSQLiteIdentifier(change.Table.Name),
		strings.Join(columns, ", "),
	)
	if strings.TrimSpace(change.Where) != "" {
		statement += " WHERE " + strings.TrimSpace(change.Where)
	}
	return statement + ";", nil
}

func buildSQLiteAddColumn(
	change database.AddColumnChange,
) (string, error) {
	if change.First || strings.TrimSpace(change.After) != "" {
		return "", fmt.Errorf(
			"SQLite appends new columns; changing physical column order requires rebuilding the table",
		)
	}
	column := change.Column
	if column.PrimaryKey || column.Unique {
		return "", fmt.Errorf(
			"SQLite cannot add a PRIMARY KEY or UNIQUE column in place; add the column first and create a reviewed unique index, or rebuild the table",
		)
	}
	if !column.Nullable && strings.TrimSpace(column.Default) == "" {
		return "", fmt.Errorf(
			"SQLite requires a non-NULL default when adding a required column to an existing table",
		)
	}
	if err := validateSQLiteFragment(column.Type, "column data type"); err != nil {
		return "", err
	}
	if err := validateSQLiteFragment(column.Default, "column default"); err != nil {
		return "", err
	}
	statement := fmt.Sprintf(
		"ALTER TABLE %s ADD COLUMN %s %s",
		quoteSQLiteQualifiedIdentifier(
			normalizeSQLiteSchema(change.Table.Schema),
			change.Table.Name,
		),
		quoteSQLiteIdentifier(strings.TrimSpace(column.Name)),
		strings.TrimSpace(column.Type),
	)
	if strings.TrimSpace(column.Default) != "" {
		statement += " DEFAULT " + strings.TrimSpace(column.Default)
	}
	if !column.Nullable {
		statement += " NOT NULL"
	}
	return statement + ";", nil
}

func buildSQLiteDropColumn(change database.DropColumnChange) string {
	return fmt.Sprintf(
		"ALTER TABLE %s DROP COLUMN %s;",
		quoteSQLiteQualifiedIdentifier(
			normalizeSQLiteSchema(change.Table.Schema),
			change.Table.Name,
		),
		quoteSQLiteIdentifier(strings.TrimSpace(change.Name)),
	)
}

func sqliteRefreshReference(
	kind database.ObjectKind,
	table database.Table,
) database.ObjectReference {
	return database.ObjectReference{
		Kind:   kind,
		Schema: normalizeSQLiteSchema(table.Schema),
		Name:   table.Name,
	}
}

func (s *SQLite) BuildObjectChange(
	_ context.Context,
	request database.ObjectChangeRequest,
) (database.ObjectChangePlan, error) {
	if err := request.Validate(); err != nil {
		return database.ObjectChangePlan{}, err
	}
	if request.Cascade {
		return database.ObjectChangePlan{}, fmt.Errorf(
			"SQLite does not support CASCADE for this reviewed object change",
		)
	}
	request.Reference.Schema = normalizeSQLiteSchema(request.Reference.Schema)

	switch request.Action {
	case database.ObjectChangeCreate, database.ObjectChangeReplace:
		replace := request.Action == database.ObjectChangeReplace
		var statement string
		var err error
		switch request.Reference.Kind {
		case database.ObjectKindView:
			statement, err = sqliteViewStatement(
				request.Reference,
				request.Definition,
			)
		case database.ObjectKindTrigger:
			statement, err = reviewedSQLiteDDL(
				request.Definition,
				database.ObjectKindTrigger,
			)
		default:
			return database.ObjectChangePlan{}, fmt.Errorf(
				"creating SQLite %s objects is not supported by the structural editor",
				request.Reference.Kind,
			)
		}
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		statements := []string{statement}
		warnings := make([]string, 0)
		if replace {
			drop, err := sqliteDropStatement(request.Reference)
			if err != nil {
				return database.ObjectChangePlan{}, err
			}
			statements = []string{drop, statement}
			warnings = append(
				warnings,
				"SQLite replaces this object with DROP followed by CREATE inside one transaction.",
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
			Transactional: true,
			Warnings:      warnings,
			Refresh:       []database.ObjectReference{request.Reference},
		}, nil

	case database.ObjectChangeRename:
		if request.Reference.Kind != database.ObjectKindTable {
			return database.ObjectChangePlan{}, fmt.Errorf(
				"SQLite can only rename tables in place; recreate this %s with a new name",
				request.Reference.Kind,
			)
		}
		newName := strings.TrimSpace(request.NewName)
		statement := fmt.Sprintf(
			"ALTER TABLE %s RENAME TO %s;",
			quoteSQLiteQualifiedIdentifier(
				request.Reference.Schema,
				request.Reference.Name,
			),
			quoteSQLiteIdentifier(newName),
		)
		refresh := request.Reference
		refresh.ID = ""
		refresh.Name = newName
		return database.ObjectChangePlan{
			Summary: fmt.Sprintf(
				"Rename table %s to %s",
				request.Reference.QualifiedName(),
				newName,
			),
			Statements:    []string{statement},
			Transactional: true,
			Refresh:       []database.ObjectReference{refresh},
		}, nil

	case database.ObjectChangeDrop:
		statement, err := sqliteDropStatement(request.Reference)
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
			Transactional: true,
			Warnings: []string{
				"Dropping an object is permanent after the transaction commits.",
			},
			Refresh: []database.ObjectReference{request.Reference},
		}, nil

	case database.ObjectChangeEnable, database.ObjectChangeDisable:
		return database.ObjectChangePlan{}, fmt.Errorf(
			"SQLite cannot disable a trigger without dropping it",
		)

	case database.ObjectChangeCreateIndex:
		request.Index.Table.Schema = normalizeSQLiteSchema(
			request.Index.Table.Schema,
		)
		statement, err := buildSQLiteIndexChange(*request.Index)
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
				sqliteRefreshReference(
					database.ObjectKindTable,
					request.Index.Table,
				),
			},
		}, nil

	case database.ObjectChangeAddColumn:
		request.AddColumn.Table.Schema = normalizeSQLiteSchema(
			request.AddColumn.Table.Schema,
		)
		statement, err := buildSQLiteAddColumn(*request.AddColumn)
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
			Transactional: true,
			Warnings: []string{
				"SQLite appends the column at the end of the table.",
				"Existing rows receive the reviewed default value.",
			},
			Refresh: []database.ObjectReference{
				sqliteRefreshReference(
					database.ObjectKindTable,
					request.AddColumn.Table,
				),
			},
		}, nil

	case database.ObjectChangeAlterColumn:
		if strings.TrimSpace(request.Column.NewName) == "" ||
			strings.TrimSpace(request.Column.DataType) != "" ||
			request.Column.Nullable != nil ||
			request.Column.Default != nil ||
			request.Column.DropDefault ||
			strings.TrimSpace(request.Column.Using) != "" {
			return database.ObjectChangePlan{}, fmt.Errorf(
				"SQLite only supports direct column rename; changing type, nullability, default, or constraints requires an explicit reviewed table rebuild",
			)
		}
		request.Column.Table.Schema = normalizeSQLiteSchema(
			request.Column.Table.Schema,
		)
		statement := fmt.Sprintf(
			"ALTER TABLE %s RENAME COLUMN %s TO %s;",
			quoteSQLiteQualifiedIdentifier(
				request.Column.Table.Schema,
				request.Column.Table.Name,
			),
			quoteSQLiteIdentifier(request.Column.Name),
			quoteSQLiteIdentifier(request.Column.NewName),
		)
		return database.ObjectChangePlan{
			Summary: fmt.Sprintf(
				"Rename column %s on %s",
				request.Column.Name,
				request.Column.Table.Name,
			),
			Statements:    []string{statement},
			Transactional: true,
			Refresh: []database.ObjectReference{
				sqliteRefreshReference(
					database.ObjectKindTable,
					request.Column.Table,
				),
			},
		}, nil

	case database.ObjectChangeDropColumn:
		request.DropColumn.Table.Schema = normalizeSQLiteSchema(
			request.DropColumn.Table.Schema,
		)
		return database.ObjectChangePlan{
			Summary: fmt.Sprintf(
				"Drop column %s from %s",
				request.DropColumn.Name,
				request.DropColumn.Table.Name,
			),
			Statements: []string{
				buildSQLiteDropColumn(*request.DropColumn),
			},
			Destructive:   true,
			Transactional: true,
			Warnings: []string{
				"Dropping a column permanently removes its data.",
				"SQLite refuses the change when an index, trigger, generated column, or constraint still depends on the column.",
			},
			Refresh: []database.ObjectReference{
				sqliteRefreshReference(
					database.ObjectKindTable,
					request.DropColumn.Table,
				),
			},
		}, nil

	case database.ObjectChangeAddConstraint,
		database.ObjectChangeDropConstraint:
		return database.ObjectChangePlan{}, fmt.Errorf(
			"SQLite constraint changes require an explicit reviewed table rebuild and are not exposed by this driver",
		)
	default:
		return database.ObjectChangePlan{}, fmt.Errorf(
			"unsupported SQLite object change %q",
			request.Action,
		)
	}
}

func (s *SQLite) ApplyObjectChange(
	ctx context.Context,
	plan database.ObjectChangePlan,
) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	if !plan.Transactional {
		return fmt.Errorf("SQLite structural plans must be transactional")
	}
	transaction, err := s.conn.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin SQLite structural transaction: %w", err)
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
		return fmt.Errorf("commit SQLite structural change: %w", err)
	}
	committed = true
	return nil
}

var _ database.ObjectChangeDriver = (*SQLite)(nil)
