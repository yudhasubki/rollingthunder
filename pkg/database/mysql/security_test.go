package mysql

import (
	"context"
	"strings"
	"testing"

	"rollingthunder/pkg/database"
)

func TestMySQLSecurityPreviewRedactsPassword(t *testing.T) {
	driver := NewMySQL(context.Background(), Config{})
	plan, err := driver.BuildSecurityChange(
		context.Background(),
		database.SecurityChangeRequest{
			Action: database.SecurityCreatePrincipal,
			Principal: database.PrincipalOptions{
				Name:     "reporter",
				Host:     "10.%",
				Kind:     database.PrincipalUser,
				Password: "secret'value",
			},
		},
	)
	if err != nil {
		t.Fatalf("BuildSecurityChange() error = %v", err)
	}
	if !strings.Contains(plan.Statements[0], `'secret''value'`) {
		t.Fatalf("execution statement = %s", plan.Statements[0])
	}
	if strings.Contains(plan.PreviewStatements[0], "secret") {
		t.Fatalf("preview leaked password: %s", plan.PreviewStatements[0])
	}
}

func TestMySQLSecurityLiteralEscapesBackslashBeforeQuote(t *testing.T) {
	value := `path\'; DROP USER root; --`
	if got, want := quoteMySQLLiteral(value), `'path\\''; DROP USER root; --'`; got != want {
		t.Fatalf("quoted literal = %q, want %q", got, want)
	}
	account := mysqlAccount(`ops\'; DROP USER root; --`, `host\'; --`)
	if !strings.Contains(account, `\\''`) {
		t.Fatalf("quoted account = %s", account)
	}
}

func TestMySQLPrivilegeWhitelist(t *testing.T) {
	driver := NewMySQL(context.Background(), Config{})
	_, err := driver.BuildSecurityChange(
		context.Background(),
		database.SecurityChangeRequest{
			Action: database.SecurityGrantPrivilege,
			Grant: database.GrantOptions{
				Grantee:    "reporter",
				ObjectType: "global",
				Privilege:  "FILE; DROP USER root",
			},
		},
	)
	if err == nil {
		t.Fatal("unsafe privilege fragment should be rejected")
	}
}
