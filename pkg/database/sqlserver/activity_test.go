package sqlserver

import (
	"strings"
	"testing"
)

func TestSQLServerSessionActionRequiresTermination(t *testing.T) {
	if _, err := sqlServerSessionActionStatement("51", false); err == nil ||
		!strings.Contains(err.Error(), "cannot cancel only") {
		t.Fatalf("cancel-only action error = %v", err)
	}
	statement, err := sqlServerSessionActionStatement("51", true)
	if err != nil {
		t.Fatal(err)
	}
	if statement != "KILL 51" {
		t.Fatalf("statement = %q", statement)
	}
}

func TestSQLServerSessionActionRejectsInjectedIdentifier(t *testing.T) {
	for _, value := range []string{"", "0", "-1", "51; DROP DATABASE app", "1.5"} {
		if _, err := sqlServerSessionActionStatement(value, true); err == nil {
			t.Fatalf("accepted unsafe session ID %q", value)
		}
	}
}
