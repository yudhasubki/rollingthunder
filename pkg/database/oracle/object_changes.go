package oracle

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"rollingthunder/pkg/database"
)

var oracleIndexMethodPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_ ]*$`)
var oracleCompoundEndPattern = regexp.MustCompile(
	`(?is)\bEND(?:\s+(?:"(?:[^"]|"")*"|[A-Z][A-Z0-9_$#]*))?\s*;\s*$`,
)

func reviewedOracleDDL(
	value string,
	kind database.ObjectKind,
	replace bool,
) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("object definition is empty")
	}
	if !strings.HasPrefix(strings.ToUpper(value), "CREATE") {
		return "", fmt.Errorf("object definition must start with CREATE")
	}
	if database.CountSQLStatements(value) != 1 {
		return "", fmt.Errorf(
			"object definition must contain exactly one SQL or PL/SQL statement",
		)
	}
	keywords := database.LeadingSQLKeywords(value, 20)
	if len(keywords) == 0 || keywords[0] != "CREATE" {
		return "", fmt.Errorf("object definition must start with CREATE")
	}
	expected := strings.ToUpper(string(kind))
	kindPosition := 1
	if len(keywords) > 3 &&
		keywords[1] == "OR" &&
		keywords[2] == "REPLACE" {
		kindPosition = 3
	}
	for kindPosition < len(keywords) {
		switch keywords[kindPosition] {
		case "EDITIONABLE", "NONEDITIONABLE", "FORCE", "NO":
			kindPosition++
		default:
			goto kindResolved
		}
	}
kindResolved:
	found := kindPosition < len(keywords) &&
		keywords[kindPosition] == expected
	if kind == database.ObjectKindMaterializedView {
		found = kindPosition+1 < len(keywords) &&
			keywords[kindPosition] == "MATERIALIZED" &&
			keywords[kindPosition+1] == "VIEW"
	}
	if !found {
		return "", fmt.Errorf(
			"object definition does not create a %s",
			strings.ReplaceAll(string(kind), "_", " "),
		)
	}
	compound := kind == database.ObjectKindFunction ||
		kind == database.ObjectKindProcedure ||
		kind == database.ObjectKindTrigger
	if compound &&
		strings.Contains(strings.ToUpper(value), "BEGIN") &&
		!oracleCompoundEndPattern.MatchString(value) {
		return "", fmt.Errorf(
			"compound Oracle routine or trigger definition must end with END",
		)
	}
	if replace {
		switch kind {
		case database.ObjectKindView,
			database.ObjectKindFunction,
			database.ObjectKindProcedure,
			database.ObjectKindTrigger:
		default:
			return "", fmt.Errorf(
				"Oracle cannot replace %s objects in place",
				strings.ReplaceAll(string(kind), "_", " "),
			)
		}
		upper := strings.ToUpper(value)
		prefixEnd := min(len(upper), 48)
		if !strings.Contains(upper[:prefixEnd], "CREATE OR REPLACE") {
			value = "CREATE OR REPLACE " +
				strings.TrimSpace(value[len("CREATE"):])
		}
	}
	if !strings.HasSuffix(value, ";") {
		value += ";"
	}
	return value, nil
}

func oracleViewStatement(
	reference database.ObjectReference,
	body string,
	replace bool,
) (string, error) {
	body = strings.TrimSpace(body)
	keywords := database.LeadingSQLKeywords(body, 4)
	if len(keywords) == 0 {
		return "", fmt.Errorf("view definition is empty")
	}
	if keywords[0] == "CREATE" {
		return reviewedOracleDDL(body, reference.Kind, replace)
	}
	switch keywords[0] {
	case "SELECT", "WITH":
	default:
		return "", fmt.Errorf("view body must be a SELECT or WITH statement")
	}
	if database.CountSQLStatements(body) != 1 {
		return "", fmt.Errorf("view body must contain exactly one statement")
	}
	body = strings.TrimSuffix(body, ";")
	prefix := "CREATE VIEW"
	if reference.Kind == database.ObjectKindMaterializedView {
		if replace {
			return "", fmt.Errorf(
				"Oracle materialized views must be dropped and recreated",
			)
		}
		prefix = "CREATE MATERIALIZED VIEW"
	} else if replace {
		prefix = "CREATE OR REPLACE VIEW"
	}
	return prefix + " " + quoteQualified(reference.Schema, reference.Name) +
		" AS\n" + body + ";", nil
}

