package db

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"rollingthunder/internal/diagnostics"
	"rollingthunder/internal/updater"
	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"

	"github.com/google/uuid"
)

// Connection represents an active database connection
type Connection struct {
	ID            string           `json:"id"`
	ProfileID     string           `json:"profileId,omitempty"`
	Name          string           `json:"name"`
	Driver        database.Driver  `json:"-"`
	Config        database.Config  `json:"-"`
	Tunnel        connectionTunnel `json:"-"`
	EndpointHost  string           `json:"-"`
	EndpointPort  string           `json:"-"`
	Environment   string           `json:"environment"`
	ConnectedAt   time.Time        `json:"connectedAt"`
	mu            sync.RWMutex
	healthMu      sync.RWMutex
	health        database.ConnectionHealth
	writeUnlocked bool
	closed        bool
}

// effectiveConfig keeps the profile endpoint available to the UI while
// routing external client tools through the same local SSH endpoint as the
// active driver.
func (connection *Connection) effectiveConfig() database.Config {
	config := connection.Config
	if strings.TrimSpace(connection.EndpointHost) != "" {
		if connection.Config.SSHEnabled {
			config.TLSServerName = connection.Config.Host
		}
		config.Host = connection.EndpointHost
	}
	if strings.TrimSpace(connection.EndpointPort) != "" {
		config.Port = connection.EndpointPort
	}
	return config
}

// ConnectionInfo is the public info about a connection (without driver)
type ConnectionInfo struct {
	ID            string                    `json:"id"`
	ProfileID     string                    `json:"profileId,omitempty"`
	Name          string                    `json:"name"`
	Driver        string                    `json:"driver"`
	Database      string                    `json:"database"`
	Host          string                    `json:"host"`
	Environment   string                    `json:"environment"`
	AccessMode    string                    `json:"accessMode"`
	ReadOnly      bool                      `json:"readOnly"`
	WriteUnlocked bool                      `json:"writeUnlocked"`
	SSHTunnel     bool                      `json:"sshTunnel"`
	IsActive      bool                      `json:"isActive"`
	Health        database.ConnectionHealth `json:"health"`
}

type Service struct {
	ctx                 context.Context
	connections         map[string]*Connection
	activeID            string
	mu                  sync.RWMutex
	saveDialog          saveFileDialogFunc
	sqliteOpenDialog    openFileDialogFunc
	sqliteSaveDialog    saveFileDialogFunc
	oracleTNSOpenDialog openFileDialogFunc
	oracleWalletDialog  openFileDialogFunc
	importOpenDialog    openFileDialogFunc
	restoreOpenDialog   openFileDialogFunc
	sqlOpenDialog       openFileDialogFunc
	importFiles         map[string]importFileGrant
	importFileMu        sync.RWMutex
	restoreFiles        map[string]restoreFileGrant
	restoreFileMu       sync.RWMutex
	sqlFiles            map[string]sqlFileGrant
	sqlFileMu           sync.RWMutex
	exportJobs          map[string]*exportJob
	exportMu            sync.RWMutex
	maintenanceJobs     map[string]*maintenanceJob
	maintenanceMu       sync.RWMutex
	lookPath            executableLookup
	commandContext      commandFactory
	newDriver           driverFactory
	newTunnel           tunnelFactory
	connectionTimeout   time.Duration
	connectionAttempts  map[string]*connectionAttempt
	connectionAttemptMu sync.Mutex
	queryAttempts       map[string]*queryAttempt
	queryAttemptMu      sync.RWMutex
	transactions        map[string]*transactionSession
	transactionMu       sync.RWMutex
	connectionStorage   *ConnectionStorage
	credentialStore     CredentialStore
	healthInterval      time.Duration
	healthTimeout       time.Duration
	healthCancel        context.CancelFunc
	healthDone          chan struct{}
	diagnostics         *diagnostics.Manager
	updateChecker       *updater.Checker
}

func NewService() *Service {
	return NewServiceWithDiagnostics(diagnostics.NewManager())
}

func NewServiceWithDiagnostics(
	diagnosticManager *diagnostics.Manager,
) *Service {
	return NewServiceWithDiagnosticsAndVersion(
		diagnosticManager,
		defaultServiceVersion,
	)
}

