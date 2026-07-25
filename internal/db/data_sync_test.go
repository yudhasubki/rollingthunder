package db

import (
	"testing"

	"rollingthunder/pkg/database"
)

func dataSyncTestService(t *testing.T) (*Service, *routingTestDriver) {
	t.Helper()
	structures := database.Structures{
		{Name: "id", DataType: "integer", IsPrimary: true},
		{Name: "name", DataType: "text", Nullable: false},
		{Name: "updated_at", DataType: "timestamp", Nullable: true},
	}
	source := &routingTestDriver{
		name:       "source",
		structures: structures,
		queryRows: []map[string]interface{}{
			{"id": int64(1), "name": "Ada", "updated_at": nil},
			{"id": int64(2), "name": "Grace", "updated_at": "2026-07-25"},
		},
	}
	target := &routingTestDriver{
		name:       "target",
		structures: structures,
		queryRows: []map[string]interface{}{
			{"id": int64(1), "name": "Ada Lovelace", "updated_at": nil},
			{"id": int64(3), "name": "Linus", "updated_at": nil},
		},
		changeResult: database.TableChangeResult{
			Inserted: 1,
			Updated:  1,
			Deleted:  1,
		},
	}
	service := newRoutingTestService(
		map[string]*routingTestDriver{
			"source": source,
			"target": target,
		},
		"source",
	)
	return service, target
}

func TestPreviewAndApplyDataSync(t *testing.T) {
	service, target := dataSyncTestService(t)
	request := database.DataSyncRequest{
		SourceConnectionID: "source",
		SourceSchema:       "public",
		SourceTable:        "people",
		TargetConnectionID: "target",
		TargetSchema:       "public",
		TargetTable:        "people",
		MaxRows:            100,
	}

	preview := service.PreviewDataSync(request)
	if len(preview.Errors) > 0 {
		t.Fatalf("PreviewDataSync() errors = %+v", preview.Errors)
	}
	if preview.Data.Added != 1 || preview.Data.Updated != 1 ||
		preview.Data.Deleted != 1 || !preview.Data.SafeToApply {
		t.Fatalf("PreviewDataSync() = %+v", preview.Data)
	}
	if len(preview.Data.KeyColumns) != 1 ||
		preview.Data.KeyColumns[0] != "id" ||
		preview.Data.Fingerprint == "" {
		t.Fatalf("PreviewDataSync() metadata = %+v", preview.Data)
	}

	applied := service.ApplyDataSync(database.ApplyDataSyncRequest{
		Sync:              request,
		Fingerprint:       preview.Data.Fingerprint,
		SelectedChangeIDs: dataSyncChangeIDs(preview.Data.Changes),
	})
	if len(applied.Errors) > 0 {
		t.Fatalf("ApplyDataSync() errors = %+v", applied.Errors)
	}
	if !applied.Data.Applied || applied.Data.Inserted != 1 ||
		applied.Data.Updated != 1 || applied.Data.Deleted != 1 {
		t.Fatalf("ApplyDataSync() = %+v", applied.Data)
	}
	if target.changeRequest.Count() != 3 {
		t.Fatalf("applied change set = %+v", target.changeRequest)
	}
	if target.changeRequest.Updated[0].Original["name"] != "Ada Lovelace" ||
		target.changeRequest.Updated[0].Values["name"] != "Ada" {
		t.Fatalf("update change = %+v", target.changeRequest.Updated[0])
	}
}

func dataSyncChangeIDs(changes []database.DataSyncChange) []string {
	result := make([]string, 0, len(changes))
	for _, change := range changes {
		result = append(result, change.ID)
	}
	return result
}

