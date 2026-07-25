package oracle

import (
	"context"
	"strings"
	"testing"

	"rollingthunder/pkg/database"
)

func TestOracleSecurityOverviewUsesCatalogGrants(t *testing.T) {
	for _, expected := range []string{
		"DBA_USERS",
		"DBA_ROLES",
		"DBA_ROLE_PRIVS",
		"DBA_SYS_PRIVS",
	} {
		if !strings.Contains(strings.ToUpper(oraclePrincipalsQuery), expected) {
			t.Fatalf("principal query is missing %q", expected)
		}
	}
	for _, expected := range []string{
		"DBA_ROLE_PRIVS",
		"DBA_SYS_PRIVS",
		"DBA_TAB_PRIVS",
	} {
		if !strings.Contains(strings.ToUpper(oracleGrantsQuery), expected) {
			t.Fatalf("grant query is missing %q", expected)
		}
	}
}

func TestOracleSecurityPlansRedactPasswords(t *testing.T) {
	driver := &Oracle{}
	plan, err := driver.BuildSecurityChange(
		context.Background(),
		database.SecurityChangeRequest{
			Action: database.SecurityCreatePrincipal,
			Principal: database.PrincipalOptions{
				Name:     "report_user",
				Kind:     database.PrincipalUser,
				Password: `Thunder"Secret`,
				CanLogin: true,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Transactional || len(plan.Statements) != 1 ||
		len(plan.PreviewStatements) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Statements[0] !=
		`CREATE USER "REPORT_USER" IDENTIFIED BY "Thunder""Secret";` {
		t.Fatalf("execution SQL = %q", plan.Statements[0])
	}
	if strings.Contains(plan.PreviewStatements[0], "Thunder") ||
		!strings.Contains(plan.PreviewStatements[0], "••••••") {
		t.Fatalf("preview SQL = %q", plan.PreviewStatements[0])
	}
}

func TestOracleSecurityRoleAndPrivilegePlans(t *testing.T) {
	driver := &Oracle{}
	rolePlan, err := driver.BuildSecurityChange(
		context.Background(),
		database.SecurityChangeRequest{
			Action: database.SecurityGrantRole,
			Grant: database.GrantOptions{
				Role:      "report_reader",
				Grantee:   "report_user",
				Grantable: true,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if rolePlan.Statements[0] !=
		`GRANT "REPORT_READER" TO "REPORT_USER" WITH ADMIN OPTION;` {
		t.Fatalf("role SQL = %q", rolePlan.Statements[0])
	}

	privilegePlan, err := driver.BuildSecurityChange(
		context.Background(),
		database.SecurityChangeRequest{
			Action: database.SecurityGrantPrivilege,
			Grant: database.GrantOptions{
				Grantee:    "report_user",
				ObjectType: "table",
				Schema:     "APP",
				Object:     "ORDERS",
				Privilege:  "select",
				Grantable:  true,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if privilegePlan.Statements[0] !=
		`GRANT SELECT ON "APP"."ORDERS" TO "REPORT_USER" WITH GRANT OPTION;` {
		t.Fatalf("object privilege SQL = %q", privilegePlan.Statements[0])
	}

	systemPlan, err := driver.BuildSecurityChange(
		context.Background(),
		database.SecurityChangeRequest{
			Action: database.SecurityGrantPrivilege,
			Grant: database.GrantOptions{
				Grantee:    "report_user",
				ObjectType: "system",
				Privilege:  "create session",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if systemPlan.Statements[0] !=
		`GRANT CREATE SESSION TO "REPORT_USER";` {
		t.Fatalf("system privilege SQL = %q", systemPlan.Statements[0])
	}
}

func TestOracleSecurityRejectsUnsafeNamesAndPrivileges(t *testing.T) {
	driver := &Oracle{}
	for _, request := range []database.SecurityChangeRequest{
		{
			Action: database.SecurityCreatePrincipal,
			Principal: database.PrincipalOptions{
				Name: "unsafe\nname", Kind: database.PrincipalRole,
			},
		},
		{
			Action: database.SecurityGrantPrivilege,
			Grant: database.GrantOptions{
				Grantee: "APP", ObjectType: "table",
				Schema: "APP", Object: "ORDERS", Privilege: "DROP USER",
			},
		},
	} {
		if _, err := driver.BuildSecurityChange(
			context.Background(),
			request,
		); err == nil {
			t.Fatalf("unsafe request was accepted: %+v", request)
		}
	}
}
