package sqlserver

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"rollingthunder/pkg/database"
)

var indexMethodPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_ ]*$`)

func sqlString(value string) string {
	return "N'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func reviewedDDL(
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
		return "", fmt.Errorf("object definition must contain exactly one SQL batch")
	}
	keywords := database.LeadingSQLKeywords(value, 16)
	if len(keywords) == 0 || keywords[0] != "CREATE" {
		return "", fmt.Errorf("object definition must start with CREATE")
	}
	expected := strings.ToUpper(string(kind))
	kindPosition := 1
	if len(keywords) > 3 &&
		keywords[1] == "OR" &&
		keywords[2] == "ALTER" {
		kindPosition = 3
	}
	if len(keywords) <= kindPosition ||
		keywords[kindPosition] != expected {
		return "", fmt.Errorf("object definition does not create a %s", kind)
	}
	if replace {
		upper := strings.ToUpper(value)
		prefixEnd := min(len(upper), 48)
		if !strings.Contains(upper[:prefixEnd], "CREATE OR ALTER") {
			value = "CREATE OR ALTER " + strings.TrimSpace(value[len("CREATE"):])
		}
	}
	if !strings.HasSuffix(value, ";") {
		value += ";"
	}
	return value, nil
}

func viewStatement(
	reference database.ObjectReference,
	body string,
	replace bool,
) (string, error) {
	body = strings.TrimSpace(body)
	keywords := database.LeadingSQLKeywords(body, 3)
	if len(keywords) == 0 {
		return "", fmt.Errorf("view definition is empty")
	}
	if keywords[0] == "CREATE" {
		return reviewedDDL(body, database.ObjectKindView, replace)
	}
	switch keywords[0] {
	case "SELECT", "WITH":
	default:
		return "", fmt.Errorf("view body must be a SELECT or WITH statement")
	}
	if database.CountSQLStatements(body) != 1 {
		return "", fmt.Errorf("view body must contain exactly one statement")
	}
	prefix := "CREATE VIEW"
	if replace {
		prefix = "CREATE OR ALTER VIEW"
	}
	return prefix + " " + quoteQualified(reference.Schema, reference.Name) +
		" AS\n" + strings.TrimSuffix(body, ";") + ";", nil
}

func parentTable(reference database.ObjectReference) (string, error) {
	schema := reference.ParentSchema
	if schema == "" {
		schema = reference.Schema
	}
	if strings.TrimSpace(schema) == "" ||
		strings.TrimSpace(reference.ParentName) == "" {
		return "", fmt.Errorf("parent table is required for %s", reference.Kind)
	}
	return quoteQualified(schema, reference.ParentName), nil
}

func dropStatement(reference database.ObjectReference) (string, error) {
	qualified := quoteQualified(reference.Schema, reference.Name)
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
	case database.ObjectKindSequence:
		return "DROP SEQUENCE IF EXISTS " + qualified + ";", nil
	case database.ObjectKindIndex:
		parent, err := parentTable(reference)
		if err != nil {
			return "", err
		}
		return "DROP INDEX " + quoteIdentifier(reference.Name) +
			" ON " + parent + ";", nil
	case database.ObjectKindConstraint:
		parent, err := parentTable(reference)
		if err != nil {
			return "", err
		}
		return "ALTER TABLE " + parent + " DROP CONSTRAINT " +
			quoteIdentifier(reference.Name) + ";", nil
	default:
		return "", fmt.Errorf("dropping SQL Server %s objects is not supported", reference.Kind)
	}
}

func renameStatement(
	reference database.ObjectReference,
	newName string,
) (string, error) {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return "", fmt.Errorf("new object name is required")
	}
	objectName := quoteQualified(reference.Schema, reference.Name)
	objectType := "OBJECT"
	if reference.Kind == database.ObjectKindIndex {
		if reference.ParentName == "" {
			return "", fmt.Errorf("parent table is required to rename an index")
		}
		schema := reference.ParentSchema
		if schema == "" {
			schema = reference.Schema
		}
		objectName = quoteQualified(schema, reference.ParentName) +
			"." + quoteIdentifier(reference.Name)
		objectType = "INDEX"
	}
	return "EXEC sys.sp_rename " + sqlString(objectName) + ", " +
		sqlString(newName) + ", " + sqlString(objectType) + ";", nil
}

func buildIndexChange(change database.IndexChange) (string, error) {
	if strings.TrimSpace(change.Table.Name) == "" ||
		strings.TrimSpace(change.Name) == "" ||
		len(change.Columns) == 0 {
		return "", fmt.Errorf("index table, name, and columns are required")
	}
	method := strings.ToUpper(strings.TrimSpace(change.Method))
	if method == "" || method == "BTREE" {
		method = "NONCLUSTERED"
	}
	if !indexMethodPattern.MatchString(method) {
		return "", fmt.Errorf("invalid SQL Server index type %q", change.Method)
	}
	switch method {
	case "CLUSTERED", "NONCLUSTERED":
	default:
		return "", fmt.Errorf(
			"SQL Server structural editor supports clustered or nonclustered indexes",
		)
	}
	if strings.TrimSpace(change.Where) != "" &&
		method != "NONCLUSTERED" {
		return "", fmt.Errorf(
			"SQL Server filtered indexes must be nonclustered",
		)
	}
	columns := make([]string, 0, len(change.Columns))
	seen := make(map[string]struct{}, len(change.Columns))
	for _, column := range change.Columns {
		column = strings.TrimSpace(column)
		if column == "" {
			return "", fmt.Errorf("index column cannot be empty")
		}
		key := strings.ToLower(column)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		columns = append(columns, quoteIdentifier(column))
	}
	unique := ""
	if change.Unique {
		unique = "UNIQUE "
	}
	statement := fmt.Sprintf(
		"CREATE %s%s INDEX %s ON %s (%s)",
		unique,
		method,
		quoteIdentifier(change.Name),
		quoteQualified(change.Table.Schema, change.Table.Name),
		strings.Join(columns, ", "),
	)
	if strings.TrimSpace(change.Where) != "" {
		if err := database.ValidateDDLFragment(
			change.Where,
			"filtered-index predicate",
		); err != nil {
			return "", err
		}
		statement += " WHERE " + strings.TrimSpace(change.Where)
	}
	return statement + ";", nil
}

func buildAddColumn(change database.AddColumnChange) (string, error) {
	column := change.Column
	if change.First || strings.TrimSpace(change.After) != "" {
		return "", fmt.Errorf("SQL Server does not support positioning a new column")
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
	return statement + ";", nil
}

func dropDefaultConstraintBatch(table database.Table, column string) string {
	tableName := quoteQualified(table.Schema, table.Name)
	return fmt.Sprintf(
		`DECLARE @rt_default sysname;