func TestApplyDataSyncRequiresExplicitSelection(t *testing.T) {
	service, _ := dataSyncTestService(t)
	request := database.DataSyncRequest{
		SourceConnectionID: "source",
		SourceSchema:       "public",
		SourceTable:        "people",
		TargetConnectionID: "target",
		TargetSchema:       "public",
		TargetTable:        "people",
		MaxRows:            100,
	}
	preview := service.PreviewDataSync(request)
	if len(preview.Errors) > 0 {
		t.Fatalf("PreviewDataSync() errors = %+v", preview.Errors)
	}
	applied := service.ApplyDataSync(database.ApplyDataSyncRequest{
		Sync:        request,
		Fingerprint: preview.Data.Fingerprint,
	})
	if len(applied.Errors) == 0 {
		t.Fatal("ApplyDataSync() accepted an empty reviewed selection")
	}
}

func TestApplyDataSyncHonorsSelectionAndReadOnlyGuard(t *testing.T) {
	service, target := dataSyncTestService(t)
	request := database.DataSyncRequest{
		SourceConnectionID: "source",
		SourceTable:        "people",
		TargetConnectionID: "target",
		TargetTable:        "people",
		MaxRows:            100,
	}
	preview := service.PreviewDataSync(request)
	if len(preview.Errors) > 0 {
		t.Fatalf("PreviewDataSync() errors = %+v", preview.Errors)
	}
	insertID := ""
	for _, change := range preview.Data.Changes {
		if change.Kind == "insert" {
			insertID = change.ID
		}
	}
	if insertID == "" {
		t.Fatal("insert change was not generated")
	}

	selected := service.ApplyDataSync(database.ApplyDataSyncRequest{
		Sync:              request,
		Fingerprint:       preview.Data.Fingerprint,
		SelectedChangeIDs: []string{insertID},
	})
	if len(selected.Errors) > 0 {
		t.Fatalf("ApplyDataSync(selected) errors = %+v", selected.Errors)
	}
	if target.changeRequest.Count() != 1 || len(target.changeRequest.Added) != 1 {
		t.Fatalf("selected change set = %+v", target.changeRequest)
	}

	service.connections["target"].Config.AccessMode = database.ConnectionAccessReadOnly
	blocked := service.ApplyDataSync(database.ApplyDataSyncRequest{
		Sync:              request,
		Fingerprint:       preview.Data.Fingerprint,
		SelectedChangeIDs: []string{insertID},
	})
	if len(blocked.Errors) != 1 ||
		blocked.Errors[0].Code != errorCodeReadOnlyConnection {
		t.Fatalf("ApplyDataSync(read-only) = %+v", blocked)
	}
}

func TestDataSyncRequiresStableKey(t *testing.T) {
	service, _ := dataSyncTestService(t)
	service.connections["source"].Driver.(*routingTestDriver).structures[0].IsPrimary = false
	result := service.PreviewDataSync(database.DataSyncRequest{
		SourceConnectionID: "source",
		SourceTable:        "people",
		TargetConnectionID: "target",
		TargetTable:        "people",
	})
	if len(result.Errors) != 1 || result.Errors[0].Code != errorCodeDataSyncFailed {
		t.Fatalf("PreviewDataSync() = %+v", result)
	}
}

func TestResolveDataSyncColumnsExcludesTargetIdentityColumns(t *testing.T) {
	source := database.Structures{
		{Name: "external_key", IsPrimary: true},
		{Name: "warehouse_id"},
		{Name: "name"},
	}
	target := database.Structures{
		{Name: "external_key", IsPrimary: true},
		{Name: "warehouse_id", IsAutoInc: true},
		{Name: "name"},
	}
	keys, compare, writes, err := resolveDataSyncColumns(
		database.DataSyncRequest{},
		source,
		target,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != "external_key" {
		t.Fatalf("keys = %#v", keys)
	}
	if slicesContainsFold(compare, "warehouse_id") ||
		slicesContainsFold(writes, "warehouse_id") {
		t.Fatalf("identity column leaked into sync: compare=%#v writes=%#v", compare, writes)
	}
	if _, _, _, err := resolveDataSyncColumns(
		database.DataSyncRequest{
			CompareColumns: []string{"warehouse_id"},
		},
		source,
		target,
	); err == nil {
		t.Fatal("explicit identity-column synchronization was accepted")
	}
}