func NewServiceWithDiagnosticsAndVersion(
	diagnosticManager *diagnostics.Manager,
	currentVersion string,
) *Service {
	if diagnosticManager == nil {
		diagnosticManager = diagnostics.NewManager()
	}
	return &Service{
		connections:         make(map[string]*Connection),
		saveDialog:          defaultSaveFileDialog,
		sqliteOpenDialog:    defaultOpenFileDialog,
		sqliteSaveDialog:    defaultSaveFileDialog,
		oracleTNSOpenDialog: defaultOpenFileDialog,
		oracleWalletDialog:  defaultOpenDirectoryDialog,
		importOpenDialog:    defaultOpenFileDialog,
		restoreOpenDialog:   defaultOpenFileDialog,
		sqlOpenDialog:       defaultOpenFileDialog,
		importFiles:         make(map[string]importFileGrant),
		restoreFiles:        make(map[string]restoreFileGrant),
		sqlFiles:            make(map[string]sqlFileGrant),
		exportJobs:          make(map[string]*exportJob),
		maintenanceJobs:     make(map[string]*maintenanceJob),
		lookPath:            defaultExecutableLookup,
		commandContext:      defaultCommandFactory,
		newDriver:           NewDriver,
		newTunnel:           newSSHTunnel,
		connectionTimeout:   defaultConnectionTimeout,
		connectionAttempts:  make(map[string]*connectionAttempt),
		queryAttempts:       make(map[string]*queryAttempt),
		transactions:        make(map[string]*transactionSession),
		connectionStorage:   NewConnectionStorage(),
		credentialStore:     newOperatingSystemCredentialStore(),
		healthInterval:      defaultHealthMonitorInterval,
		healthTimeout:       defaultHealthCheckTimeout,
		diagnostics:         diagnosticManager,
		updateChecker:       updater.NewChecker(currentVersion),
	}
}

func (s *Service) Start(ctx context.Context) {
	s.ctx = ctx
	s.startHealthMonitor(ctx)
}

func serviceError[T any](detail string) response.BaseResponse[T] {
	return serviceErrorWithCode[T](
		500,
		errorCodeDatabaseOperationFailed,
		"Database operation failed",
		detail,
		"Retry the operation or inspect the activity console for more context.",
	)
}

// driverFor pins an active operation to one connection. The release function
// must be called after the driver operation completes. Disconnect waits for
// pinned operations instead of closing a driver while it is still in use.
func (s *Service) pinnedConnection(
	connectionID string,
) (*Connection, func(), error) {
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

	return conn, conn.mu.RUnlock, nil
}

func (s *Service) driverFor(connectionID string) (database.Driver, func(), error) {
	conn, release, err := s.pinnedConnection(connectionID)
	if err != nil {
		return nil, nil, err
	}
	return conn.Driver, release, nil
}