func oracleParentTable(
	reference database.ObjectReference,
) (database.Table, error) {
	schema := reference.ParentSchema
	if schema == "" {
		schema = reference.Schema
	}
	if strings.TrimSpace(schema) == "" ||
		strings.TrimSpace(reference.ParentName) == "" {
		return database.Table{}, fmt.Errorf(
			"parent table is required for %s",
			reference.Kind,
		)
	}
	return database.Table{Schema: schema, Name: reference.ParentName}, nil
}

func oracleDropStatement(
	reference database.ObjectReference,
	cascade bool,
) (string, error) {
	qualified := quoteQualified(reference.Schema, reference.Name)
	switch reference.Kind {
	case database.ObjectKindTable:
		statement := "DROP TABLE " + qualified
		if cascade {
			statement += " CASCADE CONSTRAINTS"
		}
		return statement + ";", nil
	case database.ObjectKindView:
		return "DROP VIEW " + qualified + ";", nil
	case database.ObjectKindMaterializedView:
		return "DROP MATERIALIZED VIEW " + qualified + ";", nil
	case database.ObjectKindFunction:
		return "DROP FUNCTION " + qualified + ";", nil
	case database.ObjectKindProcedure:
		return "DROP PROCEDURE " + qualified + ";", nil
	case database.ObjectKindTrigger:
		return "DROP TRIGGER " + qualified + ";", nil
	case database.ObjectKindSequence:
		return "DROP SEQUENCE " + qualified + ";", nil
	case database.ObjectKindType:
		return "DROP TYPE " + qualified + ";", nil
	case database.ObjectKindIndex:
		return "DROP INDEX " + qualified + ";", nil
	case database.ObjectKindConstraint:
		parent, err := oracleParentTable(reference)
		if err != nil {
			return "", err
		}
		statement := "ALTER TABLE " +
			quoteQualified(parent.Schema, parent.Name) +
			" DROP CONSTRAINT " + quoteIdentifier(reference.Name)
		if cascade {
			statement += " CASCADE"
		}
		return statement + ";", nil
	default:
		return "", fmt.Errorf(
			"dropping Oracle %s objects is not supported",
			reference.Kind,
		)
	}
}

func oracleRenameStatement(
	reference database.ObjectReference,
	newName string,
) (string, error) {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return "", fmt.Errorf("new object name is required")
	}
	qualified := quoteQualified(reference.Schema, reference.Name)
	switch reference.Kind {
	case database.ObjectKindTable:
		return "ALTER TABLE " + qualified + " RENAME TO " +
			quoteIdentifier(newName) + ";", nil
	case database.ObjectKindView, database.ObjectKindSequence:
		return "RENAME " + quoteIdentifier(reference.Name) + " TO " +
			quoteIdentifier(newName) + ";", nil
	case database.ObjectKindMaterializedView:
		return "ALTER MATERIALIZED VIEW " + qualified + " RENAME TO " +
			quoteIdentifier(newName) + ";", nil
	case database.ObjectKindTrigger:
		return "ALTER TRIGGER " + qualified + " RENAME TO " +
			quoteIdentifier(newName) + ";", nil
	case database.ObjectKindIndex:
		return "ALTER INDEX " + qualified + " RENAME TO " +
			quoteIdentifier(newName) + ";", nil
	case database.ObjectKindConstraint:
		parent, err := oracleParentTable(reference)
		if err != nil {
			return "", err
		}
		return "ALTER TABLE " +
			quoteQualified(parent.Schema, parent.Name) +
			" RENAME CONSTRAINT " + quoteIdentifier(reference.Name) +
			" TO " + quoteIdentifier(newName) + ";", nil
	default:
		return "", fmt.Errorf(
			"Oracle cannot rename %s objects in place; recreate the object with the new name",
			reference.Kind,
		)
	}
}

