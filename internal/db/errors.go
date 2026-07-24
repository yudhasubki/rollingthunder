package db

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"rollingthunder/pkg/response"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	errorCodeDatabaseOperationFailed    = "DATABASE_OPERATION_FAILED"
	errorCodeConnectionFailed           = "CONNECTION_FAILED"
	errorCodeConnectionTimeout          = "CONNECTION_TIMEOUT"
	errorCodeConnectionCancelled        = "CONNECTION_CANCELLED"
	errorCodeConnectionRefused          = "CONNECTION_REFUSED"
	errorCodeAuthenticationFailed       = "AUTHENTICATION_FAILED"
	errorCodeDatabaseNotFound           = "DATABASE_NOT_FOUND"
	errorCodeTLSConfiguration           = "TLS_CONFIGURATION_ERROR"
	errorCodeQueryFailed                = "QUERY_FAILED"
	errorCodeQueryCancelled             = "QUERY_CANCELLED"
	errorCodeQueryNotRunning            = "QUERY_NOT_RUNNING"
	errorCodeQuerySyntax                = "QUERY_SYNTAX_ERROR"
	errorCodeQueryConstraint            = "QUERY_CONSTRAINT_VIOLATION"
	errorCodeQueryPermission            = "QUERY_PERMISSION_DENIED"
	errorCodeTransactionAborted         = "TRANSACTION_ABORTED"
	errorCodeTransactionNotFound        = "TRANSACTION_NOT_FOUND"
	errorCodeTransactionUnsupported     = "TRANSACTION_UNSUPPORTED"
	errorCodeTransactionFailed          = "TRANSACTION_FAILED"
	errorCodeTransactionControl         = "TRANSACTION_CONTROL_REQUIRES_MODE"
	errorCodeUnsafeMutation             = "UNFILTERED_MUTATION_REQUIRES_CONFIRMATION"
	errorCodeTableChangesFailed         = "TABLE_CHANGES_FAILED"
	errorCodeTableChangesUnsupported    = "TABLE_CHANGES_UNSUPPORTED"
	errorCodeCapabilityInvalid          = "DRIVER_CAPABILITY_INVALID"
	errorCodeObjectMetadataFailed       = "OBJECT_METADATA_FAILED"
	errorCodeObjectMetadataUnsupported  = "OBJECT_METADATA_UNSUPPORTED"
	errorCodeObjectChangeUnsupported    = "OBJECT_CHANGE_UNSUPPORTED"
	errorCodeObjectChangePreviewFailed  = "OBJECT_CHANGE_PREVIEW_FAILED"
	errorCodeObjectChangeReviewRequired = "OBJECT_CHANGE_REVIEW_REQUIRED"
	errorCodeObjectChangeFailed         = "OBJECT_CHANGE_FAILED"
	errorCodeInvalidRequest             = "INVALID_REQUEST"
)

func serviceErrorWithCode[T any](
	status int,
	code string,
	title string,
	detail string,
	hint string,
) response.BaseResponse[T] {
	return response.BaseResponse[T]{
		Errors: []response.BaseErrorResponse{
			{
				Title:  title,
				Status: status,
				Code:   code,
				Detail: detail,
				Hint:   hint,
			},
		},
	}
}

