package database

import "testing"

func TestCancelSessionRequiresExplicitConfirmation(t *testing.T) {
	request := CancelSessionRequest{
		ConnectionID: "connection",
		SessionID:    "42",
	}
	if err := request.Validate(); err == nil {
		t.Fatal("unconfirmed session action should be rejected")
	}
	request.Confirmed = true
	if err := request.Validate(); err != nil {
		t.Fatalf("confirmed session action rejected: %v", err)
	}
}
