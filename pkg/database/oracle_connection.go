package database

// OracleTNSSelection is returned by the native tnsnames.ora picker.
type OracleTNSSelection struct {
	Path    string   `json:"path"`
	Aliases []string `json:"aliases"`
}

// OracleWalletSelection describes a reviewed Oracle Wallet directory without
// exposing or persisting its password.
type OracleWalletSelection struct {
	Path             string `json:"path"`
	HasAutoLogin     bool   `json:"hasAutoLogin"`
	PasswordRequired bool   `json:"passwordRequired"`
}
