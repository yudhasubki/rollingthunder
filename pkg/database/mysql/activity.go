package mysql

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

type mysqlActivityRow struct {
	ID        uint64         `db:"session_id"`
	User      string         `db:"session_user"`
	Host      string         `db:"client_address"`
	Database  sql.NullString `db:"database_name"`
	Command   string         `db:"session_command"`
	Duration  int64          `db:"duration_seconds"`
	State     sql.NullString `db:"session_state"`
	Query     sql.NullString `db:"query_text"`
	IsCurrent bool           `db:"is_current"`
}

func mysqlActivityQuery(withAttributes bool) string {
	isCurrent := "process.ID = CONNECTION_ID()"
	if withAttributes {
		isCurrent = `(process.ID = CONNECTION_ID() OR EXISTS (
			SELECT 1
			FROM performance_schema.session_connect_attrs attribute
			WHERE attribute.PROCESSLIST_ID = process.ID
			  AND attribute.ATTR_NAME = 'program_name'
			  AND attribute.ATTR_VALUE = ?
		))`
	}
	return fmt.Sprintf(`
		SELECT
			process.ID AS session_id,
			process.USER AS session_user,
			process.HOST AS client_address,
			process.DB AS database_name,
			process.COMMAND AS session_command,
			process.TIME AS duration_seconds,
			process.STATE AS session_state,
			LEFT(process.INFO, 4000) AS query_text,
			%s AS is_current
		FROM information_schema.PROCESSLIST process
		ORDER BY
			CASE WHEN process.COMMAND = 'Sleep' THEN 1 ELSE 0 END,
			process.TIME DESC,
			process.ID`, isCurrent)
}

func (m *MySQL) GetDatabaseActivity(
	ctx context.Context,
) (database.DatabaseActivity, error) {
	if err := m.ensureConnected(); err != nil {
		return database.DatabaseActivity{}, err
	}
	var rows []mysqlActivityRow
	if err := m.conn.SelectContext(
		ctx,
		&rows,
		mysqlActivityQuery(true),
		application.DatabaseClientName,
	); err != nil {
		if fallbackErr := m.conn.SelectContext(
			ctx,
			&rows,
			mysqlActivityQuery(false),
		); fallbackErr != nil {
			return database.DatabaseActivity{}, fallbackErr
		}
	}
	sessions := make([]database.DatabaseSession, 0, len(rows))
	currentID := ""
	now := time.Now()
	for _, row := range rows {
		id := strconv.FormatUint(row.ID, 10)
		if row.IsCurrent && currentID == "" {
			currentID = id
		}
		state := strings.TrimSpace(row.State.String)
		if state == "" {
			state = strings.ToLower(strings.TrimSpace(row.Command))
		}
		lowerState := strings.ToLower(state)
		waiting := strings.Contains(lowerState, "lock") ||
			strings.Contains(lowerState, "wait")
		var queryStarted *time.Time
		if row.Duration > 0 && !strings.EqualFold(row.Command, "Sleep") {
			started := now.Add(-time.Duration(row.Duration) * time.Second)
			queryStarted = &started
		}
		sessions = append(sessions, database.DatabaseSession{
			ID:           id,
			User:         row.User,
			Database:     row.Database.String,
			Client:       row.Host,
			Command:      row.Command,
			State:        state,
			Query:        row.Query.String,
			WaitEvent:    row.State.String,
			Waiting:      waiting,
			BlockedBy:    []string{},
			DurationMS:   row.Duration * 1000,
			QueryStarted: queryStarted,
			IsCurrent:    row.IsCurrent,
		})
	}
	return database.DatabaseActivity{
		Supported:        true,
		Engine:           "mysql",
		CurrentSessionID: currentID,
		Sessions:         sessions,
		CapturedAt:       now,
	}, nil
}

func (m *MySQL) protectedActivitySession(
	ctx context.Context,
	id uint64,
) (bool, error) {
	var current uint64
	if err := m.conn.GetContext(ctx, &current, "SELECT CONNECTION_ID()"); err != nil {
		return false, err
	}
	if id == current {
		return true, nil
	}
	var protected int
	err := m.conn.GetContext(
		ctx,
		&protected,
		`SELECT COUNT(*)
		 FROM performance_schema.session_connect_attrs
		 WHERE PROCESSLIST_ID = ?
		   AND ATTR_NAME = 'program_name'
		   AND ATTR_VALUE = ?`,
		id,
		application.DatabaseClientName,
	)
	if err != nil {
		return false, fmt.Errorf(
			"could not verify whether session %d belongs to Rolling Thunder; refusing to stop it: %w",
			id,
			err,
		)
	}
	return protected > 0, nil
}

func (m *MySQL) CancelDatabaseSession(
	ctx context.Context,
	sessionID string,
	terminate bool,
) error {
	id, err := strconv.ParseUint(strings.TrimSpace(sessionID), 10, 64)
	if err != nil || id == 0 {
		return fmt.Errorf("invalid MySQL session ID %q", sessionID)
	}
	protected, err := m.protectedActivitySession(ctx, id)
	if err != nil {
		return err
	}
	if protected {
		return fmt.Errorf(
			"Rolling Thunder protects its own database sessions; cancel the query from its query tab",
		)
	}
	command := "KILL QUERY "
	if terminate {
		command = "KILL CONNECTION "
	}
	if _, err := m.conn.ExecContext(
		ctx,
		command+strconv.FormatUint(id, 10),
	); err != nil {
		return err
	}
	return nil
}

var _ database.ActivityDriver = (*MySQL)(nil)
