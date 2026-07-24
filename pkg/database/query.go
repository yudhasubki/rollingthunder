package database

import "context"

const DefaultQueryResultLimit = 1000

type QueryOptions struct {
	MaxRows int
	Args    []interface{}
}

type QueryVariable struct {
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
	Type  string      `json:"type,omitempty"`
}

type QueryRequest struct {
	ConnectionID            string          `json:"connectionId"`
	Query                   string          `json:"query"`
	AttemptID               string          `json:"attemptId"`
	TransactionID           string          `json:"transactionId,omitempty"`
	AllowUnfilteredMutation bool            `json:"allowUnfilteredMutation"`
	Variables               []QueryVariable `json:"variables,omitempty"`
}

type QueryResultSet struct {
	Index     int                      `json:"index"`
	Statement string                   `json:"statement"`
	Columns   []string                 `json:"columns"`
	Rows      []map[string]interface{} `json:"rows"`
	Truncated bool                     `json:"truncated"`
	RowLimit  int                      `json:"rowLimit"`
}

type QueryResult struct {
	Rows           []map[string]interface{} `json:"rows"`
	Truncated      bool                     `json:"truncated"`
	RowLimit       int                      `json:"rowLimit"`
	Columns        []string                 `json:"columns"`
	ResultSets     []QueryResultSet         `json:"resultSets"`
	StatementCount int                      `json:"statementCount"`
}

type Transaction interface {
	ExecuteQuery(
		ctx context.Context,
		query string,
		options QueryOptions,
	) (QueryResult, error)
	Commit() error
	Rollback() error
}

type TransactionalDriver interface {
	BeginTransaction(ctx context.Context) (Transaction, error)
}
