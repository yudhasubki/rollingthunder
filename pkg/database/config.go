package database

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const (
	DriverPostgres  = "postgres"
	DriverMySQL     = "mysql"
	DriverMariaDB   = "mariadb"
	DriverSQLite    = "sqlite"
	DriverOracle    = "oracle"
	DriverSQLServer = "sqlserver"

	DefaultHost          = "127.0.0.1"
	DefaultPostgresPort  = "5432"
	DefaultMySQLPort     = "3306"
	DefaultOraclePort    = "1521"
	DefaultSQLServerPort = "1433"
	DefaultSSHPort       = "22"
	DefaultSSLMode       = "disable"

	ConnectionEnvironmentUnclassified = "unclassified"
	ConnectionEnvironmentDevelopment  = "development"
	ConnectionEnvironmentStaging      = "staging"
	ConnectionEnvironmentProduction   = "production"

	ConnectionAccessReadWrite = "read-write"
	ConnectionAccessReadOnly  = "read-only"

	MaxConnectionTags = 32
)

type Config struct {
	// Connection metadata
	Name        string   `json:"name"`        // Connection display name
	Environment string   `json:"environment"` // Operational risk classification
	AccessMode  string   `json:"accessMode"`  // read-write, read-only
	Folder      string   `json:"folder,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Driver      string   `json:"driver"` // postgres, mysql, sqlite, oracle, sqlserver

	// Color is retained only to decode profiles written before environment
	// classifications were introduced. New profiles never persist or render it.
	Color string `json:"color,omitempty"`

	// Basic connection
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Db       string `json:"db"`

	// SSL options
	SSLMode     string `json:"sslMode"`     // disable, require, verify-ca, verify-full
	SSLCert     string `json:"sslCert"`     // Client certificate path
	SSLKey      string `json:"sslKey"`      // Client key path
	SSLRootCert string `json:"sslRootCert"` // CA certificate path

	// SSH tunnel options. SSHPassword and SSHKeyPassphrase are transient
	// secrets; saved profiles persist them only in the operating system
	// credential store.
	SSHEnabled            bool   `json:"sshEnabled"`
	SSHHost               string `json:"sshHost"`
	SSHPort               string `json:"sshPort"`
	SSHUser               string `json:"sshUser"`
	SSHAuthMode           string `json:"sshAuthMode"` // agent, private-key, password
	SSHPrivateKeyPath     string `json:"sshPrivateKeyPath"`
	SSHKnownHostsPath     string `json:"sshKnownHostsPath"`
	SSHHostKeyFingerprint string `json:"sshHostKeyFingerprint"`
	SSHPassword           string `json:"sshPassword"`
	SSHKeyPassphrase      string `json:"sshKeyPassphrase"`

	// TLSServerName is set internally when Host is replaced by a local SSH
	// endpoint. It preserves certificate hostname verification and is never
	// persisted or exposed to the frontend.
	TLSServerName string `json:"-"`
}

// NormalizeConnectionEnvironment returns a known operational classification.
// Unknown or legacy values deliberately become unclassified instead of
// inheriting a potentially unsafe production/development assumption.
func NormalizeConnectionEnvironment(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ConnectionEnvironmentDevelopment:
		return ConnectionEnvironmentDevelopment
	case ConnectionEnvironmentStaging:
		return ConnectionEnvironmentStaging
	case ConnectionEnvironmentProduction:
		return ConnectionEnvironmentProduction
	default:
		return ConnectionEnvironmentUnclassified
	}
}

// NormalizeConnectionAccessMode defaults production profiles to read-only.
// Existing non-production profiles remain read-write when the field is absent.
func NormalizeConnectionAccessMode(value, environment string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ConnectionAccessReadOnly:
		return ConnectionAccessReadOnly
	case ConnectionAccessReadWrite:
		return ConnectionAccessReadWrite
	default:
		if NormalizeConnectionEnvironment(environment) == ConnectionEnvironmentProduction {
			return ConnectionAccessReadOnly
		}
		return ConnectionAccessReadWrite
	}
}

func NormalizeConnectionTags(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, min(len(values), MaxConnectionTags))
	seen := make(map[string]struct{}, min(len(values), MaxConnectionTags))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, value)
		if len(normalized) == MaxConnectionTags {
			break
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

// NormalizeConfigMetadata removes deprecated presentation-only data and makes
// the operational classification safe to consume throughout the application.
func NormalizeConfigMetadata(config Config) Config {
	config.Environment = NormalizeConnectionEnvironment(config.Environment)
	config.AccessMode = NormalizeConnectionAccessMode(
		config.AccessMode,
		config.Environment,
	)
	config.Folder = strings.TrimSpace(config.Folder)
	config.Tags = NormalizeConnectionTags(config.Tags)
	config.Color = ""
	return config
}

func ConfigMetadataEqual(left, right Config) bool {
	return left.Environment == right.Environment &&
		left.AccessMode == right.AccessMode &&
		left.Folder == right.Folder &&
		slices.Equal(left.Tags, right.Tags) &&
		left.Color == right.Color
}

func validateConfigText(name, value string, maxBytes int) error {
	if len(value) > maxBytes {
		return fmt.Errorf("%s exceeds the %d-byte safety limit", name, maxBytes)
	}
	if strings.IndexFunc(value, func(character rune) bool {
		return character < 0x20 || character == 0x7f
	}) >= 0 {
		return fmt.Errorf("%s contains control characters", name)
	}
	return nil
}

func validateConfigPort(name, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("%s must be a number from 1 to 65535", name)
	}
	return nil
}

// ValidateSafety rejects control characters, oversized metadata, ambiguous TLS
// modes, and invalid network ports before values reach a driver or external
// maintenance command. Passwords remain opaque because line breaks are valid
// credentials for native database connections.
func (config Config) ValidateSafety() error {
	textFields := []struct {
		name     string
		value    string
		maxBytes int
	}{
		{"connection name", config.Name, 256},
		{"connection folder", config.Folder, 256},
		{"driver", config.Driver, 32},
		{"database host", config.Host, 255},
		{"database user", config.User, 256},
		{"database name or path", config.Db, 4096},
		{"TLS mode", config.SSLMode, 32},
		{"TLS client certificate path", config.SSLCert, 4096},
		{"TLS client key path", config.SSLKey, 4096},
		{"TLS CA certificate path", config.SSLRootCert, 4096},
		{"SSH host", config.SSHHost, 255},
		{"SSH user", config.SSHUser, 256},
		{"SSH authentication mode", config.SSHAuthMode, 32},
		{"SSH private key path", config.SSHPrivateKeyPath, 4096},
		{"SSH known-hosts path", config.SSHKnownHostsPath, 4096},
		{"SSH host-key fingerprint", config.SSHHostKeyFingerprint, 512},
	}
	for _, field := range textFields {
		if err := validateConfigText(field.name, field.value, field.maxBytes); err != nil {
			return err
		}
	}
	if len(config.Tags) > MaxConnectionTags {
		return fmt.Errorf(
			"connection profile supports at most %d tags",
			MaxConnectionTags,
		)
	}
	for _, tag := range config.Tags {
		if err := validateConfigText("connection tag", tag, 64); err != nil {
			return err
		}
	}
	switch strings.ToLower(strings.TrimSpace(config.AccessMode)) {
	case "", ConnectionAccessReadOnly, ConnectionAccessReadWrite:
	default:
		return fmt.Errorf("connection access mode is not supported")
	}
	switch strings.ToLower(strings.TrimSpace(config.Driver)) {
	case "",
		DriverPostgres,
		DriverMySQL,
		DriverMariaDB,
		DriverSQLite,
		DriverOracle,
		DriverSQLServer:
	default:
		return fmt.Errorf("database driver is not supported")
	}
	if err := validateConfigPort("database port", config.Port); err != nil {
		return err
	}
	if err := validateConfigPort("SSH port", config.SSHPort); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(config.SSLMode)) {
	case "", "disable", "require", "verify-ca", "verify-full":
	default:
		return fmt.Errorf("TLS mode is not supported")
	}
	if config.SSHEnabled {
		switch strings.ToLower(strings.TrimSpace(config.SSHAuthMode)) {
		case "", "agent", "private-key", "key", "password":
		default:
			return fmt.Errorf("SSH authentication mode is not supported")
		}
	}
	return nil
}
