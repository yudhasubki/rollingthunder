package db

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"rollingthunder/pkg/database"

	"github.com/jackc/pgx/v5/pgconn"
)

type routingTestTransaction struct {
	mu sync.Mutex

	queries      []string
	queryStarted chan struct{}
	queryRelease chan struct{}
	startOnce    sync.Once
	queryErr     error
	commitErr    error
	rollbackErr  error
	commits      int
	rollbacks    int
}

func (transaction *routingTestTransaction) ExecuteQuery(
	ctx context.Context,
	query string,
	options database.QueryOptions,
) (database.QueryResult, error) {
	transaction.mu.Lock()
	transaction.queries = append(transaction.queries, query)
	queryErr := transaction.queryErr
	transaction.mu.Unlock()

	if transaction.queryStarted != nil {
		transaction.startOnce.Do(func() {
			close(transaction.queryStarted)
		})
	}
	if transaction.queryRelease != nil {
		select {
		case <-transaction.queryRelease:
		case <-ctx.Done():
			return database.QueryResult{}, ctx.Err()
		}
	}
	if queryErr != nil {
		return database.QueryResult{}, queryErr
	}
	return database.QueryResult{
		Rows:     []map[string]interface{}{{"transaction": true}},
		RowLimit: options.MaxRows,
	}, nil
}

func (transaction *routingTestTransaction) Commit() error {
	transaction.mu.Lock()
	defer transaction.mu.Unlock()
	transaction.commits++
	return transaction.commitErr
}

func (transaction *routingTestTransaction) Rollback() error {
	transaction.mu.Lock()
	defer transaction.mu.Unlock()
	transaction.rollbacks++
	return transaction.rollbackErr
}

func (transaction *routingTestTransaction) snapshot() (
	queries int,
	commits int,
	rollbacks int,
) {
	transaction.mu.Lock()
	defer transaction.mu.Unlock()
	return len(transaction.queries), transaction.commits, transaction.rollbacks
}

func TestCancelQueryStopsDriverExecution(t *testing.T) {
	driver := &routingTestDriver{
		name:         "alpha",
		queryStarted: make(chan struct{}),
		queryRelease: make(chan struct{}),
	}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": driver},
		"alpha",
	)
	done := make(chan string, 1)

	go func() {
		result := service.ExecuteQuery(database.QueryRequest{
			ConnectionID: "alpha",
			Query:        "select pg_sleep(60)",
			AttemptID:    "cancel-query",
		})
		if len(result.Errors) == 0 {
			done <- ""
			return
		}
		done <- result.Errors[0].Code
	}()

	select {
	case <-driver.queryStarted:
	case <-time.After(time.Second):
		t.Fatal("query did not start")
	}
	cancelled := service.CancelQuery("cancel-query")
	if len(cancelled.Errors) != 0 || !cancelled.Data {
		t.Fatalf("CancelQuery = %+v", cancelled)
	}

	select {
	case code := <-done:
		if code != errorCodeQueryCancelled {
			t.Fatalf("query error code = %q", code)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled query did not return")
	}
	if len(service.queryAttempts) != 0 {
		t.Fatalf("running query attempts = %d, want 0", len(service.queryAttempts))
	}
}

func TestExecuteQueryRequiresConfirmationForUnfilteredMutation(t *testing.T) {
	driver := &routingTestDriver{name: "alpha"}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": driver},
		"alpha",
	)
	request := database.QueryRequest{
		ConnectionID: "alpha",
		Query:        "UPDATE accounts SET active = false",
	}

	blocked := service.ExecuteQuery(request)

	if len(blocked.Errors) != 1 ||
		blocked.Errors[0].Code != errorCodeUnsafeMutation {
		t.Fatalf("unsafe query errors = %+v", blocked.Errors)
	}
	if driver.queryCount() != 0 {
		t.Fatal("unsafe query reached the driver before confirmation")
	}

	request.AllowUnfilteredMutation = true
	confirmed := service.ExecuteQuery(request)
	if len(confirmed.Errors) != 0 {
		t.Fatalf("confirmed query errors = %+v", confirmed.Errors)
	}
	if driver.queryCount() != 1 {
		t.Fatalf("driver query count = %d, want 1", driver.queryCount())
	}
}

func TestExecuteQueryRejectsRawTransactionControl(t *testing.T) {
	driver := &routingTestDriver{name: "alpha"}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": driver},
		"alpha",
	)

	result := service.ExecuteQuery(database.QueryRequest{
		ConnectionID: "alpha",
		Query:        "BEGIN",
	})

	if len(result.Errors) != 1 ||
		result.Errors[0].Code != errorCodeTransactionControl {
		t.Fatalf("transaction control errors = %+v", result.Errors)
	}
	if driver.queryCount() != 0 {
		t.Fatal("raw transaction control reached the pooled driver")
	}
}

func TestExecuteQueryReportsMissingTransactionWithStableCode(t *testing.T) {
	driver := &routingTestDriver{name: "alpha"}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": driver},
		"alpha",
	)

	result := service.ExecuteQuery(database.QueryRequest{
		ConnectionID:  "alpha",
		Query:         "SELECT 1",
		TransactionID: "missing",
	})

	if len(result.Errors) != 1 ||
		result.Errors[0].Code != errorCodeTransactionNotFound {
		t.Fatalf("missing transaction errors = %+v", result.Errors)
	}
	if driver.queryCount() != 0 {
		t.Fatal("missing transaction query reached the pooled driver")
	}
}

