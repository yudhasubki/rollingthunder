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

	SQLServerAuthSQL                   = "sql"
	SQLServerAuthIntegrated            = "integrated"
	SQLServerAuthEntraDefault          = "entra-default"
	SQLServerAuthEntraPassword         = "entra-password"
	SQLServerAuthEntraServicePrincipal = "entra-service-principal"
	SQLServerAuthEntraManagedIdentity  = "entra-managed-identity"
	SQLServerAuthEntraAzureCLI         = "entra-azure-cli"

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

	// Oracle connection options. OracleWalletPassword is transient and is
	// persisted only in the operating system credential store.
	OracleConnectionMode string `json:"oracleConnectionMode,omitempty"` // direct, tns
	OracleTNSConfigPath  string `json:"oracleTnsConfigPath,omitempty"`
	OracleTNSAlias       string `json:"oracleTnsAlias,omitempty"`
	OracleWalletPath     string `json:"oracleWalletPath,omitempty"`
	OracleWalletPassword string `json:"oracleWalletPassword,omitempty"`

	// SQL Server authentication options. Password contains either the SQL
	// login password, Entra user password, or service-principal client secret
	// and remains transient in the operating-system credential store.
	SQLServerAuthMode      string `json:"sqlServerAuthMode,omitempty"`
	SQLServerEntraClientID string `json:"sqlServerEntraClientId,omitempty"`
	SQLServerEntraTenantID string `json:"sqlServerEntraTenantId,omitempty"`

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

func NormalizeSQLServerAuthMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SQLServerAuthIntegrated:
		return SQLServerAuthIntegrated
	case SQLServerAuthEntraDefault:
		return SQLServerAuthEntraDefault
	case SQLServerAuthEntraPassword:
		return SQLServerAuthEntraPassword
	case SQLServerAuthEntraServicePrincipal:
		return SQLServerAuthEntraServicePrincipal
	case SQLServerAuthEntraManagedIdentity:
		return SQLServerAuthEntraManagedIdentity
	case SQLServerAuthEntraAzureCLI:
		return SQLServerAuthEntraAzureCLI
	default:
		return SQLServerAuthSQL
	}
}

