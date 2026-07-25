package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rollingthunder/pkg/application"
	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"

	"github.com/google/uuid"
)

const (
	connectionStorageVersion             = 6
	sshPasswordCredentialSuffix          = ":ssh-password"
	sshPassphraseCredentialSuffix        = ":ssh-key-passphrase"
	oracleWalletPasswordCredentialSuffix = ":oracle-wallet-password"
)

// SavedConnection contains non-secret profile metadata. Passwords are stored
// under the profile ID in the operating system credential store.
type SavedConnection struct {
	ID                      string          `json:"id"`
	Config                  database.Config `json:"config"`
	HasPassword             bool            `json:"hasPassword"`
	HasSSHPassword          bool            `json:"hasSshPassword"`
	HasSSHKeyPassphrase     bool            `json:"hasSshKeyPassphrase"`
	HasOracleWalletPassword bool            `json:"hasOracleWalletPassword"`
}

type connectionStorageEnvelope struct {
	Version     int               `json:"version"`
	Connections []SavedConnection `json:"connections"`
}

// ConnectionStorage manages versioned, non-secret saved connection metadata.
type ConnectionStorage struct {
	FilePath string
	initErr  error
}

func NewConnectionStorage() *ConnectionStorage {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return &ConnectionStorage{
			initErr: fmt.Errorf("resolve user configuration directory: %w", err),
		}
	}
	appDir := filepath.Join(configDir, application.SettingsDirectoryName)
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		return &ConnectionStorage{
			FilePath: filepath.Join(appDir, "connections.json"),
			initErr:  fmt.Errorf("create connection settings directory: %w", err),
		}
	}

	return &ConnectionStorage{
		FilePath: filepath.Join(appDir, "connections.json"),
	}
}

func (cs *ConnectionStorage) Load() ([]SavedConnection, bool, error) {
	if cs.initErr != nil {
		return nil, false, cs.initErr
	}
	data, err := os.ReadFile(cs.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []SavedConnection{}, false, nil
		}
		return nil, false, fmt.Errorf("read saved connections: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return []SavedConnection{}, false, nil
	}

	// Version 1 stored a raw JSON array. It may contain plaintext passwords;
	// Service.loadSavedConnections migrates those secrets before rewriting.
	var legacy []SavedConnection
	if err := json.Unmarshal(data, &legacy); err == nil {
		return legacy, true, nil
	}

	var envelope connectionStorageEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, false, fmt.Errorf("decode saved connections: %w", err)
	}
	if envelope.Version <= 0 || envelope.Version > connectionStorageVersion {
		return nil, false, fmt.Errorf(
			"unsupported saved connection version %d",
			envelope.Version,
		)
	}
	if envelope.Connections == nil {
		envelope.Connections = []SavedConnection{}
	}
	if err := os.Chmod(cs.FilePath, 0o600); err != nil {
		return nil, false, fmt.Errorf("secure saved connections: %w", err)
	}
	if err := os.Chmod(filepath.Dir(cs.FilePath), 0o700); err != nil {
		return nil, false, fmt.Errorf("secure connection settings directory: %w", err)
	}
	return envelope.Connections, envelope.Version < connectionStorageVersion, nil
}

func scrubSavedConnection(connection SavedConnection) SavedConnection {
	connection.Config = database.NormalizeConfigMetadata(connection.Config)
	connection.Config.Password = ""
	connection.Config.SSHPassword = ""
	connection.Config.SSHKeyPassphrase = ""
	connection.Config.OracleWalletPassword = ""
	return connection
}

func sshPasswordCredentialID(profileID string) string {
	return profileID + sshPasswordCredentialSuffix
}

func sshPassphraseCredentialID(profileID string) string {
	return profileID + sshPassphraseCredentialSuffix
}

func oracleWalletPasswordCredentialID(profileID string) string {
	return profileID + oracleWalletPasswordCredentialSuffix
}

func (s *Service) migratePlaintextSecret(
	value string,
	credentialID string,
) (bool, error) {
	if value == "" {
		return false, nil
	}
	if err := s.credentialStore.Set(credentialID, value); err != nil {
		return false, err
	}
	return true, nil
}

