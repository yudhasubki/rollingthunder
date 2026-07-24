package db

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type saveFileDialogFunc func(
	context.Context,
	wailsruntime.SaveDialogOptions,
) (string, error)

var defaultSaveFileDialog saveFileDialogFunc = wailsruntime.SaveFileDialog

type exportWriterFunc func(io.Writer) (database.ExportStats, error)

func validateExportOptions(options database.ExportOptions) error {
	switch options.Format {
	case database.ExportFormatCSV:
		return nil
	default:
		return fmt.Errorf("unsupported export format %q", options.Format)
	}
}

func sanitizeSuggestedFilename(value string, fallback string) string {
	name := strings.TrimSpace(filepath.Base(value))
	if name == "" || name == "." {
		name = fallback
	}

	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "-",
		"<", "-",
		">", "-",
		"|", "-",
	)
	name = strings.Trim(replacer.Replace(name), " .")
	if name == "" {
		return fallback
	}
	return name
}

func ensureExportExtension(path string, format database.ExportFormat) string {
	if filepath.Ext(path) != "" {
		return path
	}
	if format == database.ExportFormatCSV {
		return path + ".csv"
	}
	return path
}

func replaceExportFile(tempPath string, targetPath string) error {
	if err := os.Rename(tempPath, targetPath); err == nil {
		return nil
	} else if _, statErr := os.Stat(targetPath); statErr != nil {
		return err
	}

	backupFile, err := os.CreateTemp(filepath.Dir(targetPath), ".rollingthunder-backup-*")
	if err != nil {
		return fmt.Errorf("prepare existing export backup: %w", err)
	}
	backupPath := backupFile.Name()
	if err := backupFile.Close(); err != nil {
		_ = os.Remove(backupPath)
		return fmt.Errorf("close export backup: %w", err)
	}
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("prepare export backup path: %w", err)
	}

	if err := os.Rename(targetPath, backupPath); err != nil {
		return fmt.Errorf("back up existing export: %w", err)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		restoreErr := os.Rename(backupPath, targetPath)
		if restoreErr != nil {
			return fmt.Errorf(
				"replace export: %w; additionally failed to restore previous file: %v",
				err,
				restoreErr,
			)
		}
		return fmt.Errorf("replace export: %w", err)
	}
	_ = os.Remove(backupPath)
	return nil
}

func (s *Service) writeExport(
	suggestedName string,
	options database.ExportOptions,
	write exportWriterFunc,
) (database.ExportResult, error) {
	if err := validateExportOptions(options); err != nil {
		return database.ExportResult{}, err
	}
	if s.ctx == nil {
		return database.ExportResult{}, fmt.Errorf("application context is unavailable")
	}

	defaultName := sanitizeSuggestedFilename(suggestedName, "rollingthunder-export.csv")
	path, err := s.saveDialog(s.ctx, wailsruntime.SaveDialogOptions{
		Title:           "Export CSV",
		DefaultFilename: defaultName,
		Filters: []wailsruntime.FileFilter{
			{
				DisplayName: "CSV files (*.csv)",
				Pattern:     "*.csv",
			},
		},
		CanCreateDirectories: true,
	})
	if err != nil {
		return database.ExportResult{}, fmt.Errorf("choose export destination: %w", err)
	}
	if path == "" {
		return database.ExportResult{
			Cancelled: true,
			Format:    options.Format,
		}, nil
	}
	path = ensureExportExtension(path, options.Format)

	tempFile, err := os.CreateTemp(filepath.Dir(path), ".rollingthunder-export-*")
	if err != nil {
		return database.ExportResult{}, fmt.Errorf("create export file: %w", err)
	}
	tempPath := tempFile.Name()
	keepTemp := false
	defer func() {
		if !keepTemp {
			_ = os.Remove(tempPath)
		}
	}()

	stats, writeErr := write(tempFile)
	if writeErr != nil {
		_ = tempFile.Close()
		return database.ExportResult{}, writeErr
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return database.ExportResult{}, fmt.Errorf("sync export file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return database.ExportResult{}, fmt.Errorf("close export file: %w", err)
	}

	fileInfo, err := os.Stat(tempPath)
	if err != nil {
		return database.ExportResult{}, fmt.Errorf("inspect export file: %w", err)
	}
	if err := replaceExportFile(tempPath, path); err != nil {
		return database.ExportResult{}, err
	}
	keepTemp = true

	return database.ExportResult{
		Path:   path,
		Rows:   stats.Rows,
		Bytes:  fileInfo.Size(),
		Format: options.Format,
	}, nil
}

func (s *Service) ExportTableData(
	connectionID string,
	request database.TableExportRequest,
) response.BaseResponse[database.ExportResult] {
	result, err := s.writeExport(request.SuggestedName, request.Options, func(writer io.Writer) (database.ExportStats, error) {
		driver, release, err := s.driverFor(connectionID)
		if err != nil {
			return database.ExportStats{}, err
		}
		defer release()

		return driver.ExportTable(s.ctx, request, writer)
	})
	if err != nil {
		return serviceError[database.ExportResult](err.Error())
	}
	return response.BaseResponse[database.ExportResult]{Data: result}
}

func (s *Service) ExportQueryResults(
	request database.RowsExportRequest,
) response.BaseResponse[database.ExportResult] {
	result, err := s.writeExport(request.SuggestedName, request.Options, func(writer io.Writer) (database.ExportStats, error) {
		return database.WriteCSVRows(
			writer,
			request.Columns,
			request.Rows,
			request.Options.CSV,
		)
	})
	if err != nil {
		return serviceError[database.ExportResult](err.Error())
	}
	return response.BaseResponse[database.ExportResult]{Data: result}
}
