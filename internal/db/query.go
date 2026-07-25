package db

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"

	"github.com/google/uuid"
)

var (
	errTransactionNotActive = errors.New("transaction is not active")
	errTransactionMismatch  = errors.New(
		"transaction belongs to a different connection",
	)
	errTransactionConnectionClosed = errors.New(
		"transaction connection is disconnected",
	)
)

type queryAttempt struct {
	id        string
	cancel    context.CancelFunc
	cancelled atomic.Bool
}

func (s *Service) startQueryAttempt(
	requestedID string,
) (context.Context, *queryAttempt, error) {
	if s.ctx == nil {
		return nil, nil, fmt.Errorf("application context is unavailable")
	}

	attemptID := strings.TrimSpace(requestedID)
	if attemptID == "" {
		attemptID = uuid.NewString()
	}
	if len(attemptID) > 128 {
		return nil, nil, fmt.Errorf("query attempt ID is too long")
	}

	ctx, cancel := context.WithCancel(s.ctx)
	attempt := &queryAttempt{id: attemptID, cancel: cancel}

	s.queryAttemptMu.Lock()
	if _, exists := s.queryAttempts[attemptID]; exists {
		s.queryAttemptMu.Unlock()
		cancel()
		return nil, nil, fmt.Errorf(
			"query attempt %q is already running",
			attemptID,
		)
	}
	s.queryAttempts[attemptID] = attempt
	s.queryAttemptMu.Unlock()
	return ctx, attempt, nil
}

func (s *Service) finishQueryAttempt(attempt *queryAttempt) {
	if attempt == nil {
		return
	}
	attempt.cancel()

	s.queryAttemptMu.Lock()
	if s.queryAttempts[attempt.id] == attempt {
		delete(s.queryAttempts, attempt.id)
	}
	s.queryAttemptMu.Unlock()
}

func (s *Service) CancelQuery(
	attemptID string,
) response.BaseResponse[bool] {
	s.queryAttemptMu.RLock()
	attempt := s.queryAttempts[strings.TrimSpace(attemptID)]
	s.queryAttemptMu.RUnlock()
	if attempt == nil {
		return serviceErrorWithCode[bool](
			http.StatusNotFound,
			errorCodeQueryNotRunning,
			"Query is not running",
			"The query has already finished or its attempt ID is unknown.",
			"Use the result of ExecuteQuery as the authoritative final state.",
		)
	}

	attempt.cancelled.Store(true)
	attempt.cancel()
	return response.BaseResponse[bool]{Data: true}
}