func (s *Service) Connect(req ConnectRequest) response.BaseResponse[ConnectResponse] {
	req.Config = database.NormalizeConfigMetadata(req.Config)
	driverName := req.Driver
	if driverName == "" {
		driverName = req.Config.Driver
	}
	if driverName == "" {
		driverName = database.DriverPostgres
	}
	req.Config.Driver = driverName
	if err := req.Config.ValidateSafety(); err != nil {
		return serviceErrorWithCode[ConnectResponse](
			400,
			errorCodeInvalidRequest,
			"Invalid connection settings",
			err.Error(),
			"Review the connection profile and try again.",
		)
	}

	attempt, err := s.startConnectionAttempt(req.AttemptID)
	if err != nil {
		return serviceErrorWithCode[ConnectResponse](
			400,
			errorCodeInvalidRequest,
			"Invalid connection request",
			err.Error(),
			"Check the connection profile and try again.",
		)
	}
	defer s.finishConnectionAttempt(attempt)

	effectiveConfig := req.Config
	var tunnel connectionTunnel
	if req.Config.SSHEnabled {
		tunnel, err = s.newTunnel(attempt.ctx, req.Config)
		if err != nil {
			return connectionFailure[ConnectResponse](attempt, err)
		}
		effectiveConfig.TLSServerName = req.Config.Host
		effectiveConfig.Host = tunnel.LocalHost()
		effectiveConfig.Port = tunnel.LocalPort()
	}

	driver, err := s.newDriver(attempt.ctx, driverName, effectiveConfig)
	if err != nil {
		if tunnel != nil {
			_ = tunnel.Close()
		}
		return serviceErrorWithCode[ConnectResponse](
			400,
			errorCodeInvalidRequest,
			"Unsupported database provider",
			err.Error(),
			"Choose one of the database providers currently marked as available.",
		)
	}

	err = driver.Connect(attempt.ctx)
	if err != nil {
		_ = driver.Close()
		if tunnel != nil {
			_ = tunnel.Close()
		}
		return connectionFailure[ConnectResponse](attempt, err)
	}

	if !s.claimConnectionAttempt(attempt) {
		_ = driver.Close()
		if tunnel != nil {
			_ = tunnel.Close()
		}
		return connectionFailure[ConnectResponse](
			attempt,
			connectionAttemptError(attempt, nil),
		)
	}

	// Generate connection ID and store in registry
	connID := uuid.New().String()
	connectedAt := time.Now()
	healthTimestamp := connectedAt.UTC().Format(time.RFC3339Nano)
	conn := &Connection{
		ID:           connID,
		ProfileID:    req.ProfileID,
		Name:         req.Config.Name,
		Driver:       driver,
		Config:       req.Config,
		Tunnel:       tunnel,
		EndpointHost: effectiveConfig.Host,
		EndpointPort: effectiveConfig.Port,
		Environment:  req.Config.Environment,
		ConnectedAt:  connectedAt,
		health: database.ConnectionHealth{
			ConnectionID: connID,
			State:        database.ConnectionHealthHealthy,
			Message:      "Connected",
			LastChecked:  healthTimestamp,
			LastHealthy:  healthTimestamp,
		},
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
	driver, release, err := s.writeDriverFor(connectionID)
	if err != nil {
		if err == errConnectionReadOnly {
			return readOnlyConnectionError[bool]()
		}
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
	driver, release, err := s.writeDriverFor(connectionID)
	if err != nil {
		if err == errConnectionReadOnly {
			return readOnlyConnectionError[bool]()
		}
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
	driver, release, err := s.writeDriverFor(connectionID)
	if err != nil {
		if err == errConnectionReadOnly {
			return readOnlyConnectionError[bool]()
		}
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

// CreateTable creates a new table in the database
func (s *Service) CreateTable(connectionID string, table database.Table, columns []database.ColumnDefinition) response.BaseResponse[bool] {
	driver, release, err := s.writeDriverFor(connectionID)
	if err != nil {
		if err == errConnectionReadOnly {
			return readOnlyConnectionError[bool]()
		}
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
	driver, release, err := s.writeDriverFor(connectionID)
	if err != nil {
		if err == errConnectionReadOnly {
			return readOnlyConnectionError[bool]()
		}
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
	driver, release, err := s.writeDriverFor(connectionID)
	if err != nil {
		if err == errConnectionReadOnly {
			return readOnlyConnectionError[bool]()
		}
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
	type connectionSnapshot struct {
		info        ConnectionInfo
		connectedAt time.Time
	}

	// Copy public fields while holding the same locks used by reconnect and
	// disconnect, then sort the immutable snapshots outside the registry lock.
	s.mu.RLock()
	activeID := s.activeID
	snapshots := make([]connectionSnapshot, 0, len(s.connections))
	for _, conn := range s.connections {
		conn.mu.RLock()
		writeAccess := connectionWriteAccessLocked(conn)
		displayHost := conn.Config.Host
		if conn.Config.Driver == database.DriverOracle &&
			conn.Config.OracleConnectionMode == "tns" {
			displayHost = conn.Config.OracleTNSAlias
		}
		snapshot := connectionSnapshot{
			info: ConnectionInfo{
				ID:            conn.ID,
				ProfileID:     conn.ProfileID,
				Name:          conn.Name,
				Driver:        conn.Config.Driver,
				Database:      conn.Config.Db,
				Host:          displayHost,
				Environment:   conn.Environment,
				AccessMode:    writeAccess.AccessMode,
				ReadOnly:      writeAccess.AccessMode == database.ConnectionAccessReadOnly,
				WriteUnlocked: writeAccess.TemporaryUnlock,
				SSHTunnel:     conn.Config.SSHEnabled,
				IsActive:      conn.ID == activeID,
				Health:        conn.healthSnapshot(),
			},
			connectedAt: conn.ConnectedAt,
		}
		conn.mu.RUnlock()
		snapshots = append(snapshots, snapshot)
	}
	s.mu.RUnlock()

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].connectedAt.Before(snapshots[j].connectedAt)
	})

	connections := make([]ConnectionInfo, 0, len(snapshots))
	for _, snapshot := range snapshots {
		connections = append(connections, snapshot.info)
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
	s.rollbackTransactionsForConnection(connectionID)
	err := conn.Driver.Close()
	if conn.Tunnel != nil {
		if tunnelErr := conn.Tunnel.Close(); err == nil {
			err = tunnelErr
		}
	}
	conn.mu.Unlock()
	if err != nil {
		return serviceError[bool](err.Error())
	}

	return response.BaseResponse[bool]{
		Data: true,
	}
}
