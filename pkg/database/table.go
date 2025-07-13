package database

type Table struct {
	Schema string
	Name   string
	Offset int
	Limit  int
}

type TableData struct {
	Structures Structures               `json:"structures"`
	Data       []map[string]interface{} `json:"data"`
}
