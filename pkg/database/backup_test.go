package database

import "testing"

func TestBackupRequestValidation(t *testing.T) {
	if err := (BackupRequest{}).Validate(); err != ErrBackupConnectionRequired {
		t.Fatalf("empty request error = %v", err)
	}
	if err := (BackupRequest{
		ConnectionID: "connection",
		SchemaOnly:   true,
		DataOnly:     true,
	}).Validate(); err != ErrBackupScopeConflict {
		t.Fatalf("conflicting scope error = %v", err)
	}
	if err := (BackupRequest{ConnectionID: "connection"}).Validate(); err != nil {
		t.Fatalf("valid backup request rejected: %v", err)
	}
}
