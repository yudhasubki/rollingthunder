package postgres

type Index struct {
	IndexName  string `db:"index_name"`
	ColumnName string `db:"column_name"`
	IsUnique   bool   `db:"is_unique"`
	IsPrimary  bool   `db:"is_primary"`
	Algorithm  string `db:"algorithm"`
}

type Indices []Index
