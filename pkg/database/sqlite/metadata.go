package sqlite

import (
	"database/sql"
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

type sqliteColumnRow struct {
	CID        int            `db:"cid"`
	Name       string         `db:"name"`
	Type       string         `db:"type"`
	NotNull    int            `db:"notnull"`
	Default    sql.NullString `db:"dflt_value"`
	PrimaryKey int            `db:"pk"`
	Hidden     int            `db:"hidden"`
}

type sqliteForeignKeyRow struct {
	ID           int            `db:"id"`
	Sequence     int            `db:"seq"`
	ForeignTable string         `db:"table"`
	Column       string         `db:"from"`
	ForeignCol   sql.NullString `db:"to"`
	OnUpdate     string         `db:"on_update"`
	OnDelete     string         `db:"on_delete"`
	Match        string         `db:"match"`
}

func sqliteAffinity(declaredType string) string {
	upper := strings.ToUpper(strings.TrimSpace(declaredType))
	switch {
	case strings.Contains(upper, "INT"):
		return "INTEGER"
	case strings.Contains(upper, "CHAR"),
		strings.Contains(upper, "CLOB"),
		strings.Contains(upper, "TEXT"):
		return "TEXT"
	case strings.Contains(upper, "BLOB"), upper == "":
		return "BLOB"
	case strings.Contains(upper, "REAL"),
		strings.Contains(upper, "FLOA"),
		strings.Contains(upper, "DOUB"):
		return "REAL"
	default:
		return "NUMERIC"
	}
}

func (s *SQLite) sqliteColumns(
	table database.Table,
) ([]sqliteColumnRow, error) {
	table.Schema = normalizeSQLiteSchema(table.Schema)
	query := fmt.Sprintf(
		"PRAGMA %s.table_xinfo(%s)",
		quoteSQLiteIdentifier(table.Schema),
		quoteSQLiteLiteral(table.Name),
	)
	var rows []sqliteColumnRow
	if err := s.conn.Select(&rows, query); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *SQLite) sqliteForeignKeys(
	table database.Table,
) ([]sqliteForeignKeyRow, error) {
	table.Schema = normalizeSQLiteSchema(table.Schema)
	query := fmt.Sprintf(
		"PRAGMA %s.foreign_key_list(%s)",
		quoteSQLiteIdentifier(table.Schema),
		quoteSQLiteLiteral(table.Name),
	)
	var rows []sqliteForeignKeyRow
	if err := s.conn.Select(&rows, query); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *SQLite) GetCollectionStructures(
	table database.Table,
) (database.Structures, error) {
	if err := s.ensureConnected(); err != nil {
		return nil, err
	}
	table.Schema = normalizeSQLiteSchema(table.Schema)
	if strings.TrimSpace(table.Name) == "" {
		return nil, fmt.Errorf("table name is required")
	}
	rows, err := s.sqliteColumns(table)
	if err != nil {
		return nil, err
	}
	foreignKeys, err := s.sqliteForeignKeys(table)
	if err != nil {
		return nil, err
	}
	foreignByColumn := make(map[string]sqliteForeignKeyRow, len(foreignKeys))
	for _, foreignKey := range foreignKeys {
		if _, exists := foreignByColumn[foreignKey.Column]; !exists {
			foreignByColumn[foreignKey.Column] = foreignKey
		}
	}

	indices, err := s.GetIndices(table)
	if err != nil {
		return nil, err
	}
	uniqueColumns := make(map[string]struct{})
	for _, index := range indices {
		if index.IsUnique && len(index.Columns) == 1 {
			uniqueColumns[index.Columns[0]] = struct{}{}
		}
	}

	ddl, err := s.GetTableDDL(table)
	if err != nil {
		return nil, err
	}
	withoutRowID := strings.Contains(strings.ToUpper(ddl), "WITHOUT ROWID")
	primaryCount := 0
	for _, row := range rows {
		if row.PrimaryKey > 0 {
			primaryCount++
		}
	}

	structures := make(database.Structures, 0, len(rows))
	for _, row := range rows {
		// hidden=1 is a virtual-table implementation column that SELECT *
		// does not expose. hidden=2/3 are generated columns and remain useful.
		if row.Hidden == 1 {
			continue
		}
		nativeType := strings.TrimSpace(row.Type)
		if nativeType == "" {
			nativeType = "BLOB"
		}
		affinity := sqliteAffinity(row.Type)
		isPrimary := row.PrimaryKey > 0
		isRowID := !withoutRowID &&
			primaryCount == 1 &&
			isPrimary &&
			strings.EqualFold(strings.TrimSpace(row.Type), "INTEGER")
		structure := database.Structure{
			Name:           row.Name,
			DataType:       nativeType,
			NativeType:     nativeType,
			Affinity:       affinity,
			Nullable:       row.NotNull == 0 && !isPrimary,
			IsPrimary:      isPrimary,
			IsPrimaryLabel: "",
			IsAutoInc:      isRowID,
			IsGenerated:    row.Hidden == 2 || row.Hidden == 3,
			IsRowID:        isRowID,
		}
		if isPrimary {
			structure.IsPrimaryLabel = "PRI"
		}
		if row.Hidden == 2 {
			structure.Generation = "VIRTUAL"
		} else if row.Hidden == 3 {
			structure.Generation = "STORED"
		}
		if row.Default.Valid {
			value := row.Default.String
			structure.Default = &value
		}
		if _, unique := uniqueColumns[row.Name]; unique {
			structure.IsUnique = true
		}
		if foreignKey, exists := foreignByColumn[row.Name]; exists {
			foreignSchema := table.Schema
			foreignTable := foreignKey.ForeignTable
			foreignColumn := foreignKey.ForeignCol.String
			label := foreignSchema + "." + foreignTable
			if foreignColumn != "" {
				label += "(" + foreignColumn + ")"
			}
			structure.ForeignSchema = &foreignSchema
			structure.ForeignTable = &foreignTable
			if foreignColumn != "" {
				structure.ForeignColumn = &foreignColumn
			}
			structure.ForeignKey = &label
		}
		structures = append(structures, structure)
	}
	return structures, nil
}

type sqliteIndexListRow struct {
	Sequence int    `db:"seq"`
	Name     string `db:"name"`
	Unique   int    `db:"unique"`
	Origin   string `db:"origin"`
	Partial  int    `db:"partial"`
}

type sqliteIndexInfoRow struct {
	Sequence int            `db:"seqno"`
	CID      int            `db:"cid"`
	Name     sql.NullString `db:"name"`
}

func (s *SQLite) GetIndices(table database.Table) (database.Indices, error) {
	if err := s.ensureConnected(); err != nil {
		return nil, err
	}
	table.Schema = normalizeSQLiteSchema(table.Schema)
	query := fmt.Sprintf(
		"PRAGMA %s.index_list(%s)",
		quoteSQLiteIdentifier(table.Schema),
		quoteSQLiteLiteral(table.Name),
	)
	var rows []sqliteIndexListRow
	if err := s.conn.Select(&rows, query); err != nil {
		return nil, err
	}

	indices := make(database.Indices, 0, len(rows))
	for _, row := range rows {
		detailQuery := fmt.Sprintf(
			"PRAGMA %s.index_info(%s)",
			quoteSQLiteIdentifier(table.Schema),
			quoteSQLiteLiteral(row.Name),
		)
		var details []sqliteIndexInfoRow
		if err := s.conn.Select(&details, detailQuery); err != nil {
			return nil, err
		}
		columns := make([]string, 0, len(details))
		for _, detail := range details {
			if detail.Name.Valid {
				columns = append(columns, detail.Name.String)
			} else {
				columns = append(columns, "<expression>")
			}
		}
		algorithm := "B-tree"
		if row.Partial != 0 {
			algorithm += " · partial"
		}
		indices = append(indices, database.Index{
			Name:      row.Name,
			Columns:   columns,
			IsUnique:  row.Unique != 0,
			IsPrimary: row.Origin == "pk",
			Algorithm: algorithm,
		})
	}
	return indices, nil
}
