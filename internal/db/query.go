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

	options := database.QueryOptions{
		MaxRows: database.DefaultQueryResultLimit,
	}
	var result database.QueryResult
	inTransaction := strings.TrimSpace(request.TransactionID) != ""

	if inTransaction {
		result, err = s.executeTransactionQuery(
			ctx,
			request.ConnectionID,
			request.TransactionID,
			request.Query,
			options,
		)
	} else {
		var driver database.Driver
		var release func()
		driver, release, err = s.driverFor(request.ConnectionID)
		if err == nil {
			defer release()
			result, err = driver.ExecuteQuery(
				ctx,
				request.Query,
				options,
			)
		}
	}

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
		}
		return queryFailure[database.QueryResult](err, inTransaction)
	}
	return response.BaseResponse[database.QueryResult]{Data: result}
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
