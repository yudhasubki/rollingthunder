package postgres

type Column struct {
	ColumnName    string  `db:"column_name"`
	DataType      string  `db:"data_type"`
	UDTSchema     string  `db:"udt_schema"`
	UDTName       string  `db:"udt_name"`
	IsEnum        bool    `db:"is_enum"`
	IsNullable    string  `db:"is_nullable"`
	MaxLength     *int    `db:"character_maximum_length"`
	ColumnDefault *string `db:"column_default"`
	IsIdentity    string  `db:"is_identity"`
	IdentityMode  *string `db:"identity_generation"`
	IsGenerated   string  `db:"is_generated"`
	Generation    *string `db:"generation_expression"`
	IsPrimary     bool    `db:"is_primary"`
}

type Columns []Column

type Constraint struct {
	Column        string  `db:"column"`
	Type          string  `db:"type"`
	ForeignSchema *string `db:"foreign_schema"`
	ForeignTable  *string `db:"foreign_table"`
	ForeignCol    *string `db:"foreign_column"`
}

type Constraints []Constraint

func (c Constraint) IsForeign() bool {
	return c.ForeignCol != nil && c.ForeignTable != nil
}
