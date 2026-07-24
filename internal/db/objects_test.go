package db

import (
	"context"
	"errors"
	"testing"

	"rollingthunder/pkg/database"
)

func TestGetCapabilitiesUsesOwningConnection(t *testing.T) {
	alpha := &routingTestDriver{name: "alpha"}
	bravo := &routingTestDriver{name: "bravo"}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": alpha, "bravo": bravo},
		"bravo",
	)

	response := service.GetCapabilities("alpha")
	if len(response.Errors) != 0 {
		t.Fatalf("GetCapabilities returned errors: %+v", response.Errors)
	}
	if response.Data.Engine != "test" {
		t.Fatalf("capability engine = %q, want test", response.Data.Engine)
	}
}

func TestGetDatabaseObjectsRoutesFilterAndNormalizesEmptyResult(t *testing.T) {
	driver := &routingTestDriver{name: "alpha"}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": driver},
		"alpha",
	)
	filter := database.ObjectFilter{
		Schema: "public",
		Kinds:  []database.ObjectKind{database.ObjectKindView},
		Search: "report",
	}

	response := service.GetDatabaseObjects("alpha", filter)
	if len(response.Errors) != 0 {
		t.Fatalf("GetDatabaseObjects returned errors: %+v", response.Errors)
	}
	if response.Data == nil || len(response.Data) != 0 {
		t.Fatalf("objects = %#v, want non-nil empty slice", response.Data)
	}
	if driver.objectFilter.Schema != filter.Schema ||
		driver.objectFilter.Search != filter.Search {
		t.Fatalf("object filter = %+v, want %+v", driver.objectFilter, filter)
	}
}

func TestGetDatabaseObjectReturnsStableMetadataError(t *testing.T) {
	driver := &routingTestDriver{
		name:      "alpha",
		objectErr: errors.New("permission denied for pg_proc"),
	}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": driver},
		"alpha",
	)

	response := service.GetDatabaseObject("alpha", database.ObjectReference{
		Kind:   database.ObjectKindFunction,
		Schema: "public",
		Name:   "refresh_report",
	})
	if len(response.Errors) != 1 {
		t.Fatalf("errors = %+v, want one error", response.Errors)
	}
	if response.Errors[0].Code != errorCodeObjectMetadataFailed {
		t.Fatalf("error code = %q", response.Errors[0].Code)
	}
}

func TestGetDatabaseObjectRejectsUnsupportedDriver(t *testing.T) {
	base := &routingTestDriver{name: "alpha"}
	driver := &connectionTestDriver{Driver: base}
	service := NewService()
	service.Start(context.Background())
	service.connections["alpha"] = &Connection{
		ID:     "alpha",
		Driver: driver,
	}

	response := service.GetDatabaseObject("alpha", database.ObjectReference{
		Kind: database.ObjectKindTable,
		Name: "orders",
	})
	if len(response.Errors) != 1 ||
		response.Errors[0].Code != errorCodeObjectMetadataUnsupported {
		t.Fatalf("unsupported response = %+v", response)
	}
}
