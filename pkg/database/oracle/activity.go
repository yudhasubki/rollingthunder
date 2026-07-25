package oracle

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

const oracleActivityQuery = `
	SELECT
		TO_CHAR(session_object.sid) || ',' ||
			TO_CHAR(session_object.serial#) AS session_id,
		NVL(session_object.username, '') AS session_user,
		NVL(session_object.service_name, '') AS database_name,
		NVL(session_object.machine, '') AS client_address,
		COALESCE(
			NULLIF(session_object.module, ''),
			NULLIF(session_object.program, ''),
			''
		) AS application_name,
		TO_CHAR(session_object.command) AS session_command,
		LOWER(NVL(session_object.status, 'unknown')) AS session_state,
		NVL(DBMS_LOB.SUBSTR(sql_object.sql_fulltext, 4000, 1), '') AS query_text,
		CASE
			WHEN session_object.wait_class = 'Idle' THEN ''
			ELSE NVL(session_object.event, '')
		END AS wait_event,
		CASE
			WHEN blocker.sid IS NULL THEN ''
			ELSE TO_CHAR(blocker.sid) || ',' || TO_CHAR(blocker.serial#)
		END AS blocked_by,
		CAST(
			CASE
				WHEN session_object.sql_exec_start IS NOT NULL THEN
					GREATEST(
						0,
						ROUND(
							(SYSDATE - session_object.sql_exec_start) *
							86400000
						)
					)
				ELSE GREATEST(0, session_object.last_call_et * 1000)
			END
			AS NUMBER(19, 0)
		) AS duration_ms,
		session_object.sql_exec_start AS query_started,
		session_object.logon_time AS started_at,
		CASE
			WHEN session_object.sid =
				TO_NUMBER(SYS_CONTEXT('USERENV', 'SID')) THEN 1
			WHEN INSTR(
				UPPER(
					NVL(session_object.program, '') || ' ' ||
					NVL(session_object.module, '') || ' ' ||
					NVL(session_object.client_identifier, '')
				),
				UPPER(:1)
			) > 0 THEN 1
			ELSE 0
		END AS is_current
	FROM v$session session_object
	LEFT JOIN v$sql sql_object
		ON sql_object.sql_id = session_object.sql_id
		AND sql_object.child_number = session_object.sql_child_number
	LEFT JOIN v$session blocker
		ON blocker.sid = session_object.blocking_session
	WHERE session_object.type = 'USER'
	ORDER BY
		CASE WHEN session_object.status = 'ACTIVE' THEN 0 ELSE 1 END,
		session_object.sql_exec_start NULLS LAST,
		session_object.sid`

type oracleActivityRow struct {
	ID           string
	User         string
	Database     string
	Client       string
	Application  string
	Command      string
	State        string
	Query        string
	WaitEvent    string
	BlockedBy    string
	DurationMS   int64
	QueryStarted sql.NullTime
	StartedAt    sql.NullTime
	IsCurrent    int
}

func oracleActivityTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func oracleBlockedBy(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return []string{}
	}
	return []string{value}
}

