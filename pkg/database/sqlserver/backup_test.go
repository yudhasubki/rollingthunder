package sqlserver

import (
	"strings"
	"testing"
)

func TestNormalizeSQLServerBackupPath(t *testing.T) {
	for _, path := range []string{
		`C:\Program Files\Microsoft SQL Server\Backup\rolling.bak`,
		`\\backup-host\database\rolling.BAK`,
		"/var/opt/mssql/backup/rolling.bak",
	} {
		t.Run(path, func(t *testing.T) {
			got, err := normalizeSQLServerBackupPath(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != path {
				t.Fatalf("path = %q", got)
			}
		})
	}
	for _, path := range []string{
		"",
		"rolling.bak",
		"/var/opt/mssql/backup/rolling.dump",
		"/var/opt/mssql/backup/*.bak",
		"/var/opt/mssql/backup/rolling.bak\nRESTORE",
	} {
		t.Run("reject "+path, func(t *testing.T) {
			if _, err := normalizeSQLServerBackupPath(path); err == nil {
				t.Fatalf("accepted unsafe path %q", path)
			}
		})
	}
}

func TestSQLServerBackupIdentityChangesWithReviewedMetadata(t *testing.T) {
	header := sqlServerBackupHeader{
		database:      "rolling",
		bytes:         42,
		position:      1,
		finishedAt:    "2026-07-26T10:00:00Z",
		backupSetID:   "set-id",
		checkpointLSN: "100",
		databaseLSN:   "90",
	}
	first := sqlServerBackupIdentity("/backup/rolling.bak", header)
	header.bytes++
	second := sqlServerBackupIdentity("/backup/rolling.bak", header)
	if first == second {
		t.Fatal("backup identity did not change with backup metadata")
	}
	if len(first) != 64 || strings.Trim(first, "0123456789abcdef") != "" {
		t.Fatalf("identity = %q", first)
	}
}

func TestSQLServerSupportsDiskBackup(t *testing.T) {
	for _, edition := range []int{5, 6, 8, 11} {
		if sqlServerSupportsDiskBackup(edition) {
			t.Fatalf("cloud engine edition %d accepted DISK backups", edition)
		}
	}
	if !sqlServerSupportsDiskBackup(3) {
		t.Fatal("standard SQL Server engine rejected DISK backups")
	}
}
