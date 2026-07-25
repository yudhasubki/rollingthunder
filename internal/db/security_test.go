package db

import (
	"context"
	"strings"
	"testing"

	"rollingthunder/pkg/database"
)

type securityTestDriver struct {
	routingTestDriver
	plan    database.SecurityChangePlan
	applied database.SecurityChangePlan
}

func (driver *securityTestDriver) GetSecurityOverview(
	context.Context,
	string,
	string,
) (database.SecurityOverview, error) {
	return database.SecurityOverview{
		Supported:  true,
		Engine:     "test",
		Principals: []database.DatabasePrincipal{},
		Grants:     []database.DatabaseGrant{},
	}, nil
}

func (driver *securityTestDriver) BuildSecurityChange(
	context.Context,
	database.SecurityChangeRequest,
) (database.SecurityChangePlan, error) {
	return driver.plan, nil
}

func (driver *securityTestDriver) ApplySecurityChange(
	_ context.Context,
	plan database.SecurityChangePlan,
) error {
	driver.applied = plan
	return nil
}

func TestSecurityChangeUsesRedactedReviewedPlan(t *testing.T) {
	driver := &securityTestDriver{
		plan: database.SecurityChangePlan{
			Summary:           "Create user reporter",
			Statements:        []string{"CREATE USER reporter PASSWORD 'secret';"},
			PreviewStatements: []string{"CREATE USER reporter PASSWORD '••••••';"},
			Transactional:     true,
		},
	}
	service := NewService()
	service.connections["security"] = &Connection{
		ID:     "security",
		Driver: driver,
		Config: database.Config{Driver: "test"},
	}
	change := database.SecurityChangeRequest{
		Action: database.SecurityCreatePrincipal,
		Principal: database.PrincipalOptions{
			Name: "reporter",
			Kind: database.PrincipalUser,
		},
	}
	preview := service.PreviewSecurityChange("security", change)
	if len(preview.Errors) != 0 {
		t.Fatalf("preview errors = %+v", preview.Errors)
	}
	if strings.Contains(preview.Data.SQL, "secret") {
		t.Fatalf("preview leaked secret: %s", preview.Data.SQL)
	}
	applied := service.ApplySecurityChange(
		"security",
		database.ApplySecurityChangeRequest{
			Change:      change,
			Fingerprint: preview.Data.Fingerprint,
		},
	)
	if len(applied.Errors) != 0 || !applied.Data.Applied {
		t.Fatalf("apply = %+v", applied)
	}
	if len(driver.applied.Statements) != 1 {
		t.Fatalf("applied plan = %+v", driver.applied)
	}
}