DECLARE @rt_sql nvarchar(max);
SELECT @rt_default = default_object.name
FROM sys.default_constraints default_object
JOIN sys.columns column_object
	ON column_object.object_id = default_object.parent_object_id
	AND column_object.column_id = default_object.parent_column_id
WHERE default_object.parent_object_id = OBJECT_ID(%s)
	AND column_object.name = %s;
IF @rt_default IS NOT NULL
BEGIN
	SET @rt_sql = %s + QUOTENAME(@rt_default) + N';';
	EXEC sys.sp_executesql @rt_sql;
END;`,
		sqlString(quoteQualified(table.Schema, table.Name)),
		sqlString(column),
		sqlString("ALTER TABLE "+tableName+" DROP CONSTRAINT "),
	)
}

func (s *SQLServer) buildColumnChange(
	change database.ColumnChange,
) ([]string, error) {
	change.Table.Schema = s.defaultSchema(change.Table.Schema)
	structures, err := s.GetCollectionStructures(change.Table)
	if err != nil {
		return nil, err
	}
	var current *database.Structure
	for index := range structures {
		if strings.EqualFold(structures[index].Name, change.Name) {
			current = &structures[index]
			break
		}
	}
	if current == nil {
		return nil, fmt.Errorf("column %q was not found", change.Name)
	}
	if strings.TrimSpace(change.Using) != "" {
		return nil, fmt.Errorf("SQL Server does not support a PostgreSQL USING clause")
	}
	tableName := quoteQualified(change.Table.Schema, change.Table.Name)
	activeName := change.Name
	statements := make([]string, 0, 4)
	if newName := strings.TrimSpace(change.NewName); newName != "" &&
		!strings.EqualFold(newName, activeName) {
		statements = append(
			statements,
			"EXEC sys.sp_rename "+
				sqlString(
					quoteQualified(change.Table.Schema, change.Table.Name)+
						"."+quoteIdentifier(activeName),
				)+", "+
				sqlString(newName)+", N'COLUMN';",
		)
		activeName = newName
	}
	if strings.TrimSpace(change.DataType) != "" || change.Nullable != nil {
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
		nullability := " NOT NULL"
		if nullable {
			nullability = " NULL"
		}
		statements = append(
			statements,
			"ALTER TABLE "+tableName+" ALTER COLUMN "+
				quoteIdentifier(activeName)+" "+dataType+nullability+";",
		)
	}
	if change.Default != nil || change.DropDefault {
		statements = append(
			statements,
			dropDefaultConstraintBatch(change.Table, activeName),
		)
		if change.Default != nil {
			if err := database.ValidateDDLFragment(
				*change.Default,
				"column default",
			); err != nil {
				return nil, err
			}
			statements = append(
				statements,
				"ALTER TABLE "+tableName+" ADD DEFAULT "+
					strings.TrimSpace(*change.Default)+" FOR "+
					quoteIdentifier(activeName)+";",
			)
		}
	}
	if len(statements) == 0 {
		return nil, fmt.Errorf("column change has no effective alterations")
	}
	return statements, nil
}

func validateConstraintDefinition(value string) error {
	if err := database.ValidateDDLFragment(value, "constraint definition"); err != nil {
		return err
	}
	keywords := database.LeadingSQLKeywords(value, 2)
	if len(keywords) == 0 {
		return fmt.Errorf("constraint definition is empty")
	}
	switch keywords[0] {
	case "PRIMARY", "UNIQUE", "FOREIGN", "CHECK", "DEFAULT":
		return nil
	default:
		return fmt.Errorf(
			"constraint must start with PRIMARY KEY, UNIQUE, FOREIGN KEY, CHECK, or DEFAULT",
		)
	}
}

func refreshReference(kind database.ObjectKind, table database.Table) database.ObjectReference {
	return database.ObjectReference{
		Kind:   kind,
		Schema: table.Schema,
		Name:   table.Name,
	}
}

func (s *SQLServer) BuildObjectChange(
	ctx context.Context,
	request database.ObjectChangeRequest,
) (database.ObjectChangePlan, error) {
	if err := request.Validate(); err != nil {
		return database.ObjectChangePlan{}, err
	}
	request.Reference.Schema = s.defaultSchema(request.Reference.Schema)
	switch request.Action {
	case database.ObjectChangeCreate, database.ObjectChangeReplace:
		replace := request.Action == database.ObjectChangeReplace
		var (
			statement string
			err       error
		)
		switch request.Reference.Kind {
		case database.ObjectKindView:
			statement, err = viewStatement(request.Reference, request.Definition, replace)
		case database.ObjectKindFunction,
			database.ObjectKindProcedure,
			database.ObjectKindTrigger:
			statement, err = reviewedDDL(
				request.Definition,
				request.Reference.Kind,
				replace,
			)
		default:
			err = fmt.Errorf(
				"creating SQL Server %s objects is not supported by the structural editor",
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
			Transactional: true,
			Refresh:       []database.ObjectReference{request.Reference},
		}, nil
	case database.ObjectChangeRename:
		statement, err := renameStatement(request.Reference, request.NewName)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		refresh := request.Reference
		refresh.ID = ""
		refresh.Name = strings.TrimSpace(request.NewName)
		return database.ObjectChangePlan{
			Summary:       "Rename " + string(request.Reference.Kind) + " " + request.Reference.QualifiedName(),
			Statements:    []string{statement},
			Transactional: true,
			Warnings: []string{
				"SQL Server sp_rename does not update references in dependent object definitions.",
			},
			Refresh: []database.ObjectReference{refresh},
		}, nil
	case database.ObjectChangeDrop:
		statement, err := dropStatement(request.Reference)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		return database.ObjectChangePlan{
			Summary:       "Drop " + string(request.Reference.Kind) + " " + request.Reference.QualifiedName(),
			Statements:    []string{statement},
			Destructive:   true,
			Transactional: true,
			Warnings:      []string{"Dropping an object can remove data or break dependencies."},
			Refresh:       []database.ObjectReference{request.Reference},
		}, nil
	case database.ObjectChangeEnable, database.ObjectChangeDisable:
		parent, err := parentTable(request.Reference)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		verb := "ENABLE"
		if request.Action == database.ObjectChangeDisable {
			verb = "DISABLE"
		}
		return database.ObjectChangePlan{
			Summary: strings.ToLower(verb) + " trigger " + request.Reference.Name,
			Statements: []string{
				verb + " TRIGGER " + quoteIdentifier(request.Reference.Name) +
					" ON " + parent + ";",
			},
			Transactional: true,
			Refresh:       []database.ObjectReference{request.Reference},
		}, nil
	case database.ObjectChangeCreateIndex:
		request.Index.Table.Schema = s.defaultSchema(request.Index.Table.Schema)
		statement, err := buildIndexChange(*request.Index)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		return database.ObjectChangePlan{
			Summary:       "Create index " + request.Index.Name,
			Statements:    []string{statement},
			Transactional: true,
			Warnings:      []string{"Creating an index can consume significant locks, CPU, and storage."},
			Refresh: []database.ObjectReference{
				refreshReference(database.ObjectKindTable, request.Index.Table),
			},
		}, nil
	case database.ObjectChangeAddColumn:
		request.AddColumn.Table.Schema = s.defaultSchema(request.AddColumn.Table.Schema)
		statement, err := buildAddColumn(*request.AddColumn)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		return database.ObjectChangePlan{
			Summary:       "Add column " + request.AddColumn.Column.Name,
			Statements:    []string{statement},
			Transactional: true,
			Warnings:      []string{"Adding a populated NOT NULL column can lock and rewrite the table."},
			Refresh: []database.ObjectReference{
				refreshReference(database.ObjectKindTable, request.AddColumn.Table),
			},
		}, nil
	case database.ObjectChangeAlterColumn:
		request.Column.Table.Schema = s.defaultSchema(request.Column.Table.Schema)
		statements, err := s.buildColumnChange(*request.Column)
		if err != nil {
			return database.ObjectChangePlan{}, err
		}
		return database.ObjectChangePlan{
			Summary:       "Alter column " + request.Column.Name,
			Statements:    statements,
			Transactional: true,
			Warnings:      []string{"Changing a column type or nullability can scan, lock, or rebuild the table."},
			Refresh: []database.ObjectReference{
				refreshReference(database.ObjectKindTable, request.Column.Table),
			},
		}, nil
	case database.ObjectChangeDropColumn:
		request.DropColumn.Table.Schema = s.defaultSchema(request.DropColumn.Table.Schema)
		return database.ObjectChangePlan{
			Summary: "Drop column " + request.DropColumn.Name,
			Statements: []string{
				dropDefaultConstraintBatch(
					request.DropColumn.Table,
					request.DropColumn.Name,
				),
				"ALTER TABLE " +
					quoteQualified(
						request.DropColumn.Table.Schema,
						request.DropColumn.Table.Name,
					) +
					" DROP COLUMN " + quoteIdentifier(request.DropColumn.Name) + ";",
			},
			Destructive:   true,
			Transactional: true,
			Warnings:      []string{"Dropping a column permanently removes its data."},
			Refresh: []database.ObjectReference{
				refreshReference(database.ObjectKindTable, request.DropColumn.Table),
			},
		}, nil
	case database.ObjectChangeAddConstraint,
		database.ObjectChangeDropConstraint:
		request.Constraint.Table.Schema = s.defaultSchema(request.Constraint.Table.Schema)
		table := quoteQualified(
			request.Constraint.Table.Schema,
			request.Constraint.Table.Name,
		)
		destructive := request.Action == database.ObjectChangeDropConstraint
		statement := "ALTER TABLE " + table
		if destructive {
			statement += " DROP CONSTRAINT " +
				quoteIdentifier(request.Constraint.Name) + ";"
		} else {
			if err := validateConstraintDefinition(
				request.Constraint.Definition,
			); err != nil {
				return database.ObjectChangePlan{}, err
			}
			statement += " ADD CONSTRAINT " +
				quoteIdentifier(request.Constraint.Name) + " " +
				strings.TrimSpace(request.Constraint.Definition) + ";"
		}
		verb := "Add"
		if destructive {
			verb = "Drop"
		}
		return database.ObjectChangePlan{
			Summary:       verb + " constraint " + request.Constraint.Name,
			Statements:    []string{statement},
			Destructive:   destructive,
			Transactional: true,
			Warnings:      []string{"Constraint validation can scan and lock the table."},
			Refresh: []database.ObjectReference{
				refreshReference(database.ObjectKindTable, request.Constraint.Table),
			},
		}, nil
	default:
		return database.ObjectChangePlan{}, fmt.Errorf(
			"unsupported SQL Server object change %q",
			request.Action,
		)
	}
}

func (s *SQLServer) ApplyObjectChange(
	ctx context.Context,
	plan database.ObjectChangePlan,
) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	if !plan.Transactional {
		return fmt.Errorf("SQL Server structural plans must be transactional")
	}
	if err := s.ensureConnected(); err != nil {
		return err
	}
	transaction, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
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
		return err
	}
	committed = true
	return nil
}

var _ database.ObjectChangeDriver = (*SQLServer)(nil)
