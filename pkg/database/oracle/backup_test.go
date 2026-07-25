package oracle

import (
	"strings"
	"testing"

	"rollingthunder/pkg/database"
)

func TestOracleDataPumpNamesAreBoundedAndOperationScoped(t *testing.T) {
	for _, operation := range []string{"EXP", "IMP"} {
		job, dump, log, err := oracleDataPumpNames(operation)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(job, "RT_"+operation+"_") ||
			len(job) > 30 ||
			dump != strings.ToLower(job)+".dmp" ||
			log != strings.ToLower(job)+".log" {
			t.Fatalf(
				"Data Pump names = job %q, dump %q, log %q",
				job,
				dump,
				log,
			)
		}
	}
	if _, _, _, err := oracleDataPumpNames("DROP"); err == nil {
		t.Fatal("unsupported Data Pump operation was accepted")
	}
}

func TestOracleDataPumpSchemaExpressionEscapesQuotedSchema(t *testing.T) {
	if got, want := oracleDataPumpSchemaExpression("APP'S"), "IN ('APP''S')"; got != want {
		t.Fatalf("schema expression = %q, want %q", got, want)
	}
}

func TestOracleDataPumpBlocksUseBoundInputsAndCleanup(t *testing.T) {
	export := strings.ToUpper(oracleDataPumpExportBlock)
	for _, expected := range []string{
		"DBMS_DATAPUMP.OPEN",
		"DBMS_DATAPUMP.ADD_FILE",
		"DBMS_DATAPUMP.METADATA_FILTER",
		"DBMS_DATAPUMP.WAIT_FOR_JOB",
		":SCHEMA_EXPRESSION",
	} {
		if !strings.Contains(export, expected) {
			t.Fatalf("export block is missing %q", expected)
		}
	}
	importBlock := strings.ToUpper(oracleDataPumpImportBlock)
	if !strings.Contains(importBlock, "TABLE_EXISTS_ACTION") ||
		!strings.Contains(importBlock, "'REPLACE'") {
		t.Fatal("import block does not define reviewed replacement behavior")
	}
	if oracleDataPumpUploadChunk > 32767 {
		t.Fatalf("UTL_FILE upload chunk = %d", oracleDataPumpUploadChunk)
	}
	var _ database.StreamingBackupDriver = (*Oracle)(nil)
}
