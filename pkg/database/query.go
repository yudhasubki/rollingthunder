package database

const DefaultQueryResultLimit = 1000

type QueryOptions struct {
	MaxRows int
}

type QueryResult struct {
	Rows      []map[string]interface{} `json:"rows"`
	Truncated bool                     `json:"truncated"`
	RowLimit  int                      `json:"rowLimit"`
}
