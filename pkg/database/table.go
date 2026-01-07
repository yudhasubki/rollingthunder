package database

type Table struct {
	Schema string
	Name   string
	Offset int
	Limit  int
	Filter string // WHERE clause for filtering data
}

type TableData struct {
	Structures Structures               `json:"structures"`
	Data       []map[string]interface{} `json:"data"`
}
