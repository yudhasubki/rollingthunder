package oracle

import (
	"database/sql"
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

func (o *Oracle) GetSchemas() ([]string, error) {
	if err := o.ensureConnected(); err != nil {
		return nil, err
	}
	rows, err := o.conn.Query(`
		SELECT schema_name
		FROM (
			SELECT DISTINCT owner AS schema_name
			FROM all_objects
			WHERE object_type IN (
				'TABLE', 'VIEW', 'MATERIALIZED VIEW', 'FUNCTION',
				'PROCEDURE', 'TRIGGER', 'SEQUENCE', 'TYPE'
			)
			UNION
			SELECT SYS_CONTEXT('USERENV', 'CURRENT_SCHEMA') FROM dual
		)
		WHERE schema_name = SYS_CONTEXT('USERENV', 'CURRENT_SCHEMA')
			OR schema_name NOT IN (
				'ANONYMOUS', 'APPQOSSYS', 'AUDSYS', 'CTXSYS', 'DBSFWUSER',
				'DBSNMP', 'DIP', 'DVF', 'DVSYS', 'GGSYS', 'GSMADMIN_INTERNAL',
				'GSMCATUSER', 'GSMROOTUSER', 'GSMUSER', 'LBACSYS', 'MDDATA',
				'MDSYS', 'OJVMSYS', 'OLAPSYS', 'ORACLE_OCM', 'ORDDATA',
				'ORDPLUGINS', 'ORDSYS', 'OUTLN', 'REMOTE_SCHEDULER_AGENT',
				'SI_INFORMTN_SCHEMA', 'SYS', 'SYS$UMF', 'SYSBACKUP', 'SYSDG',
				'SYSKM', 'SYSRAC', 'SYSTEM', 'WMSYS', 'XDB', 'XS$NULL'
			)
		ORDER BY schema_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	schemas := make([]string, 0)
	for rows.Next() {
		var schema string
		if err := rows.Scan(&schema); err != nil {
			return nil, err
		}
		schemas = append(schemas, schema)
	}
	return schemas, rows.Err()
}

func (o *Oracle) GetCollections(schema ...string) ([]string, error) {
	if err := o.ensureConnected(); err != nil {
		return nil, err
	}
	target := ""
	if len(schema) > 0 {
		target = schema[0]
	}
	target = o.defaultSchema(target)
	rows, err := o.conn.Query(`
		SELECT table_name
		FROM all_tables
		WHERE owner = :1
			AND nested = 'NO'
			AND secondary = 'N'
		ORDER BY table_name`,
		target,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tables := make([]string, 0)
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	return tables, rows.Err()
}

type columnMetadata struct {
	name          string
	nativeType    string
	dataLength    sql.NullInt64
	charLength    sql.NullInt64
	precision     sql.NullInt64
	scale         sql.NullInt64
	nullable      string
	defaultValue  sql.NullString
	identity      string
	identityMode  sql.NullString
	virtualColumn string
}

func displayOracleType(column columnMetadata) string {
	nativeType := strings.ToUpper(strings.TrimSpace(column.nativeType))
	switch nativeType {
	case "CHAR", "NCHAR", "VARCHAR2", "NVARCHAR2", "RAW":
		length := column.charLength
		if nativeType == "RAW" {
			length = column.dataLength
		}
		if length.Valid && length.Int64 > 0 {
			return fmt.Sprintf("%s(%d)", nativeType, length.Int64)
		}
	case "NUMBER":
		if column.precision.Valid {
			if column.scale.Valid && column.scale.Int64 != 0 {
				return fmt.Sprintf(
					"NUMBER(%d,%d)",
					column.precision.Int64,
					column.scale.Int64,
				)
			}
			return fmt.Sprintf("NUMBER(%d)", column.precision.Int64)
		}
	case "FLOAT":
		if column.precision.Valid {
			return fmt.Sprintf("FLOAT(%d)", column.precision.Int64)
		}
	}
	return nativeType
}

type constraintMetadata struct {
	constraintType string
	column         string
	name           string
	foreignSchema  sql.NullString
	foreignTable   sql.NullString
	foreignColumn  sql.NullString
	columnCount    int
}

func (o *Oracle) collectionConstraints(
	table database.Table,
) ([]constraintMetadata, error) {
	rows, err := o.conn.Query(`
		SELECT
			constraint_object.constraint_type,
			constraint_column.column_name,
			constraint_object.constraint_name,
			referenced_object.owner,
			referenced_object.table_name,
			referenced_column.column_name,
			COUNT(*) OVER (
				PARTITION BY constraint_object.owner,
					constraint_object.constraint_name
			)
		FROM all_constraints constraint_object
		JOIN all_cons_columns constraint_column
			ON constraint_column.owner = constraint_object.owner
			AND constraint_column.constraint_name = constraint_object.constraint_name
			AND constraint_column.table_name = constraint_object.table_name
		LEFT JOIN all_constraints referenced_object
			ON referenced_object.owner = constraint_object.r_owner
			AND referenced_object.constraint_name = constraint_object.r_constraint_name
		LEFT JOIN all_cons_columns referenced_column
			ON referenced_column.owner = referenced_object.owner
			AND referenced_column.constraint_name = referenced_object.constraint_name
			AND referenced_column.table_name = referenced_object.table_name
			AND referenced_column.position = constraint_column.position
		WHERE constraint_object.owner = :1
			AND constraint_object.table_name = :2
			AND constraint_object.constraint_type IN ('P', 'U', 'R')
		ORDER BY constraint_object.constraint_name, constraint_column.position`,
		table.Schema,
		table.Name,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]constraintMetadata, 0)
	for rows.Next() {
		var item constraintMetadata
		if err := rows.Scan(
			&item.constraintType,
			&item.column,
			&item.name,
			&item.foreignSchema,
			&item.foreignTable,
			&item.foreignColumn,
			&item.columnCount,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (o *Oracle) columnComments(
	table database.Table,
) (map[string]string, error) {
	rows, err := o.conn.Query(`
		SELECT column_name, comments
		FROM all_col_comments
		WHERE owner = :1 AND table_name = :2`,
		table.Schema,
		table.Name,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	comments := make(map[string]string)
	for rows.Next() {
		var (
			column  string
			comment sql.NullString
		)
		if err := rows.Scan(&column, &comment); err != nil {
			return nil, err
		}
		if comment.Valid && strings.TrimSpace(comment.String) != "" {
			comments[column] = comment.String
		}
	}
	return comments, rows.Err()
}

func (o *Oracle) GetCollectionStructures(
	table database.Table,
) (database.Structures, error) {
	if err := o.ensureConnected(); err != nil {
		return nil, err
	}
	table.Schema = o.defaultSchema(table.Schema)
	constraints, err := o.collectionConstraints(table)
	if err != nil {
		return nil, err
	}
	comments, err := o.columnComments(table)
	if err != nil {
		return nil, err
	}
	primary := make(map[string]string)
	unique := make(map[string]bool)
	foreign := make(map[string]constraintMetadata)
	for _, constraint := range constraints {
		switch constraint.constraintType {
		case "P":
			primary[constraint.column] = constraint.name
		case "U":
			if constraint.columnCount == 1 {
				unique[constraint.column] = true
			}
		case "R":
			foreign[constraint.column] = constraint
		}
	}
	rows, err := o.conn.Query(`
		SELECT
			column_name,
			data_type,
			data_length,
			char_length,
			data_precision,
			data_scale,
			nullable,
			data_default,
			identity_column,
			(
				SELECT identity_object.generation_type
				FROM all_tab_identity_cols identity_object
				WHERE identity_object.owner = all_tab_columns.owner
					AND identity_object.table_name = all_tab_columns.table_name
					AND identity_object.column_name = all_tab_columns.column_name
			),
			virtual_column
		FROM all_tab_columns
		WHERE owner = :1 AND table_name = :2
		ORDER BY column_id`,
		table.Schema,
		table.Name,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	structures := make(database.Structures, 0)
	for rows.Next() {
		var column columnMetadata
		if err := rows.Scan(
			&column.name,
			&column.nativeType,
			&column.dataLength,
			&column.charLength,
			&column.precision,
			&column.scale,
			&column.nullable,
			&column.defaultValue,
			&column.identity,
			&column.identityMode,
			&column.virtualColumn,
		); err != nil {
			return nil, err
		}
		length := column.charLength
		if strings.EqualFold(column.nativeType, "RAW") {
			length = column.dataLength
		}
		structure := database.Structure{
			Name:           column.name,
			DataType:       displayOracleType(column),
			NativeType:     strings.ToUpper(column.nativeType),
			Nullable:       strings.EqualFold(column.nullable, "Y"),
			IsPrimary:      primary[column.name] != "",
			IsPrimaryLabel: primary[column.name],
			IsUnique:       unique[column.name],
			IsAutoInc:      strings.EqualFold(column.identity, "YES"),
			IsGenerated: strings.EqualFold(column.identity, "YES") ||
				strings.EqualFold(column.virtualColumn, "YES"),
		}
		if length.Valid && length.Int64 > 0 {
			value := int(length.Int64)
			structure.Length = &value
		}
		if column.defaultValue.Valid {
			value := strings.TrimSpace(column.defaultValue.String)
			if value != "" {
				structure.Default = &value
			}
		}
		if strings.EqualFold(column.identity, "YES") {
			structure.Generation = "IDENTITY"
			if column.identityMode.Valid &&
				strings.TrimSpace(column.identityMode.String) != "" {
				structure.Generation += " " + strings.ToUpper(
					strings.TrimSpace(column.identityMode.String),
				)
			}
		} else if strings.EqualFold(column.virtualColumn, "YES") {
			structure.Generation = "VIRTUAL"
		}
		if comment, ok := comments[column.name]; ok {
			value := comment
			structure.Comment = &value
		}
		if relation, ok := foreign[column.name]; ok &&
			relation.foreignTable.Valid &&
			relation.foreignColumn.Valid {
			schema := relation.foreignSchema.String
			foreignTable := relation.foreignTable.String
			foreignColumn := relation.foreignColumn.String
			label := fmt.Sprintf(
				"%s.%s(%s)",
				schema,
				foreignTable,
				foreignColumn,
			)
			structure.ForeignKey = &label
			structure.ForeignSchema = &schema
			structure.ForeignTable = &foreignTable
			structure.ForeignColumn = &foreignColumn
		}
		structures = append(structures, structure)
	}
	return structures, rows.Err()
}

func (o *Oracle) GetIndices(
	table database.Table,
) (database.Indices, error) {
	if err := o.ensureConnected(); err != nil {
		return nil, err
	}
	table.Schema = o.defaultSchema(table.Schema)
	rows, err := o.conn.Query(`
		SELECT
			index_object.index_name,
			index_column.column_name,
			index_expression.column_expression,
			index_object.uniqueness,
			index_object.index_type,
			CASE
				WHEN primary_constraint.constraint_type = 'P' THEN 1
				ELSE 0
			END
		FROM all_indexes index_object
		JOIN all_ind_columns index_column
			ON index_column.index_owner = index_object.owner
			AND index_column.index_name = index_object.index_name
		LEFT JOIN all_ind_expressions index_expression
			ON index_expression.index_owner = index_column.index_owner
			AND index_expression.index_name = index_column.index_name
			AND index_expression.table_owner = index_object.table_owner
			AND index_expression.table_name = index_object.table_name
			AND index_expression.column_position = index_column.column_position
		LEFT JOIN all_constraints primary_constraint
			ON primary_constraint.owner = index_object.owner
			AND primary_constraint.table_name = index_object.table_name
			AND primary_constraint.index_name = index_object.index_name
			AND primary_constraint.constraint_type = 'P'
		WHERE index_object.table_owner = :1
			AND index_object.table_name = :2
		ORDER BY index_object.index_name, index_column.column_position`,
		table.Schema,
		table.Name,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	indices := make(database.Indices, 0)
	positions := make(map[string]int)
	for rows.Next() {
		var (
			name       string
			column     sql.NullString
			expression sql.NullString
			uniqueness string
			algorithm  string
			primary    int
		)
		if err := rows.Scan(
			&name,
			&column,
			&expression,
			&uniqueness,
			&algorithm,
			&primary,
		); err != nil {
			return nil, err
		}
		position, exists := positions[name]
		if !exists {
			position = len(indices)
			positions[name] = position
			indices = append(indices, database.Index{
				Name:      name,
				Columns:   make([]string, 0),
				IsUnique:  strings.EqualFold(uniqueness, "UNIQUE"),
				IsPrimary: primary == 1,
				Algorithm: algorithm,
			})
		}
		indices[position].Columns = append(
			indices[position].Columns,
			oracleIndexColumn(column, expression),
		)
	}
	return indices, rows.Err()
}

func oracleIndexColumn(
	column sql.NullString,
	expression sql.NullString,
) string {
	if expression.Valid && strings.TrimSpace(expression.String) != "" {
		return strings.TrimSpace(expression.String)
	}
	if column.Valid && strings.TrimSpace(column.String) != "" {
		return column.String
	}
	return "(expression)"
}

func (o *Oracle) GetDatabaseInfo() (database.Info, error) {
	if err := o.ensureConnected(); err != nil {
		return database.Info{}, err
	}
	var (
		version      string
		databaseName string
	)
	if err := o.conn.QueryRow(`
		SELECT version_full
		FROM product_component_version
		WHERE product LIKE 'Oracle Database%'
		FETCH FIRST 1 ROW ONLY`).Scan(&version); err != nil {
		if fallbackErr := o.conn.QueryRow(`
			SELECT version
			FROM product_component_version
			WHERE product LIKE 'Oracle Database%'
			FETCH FIRST 1 ROW ONLY`).Scan(&version); fallbackErr != nil {
			return database.Info{}, fallbackErr
		}
	}
	if err := o.conn.QueryRow(
		"SELECT SYS_CONTEXT('USERENV', 'DB_NAME') FROM dual",
	).Scan(&databaseName); err != nil {
		return database.Info{}, err
	}
	return database.Info{
		Engine:   "Oracle Database",
		Version:  version,
		Database: databaseName,
	}, nil
}