func TestQueryErrorsExposeStableCodeAndHint(t *testing.T) {
	transaction := &routingTestTransaction{
		queryErr: &pgconn.PgError{
			Code:     "42601",
			Message:  "syntax error at or near SELECT",
			Position: 8,
		},
	}
	driver := &routingTestDriver{
		name:        "alpha",
		transaction: transaction,
	}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": driver},
		"alpha",
	)
	begin := service.BeginTransaction("alpha", "syntax-transaction")
	if len(begin.Errors) != 0 {
		t.Fatalf("begin transaction errors = %+v", begin.Errors)
	}

	result := service.ExecuteQuery(database.QueryRequest{
		ConnectionID:  "alpha",
		Query:         "SELECT SELECT",
		TransactionID: "syntax-transaction",
	})

	if len(result.Errors) != 1 {
		t.Fatalf("query errors = %+v", result.Errors)
	}
	queryError := result.Errors[0]
	if queryError.Code != errorCodeQuerySyntax {
		t.Fatalf("query error code = %q", queryError.Code)
	}
	if queryError.Hint == "" {
		t.Fatal("query error did not include an actionable hint")
	}
	_ = service.RollbackTransaction("syntax-transaction")
}

func TestExplicitTransactionRoutesQueriesAndCommits(t *testing.T) {
	transaction := &routingTestTransaction{}
	driver := &routingTestDriver{
		name:        "alpha",
		transaction: transaction,
	}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": driver},
		"alpha",
	)

	begin := service.BeginTransaction("alpha", "transaction-1")
	if len(begin.Errors) != 0 || begin.Data.State != "active" {
		t.Fatalf("BeginTransaction = %+v", begin)
	}
	result := service.ExecuteQuery(database.QueryRequest{
		ConnectionID:  "alpha",
		Query:         "select 1",
		TransactionID: "transaction-1",
	})
	if len(result.Errors) != 0 {
		t.Fatalf("transaction query errors = %+v", result.Errors)
	}
	if driver.queryCount() != 0 {
		t.Fatal("transaction query ran through the auto-commit driver")
	}

	committed := service.CommitTransaction("transaction-1")
	if len(committed.Errors) != 0 || committed.Data.State != "committed" {
		t.Fatalf("CommitTransaction = %+v", committed)
	}
	queries, commits, rollbacks := transaction.snapshot()
	if queries != 1 || commits != 1 || rollbacks != 0 {
		t.Fatalf(
			"transaction snapshot = queries:%d commits:%d rollbacks:%d",
			queries,
			commits,
			rollbacks,
		)
	}
	if len(service.transactions) != 0 {
		t.Fatalf("active transactions = %d, want 0", len(service.transactions))
	}
}

func TestTransactionContextLivesUntilTransactionFinishes(t *testing.T) {
	transaction := &routingTestTransaction{}
	driver := &routingTestDriver{
		name:        "alpha",
		transaction: transaction,
	}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": driver},
		"alpha",
	)

	begin := service.BeginTransaction("alpha", "context-transaction")
	if len(begin.Errors) != 0 {
		t.Fatalf("BeginTransaction errors = %+v", begin.Errors)
	}
	transactionContext := driver.transactionContext()
	if transactionContext == nil {
		t.Fatal("driver did not receive a transaction context")
	}
	select {
	case <-transactionContext.Done():
		t.Fatal("transaction context was cancelled immediately after begin")
	default:
	}

	committed := service.CommitTransaction("context-transaction")
	if len(committed.Errors) != 0 {
		t.Fatalf("CommitTransaction errors = %+v", committed.Errors)
	}
	select {
	case <-transactionContext.Done():
	case <-time.After(time.Second):
		t.Fatal("transaction context was not cancelled after commit")
	}
}

func TestDisconnectRollsBackActiveTransactions(t *testing.T) {
	transaction := &routingTestTransaction{}
	driver := &routingTestDriver{
		name:        "alpha",
		transaction: transaction,
	}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": driver},
		"alpha",
	)
	if result := service.BeginTransaction(
		"alpha",
		"disconnect-transaction",
	); len(result.Errors) != 0 {
		t.Fatalf("BeginTransaction errors = %+v", result.Errors)
	}

	disconnected := service.DisconnectConnection("alpha")

	if len(disconnected.Errors) != 0 {
		t.Fatalf("DisconnectConnection errors = %+v", disconnected.Errors)
	}
	_, commits, rollbacks := transaction.snapshot()
	if commits != 0 || rollbacks != 1 {
		t.Fatalf(
			"transaction commits = %d, rollbacks = %d",
			commits,
			rollbacks,
		)
	}
}

func TestFailedCommitStillClosesTransaction(t *testing.T) {
	transaction := &routingTestTransaction{
		commitErr: errors.New("commit failed"),
	}
	driver := &routingTestDriver{
		name:        "alpha",
		transaction: transaction,
	}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": driver},
		"alpha",
	)
	if result := service.BeginTransaction(
		"alpha",
		"failed-commit",
	); len(result.Errors) != 0 {
		t.Fatalf("BeginTransaction errors = %+v", result.Errors)
	}

	result := service.CommitTransaction("failed-commit")

	if len(result.Errors) != 1 ||
		result.Errors[0].Code != errorCodeTransactionFailed {
		t.Fatalf("CommitTransaction errors = %+v", result.Errors)
	}
	if len(service.transactions) != 0 {
		t.Fatal("failed commit left the transaction registered")
	}
}
