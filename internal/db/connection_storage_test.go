package db

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"rollingthunder/pkg/database"
)

type memoryCredentialStore struct {
	mu        sync.Mutex
	values    map[string]string
	setErr    error
	getErr    error
	deleteErr error
}

func newMemoryCredentialStore() *memoryCredentialStore {
	return &memoryCredentialStore{values: make(map[string]string)}
}

func (store *memoryCredentialStore) Set(profileID string, password string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.setErr != nil {
		return store.setErr
	}
	store.values[profileID] = password
	return nil
}

func (store *memoryCredentialStore) Get(profileID string) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.getErr != nil {
		return "", store.getErr
	}
	password, ok := store.values[profileID]
	if !ok {
		return "", ErrCredentialNotFound
	}
	return password, nil
}

func (store *memoryCredentialStore) Delete(profileID string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.deleteErr != nil {
		return store.deleteErr
	}
	if _, ok := store.values[profileID]; !ok {
		return ErrCredentialNotFound
	}
	delete(store.values, profileID)
	return nil
}

func credentialTestService(t *testing.T) (*Service, *memoryCredentialStore) {
	t.Helper()
	service := NewService()
	service.Start(context.Background())
	service.connectionStorage = &ConnectionStorage{
		FilePath: filepath.Join(t.TempDir(), "connections.json"),
	}
	credentials := newMemoryCredentialStore()
	service.credentialStore = credentials
	return service, credentials
}

func TestSavedConnectionStoresPasswordOutsideProfileFile(t *testing.T) {
	service, credentials := credentialTestService(t)
	saved := service.SaveConnection(database.Config{
		Name:     "Production",
		Driver:   "postgres",
		Host:     "db.internal",
		User:     "rolling",
		Password: "super-secret-value",
		Db:       "thunder",
	})
	if len(saved.Errors) > 0 {
		t.Fatalf("SaveConnection() errors = %+v", saved.Errors)
	}
	if saved.Data.Config.Password != "" || !saved.Data.HasPassword {
		t.Fatalf("saved profile leaked password metadata: %+v", saved.Data)
	}
	password, err := credentials.Get(saved.Data.ID)
	if err != nil || password != "super-secret-value" {
		t.Fatalf("credential = %q, %v", password, err)
	}
	data, err := os.ReadFile(service.connectionStorage.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "super-secret-value") ||
		!strings.Contains(string(data), `"version": 7`) {
		t.Fatalf("profile file = %s", data)
	}
	info, err := os.Stat(service.connectionStorage.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("profile permissions = %o, want 600", info.Mode().Perm())
	}

	updated := service.UpdateConnection(saved.Data.ID, database.Config{
		Name:   "Renamed",
		Driver: "postgres",
		Host:   "db.internal",
		User:   "rolling",
		Db:     "thunder",
	})
	if len(updated.Errors) > 0 || !updated.Data.HasPassword {
		t.Fatalf("UpdateConnection() = %+v", updated)
	}
	password, err = credentials.Get(saved.Data.ID)
	if err != nil || password != "super-secret-value" {
		t.Fatalf("preserved credential = %q, %v", password, err)
	}
}

func TestPasswordlessSQLServerProfileRemovesStoredPassword(t *testing.T) {
	service, credentials := credentialTestService(t)
	saved := service.SaveConnection(database.Config{
		Name:              "SQL Server",
		Driver:            database.DriverSQLServer,
		Host:              "sql.internal",
		Port:              "1433",
		User:              "sa",
		Password:          "sql-secret",
		Db:                "rolling",
		SSLMode:           "require",
		SQLServerAuthMode: database.SQLServerAuthSQL,
	})
	if len(saved.Errors) > 0 || !saved.Data.HasPassword {
		t.Fatalf("SaveConnection() = %+v", saved)
	}
	updated := service.UpdateConnection(saved.Data.ID, database.Config{
		Name:              "SQL Server",
		Driver:            database.DriverSQLServer,
		Host:              "sql.internal",
		Port:              "1433",
		Db:                "rolling",
		SSLMode:           "verify-full",
		SQLServerAuthMode: database.SQLServerAuthEntraDefault,
	})
	if len(updated.Errors) > 0 {
		t.Fatalf("UpdateConnection() = %+v", updated.Errors)
	}
	if updated.Data.HasPassword {
		t.Fatal("passwordless profile retained a credential flag")
	}
	if _, err := credentials.Get(saved.Data.ID); !errors.Is(
		err,
		ErrCredentialNotFound,
	) {
		t.Fatalf("passwordless profile retained credential: %v", err)
	}
}

