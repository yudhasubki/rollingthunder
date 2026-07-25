package db

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rollingthunder/pkg/database"
)

func executableFixture(available ...string) executableLookup {
	set := make(map[string]struct{}, len(available))
	for _, name := range available {
		set[name] = struct{}{}
	}
	return func(name string) (string, error) {
		if _, exists := set[name]; exists {
			return filepath.Join("/tools", name), nil
		}
		return "", errors.New("not found")
	}
}

func TestBackupCapabilitiesReportToolReadiness(t *testing.T) {
	postgres := backupCapabilitiesFor(
		executableFixture("pg_dump"),
		"postgres",
	)
	if !postgres.Available || postgres.RestoreReady ||
		!strings.Contains(postgres.Message, "pg_restore") {
		t.Fatalf("postgres capabilities = %+v", postgres)
	}

	mysql := backupCapabilitiesFor(
		executableFixture("mariadb-dump", "mariadb"),
		"mysql",
	)
	if !mysql.Available || !mysql.RestoreReady ||
		mysql.Format != database.BackupFormatMySQLSQL {
		t.Fatalf("mysql capabilities = %+v", mysql)
	}

	sqlite := backupCapabilitiesFor(executableFixture(), "sqlite")
	if !sqlite.Available || !sqlite.BuiltIn || !sqlite.RestoreReady {
		t.Fatalf("sqlite capabilities = %+v", sqlite)
	}

	oracle := backupCapabilitiesFor(executableFixture(), "oracle")
	if !oracle.Available ||
		!oracle.BuiltIn ||
		!oracle.RestoreReady ||
		oracle.Format != database.BackupFormatOracleDataPump ||
		oracle.Extension != ".dmp" ||
		oracle.SupportsScope ||
		!oracle.RequiresDirectory {
		t.Fatalf("oracle capabilities = %+v", oracle)
	}
}

func TestMySQLDefaultsFileUsesPrivatePermissionsAndEscaping(t *testing.T) {
	path, err := writeMySQLDefaults(database.Config{
		Host:     "db.internal",
		Port:     "3306",
		User:     "rolling",
		Password: "line one\n\"secret\"",
		SSLMode:  "verify-full",
	})
	if err != nil {
		t.Fatalf("write defaults: %v", err)
	}
	defer os.Remove(path)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat defaults: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("defaults permissions = %o", info.Mode().Perm())
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read defaults: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, `password="line one\n\"secret\""`) {
		t.Fatalf("password was not safely escaped: %q", text)
	}
	if !strings.Contains(text, `ssl-mode="VERIFY_IDENTITY"`) {
		t.Fatalf("TLS mode was not written for the native client: %q", text)
	}
}

func TestPostgresMaintenancePreservesTLSNameThroughTunnel(t *testing.T) {
	config := database.Config{
		Host:          "127.0.0.1",
		Port:          "41001",
		User:          "rolling",
		TLSServerName: "database.internal",
	}
	args := postgresConnectionArguments(config)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--host=database.internal") ||
		!strings.Contains(joined, "--port=41001") {
		t.Fatalf("PostgreSQL client arguments = %q", joined)
	}
	environment := commandEnvironment(config)
	found := false
	for _, value := range environment {
		if value == "PGHOSTADDR=127.0.0.1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("PGHOSTADDR missing from maintenance environment")
	}
}

func TestPostgresMaintenanceUsesPrivatePasswordFile(t *testing.T) {
	t.Setenv("PGPASSWORD", "inherited-secret")
	environment, cleanup, err := postgresCommandEnvironment(database.Config{
		Host:          "127.0.0.1",
		Port:          "41001",
		Db:            "rolling",
		User:          "operator",
		Password:      `pa:ss\word`,
		TLSServerName: "database.internal",
	})
	if err != nil {
		t.Fatalf("create PostgreSQL environment: %v", err)
	}
	var passwordFile string
	for _, value := range environment {
		if strings.HasPrefix(value, "PGPASSWORD=") {
			t.Fatalf("password leaked into child environment")
		}
		if strings.HasPrefix(value, "PGPASSFILE=") {
			passwordFile = strings.TrimPrefix(value, "PGPASSFILE=")
		}
	}
	if passwordFile == "" {
		t.Fatal("PGPASSFILE missing from child environment")
	}
	info, err := os.Stat(passwordFile)
	if err != nil {
		t.Fatalf("stat password file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("password-file permissions = %o", info.Mode().Perm())
	}
	content, err := os.ReadFile(passwordFile)
	if err != nil {
		t.Fatalf("read password file: %v", err)
	}
	if got, want := string(content), "database.internal:41001:rolling:operator:pa\\:ss\\\\word\n"; got != want {
		t.Fatalf("password-file content = %q, want %q", got, want)
	}
	cleanup()
	if _, err := os.Stat(passwordFile); !os.IsNotExist(err) {
		t.Fatalf("password file still exists after cleanup: %v", err)
	}
}

func TestPostgresPasswordFileRejectsLineBreaks(t *testing.T) {
	_, cleanup, err := postgresCommandEnvironment(database.Config{
		Password: "line one\nline two",
	})
	if cleanup != nil {
		cleanup()
	}
	if err == nil {
		t.Fatal("expected password-file line break validation")
	}
}
