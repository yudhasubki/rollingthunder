package database

import "testing"

func TestSecurityChangeRequestValidation(t *testing.T) {
	request := SecurityChangeRequest{
		Action: SecurityCreatePrincipal,
		Principal: PrincipalOptions{
			Name: "reporter",
			Kind: PrincipalUser,
		},
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("valid security request rejected: %v", err)
	}
	request.Principal.Name = ""
	if err := request.Validate(); err == nil {
		t.Fatal("missing principal name should be rejected")
	}
}

func TestSecurityPlanRequiresRedactedPreviewForEveryStatement(t *testing.T) {
	plan := SecurityChangePlan{
		Summary:    "Create user",
		Statements: []string{"CREATE USER reporter;"},
	}
	if err := plan.Validate(); err == nil {
		t.Fatal("missing preview statement should be rejected")
	}
}