func buildOracleIndexChange(change database.IndexChange) (string, error) {
	if strings.TrimSpace(change.Table.Name) == "" ||
		strings.TrimSpace(change.Name) == "" ||
		len(change.Columns) == 0 {
		return "", fmt.Errorf("index table, name, and columns are required")
	}
	if strings.TrimSpace(change.Where) != "" {
		return "", fmt.Errorf(
			"Oracle does not support a PostgreSQL-style partial-index predicate",
		)
	}
	method := strings.ToUpper(strings.TrimSpace(change.Method))
	if method == "" || method == "BTREE" || method == "NORMAL" {
		method = ""
	}
	if method != "" && !oracleIndexMethodPattern.MatchString(method) {
		return "", fmt.Errorf("invalid Oracle index type %q", change.Method)
	}
	switch method {
	case "", "BITMAP":
	default:
		return "", fmt.Errorf(
			"Oracle structural editor supports B-tree or bitmap indexes",
		)
	}
	if change.Unique && method == "BITMAP" {
		return "", fmt.Errorf("Oracle bitmap indexes cannot be unique")
	}
	columns := make([]string, 0, len(change.Columns))
	seen := make(map[string]struct{}, len(change.Columns))
	for _, column := range change.Columns {
		column = strings.TrimSpace(column)
		if column == "" {
			return "", fmt.Errorf("index column cannot be empty")
		}
		key := strings.ToLower(column)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		columns = append(columns, quoteIdentifier(column))
	}
	prefix := "CREATE "
	if change.Unique {
		prefix += "UNIQUE "
	}
	if method == "BITMAP" {
		prefix += "BITMAP "
	}
	return prefix + "INDEX " + quoteQualified(
		change.Table.Schema,
		change.Name,
	) + " ON " + quoteQualified(
		change.Table.Schema,
		change.Table.Name,
	) + " (" + strings.Join(columns, ", ") + ");", nil
}

func buildOracleAddColumn(
	change database.AddColumnChange,
) (string, error) {
	column := change.Column
	if change.First || strings.TrimSpace(change.After) != "" {
		return "", fmt.Errorf(
			"Oracle does not support positioning a new column",
		)
	}
	if err := database.ValidateDDLFragment(column.Type, "column data type"); err != nil {
		return "", err
	}
	if err := database.ValidateDDLFragment(column.Default, "column default"); err != nil {
		return "", err
	}
	statement := "ALTER TABLE " +
		quoteQualified(change.Table.Schema, change.Table.Name) +
		" ADD " + quoteIdentifier(column.Name) + " " +
		strings.TrimSpace(column.Type)
	if strings.TrimSpace(column.Default) != "" {
		statement += " DEFAULT " + strings.TrimSpace(column.Default)
	}
	if !column.Nullable {
		statement += " NOT NULL"
	}
	if column.Unique {
		statement += " UNIQUE"
	}
	if column.PrimaryKey {
		statement += " PRIMARY KEY"
	}
	return statement + ";", nil
}

