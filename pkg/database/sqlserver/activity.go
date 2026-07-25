package sqlserver

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"rollingthunder/pkg/application"
	"rollingthunder/pkg/database"
)

const sqlServerActivityQuery = `
	SELECT
		session_value.session_id,
		COALESCE(session_value.login_name, N'') AS session_user,
		COALESCE(
			DB_NAME(COALESCE(request_value.database_id, session_value.database_id)),
			N''
		) AS database_name,
		COALESCE(session_value.host_name, N'') AS client_address,
		COALESCE(
			NULLIF(session_value.program_name, N''),
			NULLIF(session_value.client_interface_name, N''),
			N''
		) AS application_name,
		COALESCE(request_value.command, N'') AS session_command,
		LOWER(
			COALESCE(
				NULLIF(request_value.status, N''),
				NULLIF(session_value.status, N''),
				N'unknown'
			)
		) AS session_state,
		LEFT(COALESCE(statement_value.text, N''), 4000) AS query_text,
		COALESCE(request_value.wait_type, N'') AS wait_event,
		CASE
			WHEN request_value.blocking_session_id > 0 THEN
				CONVERT(nvarchar(20), request_value.blocking_session_id)
			ELSE N''
		END AS blocked_by,
		CONVERT(
			bigint,
			CASE
				WHEN request_value.start_time IS NOT NULL THEN
					CASE
						WHEN DATEDIFF_BIG(
							millisecond,
							request_value.start_time,
							SYSDATETIME()
						) < 0 THEN 0
						ELSE DATEDIFF_BIG(
							millisecond,
							request_value.start_time,
							SYSDATETIME()
						)
					END
				ELSE 0
			END
		) AS duration_ms,
		request_value.start_time AS query_started,
		session_value.login_time AS started_at,
		CONVERT(
			bit,
			CASE
				WHEN session_value.session_id = @@SPID THEN 1
				WHEN session_value.program_name = @p1 THEN 1
				ELSE 0
			END
		) AS is_current
	FROM sys.dm_exec_sessions session_value
	LEFT JOIN sys.dm_exec_requests request_value
		ON request_value.session_id = session_value.session_id
	OUTER APPLY sys.dm_exec_sql_text(request_value.sql_handle) statement_value
	WHERE session_value.is_user_process = 1
	ORDER BY
		CASE WHEN request_value.session_id IS NULL THEN 1 ELSE 0 END,
		request_value.start_time,
		session_value.session_id`

const sqlServerTransactionStartsQuery = `
	SELECT
		st.session_id,
		MIN(atx.transaction_begin_time)
	FROM sys.dm_tran_session_transactions st
	JOIN sys.dm_tran_active_transactions atx
		ON atx.transaction_id = st.transaction_id
	GROUP BY st.session_id`

type sqlServerActivityRow struct {
	id           int64
	user         sql.NullString
	database     sql.NullString
	client       sql.NullString
	application  sql.NullString
	command      sql.NullString
	state        sql.NullString
	query        sql.NullString
	waitEvent    sql.NullString
	blockedBy    sql.NullString
	durationMS   int64
	queryStarted sql.NullTime
	startedAt    sql.NullTime
	isCurrent    bool
}

