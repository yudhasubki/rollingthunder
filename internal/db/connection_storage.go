package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"

	"github.com/google/uuid"
)

const connectionStorageVersion = 2

// SavedConnection contains non-secret profile metadata. Passwords are stored
// under the profile ID in the operating system credential store.
type SavedConnection struct {
	ID          string          `json:"id"`
	Config      database.Config `json:"config"`
	HasPassword bool            `json:"hasPassword"`
}

type connectionStorageEnvelope struct {
	Version     int               `json:"version"`
	Connections []SavedConnection `json:"connections"`
}

// ConnectionStorage manages versioned, non-secret saved connection metadata.
type ConnectionStorage struct {
	FilePath string
}

func NewConnectionStorage() *ConnectionStorage {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "."
	}
	appDir := filepath.Join(configDir, "RollingThunder")
	_ = os.MkdirAll(appDir, 0o700)

	return &ConnectionStorage{
		FilePath: filepath.Join(appDir, "connections.json"),
	}
}

func (cs *ConnectionStorage) Load() ([]SavedConnection, bool, error) {
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
	return envelope.Connections, false, nil
}

func scrubSavedConnection(connection SavedConnection) SavedConnection {
	connection.Config.Password = ""
	return connection
}

// Save writes an atomic 0600 versioned envelope. Even callers that
// accidentally pass a password cannot persist it in plaintext.
func (cs *ConnectionStorage) Save(connections []SavedConnection) error {
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
		password := connections[index].Config.Password
		if password == "" {
			connections[index].Config.Password = ""
			continue
		}
		if err := s.credentialStore.Set(connections[index].ID, password); err != nil {
			return nil, fmt.Errorf(
				"migrate password for %q to the operating system credential store: %w",
				connections[index].Config.Name,
				err,
			)
		}
		connections[index].Config.Password = ""
		connections[index].HasPassword = true
		needsRewrite = true
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
		config.Driver = "postgres"
	}
	password := config.Password
	config.Password = ""
	connection := SavedConnection{
		ID:          uuid.NewString(),
		Config:      config,
		HasPassword: password != "",
	}
	if password != "" {
		if err := s.credentialStore.Set(connection.ID, password); err != nil {
			return connectionStorageError[SavedConnection](
				"Could not secure connection password",
				err,
			)
		}
	}
	connections = append(connections, connection)
	if err := s.connectionStorage.Save(connections); err != nil {
		if password != "" {
			_ = s.credentialStore.Delete(connection.ID)
		}
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
		config.Driver = "postgres"
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
	newPassword := config.Password
	config.Password = ""
	updated := SavedConnection{
		ID:          previous.ID,
		Config:      config,
		HasPassword: previous.HasPassword || newPassword != "",
	}
	var previousPassword string
	if newPassword != "" {
		if previous.HasPassword {
			previousPassword, err = s.credentialStore.Get(id)
			if err != nil {
				return connectionStorageError[SavedConnection](
					"Could not unlock the existing connection password",
					err,
				)
			}
		}
		if err := s.credentialStore.Set(id, newPassword); err != nil {
			return connectionStorageError[SavedConnection](
				"Could not secure connection password",
				err,
			)
		}
	}
	connections[index] = updated
	if err := s.connectionStorage.Save(connections); err != nil {
		if newPassword != "" {
			if previousPassword != "" {
				_ = s.credentialStore.Set(id, previousPassword)
			} else {
				_ = s.credentialStore.Delete(id)
			}
		}
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

func (s *Service) DeleteConnection(id string) response.BaseResponse[bool] {
	connections, err := s.loadSavedConnections()
	if err != nil {
		return connectionStorageError[bool](
			"Could not load saved connections",
			err,
		)
	}
	filtered := make([]SavedConnection, 0, len(connections))
	found := false
	hadPassword := false
	for _, connection := range connections {
		if connection.ID == id {
			found = true
			hadPassword = connection.HasPassword
			continue
		}
		filtered = append(filtered, connection)
	}
	if !found {
		return serviceErrorWithCode[bool](
			404,
			errorCodeInvalidRequest,
			"Connection profile not found",
			"The saved profile no longer exists.",
			"Refresh saved connections and choose another profile.",
		)
	}
	var previousPassword string
	credentialFound := false
	if hadPassword {
		previousPassword, err = s.credentialStore.Get(id)
		if err != nil && !errors.Is(err, ErrCredentialNotFound) {
			return connectionStorageError[bool](
				"Could not unlock the connection password before deleting the profile",
				err,
			)
		}
		if err == nil {
			credentialFound = true
			if err := s.credentialStore.Delete(id); err != nil &&
				!errors.Is(err, ErrCredentialNotFound) {
				return connectionStorageError[bool](
					"Could not remove the connection password",
					err,
				)
			}
		}
	}
	if err := s.connectionStorage.Save(filtered); err != nil {
		if credentialFound {
			_ = s.credentialStore.Set(id, previousPassword)
		}
		return connectionStorageError[bool](
			"Could not delete connection profile",
			err,
		)
	}
	return response.BaseResponse[bool]{Data: true}
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
	config := profile.Config
	if profile.HasPassword {
		password, credentialErr := s.credentialStore.Get(profile.ID)
		if credentialErr != nil {
			return connectionStorageError[ConnectResponse](
				"Could not unlock connection password",
				credentialErr,
			)
		}
		config.Password = password
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
	if config.Password == "" && profile.HasPassword {
		password, credentialErr := s.credentialStore.Get(profile.ID)
		if credentialErr != nil {
			return connectionStorageError[ConnectResponse](
				"Could not unlock connection password",
				credentialErr,
			)
		}
		config.Password = password
	}
	return s.Connect(ConnectRequest{
		Driver:    config.Driver,
		Config:    config,
		AttemptID: attemptID,
		ProfileID: profile.ID,
	})
}