func buildOracleColumnChange(
	change database.ColumnChange,
) ([]string, error) {
	if strings.TrimSpace(change.Using) != "" {
		return nil, fmt.Errorf(
			"Oracle does not support a PostgreSQL USING clause",
		)
	}
	tableName := quoteQualified(change.Table.Schema, change.Table.Name)
	activeName := change.Name
	statements := make([]string, 0, 4)
	if newName := strings.TrimSpace(change.NewName); newName != "" &&
		!strings.EqualFold(newName, activeName) {
		statements = append(
			statements,
			"ALTER TABLE "+tableName+" RENAME COLUMN "+
				quoteIdentifier(activeName)+" TO "+
				quoteIdentifier(newName)+";",
		)
		activeName = newName
	}
	if dataType := strings.TrimSpace(change.DataType); dataType != "" {
		if err := database.ValidateDDLFragment(dataType, "column data type"); err != nil {
			return nil, err
		}
		statements = append(
			statements,
			"ALTER TABLE "+tableName+" MODIFY ("+
				quoteIdentifier(activeName)+" "+dataType+");",
		)
	}
	if change.Nullable != nil {
		nullability := " NOT NULL"
		if *change.Nullable {
			nullability = " NULL"
		}
		statements = append(
			statements,
			"ALTER TABLE "+tableName+" MODIFY ("+
				quoteIdentifier(activeName)+nullability+");",
		)
	}
	if change.Default != nil {
		if err := database.ValidateDDLFragment(
			*change.Default,
			"column default",
		); err != nil {
			return nil, err
		}
		statements = append(
			statements,
			"ALTER TABLE "+tableName+" MODIFY ("+
				quoteIdentifier(activeName)+" DEFAULT "+
				strings.TrimSpace(*change.Default)+");",
		)
	} else if change.DropDefault {
		statements = append(
			statements,
			"ALTER TABLE "+tableName+" MODIFY ("+
				quoteIdentifier(activeName)+" DEFAULT NULL);",
		)
	}
	if len(statements) == 0 {
		return nil, fmt.Errorf("column change has no effective alterations")
	}
	return statements, nil
}

