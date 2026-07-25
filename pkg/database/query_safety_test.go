package database

import (
	"reflect"
	"testing"
)

func TestAnalyzeQuerySafetyFindsUnfilteredUpdateAndDelete(t *testing.T) {
	for _, query := range []string{
		`UPDATE accounts SET active = false`,
		`DELETE FROM audit_log`,
		`WITH changed AS (UPDATE accounts SET active = false RETURNING *) SELECT * FROM changed`,
		`EXPLAIN ANALYZE UPDATE accounts SET active = false`,
	} {
		analysis := AnalyzeQuerySafety(query)
		if !analysis.RequiresConfirmation() {
			t.Fatalf("query was not flagged: %s", query)
		}
	}
}

func TestAnalyzeQuerySafetyAcceptsFilteredMutations(t *testing.T) {
	for _, query := range []string{
		`UPDATE accounts SET active = false WHERE tenant_id = 7`,
		`DELETE FROM audit_log WHERE created_at < now() - interval '30 days'`,
		`UPDATE accounts SET note = (SELECT note FROM defaults WHERE id = 1) WHERE id = 7`,
	} {
		analysis := AnalyzeQuerySafety(query)
		if analysis.RequiresConfirmation() {
			t.Fatalf("safe query was flagged: %s", query)
		}
	}
}

func TestAnalyzeQuerySafetyIgnoresCommentsStringsAndNonMutationUpdateTokens(t *testing.T) {
	query := `
		SELECT 'DELETE FROM accounts', $$UPDATE accounts SET active = false$$;
		SELECT * FROM accounts FOR UPDATE;
		INSERT INTO accounts (id) VALUES (1)
		ON CONFLICT (id) DO UPDATE SET id = excluded.id;
		ALTER TABLE orders
			ADD CONSTRAINT orders_account_fk FOREIGN KEY (account_id)
			REFERENCES accounts (id) ON DELETE CASCADE;
		GRANT UPDATE, DELETE ON accounts TO reporting;
		CREATE TRIGGER accounts_updated BEFORE UPDATE ON accounts
			FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
		-- DELETE FROM accounts
		/* UPDATE accounts SET active = false */
	`

	analysis := AnalyzeQuerySafety(query)

	if analysis.RequiresConfirmation() {
		t.Fatalf("non-mutation tokens were flagged: %+v", analysis)
	}
}

func TestAnalyzeQuerySafetyFindsMutationAfterCTEAndStatementBoundary(t *testing.T) {
	query := `
		SELECT 1;
		WITH stale AS (SELECT id FROM accounts WHERE active = false)
		DELETE FROM accounts
	`

	analysis := AnalyzeQuerySafety(query)

	if !analysis.RequiresConfirmation() {
		t.Fatal("unfiltered DELETE after a CTE was not flagged")
	}
}

func TestAnalyzeQuerySafetyIgnoresLockingUpdateAfterCTE(t *testing.T) {
	query := `
		WITH active AS (SELECT id FROM accounts WHERE active = true)
		SELECT * FROM active FOR UPDATE
	`

	analysis := AnalyzeQuerySafety(query)

	if analysis.RequiresConfirmation() {
		t.Fatalf("row-locking UPDATE token was flagged: %+v", analysis)
	}
}

func TestAnalyzeQuerySafetyDoesNotCountNestedWhereForOuterMutation(t *testing.T) {
	query := `UPDATE accounts SET note = (SELECT note FROM defaults WHERE id = 1)`

	analysis := AnalyzeQuerySafety(query)

	if !analysis.RequiresConfirmation() {
		t.Fatal("nested WHERE incorrectly made an unfiltered UPDATE safe")
	}
}

func TestFindWriteStatementAllowsReadOnlyQueries(t *testing.T) {
	for _, query := range []string{
		`SELECT * FROM accounts`,
		`WITH active AS (SELECT id FROM accounts) SELECT * FROM active`,
		`SHOW server_version`,
		`DESCRIBE accounts`,
		`EXPLAIN SELECT * FROM accounts`,
		`SELECT 'DROP TABLE accounts', $$UPDATE accounts SET active = false$$`,
		`SELECT q'[DROP TABLE accounts; UPDATE accounts SET active = false]' FROM dual`,
		`SELECT [UPDATE], [DROP] FROM [CREATE]`,
		"SELECT `UPDATE`, `DROP` FROM `CREATE`",
	} {
		if keyword := FindWriteStatement(query); keyword != "" {
			t.Fatalf("FindWriteStatement(%q) = %q", query, keyword)
		}
	}
}

