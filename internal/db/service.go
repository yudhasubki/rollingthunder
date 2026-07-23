package db

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"

	"github.com/google/uuid"
)

// Connection represents an active database connection
type Connection struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Driver      database.Driver `json:"-"`
	Config      database.Config `json:"config"`
	Color       string          `json:"color"`
	ConnectedAt time.Time       `json:"connectedAt"`
	mu          sync.RWMutex
	closed      bool
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
	mu          sync.RWMutex
}

func NewService() *Service {
	return &Service{
		connections: make(map[string]*Connection),
	}
}

func (s *Service) Start(ctx context.Context) {
	s.ctx = ctx
}

func serviceError[T any](detail string) response.BaseResponse[T] {
	return response.BaseResponse[T]{
		Errors: []response.BaseErrorResponse{
			{Detail: detail},
		},
	}
}

// driverFor pins an active operation to one connection. The release function
// must be called after the driver operation completes. Disconnect waits for
// pinned operations instead of closing a driver while it is still in use.
func (s *Service) driverFor(connectionID string) (database.Driver, func(), error) {
	if connectionID == "" {
		return nil, nil, fmt.Errorf("connection ID is required")
	}

	s.mu.RLock()
	conn, ok := s.connections[connectionID]
	s.mu.RUnlock()
	if !ok {
		return nil, nil, fmt.Errorf("connection not found or disconnected")
	}

	conn.mu.RLock()
	if conn.closed {
		conn.mu.RUnlock()
		return nil, nil, fmt.Errorf("connection not found or disconnected")
	}

	return conn.Driver, conn.mu.RUnlock, nil
}

func (s *Service) Connect(req ConnectRequest) response.BaseResponse[ConnectResponse] {
	driverName := req.Driver
	if driverName == "" {
		driverName = req.Config.Driver
	}
	if driverName == "" {
		driverName = "postgres"
	}
	req.Config.Driver = driverName

	driver, err := NewDriver(s.ctx, driverName, req.Config)
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
		ID:          connID,
		Name:        req.Config.Name,
		Driver:      driver,
		Config:      req.Config,
		Color:       req.Config.Color,
		ConnectedAt: time.Now(),
	}
	s.mu.Lock()
	s.connections[connID] = conn
	s.activeID = connID
	s.mu.Unlock()

	return response.BaseResponse[ConnectResponse]{
		Data: ConnectResponse{
			Connected:    true,
			ConnectionID: connID,
		},
	}
}

func (s *Service) GetCollections(connectionID string, schema []string) response.BaseResponse[[]string] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[[]string](err.Error())
	}
	defer release()

	collections, err := driver.GetCollections(schema...)
	if err != nil {
		return serviceError[[]string](err.Error())
	}

	return response.BaseResponse[[]string]{
		Data: collections,
	}
}

func (s *Service) GetCollectionStructures(connectionID string, table database.Table) response.BaseResponse[database.Structures] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[database.Structures](err.Error())
	}
	defer release()

	structures, err := driver.GetCollectionStructures(table)
	if err != nil {
		return serviceError[database.Structures](err.Error())
	}

	return response.BaseResponse[database.Structures]{
		Data: structures,
	}
}

func (s *Service) GetIndices(connectionID string, table database.Table) response.BaseResponse[database.Indices] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[database.Indices](err.Error())
	}
	defer release()

	indices, err := driver.GetIndices(table)
	if err != nil {
		return serviceError[database.Indices](err.Error())
	}

	return response.BaseResponse[database.Indices]{
		Data: indices,
	}
}

func (s *Service) GetSchemas(connectionID string) response.BaseResponse[[]string] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[[]string](err.Error())
	}
	defer release()

	if d, ok := driver.(database.DriverWithSchema); ok {
		schemas, err := d.GetSchemas()
		if err != nil {
			return serviceError[[]string](err.Error())
		}

		return response.BaseResponse[[]string]{
			Data: schemas,
		}
	}

	return response.BaseResponse[[]string]{
		Data: []string{},
	}
}

func (s *Service) GetDatabaseInfo(connectionID string) response.BaseResponse[database.Info] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[database.Info](err.Error())
	}
	defer release()

	info, err := driver.GetDatabaseInfo()
	if err != nil {
		return serviceError[database.Info](err.Error())
	}

	return response.BaseResponse[database.Info]{
		Data: info,
	}
}

func (s *Service) CountCollectionData(connectionID string, table database.Table) response.BaseResponse[int] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[int](err.Error())
	}
	defer release()

	count, err := driver.CountCollectionData(table)
	if err != nil {
		return serviceError[int](err.Error())
	}

	return response.BaseResponse[int]{
		Data: count,
	}
}

