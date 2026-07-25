package sqlserver

import (
	"database/sql"
	"strings"
	"testing"

	"rollingthunder/pkg/database"
)

func indexReference() database.ObjectReference {
	return database.ObjectReference{
		Kind:         database.ObjectKindIndex,
		Schema:       "dbo",
		Name:         "ix_events_name",
		ParentSchema: "dbo",
		ParentName:   "events",
	}
}

func TestFormatSQLServerIndexDefinitionPreservesIndexDetails(t *testing.T) {
	definition, err := formatSQLServerIndexDefinition(
		indexReference(),
		[]sqlServerIndexMetadata{
			{
				unique:          true,
				typeDescription: "NONCLUSTERED",
				filter: sql.NullString{
					String: "[archived_at] IS NULL",
					Valid:  true,
				},
				column:     sql.NullString{String: "name", Valid: true},
				descending: true,
			},
			{
				unique:          true,
				typeDescription: "NONCLUSTERED",
				column:          sql.NullString{String: "payload", Valid: true},
				included:        true,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	expected := "CREATE UNIQUE NONCLUSTERED INDEX [ix_events_name] " +
		"ON [dbo].[events] ([name] DESC) INCLUDE ([payload]) " +
		"WHERE [archived_at] IS NULL;"
	if definition != expected {
		t.Fatalf("definition = %q", definition)
	}
}

func TestFormatSQLServerIndexDefinitionUsesConstraintDDL(t *testing.T) {
	reference := indexReference()
	reference.Name = "pk_events"
	definition, err := formatSQLServerIndexDefinition(
		reference,
		[]sqlServerIndexMetadata{
			{
				unique:          true,
				primary:         true,
				typeDescription: "CLUSTERED",
				column:          sql.NullString{String: "id", Valid: true},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	expected := "ALTER TABLE [dbo].[events] ADD CONSTRAINT [pk_events] " +
		"PRIMARY KEY CLUSTERED ([id] ASC);"
	if definition != expected {
		t.Fatalf("definition = %q", definition)
	}
}

func TestFormatSQLServerColumnstoreIndexOmitsSortDirection(t *testing.T) {
	definition, err := formatSQLServerIndexDefinition(
		indexReference(),
		[]sqlServerIndexMetadata{
			{
				typeDescription: "NONCLUSTERED COLUMNSTORE",
				column:          sql.NullString{String: "payload", Valid: true},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(definition, " ASC") ||
		definition != "CREATE NONCLUSTERED COLUMNSTORE INDEX "+
			"[ix_events_name] ON [dbo].[events] ([payload]);" {
		t.Fatalf("definition = %q", definition)
	}
}

func TestFormatSQLServerSpecializedIndexIsHonest(t *testing.T) {
	definition, err := formatSQLServerIndexDefinition(
		indexReference(),
		[]sqlServerIndexMetadata{
			{
				typeDescription: "XML",
				column:          sql.NullString{String: "payload", Valid: true},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(definition, "cannot safely reconstruct") {
		t.Fatalf("definition = %q", definition)
	}
}
