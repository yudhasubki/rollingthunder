package database

type Structure struct {
	Name           string  `json:"name"`
	DataType       string  `json:"data_type"`
	TypeSchema     *string `json:"type_schema,omitempty"`
	TypeName       *string `json:"type_name,omitempty"`
	IsEnum         bool    `json:"is_enum,omitempty"`
	Length         *int    `json:"length,omitempty"`
	Nullable       bool    `json:"nullable"`
	Default        *string `json:"default,omitempty"`
	IsPrimary      bool    `json:"is_primary,omitempty"`
	IsPrimaryLabel string  `json:"is_primary_label,omitempty"`
	IsUnique       bool    `json:"is_unique,omitempty"`
	IsAutoInc      bool    `json:"is_autoinc,omitempty"`
	ForeignKey     *string `json:"foreign_key,omitempty"`
	ForeignSchema  *string `json:"foreign_schema,omitempty"`
	ForeignTable   *string `json:"foreign_table,omitempty"`
	ForeignColumn  *string `json:"foreign_column,omitempty"`
	Comment        *string `json:"comment,omitempty"`
}

type Structures []Structure
