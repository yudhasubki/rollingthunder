package sqlserver

import (
	"database/sql"
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

func (s *SQLServer) GetSchemas() ([]string, error) {
	if err := s.ensureConnected(); err != nil {
		return nil, err
	}
	rows, err := s.conn.Query(`
		SELECT name
		FROM sys.schemas
		WHERE name NOT IN (
			'db_accessadmin', 'db_backupoperator', 'db_datareader',
			'db_datawriter', 'db_ddladmin', 'db_denydatareader',
			'db_denydatawriter', 'db_owner', 'db_securityadmin',
			'guest', 'INFORMATION_SCHEMA', 'sys'
		)
		ORDER BY name`)
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

func (s *SQLServer) GetCollections(schema ...string) ([]string, error) {
	if err := s.ensureConnected(); err != nil {
		return nil, err
	}
	target := ""
	if len(schema) > 0 {
		target = schema[0]
	}
	target = s.defaultSchema(target)
	rows, err := s.conn.Query(`
		SELECT table_object.name
		FROM sys.tables table_object
		JOIN sys.schemas schema_object
			ON schema_object.schema_id = table_object.schema_id
		WHERE schema_object.name = @p1
			AND table_object.is_ms_shipped = 0
		ORDER BY table_object.name`,
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
	name            string
	typeSchema      string
	typeName        string
	systemTypeName  string
	userDefined     bool
	maxLength       int64
	precision       int64
	scale           int64
	nullable        bool
	identity        bool
	computed        bool
	defaultValue    sql.NullString
	computedFormula sql.NullString
	computedPersist sql.NullBool
	identitySeed    sql.NullString
	identityStep    sql.NullString
	identityNoRepl  sql.NullBool
	primary         bool
	primaryName     sql.NullString
	unique          bool
	comment         sql.NullString
}

func displayType(column columnMetadata) string {
	if column.userDefined {
		return column.typeSchema + "." + column.typeName
	}
	native := strings.ToLower(column.systemTypeName)
	switch native {
	case "char", "varchar", "binary", "varbinary":
		if column.maxLength == -1 {
			return native + "(max)"
		}
		return fmt.Sprintf("%s(%d)", native, column.maxLength)
	case "nchar", "nvarchar":
		if column.maxLength == -1 {
			return native + "(max)"
		}
		return fmt.Sprintf("%s(%d)", native, column.maxLength/2)
	case "decimal", "numeric":
		return fmt.Sprintf("%s(%d,%d)", native, column.precision, column.scale)
	case "datetime2", "datetimeoffset", "time":
		return fmt.Sprintf("%s(%d)", native, column.scale)
	default:
		return native
	}
}

type foreignKeyMetadata struct {
	column        string
	foreignSchema string
	foreignTable  string
	foreignColumn string
	name          string
	position      int
	deleteAction  string
	updateAction  string
	notReplicated bool
}

func (s *SQLServer) foreignKeyRows(
	table database.Table,
) ([]foreignKeyMetadata, error) {
	rows, err := s.conn.Query(`
		SELECT
			parent_column.name,
			referenced_schema.name,
			referenced_table.name,
			referenced_column.name,
			foreign_key.name,
			foreign_key_column.constraint_column_id,
			foreign_key.delete_referential_action_desc,
			foreign_key.update_referential_action_desc,
			foreign_key.is_not_for_replication
		FROM sys.foreign_key_columns foreign_key_column
		JOIN sys.foreign_keys foreign_key
			ON foreign_key.object_id = foreign_key_column.constraint_object_id
		JOIN sys.tables parent_table
			ON parent_table.object_id = foreign_key_column.parent_object_id
		JOIN sys.schemas parent_schema
			ON parent_schema.schema_id = parent_table.schema_id
		JOIN sys.columns parent_column
			ON parent_column.object_id = parent_table.object_id
			AND parent_column.column_id = foreign_key_column.parent_column_id
		JOIN sys.tables referenced_table
			ON referenced_table.object_id = foreign_key_column.referenced_object_id
		JOIN sys.schemas referenced_schema
			ON referenced_schema.schema_id = referenced_table.schema_id
		JOIN sys.columns referenced_column
			ON referenced_column.object_id = referenced_table.object_id
			AND referenced_column.column_id = foreign_key_column.referenced_column_id
		WHERE parent_schema.name = @p1 AND parent_table.name = @p2
		ORDER BY foreign_key.name, foreign_key_column.constraint_column_id`,
		table.Schema,
		table.Name,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]foreignKeyMetadata, 0)
	for rows.Next() {
		var item foreignKeyMetadata
		if err := rows.Scan(
			&item.column,
			&item.foreignSchema,
			&item.foreignTable,
			&item.foreignColumn,
			&item.name,
			&item.position,
			&item.deleteAction,
			&item.updateAction,
			&item.notReplicated,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *SQLServer) foreignKeys(
	table database.Table,
) (map[string]foreignKeyMetadata, error) {
	rows, err := s.foreignKeyRows(table)
	if err != nil {
		return nil, err
	}
	result := make(map[string]foreignKeyMetadata, len(rows))
	for _, item := range rows {
		if _, exists := result[item.column]; !exists {
			result[item.column] = item
		}
	}
	return result, nil
}

func (s *SQLServer) GetCollectionStructures(
	table database.Table,
) (database.Structures, error) {
	if err := s.ensureConnected(); err != nil {
		return nil, err
	}
	table.Schema = s.defaultSchema(table.Schema)
	foreignKeys, err := s.foreignKeys(table)
	if err != nil {
		return nil, err
	}
	rows, err := s.conn.Query(`
		SELECT
			column_object.name,
			type_schema.name,
			user_type.name,
			system_type.name,
			user_type.is_user_defined,
			column_object.max_length,
			column_object.precision,
			column_object.scale,
			column_object.is_nullable,
			column_object.is_identity,
			column_object.is_computed,
			default_object.definition,
			computed_object.definition,
			computed_object.is_persisted,
			CONVERT(nvarchar(128), identity_object.seed_value),
			CONVERT(nvarchar(128), identity_object.increment_value),
			identity_object.is_not_for_replication,
			CONVERT(bit, CASE WHEN primary_index.index_id IS NULL THEN 0 ELSE 1 END),
			primary_index.name,
			CONVERT(bit, CASE WHEN unique_index.index_id IS NULL THEN 0 ELSE 1 END),
			CONVERT(nvarchar(max), description.value)
		FROM sys.objects relation
		JOIN sys.schemas relation_schema
			ON relation_schema.schema_id = relation.schema_id
		JOIN sys.columns column_object
			ON column_object.object_id = relation.object_id
		JOIN sys.types user_type
			ON user_type.user_type_id = column_object.user_type_id
		JOIN sys.schemas type_schema
			ON type_schema.schema_id = user_type.schema_id
		JOIN sys.types system_type
			ON system_type.user_type_id = column_object.system_type_id
			AND system_type.system_type_id = column_object.system_type_id
		LEFT JOIN sys.default_constraints default_object
			ON default_object.object_id = column_object.default_object_id
		LEFT JOIN sys.computed_columns computed_object
			ON computed_object.object_id = column_object.object_id
			AND computed_object.column_id = column_object.column_id
		LEFT JOIN sys.identity_columns identity_object
			ON identity_object.object_id = column_object.object_id
			AND identity_object.column_id = column_object.column_id
		OUTER APPLY (
			SELECT TOP (1) index_object.index_id, index_object.name
			FROM sys.indexes index_object
			JOIN sys.index_columns index_column
				ON index_column.object_id = index_object.object_id
				AND index_column.index_id = index_object.index_id
			WHERE index_object.object_id = relation.object_id
				AND index_column.column_id = column_object.column_id
				AND index_object.is_primary_key = 1
		) primary_index
		OUTER APPLY (
			SELECT TOP (1) index_object.index_id
			FROM sys.indexes index_object
			JOIN sys.index_columns index_column
				ON index_column.object_id = index_object.object_id
				AND index_column.index_id = index_object.index_id
			WHERE index_object.object_id = relation.object_id
				AND index_column.column_id = column_object.column_id
				AND index_object.is_unique = 1
				AND index_object.is_primary_key = 0
				AND (
					SELECT COUNT(*)
					FROM sys.index_columns unique_column
					WHERE unique_column.object_id = index_object.object_id
						AND unique_column.index_id = index_object.index_id
						AND unique_column.key_ordinal > 0
				) = 1
		) unique_index
		LEFT JOIN sys.extended_properties description
			ON description.major_id = relation.object_id
			AND description.minor_id = column_object.column_id
			AND description.name = 'MS_Description'
			AND description.class = 1
		WHERE relation_schema.name = @p1
			AND relation.name = @p2
			AND relation.type IN ('U', 'V')
		ORDER BY column_object.column_id`,
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
			&column.typeSchema,
			&column.typeName,
			&column.systemTypeName,
			&column.userDefined,
			&column.maxLength,
			&column.precision,
			&column.scale,
			&column.nullable,
			&column.identity,
			&column.computed,
			&column.defaultValue,
			&column.computedFormula,
			&column.computedPersist,
			&column.identitySeed,
			&column.identityStep,
			&column.identityNoRepl,
			&column.primary,
			&column.primaryName,
			&column.unique,
			&column.comment,
		); err != nil {
			return nil, err
		}
		structure := database.Structure{
			Name:       column.name,
			DataType:   displayType(column),
			NativeType: strings.ToLower(column.systemTypeName),
			Nullable:   column.nullable,
			IsPrimary:  column.primary,
			IsUnique:   column.unique,
			IsAutoInc:  column.identity,
			IsGenerated: column.identity ||
				column.computed,
		}
		if column.primaryName.Valid {
			structure.IsPrimaryLabel = column.primaryName.String
		}
		if column.maxLength > 0 {
			length := int(column.maxLength)
			if strings.HasPrefix(strings.ToLower(column.systemTypeName), "n") {
				length /= 2
			}
			structure.Length = &length
		}
		if column.defaultValue.Valid {
			value := column.defaultValue.String
			structure.Default = &value
		}
		if column.computedFormula.Valid {
			structure.Generation = column.computedFormula.String
			if column.computedPersist.Valid && column.computedPersist.Bool {
				structure.Generation += " PERSISTED"
			}
		} else if column.identity {
			seed := "1"
			step := "1"
			if column.identitySeed.Valid &&
				strings.TrimSpace(column.identitySeed.String) != "" {
				seed = strings.TrimSpace(column.identitySeed.String)
			}
			if column.identityStep.Valid &&
				strings.TrimSpace(column.identityStep.String) != "" {
				step = strings.TrimSpace(column.identityStep.String)
			}
			structure.Generation = "IDENTITY(" + seed + "," + step + ")"
			if column.identityNoRepl.Valid && column.identityNoRepl.Bool {
				structure.Generation += " NOT FOR REPLICATION"
			}
		}
		if column.userDefined {
			typeSchema := column.typeSchema
			typeName := column.typeName
			structure.TypeSchema = &typeSchema
			structure.TypeName = &typeName
		}
		if column.comment.Valid && strings.TrimSpace(column.comment.String) != "" {
			comment := column.comment.String
			structure.Comment = &comment
		}
		if relation, exists := foreignKeys[column.name]; exists {
			schema := relation.foreignSchema
			foreignTable := relation.foreignTable
			foreignColumn := relation.foreignColumn
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

func (s *SQLServer) GetIndices(
	table database.Table,
) (database.Indices, error) {
	if err := s.ensureConnected(); err != nil {
		return nil, err
	}
	table.Schema = s.defaultSchema(table.Schema)
	rows, err := s.conn.Query(`
		SELECT
			index_object.name,
			column_object.name,
			index_object.is_unique,
			index_object.is_primary_key,
			index_object.type_desc
		FROM sys.indexes index_object
		JOIN sys.tables table_object
			ON table_object.object_id = index_object.object_id
		JOIN sys.schemas schema_object
			ON schema_object.schema_id = table_object.schema_id
		JOIN sys.index_columns index_column
			ON index_column.object_id = index_object.object_id
			AND index_column.index_id = index_object.index_id
		JOIN sys.columns column_object
			ON column_object.object_id = index_column.object_id
			AND column_object.column_id = index_column.column_id
		WHERE schema_object.name = @p1
			AND table_object.name = @p2
			AND index_object.index_id > 0
			AND index_object.is_hypothetical = 0
			AND index_column.is_included_column = 0
		ORDER BY index_object.name, index_column.key_ordinal`,
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
			name      string
			column    string
			unique    bool
			primary   bool
			algorithm string
		)
		if err := rows.Scan(
			&name,
			&column,
			&unique,
			&primary,
			&algorithm,
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
				IsUnique:  unique,
				IsPrimary: primary,
				Algorithm: algorithm,
			})
		}
		indices[position].Columns = append(indices[position].Columns, column)
	}
	return indices, rows.Err()
}

func (s *SQLServer) GetDatabaseInfo() (database.Info, error) {
	if err := s.ensureConnected(); err != nil {
		return database.Info{}, err
	}
	var (
		version      string
		edition      string
		databaseName string
	)
	if err := s.conn.QueryRow(`
		SELECT
			CONVERT(nvarchar(128), SERVERPROPERTY('ProductVersion')),
			CONVERT(nvarchar(128), SERVERPROPERTY('Edition')),
			DB_NAME()`,
	).Scan(&version, &edition, &databaseName); err != nil {
		return database.Info{}, err
	}
	if strings.TrimSpace(edition) != "" {
		version += " · " + edition
	}
	return database.Info{
		Engine:   "Microsoft SQL Server",
		Version:  version,
		Database: databaseName,
	}, nil
}
