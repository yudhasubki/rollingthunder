package db

import (
	"testing"

	"rollingthunder/pkg/database"
)

func readOnlyRoutingService(t *testing.T) (*Service, *routingTestDriver) {
	t.Helper()
	driver := &routingTestDriver{name: "production"}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"production": driver},
		"production",
	)
	service.connections["production"].Config = database.Config{
		Name:        "Production",
		Environment: database.ConnectionEnvironmentProduction,
		AccessMode:  database.ConnectionAccessReadOnly,
	}
	service.connections["production"].Name = "Production"
	return service, driver
}

func assertReadOnlyError(t *testing.T, code string, errorsCount int) {
	t.Helper()
	if errorsCount != 1 {
		t.Fatalf("errors = %d, want 1", errorsCount)
	}
	if code != errorCodeReadOnlyConnection {
		t.Fatalf("error code = %q, want %q", code, errorCodeReadOnlyConnection)
	}
}

func TestReadOnlyConnectionAllowsReadsAndBlocksQueryWrites(t *testing.T) {
	service, driver := readOnlyRoutingService(t)

	read := service.ExecuteQuery(database.QueryRequest{
		ConnectionID: "production",
		Query:        "SELECT * FROM customers",
	})
	if len(read.Errors) > 0 {
		t.Fatalf("read query errors = %+v", read.Errors)
	}

	write := service.ExecuteQuery(database.QueryRequest{
		ConnectionID: "production",
		Query:        "INSERT INTO customers (name) VALUES ('Ada')",
	})
	code := ""
	if len(write.Errors) > 0 {
		code = write.Errors[0].Code
	}
	assertReadOnlyError(t, code, len(write.Errors))
	if driver.queryCount() != 1 {
		t.Fatalf("driver query count = %d, want only the read query", driver.queryCount())
	}
}

func TestReadOnlyConnectionUnlockRequiresExactNameAndRelocks(t *testing.T) {
	service, _ := readOnlyRoutingService(t)

	rejected := service.SetConnectionWriteAccess(SetConnectionWriteAccessRequest{
		ConnectionID: "production",
		Enable:       true,
		Confirmation: "production",
	})
	code := ""
	if len(rejected.Errors) > 0 {
		code = rejected.Errors[0].Code
	}
	assertReadOnlyError(t, code, len(rejected.Errors))

	unlocked := service.SetConnectionWriteAccess(SetConnectionWriteAccessRequest{
		ConnectionID: "production",
		Enable:       true,
		Confirmation: "Production",
	})
	if len(unlocked.Errors) > 0 || !unlocked.Data.WriteEnabled ||
		!unlocked.Data.TemporaryUnlock {
		t.Fatalf("unlock response = %+v", unlocked)
	}
	inserted := service.InsertRow(
		"production",
		database.Table{Name: "customers"},
		map[string]interface{}{"name": "Ada"},
	)
	if len(inserted.Errors) > 0 || !inserted.Data {
		t.Fatalf("InsertRow() after unlock = %+v", inserted)
	}

	locked := service.SetConnectionWriteAccess(SetConnectionWriteAccessRequest{
		ConnectionID: "production",
		Enable:       false,
	})
	if len(locked.Errors) > 0 || locked.Data.WriteEnabled {
		t.Fatalf("lock response = %+v", locked)
	}
	blocked := service.InsertRow(
		"production",
		database.Table{Name: "customers"},
		map[string]interface{}{"name": "Grace"},
	)
	code = ""
	if len(blocked.Errors) > 0 {
		code = blocked.Errors[0].Code
	}
	assertReadOnlyError(t, code, len(blocked.Errors))
}