func validateOracleConstraintDefinition(value string) error {
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

func oracleRefreshReference(
	kind database.ObjectKind,
	table database.Table,
) database.ObjectReference {
	return database.ObjectReference{
		Kind:   kind,
		Schema: table.Schema,
		Name:   table.Name,
	}
}

func oracleDDLWarnings(values ...string) []string {
	return append(
		[]string{
			"Oracle auto-commits DDL; this structural change cannot be rolled back as one transaction.",
		},
		values...,
	)
}

func (o *Oracle) BuildObjectChange(
	ctx context.Context,
	request database.ObjectChangeRequest,
) (database.ObjectChangePlan, error) {
	if err := request.Validate(); err != nil {
		return database.ObjectChangePlan{}, err
	}
	request.Reference.Schema = o.defaultSchema(request.Reference.Schema)
	switch request.Action {
	case database.ObjectChangeCreate, database.ObjectChangeReplace:
		replace := request.Action == database.ObjectChangeReplace
		var (
			statement string
			err       error
		)
		switch request.Reference.Kind {
		case database.ObjectKindView,
			database.ObjectKindMaterializedView:
			statement, err = oracleViewStatement(
				request.Reference,
				request.Definition,
				replace,
			)
		case database.ObjectKindFunction,
			database.ObjectKindProcedure,
			database.ObjectKindTrigger:
			statement, err = reviewedOracleDDL(
				request.Definition,
				request.Reference.Kind,
				replace,
			)
		default:
			err = fmt.Errorf(
				"creating Oracle %s objects is not supported by the structural editor",
				request.Reference.Kind,
			)
		}
		if err != nil {
			return database.ObjectChangePlan{}, err
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
			Statements:    []string{statement},
			Transactional: false,
			Warnings:      oracleDDLWarnings(),
			Refresh:       []database.ObjectReference{request.Reference},
		}, nil
	case database.ObjectChangeRename:
		if (request.Reference.Kind == database.ObjectKindView ||
			request.Reference.Kind == database.ObjectKindSequence) &&
			!strings.EqualFold(
				request.Reference.Schema,
				o.defaultSchema(""),
			) {
			return database.ObjectChangePlan{}, fmt.Errorf(
				"Oracle can only rename %s objects in the active schema",
				request.Reference.Kind,
			)
		}
		statement, err := oracleRenameStatement(
			request.Reference,
			request.NewName,
		)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		refresh := request.Reference
		refresh.ID = ""
		refresh.Name = strings.TrimSpace(request.NewName)
		return database.ObjectChangePlan{
			Summary: "Rename " + string(request.Reference.Kind) + " " +
				request.Reference.QualifiedName(),
			Statements:    []string{statement},
			Transactional: false,
			Warnings:      oracleDDLWarnings(),
			Refresh:       []database.ObjectReference{refresh},
		}, nil
	case database.ObjectChangeDrop:
		statement, err := oracleDropStatement(
			request.Reference,
			request.Cascade,
		)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		return database.ObjectChangePlan{
			Summary:       "Drop " + string(request.Reference.Kind) + " " + request.Reference.QualifiedName(),
			Statements:    []string{statement},
			Destructive:   true,
			Transactional: false,
			Warnings: oracleDDLWarnings(
				"Dropping an object can remove data or break dependencies.",
			),
			Refresh: []database.ObjectReference{request.Reference},
		}, nil
	case database.ObjectChangeEnable, database.ObjectChangeDisable:
		verb := "ENABLE"
		if request.Action == database.ObjectChangeDisable {
			verb = "DISABLE"
		}
		return database.ObjectChangePlan{
			Summary: strings.ToLower(verb) + " trigger " +
				request.Reference.Name,
			Statements: []string{
				"ALTER TRIGGER " +
					quoteQualified(
						request.Reference.Schema,
						request.Reference.Name,
					) +
					" " + verb + ";",
			},
			Transactional: false,
			Warnings:      oracleDDLWarnings(),
			Refresh:       []database.ObjectReference{request.Reference},
		}, nil
	case database.ObjectChangeCreateIndex:
		request.Index.Table.Schema = o.defaultSchema(
			request.Index.Table.Schema,
		)
		statement, err := buildOracleIndexChange(*request.Index)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		return database.ObjectChangePlan{
			Summary:       "Create index " + request.Index.Name,
			Statements:    []string{statement},
			Transactional: false,
			Warnings: oracleDDLWarnings(
				"Creating an index can consume significant locks, CPU, and storage.",
			),
			Refresh: []database.ObjectReference{
				oracleRefreshReference(
					database.ObjectKindTable,
					request.Index.Table,
				),
			},
		}, nil
	case database.ObjectChangeAddColumn:
		request.AddColumn.Table.Schema = o.defaultSchema(
			request.AddColumn.Table.Schema,
		)
		statement, err := buildOracleAddColumn(*request.AddColumn)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		return database.ObjectChangePlan{
			Summary:       "Add column " + request.AddColumn.Column.Name,
			Statements:    []string{statement},
			Transactional: false,
			Warnings: oracleDDLWarnings(
				"Adding a populated NOT NULL column can lock and rewrite the table.",
			),
			Refresh: []database.ObjectReference{
				oracleRefreshReference(
					database.ObjectKindTable,
					request.AddColumn.Table,
				),
			},
		}, nil
	case database.ObjectChangeAlterColumn:
		request.Column.Table.Schema = o.defaultSchema(
			request.Column.Table.Schema,
		)
		statements, err := buildOracleColumnChange(*request.Column)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		return database.ObjectChangePlan{
			Summary:       "Alter column " + request.Column.Name,
			Statements:    statements,
			Transactional: false,
			Warnings: oracleDDLWarnings(
				"Changing a column type or nullability can scan, lock, or rewrite the table.",
			),
			Refresh: []database.ObjectReference{
				oracleRefreshReference(
					database.ObjectKindTable,
					request.Column.Table,
				),
			},
		}, nil
	case database.ObjectChangeDropColumn:
		request.DropColumn.Table.Schema = o.defaultSchema(
			request.DropColumn.Table.Schema,
		)
		statement := "ALTER TABLE " +
			quoteQualified(
				request.DropColumn.Table.Schema,
				request.DropColumn.Table.Name,
			) +
			" DROP COLUMN " + quoteIdentifier(request.DropColumn.Name)
		if request.Cascade {
			statement += " CASCADE CONSTRAINTS"
		}
		return database.ObjectChangePlan{
			Summary:       "Drop column " + request.DropColumn.Name,
			Statements:    []string{statement + ";"},
			Destructive:   true,
			Transactional: false,
			Warnings: oracleDDLWarnings(
				"Dropping a column permanently removes its data.",
			),
			Refresh: []database.ObjectReference{
				oracleRefreshReference(
					database.ObjectKindTable,
					request.DropColumn.Table,
				),
			},
		}, nil
	case database.ObjectChangeAddConstraint,
		database.ObjectChangeDropConstraint:
		request.Constraint.Table.Schema = o.defaultSchema(
			request.Constraint.Table.Schema,
		)
		tableName := quoteQualified(
			request.Constraint.Table.Schema,
			request.Constraint.Table.Name,
		)
		destructive := request.Action ==
			database.ObjectChangeDropConstraint
		statement := "ALTER TABLE " + tableName
		if destructive {
			statement += " DROP CONSTRAINT " +
				quoteIdentifier(request.Constraint.Name)
			if request.Cascade {
				statement += " CASCADE"
			}
		} else {
			if err := validateOracleConstraintDefinition(
				request.Constraint.Definition,
			); err != nil {
				return database.ObjectChangePlan{}, err
			}
			statement += " ADD CONSTRAINT " +
				quoteIdentifier(request.Constraint.Name) + " " +
				strings.TrimSpace(request.Constraint.Definition)
		}
		verb := "Add"
		if destructive {
			verb = "Drop"
		}
		return database.ObjectChangePlan{
			Summary:       verb + " constraint " + request.Constraint.Name,
			Statements:    []string{statement + ";"},
			Destructive:   destructive,
			Transactional: false,
			Warnings: oracleDDLWarnings(
				"Constraint validation can scan and lock the table.",
			),
			Refresh: []database.ObjectReference{
				oracleRefreshReference(
					database.ObjectKindTable,
					request.Constraint.Table,
				),
			},
		}, nil
	default:
		return database.ObjectChangePlan{}, fmt.Errorf(
			"unsupported Oracle object change %q",
			request.Action,
		)
	}
}

func executableOracleDDL(statement string) string {
	statement = strings.TrimSpace(statement)
	upper := strings.ToUpper(statement)
	plsql := strings.HasPrefix(upper, "BEGIN") ||
		strings.HasPrefix(upper, "DECLARE") ||
		(strings.HasPrefix(upper, "CREATE") &&
			(strings.Contains(upper[:min(len(upper), 96)], " FUNCTION ") ||
				strings.Contains(upper[:min(len(upper), 96)], " PROCEDURE ") ||
				strings.Contains(upper[:min(len(upper), 96)], " TRIGGER ") ||
				strings.Contains(upper[:min(len(upper), 96)], " PACKAGE ")))
	if !plsql {
		statement = strings.TrimSuffix(statement, ";")
	}
	return statement
}

func (o *Oracle) ApplyObjectChange(
	ctx context.Context,
	plan database.ObjectChangePlan,
) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	if plan.Transactional {
		return fmt.Errorf("Oracle DDL plans must disclose auto-commit behavior")
	}
	if err := o.ensureConnected(); err != nil {
		return err
	}
	for index, statement := range plan.Statements {
		if _, err := o.conn.ExecContext(
			ctx,
			executableOracleDDL(statement),
		); err != nil {
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

var _ database.ObjectChangeDriver = (*Oracle)(nil)
