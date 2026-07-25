package sqlserver

import (
	"context"
	"strings"
	"testing"

	"rollingthunder/pkg/database"
)

func TestSQLServerCreateLoginPreviewRedactsPassword(t *testing.T) {
	driver := NewSQLServer(context.Background(), Config{})
	plan, err := driver.BuildSecurityChange(
		context.Background(),
		database.SecurityChangeRequest{
			Action: database.SecurityCreatePrincipal,
			Principal: database.PrincipalOptions{
				Name:     "app]login",
				Kind:     database.PrincipalLogin,
				Password: "secret'value",
				CanLogin: true,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plan.PreviewStatements[0], "secret") ||
		!strings.Contains(plan.PreviewStatements[0], "••••••") {
		t.Fatalf("preview leaked password: %q", plan.PreviewStatements[0])
	}
	if !strings.Contains(
		plan.Statements[0],
		"CREATE LOGIN [app]]login] WITH PASSWORD = N'secret''value'",
	) {
		t.Fatalf("execution statement = %q", plan.Statements[0])
	}
	if plan.Transactional || len(plan.Warnings) == 0 {
		t.Fatalf("server login plan did not disclose non-transactional apply: %+v", plan)
	}
}

func TestSQLServerCreateMappedUser(t *testing.T) {
	driver := NewSQLServer(context.Background(), Config{})
	plan, err := driver.BuildSecurityChange(
		context.Background(),
		database.SecurityChangeRequest{
			Action: database.SecurityCreatePrincipal,
			Principal: database.PrincipalOptions{
				Name:     "application",
				Kind:     database.PrincipalUser,
				Login:    "application_login",
				CanLogin: true,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Statements[0]; got !=
		"CREATE USER [application] FOR LOGIN [application_login];" {
		t.Fatalf("statement = %q", got)
	}
}

func TestSQLServerPrivilegeWhitelist(t *testing.T) {
	privilege, target, err := sqlServerGrantTarget(database.GrantOptions{
		ObjectType: "table",
		Schema:     "sales",
		Object:     "orders",
		Privilege:  "select",
	})
	if err != nil {
		t.Fatal(err)
	}
	if privilege != "SELECT" ||
		target != "OBJECT::[sales].[orders]" {
		t.Fatalf("grant target = %q on %q", privilege, target)
	}
	if _, _, err := sqlServerGrantTarget(database.GrantOptions{
		ObjectType: "table",
		Schema:     "dbo",
		Object:     "orders",
		Privilege:  "SELECT; DROP DATABASE app",
	}); err == nil {
		t.Fatal("accepted an injected privilege")
	}
}

func TestSQLServerDropLoginIsReviewedAndDestructive(t *testing.T) {
	driver := NewSQLServer(context.Background(), Config{})
	plan, err := driver.BuildSecurityChange(
		context.Background(),
		database.SecurityChangeRequest{
			Action: database.SecurityDropPrincipal,
			Principal: database.PrincipalOptions{
				Name: "legacy",
				Kind: database.PrincipalLogin,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Destructive ||
		plan.Statements[0] != "DROP LOGIN [legacy];" ||
		len(plan.Warnings) == 0 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestSQLServerServerRoleUsesExplicitScope(t *testing.T) {
	driver := NewSQLServer(context.Background(), Config{})
	plan, err := driver.BuildSecurityChange(
		context.Background(),
		database.SecurityChangeRequest{
			Action: database.SecurityCreatePrincipal,
			Principal: database.PrincipalOptions{
				Name:  "operations",
				Kind:  database.PrincipalRole,
				Scope: "server",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Statements[0]; got !=
		"CREATE SERVER ROLE [operations];" {
		t.Fatalf("statement = %q", got)
	}
	if plan.Transactional {
		t.Fatalf("server role plan unexpectedly claims transactional apply: %+v", plan)
	}
	if _, err := driver.BuildSecurityChange(
		context.Background(),
		database.SecurityChangeRequest{
			Action: database.SecurityDropPrincipal,
			Principal: database.PrincipalOptions{
				Name:  "sysadmin",
				Kind:  database.PrincipalRole,
				Scope: "server",
			},
		},
	); err == nil {
		t.Fatal("accepted dropping a fixed SQL Server role")
	}
}

func TestSQLServerPrincipalScopeRejectsMismatchedKinds(t *testing.T) {
	if _, err := sqlServerPrincipalScope(database.PrincipalOptions{
		Kind:  database.PrincipalLogin,
		Scope: "database",
	}); err == nil {
		t.Fatal("accepted a database-scoped SQL Server login")
	}
	if scope, err := sqlServerPrincipalScope(database.PrincipalOptions{
		Kind: database.PrincipalRole,
	}); err != nil || scope != "database" {
		t.Fatalf("default role scope = %q, %v", scope, err)
	}
}

func TestSQLServerGrantScopeSelector(t *testing.T) {
	server, databaseScope, err := sqlServerGrantScopes("server")
	if err != nil || !server || databaseScope {
		t.Fatalf("server scopes = %v, %v, %v", server, databaseScope, err)
	}
	server, databaseScope, err = sqlServerGrantScopes("database")
	if err != nil || server || !databaseScope {
		t.Fatalf("database scopes = %v, %v, %v", server, databaseScope, err)
	}
	server, databaseScope, err = sqlServerGrantScopes("")
	if err != nil || !server || !databaseScope {
		t.Fatalf("default scopes = %v, %v, %v", server, databaseScope, err)
	}
	if _, _, err := sqlServerGrantScopes("instance"); err == nil {
		t.Fatal("accepted an unsupported SQL Server grant scope")
	}
}
