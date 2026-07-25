package database

import "testing"

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
	})
	if config.Environment != ConnectionEnvironmentUnclassified {
		t.Fatalf("environment = %q", config.Environment)
	}
	if config.Color != "" {
		t.Fatalf("legacy presentation color was retained: %q", config.Color)
	}
}

func TestConfigSafetyValidation(t *testing.T) {
	valid := Config{
		Host:        "database.internal",
		Port:        "5432",
		SSLMode:     "verify-full",
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
	}
	for _, config := range tests {
		if err := config.ValidateSafety(); err == nil {
			t.Errorf("expected unsafe config to fail: %+v", config)
		}
	}
}
