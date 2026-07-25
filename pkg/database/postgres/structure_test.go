package postgres

import (
	"testing"

	"rollingthunder/pkg/database"
)

func TestApplyColumnConstraintsPreservesPrimaryAndForeignKey(t *testing.T) {
	foreignSchema := "public"
	foreignTable := "organizations"
	foreignColumn := "id"
	info := database.Structure{Name: "organization_id"}

	applyColumnConstraints(
		&info,
		true,
		Constraints{
			{Column: "organization_id", Type: "p"},
			{
				Column:        "organization_id",
				Type:          "f",
				ForeignSchema: &foreignSchema,
				ForeignTable:  &foreignTable,
				ForeignCol:    &foreignColumn,
			},
		},
	)

	if !info.IsPrimary || info.IsPrimaryLabel != "PRI" {
		t.Fatalf("expected primary-key metadata, got %+v", info)
	}
	if info.ForeignKey == nil || *info.ForeignKey != "public.organizations(id)" {
		t.Fatalf("expected foreign-key reference, got %+v", info.ForeignKey)
	}
	if info.ForeignSchema == nil || *info.ForeignSchema != "public" {
		t.Fatalf("expected foreign schema, got %+v", info.ForeignSchema)
	}
	if info.ForeignTable == nil || *info.ForeignTable != "organizations" {
		t.Fatalf("expected foreign table, got %+v", info.ForeignTable)
	}
	if info.ForeignColumn == nil || *info.ForeignColumn != "id" {
		t.Fatalf("expected foreign column, got %+v", info.ForeignColumn)
	}
}

func TestApplyColumnConstraintsKeepsIndependentProperties(t *testing.T) {
	info := database.Structure{Name: "email"}

	applyColumnConstraints(
		&info,
		false,
		Constraints{
			{Column: "email", Type: "u"},
			{Column: "unrelated", Type: "p"},
		},
	)

	if !info.IsUnique {
		t.Fatal("expected unique constraint to be preserved")
	}
	if info.IsPrimary || info.ForeignKey != nil {
		t.Fatalf("unexpected unrelated constraint metadata: %+v", info)
	}
}

func TestApplyColumnTypeIdentifiesEnums(t *testing.T) {
	info := database.Structure{}

	applyColumnType(
		&info,
		Column{
			DataType:  "USER-DEFINED",
			UDTSchema: "public",
			UDTName:   "order_status",
			IsEnum:    true,
		},
	)

	if info.DataType != "enum" || !info.IsEnum {
		t.Fatalf("expected enum data type, got %+v", info)
	}
	if info.TypeSchema == nil || *info.TypeSchema != "public" {
		t.Fatalf("expected enum schema, got %+v", info.TypeSchema)
	}
	if info.TypeName == nil || *info.TypeName != "order_status" {
		t.Fatalf("expected enum type name, got %+v", info.TypeName)
	}
}

func TestApplyColumnGenerationPreservesGeneratedExpression(t *testing.T) {
	expression := "(quantity * unit_price)"
	info := database.Structure{}

	applyColumnGeneration(
		&info,
		Column{
			IsGenerated: "ALWAYS",
			Generation:  &expression,
		},
	)

	if !info.IsGenerated || info.Generation != expression {
		t.Fatalf("expected generated-column metadata, got %+v", info)
	}
}

func TestApplyColumnGenerationPreservesIdentityMode(t *testing.T) {
	mode := "ALWAYS"
	info := database.Structure{}

	applyColumnGeneration(
		&info,
		Column{
			IsIdentity:   "YES",
			IdentityMode: &mode,
		},
	)

	if !info.IsAutoInc || info.IsGenerated {
		t.Fatalf("expected identity metadata, got %+v", info)
	}
	if info.Generation != "IDENTITY ALWAYS" {
		t.Fatalf("identity generation = %q", info.Generation)
	}
}
