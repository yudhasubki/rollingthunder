package database

type Config struct {
	// Connection metadata
	Name   string `json:"name"`   // Connection display name
	Color  string `json:"color"`  // Connection color (hex)
	Driver string `json:"driver"` // postgres, mysql, sqlite

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
