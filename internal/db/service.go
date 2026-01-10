package db

import (
	"context"
	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"

	"github.com/google/uuid"
)

// Connection represents an active database connection
type Connection struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Driver database.Driver `json:"-"`
	Config database.Config `json:"config"`
	Color  string          `json:"color"`
}

// ConnectionInfo is the public info about a connection (without driver)
type ConnectionInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Database string `json:"database"`
	Host     string `json:"host"`
	Color    string `json:"color"`
	IsActive bool   `json:"isActive"`
}

type Service struct {
	ctx         context.Context
	connections map[string]*Connection
	activeID    string
	driver      database.Driver // backward compat - points to active connection's driver
}

func NewService() *Service {
	return &Service{
		connections: make(map[string]*Connection),
	}
}

func (s *Service) Start(ctx context.Context) {
	s.ctx = ctx
}

func (s *Service) Connect(req ConnectRequest) response.BaseResponse[ConnectResponse] {
	driver, err := NewDriver(s.ctx, req.Driver, req.Config)
	if err != nil {
		return response.BaseResponse[ConnectResponse]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
			Data: ConnectResponse{
				Connected: false,
			},
		}
	}

	err = driver.Connect()
	if err != nil {
		return response.BaseResponse[ConnectResponse]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
			Data: ConnectResponse{
				Connected: false,
			},
		}
	}

	// Generate connection ID and store in registry
	connID := uuid.New().String()
	conn := &Connection{
		ID:     connID,
		Name:   req.Config.Name,
		Driver: driver,
		Config: req.Config,
		Color:  req.Config.Color,
	}
	s.connections[connID] = conn
	s.activeID = connID
	s.driver = driver

	return response.BaseResponse[ConnectResponse]{
		Data: ConnectResponse{
			Connected:    true,
			ConnectionID: connID,
		},
	}
}

func (s *Service) GetCollections(schema []string) response.BaseResponse[[]string] {
	collections, err := s.driver.GetCollections(schema...)
	if err != nil {
		return response.BaseResponse[[]string]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
		}
	}

	return response.BaseResponse[[]string]{
		Data: collections,
	}
}

func (s *Service) GetCollectionStructures(table database.Table) response.BaseResponse[database.Structures] {
	structures, err := s.driver.GetCollectionStructures(table)
	if err != nil {
		return response.BaseResponse[database.Structures]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
		}
	}

	return response.BaseResponse[database.Structures]{
		Data: structures,
	}
}

func (s *Service) GetIndices(table database.Table) response.BaseResponse[database.Indices] {
	indices, err := s.driver.GetIndices(table)
	if err != nil {
		return response.BaseResponse[database.Indices]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
		}
	}

	return response.BaseResponse[database.Indices]{
		Data: indices,
	}
}

func (s *Service) GetSchemas() response.BaseResponse[[]string] {
	if d, ok := s.driver.(database.DriverWithSchema); ok {
		schemas, err := d.GetSchemas()
		if err != nil {
			return response.BaseResponse[[]string]{
				Errors: []response.BaseErrorResponse{
					{
						Detail: err.Error(),
					},
				},
			}
		}

		return response.BaseResponse[[]string]{
			Data: schemas,
		}
	}

	return response.BaseResponse[[]string]{
		Data: []string{},
	}
}

func (s *Service) GetDatabaseInfo() response.BaseResponse[database.Info] {
	info, err := s.driver.GetDatabaseInfo()
	if err != nil {
		return response.BaseResponse[database.Info]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
		}
	}

	return response.BaseResponse[database.Info]{
		Data: info,
	}
}

func (s *Service) CountCollectionData(table database.Table) response.BaseResponse[int] {
	count, err := s.driver.CountCollectionData(table)
	if err != nil {
		return response.BaseResponse[int]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
		}
	}

	return response.BaseResponse[int]{
		Data: count,
	}
}

func (s *Service) GetCollectionData(table database.Table) response.BaseResponse[database.TableData] {
	structures, results, err := s.driver.GetCollectionData(table)
	if err != nil {
		return response.BaseResponse[database.TableData]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
		}
	}

	resp := response.BaseResponse[database.TableData]{
		Data: database.TableData{
			Structures: make(database.Structures, 0),
			Data:       make([]map[string]interface{}, 0),
		},
	}
	if len(structures) > 0 {
		resp.Data.Structures = structures
	}

	if len(results) > 0 {
		resp.Data.Data = results
	}

	return resp
}

