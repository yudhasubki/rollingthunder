package database

type SortDirection string

const (
	SortAscending  SortDirection = "asc"
	SortDescending SortDirection = "desc"
)

type NullsPosition string

const (
	NullsFirst NullsPosition = "first"
	NullsLast  NullsPosition = "last"
)

type Sort struct {
	Column    string
	Direction SortDirection
	Nulls     NullsPosition
}

type Table struct {
	Schema  string
	Name    string
	Offset  int
	Limit   int
	Filters []Filter
	Sorts   []Sort
}

type TableData struct {
	Structures Structures               `json:"structures"`
	Data       []map[string]interface{} `json:"data"`
}
