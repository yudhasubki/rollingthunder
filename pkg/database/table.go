package database

type Table struct {
	Schema string
	Name   string
	Page   int
	Limit  int
}

type TableData struct {
	Structures Structures               `json:"structures"`
	Data       []map[string]interface{} `json:"data"`
}
