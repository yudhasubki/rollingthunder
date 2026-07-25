package database

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type DatabaseSession struct {
	ID                 string     `json:"id"`
	User               string     `json:"user"`
	Database           string     `json:"database,omitempty"`
	Client             string     `json:"client,omitempty"`
	Application        string     `json:"application,omitempty"`
	Command            string     `json:"command,omitempty"`
	State              string     `json:"state"`
	Query              string     `json:"query,omitempty"`
	WaitEvent          string     `json:"waitEvent,omitempty"`
	Waiting            bool       `json:"waiting"`
	BlockedBy          []string   `json:"blockedBy"`
	DurationMS         int64      `json:"durationMs"`
	TransactionStarted *time.Time `json:"transactionStarted,omitempty"`
	QueryStarted       *time.Time `json:"queryStarted,omitempty"`
	StartedAt          *time.Time `json:"startedAt,omitempty"`
	IsCurrent          bool       `json:"isCurrent"`
}

type DatabaseActivity struct {
	Supported           bool              `json:"supported"`
	Engine              string            `json:"engine"`
	CurrentSessionID    string            `json:"currentSessionId,omitempty"`
	CanCancelQuery      bool              `json:"canCancelQuery"`
	CanTerminateSession bool              `json:"canTerminateSession"`
	Sessions            []DatabaseSession `json:"sessions"`
	CapturedAt          time.Time         `json:"capturedAt"`
	Message             string            `json:"message,omitempty"`
}

type CancelSessionRequest struct {
	ConnectionID string `json:"connectionId"`
	SessionID    string `json:"sessionId"`
	Terminate    bool   `json:"terminate"`
	Confirmed    bool   `json:"confirmed"`
}

func (request CancelSessionRequest) Validate() error {
	if strings.TrimSpace(request.ConnectionID) == "" {
		return fmt.Errorf("connection is required")
	}
	if strings.TrimSpace(request.SessionID) == "" {
		return fmt.Errorf("session ID is required")
	}
	if !request.Confirmed {
		return fmt.Errorf("session cancellation must be explicitly confirmed")
	}
	return nil
}

type CancelSessionResult struct {
	Cancelled  bool   `json:"cancelled"`
	Terminated bool   `json:"terminated"`
	SessionID  string `json:"sessionId"`
}

type ActivityDriver interface {
	GetDatabaseActivity(ctx context.Context) (DatabaseActivity, error)
	CancelDatabaseSession(
		ctx context.Context,
		sessionID string,
		terminate bool,
	) error
}
