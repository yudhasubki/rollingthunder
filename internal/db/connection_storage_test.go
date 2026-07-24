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
		!strings.Contains(string(data), `"version": 2`) {
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
		!strings.Contains(string(migrated), `"version": 2`) {
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
	if !strings.Contains(string(migrated), `"version": 2`) {
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
