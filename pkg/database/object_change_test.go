package database

import "testing"

func TestPreviewObjectChangeCreatesStableReviewedFingerprint(t *testing.T) {
	plan := ObjectChangePlan{
		Summary:       "Rename view",
		Statements:    []string{`ALTER VIEW "public"."old" RENAME TO "new";`},
		Transactional: true,
	}
	first, err := PreviewObjectChange("postgres", plan)
	if err != nil {
		t.Fatalf("PreviewObjectChange: %v", err)
	}
	second, err := PreviewObjectChange("postgres", plan)
	if err != nil {
		t.Fatalf("PreviewObjectChange second call: %v", err)
	}
	if first.Fingerprint == "" || first.Fingerprint != second.Fingerprint {
		t.Fatalf("unstable fingerprints: %q and %q", first.Fingerprint, second.Fingerprint)
	}

	plan.Statements[0] = `ALTER VIEW "public"."old" RENAME TO "different";`
	changed, err := PreviewObjectChange("postgres", plan)
	if err != nil {
		t.Fatalf("PreviewObjectChange changed plan: %v", err)
	}
	if changed.Fingerprint == first.Fingerprint {
		t.Fatal("changed SQL retained the reviewed fingerprint")
	}
}

func TestObjectChangeRequestValidatesStructuredChanges(t *testing.T) {
	request := ObjectChangeRequest{
		Action: ObjectChangeCreateIndex,
		Index: &IndexChange{
			Table:   Table{Schema: "public", Name: "orders"},
			Name:    "orders_customer_idx",
			Columns: []string{"customer_id"},
		},
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("valid index request: %v", err)
	}
	request.Index.Columns = nil
	if err := request.Validate(); err == nil {
		t.Fatal("index request without columns was accepted")
	}
}