func TestEditingEntraProfilePreservesStoredSecret(t *testing.T) {
	service, credentials := credentialTestService(t)
	saved := service.SaveConnection(database.Config{
		Name:                   "SQL Server Entra",
		Driver:                 database.DriverSQLServer,
		Host:                   "tenant.database.windows.net",
		Port:                   "1433",
		User:                   "operator@example.com",
		Password:               "entra-secret",
		Db:                     "rolling",
		SSLMode:                "verify-full",
		SQLServerAuthMode:      database.SQLServerAuthEntraPassword,
		SQLServerEntraClientID: "application-id",
	})
	if len(saved.Errors) > 0 || !saved.Data.HasPassword {
		t.Fatalf("SaveConnection() = %+v", saved)
	}
	config := saved.Data.Config
	config.Name = "Renamed SQL Server Entra"
	updated := service.UpdateConnection(saved.Data.ID, config)
	if len(updated.Errors) > 0 || !updated.Data.HasPassword {
		t.Fatalf("UpdateConnection() = %+v", updated)
	}
	if password, err := credentials.Get(saved.Data.ID); err != nil ||
		password != "entra-secret" {
		t.Fatalf("preserved Entra credential = %q, %v", password, err)
	}
}

func TestSavedConnectionStoresSSHSecretsOutsideProfileFile(t *testing.T) {
	service, credentials := credentialTestService(t)
	saved := service.SaveConnection(database.Config{
		Name:                  "Private production",
		Driver:                "postgres",
		Host:                  "database.internal",
		Port:                  "5432",
		Password:              "database-secret",
		Db:                    "rolling",
		SSHEnabled:            true,
		SSHHost:               "bastion.example",
		SSHPort:               "22",
		SSHUser:               "deploy",
		SSHAuthMode:           "password",
		SSHPassword:           "ssh-secret",
		SSHKeyPassphrase:      "unused-passphrase",
		SSHHostKeyFingerprint: "SHA256:trusted",
	})
	if len(saved.Errors) > 0 {
		t.Fatalf("SaveConnection() errors = %+v", saved.Errors)
	}
	if !saved.Data.HasPassword || !saved.Data.HasSSHPassword ||
		saved.Data.HasSSHKeyPassphrase {
		t.Fatalf("saved secret flags = %+v", saved.Data)
	}
	if saved.Data.Config.Password != "" ||
		saved.Data.Config.SSHPassword != "" ||
		saved.Data.Config.SSHKeyPassphrase != "" {
		t.Fatalf("saved profile exposed credentials: %+v", saved.Data.Config)
	}
	if password, err := credentials.Get(saved.Data.ID); err != nil ||
		password != "database-secret" {
		t.Fatalf("database credential = %q, %v", password, err)
	}
	if password, err := credentials.Get(
		sshPasswordCredentialID(saved.Data.ID),
	); err != nil || password != "ssh-secret" {
		t.Fatalf("SSH credential = %q, %v", password, err)
	}
	if _, err := credentials.Get(
		sshPassphraseCredentialID(saved.Data.ID),
	); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("unused passphrase was stored: %v", err)
	}
	data, err := os.ReadFile(service.connectionStorage.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{
		"database-secret",
		"ssh-secret",
		"unused-passphrase",
	} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("profile file contains %q: %s", secret, data)
		}
	}
}

