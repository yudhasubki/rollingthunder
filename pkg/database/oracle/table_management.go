package oracle

import (
	"database/sql"
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

func (o *Oracle) CreateTable(
	table database.Table,
	columns []database.ColumnDefinition,
) error {
	if err := o.ensureConnected(); err != nil {
		return err
	}
	if strings.TrimSpace(table.Name) == "" {
		return fmt.Errorf("table name is required")
	}
	if len(columns) == 0 {
		return fmt.Errorf("at least one column is required")
	}
	table.Schema = o.defaultSchema(table.Schema)
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
		if strings.TrimSpace(column.Default) != "" {
			if err := database.ValidateDDLFragment(
				column.Default,
				"column default",
			); err != nil {
				return err
			}
			definition += " DEFAULT " + strings.TrimSpace(column.Default)
		}
		if !column.Nullable {
			definition += " NOT NULL"
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
	_, err := o.conn.Exec(
		"CREATE TABLE " + quoteQualified(table.Schema, table.Name) +
			" (" + strings.Join(definitions, ", ") + ")",
	)
	return err
}

func (o *Oracle) DropTable(table database.Table) error {
	if err := o.ensureConnected(); err != nil {
		return err
	}
	if strings.TrimSpace(table.Name) == "" {
		return fmt.Errorf("table name is required")
	}
	table.Schema = o.defaultSchema(table.Schema)
	_, err := o.conn.Exec(
		"DROP TABLE " + quoteQualified(table.Schema, table.Name),
	)
	return err
}

func (o *Oracle) TruncateTable(table database.Table) error {
	if err := o.ensureConnected(); err != nil {
		return err
	}
	if strings.TrimSpace(table.Name) == "" {
		return fmt.Errorf("table name is required")
	}
	table.Schema = o.defaultSchema(table.Schema)
	_, err := o.conn.Exec(
		"TRUNCATE TABLE " + quoteQualified(table.Schema, table.Name),
	)
	return err
}

func (o *Oracle) GetTableDDL(table database.Table) (string, error) {
	if err := o.ensureConnected(); err != nil {
		return "", err
	}
	if strings.TrimSpace(table.Name) == "" {
		return "", fmt.Errorf("table name is required")
	}
	table.Schema = o.defaultSchema(table.Schema)
	var definition sql.NullString
	if err := o.conn.QueryRow(
		"SELECT DBMS_METADATA.GET_DDL('TABLE', :1, :2) FROM dual",
		table.Name,
		table.Schema,
	).Scan(&definition); err != nil {
		return "", err
	}
	if !definition.Valid || strings.TrimSpace(definition.String) == "" {
		return "", fmt.Errorf("Oracle returned no DDL for table %q", table.Name)
	}
	result := strings.TrimSpace(definition.String)
	if !strings.HasSuffix(result, ";") {
		result += ";"
	}
	return result, nil
}

func (o *Oracle) GetDataTypes() []database.DataType {
	return []database.DataType{
		{Name: "NUMBER", Category: "Numeric", Description: "Exact or floating-point number"},
		{Name: "BINARY_FLOAT", Category: "Numeric", Description: "32-bit floating-point number"},
		{Name: "BINARY_DOUBLE", Category: "Numeric", Description: "64-bit floating-point number"},
		{Name: "CHAR", Category: "Character", Description: "Fixed-length text"},
		{Name: "VARCHAR2", Category: "Character", Description: "Variable-length text"},
		{Name: "NCHAR", Category: "Character", Description: "Fixed-length national text"},
		{Name: "NVARCHAR2", Category: "Character", Description: "Variable-length national text"},
		{Name: "CLOB", Category: "Large object", Description: "Character large object"},
		{Name: "NCLOB", Category: "Large object", Description: "National character large object"},
		{Name: "BLOB", Category: "Large object", Description: "Binary large object"},
		{Name: "RAW", Category: "Binary", Description: "Variable-length raw bytes"},
		{Name: "DATE", Category: "Date/Time", Description: "Date and time to seconds"},
		{Name: "TIMESTAMP", Category: "Date/Time", Description: "Timestamp with fractional seconds"},
		{Name: "TIMESTAMP WITH TIME ZONE", Category: "Date/Time", Description: "Timestamp with time zone"},
		{Name: "INTERVAL YEAR TO MONTH", Category: "Interval", Description: "Year and month interval"},
		{Name: "INTERVAL DAY TO SECOND", Category: "Interval", Description: "Day and time interval"},
		{Name: "JSON", Category: "JSON", Description: "Native JSON value on supported Oracle versions"},
		{Name: "BOOLEAN", Category: "Boolean", Description: "SQL boolean on Oracle 23c and newer"},
	}
}
