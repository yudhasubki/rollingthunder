package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"

	"github.com/google/uuid"
)

const defaultConnectionTimeout = 15 * time.Second

type ConnectRequest struct {
	Driver    string          `json:"driver"`
	Config    database.Config `json:"config"`
	AttemptID string          `json:"attemptId,omitempty"`
}

type ConnectResponse struct {
	Connected    bool   `json:"connected"`
	ConnectionID string `json:"connectionId,omitempty"`
}

type connectionAttempt struct {
	id        string
	ctx       context.Context
	cancel    context.CancelFunc
	timeout   time.Duration
	cancelled atomic.Bool
}

func (s *Service) startConnectionAttempt(
	requestedID string,
) (*connectionAttempt, error) {
	if s.ctx == nil {
		return nil, fmt.Errorf("application context is unavailable")
	}

	attemptID := strings.TrimSpace(requestedID)
	if attemptID == "" {
		attemptID = uuid.NewString()
	}
	if len(attemptID) > 128 {
		return nil, fmt.Errorf("connection attempt ID is too long")
	}

	timeout := s.connectionTimeout
	if timeout <= 0 {
		timeout = defaultConnectionTimeout
	}
	ctx, cancel := context.WithTimeout(s.ctx, timeout)
	attempt := &connectionAttempt{
		id:      attemptID,
		ctx:     ctx,
		cancel:  cancel,
		timeout: timeout,
	}

	s.connectionAttemptMu.Lock()
	if _, exists := s.connectionAttempts[attemptID]; exists {
		s.connectionAttemptMu.Unlock()
		cancel()
		return nil, fmt.Errorf("connection attempt %q is already running", attemptID)
	}
	s.connectionAttempts[attemptID] = attempt
	s.connectionAttemptMu.Unlock()
	return attempt, nil
}

func (s *Service) finishConnectionAttempt(attempt *connectionAttempt) {
	if attempt == nil {
		return
	}
	attempt.cancel()

	s.connectionAttemptMu.Lock()
	if s.connectionAttempts[attempt.id] == attempt {
		delete(s.connectionAttempts, attempt.id)
	}
	s.connectionAttemptMu.Unlock()
}

// claimConnectionAttempt makes successful completion atomic with cancellation.
// If cancellation wins the lock first, the connected driver must be discarded.
func (s *Service) claimConnectionAttempt(attempt *connectionAttempt) bool {
	s.connectionAttemptMu.Lock()
	defer s.connectionAttemptMu.Unlock()

	if s.connectionAttempts[attempt.id] != attempt ||
		attempt.cancelled.Load() ||
		attempt.ctx.Err() != nil {
		return false
	}
	delete(s.connectionAttempts, attempt.id)
	return true
}

func connectionAttemptError(attempt *connectionAttempt, connectErr error) error {
	contextErr := attempt.ctx.Err()
	switch {
	case errors.Is(contextErr, context.DeadlineExceeded),
		errors.Is(connectErr, context.DeadlineExceeded):
		return fmt.Errorf(
			"connection timed out after %s",
			formatConnectionTimeout(attempt.timeout),
		)
	case attempt.cancelled.Load(),
		errors.Is(contextErr, context.Canceled),
		errors.Is(connectErr, context.Canceled):
		return fmt.Errorf("connection attempt cancelled")
	case connectErr != nil:
		return connectErr
	default:
		return fmt.Errorf("connection attempt did not complete")
	}
}

func formatConnectionTimeout(timeout time.Duration) string {
	if timeout%time.Second == 0 {
		seconds := int(timeout / time.Second)
		if seconds == 1 {
			return "1 second"
		}
		return fmt.Sprintf("%d seconds", seconds)
	}
	return timeout.String()
}

func (s *Service) CancelConnectionAttempt(
	attemptID string,
) response.BaseResponse[bool] {
	attemptID = strings.TrimSpace(attemptID)
	s.connectionAttemptMu.Lock()
	attempt := s.connectionAttempts[attemptID]
	if attempt == nil {
		s.connectionAttemptMu.Unlock()
		return serviceError[bool]("connection attempt is not running")
	}
	attempt.cancelled.Store(true)
	attempt.cancel()
	s.connectionAttemptMu.Unlock()

	return response.BaseResponse[bool]{Data: true}
}
