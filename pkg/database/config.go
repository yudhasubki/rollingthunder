package database

type Config struct {
	// Connection metadata
	Name  string `json:"name"`  // Connection display name
	Color string `json:"color"` // Connection color (hex)

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
}
