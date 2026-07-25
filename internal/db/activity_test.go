package db

import (
	"context"
	"testing"
	"time"

	"rollingthunder/pkg/database"
)

type activityTestDriver struct {
	routingTestDriver
	sessionID string
	terminate bool
}

func (driver *activityTestDriver) GetDatabaseActivity(
	context.Context,
) (database.DatabaseActivity, error) {
	return database.DatabaseActivity{
		Supported: true,
		Engine:    "test",
		Sessions: []database.DatabaseSession{{
			ID:        "42",
			User:      "analyst",
			State:     "active",
			BlockedBy: []string{},
		}},
		CapturedAt: time.Now(),
	}, nil
}

func (driver *activityTestDriver) CancelDatabaseSession(
	_ context.Context,
	sessionID string,
	terminate bool,
) error {
	driver.sessionID = sessionID
	driver.terminate = terminate
	return nil
}

func TestCancelDatabaseSessionRoutesConfirmedAction(t *testing.T) {
	driver := &activityTestDriver{}
	service := NewService()
	service.connections["activity"] = &Connection{
		ID:     "activity",
		Driver: driver,
		Config: database.Config{Driver: "test"},
	}
	unconfirmed := service.CancelDatabaseSession(database.CancelSessionRequest{
		ConnectionID: "activity",
		SessionID:    "42",
	})
	if len(unconfirmed.Errors) != 1 || driver.sessionID != "" {
		t.Fatalf("unconfirmed response = %+v", unconfirmed)
	}
	confirmed := service.CancelDatabaseSession(database.CancelSessionRequest{
		ConnectionID: "activity",
		SessionID:    "42",
		Terminate:    true,
		Confirmed:    true,
	})
	if len(confirmed.Errors) != 0 ||
		!confirmed.Data.Terminated ||
		driver.sessionID != "42" ||
		!driver.terminate {
		t.Fatalf("confirmed response = %+v", confirmed)
	}
}
