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
