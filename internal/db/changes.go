package db

import (
	"net/http"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"
)

func (s *Service) ApplyTableChanges(
	connectionID string,
	changes database.TableChangeSet,
) response.BaseResponse[database.TableChangeResult] {
	if changes.Count() == 0 {
		return serviceErrorWithCode[database.TableChangeResult](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"No staged changes",
			"There are no row changes to apply.",
			"Edit, add, or delete a row before reviewing changes.",
		)
	}
	if s.ctx == nil {
		return serviceErrorWithCode[database.TableChangeResult](
			http.StatusServiceUnavailable,
			errorCodeTableChangesFailed,
			"Application is not ready",
			"The application context is unavailable.",
			"Wait for Rolling Thunder to finish starting, then try again.",
		)
	}

	driver, release, err := s.writeDriverFor(connectionID)
	if err != nil {
		if err == errConnectionReadOnly {
			return readOnlyConnectionError[database.TableChangeResult]()
		}
		return serviceErrorWithCode[database.TableChangeResult](
			http.StatusNotFound,
			errorCodeTableChangesFailed,
			"Connection unavailable",
			err.Error(),
			"Reconnect the database before applying staged changes.",
		)
	}
	defer release()

	changeDriver, ok := driver.(database.TableChangeDriver)
	if !ok {
		return serviceErrorWithCode[database.TableChangeResult](
			http.StatusNotImplemented,
			errorCodeTableChangesUnsupported,
			"Atomic row changes are not supported",
			"The active database driver cannot apply a reviewed change set atomically.",
			"Use a supported driver or apply the equivalent SQL inside an explicit transaction.",
		)
	}

	result, err := changeDriver.ApplyTableChanges(s.ctx, changes)
	if err != nil {
		return serviceErrorWithCode[database.TableChangeResult](
			http.StatusConflict,
			errorCodeTableChangesFailed,
			"Changes were not applied",
			err.Error(),
			"The complete change set was rolled back. Review the highlighted rows and try again.",
		)
	}
	return response.BaseResponse[database.TableChangeResult]{Data: result}
}
