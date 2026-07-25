package sqlserver

import (
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

func (s *SQLServer) CreateTable(
	table database.Table,
	columns []database.ColumnDefinition,
) error {
	if err := s.ensureConnected(); err != nil {
		return err
	}
	if strings.TrimSpace(table.Name) == "" {
		return fmt.Errorf("table name is required")
	}
	if len(columns) == 0 {
		return fmt.Errorf("at least one column is required")
	}
	table.Schema = s.defaultSchema(table.Schema)
	definitions := make([]string, 0, len(columns)+1)
	primaryKeys := make([]string, 0)
	for _, column := range columns {
		name := strings.TrimSpace(column.Name)
		dataType := strings.TrimSpace(column.Type)
		if name == "" {
			continue
		}
		if dataType == "" {
			return fmt.Errorf("data type is required for column %q", name)
		}
		if err := database.ValidateDDLFragment(
			dataType,
			"column data type",
		); err != nil {
			return err
		}
		definition := quoteIdentifier(name) + " " + dataType
		if !column.Nullable {
			definition += " NOT NULL"
		}
		if strings.TrimSpace(column.Default) != "" {
			if err := database.ValidateDDLFragment(
				column.Default,
				"column default",
			); err != nil {
				return err
			}
			definition += " DEFAULT " + strings.TrimSpace(column.Default)
		}
		if column.Unique {
			definition += " UNIQUE"
		}
		if column.PrimaryKey {
			primaryKeys = append(primaryKeys, quoteIdentifier(name))
		}
		definitions = append(definitions, definition)
	}
	if len(definitions) == 0 {
		return fmt.Errorf("at least one named column is required")
	}
	if len(primaryKeys) > 0 {
		definitions = append(
			definitions,
			"PRIMARY KEY ("+strings.Join(primaryKeys, ", ")+")",
		)
	}
	_, err := s.conn.Exec(
		"CREATE TABLE " + quoteQualified(table.Schema, table.Name) +
			" (" + strings.Join(definitions, ", ") + ")",
	)
	return err
}

func (s *SQLServer) DropTable(table database.Table) error {
	if err := s.ensureConnected(); err != nil {
		return err
	}
	if strings.TrimSpace(table.Name) == "" {
		return fmt.Errorf("table name is required")
	}
	table.Schema = s.defaultSchema(table.Schema)
	_, err := s.conn.Exec(
		"DROP TABLE IF EXISTS " + quoteQualified(table.Schema, table.Name),
	)
	return err
}

func (s *SQLServer) TruncateTable(table database.Table) error {
	if err := s.ensureConnected(); err != nil {
		return err
	}
	if strings.TrimSpace(table.Name) == "" {
		return fmt.Errorf("table name is required")
	}
	table.Schema = s.defaultSchema(table.Schema)
	_, err := s.conn.Exec(
		"TRUNCATE TABLE " + quoteQualified(table.Schema, table.Name),
	)
	return err
}

func (s *SQLServer) GetTableDDL(table database.Table) (string, error) {
	if err := s.ensureConnected(); err != nil {
		return "", err
	}
	if strings.TrimSpace(table.Name) == "" {
		return "", fmt.Errorf("table name is required")
	}
	table.Schema = s.defaultSchema(table.Schema)
	var tableCount int
	if err := s.conn.QueryRow(`
		SELECT COUNT(*)
		FROM sys.tables table_object
		JOIN sys.schemas schema_object
			ON schema_object.schema_id = table_object.schema_id
		WHERE schema_object.name = @p1 AND table_object.name = @p2`,
		table.Schema,
		table.Name,
	).Scan(&tableCount); err != nil {
		return "", err
	}
	if tableCount == 0 {
		return "", fmt.Errorf("SQL Server table %q was not found", table.Name)
	}
	structures, err := s.GetCollectionStructures(table)
	if err != nil {
		return "", err
	}
	if len(structures) == 0 {
		return "", fmt.Errorf("SQL Server table %q has no visible columns", table.Name)
	}
	definitions := make([]string, 0, len(structures)+1)
	for _, structure := range structures {
		var definition string
		if structure.IsGenerated &&
			structure.Generation != "" &&
			!strings.HasPrefix(structure.Generation, "IDENTITY") {
			definition = quoteIdentifier(structure.Name) +
				" AS " + structure.Generation
		} else {
			dataType := structure.DataType
			if structure.TypeSchema != nil &&
				structure.TypeName != nil {
				dataType = quoteQualified(
					*structure.TypeSchema,
					*structure.TypeName,
				)
			}
			definition = quoteIdentifier(structure.Name) +
				" " + dataType
			if structure.IsAutoInc {
				identity := structure.Generation
				if !strings.HasPrefix(identity, "IDENTITY(") {
					identity = "IDENTITY(1,1)"
				}
				definition += " " + identity
			}
			if !structure.Nullable {
				definition += " NOT NULL"
			} else {
				definition += " NULL"
			}
			if structure.Default != nil {
				definition += " DEFAULT " + *structure.Default
			}
		}
		definitions = append(definitions, definition)
	}
	keyConstraints, err := s.tableKeyConstraintDefinitions(table)
	if err != nil {
		return "", err
	}
	definitions = append(definitions, keyConstraints...)
	checkConstraints, err := s.tableCheckConstraintDefinitions(table)
	if err != nil {
		return "", err
	}
	definitions = append(definitions, checkConstraints...)
	foreignKeys, err := s.tableForeignKeyDefinitions(table)
	if err != nil {
		return "", err
	}
	definitions = append(definitions, foreignKeys...)
	return "CREATE TABLE " + quoteQualified(table.Schema, table.Name) +
		" (\n    " + strings.Join(definitions, ",\n    ") + "\n);", nil
}

type keyConstraintColumn struct {
	name       string
	kind       string
	algorithm  string
	column     string
	descending bool
	position   int
}

func (s *SQLServer) tableKeyConstraintDefinitions(
	table database.Table,
) ([]string, error) {
	rows, err := s.conn.Query(`
		SELECT
			constraint_object.name,
			constraint_object.type,
			index_object.type_desc,
			column_object.name,
			index_column.is_descending_key,
			index_column.key_ordinal
		FROM sys.key_constraints constraint_object
		JOIN sys.tables table_object
			ON table_object.object_id = constraint_object.parent_object_id
		JOIN sys.schemas schema_object
			ON schema_object.schema_id = table_object.schema_id
		JOIN sys.indexes index_object
			ON index_object.object_id = constraint_object.parent_object_id
			AND index_object.index_id = constraint_object.unique_index_id
		JOIN sys.index_columns index_column
			ON index_column.object_id = index_object.object_id
			AND index_column.index_id = index_object.index_id
			AND index_column.key_ordinal > 0
		JOIN sys.columns column_object
			ON column_object.object_id = index_column.object_id
			AND column_object.column_id = index_column.column_id
		WHERE schema_object.name = @p1 AND table_object.name = @p2
		ORDER BY constraint_object.name, index_column.key_ordinal`,
		table.Schema,
		table.Name,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]keyConstraintColumn, 0)
	for rows.Next() {
		var item keyConstraintColumn
		if err := rows.Scan(
			&item.name,
			&item.kind,
			&item.algorithm,
			&item.column,
			&item.descending,
			&item.position,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return buildKeyConstraintDefinitions(items), nil
}

func buildKeyConstraintDefinitions(
	items []keyConstraintColumn,
) []string {
	type constraint struct {
		name      string
		kind      string
		algorithm string
		columns   []string
	}
	constraints := make([]constraint, 0)
	positions := make(map[string]int)
	for _, item := range items {
		position, exists := positions[item.name]
		if !exists {
			position = len(constraints)
			positions[item.name] = position
			constraints = append(constraints, constraint{
				name:      item.name,
				kind:      item.kind,
				algorithm: item.algorithm,
				columns:   make([]string, 0),
			})
		}
		column := quoteIdentifier(item.column)
		if item.descending {
			column += " DESC"
		} else {
			column += " ASC"
		}
		constraints[position].columns = append(
			constraints[position].columns,
			column,
		)
	}
	definitions := make([]string, 0, len(constraints))
	for _, item := range constraints {
		kind := "UNIQUE"
		if item.kind == "PK" {
			kind = "PRIMARY KEY"
		}
		algorithm := strings.TrimSuffix(item.algorithm, "_INDEX")
		if algorithm != "CLUSTERED" && algorithm != "NONCLUSTERED" {
			algorithm = ""
		}
		definition := "CONSTRAINT " + quoteIdentifier(item.name) +
			" " + kind
		if algorithm != "" {
			definition += " " + algorithm
		}
		definition += " (" + strings.Join(item.columns, ", ") + ")"
		definitions = append(definitions, definition)
	}
	return definitions
}

func (s *SQLServer) tableForeignKeyDefinitions(
	table database.Table,
) ([]string, error) {
	rows, err := s.foreignKeyRows(table)
	if err != nil {
		return nil, err
	}
	return buildForeignKeyDefinitions(rows), nil
}

func buildForeignKeyDefinitions(
	rows []foreignKeyMetadata,
) []string {
	type constraint struct {
		name          string
		foreignSchema string
		foreignTable  string
		columns       []string
		foreignCols   []string
		deleteAction  string
		updateAction  string
		notReplicated bool
	}
	constraints := make([]constraint, 0)
	positions := make(map[string]int)
	for _, row := range rows {
		position, exists := positions[row.name]
		if !exists {
			position = len(constraints)
			positions[row.name] = position
			constraints = append(constraints, constraint{
				name:          row.name,
				foreignSchema: row.foreignSchema,
				foreignTable:  row.foreignTable,
				columns:       make([]string, 0),
				foreignCols:   make([]string, 0),
				deleteAction:  row.deleteAction,
				updateAction:  row.updateAction,
				notReplicated: row.notReplicated,
			})
		}
		constraints[position].columns = append(
			constraints[position].columns,
			quoteIdentifier(row.column),
		)
		constraints[position].foreignCols = append(
			constraints[position].foreignCols,
			quoteIdentifier(row.foreignColumn),
		)
	}
	definitions := make([]string, 0, len(constraints))
	for _, item := range constraints {
		definition := "CONSTRAINT " + quoteIdentifier(item.name) +
			" FOREIGN KEY (" + strings.Join(item.columns, ", ") + ") REFERENCES " +
			quoteQualified(item.foreignSchema, item.foreignTable) +
			" (" + strings.Join(item.foreignCols, ", ") + ")"
		if action := sqlServerReferentialAction(item.deleteAction); action != "" {
			definition += " ON DELETE " + action
		}
		if action := sqlServerReferentialAction(item.updateAction); action != "" {
			definition += " ON UPDATE " + action
		}
		if item.notReplicated {
			definition += " NOT FOR REPLICATION"
		}
		definitions = append(definitions, definition)
	}
	return definitions
}

func sqlServerReferentialAction(action string) string {
	action = strings.ToUpper(strings.TrimSpace(action))
	if action == "" || action == "NO_ACTION" {
		return ""
	}
	switch action {
	case "CASCADE", "SET_NULL", "SET_DEFAULT":
		return strings.ReplaceAll(action, "_", " ")
	default:
		return ""
	}
}

func (s *SQLServer) tableCheckConstraintDefinitions(
	table database.Table,
) ([]string, error) {
	rows, err := s.conn.Query(`
		SELECT
			constraint_object.name,
			constraint_object.definition,
			constraint_object.is_not_for_replication
		FROM sys.check_constraints constraint_object
		JOIN sys.tables table_object
			ON table_object.object_id = constraint_object.parent_object_id
		JOIN sys.schemas schema_object
			ON schema_object.schema_id = table_object.schema_id
		WHERE schema_object.name = @p1 AND table_object.name = @p2
		ORDER BY constraint_object.name`,
		table.Schema,
		table.Name,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	definitions := make([]string, 0)
	for rows.Next() {
		var (
			name          string
			expression    string
			notReplicated bool
		)
		if err := rows.Scan(
			&name,
			&expression,
			&notReplicated,
		); err != nil {
			return nil, err
		}
		definition := "CONSTRAINT " + quoteIdentifier(name) + " CHECK "
		if notReplicated {
			definition += "NOT FOR REPLICATION "
		}
		definition += expression
		definitions = append(definitions, definition)
	}
	return definitions, rows.Err()
}

func (s *SQLServer) GetDataTypes() []database.DataType {
	return []database.DataType{
		{Name: "tinyint", Category: "Numeric", Description: "Unsigned 8-bit integer"},
		{Name: "smallint", Category: "Numeric", Description: "16-bit integer"},
		{Name: "int", Category: "Numeric", Description: "32-bit integer"},
		{Name: "bigint", Category: "Numeric", Description: "64-bit integer"},
		{Name: "decimal", Category: "Numeric", Description: "Exact fixed-point number"},
		{Name: "float", Category: "Numeric", Description: "Floating-point number"},
		{Name: "real", Category: "Numeric", Description: "Single-precision number"},
		{Name: "money", Category: "Numeric", Description: "Currency value"},
		{Name: "bit", Category: "Boolean", Description: "Boolean-compatible bit"},
		{Name: "char", Category: "Character", Description: "Fixed-length text"},
		{Name: "varchar", Category: "Character", Description: "Variable-length text"},
		{Name: "nchar", Category: "Character", Description: "Fixed-length Unicode text"},
		{Name: "nvarchar", Category: "Character", Description: "Variable-length Unicode text"},
		{Name: "text", Category: "Character", Description: "Legacy long text"},
		{Name: "binary", Category: "Binary", Description: "Fixed-length bytes"},
		{Name: "varbinary", Category: "Binary", Description: "Variable-length bytes"},
		{Name: "date", Category: "Date/Time", Description: "Calendar date"},
		{Name: "time", Category: "Date/Time", Description: "Time of day"},
		{Name: "datetime2", Category: "Date/Time", Description: "High-precision date and time"},
		{Name: "datetimeoffset", Category: "Date/Time", Description: "Date and time with offset"},
		{Name: "uniqueidentifier", Category: "Identifier", Description: "Globally unique identifier"},
		{Name: "xml", Category: "Structured", Description: "XML document"},
		{Name: "sql_variant", Category: "Other", Description: "Value of several SQL Server types"},
	}
}