func (s *Service) ExecuteQuery(
	request database.QueryRequest,
) response.BaseResponse[database.QueryResult] {
	request.Query = strings.TrimSpace(request.Query)
	if request.Query == "" {
		return serviceErrorWithCode[database.QueryResult](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Query is empty",
			"Enter a SQL statement before running it.",
			"Select a statement or place the cursor inside the statement to execute.",
		)
	}

	if control := database.FindTransactionControl(request.Query); control != "" {
		return serviceErrorWithCode[database.QueryResult](
			http.StatusConflict,
			errorCodeTransactionControl,
			"Use explicit transaction controls",
			fmt.Sprintf(
				"%s cannot be run as a pooled auto-commit query.",
				control,
			),
			"Use Begin, Commit, and Rollback in the SQL editor toolbar so every statement stays on the same database transaction.",
		)
	}

	statements, err := database.SplitSQLStatements(request.Query)
	if err != nil {
		return serviceErrorWithCode[database.QueryResult](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Invalid query batch",
			err.Error(),
			"Run fewer statements at once or select one statement in the editor.",
		)
	}
	if len(statements) == 0 {
		return serviceErrorWithCode[database.QueryResult](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Query is empty",
			"The selected SQL contains no executable statement.",
			"Select a statement or place the cursor inside one before running it.",
		)
	}

	safety := database.AnalyzeQuerySafety(request.Query)
	if safety.RequiresConfirmation() && !request.AllowUnfilteredMutation {
		mutations := strings.Join(safety.UnfilteredMutations, " and ")
		return serviceErrorWithCode[database.QueryResult](
			http.StatusConflict,
			errorCodeUnsafeMutation,
			"Unfiltered mutation requires confirmation",
			fmt.Sprintf(
				"%s without a top-level WHERE clause can affect every row.",
				mutations,
			),
			"Review the statement, add a WHERE clause, or explicitly confirm that you want to run it.",
		)
	}

	ctx, attempt, err := s.startQueryAttempt(request.AttemptID)
	if err != nil {
		return serviceErrorWithCode[database.QueryResult](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Invalid query request",
			err.Error(),
			"Create a unique query attempt and try again.",
		)
	}
	defer s.finishQueryAttempt(attempt)

	inTransaction := strings.TrimSpace(request.TransactionID) != ""
	result, err := s.executeQueryBatch(ctx, request, statements)

	if err != nil {
		if attempt.cancelled.Load() {
			return queryFailure[database.QueryResult](
				context.Canceled,
				inTransaction,
			)
		}
		switch {
		case errors.Is(err, errTransactionNotActive):
			return serviceErrorWithCode[database.QueryResult](
				http.StatusNotFound,
				errorCodeTransactionNotFound,
				"Transaction is not active",
				err.Error(),
				"Start a new transaction before running the query.",
			)
		case errors.Is(err, errTransactionMismatch):
			return serviceErrorWithCode[database.QueryResult](
				http.StatusConflict,
				errorCodeInvalidRequest,
				"Transaction connection mismatch",
				err.Error(),
				"Run the query from the tab and connection that started the transaction.",
			)
		case errors.Is(err, errTransactionConnectionClosed):
			return serviceErrorWithCode[database.QueryResult](
				http.StatusGone,
				errorCodeTransactionNotFound,
				"Transaction connection was closed",
				err.Error(),
				"Reconnect the database and start a new transaction.",
			)
		case errors.Is(err, errConnectionReadOnly):
			return readOnlyConnectionError[database.QueryResult]()
		}
		return queryFailure[database.QueryResult](err, inTransaction)
	}
	return response.BaseResponse[database.QueryResult]{Data: result}
}

type queryStatementRunner func(
	context.Context,
	string,
	database.QueryOptions,
) (database.QueryResult, error)

func (s *Service) executeQueryBatch(
	ctx context.Context,
	request database.QueryRequest,
	statements []string,
) (database.QueryResult, error) {
	var (
		capabilityDriver database.CapabilityDriver
		run              queryStatementRunner
		release          func()
	)
	requiresWrite := database.FindWriteStatement(request.Query) != ""

	if strings.TrimSpace(request.TransactionID) != "" {
		s.transactionMu.RLock()
		session := s.transactions[strings.TrimSpace(request.TransactionID)]
		s.transactionMu.RUnlock()
		if session == nil {
			return database.QueryResult{}, errTransactionNotActive
		}
		if session.connectionID != request.ConnectionID {
			return database.QueryResult{}, errTransactionMismatch
		}

		session.connection.mu.RLock()
		if session.connection.closed {
			session.connection.mu.RUnlock()
			return database.QueryResult{}, errTransactionConnectionClosed
		}
		session.mu.Lock()
		if session.closed {
			session.mu.Unlock()
			session.connection.mu.RUnlock()
			return database.QueryResult{}, errTransactionNotActive
		}
		if requiresWrite &&
			!connectionWriteAccessLocked(session.connection).WriteEnabled {
			session.mu.Unlock()
			session.connection.mu.RUnlock()
			return database.QueryResult{}, errConnectionReadOnly
		}
		capabilityDriver = session.connection.Driver
		run = session.transaction.ExecuteQuery
		release = func() {
			session.mu.Unlock()
			session.connection.mu.RUnlock()
		}
	} else {
		var (
			driver        database.Driver
			driverRelease func()
			err           error
		)
		if requiresWrite {
			driver, driverRelease, err = s.writeDriverFor(request.ConnectionID)
		} else {
			driver, driverRelease, err = s.driverFor(request.ConnectionID)
		}
		if err != nil {
			return database.QueryResult{}, err
		}
		capabilityDriver = driver
		run = driver.ExecuteQuery
		release = driverRelease
	}
	defer release()

	batch := database.QueryResult{
		Rows:           make([]map[string]interface{}, 0),
		Columns:        make([]string, 0),
		RowLimit:       database.DefaultQueryResultLimit,
		ResultSets:     make([]database.QueryResultSet, 0, len(statements)),
		StatementCount: len(statements),
	}

	for index, statement := range statements {
		boundQuery, args, err := database.BindQueryVariables(
			statement,
			capabilityDriver,
			request.Variables,
		)
		if err != nil {
			return database.QueryResult{}, fmt.Errorf(
				"statement %d: %w",
				index+1,
				err,
			)
		}
		result, err := run(ctx, boundQuery, database.QueryOptions{
			MaxRows: database.DefaultQueryResultLimit,
			Args:    args,
		})
		if err != nil {
			return database.QueryResult{}, fmt.Errorf(
				"statement %d of %d failed: %w",
				index+1,
				len(statements),
				err,
			)
		}
		set := database.QueryResultSet{
			Index:     index,
			Statement: statement,
			Columns:   result.Columns,
			Rows:      result.Rows,
			Truncated: result.Truncated,
			RowLimit:  result.RowLimit,
		}
		if set.Columns == nil {
			set.Columns = make([]string, 0)
		}
		if set.Rows == nil {
			set.Rows = make([]map[string]interface{}, 0)
		}
		batch.ResultSets = append(batch.ResultSets, set)
	}

	if len(batch.ResultSets) > 0 {
		first := batch.ResultSets[0]
		batch.Rows = first.Rows
		batch.Columns = first.Columns
		batch.Truncated = first.Truncated
		batch.RowLimit = first.RowLimit
	}
	return batch, nil
}