func (config Config) UsesDatabasePassword() bool {
	if !strings.EqualFold(strings.TrimSpace(config.Driver), DriverSQLServer) {
		return true
	}
	switch NormalizeSQLServerAuthMode(config.SQLServerAuthMode) {
	case SQLServerAuthSQL,
		SQLServerAuthEntraPassword,
		SQLServerAuthEntraServicePrincipal:
		return true
	default:
		return false
	}
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
		{"Oracle connection mode", config.OracleConnectionMode, 32},
		{"Oracle TNS configuration path", config.OracleTNSConfigPath, 4096},
		{"Oracle TNS alias", config.OracleTNSAlias, 256},
		{"Oracle Wallet path", config.OracleWalletPath, 4096},
		{"SQL Server authentication mode", config.SQLServerAuthMode, 64},
		{"SQL Server Entra client ID", config.SQLServerEntraClientID, 256},
		{"SQL Server Entra tenant ID", config.SQLServerEntraTenantID, 256},
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
	oracleMode := strings.ToLower(strings.TrimSpace(config.OracleConnectionMode))
	if strings.EqualFold(config.Driver, DriverOracle) {
		switch oracleMode {
		case "", "direct":
		case "tns":
			if strings.TrimSpace(config.OracleTNSConfigPath) == "" ||
				strings.TrimSpace(config.OracleTNSAlias) == "" {
				return fmt.Errorf(
					"Oracle TNS mode requires a tnsnames.ora file and alias",
				)
			}
			if config.SSHEnabled {
				return fmt.Errorf(
					"Oracle TNS aliases cannot be combined with an SSH tunnel; use a direct endpoint through the tunnel",
				)
			}
		default:
			return fmt.Errorf("Oracle connection mode is not supported")
		}
		if strings.TrimSpace(config.OracleWalletPath) != "" &&
			config.SSHEnabled {
			return fmt.Errorf(
				"Oracle Wallet connections cannot be combined with an SSH tunnel because certificate host verification would be ambiguous",
			)
		}
		if strings.TrimSpace(config.OracleWalletPath) == "" &&
			config.OracleWalletPassword != "" {
			return fmt.Errorf(
				"Oracle Wallet password requires a selected Wallet directory",
			)
		}
		if strings.TrimSpace(config.OracleWalletPath) != "" {
			if strings.TrimSpace(config.SSLMode) == "" ||
				strings.EqualFold(config.SSLMode, "disable") {
				return fmt.Errorf(
					"Oracle Wallet requires an encrypted TLS mode",
				)
			}
			if strings.EqualFold(config.SSLMode, "verify-ca") {
				return fmt.Errorf(
					"Oracle Wallet supports require or verify-full TLS modes; the Oracle driver cannot apply CA-only verification without also checking the endpoint hostname",
				)
			}
			if strings.TrimSpace(config.SSLRootCert) != "" ||
				strings.TrimSpace(config.SSLCert) != "" ||
				strings.TrimSpace(config.SSLKey) != "" {
				return fmt.Errorf(
					"Oracle Wallet cannot be combined with separate TLS certificate paths",
				)
			}
		}
	} else if oracleMode != "" ||
		strings.TrimSpace(config.OracleTNSConfigPath) != "" ||
		strings.TrimSpace(config.OracleTNSAlias) != "" ||
		strings.TrimSpace(config.OracleWalletPath) != "" ||
		config.OracleWalletPassword != "" {
		return fmt.Errorf(
			"Oracle connection options can only be used by the Oracle driver",
		)
	}
	sqlServerMode := strings.ToLower(strings.TrimSpace(config.SQLServerAuthMode))
	sqlServerClientID := strings.TrimSpace(config.SQLServerEntraClientID)
	sqlServerTenantID := strings.TrimSpace(config.SQLServerEntraTenantID)
	if strings.EqualFold(config.Driver, DriverSQLServer) {
		normalizedMode := NormalizeSQLServerAuthMode(sqlServerMode)
		if sqlServerMode != "" && normalizedMode != sqlServerMode {
			return fmt.Errorf("SQL Server authentication mode is not supported")
		}
		switch normalizedMode {
		case SQLServerAuthSQL:
			if strings.TrimSpace(config.User) == "" {
				return fmt.Errorf(
					"SQL Server password authentication requires a username",
				)
			}
			if sqlServerClientID != "" || sqlServerTenantID != "" {
				return fmt.Errorf(
					"SQL Server Entra identifiers require an Entra authentication mode",
				)
			}
		case SQLServerAuthIntegrated:
			if config.SSHEnabled {
				return fmt.Errorf(
					"SQL Server Integrated authentication cannot be combined with SSH because the Kerberos or SSPI server identity would be ambiguous",
				)
			}
			if config.User != "" || config.Password != "" ||
				sqlServerClientID != "" || sqlServerTenantID != "" {
				return fmt.Errorf(
					"SQL Server Integrated authentication uses the current Windows identity and does not accept profile credentials",
				)
			}
		case SQLServerAuthEntraPassword:
			if strings.TrimSpace(config.User) == "" ||
				sqlServerClientID == "" {
				return fmt.Errorf(
					"Microsoft Entra password authentication requires a username and application client ID",
				)
			}
			if sqlServerTenantID != "" {
				return fmt.Errorf(
					"Microsoft Entra password authentication discovers the tenant from SQL Server and does not accept a tenant override",
				)
			}
		case SQLServerAuthEntraServicePrincipal:
			if sqlServerClientID == "" ||
				sqlServerTenantID == "" {
				return fmt.Errorf(
					"Microsoft Entra service-principal authentication requires a client ID and tenant ID",
				)
			}
			if strings.TrimSpace(config.User) != "" {
				return fmt.Errorf(
					"Microsoft Entra service-principal authentication uses the client ID instead of a username",
				)
			}
		case SQLServerAuthEntraManagedIdentity:
			if config.User != "" || config.Password != "" ||
				sqlServerTenantID != "" {
				return fmt.Errorf(
					"Microsoft Entra managed identity accepts only an optional user-assigned client ID",
				)
			}
		case SQLServerAuthEntraDefault, SQLServerAuthEntraAzureCLI:
			if config.User != "" || config.Password != "" ||
				sqlServerClientID != "" || sqlServerTenantID != "" {
				return fmt.Errorf(
					"the selected Microsoft Entra authentication mode does not accept profile credentials",
				)
			}
		}
		if strings.HasPrefix(normalizedMode, "entra-") &&
			(strings.TrimSpace(config.SSLMode) == "" ||
				strings.EqualFold(config.SSLMode, "disable")) {
			return fmt.Errorf(
				"Microsoft Entra authentication requires encrypted SQL Server TLS",
			)
		}
	} else if sqlServerMode != "" ||
		sqlServerClientID != "" ||
		sqlServerTenantID != "" {
		return fmt.Errorf(
			"SQL Server authentication options can only be used by the SQL Server driver",
		)
	}
	return nil
}
