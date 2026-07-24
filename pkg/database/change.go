package database

import "context"

type RowUpdate struct {
	Original       map[string]interface{} `json:"original"`
	Values         map[string]interface{} `json:"values"`
	ChangedColumns []string               `json:"changedColumns"`
}

type TableChangeSet struct {
	Table   Table                    `json:"table"`
	Added   []map[string]interface{} `json:"added"`
	Updated []RowUpdate              `json:"updated"`
	Deleted []map[string]interface{} `json:"deleted"`
}

type TableChangeResult struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
	Deleted  int `json:"deleted"`
}

func (changes TableChangeSet) Count() int {
	return len(changes.Added) + len(changes.Updated) + len(changes.Deleted)
}

type TableChangeDriver interface {
	ApplyTableChanges(
		ctx context.Context,
		changes TableChangeSet,
	) (TableChangeResult, error)
}
