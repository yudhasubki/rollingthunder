package db

import (
	"context"
	"net/http"
	"time"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"
)

func (s *Service) activityContext() (context.Context, context.CancelFunc) {
	parent := s.ctx
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, activityTimeout)
}

func (s *Service) GetDatabaseActivity(
	connectionID string,
) response.BaseResponse[database.DatabaseActivity] {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return serviceError[database.DatabaseActivity](err.Error())
	}
	defer release()
	activityDriver, ok := driver.(database.ActivityDriver)
	if !ok {
		return response.BaseResponse[database.DatabaseActivity]{
			Data: database.DatabaseActivity{
				Supported:  false,
				Engine:     driver.Capabilities().Engine,
				Sessions:   []database.DatabaseSession{},
				CapturedAt: time.Now(),
				Message:    "This engine does not expose server sessions. SQLite work runs inside the application process.",
			},
		}
	}
	ctx, cancel := s.activityContext()
	defer cancel()
	activity, err := activityDriver.GetDatabaseActivity(ctx)
	if err != nil {
		return serviceErrorWithCode[database.DatabaseActivity](
			http.StatusForbidden,
			errorCodeActivityFailed,
			"Could not load database activity",
			err.Error(),
			"Connect with an account allowed to inspect server sessions.",
		)
	}
	if activity.Sessions == nil {
		activity.Sessions = []database.DatabaseSession{}
	}
	return response.BaseResponse[database.DatabaseActivity]{Data: activity}
}

func (s *Service) CancelDatabaseSession(
	request database.CancelSessionRequest,
) response.BaseResponse[database.CancelSessionResult] {
	if err := request.Validate(); err != nil {
		return serviceErrorWithCode[database.CancelSessionResult](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Session action requires confirmation",
			err.Error(),
			"Review the session details and explicitly confirm the action.",
		)
	}
	driver, release, err := s.driverFor(request.ConnectionID)
	if err != nil {
		return serviceError[database.CancelSessionResult](err.Error())
	}
	defer release()
	activityDriver, ok := driver.(database.ActivityDriver)
	if !ok {
		return serviceErrorWithCode[database.CancelSessionResult](
			http.StatusNotImplemented,
			errorCodeActivityUnsupported,
			"Session cancellation unavailable",
			"The connected engine does not expose cancellable server sessions.",
			"Cancel a Rolling Thunder query from its query tab instead.",
		)
	}
	ctx, cancel := s.activityContext()
	defer cancel()
	if err := activityDriver.CancelDatabaseSession(
		ctx,
		request.SessionID,
		request.Terminate,
	); err != nil {
		return serviceErrorWithCode[database.CancelSessionResult](
			http.StatusForbidden,
			errorCodeSessionCancellationFailed,
			"Could not stop database session",
			err.Error(),
			"Verify session ownership and server privileges, then refresh activity.",
		)
	}
	return response.BaseResponse[database.CancelSessionResult]{
		Data: database.CancelSessionResult{
			Cancelled:  true,
			Terminated: request.Terminate,
			SessionID:  request.SessionID,
		},
	}
}
