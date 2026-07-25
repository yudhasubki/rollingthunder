package db

import (
	"errors"
	"net/http"
	"strings"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"
)

var errConnectionReadOnly = errors.New("connection is locked in read-only mode")

type ConnectionWriteAccess struct {
	ConnectionID    string `json:"connectionId"`
	AccessMode      string `json:"accessMode"`
	WriteEnabled    bool   `json:"writeEnabled"`
	TemporaryUnlock bool   `json:"temporaryUnlock"`
	Confirmation    string `json:"confirmation"`
}

type SetConnectionWriteAccessRequest struct {
	ConnectionID string `json:"connectionId"`
	Enable       bool   `json:"enable"`
	Confirmation string `json:"confirmation"`
}

func connectionWriteAccessLocked(connection *Connection) ConnectionWriteAccess {
	accessMode := database.NormalizeConnectionAccessMode(
		connection.Config.AccessMode,
		connection.Config.Environment,
	)
	readOnly := accessMode == database.ConnectionAccessReadOnly
	confirmation := strings.TrimSpace(connection.Name)
	if confirmation == "" {
		confirmation = strings.TrimSpace(connection.Config.Db)
	}
	return ConnectionWriteAccess{
		ConnectionID:    connection.ID,
		AccessMode:      accessMode,
		WriteEnabled:    !readOnly || connection.writeUnlocked,
		TemporaryUnlock: readOnly && connection.writeUnlocked,
		Confirmation:    confirmation,
	}
}

func (s *Service) GetConnectionWriteAccess(
	connectionID string,
) response.BaseResponse[ConnectionWriteAccess] {
	connection, release, err := s.pinnedConnection(connectionID)
	if err != nil {
		return serviceErrorWithCode[ConnectionWriteAccess](
			http.StatusNotFound,
			errorCodeInvalidRequest,
			"Connection unavailable",
			err.Error(),
			"Reconnect the database before changing its write-access state.",
		)
	}
	defer release()
	return response.BaseResponse[ConnectionWriteAccess]{
		Data: connectionWriteAccessLocked(connection),
	}
}

func (s *Service) SetConnectionWriteAccess(
	request SetConnectionWriteAccessRequest,
) response.BaseResponse[ConnectionWriteAccess] {
	connectionID := strings.TrimSpace(request.ConnectionID)
	if connectionID == "" {
		return serviceErrorWithCode[ConnectionWriteAccess](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Connection is required",
			"A connection ID is required to change write access.",
			"Choose an active connection and try again.",
		)
	}

	s.mu.RLock()
	connection := s.connections[connectionID]
	if connection == nil {
		s.mu.RUnlock()
		return serviceErrorWithCode[ConnectionWriteAccess](
			http.StatusNotFound,
			errorCodeInvalidRequest,
			"Connection unavailable",
			"The connection is no longer active.",
			"Reconnect the database before changing its write-access state.",
		)
	}
	connection.mu.Lock()
	s.mu.RUnlock()
	defer connection.mu.Unlock()
	if connection.closed {
		return serviceErrorWithCode[ConnectionWriteAccess](
			http.StatusGone,
			errorCodeInvalidRequest,
			"Connection unavailable",
			"The connection has already been closed.",
			"Reconnect the database before changing its write-access state.",
		)
	}

	status := connectionWriteAccessLocked(connection)
	if !request.Enable {
		connection.writeUnlocked = false
		return response.BaseResponse[ConnectionWriteAccess]{
			Data: connectionWriteAccessLocked(connection),
		}
	}
	if status.AccessMode == database.ConnectionAccessReadWrite {
		return response.BaseResponse[ConnectionWriteAccess]{Data: status}
	}
	if status.Confirmation == "" ||
		strings.TrimSpace(request.Confirmation) != status.Confirmation {
		return serviceErrorWithCode[ConnectionWriteAccess](
			http.StatusConflict,
			errorCodeReadOnlyConnection,
			"Write unlock confirmation required",
			"Write access remains locked because the connection name did not match.",
			"Type the exact connection name shown in the confirmation dialog.",
		)
	}
	connection.writeUnlocked = true
	return response.BaseResponse[ConnectionWriteAccess]{
		Data: connectionWriteAccessLocked(connection),
	}
}

func (s *Service) writeDriverFor(
	connectionID string,
) (database.Driver, func(), error) {
	connection, release, err := s.writePinnedConnection(connectionID)
	if err != nil {
		return nil, nil, err
	}
	return connection.Driver, release, nil
}

func (s *Service) writePinnedConnection(
	connectionID string,
) (*Connection, func(), error) {
	connection, release, err := s.pinnedConnection(connectionID)
	if err != nil {
		return nil, nil, err
	}
	if !connectionWriteAccessLocked(connection).WriteEnabled {
		release()
		return nil, nil, errConnectionReadOnly
	}
	return connection, release, nil
}

func readOnlyConnectionError[T any]() response.BaseResponse[T] {
	return serviceErrorWithCode[T](
		http.StatusLocked,
		errorCodeReadOnlyConnection,
		"Connection is read-only",
		"Rolling Thunder blocked a database write for this connection.",
		"Temporarily unlock writes from the active-connection guard, or edit the saved profile access mode.",
	)
}
