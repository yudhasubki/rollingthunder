package database

type Structure struct {
	Name           string  `json:"name"`
	DataType       string  `json:"data_type"`
	Length         *int    `json:"length,omitempty"`
	Nullable       bool    `json:"nullable"`
	Default        *string `json:"default,omitempty"`
	IsPrimary      bool    `json:"is_primary,omitempty"`
	IsPrimaryLabel string  `json:"is_primary_label,omitempty"`
	IsUnique       bool    `json:"is_unique,omitempty"`
	IsAutoInc      bool    `json:"is_autoinc,omitempty"`
	ForeignKey     *string `json:"foreign_key,omitempty"`
	Comment        *string `json:"comment,omitempty"`
}

type Structures []Structure