// Save writes an atomic 0600 versioned envelope. Even callers that
// accidentally pass a password cannot persist it in plaintext.
func (cs *ConnectionStorage) Save(connections []SavedConnection) error {
	if cs.initErr != nil {
		return cs.initErr
	}
	clean := make([]SavedConnection, len(connections))
	for index, connection := range connections {
		clean[index] = scrubSavedConnection(connection)
	}
	data, err := json.MarshalIndent(connectionStorageEnvelope{
		Version:     connectionStorageVersion,
		Connections: clean,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode saved connections: %w", err)
	}
	directory := filepath.Dir(cs.FilePath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create connection settings directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("secure connection settings directory: %w", err)
	}
	temp, err := os.CreateTemp(directory, ".connections-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary connection settings: %w", err)
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(0o600); err != nil {
		return fmt.Errorf("secure temporary connection settings: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("write temporary connection settings: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync temporary connection settings: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary connection settings: %w", err)
	}
	if err := os.Rename(tempPath, cs.FilePath); err != nil {
		return fmt.Errorf("replace saved connections: %w", err)
	}
	if err := os.Chmod(cs.FilePath, 0o600); err != nil {
		return fmt.Errorf("secure saved connections: %w", err)
	}
	return nil
}

func (s *Service) loadSavedConnections() ([]SavedConnection, error) {
	connections, legacyFormat, err := s.connectionStorage.Load()
	if err != nil {
		return nil, err
	}
	needsRewrite := legacyFormat
	for index := range connections {
		connection := &connections[index]
		normalizedConfig := database.NormalizeConfigMetadata(connection.Config)
		if !database.ConfigMetadataEqual(connection.Config, normalizedConfig) {
			connection.Config = normalizedConfig
			needsRewrite = true
		}
		passwordMigrated, err := s.migratePlaintextSecret(
			connection.Config.Password,
			connection.ID,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"migrate password for %q to the operating system credential store: %w",
				connection.Config.Name,
				err,
			)
		}
		sshPasswordMigrated, err := s.migratePlaintextSecret(
			connection.Config.SSHPassword,
			sshPasswordCredentialID(connection.ID),
		)
		if err != nil {
			return nil, fmt.Errorf(
				"migrate SSH password for %q to the operating system credential store: %w",
				connection.Config.Name,
				err,
			)
		}
		passphraseMigrated, err := s.migratePlaintextSecret(
			connection.Config.SSHKeyPassphrase,
			sshPassphraseCredentialID(connection.ID),
		)
		if err != nil {
			return nil, fmt.Errorf(
				"migrate SSH key passphrase for %q to the operating system credential store: %w",
				connection.Config.Name,
				err,
			)
		}
		walletPasswordMigrated, err := s.migratePlaintextSecret(
			connection.Config.OracleWalletPassword,
			oracleWalletPasswordCredentialID(connection.ID),
		)
		if err != nil {
			return nil, fmt.Errorf(
				"migrate Oracle Wallet password for %q to the operating system credential store: %w",
				connection.Config.Name,
				err,
			)
		}
		if passwordMigrated {
			connection.HasPassword = true
		}
		if sshPasswordMigrated {
			connection.HasSSHPassword = true
		}
		if passphraseMigrated {
			connection.HasSSHKeyPassphrase = true
		}
		if walletPasswordMigrated {
			connection.HasOracleWalletPassword = true
		}
		if passwordMigrated ||
			sshPasswordMigrated ||
			passphraseMigrated ||
			walletPasswordMigrated {
			*connection = scrubSavedConnection(*connection)
			needsRewrite = true
		}
	}
	if needsRewrite {
		if err := s.connectionStorage.Save(connections); err != nil {
			return nil, fmt.Errorf(
				"remove migrated plaintext passwords from profile storage: %w",
				err,
			)
		}
	}
	for index := range connections {
		connections[index] = scrubSavedConnection(connections[index])
	}
	return connections, nil
}

func connectionStorageError[T any](summary string, err error) response.BaseResponse[T] {
	return serviceErrorWithCode[T](
		500,
		errorCodeDatabaseOperationFailed,
		summary,
		err.Error(),
		"Check access to the operating system credential store and the Rolling Thunder settings directory.",
	)
}

type credentialState struct {
	id     string
	value  string
	exists bool
}

func loadCredentialState(
	store CredentialStore,
	id string,
	expected bool,
) (credentialState, error) {
	state := credentialState{id: id, exists: expected}
	if !expected {
		return state, nil
	}
	value, err := store.Get(id)
	if err != nil {
		return credentialState{}, err
	}
	state.value = value
	return state, nil
}

func writeCredentialState(store CredentialStore, state credentialState) error {
	if state.exists {
		return store.Set(state.id, state.value)
	}
	err := store.Delete(state.id)
	if errors.Is(err, ErrCredentialNotFound) {
		return nil
	}
	return err
}

func restoreCredentialStates(
	store CredentialStore,
	states []credentialState,
) {
	for _, state := range states {
		_ = writeCredentialState(store, state)
	}
}

func applyCredentialStates(
	store CredentialStore,
	previous []credentialState,
	desired []credentialState,
) error {
	for index, state := range desired {
		if index < len(previous) &&
			previous[index].id == state.id &&
			previous[index].exists == state.exists &&
			(!state.exists || previous[index].value == state.value) {
			continue
		}
		if err := writeCredentialState(store, state); err != nil {
			restoreCredentialStates(store, previous)
			return err
		}
	}
	return nil
}

func profileCredentialStates(
	store CredentialStore,
	connection SavedConnection,
) ([]credentialState, error) {
	specifications := []struct {
		id       string
		expected bool
	}{
		{connection.ID, connection.HasPassword},
		{sshPasswordCredentialID(connection.ID), connection.HasSSHPassword},
		{
			sshPassphraseCredentialID(connection.ID),
			connection.HasSSHKeyPassphrase,
		},
		{
			oracleWalletPasswordCredentialID(connection.ID),
			connection.HasOracleWalletPassword,
		},
	}
	states := make([]credentialState, 0, len(specifications))
	for _, specification := range specifications {
		state, err := loadCredentialState(
			store,
			specification.id,
			specification.expected,
		)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}

func (s *Service) GetSavedConnections() response.BaseResponse[[]SavedConnection] {
	connections, err := s.loadSavedConnections()
	if err != nil {
		return connectionStorageError[[]SavedConnection](
			"Could not load saved connections",
			err,
		)
	}
	return response.BaseResponse[[]SavedConnection]{Data: connections}
}

func (s *Service) SaveConnection(
	config database.Config,
) response.BaseResponse[SavedConnection] {
	connections, err := s.loadSavedConnections()
	if err != nil {
		return connectionStorageError[SavedConnection](
			"Could not load saved connections",
			err,
		)
	}
	if config.Driver == "" {
		config.Driver = database.DriverPostgres
	}
	config = database.NormalizeConfigMetadata(config)
	if err := config.ValidateSafety(); err != nil {
		return serviceErrorWithCode[SavedConnection](
			400,
			errorCodeInvalidRequest,
			"Invalid connection profile",
			err.Error(),
			"Review the connection settings and try again.",
		)
	}
	password := config.Password
	sshPassword := config.SSHPassword
	sshKeyPassphrase := config.SSHKeyPassphrase
	oracleWalletPassword := config.OracleWalletPassword
	sshAuthMode := resolvedSSHAuthMode(config)
	config.Password = ""
	config.SSHPassword = ""
	config.SSHKeyPassphrase = ""
	config.OracleWalletPassword = ""
	connection := SavedConnection{
		ID:          uuid.NewString(),
		Config:      config,
		HasPassword: password != "",
		HasSSHPassword: config.SSHEnabled &&
			sshAuthMode == "password" &&
			sshPassword != "",
		HasSSHKeyPassphrase: config.SSHEnabled &&
			(sshAuthMode == "private-key" || sshAuthMode == "key") &&
			sshKeyPassphrase != "",
		HasOracleWalletPassword: strings.EqualFold(
			config.Driver,
			database.DriverOracle,
		) &&
			strings.TrimSpace(config.OracleWalletPath) != "" &&
			oracleWalletPassword != "",
	}
	previous := []credentialState{
		{id: connection.ID},
		{id: sshPasswordCredentialID(connection.ID)},
		{id: sshPassphraseCredentialID(connection.ID)},
		{id: oracleWalletPasswordCredentialID(connection.ID)},
	}
	desired := []credentialState{
		{id: connection.ID, value: password, exists: connection.HasPassword},
		{
			id:     sshPasswordCredentialID(connection.ID),
			value:  sshPassword,
			exists: connection.HasSSHPassword,
		},
		{
			id:     sshPassphraseCredentialID(connection.ID),
			value:  sshKeyPassphrase,
			exists: connection.HasSSHKeyPassphrase,
		},
		{
			id:     oracleWalletPasswordCredentialID(connection.ID),
			value:  oracleWalletPassword,
			exists: connection.HasOracleWalletPassword,
		},
	}
	if err := applyCredentialStates(s.credentialStore, previous, desired); err != nil {
		return connectionStorageError[SavedConnection](
			"Could not secure connection credentials",
			err,
		)
	}
	connections = append(connections, connection)
	if err := s.connectionStorage.Save(connections); err != nil {
		restoreCredentialStates(s.credentialStore, previous)
		return connectionStorageError[SavedConnection](
			"Could not save connection profile",
			err,
		)
	}
	return response.BaseResponse[SavedConnection]{Data: connection}
}

func (s *Service) UpdateConnection(
	id string,
	config database.Config,
) response.BaseResponse[SavedConnection] {
	connections, err := s.loadSavedConnections()
	if err != nil {
		return connectionStorageError[SavedConnection](
			"Could not load saved connections",
			err,
		)
	}
	if config.Driver == "" {
		config.Driver = database.DriverPostgres
	}
	config = database.NormalizeConfigMetadata(config)
	if err := config.ValidateSafety(); err != nil {
		return serviceErrorWithCode[SavedConnection](
			400,
			errorCodeInvalidRequest,
			"Invalid connection profile",
			err.Error(),
			"Review the connection settings and try again.",
		)
	}
	index := -1
	for candidateIndex := range connections {
		if connections[candidateIndex].ID == id {
			index = candidateIndex
			break
		}
	}
	if index < 0 {
		return serviceErrorWithCode[SavedConnection](
			404,
			errorCodeInvalidRequest,
			"Connection profile not found",
			"The saved profile no longer exists.",
			"Refresh saved connections and choose another profile.",
		)
	}

	previous := connections[index]
	previousCredentials, err := profileCredentialStates(
		s.credentialStore,
		previous,
	)
	if err != nil {
		return connectionStorageError[SavedConnection](
			"Could not unlock the existing connection credentials",
			err,
		)
	}
	newPassword := config.Password
	newSSHPassword := config.SSHPassword
	newSSHKeyPassphrase := config.SSHKeyPassphrase
	newOracleWalletPassword := config.OracleWalletPassword
	sshAuthMode := resolvedSSHAuthMode(config)
	config.Password = ""
	config.SSHPassword = ""
	config.SSHKeyPassphrase = ""
	config.OracleWalletPassword = ""
	passwordState := previousCredentials[0]
	if newPassword != "" {
		passwordState.value = newPassword
		passwordState.exists = true
	}
	sshPasswordState := previousCredentials[1]
	if !config.SSHEnabled || sshAuthMode != "password" {
		sshPasswordState.value = ""
		sshPasswordState.exists = false
	} else if newSSHPassword != "" {
		sshPasswordState.value = newSSHPassword
		sshPasswordState.exists = true
	}
	sshPassphraseState := previousCredentials[2]
	if !config.SSHEnabled ||
		(sshAuthMode != "private-key" && sshAuthMode != "key") {
		sshPassphraseState.value = ""
		sshPassphraseState.exists = false
	} else if newSSHKeyPassphrase != "" {
		sshPassphraseState.value = newSSHKeyPassphrase
		sshPassphraseState.exists = true
	}
	oracleWalletPasswordState := previousCredentials[3]
	walletConfigured := strings.EqualFold(
		config.Driver,
		database.DriverOracle,
	) && strings.TrimSpace(config.OracleWalletPath) != ""
	walletPathChanged := filepath.Clean(previous.Config.OracleWalletPath) !=
		filepath.Clean(config.OracleWalletPath)
	if !walletConfigured {
		oracleWalletPasswordState.value = ""
		oracleWalletPasswordState.exists = false
	} else if newOracleWalletPassword != "" {
		oracleWalletPasswordState.value = newOracleWalletPassword
		oracleWalletPasswordState.exists = true
	} else if walletPathChanged {
		oracleWalletPasswordState.value = ""
		oracleWalletPasswordState.exists = false
	}
	updated := SavedConnection{
		ID:                      previous.ID,
		Config:                  config,
		HasPassword:             passwordState.exists,
		HasSSHPassword:          sshPasswordState.exists,
		HasSSHKeyPassphrase:     sshPassphraseState.exists,
		HasOracleWalletPassword: oracleWalletPasswordState.exists,
	}
	desiredCredentials := []credentialState{
		passwordState,
		sshPasswordState,
		sshPassphraseState,
		oracleWalletPasswordState,
	}
	if err := applyCredentialStates(
		s.credentialStore,
		previousCredentials,
		desiredCredentials,
	); err != nil {
		return connectionStorageError[SavedConnection](
			"Could not secure connection credentials",
			err,
		)
	}
	connections[index] = updated
	if err := s.connectionStorage.Save(connections); err != nil {
		restoreCredentialStates(s.credentialStore, previousCredentials)
		return connectionStorageError[SavedConnection](
			"Could not update connection profile",
			err,
		)
	}
	return response.BaseResponse[SavedConnection]{Data: updated}
}

func (s *Service) ClearConnectionPassword(
	id string,
) response.BaseResponse[bool] {
	connections, err := s.loadSavedConnections()
	if err != nil {
		return connectionStorageError[bool](
			"Could not load saved connections",
			err,
		)
	}
	index := -1
	for candidateIndex := range connections {
		if connections[candidateIndex].ID == id {
			index = candidateIndex
			break
		}
	}
	if index < 0 {
		return serviceErrorWithCode[bool](
			404,
			errorCodeInvalidRequest,
			"Connection profile not found",
			"The saved profile no longer exists.",
			"Refresh saved connections and choose another profile.",
		)
	}
	previousPassword, credentialErr := s.credentialStore.Get(id)
	if credentialErr != nil && !errors.Is(credentialErr, ErrCredentialNotFound) {
		return connectionStorageError[bool](
			"Could not unlock connection password before removing it",
			credentialErr,
		)
	}
	if credentialErr == nil {
		if err := s.credentialStore.Delete(id); err != nil &&
			!errors.Is(err, ErrCredentialNotFound) {
			return connectionStorageError[bool](
				"Could not remove connection password",
				err,
			)
		}
	}
	connections[index].HasPassword = false
	if err := s.connectionStorage.Save(connections); err != nil {
		// Best-effort rollback keeps the old profile usable when its metadata
		// cannot be committed after the credential was removed.
		if credentialErr == nil {
			_ = s.credentialStore.Set(id, previousPassword)
		}
		return connectionStorageError[bool](
			"Could not update connection profile",
			err,
		)
	}
	return response.BaseResponse[bool]{Data: true}
}

func (s *Service) ClearConnectionSSHCredentials(
	id string,
) response.BaseResponse[bool] {
	connections, err := s.loadSavedConnections()
	if err != nil {
		return connectionStorageError[bool](
			"Could not load saved connections",
			err,
		)
	}
	index := -1
	for candidateIndex := range connections {
		if connections[candidateIndex].ID == id {
			index = candidateIndex
			break
		}
	}
	if index < 0 {
		return serviceErrorWithCode[bool](
			404,
			errorCodeInvalidRequest,
			"Connection profile not found",
			"The saved profile no longer exists.",
			"Refresh saved connections and choose another profile.",
		)
	}
	previous, err := profileCredentialStates(
		s.credentialStore,
		connections[index],
	)
	if err != nil {
		return connectionStorageError[bool](
			"Could not unlock SSH credentials before removing them",
			err,
		)
	}
	desired := append([]credentialState(nil), previous...)
	desired[1].value = ""
	desired[1].exists = false
	desired[2].value = ""
	desired[2].exists = false
	if err := applyCredentialStates(s.credentialStore, previous, desired); err != nil {
		return connectionStorageError[bool](
			"Could not remove SSH credentials",
			err,
		)
	}
	connections[index].HasSSHPassword = false
	connections[index].HasSSHKeyPassphrase = false
	if err := s.connectionStorage.Save(connections); err != nil {
		restoreCredentialStates(s.credentialStore, previous)
		return connectionStorageError[bool](
			"Could not update connection profile",
			err,
		)
	}
	return response.BaseResponse[bool]{Data: true}
}

func (s *Service) ClearConnectionOracleWalletPassword(
	id string,
) response.BaseResponse[bool] {
	connections, err := s.loadSavedConnections()
	if err != nil {
		return connectionStorageError[bool](
			"Could not load saved connections",
			err,
		)
	}
	index := -1
	for candidateIndex := range connections {
		if connections[candidateIndex].ID == id {
			index = candidateIndex
			break
		}
	}
	if index < 0 {
		return serviceErrorWithCode[bool](
			404,
			errorCodeInvalidRequest,
			"Connection profile not found",
			"The saved profile no longer exists.",
			"Refresh saved connections and choose another profile.",
		)
	}
	previous, err := profileCredentialStates(
		s.credentialStore,
		connections[index],
	)
	if err != nil {
		return connectionStorageError[bool](
			"Could not unlock the Oracle Wallet password before removing it",
			err,
		)
	}
	desired := append([]credentialState(nil), previous...)
	desired[3].value = ""
	desired[3].exists = false
	if err := applyCredentialStates(s.credentialStore, previous, desired); err != nil {
		return connectionStorageError[bool](
			"Could not remove the Oracle Wallet password",
			err,
		)
	}
	connections[index].HasOracleWalletPassword = false
	if err := s.connectionStorage.Save(connections); err != nil {
		restoreCredentialStates(s.credentialStore, previous)
		return connectionStorageError[bool](
			"Could not update connection profile",
			err,
		)
	}
	return response.BaseResponse[bool]{Data: true}
}

func (s *Service) DeleteConnection(id string) response.BaseResponse[bool] {
	connections, err := s.loadSavedConnections()
	if err != nil {
		return connectionStorageError[bool](
			"Could not load saved connections",
			err,
		)
	}
	filtered := make([]SavedConnection, 0, len(connections))
	var deleted *SavedConnection
	for _, connection := range connections {
		if connection.ID == id {
			copy := connection
			deleted = &copy
			continue
		}
		filtered = append(filtered, connection)
	}
	if deleted == nil {
		return serviceErrorWithCode[bool](
			404,
			errorCodeInvalidRequest,
			"Connection profile not found",
			"The saved profile no longer exists.",
			"Refresh saved connections and choose another profile.",
		)
	}
	previousCredentials, err := profileCredentialStates(
		s.credentialStore,
		*deleted,
	)
	if err != nil {
		return connectionStorageError[bool](
			"Could not unlock connection credentials before deleting the profile",
			err,
		)
	}
	removedCredentials := make([]credentialState, len(previousCredentials))
	for index, credential := range previousCredentials {
		removedCredentials[index] = credentialState{id: credential.id}
	}
	if err := applyCredentialStates(
		s.credentialStore,
		previousCredentials,
		removedCredentials,
	); err != nil {
		return connectionStorageError[bool](
			"Could not remove connection credentials",
			err,
		)
	}
	if err := s.connectionStorage.Save(filtered); err != nil {
		restoreCredentialStates(s.credentialStore, previousCredentials)
		return connectionStorageError[bool](
			"Could not delete connection profile",
			err,
		)
	}
	return response.BaseResponse[bool]{Data: true}
}

func (s *Service) hydrateProfileCredentials(
	profile SavedConnection,
	config database.Config,
) (database.Config, error) {
	if config.Password == "" && profile.HasPassword {
		password, err := s.credentialStore.Get(profile.ID)
		if err != nil {
			return database.Config{}, fmt.Errorf(
				"unlock database password: %w",
				err,
			)
		}
		config.Password = password
	}
	if config.OracleWalletPassword == "" &&
		profile.HasOracleWalletPassword {
		password, err := s.credentialStore.Get(
			oracleWalletPasswordCredentialID(profile.ID),
		)
		if err != nil {
			return database.Config{}, fmt.Errorf(
				"unlock Oracle Wallet password: %w",
				err,
			)
		}
		config.OracleWalletPassword = password
	}
	if !config.SSHEnabled {
		return config, nil
	}
	switch resolvedSSHAuthMode(config) {
	case "password":
		if config.SSHPassword == "" && profile.HasSSHPassword {
			password, err := s.credentialStore.Get(
				sshPasswordCredentialID(profile.ID),
			)
			if err != nil {
				return database.Config{}, fmt.Errorf(
					"unlock SSH password: %w",
					err,
				)
			}
			config.SSHPassword = password
		}
	case "private-key", "key":
		if config.SSHKeyPassphrase == "" && profile.HasSSHKeyPassphrase {
			passphrase, err := s.credentialStore.Get(
				sshPassphraseCredentialID(profile.ID),
			)
			if err != nil {
				return database.Config{}, fmt.Errorf(
					"unlock SSH key passphrase: %w",
					err,
				)
			}
			config.SSHKeyPassphrase = passphrase
		}
	}
	return config, nil
}

func (s *Service) ConnectSavedConnection(
	profileID string,
	attemptID string,
) response.BaseResponse[ConnectResponse] {
	connections, err := s.loadSavedConnections()
	if err != nil {
		return connectionStorageError[ConnectResponse](
			"Could not load saved connection",
			err,
		)
	}
	var profile *SavedConnection
	for index := range connections {
		if connections[index].ID == profileID {
			profile = &connections[index]
			break
		}
	}
	if profile == nil {
		return serviceErrorWithCode[ConnectResponse](
			404,
			errorCodeInvalidRequest,
			"Connection profile not found",
			"The saved profile no longer exists.",
			"Refresh saved connections and choose another profile.",
		)
	}
	config, err := s.hydrateProfileCredentials(*profile, profile.Config)
	if err != nil {
		return connectionStorageError[ConnectResponse](
			"Could not unlock connection credentials",
			err,
		)
	}
	return s.Connect(ConnectRequest{
		Driver:    config.Driver,
		Config:    config,
		AttemptID: attemptID,
		ProfileID: profile.ID,
	})
}

// ConnectWithProfile connects the current unsaved form while resolving an
// unchanged blank password from the saved profile. This keeps secrets
// backend-side without forcing users to save every endpoint edit first.
func (s *Service) ConnectWithProfile(
	profileID string,
	config database.Config,
	attemptID string,
) response.BaseResponse[ConnectResponse] {
	connections, err := s.loadSavedConnections()
	if err != nil {
		return connectionStorageError[ConnectResponse](
			"Could not load saved connection",
			err,
		)
	}
	var profile *SavedConnection
	for index := range connections {
		if connections[index].ID == profileID {
			profile = &connections[index]
			break
		}
	}
	if profile == nil {
		return serviceErrorWithCode[ConnectResponse](
			404,
			errorCodeInvalidRequest,
			"Connection profile not found",
			"The saved profile no longer exists.",
			"Refresh saved connections and choose another profile.",
		)
	}
	if config.Driver == "" {
		config.Driver = profile.Config.Driver
	}
	config, err = s.hydrateProfileCredentials(*profile, config)
	if err != nil {
		return connectionStorageError[ConnectResponse](
			"Could not unlock connection credentials",
			err,
		)
	}
	return s.Connect(ConnectRequest{
		Driver:    config.Driver,
		Config:    config,
		AttemptID: attemptID,
		ProfileID: profile.ID,
	})
}
