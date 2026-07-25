package db

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"rollingthunder/pkg/response"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	modernsqlite "modernc.org/sqlite"
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
	errorCodeSchemaMigrationFailed      = "SCHEMA_MIGRATION_FAILED"
	errorCodeSchemaMigrationReview      = "SCHEMA_MIGRATION_REVIEW_REQUIRED"
	errorCodeBackupUnavailable          = "BACKUP_UNAVAILABLE"
	errorCodeBackupFailed               = "BACKUP_FAILED"
	errorCodeRestoreFailed              = "RESTORE_FAILED"
	errorCodeRestoreReview              = "RESTORE_REVIEW_REQUIRED"
	errorCodeSecurityUnsupported        = "SECURITY_MANAGEMENT_UNSUPPORTED"
	errorCodeSecurityFailed             = "SECURITY_CHANGE_FAILED"
	errorCodeSecurityReview             = "SECURITY_CHANGE_REVIEW_REQUIRED"
	errorCodeActivityUnsupported        = "ACTIVITY_MONITOR_UNSUPPORTED"
	errorCodeActivityFailed             = "ACTIVITY_MONITOR_FAILED"
	errorCodeSessionCancellationFailed  = "SESSION_CANCELLATION_FAILED"
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

	var mysqlError *mysqldriver.MySQLError
	if errors.As(connectErr, &mysqlError) {
		switch mysqlError.Number {
		case 1045:
			return serviceErrorWithCode[T](
				http.StatusUnauthorized,
				errorCodeAuthenticationFailed,
				"Authentication failed",
				"MySQL or MariaDB rejected the username or password.",
				"Verify the username, password, authentication plugin, and allowed client host.",
			)
		case 1049:
			return serviceErrorWithCode[T](
				http.StatusNotFound,
				errorCodeDatabaseNotFound,
				"Database not found",
				"The requested MySQL or MariaDB database does not exist.",
				"Check the database name or connect without a default database first.",
			)
		case 1044:
			return serviceErrorWithCode[T](
				http.StatusForbidden,
				errorCodeAuthenticationFailed,
				"Database access denied",
				mysqlError.Message,
				"Grant access to the selected database or choose a database available to this account.",
			)
		}
	}

	var sqliteError *modernsqlite.Error
	if errors.As(connectErr, &sqliteError) {
		switch sqliteError.Code() & 0xff {
		case 5, 6:
			return serviceErrorWithCode[T](
				http.StatusLocked,
				errorCodeConnectionFailed,
				"SQLite database is locked",
				sqliteError.Error(),
				"Wait for the other writer to finish, then retry. Rolling Thunder waits up to five seconds before reporting a lock.",
			)
		case 8, 23:
			return serviceErrorWithCode[T](
				http.StatusForbidden,
				errorCodeConnectionFailed,
				"SQLite file is not writable",
				sqliteError.Error(),
				"Choose a writable database file and directory; WAL mode needs permission to create sidecar files.",
			)
		case 14:
			return serviceErrorWithCode[T](
				http.StatusNotFound,
				errorCodeDatabaseNotFound,
				"SQLite file could not be opened",
				sqliteError.Error(),
				"Check that the path exists or that its parent directory is writable for a new database.",
			)
		case 26:
			return serviceErrorWithCode[T](
				http.StatusBadRequest,
				errorCodeConnectionFailed,
				"File is not a SQLite database",
				sqliteError.Error(),
				"Choose a valid SQLite 3 database file.",
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
			"Check that the database server is running and listening on the configured host and port.",
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

	var mysqlError *mysqldriver.MySQLError
	if errors.As(queryErr, &mysqlError) {
		detail := mysqlError.Message
		switch mysqlError.Number {
		case 1062, 1048, 1451, 1452, 3819, 4025:
			return serviceErrorWithCode[T](
				http.StatusConflict,
				errorCodeQueryConstraint,
				"Constraint violation",
				detail,
				"Review primary keys, foreign keys, unique values, checks, and required columns.",
			)
		case 1044, 1142, 1143, 1227:
			return serviceErrorWithCode[T](
				http.StatusForbidden,
				errorCodeQueryPermission,
				"Permission denied",
				detail,
				"Grant the required privilege or use an account with access to this operation.",
			)
		case 1064, 1054, 1146, 1305:
			return serviceErrorWithCode[T](
				http.StatusBadRequest,
				errorCodeQuerySyntax,
				"Query or identifier error",
				detail,
				"Check SQL syntax, database-qualified names, and identifiers for the active MySQL dialect.",
			)
		case 1205, 1213:
			return serviceErrorWithCode[T](
				http.StatusConflict,
				errorCodeQueryFailed,
				"Lock conflict",
				detail,
				"Retry the transaction after competing work completes; keep transactions short.",
			)
		}
	}

	var sqliteError *modernsqlite.Error
	if errors.As(queryErr, &sqliteError) {
		switch sqliteError.Code() & 0xff {
		case 19:
			return serviceErrorWithCode[T](
				http.StatusConflict,
				errorCodeQueryConstraint,
				"Constraint violation",
				sqliteError.Error(),
				"Review primary keys, foreign keys, unique values, checks, and required columns.",
			)
		case 5, 6:
			return serviceErrorWithCode[T](
				http.StatusLocked,
				errorCodeQueryFailed,
				"SQLite database is locked",
				sqliteError.Error(),
				"Wait for the competing transaction to finish or rollback it, then retry.",
			)
		case 8, 23:
			return serviceErrorWithCode[T](
				http.StatusForbidden,
				errorCodeQueryPermission,
				"SQLite database is read-only",
				sqliteError.Error(),
				"Move the database to a writable location or run a read-only statement.",
			)
		case 1:
			return serviceErrorWithCode[T](
				http.StatusBadRequest,
				errorCodeQuerySyntax,
				"SQLite query error",
				sqliteError.Error(),
				"Check SQLite syntax and identifiers in the active main or attached database.",
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
