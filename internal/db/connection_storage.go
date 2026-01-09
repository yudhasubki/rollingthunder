package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"

	"github.com/google/uuid"
)

// SavedConnection represents a saved database connection
type SavedConnection struct {
	ID     string          `json:"id"`
	Config database.Config `json:"config"`
}

// ConnectionStorage manages saved connections
type ConnectionStorage struct {
	FilePath string
}

func NewConnectionStorage() *ConnectionStorage {
	// Get user config directory
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "."
	}
	appDir := filepath.Join(configDir, "RollingThunder")
	os.MkdirAll(appDir, 0755)

	return &ConnectionStorage{
		FilePath: filepath.Join(appDir, "connections.json"),
	}
}

// GetConnections returns all saved connections
func (s *Service) GetSavedConnections() response.BaseResponse[[]SavedConnection] {
	storage := NewConnectionStorage()
	connections, err := storage.Load()
	if err != nil {
		return response.BaseResponse[[]SavedConnection]{
			Errors: []response.BaseErrorResponse{
				{Detail: err.Error()},
			},
		}
	}
	return response.BaseResponse[[]SavedConnection]{
		Data: connections,
	}
}

// SaveConnection saves a new connection
func (s *Service) SaveConnection(config database.Config) response.BaseResponse[SavedConnection] {
	storage := NewConnectionStorage()
	connections, _ := storage.Load()

	// Generate new ID
	conn := SavedConnection{
		ID:     uuid.New().String(),
		Config: config,
	}
	connections = append(connections, conn)

	if err := storage.Save(connections); err != nil {
		return response.BaseResponse[SavedConnection]{
			Errors: []response.BaseErrorResponse{
				{Detail: err.Error()},
			},
		}
	}

	return response.BaseResponse[SavedConnection]{
		Data: conn,
	}
}

// UpdateConnection updates an existing connection
func (s *Service) UpdateConnection(id string, config database.Config) response.BaseResponse[SavedConnection] {
	storage := NewConnectionStorage()
	connections, _ := storage.Load()

	var updated *SavedConnection
	for i, c := range connections {
		if c.ID == id {
			connections[i].Config = config
			updated = &connections[i]
			break
		}
	}

	if updated == nil {
		return response.BaseResponse[SavedConnection]{
			Errors: []response.BaseErrorResponse{
				{Detail: "Connection not found"},
			},
		}
	}

	if err := storage.Save(connections); err != nil {
		return response.BaseResponse[SavedConnection]{
			Errors: []response.BaseErrorResponse{
				{Detail: err.Error()},
			},
		}
	}

	return response.BaseResponse[SavedConnection]{
		Data: *updated,
	}
}

// DeleteConnection removes a saved connection
func (s *Service) DeleteConnection(id string) response.BaseResponse[bool] {
	storage := NewConnectionStorage()
	connections, _ := storage.Load()

	var filtered []SavedConnection
	found := false
	for _, c := range connections {
		if c.ID == id {
			found = true
		} else {
			filtered = append(filtered, c)
		}
	}

	if !found {
		return response.BaseResponse[bool]{
			Errors: []response.BaseErrorResponse{
				{Detail: "Connection not found"},
			},
			Data: false,
		}
	}

	if err := storage.Save(filtered); err != nil {
		return response.BaseResponse[bool]{
			Errors: []response.BaseErrorResponse{
				{Detail: err.Error()},
			},
			Data: false,
		}
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// Load reads connections from file
func (cs *ConnectionStorage) Load() ([]SavedConnection, error) {
	data, err := os.ReadFile(cs.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []SavedConnection{}, nil
		}
		return nil, err
	}

	var connections []SavedConnection
	if err := json.Unmarshal(data, &connections); err != nil {
		return nil, err
	}
	return connections, nil
}

// Save writes connections to file
func (cs *ConnectionStorage) Save(connections []SavedConnection) error {
	data, err := json.MarshalIndent(connections, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cs.FilePath, data, 0644)
}
