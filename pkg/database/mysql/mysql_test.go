package mysql

import (
	"bytes"
	"context"
	"os"
	"testing"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/database/drivertest"
)

func TestMySQLCapabilityContract(t *testing.T) {
	driver := NewMySQL(context.Background(), Config{})
	drivertest.RunCapabilityContract(t, driver, "mysql")
}

func TestMySQLLiveConformance(t *testing.T) {
	host := os.Getenv("ROLLINGTHUNDER_MYSQL_TEST_HOST")
	if host == "" {
		t.Skip("set ROLLINGTHUNDER_MYSQL_TEST_HOST to run live MySQL conformance")
	}
	config := Config{
		Host:     host,
		Port:     os.Getenv("ROLLINGTHUNDER_MYSQL_TEST_PORT"),
		User:     os.Getenv("ROLLINGTHUNDER_MYSQL_TEST_USER"),
		Password: os.Getenv("ROLLINGTHUNDER_MYSQL_TEST_PASSWORD"),
		Db:       os.Getenv("ROLLINGTHUNDER_MYSQL_TEST_DATABASE"),
		SSLMode:  os.Getenv("ROLLINGTHUNDER_MYSQL_TEST_SSL_MODE"),
	}
	driver := NewMySQL(context.Background(), config)
	if err := driver.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = driver.Close() })
	drivertest.RunLiveContract(t, drivertest.LiveConfig{
		Driver:      driver,
		Schema:      config.Db,
		IntegerType: "INT",
		TextType:    "VARCHAR(255)",
	})
}

func TestMySQLDialectBuilders(t *testing.T) {
	structures := database.Structures{
		{Name: "id", DataType: "int", IsPrimary: true},
		{Name: "name", DataType: "varchar(255)"},
	}
	filter, args, err := buildMySQLFilterClause([]database.Filter{
		{Column: "name", Operator: database.FilterContains, Value: "storm"},
		{Column: "id", Operator: database.FilterGreaterEqual, Value: 5},
	}, structures)
	if err != nil {
		t.Fatalf("buildMySQLFilterClause() error = %v", err)
	}
	if filter != " WHERE CAST(`name` AS CHAR) LIKE ? AND `id` >= ?" {
		t.Fatalf("filter = %q", filter)
	}
	if len(args) != 2 || args[0] != "%storm%" || args[1] != 5 {
		t.Fatalf("args = %#v", args)
	}

	order, err := buildMySQLOrderClause([]database.Sort{{
		Column: "name", Direction: database.SortDescending, Nulls: database.NullsFirst,
	}}, structures)
	if err != nil {
		t.Fatalf("buildMySQLOrderClause() error = %v", err)
	}
	want := " ORDER BY (`name` IS NULL) DESC, `name` DESC, (`id` IS NULL) ASC, `id` ASC"
	if order != want {
		t.Fatalf("order = %q, want %q", order, want)
	}
}

func TestMySQLMutationBuildersUseCompositeIdentity(t *testing.T) {
	table := database.Table{Schema: "app", Name: "members"}
	structures := database.Structures{
		{Name: "tenant_id", DataType: "int", IsPrimary: true},
		{Name: "member_id", DataType: "int", IsPrimary: true},
		{Name: "name", DataType: "varchar(255)"},
	}
	mutation, err := buildMySQLUpdateMutation(
		table,
		database.RowUpdate{
			Original: map[string]interface{}{
				"tenant_id": 4,
				"member_id": 9,
				"name":      "before",
			},
			Values: map[string]interface{}{
				"tenant_id": 4,
				"member_id": 9,
				"name":      "after",
			},
			ChangedColumns: []string{"name"},
		},
		structures,
		[]string{"tenant_id", "member_id"},
	)
	if err != nil {
		t.Fatalf("buildMySQLUpdateMutation() error = %v", err)
	}
	want := "UPDATE `app`.`members` SET `name` = ? WHERE `tenant_id` = ? AND `member_id` = ?"
	if mutation.SQL != want {
		t.Fatalf("SQL = %q, want %q", mutation.SQL, want)
	}
	if len(mutation.Args) != 3 ||
		mutation.Args[0] != "after" ||
		mutation.Args[1] != 4 ||
		mutation.Args[2] != 9 {
		t.Fatalf("Args = %#v", mutation.Args)
	}
}

func TestMySQLObjectChangePlansAreNonTransactional(t *testing.T) {
	driver := NewMySQL(context.Background(), Config{Db: "app"})
	plan, err := driver.BuildObjectChange(
		context.Background(),
		database.ObjectChangeRequest{
			Action: database.ObjectChangeCreate,
			Reference: database.ObjectReference{
				Kind: database.ObjectKindView,
				Name: "active_users",
			},
			Definition: "SELECT id FROM users WHERE active = 1",
		},
	)
	if err != nil {
		t.Fatalf("BuildObjectChange() error = %v", err)
	}
	if plan.Transactional {
		t.Fatal("MySQL DDL plan unexpectedly marked transactional")
	}
	if len(plan.Statements) != 1 ||
		plan.Statements[0] != "CREATE VIEW `app`.`active_users` AS\nSELECT id FROM users WHERE active = 1;" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestMySQLTLSModes(t *testing.T) {
	tlsConfig, err := buildMySQLTLSConfig(Config{SSLMode: "require"}, "db.example")
	if err != nil {
		t.Fatalf("buildMySQLTLSConfig(require) error = %v", err)
	}
	if tlsConfig == nil || !tlsConfig.InsecureSkipVerify {
		t.Fatalf("require TLS config = %#v", tlsConfig)
	}
	if _, err := buildMySQLTLSConfig(
		Config{SSLMode: "verify-full"},
		"db.example",
	); err == nil {
		t.Fatal("verify-full accepted a missing CA certificate")
	}
	if _, err := buildMySQLTLSConfig(
		Config{SSLMode: "disable", SSLCert: "client.pem"},
		"db.example",
	); err == nil {
		t.Fatal("disable accepted a client certificate")
	}
}

func TestMySQLInsertExportCanUseDuplicateKeyUpdate(t *testing.T) {
	var output bytes.Buffer
	sink := newMySQLInsertSink(
		&output,
		database.Table{Schema: "app", Name: "members"},
		database.Structures{
			{Name: "id", IsPrimary: true},
			{Name: "name"},
		},
		database.SQLInsertOptions{Upsert: true},
	)
	if !sink.includeUpsert {
		t.Fatal("upsert option did not enable duplicate-key handling")
	}
	if sink.upsertStatement != "\nON DUPLICATE KEY UPDATE `name` = VALUES(`name`)" {
		t.Fatalf("upsert statement = %q", sink.upsertStatement)
	}
}
