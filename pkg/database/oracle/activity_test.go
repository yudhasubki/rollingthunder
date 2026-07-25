package oracle

import (
	"strings"
	"testing"
)

func TestOracleActivityQueryProtectsApplicationSessions(t *testing.T) {
	normalized := strings.ToUpper(oracleActivityQuery)
	for _, expected := range []string{
		"FROM V$SESSION",
		"BLOCKING_SESSION",
		"SQL_EXEC_START",
		"CLIENT_IDENTIFIER",
		"SYS_CONTEXT('USERENV', 'SID')",
	} {
		if !strings.Contains(normalized, expected) {
			t.Fatalf("activity query is missing %q", expected)
		}
	}
}

func TestOracleSessionActionStatements(t *testing.T) {
	cancel, err := oracleSessionActionStatement("42, 701", false)
	if err != nil {
		t.Fatal(err)
	}
	if cancel != "ALTER SYSTEM CANCEL SQL '42,701'" {
		t.Fatalf("cancel statement = %q", cancel)
	}
	terminate, err := oracleSessionActionStatement("42,701", true)
	if err != nil {
		t.Fatal(err)
	}
	if terminate != "ALTER SYSTEM KILL SESSION '42,701' IMMEDIATE" {
		t.Fatalf("terminate statement = %q", terminate)
	}
}

func TestOracleSessionActionRejectsUntrustedIdentifiers(t *testing.T) {
	for _, value := range []string{
		"",
		"42",
		"42,0",
		"0,701",
		"42,701,@2",
		"42,701' IMMEDIATE; DROP USER APP",
	} {
		if _, err := oracleSessionActionStatement(value, true); err == nil {
			t.Fatalf("session ID %q was accepted", value)
		}
	}
}
