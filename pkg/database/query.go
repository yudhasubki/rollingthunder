package database

import "context"

const DefaultQueryResultLimit = 1000

type QueryOptions struct {
	MaxRows int
}

type QueryRequest struct {
	ConnectionID            string `json:"connectionId"`
	Query                   string `json:"query"`
	AttemptID               string `json:"attemptId"`
	TransactionID           string `json:"transactionId,omitempty"`
	AllowUnfilteredMutation bool   `json:"allowUnfilteredMutation"`
}

type QueryResult struct {
	Rows      []map[string]interface{} `json:"rows"`
	Truncated bool                     `json:"truncated"`
	RowLimit  int                      `json:"rowLimit"`
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
