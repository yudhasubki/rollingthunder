package db

import (
	"errors"
	"testing"

	"rollingthunder/pkg/database"
)

func TestApplyTableChangesUsesOwningConnection(t *testing.T) {
	alpha := &routingTestDriver{
		name: "alpha",
		changeResult: database.TableChangeResult{
			Inserted: 1,
			Updated:  1,
		},
	}
	bravo := &routingTestDriver{name: "bravo"}
	service := newRoutingTestService(
		map[string]*routingTestDriver{
			"alpha": alpha,
			"bravo": bravo,
		},
		"bravo",
	)
	changes := database.TableChangeSet{
		Table: database.Table{Schema: "public", Name: "accounts"},
		Added: []map[string]interface{}{{"name": "new"}},
		Updated: []database.RowUpdate{
			{
				Original:       map[string]interface{}{"id": 7, "name": "old"},
				Values:         map[string]interface{}{"id": 7, "name": "updated"},
				ChangedColumns: []string{"name"},
			},
		},
	}

	result := service.ApplyTableChanges("alpha", changes)

	if len(result.Errors) != 0 {
		t.Fatalf("ApplyTableChanges errors = %+v", result.Errors)
	}
	if result.Data.Inserted != 1 || result.Data.Updated != 1 {
		t.Fatalf("ApplyTableChanges result = %+v", result.Data)
	}
	if request := alpha.tableChangeRequest(); request.Table.Name != "accounts" {
		t.Fatalf("alpha request = %+v", request)
	}
	if request := bravo.tableChangeRequest(); request.Count() != 0 {
		t.Fatalf("bravo received changes = %+v", request)
	}
}

func TestApplyTableChangesReturnsAtomicRollbackMessage(t *testing.T) {
	driver := &routingTestDriver{
		name:      "alpha",
		changeErr: errors.New("update 2 violated a unique constraint"),
	}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": driver},
		"alpha",
	)

	result := service.ApplyTableChanges(
		"alpha",
		database.TableChangeSet{
			Table:   database.Table{Schema: "public", Name: "accounts"},
			Deleted: []map[string]interface{}{{"id": 7}},
		},
	)

	if len(result.Errors) != 1 {
		t.Fatalf("ApplyTableChanges errors = %+v", result.Errors)
	}
	if result.Errors[0].Code != errorCodeTableChangesFailed {
		t.Fatalf("error code = %q", result.Errors[0].Code)
	}
	if result.Errors[0].Hint == "" {
		t.Fatal("atomic failure did not include a recovery hint")
	}
}
