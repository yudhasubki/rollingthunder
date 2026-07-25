package database

import (
	"strings"
	"testing"
)

func TestNormalizeConnectionEnvironment(t *testing.T) {
	tests := map[string]string{
		"":              ConnectionEnvironmentUnclassified,
		"legacy-value":  ConnectionEnvironmentUnclassified,
		" Development ": ConnectionEnvironmentDevelopment,
		"STAGING":       ConnectionEnvironmentStaging,
		"production":    ConnectionEnvironmentProduction,
	}
	for input, want := range tests {
		if got := NormalizeConnectionEnvironment(input); got != want {
			t.Errorf("NormalizeConnectionEnvironment(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeConfigMetadataDropsLegacyPresentationColor(t *testing.T) {
	config := NormalizeConfigMetadata(Config{
		Environment: "unexpected",
		Color:       "#ff00ff",
		Folder:      "  Team / Core  ",
		Tags:        []string{" Production ", "production", "", "Billing"},
	})
	if config.Environment != ConnectionEnvironmentUnclassified {
		t.Fatalf("environment = %q", config.Environment)
	}
	if config.AccessMode != ConnectionAccessReadWrite {
		t.Fatalf("access mode = %q", config.AccessMode)
	}
	if config.Folder != "Team / Core" {
		t.Fatalf("folder = %q", config.Folder)
	}
	if len(config.Tags) != 2 ||
		config.Tags[0] != "Production" ||
		config.Tags[1] != "Billing" {
		t.Fatalf("tags = %#v", config.Tags)
	}
	if config.Color != "" {
		t.Fatalf("legacy presentation color was retained: %q", config.Color)
	}
}

func TestNormalizeConfigMetadataDefaultsProductionToReadOnly(t *testing.T) {
	production := NormalizeConfigMetadata(Config{
		Environment: ConnectionEnvironmentProduction,
	})
	if production.AccessMode != ConnectionAccessReadOnly {
		t.Fatalf("production access mode = %q", production.AccessMode)
	}

	explicitWrite := NormalizeConfigMetadata(Config{
		Environment: ConnectionEnvironmentProduction,
		AccessMode:  ConnectionAccessReadWrite,
	})
	if explicitWrite.AccessMode != ConnectionAccessReadWrite {
		t.Fatalf("explicit production access mode = %q", explicitWrite.AccessMode)
	}
}

func TestConfigSafetyValidation(t *testing.T) {
	valid := Config{
		Host:        "database.internal",
		Port:        "5432",
		SSLMode:     "verify-full",
		AccessMode:  ConnectionAccessReadOnly,
		Folder:      "Finance",
		Tags:        []string{"reporting", "critical"},
		SSHEnabled:  true,
		SSHHost:     "bastion.internal",
		SSHPort:     "22",
		SSHAuthMode: "private-key",
	}
	if err := valid.ValidateSafety(); err != nil {
		t.Fatalf("valid config failed: %v", err)
	}
	tests := []Config{
		{Host: "database.internal\nsslmode=disable"},
		{Port: "70000"},
		{SSHPort: "not-a-port"},
		{SSLMode: "trust-anything"},
		{SSHEnabled: true, SSHAuthMode: "keyboard-interactive"},
		{AccessMode: "sometimes-writable"},
		{Driver: "unknown-database"},
		{Tags: []string{strings.Repeat("x", 65)}},
	}
	for _, config := range tests {
		if err := config.ValidateSafety(); err == nil {
			t.Errorf("expected unsafe config to fail: %+v", config)
		}
	}
}

func TestOracleConnectionSafetyValidation(t *testing.T) {
	validTNS := Config{
		Driver:               DriverOracle,
		SSLMode:              "verify-full",
		OracleConnectionMode: "tns",
		OracleTNSConfigPath:  "/reviewed/tnsnames.ora",
		OracleTNSAlias:       "APP",
	}
	if err := validTNS.ValidateSafety(); err != nil {
		t.Fatalf("valid Oracle TNS config failed: %v", err)
	}
	validWallet := Config{
		Driver:               DriverOracle,
		SSLMode:              "verify-full",
		OracleConnectionMode: "direct",
		OracleWalletPath:     "/reviewed/wallet",
		OracleWalletPassword: "opaque\nsecret",
	}
	if err := validWallet.ValidateSafety(); err != nil {
		t.Fatalf("valid Oracle Wallet config failed: %v", err)
	}
	tests := []Config{
		{
			Driver:               DriverOracle,
			OracleConnectionMode: "tns",
			OracleTNSAlias:       "APP",
		},
		{
			Driver:               DriverOracle,
			OracleConnectionMode: "tns",
			OracleTNSConfigPath:  "/reviewed/tnsnames.ora",
			OracleTNSAlias:       "APP",
			SSHEnabled:           true,
		},
		{
			Driver:           DriverOracle,
			SSLMode:          "disable",
			OracleWalletPath: "/reviewed/wallet",
		},
		{
			Driver:           DriverOracle,
			SSLMode:          "verify-ca",
			OracleWalletPath: "/reviewed/wallet",
		},
		{
			Driver:               DriverOracle,
			SSLMode:              "verify-full",
			OracleWalletPassword: "orphaned",
		},
		{
			Driver:           DriverOracle,
			SSLMode:          "verify-full",
			OracleWalletPath: "/reviewed/wallet",
			SSLRootCert:      "/reviewed/root.pem",
		},
		{
			Driver:               DriverPostgres,
			OracleConnectionMode: "direct",
		},
	}
	for _, config := range tests {
		if err := config.ValidateSafety(); err == nil {
			t.Errorf("expected unsafe Oracle config to fail: %+v", config)
		}
	}
}

func TestSQLServerAuthenticationSafetyValidation(t *testing.T) {
	valid := []Config{
		{
			Driver:            DriverSQLServer,
			User:              "sa",
			Password:          "secret",
			SSLMode:           "require",
			SQLServerAuthMode: SQLServerAuthSQL,
		},
		{
			Driver:                 DriverSQLServer,
			User:                   "user@example.com",
			Password:               "secret",
			SSLMode:                "verify-full",
			SQLServerAuthMode:      SQLServerAuthEntraPassword,
			SQLServerEntraClientID: "application-id",
		},
		{
			Driver:                 DriverSQLServer,
			Password:               "client-secret",
			SSLMode:                "verify-full",
			SQLServerAuthMode:      SQLServerAuthEntraServicePrincipal,
			SQLServerEntraClientID: "client-id",
			SQLServerEntraTenantID: "tenant-id",
		},
		{
			Driver:                 DriverSQLServer,
			User:                   "user@example.com",
			SSLMode:                "verify-full",
			SQLServerAuthMode:      SQLServerAuthEntraPassword,
			SQLServerEntraClientID: "application-id",
		},
		{
			Driver:                 DriverSQLServer,
			SSLMode:                "verify-full",
			SQLServerAuthMode:      SQLServerAuthEntraServicePrincipal,
			SQLServerEntraClientID: "client-id",
			SQLServerEntraTenantID: "tenant-id",
		},
		{
			Driver:            DriverSQLServer,
			SSLMode:           "verify-full",
			SQLServerAuthMode: SQLServerAuthEntraDefault,
		},
	}
	for _, config := range valid {
		if err := config.ValidateSafety(); err != nil {
			t.Errorf("valid SQL Server auth config failed: %+v: %v", config, err)
		}
	}

	invalid := []Config{
		{
			Driver:            DriverSQLServer,
			SSLMode:           "require",
			SQLServerAuthMode: SQLServerAuthSQL,
		},
		{
			Driver:            DriverSQLServer,
			User:              "unexpected",
			SSLMode:           "require",
			SQLServerAuthMode: SQLServerAuthIntegrated,
		},
		{
			Driver:                 DriverSQLServer,
			User:                   "user@example.com",
			Password:               "secret",
			SSLMode:                "disable",
			SQLServerAuthMode:      SQLServerAuthEntraPassword,
			SQLServerEntraClientID: "application-id",
		},
		{
			Driver:                 DriverSQLServer,
			Password:               "client-secret",
			SSLMode:                "verify-full",
			SQLServerAuthMode:      SQLServerAuthEntraServicePrincipal,
			SQLServerEntraClientID: "client-id",
		},
		{
			Driver:                 DriverPostgres,
			SQLServerAuthMode:      SQLServerAuthEntraDefault,
			SQLServerEntraClientID: "client-id",
		},
	}
	for _, config := range invalid {
		if err := config.ValidateSafety(); err == nil {
			t.Errorf("expected unsafe SQL Server auth config to fail: %+v", config)
		}
	}
}
