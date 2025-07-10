package postgres

type Column struct {
	ColumnName    string  `db:"column_name"`
	DataType      string  `db:"data_type"`
	IsNullable    string  `db:"is_nullable"`
	MaxLength     *int    `db:"character_maximum_length"`
	ColumnDefault *string `db:"column_default"`
}

type Columns []Column

type Constraint struct {
	Column       string  `db:"column"`
	Type         string  `db:"type"`
	ForeignTable *string `db:"foreign_table"`
	ForeignCol   *string `db:"foreign_column"`
}

type Constraints []Constraint

func (c Constraint) IsForeign() bool {
	return c.ForeignCol != nil && c.ForeignTable != nil
}