// InsertRow inserts a new row into the table
func (s *Service) InsertRow(table database.Table, data map[string]interface{}) response.BaseResponse[bool] {
	err := s.driver.InsertRow(table, data)
	if err != nil {
		return response.BaseResponse[bool]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
			Data: false,
		}
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// UpdateRow updates an existing row in the table
func (s *Service) UpdateRow(table database.Table, data map[string]interface{}, primaryKey string) response.BaseResponse[bool] {
	err := s.driver.UpdateRow(table, data, primaryKey)
	if err != nil {
		return response.BaseResponse[bool]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
			Data: false,
		}
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// DeleteRow deletes a row from the table
func (s *Service) DeleteRow(table database.Table, primaryKey string, primaryValue interface{}) response.BaseResponse[bool] {
	err := s.driver.DeleteRow(table, primaryKey, primaryValue)
	if err != nil {
		return response.BaseResponse[bool]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
			Data: false,
		}
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// ExecuteQuery executes a raw SQL query
func (s *Service) ExecuteQuery(query string) response.BaseResponse[[]map[string]interface{}] {
	results, err := s.driver.ExecuteQuery(query)
	if err != nil {
		return response.BaseResponse[[]map[string]interface{}]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
		}
	}

	return response.BaseResponse[[]map[string]interface{}]{
		Data: results,
	}
}

// CreateTable creates a new table in the database
func (s *Service) CreateTable(table database.Table, columns []database.ColumnDefinition) response.BaseResponse[bool] {
	err := s.driver.CreateTable(table, columns)
	if err != nil {
		return response.BaseResponse[bool]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
			Data: false,
		}
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// GetDataTypes returns available data types for the current database driver
func (s *Service) GetDataTypes() response.BaseResponse[[]database.DataType] {
	types := s.driver.GetDataTypes()
	return response.BaseResponse[[]database.DataType]{
		Data: types,
	}
}

// DropTable drops a table from the database
func (s *Service) DropTable(table database.Table) response.BaseResponse[bool] {
	err := s.driver.DropTable(table)
	if err != nil {
		return response.BaseResponse[bool]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
			Data: false,
		}
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// TruncateTable removes all rows from a table
func (s *Service) TruncateTable(table database.Table) response.BaseResponse[bool] {
	err := s.driver.TruncateTable(table)
	if err != nil {
		return response.BaseResponse[bool]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
			Data: false,
		}
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// GetTableDDL returns the CREATE TABLE DDL statement for a table
func (s *Service) GetTableDDL(table database.Table) response.BaseResponse[string] {
	ddl, err := s.driver.GetTableDDL(table)
	if err != nil {
		return response.BaseResponse[string]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: err.Error(),
				},
			},
		}
	}

	return response.BaseResponse[string]{
		Data: ddl,
	}
}

// SwitchConnection switches to a different active connection
func (s *Service) SwitchConnection(connectionID string) response.BaseResponse[bool] {
	conn, ok := s.connections[connectionID]
	if !ok {
		return response.BaseResponse[bool]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: "Connection not found",
				},
			},
			Data: false,
		}
	}

	s.activeID = connectionID
	s.driver = conn.Driver

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// GetActiveConnections returns all active connections
func (s *Service) GetActiveConnections() response.BaseResponse[[]ConnectionInfo] {
	var connections []ConnectionInfo
	for _, conn := range s.connections {
		connections = append(connections, ConnectionInfo{
			ID:       conn.ID,
			Name:     conn.Name,
			Database: conn.Config.Db,
			Host:     conn.Config.Host,
			Color:    conn.Color,
			IsActive: conn.ID == s.activeID,
		})
	}

	return response.BaseResponse[[]ConnectionInfo]{
		Data: connections,
	}
}

// DisconnectConnection disconnects and removes a connection from registry
func (s *Service) DisconnectConnection(connectionID string) response.BaseResponse[bool] {
	conn, ok := s.connections[connectionID]
	if !ok {
		return response.BaseResponse[bool]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: "Connection not found",
				},
			},
			Data: false,
		}
	}

	// Close the connection
	conn.Driver.Close()

	// Remove from registry
	delete(s.connections, connectionID)

	// If this was the active connection, switch to another or clear
	if s.activeID == connectionID {
		s.activeID = ""
		s.driver = nil
		// Try to switch to first available connection
		for id, c := range s.connections {
			s.activeID = id
			s.driver = c.Driver
			break
		}
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}
