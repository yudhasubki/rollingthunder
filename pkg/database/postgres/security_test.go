package postgres

import (
	"context"
	"strings"
	"testing"

	"rollingthunder/pkg/database"
)

func TestPostgresSecurityPreviewRedactsPassword(t *testing.T) {
	driver := NewPostgres(context.Background(), Config{})
	plan, err := driver.BuildSecurityChange(
		context.Background(),
		database.SecurityChangeRequest{
			Action: database.SecurityCreatePrincipal,
			Principal: database.PrincipalOptions{
				Name:     `report"writer`,
				Kind:     database.PrincipalUser,
				Password: "s3cret'value",
				CanLogin: true,
				Inherit:  true,
			},
		},
	)
	if err != nil {
		t.Fatalf("BuildSecurityChange() error = %v", err)
	}
	if !strings.Contains(plan.Statements[0], `'s3cret''value'`) {
		t.Fatalf("execution statement = %s", plan.Statements[0])
	}
	if strings.Contains(plan.PreviewStatements[0], "s3cret") ||
		!strings.Contains(plan.PreviewStatements[0], "••••••") {
		t.Fatalf("preview statement = %s", plan.PreviewStatements[0])
	}
}

func TestPostgresSecurityLiteralUsesExplicitEscapeSyntax(t *testing.T) {
	value := `path\'; DROP ROLE root; --`
	if got, want := quotePostgresLiteral(value), `E'path\\''; DROP ROLE root; --'`; got != want {
		t.Fatalf("quoted literal = %q, want %q", got, want)
	}
}

func TestPostgresGrantBuilderQuotesScope(t *testing.T) {
	driver := NewPostgres(context.Background(), Config{})
	plan, err := driver.BuildSecurityChange(
		context.Background(),
		database.SecurityChangeRequest{
			Action: database.SecurityGrantPrivilege,
			Grant: database.GrantOptions{
				Grantee:    "analyst",
				ObjectType: "table",
				Schema:     "sales",
				Object:     "order items",
				Privilege:  "select",
			},
		},
	)
	if err != nil {
		t.Fatalf("BuildSecurityChange() error = %v", err)
	}
	if got := plan.Statements[0]; got !=
		`GRANT SELECT ON TABLE "sales"."order items" TO "analyst";` {
		t.Fatalf("grant statement = %s", got)
	}
}
