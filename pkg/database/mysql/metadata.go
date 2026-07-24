package mysql

import (
	"database/sql"
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

type mysqlColumnRow struct {
	Name                 string         `db:"column_name"`
	DataType             string         `db:"data_type"`
	ColumnType           string         `db:"column_type"`
	Nullable             string         `db:"is_nullable"`
	MaxLength            sql.NullInt64  `db:"character_maximum_length"`
	Default              sql.NullString `db:"column_default"`
	Extra                string         `db:"extra"`
	ColumnKey            string         `db:"column_key"`
	GenerationExpression sql.NullString `db:"generation_expression"`
	Comment              string         `db:"column_comment"`
}

type mysqlForeignKeyRow struct {
	Column        string `db:"column_name"`
	Constraint    string `db:"constraint_name"`
	ForeignSchema string `db:"referenced_table_schema"`
	ForeignTable  string `db:"referenced_table_name"`
	ForeignColumn string `db:"referenced_column_name"`
}

func (m *MySQL) GetCollectionStructures(
	table database.Table,
) (database.Structures, error) {
	if err := m.ensureConnected(); err != nil {
		return nil, err
	}
	databaseName := m.defaultDatabase(table.Schema)
	if databaseName == "" || strings.TrimSpace(table.Name) == "" {
		return nil, fmt.Errorf("database and table names are required")
	}

	var rows []mysqlColumnRow
	const columnQuery = `
		SELECT
			column_name,
			data_type,
			column_type,
			is_nullable,
			character_maximum_length,
			column_default,
			extra,
			column_key,
			generation_expression,
			column_comment
		FROM information_schema.columns
		WHERE table_schema = ? AND table_name = ?
		ORDER BY ordinal_position`
	if err := m.conn.Select(&rows, columnQuery, databaseName, table.Name); err != nil {
		return nil, err
	}

	var foreignKeys []mysqlForeignKeyRow
	const foreignQuery = `
		SELECT
			column_name,
			constraint_name,
			referenced_table_schema,
			referenced_table_name,
			referenced_column_name
		FROM information_schema.key_column_usage
		WHERE table_schema = ?
		  AND table_name = ?
		  AND referenced_table_name IS NOT NULL
		ORDER BY constraint_name, ordinal_position`
	if err := m.conn.Select(
		&foreignKeys,
		foreignQuery,
		databaseName,
		table.Name,
	); err != nil {
		return nil, err
	}
	foreignByColumn := make(map[string]mysqlForeignKeyRow, len(foreignKeys))
	for _, foreignKey := range foreignKeys {
		if _, exists := foreignByColumn[foreignKey.Column]; !exists {
			foreignByColumn[foreignKey.Column] = foreignKey
		}
	}

	structures := make(database.Structures, 0, len(rows))
	for _, row := range rows {
		dataType := strings.ToLower(row.DataType)
		isEnum := dataType == "enum"
		if isEnum {
			dataType = "enum"
		} else if strings.TrimSpace(row.ColumnType) != "" {
			dataType = strings.ToLower(row.ColumnType)
		}
		structure := database.Structure{
			Name:           row.Name,
			DataType:       dataType,
			NativeType:     row.ColumnType,
			Nullable:       strings.EqualFold(row.Nullable, "YES"),
			IsPrimary:      strings.EqualFold(row.ColumnKey, "PRI"),
			IsUnique:       strings.EqualFold(row.ColumnKey, "UNI"),
			IsAutoInc:      strings.Contains(strings.ToLower(row.Extra), "auto_increment"),
			IsEnum:         isEnum,
			IsGenerated:    strings.Contains(strings.ToLower(row.Extra), "generated"),
			Generation:     row.GenerationExpression.String,
			IsPrimaryLabel: "",
		}
		if structure.IsPrimary {
			structure.IsPrimaryLabel = "PRI"
		}
		if row.MaxLength.Valid {
			length := int(row.MaxLength.Int64)
			structure.Length = &length
		}
		if row.Default.Valid {
			value := row.Default.String
			structure.Default = &value
		}
		if strings.TrimSpace(row.Comment) != "" {
			comment := row.Comment
			structure.Comment = &comment
		}
		if isEnum {
			typeName := row.ColumnType
			structure.TypeName = &typeName
		}
		if foreignKey, exists := foreignByColumn[row.Name]; exists {
			schema := foreignKey.ForeignSchema
			foreignTable := foreignKey.ForeignTable
			foreignColumn := foreignKey.ForeignColumn
			label := fmt.Sprintf(
				"%s.%s(%s)",
				schema,
				foreignTable,
				foreignColumn,
			)
			structure.ForeignSchema = &schema
			structure.ForeignTable = &foreignTable
			structure.ForeignColumn = &foreignColumn
			structure.ForeignKey = &label
		}
		structures = append(structures, structure)
	}
	return structures, nil
}

type mysqlIndexRow struct {
	Name      string `db:"index_name"`
	Column    string `db:"column_name"`
	Sequence  int    `db:"seq_in_index"`
	NonUnique int    `db:"non_unique"`
	IndexType string `db:"index_type"`
}

func (m *MySQL) GetIndices(table database.Table) (database.Indices, error) {
	if err := m.ensureConnected(); err != nil {
		return nil, err
	}
	databaseName := m.defaultDatabase(table.Schema)
	var rows []mysqlIndexRow
	const query = `
		SELECT
			index_name,
			COALESCE(column_name, expression, '') AS column_name,
			seq_in_index,
			non_unique,
			index_type
		FROM information_schema.statistics
		WHERE table_schema = ? AND table_name = ?
		ORDER BY index_name, seq_in_index`
	if err := m.conn.Select(&rows, query, databaseName, table.Name); err != nil {
		// MariaDB versions without STATISTICS.EXPRESSION use the compatible
		// column-only catalog shape.
		const fallback = `
			SELECT
				index_name,
				COALESCE(column_name, '') AS column_name,
				seq_in_index,
				non_unique,
				index_type
			FROM information_schema.statistics
			WHERE table_schema = ? AND table_name = ?
			ORDER BY index_name, seq_in_index`
		if fallbackErr := m.conn.Select(
			&rows,
			fallback,
			databaseName,
			table.Name,
		); fallbackErr != nil {
			return nil, err
		}
	}

	indices := make(database.Indices, 0)
	byName := make(map[string]int)
	for _, row := range rows {
		index, exists := byName[row.Name]
		if !exists {
			index = len(indices)
			byName[row.Name] = index
			indices = append(indices, database.Index{
				Name:      row.Name,
				Columns:   make([]string, 0),
				IsUnique:  row.NonUnique == 0,
				IsPrimary: strings.EqualFold(row.Name, "PRIMARY"),
				Algorithm: row.IndexType,
			})
		}
		if row.Column != "" {
			indices[index].Columns = append(indices[index].Columns, row.Column)
		}
	}
	return indices, nil
}