type transactionSession struct {
	id           string
	connectionID string
	connection   *Connection
	transaction  database.Transaction
	cancel       context.CancelFunc
	startedAt    time.Time
	mu           sync.Mutex
	closed       bool
}

type TransactionInfo struct {
	ID           string    `json:"id"`
	ConnectionID string    `json:"connectionId"`
	State        string    `json:"state"`
	StartedAt    time.Time `json:"startedAt"`
}

func transactionInfo(
	session *transactionSession,
	state string,
) TransactionInfo {
	return TransactionInfo{
		ID:           session.id,
		ConnectionID: session.connectionID,
		State:        state,
		StartedAt:    session.startedAt,
	}
}

func (s *Service) BeginTransaction(
	connectionID string,
	requestedID string,
) response.BaseResponse[TransactionInfo] {
	if s.ctx == nil {
		return serviceErrorWithCode[TransactionInfo](
			http.StatusServiceUnavailable,
			errorCodeTransactionFailed,
			"Application is not ready",
			"The application context is unavailable.",
			"Wait for Rolling Thunder to finish starting, then try again.",
		)
	}

	connection, release, err := s.pinnedConnection(connectionID)
	if err != nil {
		return serviceErrorWithCode[TransactionInfo](
			http.StatusNotFound,
			errorCodeTransactionNotFound,
			"Connection unavailable",
			err.Error(),
			"Reconnect the database before starting a transaction.",
		)
	}
	defer release()

	transactional, ok := connection.Driver.(database.TransactionalDriver)
	if !ok {
		return serviceErrorWithCode[TransactionInfo](
			http.StatusNotImplemented,
			errorCodeTransactionUnsupported,
			"Transactions are not supported",
			"The active database driver does not implement explicit transactions.",
			"Use auto-commit mode for this connection.",
		)
	}

	transactionID := strings.TrimSpace(requestedID)
	if transactionID == "" {
		transactionID = uuid.NewString()
	}
	if len(transactionID) > 128 {
		return serviceErrorWithCode[TransactionInfo](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Invalid transaction ID",
			"Transaction ID is too long.",
			"Generate a new transaction ID and try again.",
		)
	}

	transactionContext, cancel := context.WithCancel(s.ctx)
	transaction, err := transactional.BeginTransaction(transactionContext)
	if err != nil {
		cancel()
		return queryFailure[TransactionInfo](err, false)
	}

	session := &transactionSession{
		id:           transactionID,
		connectionID: connectionID,
		connection:   connection,
		transaction:  transaction,
		cancel:       cancel,
		startedAt:    time.Now(),
	}

	s.transactionMu.Lock()
	if _, exists := s.transactions[transactionID]; exists {
		s.transactionMu.Unlock()
		_ = transaction.Rollback()
		cancel()
		return serviceErrorWithCode[TransactionInfo](
			http.StatusConflict,
			errorCodeInvalidRequest,
			"Transaction already exists",
			fmt.Sprintf(
				"Transaction %q is already active.",
				transactionID,
			),
			"Generate a unique transaction ID and try again.",
		)
	}
	s.transactions[transactionID] = session
	s.transactionMu.Unlock()

	return response.BaseResponse[TransactionInfo]{
		Data: transactionInfo(session, "active"),
	}
}

