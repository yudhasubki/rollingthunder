package postgres

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

type postgresActivityRow struct {
	ID                 int64        `db:"session_id"`
	User               string       `db:"session_user"`
	Database           string       `db:"database_name"`
	Client             string       `db:"client_address"`
	Application        string       `db:"application_name"`
	State              string       `db:"session_state"`
	Query              string       `db:"query_text"`
	WaitEvent          string       `db:"wait_event"`
	BlockedBy          string       `db:"blocked_by"`
	DurationMS         int64        `db:"duration_ms"`
	TransactionStarted sql.NullTime `db:"transaction_started"`
	QueryStarted       sql.NullTime `db:"query_started"`
	StartedAt          sql.NullTime `db:"started_at"`
	IsCurrent          bool         `db:"is_current"`
}

func nullableActivityTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func splitBlockedSessions(value string) []string {
	parts := strings.Split(strings.TrimSpace(value), ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func (p *Postgres) GetDatabaseActivity(
	ctx context.Context,
) (database.DatabaseActivity, error) {
	if p.conn == nil {
		return database.DatabaseActivity{}, fmt.Errorf("PostgreSQL connection is not open")
	}
	const query = `
		SELECT
			pid::bigint AS session_id,
			COALESCE(usename, '') AS session_user,
			COALESCE(datname, '') AS database_name,
			COALESCE(client_addr::text, 'local') AS client_address,
			COALESCE(application_name, '') AS application_name,
			COALESCE(state, '') AS session_state,
			LEFT(COALESCE(query, ''), 4000) AS query_text,
			TRIM(BOTH ' ' FROM CONCAT_WS(':', wait_event_type, wait_event)) AS wait_event,
			COALESCE(array_to_string(pg_blocking_pids(pid), ','), '') AS blocked_by,
			GREATEST(
				0,
				(EXTRACT(EPOCH FROM (
					clock_timestamp() - COALESCE(query_start, backend_start)
				)) * 1000)::bigint
			) AS duration_ms,
			xact_start AS transaction_started,
			query_start AS query_started,
			backend_start AS started_at,
			(pid = pg_backend_pid() OR application_name = $1) AS is_current
		FROM pg_catalog.pg_stat_activity
		WHERE backend_type = 'client backend'
		ORDER BY
			CASE WHEN state = 'active' THEN 0 ELSE 1 END,
			query_start NULLS LAST,
			pid`
	var rows []postgresActivityRow
	if err := p.conn.SelectContext(
		ctx,
		&rows,
		query,
		application.DatabaseClientName,
	); err != nil {
		return database.DatabaseActivity{}, err
	}
	sessions := make([]database.DatabaseSession, 0, len(rows))
	currentID := ""
	for _, row := range rows {
		id := strconv.FormatInt(row.ID, 10)
		if row.IsCurrent && currentID == "" {
			currentID = id
		}
		blockedBy := splitBlockedSessions(row.BlockedBy)
		sessions = append(sessions, database.DatabaseSession{
			ID:                 id,
			User:               row.User,
			Database:           row.Database,
			Client:             row.Client,
			Application:        row.Application,
			Command:            row.State,
			State:              row.State,
			Query:              row.Query,
			WaitEvent:          row.WaitEvent,
			Waiting:            row.WaitEvent != "" || len(blockedBy) > 0,
			BlockedBy:          blockedBy,
			DurationMS:         row.DurationMS,
			TransactionStarted: nullableActivityTime(row.TransactionStarted),
			QueryStarted:       nullableActivityTime(row.QueryStarted),
			StartedAt:          nullableActivityTime(row.StartedAt),
			IsCurrent:          row.IsCurrent,
		})
	}
	return database.DatabaseActivity{
		Supported:           true,
		Engine:              "postgres",
		CurrentSessionID:    currentID,
		CanCancelQuery:      true,
		CanTerminateSession: true,
		Sessions:            sessions,
		CapturedAt:          time.Now(),
	}, nil
}

func (p *Postgres) CancelDatabaseSession(
	ctx context.Context,
	sessionID string,
	terminate bool,
) error {
	id, err := strconv.ParseInt(strings.TrimSpace(sessionID), 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("invalid PostgreSQL session ID %q", sessionID)
	}
	var protected bool
	if err := p.conn.GetContext(
		ctx,
		&protected,
		`SELECT pid = pg_backend_pid() OR application_name = $2
		 FROM pg_catalog.pg_stat_activity WHERE pid = $1`,
		id,
		application.DatabaseClientName,
	); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("PostgreSQL session %d no longer exists", id)
		}
		return err
	}
	if protected {
		return fmt.Errorf(
			"Rolling Thunder protects its own database sessions; cancel the query from its query tab",
		)
	}
	function := "pg_cancel_backend"
	if terminate {
		function = "pg_terminate_backend"
	}
	var cancelled bool
	if err := p.conn.GetContext(
		ctx,
		&cancelled,
		"SELECT "+function+"($1)",
		id,
	); err != nil {
		return err
	}
	if !cancelled {
		return fmt.Errorf(
			"PostgreSQL refused to %s session %d",
			map[bool]string{true: "terminate", false: "cancel"}[terminate],
			id,
		)
	}
	return nil
}

var _ database.ActivityDriver = (*Postgres)(nil)
