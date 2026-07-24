package db

import (
	"context"
	"net/http"

	"rollingthunder/internal/updater"
	"rollingthunder/pkg/response"
)

const errorCodeUpdateCheckFailed = "UPDATE_CHECK_FAILED"

func (s *Service) CheckForUpdates() response.BaseResponse[updater.CheckResult] {
	if s.updateChecker == nil {
		return serviceErrorWithCode[updater.CheckResult](
			http.StatusServiceUnavailable,
			errorCodeUpdateCheckFailed,
			"Update check is unavailable",
			"The update service was not initialized.",
			"Restart Rolling Thunder and try again.",
		)
	}

	parent := s.ctx
	if parent == nil {
		parent = context.Background()
	}
	result, err := s.updateChecker.Check(parent)
	if err != nil {
		return serviceErrorWithCode[updater.CheckResult](
			http.StatusBadGateway,
			errorCodeUpdateCheckFailed,
			"Could not check for updates",
			err.Error(),
			"Rolling Thunder will keep working normally. Check GitHub Releases manually if needed.",
		)
	}
	return response.BaseResponse[updater.CheckResult]{Data: result}
}