func (s *Service) executeTransactionQuery(
	ctx context.Context,
	connectionID string,
	transactionID string,
	query string,
	options database.QueryOptions,
) (database.QueryResult, error) {
	s.transactionMu.RLock()
	session := s.transactions[strings.TrimSpace(transactionID)]
	s.transactionMu.RUnlock()
	if session == nil {
		return database.QueryResult{}, errTransactionNotActive
	}
	if session.connectionID != connectionID {
		return database.QueryResult{}, errTransactionMismatch
	}

	session.connection.mu.RLock()
	defer session.connection.mu.RUnlock()
	if session.connection.closed {
		return database.QueryResult{}, errTransactionConnectionClosed
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return database.QueryResult{}, errTransactionNotActive
	}
	return session.transaction.ExecuteQuery(ctx, query, options)
}

func (s *Service) CommitTransaction(
	transactionID string,
) response.BaseResponse[TransactionInfo] {
	return s.finishTransaction(transactionID, true)
}

func (s *Service) RollbackTransaction(
	transactionID string,
) response.BaseResponse[TransactionInfo] {
	return s.finishTransaction(transactionID, false)
}

func (s *Service) finishTransaction(
	transactionID string,
	commit bool,
) response.BaseResponse[TransactionInfo] {
	s.transactionMu.RLock()
	session := s.transactions[strings.TrimSpace(transactionID)]
	s.transactionMu.RUnlock()
	if session == nil {
		return serviceErrorWithCode[TransactionInfo](
			http.StatusNotFound,
			errorCodeTransactionNotFound,
			"Transaction is not active",
			"The transaction has already finished or does not exist.",
			"Start a new transaction before running transactional queries.",
		)
	}

	session.connection.mu.RLock()
	defer session.connection.mu.RUnlock()
	session.mu.Lock()
	if session.closed {
		session.mu.Unlock()
		return serviceErrorWithCode[TransactionInfo](
			http.StatusNotFound,
			errorCodeTransactionNotFound,
			"Transaction is not active",
			"The transaction has already finished.",
			"Start a new transaction before continuing.",
		)
	}

	var err error
	state := "rolled_back"
	if commit {
		state = "committed"
		err = session.transaction.Commit()
	} else {
		err = session.transaction.Rollback()
	}
	session.closed = true
	session.cancel()
	info := transactionInfo(session, state)
	session.mu.Unlock()

	s.transactionMu.Lock()
	if s.transactions[session.id] == session {
		delete(s.transactions, session.id)
	}
	s.transactionMu.Unlock()

	if err != nil {
		return serviceErrorWithCode[TransactionInfo](
			http.StatusConflict,
			errorCodeTransactionFailed,
			"Transaction could not be completed",
			err.Error(),
			"If the transaction was aborted, start a new transaction before retrying.",
		)
	}
	return response.BaseResponse[TransactionInfo]{Data: info}
}

// rollbackTransactionsForConnection is called while the connection write lock
// is held, so no query can start or remain active on these transactions.
func (s *Service) rollbackTransactionsForConnection(connectionID string) {
	s.transactionMu.Lock()
	sessions := make([]*transactionSession, 0)
	for id, session := range s.transactions {
		if session.connectionID == connectionID {
			delete(s.transactions, id)
			sessions = append(sessions, session)
		}
	}
	s.transactionMu.Unlock()

	for _, session := range sessions {
		session.mu.Lock()
		if !session.closed {
			_ = session.transaction.Rollback()
			session.closed = true
			session.cancel()
		}
		session.mu.Unlock()
	}
}