func (o *Oracle) GetDatabaseActivity(
	ctx context.Context,
) (database.DatabaseActivity, error) {
	if err := o.ensureConnected(); err != nil {
		return database.DatabaseActivity{}, err
	}
	rows, err := o.conn.QueryContext(
		ctx,
		oracleActivityQuery,
		application.DatabaseClientName,
	)
	if err != nil {
		return database.DatabaseActivity{}, err
	}
	defer rows.Close()

	sessions := make([]database.DatabaseSession, 0)
	currentID := ""
	for rows.Next() {
		var row oracleActivityRow
		if err := rows.Scan(
			&row.ID,
			&row.User,
			&row.Database,
			&row.Client,
			&row.Application,
			&row.Command,
			&row.State,
			&row.Query,
			&row.WaitEvent,
			&row.BlockedBy,
			&row.DurationMS,
			&row.QueryStarted,
			&row.StartedAt,
			&row.IsCurrent,
		); err != nil {
			return database.DatabaseActivity{}, err
		}
		blockedBy := oracleBlockedBy(row.BlockedBy)
		isCurrent := row.IsCurrent != 0
		if isCurrent && currentID == "" {
			currentID = row.ID
		}
		sessions = append(sessions, database.DatabaseSession{
			ID:           row.ID,
			User:         row.User,
			Database:     row.Database,
			Client:       row.Client,
			Application:  row.Application,
			Command:      row.Command,
			State:        row.State,
			Query:        row.Query,
			WaitEvent:    row.WaitEvent,
			Waiting:      row.WaitEvent != "" || len(blockedBy) > 0,
			BlockedBy:    blockedBy,
			DurationMS:   row.DurationMS,
			QueryStarted: oracleActivityTime(row.QueryStarted),
			StartedAt:    oracleActivityTime(row.StartedAt),
			IsCurrent:    isCurrent,
		})
	}
	if err := rows.Err(); err != nil {
		return database.DatabaseActivity{}, err
	}
	return database.DatabaseActivity{
		Supported:        true,
		Engine:           database.DriverOracle,
		CurrentSessionID: currentID,
		Sessions:         sessions,
		CapturedAt:       time.Now(),
	}, nil
}

func parseOracleSessionID(value string) (int64, int64, error) {
	parts := strings.Split(strings.TrimSpace(value), ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf(
			"invalid Oracle session ID %q; expected SID,SERIAL#",
			value,
		)
	}
	sid, sidErr := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	serial, serialErr := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if sidErr != nil || serialErr != nil || sid <= 0 || serial <= 0 {
		return 0, 0, fmt.Errorf("invalid Oracle session ID %q", value)
	}
	return sid, serial, nil
}

func oracleSessionActionStatement(
	sessionID string,
	terminate bool,
) (string, error) {
	sid, serial, err := parseOracleSessionID(sessionID)
	if err != nil {
		return "", err
	}
	reference := strconv.FormatInt(sid, 10) + "," +
		strconv.FormatInt(serial, 10)
	if terminate {
		return "ALTER SYSTEM KILL SESSION '" + reference + "' IMMEDIATE", nil
	}
	return "ALTER SYSTEM CANCEL SQL '" + reference + "'", nil
}

func (o *Oracle) CancelDatabaseSession(
	ctx context.Context,
	sessionID string,
	terminate bool,
) error {
	if err := o.ensureConnected(); err != nil {
		return err
	}
	sid, serial, err := parseOracleSessionID(sessionID)
	if err != nil {
		return err
	}
	var total, protected int
	if err := o.conn.QueryRowContext(
		ctx,
		`SELECT
			COUNT(*),
			NVL(SUM(
				CASE
					WHEN sid = TO_NUMBER(SYS_CONTEXT('USERENV', 'SID')) THEN 1
					WHEN INSTR(
						UPPER(
							NVL(program, '') || ' ' ||
							NVL(module, '') || ' ' ||
							NVL(client_identifier, '')
						),
						UPPER(:application)
					) > 0 THEN 1
					ELSE 0
				END
			), 0)
		 FROM v$session
		 WHERE sid = :sid AND serial# = :serial`,
		sql.Named("sid", sid),
		sql.Named("serial", serial),
		sql.Named("application", application.DatabaseClientName),
	).Scan(&total, &protected); err != nil {
		return err
	}
	if total == 0 {
		return fmt.Errorf("Oracle session %d,%d no longer exists", sid, serial)
	}
	if protected > 0 {
		return fmt.Errorf(
			"Rolling Thunder protects its own database sessions; cancel the query from its query tab",
		)
	}
	statement, err := oracleSessionActionStatement(sessionID, terminate)
	if err != nil {
		return err
	}
	if _, err := o.conn.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf(
			"Oracle refused to %s session %d,%d: %w",
			map[bool]string{true: "terminate", false: "cancel SQL for"}[terminate],
			sid,
			serial,
			err,
		)
	}
	return nil
}

var _ database.ActivityDriver = (*Oracle)(nil)