func (s *Service) GetCollectionData(connectionID string, table database.Table) response.BaseResponse[database.TableData] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[database.TableData](err.Error())
	}
	defer release()

	structures, results, err := driver.GetCollectionData(table)
	if err != nil {
		return serviceError[database.TableData](err.Error())
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
func (s *Service) InsertRow(connectionID string, table database.Table, data map[string]interface{}) response.BaseResponse[bool] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[bool](err.Error())
	}
	defer release()

	err = driver.InsertRow(table, data)
	if err != nil {
		return serviceError[bool](err.Error())
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// UpdateRow updates an existing row in the table
func (s *Service) UpdateRow(connectionID string, table database.Table, data map[string]interface{}, primaryKey string) response.BaseResponse[bool] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[bool](err.Error())
	}
	defer release()

	err = driver.UpdateRow(table, data, primaryKey)
	if err != nil {
		return serviceError[bool](err.Error())
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// DeleteRow deletes a row from the table
func (s *Service) DeleteRow(connectionID string, table database.Table, primaryKey string, primaryValue interface{}) response.BaseResponse[bool] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[bool](err.Error())
	}
	defer release()

	err = driver.DeleteRow(table, primaryKey, primaryValue)
	if err != nil {
		return serviceError[bool](err.Error())
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// ExecuteQuery executes a raw SQL query
func (s *Service) ExecuteQuery(connectionID string, query string) response.BaseResponse[[]map[string]interface{}] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[[]map[string]interface{}](err.Error())
	}
	defer release()

	results, err := driver.ExecuteQuery(query)
	if err != nil {
		return serviceError[[]map[string]interface{}](err.Error())
	}

	return response.BaseResponse[[]map[string]interface{}]{
		Data: results,
	}
}

// CreateTable creates a new table in the database
func (s *Service) CreateTable(connectionID string, table database.Table, columns []database.ColumnDefinition) response.BaseResponse[bool] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[bool](err.Error())
	}
	defer release()

	err = driver.CreateTable(table, columns)
	if err != nil {
		return serviceError[bool](err.Error())
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// GetDataTypes returns available data types for the current database driver
func (s *Service) GetDataTypes(connectionID string) response.BaseResponse[[]database.DataType] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[[]database.DataType](err.Error())
	}
	defer release()

	types := driver.GetDataTypes()
	return response.BaseResponse[[]database.DataType]{
		Data: types,
	}
}

// DropTable drops a table from the database
func (s *Service) DropTable(connectionID string, table database.Table) response.BaseResponse[bool] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[bool](err.Error())
	}
	defer release()

	err = driver.DropTable(table)
	if err != nil {
		return serviceError[bool](err.Error())
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// TruncateTable removes all rows from a table
func (s *Service) TruncateTable(connectionID string, table database.Table) response.BaseResponse[bool] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[bool](err.Error())
	}
	defer release()

	err = driver.TruncateTable(table)
	if err != nil {
		return serviceError[bool](err.Error())
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// GetTableDDL returns the CREATE TABLE DDL statement for a table
func (s *Service) GetTableDDL(connectionID string, table database.Table) response.BaseResponse[string] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[string](err.Error())
	}
	defer release()

	ddl, err := driver.GetTableDDL(table)
	if err != nil {
		return serviceError[string](err.Error())
	}

	return response.BaseResponse[string]{
		Data: ddl,
	}
}

// SwitchConnection switches to a different active connection
func (s *Service) SwitchConnection(connectionID string) response.BaseResponse[bool] {
	s.mu.Lock()
	_, ok := s.connections[connectionID]
	if !ok {
		s.mu.Unlock()
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
	s.mu.Unlock()

	return response.BaseResponse[bool]{
		Data: true,
	}
}

// GetActiveConnections returns all active connections
func (s *Service) GetActiveConnections() response.BaseResponse[[]ConnectionInfo] {
	// Collect connections into a slice for sorting
	s.mu.RLock()
	var connList []*Connection
	for _, conn := range s.connections {
		connList = append(connList, conn)
	}
	activeID := s.activeID
	s.mu.RUnlock()

	// Sort by connection time (oldest first)
	sort.Slice(connList, func(i, j int) bool {
		return connList[i].ConnectedAt.Before(connList[j].ConnectedAt)
	})

	// Convert to ConnectionInfo
	var connections []ConnectionInfo
	for _, conn := range connList {
		connections = append(connections, ConnectionInfo{
			ID:       conn.ID,
			Name:     conn.Name,
			Database: conn.Config.Db,
			Host:     conn.Config.Host,
			Color:    conn.Color,
			IsActive: conn.ID == activeID,
		})
	}

	return response.BaseResponse[[]ConnectionInfo]{
		Data: connections,
	}
}

// DisconnectConnection disconnects and removes a connection from registry
func (s *Service) DisconnectConnection(connectionID string) response.BaseResponse[bool] {
	s.mu.Lock()
	conn, ok := s.connections[connectionID]
	if !ok {
		s.mu.Unlock()
		return response.BaseResponse[bool]{
			Errors: []response.BaseErrorResponse{
				{
					Detail: "Connection not found",
				},
			},
			Data: false,
		}
	}

	// Remove from registry
	delete(s.connections, connectionID)

	// If this was the active connection, switch to another or clear
	if s.activeID == connectionID {
		s.activeID = ""
		var next *Connection
		for _, candidate := range s.connections {
			if next == nil || candidate.ConnectedAt.Before(next.ConnectedAt) {
				next = candidate
			}
		}
		if next != nil {
			s.activeID = next.ID
		}
	}
	s.mu.Unlock()

	// Wait for in-flight work on this connection before closing its driver.
	conn.mu.Lock()
	conn.closed = true
	err := conn.Driver.Close()
	conn.mu.Unlock()
	if err != nil {
		return serviceError[bool](err.Error())
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}