func connectionFailure[T any](
	attempt *connectionAttempt,
	connectErr error,
) response.BaseResponse[T] {
	contextErr := attempt.ctx.Err()
	switch {
	case errors.Is(contextErr, context.DeadlineExceeded),
		errors.Is(connectErr, context.DeadlineExceeded):
		return serviceErrorWithCode[T](
			http.StatusGatewayTimeout,
			errorCodeConnectionTimeout,
			"Connection timed out",
			fmt.Sprintf(
				"Connection timed out after %s.",
				formatConnectionTimeout(attempt.timeout),
			),
			"Check the host, port, VPN, firewall, and whether the database accepts remote connections.",
		)
	case attempt.cancelled.Load(),
		errors.Is(contextErr, context.Canceled),
		errors.Is(connectErr, context.Canceled):
		return serviceErrorWithCode[T](
			499,
			errorCodeConnectionCancelled,
			"Connection cancelled",
			"Connection attempt cancelled.",
			"You can edit the profile and try again.",
		)
	}

	var postgresError *pgconn.PgError
	if errors.As(connectErr, &postgresError) {
		switch postgresError.Code {
		case "28P01", "28000":
			return serviceErrorWithCode[T](
				http.StatusUnauthorized,
				errorCodeAuthenticationFailed,
				"Authentication failed",
				"PostgreSQL rejected the username or password.",
				"Verify the username, password, and pg_hba.conf authentication rules.",
			)
		case "3D000":
			return serviceErrorWithCode[T](
				http.StatusNotFound,
				errorCodeDatabaseNotFound,
				"Database not found",
				"The requested PostgreSQL database does not exist.",
				"Check the database name or connect to an existing database first.",
			)
		}
	}

	lowerDetail := strings.ToLower(connectErr.Error())
	switch {
	case strings.Contains(lowerDetail, "connection refused"):
		return serviceErrorWithCode[T](
			http.StatusBadGateway,
			errorCodeConnectionRefused,
			"Connection refused",
			"The database host actively refused the connection.",
			"Check that PostgreSQL is running and listening on the configured host and port.",
		)
	case strings.Contains(lowerDetail, "certificate"),
		strings.Contains(lowerDetail, "tls"),
		strings.Contains(lowerDetail, "ssl"):
		return serviceErrorWithCode[T](
			http.StatusBadRequest,
			errorCodeTLSConfiguration,
			"TLS configuration failed",
			connectErr.Error(),
			"Check the SSL mode and certificate paths, then verify the server certificate hostname.",
		)
	default:
		return serviceErrorWithCode[T](
			http.StatusBadGateway,
			errorCodeConnectionFailed,
			"Connection failed",
			connectErr.Error(),
			"Verify the connection profile and confirm the database is reachable.",
		)
	}
}

func queryFailure[T any](
	queryErr error,
	inTransaction bool,
) response.BaseResponse[T] {
	if errors.Is(queryErr, context.Canceled) {
		hint := "Run the query again when you are ready."
		if inTransaction {
			hint = "The transaction may be aborted; roll it back before running another statement."
		}
		return serviceErrorWithCode[T](
			499,
			errorCodeQueryCancelled,
			"Query cancelled",
			"Query execution cancelled.",
			hint,
		)
	}

	var postgresError *pgconn.PgError
	if errors.As(queryErr, &postgresError) {
		detail := postgresError.Message
		if postgresError.Position > 0 {
			detail = fmt.Sprintf(
				"%s (position %d)",
				detail,
				postgresError.Position,
			)
		}
		hint := postgresError.Hint

		switch {
		case postgresError.Code == "25P02":
			if hint == "" {
				hint = "Rollback the current transaction before running another statement."
			}
			return serviceErrorWithCode[T](
				http.StatusConflict,
				errorCodeTransactionAborted,
				"Transaction is aborted",
				detail,
				hint,
			)
		case strings.HasPrefix(postgresError.Code, "23"):
			if hint == "" {
				hint = "Review primary keys, foreign keys, unique values, and required columns."
			}
			return serviceErrorWithCode[T](
				http.StatusConflict,
				errorCodeQueryConstraint,
				"Constraint violation",
				detail,
				hint,
			)
		case postgresError.Code == "42501":
			if hint == "" {
				hint = "Grant the required privilege or use a database role with access."
			}
			return serviceErrorWithCode[T](
				http.StatusForbidden,
				errorCodeQueryPermission,
				"Permission denied",
				detail,
				hint,
			)
		case strings.HasPrefix(postgresError.Code, "42"):
			if hint == "" {
				hint = "Check identifiers and SQL syntax near the reported position."
			}
			return serviceErrorWithCode[T](
				http.StatusBadRequest,
				errorCodeQuerySyntax,
				"Query syntax error",
				detail,
				hint,
			)
		}
	}

	hint := "Check the SQL statement and the active database connection."
	if inTransaction {
		hint = "The transaction may now be aborted; rollback before continuing."
	}
	return serviceErrorWithCode[T](
		http.StatusBadRequest,
		errorCodeQueryFailed,
		"Query failed",
		queryErr.Error(),
		hint,
	)
}
