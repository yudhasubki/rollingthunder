package db

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"
)

func (connection *Connection) healthSnapshot() database.ConnectionHealth {
	connection.healthMu.RLock()
	defer connection.healthMu.RUnlock()
	health := connection.health
	health.ConnectionID = connection.ID
	if health.State == "" {
		health.State = database.ConnectionHealthUnknown
	}
	return health
}

func (connection *Connection) updateHealth(
	state string,
	message string,
	latency time.Duration,
	healthy bool,
) database.ConnectionHealth {
	connection.healthMu.Lock()
	defer connection.healthMu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	connection.health.ConnectionID = connection.ID
	connection.health.State = state
	connection.health.Message = message
	connection.health.LatencyMS = latency.Milliseconds()
	connection.health.LastChecked = now
	if healthy {
		connection.health.LastHealthy = now
		connection.health.FailureCount = 0
	} else if state == database.ConnectionHealthDegraded {
		connection.health.FailureCount++
	}
	return connection.health
}

func (s *Service) startHealthMonitor(parent context.Context) {
	if parent == nil || parent.Done() == nil || s.healthInterval <= 0 {
		return
	}
	if s.healthCancel != nil {
		s.healthCancel()
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	s.healthCancel = cancel
	s.healthDone = done
	go func() {
		defer close(done)
		ticker := time.NewTicker(s.healthInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.checkAllConnectionHealth(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (s *Service) checkAllConnectionHealth(ctx context.Context) {
	s.mu.RLock()
	connectionIDs := make([]string, 0, len(s.connections))
	for connectionID := range s.connections {
		connectionIDs = append(connectionIDs, connectionID)
	}
	s.mu.RUnlock()
	for _, connectionID := range connectionIDs {
		select {
		case <-ctx.Done():
			return
		default:
			_, _ = s.checkConnectionHealth(ctx, connectionID)
		}
	}
}

func (s *Service) checkConnectionHealth(
	parent context.Context,
	connectionID string,
) (database.ConnectionHealth, error) {
	connection, release, err := s.pinnedConnection(connectionID)
	if err != nil {
		return database.ConnectionHealth{}, err
	}
	defer release()

	healthDriver, ok := connection.Driver.(database.HealthDriver)
	if !ok {
		return connection.updateHealth(
			database.ConnectionHealthUnknown,
			"The active driver does not expose a health check.",
			0,
			false,
		), nil
	}
	timeout := s.healthTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	started := time.Now()
	err = healthDriver.Ping(ctx)
	latency := time.Since(started)
	if err != nil {
		return connection.updateHealth(
			database.ConnectionHealthDegraded,
			err.Error(),
			latency,
			false,
		), err
	}
	return connection.updateHealth(
		database.ConnectionHealthHealthy,
		"Connection healthy",
		latency,
		true,
	), nil
}

func (s *Service) CheckConnection(
	connectionID string,
) response.BaseResponse[database.ConnectionHealth] {
	parent := s.ctx
	if parent == nil {
		parent = context.Background()
	}
	health, err := s.checkConnectionHealth(parent, connectionID)
	if err != nil {
		return response.BaseResponse[database.ConnectionHealth]{
			Data: health,
			Errors: []response.BaseErrorResponse{
				{
					Status: http.StatusServiceUnavailable,
					Code:   errorCodeConnectionFailed,
					Title:  "Connection health check failed",
					Detail: err.Error(),
					Hint:   "Retry the health check or reconnect this database profile.",
				},
			},
		}
	}
	return response.BaseResponse[database.ConnectionHealth]{Data: health}
}

func (s *Service) GetConnectionHealth(
	connectionID string,
) response.BaseResponse[database.ConnectionHealth] {
	s.mu.RLock()
	connection := s.connections[connectionID]
	s.mu.RUnlock()
	if connection == nil {
		return serviceErrorWithCode[database.ConnectionHealth](
			http.StatusNotFound,
			errorCodeConnectionFailed,
			"Connection not found",
			"The connection is no longer active.",
			"Reconnect the saved profile.",
		)
	}
	return response.BaseResponse[database.ConnectionHealth]{
		Data: connection.healthSnapshot(),
	}
}

func (s *Service) GetConnectionHealths() response.BaseResponse[[]database.ConnectionHealth] {
	s.mu.RLock()
	connections := make([]*Connection, 0, len(s.connections))
	for _, connection := range s.connections {
		connections = append(connections, connection)
	}
	s.mu.RUnlock()
	health := make([]database.ConnectionHealth, 0, len(connections))
	for _, connection := range connections {
		health = append(health, connection.healthSnapshot())
	}
	return response.BaseResponse[[]database.ConnectionHealth]{Data: health}
}

func reconnectError(
	health database.ConnectionHealth,
	status int,
	title string,
	detail string,
	hint string,
) response.BaseResponse[database.ConnectionHealth] {
	return response.BaseResponse[database.ConnectionHealth]{
		Data: health,
		Errors: []response.BaseErrorResponse{
			{
				Status: status,
				Code:   errorCodeConnectionFailed,
				Title:  title,
				Detail: detail,
				Hint:   hint,
			},
		},
	}
}

func (s *Service) reconnectConfig(
	connection *Connection,
) (database.Config, error) {
	config := connection.Config
	if connection.ProfileID == "" {
		return config, nil
	}
	profiles, err := s.loadSavedConnections()
	if err != nil {
		return database.Config{}, err
	}
	for _, profile := range profiles {
		if profile.ID != connection.ProfileID {
			continue
		}
		config.Password = ""
		config.SSHPassword = ""
		config.SSHKeyPassphrase = ""
		config, err = s.hydrateProfileCredentials(profile, config)
		if err != nil {
			return database.Config{}, fmt.Errorf(
				"unlock saved connection credentials: %w",
				err,
			)
		}
		return config, nil
	}
	// A deleted saved profile does not invalidate an already-active session.
	// Reconnect can still use its in-memory configuration until it is closed.
	return config, nil
}

func (s *Service) ReconnectConnection(
	connectionID string,
	requestedAttemptID string,
) response.BaseResponse[database.ConnectionHealth] {
	s.mu.RLock()
	connection := s.connections[strings.TrimSpace(connectionID)]
	s.mu.RUnlock()
	if connection == nil {
		return serviceErrorWithCode[database.ConnectionHealth](
			http.StatusNotFound,
			errorCodeConnectionFailed,
			"Connection not found",
			"The connection is no longer active.",
			"Open the saved profile and connect again.",
		)
	}
	attempt, err := s.startConnectionAttempt(requestedAttemptID)
	if err != nil {
		return serviceErrorWithCode[database.ConnectionHealth](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Invalid reconnect request",
			err.Error(),
			"Retry with a unique connection attempt.",
		)
	}
	defer s.finishConnectionAttempt(attempt)

	connection.mu.Lock()
	defer connection.mu.Unlock()
	if connection.closed {
		return serviceErrorWithCode[database.ConnectionHealth](
			http.StatusGone,
			errorCodeConnectionFailed,
			"Connection is closed",
			"The connection was disconnected while reconnecting.",
			"Open the saved profile and connect again.",
		)
	}
	config, err := s.reconnectConfig(connection)
	if err != nil {
		health := connection.updateHealth(
			database.ConnectionHealthDegraded,
			err.Error(),
			0,
			false,
		)
		return reconnectError(
			health,
			http.StatusServiceUnavailable,
			"Reconnect failed",
			err.Error(),
			"The existing connection was kept. Unlock the operating system credential store and retry.",
		)
	}
	connection.updateHealth(
		database.ConnectionHealthReconnecting,
		"Opening a replacement connection",
		0,
		false,
	)
	effectiveConfig := config
	var replacementTunnel connectionTunnel
	if config.SSHEnabled {
		replacementTunnel, err = s.newTunnel(attempt.ctx, config)
		if err != nil {
			failure := connectionAttemptError(attempt, err)
			health := connection.updateHealth(
				database.ConnectionHealthDegraded,
				failure.Error(),
				0,
				false,
			)
			return reconnectError(
				health,
				http.StatusServiceUnavailable,
				"Reconnect failed",
				failure.Error(),
				"The existing connection was kept. Check the SSH endpoint, authentication, and verified host key.",
			)
		}
		effectiveConfig.TLSServerName = config.Host
		effectiveConfig.Host = replacementTunnel.LocalHost()
		effectiveConfig.Port = replacementTunnel.LocalPort()
	}
	replacement, err := s.newDriver(
		attempt.ctx,
		effectiveConfig.Driver,
		effectiveConfig,
	)
	if err != nil {
		if replacementTunnel != nil {
			_ = replacementTunnel.Close()
		}
		health := connection.updateHealth(
			database.ConnectionHealthDegraded,
			err.Error(),
			0,
			false,
		)
		return reconnectError(
			health,
			http.StatusBadRequest,
			"Reconnect failed",
			err.Error(),
			"The existing connection was kept. Review the saved profile and retry.",
		)
	}
	started := time.Now()
	if err := replacement.Connect(attempt.ctx); err != nil {
		_ = replacement.Close()
		if replacementTunnel != nil {
			_ = replacementTunnel.Close()
		}
		failure := connectionAttemptError(attempt, err)
		health := connection.updateHealth(
			database.ConnectionHealthDegraded,
			failure.Error(),
			time.Since(started),
			false,
		)
		return reconnectError(
			health,
			http.StatusServiceUnavailable,
			"Reconnect failed",
			failure.Error(),
			"The existing connection was kept. Check network access and retry.",
		)
	}
	if !s.claimConnectionAttempt(attempt) {
		_ = replacement.Close()
		if replacementTunnel != nil {
			_ = replacementTunnel.Close()
		}
		failure := connectionAttemptError(attempt, nil)
		health := connection.updateHealth(
			database.ConnectionHealthDegraded,
			failure.Error(),
			time.Since(started),
			false,
		)
		return reconnectError(
			health,
			http.StatusConflict,
			"Reconnect cancelled",
			failure.Error(),
			"The existing connection was kept.",
		)
	}
	previous := connection.Driver
	previousTunnel := connection.Tunnel
	connection.Driver = replacement
	connection.Tunnel = replacementTunnel
	connection.EndpointHost = effectiveConfig.Host
	connection.EndpointPort = effectiveConfig.Port
	connection.Config.Password = config.Password
	connection.Config.SSHPassword = config.SSHPassword
	connection.Config.SSHKeyPassphrase = config.SSHKeyPassphrase
	connection.ConnectedAt = time.Now()
	cleanupErr := previous.Close()
	if previousTunnel != nil {
		if err := previousTunnel.Close(); cleanupErr == nil {
			cleanupErr = err
		}
	}
	if cleanupErr != nil {
		health := connection.updateHealth(
			database.ConnectionHealthHealthy,
			fmt.Sprintf("Reconnected; old connection cleanup reported: %v", cleanupErr),
			time.Since(started),
			true,
		)
		return response.BaseResponse[database.ConnectionHealth]{Data: health}
	}
	health := connection.updateHealth(
		database.ConnectionHealthHealthy,
		"Reconnected",
		time.Since(started),
		true,
	)
	return response.BaseResponse[database.ConnectionHealth]{Data: health}
}

func (s *Service) Shutdown(_ context.Context) {
	if s.healthCancel != nil {
		s.healthCancel()
	}
	if s.healthDone != nil {
		select {
		case <-s.healthDone:
		case <-time.After(2 * time.Second):
		}
	}
	s.mu.RLock()
	connectionIDs := make([]string, 0, len(s.connections))
	for connectionID := range s.connections {
		connectionIDs = append(connectionIDs, connectionID)
	}
	s.mu.RUnlock()
	for _, connectionID := range connectionIDs {
		_ = s.DisconnectConnection(connectionID)
	}
}
