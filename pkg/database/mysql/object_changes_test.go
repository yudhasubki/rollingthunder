package mysql

import (
	"strings"
	"testing"

	"rollingthunder/pkg/database"
)

func TestReviewedMySQLCompoundObjects(t *testing.T) {
	tests := []struct {
		name       string
		kind       database.ObjectKind
		definition string
	}{
		{
			name:       "function",
			kind:       database.ObjectKindFunction,
			definition: "CREATE FUNCTION app.answer() RETURNS INT DETERMINISTIC RETURN 42",
		},
		{
			name: "procedure",
			kind: database.ObjectKindProcedure,
			definition: `CREATE PROCEDURE app.refresh_orders()
				BEGIN
					SELECT COUNT(*) FROM app.orders;
				END`,
		},
		{
			name: "trigger",
			kind: database.ObjectKindTrigger,
			definition: `CREATE TRIGGER app.orders_before_insert
				BEFORE INSERT ON app.orders
				FOR EACH ROW
				BEGIN
					SET NEW.updated_at = CURRENT_TIMESTAMP;
				END`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statement, err := reviewedMySQLDDL(test.definition, test.kind)
			if err != nil {
				t.Fatalf("reviewedMySQLDDL() error = %v", err)
			}
			if !strings.HasSuffix(statement, ";") {
				t.Fatalf("reviewed statement = %q, want trailing semicolon", statement)
			}
		})
	}
}

func TestReviewedMySQLCompoundObjectsRejectClientDelimiter(t *testing.T) {
	_, err := reviewedMySQLDDL(
		"DELIMITER // CREATE PROCEDURE app.refresh() BEGIN SELECT 1; END//",
		database.ObjectKindProcedure,
	)
	if err == nil || !strings.Contains(err.Error(), "DELIMITER") {
		t.Fatalf("reviewedMySQLDDL() error = %v", err)
	}
}

func TestMySQLDropStatementsCoverManagedObjectKinds(t *testing.T) {
	for _, kind := range []database.ObjectKind{
		database.ObjectKindView,
		database.ObjectKindFunction,
		database.ObjectKindProcedure,
		database.ObjectKindTrigger,
	} {
		statement, err := mysqlDropStatement(database.ObjectReference{
			Kind:   kind,
			Schema: "app",
			Name:   "managed_object",
		})
		if err != nil {
			t.Fatalf("mysqlDropStatement(%s) error = %v", kind, err)
		}
		if !strings.HasPrefix(statement, "DROP ") ||
			!strings.Contains(statement, "`app`.`managed_object`") {
			t.Fatalf("mysqlDropStatement(%s) = %q", kind, statement)
		}
	}
}
