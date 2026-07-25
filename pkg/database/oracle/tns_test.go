package oracle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tnsFixture = `
# Primary application aliases
APP_RW, app_writer =
  (DESCRIPTION =
    (ADDRESS_LIST =
      (ADDRESS = (PROTOCOL = TCP)(HOST = db-one.internal)(PORT = 1521))
      (ADDRESS = (PROTOCOL = TCP)(HOST = db-two.internal)(PORT = 1522))
    )
    (CONNECT_DATA =
      (SERVER = DEDICATED)
      (SERVICE_NAME = app.internal)
    )
  )

IFILE = /review/separately/tnsnames-extra.ora

REPORTING =
  (DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=reporting.internal)(PORT=2484))
  (CONNECT_DATA=(SERVICE_NAME=reporting.internal))) # trailing comment
`

func TestParseTNSAliasesSupportsSynonymsCommentsAndNestedDescriptors(
	t *testing.T,
) {
	aliases, err := parseTNSAliases(tnsFixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"APP_RW", "APP_WRITER", "REPORTING"} {
		if _, exists := aliases[alias]; !exists {
			t.Fatalf("missing TNS alias %q: %#v", alias, aliases)
		}
	}
	if aliases["APP_RW"].descriptor != aliases["APP_WRITER"].descriptor {
		t.Fatal("TNS synonyms did not resolve to the same descriptor")
	}
	if !strings.Contains(
		aliases["REPORTING"].descriptor,
		"(PROTOCOL=TCPS)",
	) {
		t.Fatalf("reporting descriptor = %q", aliases["REPORTING"].descriptor)
	}
}

func TestParseTNSAliasesRejectsMalformedOrAmbiguousEntries(t *testing.T) {
	for _, value := range []string{
		`APP = (DESCRIPTION=(ADDRESS=(HOST=db.internal))`,
		"APP = host:1521/service",
		`APP = (DESCRIPTION=(ADDRESS=(HOST=one)))
		 APP = (DESCRIPTION=(ADDRESS=(HOST=two)))`,
	} {
		if _, err := parseTNSAliases(value); err == nil {
			t.Fatalf("malformed TNS configuration was accepted: %q", value)
		}
	}
}

func TestReadTNSAliasesAndResolveExactAlias(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tnsnames.ora")
	if err := os.WriteFile(path, []byte(tnsFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	aliases, err := ReadTNSAliases(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(aliases, ",") != "APP_RW,app_writer,REPORTING" {
		t.Fatalf("aliases = %v", aliases)
	}
	descriptor, err := resolveTNSAlias(path, "app_writer")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(descriptor, "db-one.internal") {
		t.Fatalf("descriptor = %q", descriptor)
	}
	if _, err := resolveTNSAlias(path, "missing"); err == nil {
		t.Fatal("missing TNS alias was accepted")
	}
}

func TestBuildConnectorSupportsTCPAliasAndRequiresTLSForTCPS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tnsnames.ora")
	if err := os.WriteFile(path, []byte(tnsFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	connector, err := buildConnector(Config{
		User:           "rolling",
		Password:       "secret",
		ConnectionMode: "tns",
		TNSConfigPath:  path,
		TNSAlias:       "APP_RW",
		SSLMode:        "disable",
	})
	if err != nil || connector == nil {
		t.Fatalf("TCP TNS connector = %#v, %v", connector, err)
	}
	if _, err := buildConnector(Config{
		User:           "rolling",
		Password:       "secret",
		ConnectionMode: "tns",
		TNSConfigPath:  path,
		TNSAlias:       "REPORTING",
		SSLMode:        "disable",
	}); err == nil {
		t.Fatal("TCPS alias with disabled TLS was accepted")
	}
}
