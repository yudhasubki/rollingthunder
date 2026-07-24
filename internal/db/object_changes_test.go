package db

import (
	"errors"
	"testing"

	"rollingthunder/pkg/database"
)

func sampleObjectChange() database.ObjectChangeRequest {
	return database.ObjectChangeRequest{
		Action: database.ObjectChangeRename,
		Reference: database.ObjectReference{
			Kind:   database.ObjectKindView,
			Schema: "public",
			Name:   "old_report",
		},
		NewName: "new_report",
	}
}

func TestPreviewAndApplyDatabaseObjectChangeUseReviewedPlan(t *testing.T) {
	driver := &routingTestDriver{
		name: "alpha",
		changePlan: database.ObjectChangePlan{
			Summary:       "Rename view public.old_report",
			Statements:    []string{`ALTER VIEW "public"."old_report" RENAME TO "new_report";`},
			Transactional: true,
		},
	}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": driver},
		"alpha",
	)

	preview := service.PreviewDatabaseObjectChange("alpha", sampleObjectChange())
	if len(preview.Errors) != 0 {
		t.Fatalf("preview errors = %+v", preview.Errors)
	}
	if preview.Data.Fingerprint == "" || preview.Data.StatementCount != 1 {
		t.Fatalf("preview = %+v", preview.Data)
	}

	applied := service.ApplyDatabaseObjectChange(
		"alpha",
		database.ApplyObjectChangeRequest{
			Change:      sampleObjectChange(),
			Fingerprint: preview.Data.Fingerprint,
		},
	)
	if len(applied.Errors) != 0 || !applied.Data.Applied {
		t.Fatalf("apply response = %+v", applied)
	}
	if driver.appliedPlan.Summary != driver.changePlan.Summary {
		t.Fatalf("applied plan = %+v", driver.appliedPlan)
	}
}

func TestApplyDatabaseObjectChangeRejectsChangedPreview(t *testing.T) {
	driver := &routingTestDriver{
		name: "alpha",
		changePlan: database.ObjectChangePlan{
			Summary:    "Drop view",
			Statements: []string{`DROP VIEW "public"."report";`},
		},
	}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": driver},
		"alpha",
	)

	response := service.ApplyDatabaseObjectChange(
		"alpha",
		database.ApplyObjectChangeRequest{
			Change:      sampleObjectChange(),
			Fingerprint: "stale-preview",
		},
	)
	if len(response.Errors) != 1 ||
		response.Errors[0].Code != errorCodeObjectChangeReviewRequired {
		t.Fatalf("response = %+v", response)
	}
	if driver.appliedPlan.Summary != "" {
		t.Fatalf("unreviewed plan was applied: %+v", driver.appliedPlan)
	}
}

func TestApplyDatabaseObjectChangeReturnsStableDriverError(t *testing.T) {
	driver := &routingTestDriver{
		name: "alpha",
		changePlan: database.ObjectChangePlan{
			Summary:    "Rename view",
			Statements: []string{`ALTER VIEW "public"."old" RENAME TO "new";`},
		},
		applyChangeErr: errors.New("dependent objects still reference the view"),
	}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": driver},
		"alpha",
	)
	preview := service.PreviewDatabaseObjectChange("alpha", sampleObjectChange())

	response := service.ApplyDatabaseObjectChange(
		"alpha",
		database.ApplyObjectChangeRequest{
			Change:      sampleObjectChange(),
			Fingerprint: preview.Data.Fingerprint,
		},
	)
	if len(response.Errors) != 1 ||
		response.Errors[0].Code != errorCodeObjectChangeFailed {
		t.Fatalf("response = %+v", response)
	}
}
