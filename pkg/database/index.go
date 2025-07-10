package database

type Index struct {
	Name      string   `json:"name"`
	Columns   []string `json:"columns"`
	IsUnique  bool     `json:"is_unique"`
	Algorithm string   `json:"algorithm"`
}

type Indices []Index