func TestSavedConnectionStoresOracleWalletPasswordOutsideProfileFile(
	t *testing.T,
) {
	service, credentials := credentialTestService(t)
	saved := service.SaveConnection(database.Config{
		Name:                 "Oracle production",
		Driver:               database.DriverOracle,
		Host:                 "oracle.internal",
		Port:                 "1521",
		User:                 "rolling",
		Password:             "database-secret",
		Db:                   "FREEPDB1",
		SSLMode:              "verify-full",
		OracleConnectionMode: "direct",
		OracleWalletPath:     "/secure/oracle-wallet",
		OracleWalletPassword: "wallet-secret",
	})
	if len(saved.Errors) > 0 {
		t.Fatalf("SaveConnection() errors = %+v", saved.Errors)
	}
	if !saved.Data.HasOracleWalletPassword ||
		saved.Data.Config.OracleWalletPassword != "" {
		t.Fatalf("saved Wallet metadata = %+v", saved.Data)
	}
	credentialID := oracleWalletPasswordCredentialID(saved.Data.ID)
	if password, err := credentials.Get(credentialID); err != nil ||
		password != "wallet-secret" {
		t.Fatalf("Wallet credential = %q, %v", password, err)
	}
	data, err := os.ReadFile(service.connectionStorage.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "wallet-secret") {
		t.Fatalf("profile file exposed Wallet password: %s", data)
	}
	hydrated, err := service.hydrateProfileCredentials(
		saved.Data,
		saved.Data.Config,
	)
	if err != nil {
		t.Fatal(err)
	}
	if hydrated.OracleWalletPassword != "wallet-secret" {
		t.Fatalf(
			"hydrated Wallet password = %q",
			hydrated.OracleWalletPassword,
		)
	}

	updated := service.UpdateConnection(saved.Data.ID, saved.Data.Config)
	if len(updated.Errors) > 0 ||
		!updated.Data.HasOracleWalletPassword {
		t.Fatalf("UpdateConnection() = %+v", updated)
	}
	if password, err := credentials.Get(credentialID); err != nil ||
		password != "wallet-secret" {
		t.Fatalf("preserved Wallet credential = %q, %v", password, err)
	}
}

func TestChangingOracleWalletClearsUnchangedStoredPassword(t *testing.T) {
	service, credentials := credentialTestService(t)
	saved := service.SaveConnection(database.Config{
		Name:                 "Oracle",
		Driver:               database.DriverOracle,
		Db:                   "FREEPDB1",
		SSLMode:              "verify-full",
		OracleConnectionMode: "direct",
		OracleWalletPath:     "/wallet/one",
		OracleWalletPassword: "wallet-one-secret",
	})
	if len(saved.Errors) > 0 {
		t.Fatalf("SaveConnection() errors = %+v", saved.Errors)
	}
	config := saved.Data.Config
	config.OracleWalletPath = "/wallet/two"
	updated := service.UpdateConnection(saved.Data.ID, config)
	if len(updated.Errors) > 0 ||
		updated.Data.HasOracleWalletPassword {
		t.Fatalf("UpdateConnection() = %+v", updated)
	}
	if _, err := credentials.Get(
		oracleWalletPasswordCredentialID(saved.Data.ID),
	); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("old Wallet credential still exists: %v", err)
	}
}

func TestActiveConnectionJSONCannotExposeConfigurationSecrets(t *testing.T) {
	data, err := json.Marshal(Connection{
		ID:   "active",
		Name: "Private",
		Config: database.Config{
			Password:             "database-secret",
			SSHPassword:          "ssh-secret",
			SSHKeyPassphrase:     "passphrase-secret",
			OracleWalletPassword: "wallet-secret",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{
		"database-secret",
		"ssh-secret",
		"passphrase-secret",
		"wallet-secret",
	} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("active connection JSON exposed %q: %s", secret, data)
		}
	}
}

func TestConnectSavedConnectionHydratesSSHCredentialsBackendSide(t *testing.T) {
	service, _ := credentialTestService(t)
	saved := service.SaveConnection(database.Config{
		Name:              "Private production",
		Driver:            "postgres",
		Host:              "database.internal",
		Port:              "5432",
		Password:          "database-secret",
		Db:                "rolling",
		SSHEnabled:        true,
		SSHHost:           "bastion.example",
		SSHUser:           "deploy",
		SSHAuthMode:       "private-key",
		SSHPrivateKeyPath: "~/.ssh/id_ed25519",
		SSHKeyPassphrase:  "key-secret",
	})
	if len(saved.Errors) > 0 {
		t.Fatalf("SaveConnection() errors = %+v", saved.Errors)
	}
	tunnel := &fakeConnectionTunnel{host: "127.0.0.1", port: "41003"}
	var tunnelConfig database.Config
	service.newTunnel = func(
		_ context.Context,
		config database.Config,
	) (connectionTunnel, error) {
		tunnelConfig = config
		return tunnel, nil
	}
	driver := &connectionTestDriver{}
	service.newDriver = func(
		context.Context,
		string,
		database.Config,
	) (database.Driver, error) {
		return driver, nil
	}

	connected := service.ConnectSavedConnection(saved.Data.ID, "ssh-saved")
	if len(connected.Errors) > 0 || !connected.Data.Connected {
		t.Fatalf("ConnectSavedConnection() = %+v", connected)
	}
	if tunnelConfig.Password != "database-secret" ||
		tunnelConfig.SSHKeyPassphrase != "key-secret" {
		t.Fatalf("hydrated config = %+v", tunnelConfig)
	}
	if tunnelConfig.SSHPassword != "" {
		t.Fatalf("unexpected SSH password = %q", tunnelConfig.SSHPassword)
	}
}

func TestClearSSHCredentialsPreservesDatabasePassword(t *testing.T) {
	service, credentials := credentialTestService(t)
	saved := service.SaveConnection(database.Config{
		Name:        "Private production",
		Driver:      "postgres",
		Password:    "database-secret",
		SSHEnabled:  true,
		SSHAuthMode: "password",
		SSHPassword: "ssh-secret",
	})
	if len(saved.Errors) > 0 {
		t.Fatalf("SaveConnection() errors = %+v", saved.Errors)
	}

	cleared := service.ClearConnectionSSHCredentials(saved.Data.ID)
	if len(cleared.Errors) > 0 || !cleared.Data {
		t.Fatalf("ClearConnectionSSHCredentials() = %+v", cleared)
	}
	if password, err := credentials.Get(saved.Data.ID); err != nil ||
		password != "database-secret" {
		t.Fatalf("database credential = %q, %v", password, err)
	}
	if _, err := credentials.Get(
		sshPasswordCredentialID(saved.Data.ID),
	); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("SSH credential still exists: %v", err)
	}
	loaded := service.GetSavedConnections()
	if len(loaded.Errors) > 0 || len(loaded.Data) != 1 ||
		loaded.Data[0].HasSSHPassword ||
		!loaded.Data[0].HasPassword {
		t.Fatalf("saved profile after clear = %+v", loaded)
	}
}

