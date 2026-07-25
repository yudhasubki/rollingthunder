package db

import (
	"net/http"
	"path/filepath"

	"rollingthunder/internal/diagnostics"
	"rollingthunder/pkg/application"
	"rollingthunder/pkg/response"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (s *Service) GetDiagnosticsSettings() response.BaseResponse[diagnostics.Settings] {
	settings, err := s.diagnostics.Settings()
	if err != nil {
		return serviceError[diagnostics.Settings](err.Error())
	}
	return response.BaseResponse[diagnostics.Settings]{Data: settings}
}

func (s *Service) UpdateDiagnosticsSettings(
	settings diagnostics.Settings,
) response.BaseResponse[diagnostics.Settings] {
	updated, err := s.diagnostics.UpdateSettings(settings)
	if err != nil {
		return serviceError[diagnostics.Settings](err.Error())
	}
	return response.BaseResponse[diagnostics.Settings]{Data: updated}
}

func (s *Service) RecordFrontendError(
	report diagnostics.FrontendReport,
) response.BaseResponse[bool] {
	recorded, err := s.diagnostics.RecordFrontend(report)
	if err != nil {
		return serviceError[bool](err.Error())
	}
	return response.BaseResponse[bool]{Data: recorded}
}

func (s *Service) ClearDiagnostics() response.BaseResponse[bool] {
	if err := s.diagnostics.Clear(); err != nil {
		return serviceError[bool](err.Error())
	}
	return response.BaseResponse[bool]{Data: true}
}

func (s *Service) ExportDiagnostics() response.BaseResponse[diagnostics.ExportResult] {
	if s.ctx == nil {
		return serviceErrorWithCode[diagnostics.ExportResult](
			http.StatusServiceUnavailable,
			errorCodeDatabaseOperationFailed,
			"Application is not ready",
			"The native destination picker is unavailable.",
			"Wait for Rolling Thunder to finish starting and try again.",
		)
	}
	path, err := s.saveDialog(s.ctx, wailsruntime.SaveDialogOptions{
		Title:                "Export " + application.Name + " diagnostics",
		DefaultFilename:      application.Identifier + "-diagnostics.zip",
		CanCreateDirectories: true,
		Filters: []wailsruntime.FileFilter{
			{
				DisplayName: "ZIP archive (*.zip)",
				Pattern:     "*.zip",
			},
		},
	})
	if err != nil {
		return serviceError[diagnostics.ExportResult](err.Error())
	}
	if path == "" {
		return response.BaseResponse[diagnostics.ExportResult]{}
	}
	if filepath.Ext(path) == "" {
		path += ".zip"
	}
	exported, err := s.diagnostics.Export(path)
	if err != nil {
		return serviceError[diagnostics.ExportResult](err.Error())
	}
	return response.BaseResponse[diagnostics.ExportResult]{Data: exported}
}