func TestFindWriteStatementRejectsMutatingAndLockingQueries(t *testing.T) {
	tests := map[string]string{
		`INSERT INTO accounts (id) VALUES (1)`:                                     "INSERT",
		`WITH changed AS (DELETE FROM accounts RETURNING *) SELECT * FROM changed`: "DELETE",
		`SELECT * FROM accounts FOR UPDATE`:                                        "UPDATE",
		`SELECT nextval('accounts_id_seq')`:                                        "NEXTVAL",
		`SELECT * INTO archived_accounts FROM accounts`:                            "INTO",
		`EXPLAIN ANALYZE SELECT * FROM accounts`:                                   "ANALYZE",
		`PRAGMA journal_mode = WAL`:                                                "PRAGMA",
		`CALL refresh_reports()`:                                                   "CALL",
		`BACKUP DATABASE app TO DISK = 'app.bak'`:                                  "BACKUP",
		`DBCC CHECKDB`:                          "DBCC",
		`USE master`:                            "USE",
		`SELECT * FROM accounts WITH (UPDLOCK)`: "UPDLOCK",
	}
	for query, want := range tests {
		if got := FindWriteStatement(query); got != want {
			t.Fatalf("FindWriteStatement(%q) = %q, want %q", query, got, want)
		}
	}
}

func TestFindTransactionControl(t *testing.T) {
	for _, query := range []string{
		"BEGIN",
		"START TRANSACTION",
		"COMMIT;",
		"ROLLBACK",
		"ABORT",
		"END",
		"SELECT 1; SAVEPOINT before_update",
	} {
		if control := FindTransactionControl(query); control == "" {
			t.Fatalf("transaction control was not found: %s", query)
		}
	}

	if control := FindTransactionControl(
		"SELECT 'COMMIT', $$BEGIN$$",
	); control != "" {
		t.Fatalf("non-control query returned %q", control)
	}
	for _, block := range []string{
		"BEGIN NULL; END;",
		"BEGIN TRY SELECT 1; END TRY BEGIN CATCH THROW; END CATCH;",
		"DECLARE @value int = 1; SELECT @value;",
	} {
		if control := FindTransactionControl(block); control != "" {
			t.Fatalf("compound block returned transaction control %q", control)
		}
	}
	if control := FindTransactionControl("BEGIN TRANSACTION"); control != "BEGIN" {
		t.Fatalf("BEGIN TRANSACTION returned %q", control)
	}
}

func TestCountSQLStatementsIgnoresRoutineBodySemicolons(t *testing.T) {
	query := `
		CREATE FUNCTION public.answer() RETURNS integer AS $body$
		BEGIN
			RETURN 42;
		END;
		$body$ LANGUAGE plpgsql;
	`
	if got := CountSQLStatements(query); got != 1 {
		t.Fatalf("CountSQLStatements = %d, want 1", got)
	}
	if got := CountSQLStatements("SELECT 1; SELECT 2;"); got != 2 {
		t.Fatalf("CountSQLStatements(two statements) = %d, want 2", got)
	}
}

func TestLeadingSQLKeywordsSkipsComments(t *testing.T) {
	keywords := LeadingSQLKeywords(
		"-- reviewed\nCREATE OR REPLACE VIEW public.report AS SELECT 1",
		4,
	)
	want := []string{"CREATE", "OR", "REPLACE", "VIEW"}
	if !reflect.DeepEqual(keywords, want) {
		t.Fatalf("LeadingSQLKeywords = %#v, want %#v", keywords, want)
	}
}

func TestHasTopLevelStatementSeparatorIgnoresQuotedSemicolon(t *testing.T) {
	if HasTopLevelStatementSeparator(`';'`) {
		t.Fatal("quoted semicolon was treated as a statement separator")
	}
	if !HasTopLevelStatementSeparator(`value; DROP TABLE users`) {
		t.Fatal("top-level statement separator was not detected")
	}
}

func TestValidateDDLFragmentAcceptsNestedEngineSyntax(t *testing.T) {
	valid := []string{
		`numeric(12, 4)`,
		`enum('queued', 'running', 'done')`,
		`COALESCE(NULLIF(TRIM(name), ''), 'unknown')`,
		`q'[value, with punctuation; and quotes]'`,
		`$value$comma, semicolon;$value$`,
		`CONVERT([nvarchar](40), [created_at], 126)`,
	}
	for _, value := range valid {
		if err := ValidateDDLFragment(value, "fragment"); err != nil {
			t.Fatalf("ValidateDDLFragment(%q) error = %v", value, err)
		}
	}
}

func TestValidateDDLFragmentRejectsDefinitionEscapes(t *testing.T) {
	invalid := []string{
		`text, "admin" boolean`,
		`text)`,
		`text); DROP TABLE accounts; --`,
		`now(); DELETE FROM accounts`,
		`decimal(10, 2`,
		`'unterminated`,
		`/* unterminated`,
		`$value$unterminated`,
	}
	for _, value := range invalid {
		if err := ValidateDDLFragment(value, "fragment"); err == nil {
			t.Fatalf("ValidateDDLFragment(%q) accepted unsafe input", value)
		}
	}
}