func TestLegacyPlaintextPasswordMigratesBeforeProfileRewrite(t *testing.T) {
	service, credentials := credentialTestService(t)
	legacy := []SavedConnection{
		{
			ID: "legacy-profile",
			Config: database.Config{
				Name:     "Legacy",
				Driver:   "postgres",
				Password: "legacy-password",
				Db:       "legacy",
			},
		},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(service.connectionStorage.FilePath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded := service.GetSavedConnections()
	if len(loaded.Errors) > 0 {
		t.Fatalf("GetSavedConnections() errors = %+v", loaded.Errors)
	}
	if len(loaded.Data) != 1 || !loaded.Data[0].HasPassword ||
		loaded.Data[0].Config.Password != "" {
		t.Fatalf("loaded profiles = %+v", loaded.Data)
	}
	password, err := credentials.Get("legacy-profile")
	if err != nil || password != "legacy-password" {
		t.Fatalf("migrated credential = %q, %v", password, err)
	}
	migrated, err := os.ReadFile(service.connectionStorage.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(migrated), "legacy-password") ||
		!strings.Contains(string(migrated), `"version": 7`) {
		t.Fatalf("migrated profile file = %s", migrated)
	}
}

func TestLegacyProfileWithoutPasswordStillMigratesToVersionedEnvelope(t *testing.T) {
	service, _ := credentialTestService(t)
	legacy := `[{"id":"legacy-profile","config":{"name":"Legacy","driver":"sqlite","db":"legacy.db"}}]`
	if err := os.WriteFile(
		service.connectionStorage.FilePath,
		[]byte(legacy),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	loaded := service.GetSavedConnections()
	if len(loaded.Errors) > 0 || len(loaded.Data) != 1 {
		t.Fatalf("GetSavedConnections() = %+v", loaded)
	}
	migrated, err := os.ReadFile(service.connectionStorage.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(migrated), `"version": 7`) {
		t.Fatalf("legacy profile was not versioned: %s", migrated)
	}
	info, err := os.Stat(service.connectionStorage.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("migrated profile permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestVersionFiveOracleWalletPasswordMigratesToCredentialStore(
	t *testing.T,
) {
	service, credentials := credentialTestService(t)
	legacy := connectionStorageEnvelope{
		Version: 5,
		Connections: []SavedConnection{{
			ID: "oracle-wallet-profile",
			Config: database.Config{
				Name:                 "Oracle",
				Driver:               database.DriverOracle,
				Db:                   "FREEPDB1",
				OracleConnectionMode: "direct",
				OracleWalletPath:     "/secure/oracle-wallet",
				OracleWalletPassword: "legacy-wallet-secret",
			},
		}},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		service.connectionStorage.FilePath,
		data,
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	loaded := service.GetSavedConnections()
	if len(loaded.Errors) > 0 ||
		len(loaded.Data) != 1 ||
		!loaded.Data[0].HasOracleWalletPassword ||
		loaded.Data[0].Config.OracleWalletPassword != "" {
		t.Fatalf("GetSavedConnections() = %+v", loaded)
	}
	if password, err := credentials.Get(
		oracleWalletPasswordCredentialID("oracle-wallet-profile"),
	); err != nil || password != "legacy-wallet-secret" {
		t.Fatalf("migrated Wallet credential = %q, %v", password, err)
	}
	migrated, err := os.ReadFile(service.connectionStorage.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(migrated), "legacy-wallet-secret") ||
		!strings.Contains(string(migrated), `"version": 7`) {
		t.Fatalf("migrated profile file = %s", migrated)
	}
}

func TestVersionSixPasswordlessSQLServerProfileDropsObsoleteCredential(
	t *testing.T,
) {
	service, credentials := credentialTestService(t)
	const profileID = "integrated-sqlserver"
	if err := credentials.Set(profileID, "obsolete-password"); err != nil {
		t.Fatal(err)
	}
	legacy := connectionStorageEnvelope{
		Version: 6,
		Connections: []SavedConnection{{
			ID:          profileID,
			HasPassword: true,
			Config: database.Config{
				Name:              "Integrated SQL Server",
				Driver:            database.DriverSQLServer,
				Db:                "rolling",
				SSLMode:           "require",
				SQLServerAuthMode: database.SQLServerAuthIntegrated,
			},
		}},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		service.connectionStorage.FilePath,
		data,
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	loaded := service.GetSavedConnections()
	if len(loaded.Errors) > 0 ||
		len(loaded.Data) != 1 ||
		loaded.Data[0].HasPassword {
		t.Fatalf("GetSavedConnections() = %+v", loaded)
	}
	if _, err := credentials.Get(profileID); !errors.Is(
		err,
		ErrCredentialNotFound,
	) {
		t.Fatalf("obsolete credential remains available: %v", err)
	}
	migrated, err := os.ReadFile(service.connectionStorage.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(migrated), `"version": 7`) {
		t.Fatalf("profile was not migrated to version 7: %s", migrated)
	}
}

func TestPasswordlessProfileMigrationFailsClosedWhenCredentialDeleteFails(
	t *testing.T,
) {
	service, credentials := credentialTestService(t)
	const profileID = "integrated-sqlserver"
	if err := credentials.Set(profileID, "keep-on-failure"); err != nil {
		t.Fatal(err)
	}
	credentials.deleteErr = errors.New("credential service denied deletion")
	legacy := connectionStorageEnvelope{
		Version: 6,
		Connections: []SavedConnection{{
			ID:          profileID,
			HasPassword: true,
			Config: database.Config{
				Name:              "Integrated SQL Server",
				Driver:            database.DriverSQLServer,
				Db:                "rolling",
				SSLMode:           "require",
				SQLServerAuthMode: database.SQLServerAuthIntegrated,
			},
		}},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		service.connectionStorage.FilePath,
		data,
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	loaded := service.GetSavedConnections()
	if len(loaded.Errors) == 0 {
		t.Fatalf("GetSavedConnections() unexpectedly succeeded: %+v", loaded)
	}
	unchanged, err := os.ReadFile(service.connectionStorage.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(unchanged), `"version":6`) ||
		!strings.Contains(string(unchanged), `"hasPassword":true`) {
		t.Fatalf("failed migration changed source metadata: %s", unchanged)
	}
	credentials.deleteErr = nil
	if password, err := credentials.Get(profileID); err != nil ||
		password != "keep-on-failure" {
		t.Fatalf("credential after failed migration = %q, %v", password, err)
	}
}

func TestVersionedProfilePermissionsAreTightenedOnLoad(t *testing.T) {
	service, _ := credentialTestService(t)
	versioned := `{"version":2,"connections":[]}`
	if err := os.WriteFile(
		service.connectionStorage.FilePath,
		[]byte(versioned),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	loaded := service.GetSavedConnections()
	if len(loaded.Errors) > 0 {
		t.Fatalf("GetSavedConnections() = %+v", loaded)
	}
	info, err := os.Stat(service.connectionStorage.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("profile permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestLegacyMigrationDoesNotErasePasswordWhenCredentialStoreFails(t *testing.T) {
	service, credentials := credentialTestService(t)
	credentials.setErr = errors.New("credential service unavailable")
	legacy := `[{"id":"legacy-profile","config":{"name":"Legacy","driver":"postgres","password":"keep-me"}}]`
	if err := os.WriteFile(
		service.connectionStorage.FilePath,
		[]byte(legacy),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	loaded := service.GetSavedConnections()
	if len(loaded.Errors) == 0 {
		t.Fatal("GetSavedConnections() unexpectedly succeeded")
	}
	unchanged, err := os.ReadFile(service.connectionStorage.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(unchanged), "keep-me") {
		t.Fatalf("failed migration erased legacy password: %s", unchanged)
	}
}

func TestVersionThreeProfileMigratesToSemanticEnvironment(t *testing.T) {
	service, _ := credentialTestService(t)
	legacy := `{"version":3,"connections":[{"id":"legacy-color","config":{"name":"Legacy","driver":"postgres","color":"#ff00ff","environment":"custom"}}]}`
	if err := os.WriteFile(
		service.connectionStorage.FilePath,
		[]byte(legacy),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	loaded := service.GetSavedConnections()
	if len(loaded.Errors) > 0 || len(loaded.Data) != 1 {
		t.Fatalf("GetSavedConnections() = %+v", loaded)
	}
	config := loaded.Data[0].Config
	if config.Environment != database.ConnectionEnvironmentUnclassified {
		t.Fatalf("environment = %q", config.Environment)
	}
	if config.Color != "" {
		t.Fatalf("legacy color was exposed: %q", config.Color)
	}
	if config.AccessMode != database.ConnectionAccessReadWrite {
		t.Fatalf("access mode = %q", config.AccessMode)
	}
	migrated, err := os.ReadFile(service.connectionStorage.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(migrated)
	if !strings.Contains(text, `"version": 7`) ||
		!strings.Contains(text, `"environment": "unclassified"`) ||
		!strings.Contains(text, `"accessMode": "read-write"`) ||
		strings.Contains(text, "#ff00ff") {
		t.Fatalf("migrated profile file = %s", migrated)
	}
}

func TestVersionFourProductionProfileDefaultsToReadOnly(t *testing.T) {
	service, _ := credentialTestService(t)
	legacy := `{"version":4,"connections":[{"id":"production","config":{"name":"Production","driver":"postgres","environment":"production","folder":"  Core  ","tags":["Critical","critical","Billing"]}}]}`
	if err := os.WriteFile(
		service.connectionStorage.FilePath,
		[]byte(legacy),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	loaded := service.GetSavedConnections()
	if len(loaded.Errors) > 0 || len(loaded.Data) != 1 {
		t.Fatalf("GetSavedConnections() = %+v", loaded)
	}
	config := loaded.Data[0].Config
	if config.AccessMode != database.ConnectionAccessReadOnly {
		t.Fatalf("access mode = %q", config.AccessMode)
	}
	if config.Folder != "Core" {
		t.Fatalf("folder = %q", config.Folder)
	}
	if len(config.Tags) != 2 ||
		config.Tags[0] != "Critical" ||
		config.Tags[1] != "Billing" {
		t.Fatalf("tags = %#v", config.Tags)
	}
}

func TestFutureProfileVersionFailsClosedWithoutRewriting(t *testing.T) {
	service, _ := credentialTestService(t)
	future := `{"version":99,"connections":[{"id":"future-profile","config":{"name":"Future"}}]}`
	if err := os.WriteFile(
		service.connectionStorage.FilePath,
		[]byte(future),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	loaded := service.GetSavedConnections()
	if len(loaded.Errors) == 0 {
		t.Fatal("GetSavedConnections() unexpectedly accepted a future version")
	}
	unchanged, err := os.ReadFile(service.connectionStorage.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != future {
		t.Fatalf("future profile was modified: %s", unchanged)
	}
}

func TestConnectSavedConnectionResolvesCredentialBackendSide(t *testing.T) {
	service, credentials := credentialTestService(t)
	saved := service.SaveConnection(database.Config{
		Name:     "Secure",
		Driver:   "postgres",
		Password: "resolved-secret",
		Db:       "rolling",
	})
	if len(saved.Errors) > 0 {
		t.Fatalf("SaveConnection() errors = %+v", saved.Errors)
	}
	var received database.Config
	driver := &connectionTestDriver{}
	service.newDriver = func(
		_ context.Context,
		_ string,
		config database.Config,
	) (database.Driver, error) {
		received = config
		return driver, nil
	}

	connected := service.ConnectSavedConnection(saved.Data.ID, "saved-attempt")
	if len(connected.Errors) > 0 || !connected.Data.Connected {
		t.Fatalf("ConnectSavedConnection() = %+v", connected)
	}
	if received.Password != "resolved-secret" {
		t.Fatalf("driver received password %q", received.Password)
	}
	active := service.GetActiveConnections()
	if len(active.Data) != 1 || active.Data[0].ProfileID != saved.Data.ID {
		t.Fatalf("active connection = %+v", active.Data)
	}
	if password, err := credentials.Get(saved.Data.ID); err != nil ||
		password != "resolved-secret" {
		t.Fatalf("stored credential = %q, %v", password, err)
	}
}

func TestClearConnectionPasswordRemovesCredentialAndProfileFlag(t *testing.T) {
	service, credentials := credentialTestService(t)
	saved := service.SaveConnection(database.Config{
		Name:     "Disposable",
		Driver:   "postgres",
		Password: "remove-me",
		Db:       "rolling",
	})
	if len(saved.Errors) > 0 {
		t.Fatalf("SaveConnection() errors = %+v", saved.Errors)
	}

	cleared := service.ClearConnectionPassword(saved.Data.ID)
	if len(cleared.Errors) > 0 || !cleared.Data {
		t.Fatalf("ClearConnectionPassword() = %+v", cleared)
	}
	if _, err := credentials.Get(saved.Data.ID); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("credential still available: %v", err)
	}
	loaded := service.GetSavedConnections()
	if len(loaded.Errors) > 0 || len(loaded.Data) != 1 ||
		loaded.Data[0].HasPassword {
		t.Fatalf("saved profile after clear = %+v", loaded)
	}
}

func TestClearConnectionOracleWalletPasswordRemovesCredentialAndFlag(
	t *testing.T,
) {
	service, credentials := credentialTestService(t)
	saved := service.SaveConnection(database.Config{
		Name:                 "Oracle",
		Driver:               database.DriverOracle,
		Db:                   "FREEPDB1",
		SSLMode:              "verify-full",
		OracleConnectionMode: "direct",
		OracleWalletPath:     "/secure/oracle-wallet",
		OracleWalletPassword: "remove-wallet-secret",
	})
	if len(saved.Errors) > 0 {
		t.Fatalf("SaveConnection() errors = %+v", saved.Errors)
	}

	cleared := service.ClearConnectionOracleWalletPassword(saved.Data.ID)
	if len(cleared.Errors) > 0 || !cleared.Data {
		t.Fatalf(
			"ClearConnectionOracleWalletPassword() = %+v",
			cleared,
		)
	}
	if _, err := credentials.Get(
		oracleWalletPasswordCredentialID(saved.Data.ID),
	); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Wallet credential still available: %v", err)
	}
	loaded := service.GetSavedConnections()
	if len(loaded.Errors) > 0 ||
		len(loaded.Data) != 1 ||
		loaded.Data[0].HasOracleWalletPassword {
		t.Fatalf("saved profile after clear = %+v", loaded)
	}
}

func TestDeleteProfileKeepsMetadataWhenCredentialRemovalFails(t *testing.T) {
	service, credentials := credentialTestService(t)
	saved := service.SaveConnection(database.Config{
		Name:     "Protected",
		Driver:   "postgres",
		Password: "keep-until-atomic",
		Db:       "rolling",
	})
	if len(saved.Errors) > 0 {
		t.Fatalf("SaveConnection() errors = %+v", saved.Errors)
	}
	credentials.deleteErr = errors.New("credential service denied deletion")

	deleted := service.DeleteConnection(saved.Data.ID)
	if len(deleted.Errors) == 0 {
		t.Fatal("DeleteConnection() unexpectedly succeeded")
	}
	credentials.deleteErr = nil
	loaded := service.GetSavedConnections()
	if len(loaded.Errors) > 0 || len(loaded.Data) != 1 ||
		loaded.Data[0].ID != saved.Data.ID {
		t.Fatalf("profile was removed after credential failure: %+v", loaded)
	}
	password, err := credentials.Get(saved.Data.ID)
	if err != nil || password != "keep-until-atomic" {
		t.Fatalf("credential after failed deletion = %q, %v", password, err)
	}
}