func sqlServerActivityText(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func sqlServerActivityTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func sqlServerBlockedBy(value sql.NullString) []string {
	text := strings.TrimSpace(sqlServerActivityText(value))
	if text == "" {
		return []string{}
	}
	return []string{text}
}

func (s *SQLServer) sqlServerTransactionStarts(
	ctx context.Context,
) (map[int64]time.Time, error) {
	rows, err := s.conn.QueryContext(ctx, sqlServerTransactionStartsQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	starts := make(map[int64]time.Time)
	for rows.Next() {
		var sessionID int64
		var started sql.NullTime
		if err := rows.Scan(&sessionID, &started); err != nil {
			return nil, err
		}
		if started.Valid {
			starts[sessionID] = started.Time
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return starts, nil
}

func (s *SQLServer) GetDatabaseActivity(
	ctx context.Context,
) (database.DatabaseActivity, error) {
	if err := s.ensureConnected(); err != nil {
		return database.DatabaseActivity{}, err
	}
	transactionStarts, transactionStartsErr := s.sqlServerTransactionStarts(ctx)
	rows, err := s.conn.QueryContext(
		ctx,
		sqlServerActivityQuery,
		application.DatabaseClientName,
	)
	if err != nil {
		return database.DatabaseActivity{}, fmt.Errorf(
			"read SQL Server sessions (VIEW SERVER STATE or VIEW SERVER PERFORMANCE STATE may be required): %w",
			err,
		)
	}
	defer rows.Close()

	sessions := make([]database.DatabaseSession, 0)
	currentID := ""
	for rows.Next() {
		var row sqlServerActivityRow
		if err := rows.Scan(
			&row.id,
			&row.user,
			&row.database,
			&row.client,
			&row.application,
			&row.command,
			&row.state,
			&row.query,
			&row.waitEvent,
			&row.blockedBy,
			&row.durationMS,
			&row.queryStarted,
			&row.startedAt,
			&row.isCurrent,
		); err != nil {
			return database.DatabaseActivity{}, err
		}
		id := strconv.FormatInt(row.id, 10)
		blockedBy := sqlServerBlockedBy(row.blockedBy)
		waitEvent := sqlServerActivityText(row.waitEvent)
		var transactionStarted *time.Time
		if started, exists := transactionStarts[row.id]; exists {
			transactionStarted = &started
		}
		if row.isCurrent && currentID == "" {
			currentID = id
		}
		sessions = append(sessions, database.DatabaseSession{
			ID:                 id,
			User:               sqlServerActivityText(row.user),
			Database:           sqlServerActivityText(row.database),
			Client:             sqlServerActivityText(row.client),
			Application:        sqlServerActivityText(row.application),
			Command:            sqlServerActivityText(row.command),
			State:              sqlServerActivityText(row.state),
			Query:              sqlServerActivityText(row.query),
			WaitEvent:          waitEvent,
			Waiting:            waitEvent != "" || len(blockedBy) > 0,
			BlockedBy:          blockedBy,
			DurationMS:         row.durationMS,
			TransactionStarted: transactionStarted,
			QueryStarted:       sqlServerActivityTime(row.queryStarted),
			StartedAt:          sqlServerActivityTime(row.startedAt),
			IsCurrent:          row.isCurrent,
		})
	}
	if err := rows.Err(); err != nil {
		return database.DatabaseActivity{}, err
	}
	message := "SQL Server can terminate a session with KILL, but it cannot " +
		"cancel only the current statement through a separate monitoring session."
	if transactionStartsErr != nil {
		message += " Transaction start timestamps are unavailable for this account."
	}
	return database.DatabaseActivity{
		Supported:           true,
		Engine:              database.DriverSQLServer,
		CurrentSessionID:    currentID,
		CanCancelQuery:      false,
		CanTerminateSession: true,
		Sessions:            sessions,
		CapturedAt:          time.Now(),
		Message:             message,
	}, nil
}

func parseSQLServerSessionID(value string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid SQL Server session ID %q", value)
	}
	return id, nil
}

func sqlServerSessionActionStatement(
	sessionID string,
	terminate bool,
) (string, error) {
	id, err := parseSQLServerSessionID(sessionID)
	if err != nil {
		return "", err
	}
	if !terminate {
		return "", fmt.Errorf(
			"SQL Server cannot cancel only the current statement from another session; terminate session %d or cancel the query from its client",
			id,
		)
	}
	return "KILL " + strconv.FormatInt(id, 10), nil
}

func (s *SQLServer) CancelDatabaseSession(
	ctx context.Context,
	sessionID string,
	terminate bool,
) error {
	if err := s.ensureConnected(); err != nil {
		return err
	}
	id, err := parseSQLServerSessionID(sessionID)
	if err != nil {
		return err
	}
	var total, protected int
	if err := s.conn.QueryRowContext(
		ctx,
		`SELECT
			COUNT(*),
			COALESCE(SUM(
				CASE
					WHEN session_id = @@SPID THEN 1
					WHEN program_name = @p2 THEN 1
					ELSE 0
				END
			), 0)
		 FROM sys.dm_exec_sessions
		 WHERE session_id = @p1`,
		id,
		application.DatabaseClientName,
	).Scan(&total, &protected); err != nil {
		return fmt.Errorf("verify SQL Server session %d: %w", id, err)
	}
	if total == 0 {
		return fmt.Errorf("SQL Server session %d no longer exists", id)
	}
	if protected > 0 {
		return fmt.Errorf(
			"Rolling Thunder protects its own database sessions; cancel the query from its query tab",
		)
	}
	statement, err := sqlServerSessionActionStatement(sessionID, terminate)
	if err != nil {
		return err
	}
	if _, err := s.conn.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("SQL Server refused to terminate session %d: %w", id, err)
	}
	return nil
}

var _ database.ActivityDriver = (*SQLServer)(nil)
