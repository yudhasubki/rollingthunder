package db

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"

	"github.com/google/uuid"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type saveFileDialogFunc func(
	context.Context,
	wailsruntime.SaveDialogOptions,
) (string, error)

var defaultSaveFileDialog saveFileDialogFunc = wailsruntime.SaveFileDialog

type exportWriterFunc func(context.Context, io.Writer) (database.ExportStats, error)

const (
	exportStatusPreparing  = "preparing"
	exportStatusRunning    = "running"
	exportStatusCancelling = "cancelling"
)

type exportJob struct {
	id        string
	cancel    context.CancelFunc
	startedAt time.Time
	totalRows int64
	rows      atomic.Int64
	bytes     atomic.Int64
	status    atomic.Value
}

func newExportJob(id string, totalRows int64, cancel context.CancelFunc) *exportJob {
	job := &exportJob{
		id:        id,
		cancel:    cancel,
		startedAt: time.Now(),
		totalRows: max(totalRows, 0),
	}
	job.status.Store(exportStatusPreparing)
	return job
}

func (job *exportJob) progress() database.ExportProgress {
	status, _ := job.status.Load().(string)
	return database.ExportProgress{
		JobID:       job.id,
		Status:      status,
		Rows:        job.rows.Load(),
		Bytes:       job.bytes.Load(),
		TotalRows:   job.totalRows,
		ElapsedMS:   time.Since(job.startedAt).Milliseconds(),
		Cancellable: status != exportStatusCancelling,
	}
}

type exportProgressWriter struct {
	writer io.Writer
	job    *exportJob
}

func (writer *exportProgressWriter) Write(value []byte) (int, error) {
	written, err := writer.writer.Write(value)
	writer.job.bytes.Add(int64(written))
	return written, err
}

type exportFileConfig struct {
	title           string
	defaultFilename string
	extension       string
	filter          wailsruntime.FileFilter
}

func exportFileConfiguration(format database.ExportFormat) (exportFileConfig, error) {
	switch format {
	case database.ExportFormatCSV:
		return exportFileConfig{
			title:           "Export CSV",
			defaultFilename: "rollingthunder-export.csv",
			extension:       ".csv",
			filter: wailsruntime.FileFilter{
				DisplayName: "CSV files (*.csv)",
				Pattern:     "*.csv",
			},
		}, nil
	case database.ExportFormatJSON:
		return exportFileConfig{
			title:           "Export JSON",
			defaultFilename: "rollingthunder-export.json",
			extension:       ".json",
			filter: wailsruntime.FileFilter{
				DisplayName: "JSON files (*.json)",
				Pattern:     "*.json",
			},
		}, nil
	case database.ExportFormatSQL:
		return exportFileConfig{
			title:           "Export SQL",
			defaultFilename: "rollingthunder-export.sql",
			extension:       ".sql",
			filter: wailsruntime.FileFilter{
				DisplayName: "SQL files (*.sql)",
				Pattern:     "*.sql",
			},
		}, nil
	default:
		return exportFileConfig{}, fmt.Errorf("unsupported export format %q", format)
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
	config, err := exportFileConfiguration(format)
	if err != nil {
		return path
	}
	extension := filepath.Ext(path)
	if strings.EqualFold(extension, config.extension) {
		return path
	}
	if extension == "" {
		return path + config.extension
	}
	return strings.TrimSuffix(path, extension) + config.extension
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

func (s *Service) startExportJob(
	requestedID string,
	totalRows int64,
) (context.Context, *exportJob, error) {
	if s.ctx == nil {
		return nil, nil, fmt.Errorf("application context is unavailable")
	}

	jobID := strings.TrimSpace(requestedID)
	if jobID == "" {
		jobID = uuid.NewString()
	}
	if len(jobID) > 128 {
		return nil, nil, fmt.Errorf("export job ID is too long")
	}

	ctx, cancel := context.WithCancel(s.ctx)
	job := newExportJob(jobID, totalRows, cancel)

	s.exportMu.Lock()
	if _, exists := s.exportJobs[jobID]; exists {
		s.exportMu.Unlock()
		cancel()
		return nil, nil, fmt.Errorf("export job %q is already running", jobID)
	}
	s.exportJobs[jobID] = job
	s.exportMu.Unlock()

	ctx = database.WithExportProgressReporter(ctx, func(rows int64) {
		job.rows.Store(rows)
	})
	return ctx, job, nil
}

func (s *Service) finishExportJob(job *exportJob) {
	if job == nil {
		return
	}
	job.cancel()

	s.exportMu.Lock()
	if s.exportJobs[job.id] == job {
		delete(s.exportJobs, job.id)
	}
	s.exportMu.Unlock()
}

func (s *Service) GetExportProgress(
	jobID string,
) response.BaseResponse[database.ExportProgress] {
	s.exportMu.RLock()
	job := s.exportJobs[strings.TrimSpace(jobID)]
	s.exportMu.RUnlock()
	if job == nil {
		return serviceError[database.ExportProgress]("export job is not running")
	}
	return response.BaseResponse[database.ExportProgress]{Data: job.progress()}
}

func (s *Service) CancelExport(jobID string) response.BaseResponse[bool] {
	s.exportMu.RLock()
	job := s.exportJobs[strings.TrimSpace(jobID)]
	s.exportMu.RUnlock()
	if job == nil {
		return serviceError[bool]("export job is not running")
	}

	job.status.Store(exportStatusCancelling)
	job.cancel()
	return response.BaseResponse[bool]{Data: true}
}

func (s *Service) writeExport(
	ctx context.Context,
	job *exportJob,
	suggestedName string,
	options database.ExportOptions,
	write exportWriterFunc,
) (database.ExportResult, error) {
	if err := database.ValidateExportOptions(options); err != nil {
		return database.ExportResult{}, err
	}
	if err := database.CheckExportContext(ctx); err != nil {
		return database.ExportResult{
			Cancelled: true,
			Format:    options.Format,
		}, nil
	}

	fileConfig, err := exportFileConfiguration(options.Format)
	if err != nil {
		return database.ExportResult{}, err
	}
	defaultName := ensureExportExtension(
		sanitizeSuggestedFilename(suggestedName, fileConfig.defaultFilename),
		options.Format,
	)
	path, err := s.saveDialog(ctx, wailsruntime.SaveDialogOptions{
		Title:                fileConfig.title,
		DefaultFilename:      defaultName,
		Filters:              []wailsruntime.FileFilter{fileConfig.filter},
		CanCreateDirectories: true,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return database.ExportResult{
				Cancelled: true,
				Format:    options.Format,
			}, nil
		}
		return database.ExportResult{}, fmt.Errorf("choose export destination: %w", err)
	}
	if path == "" {
		return database.ExportResult{
			Cancelled: true,
			Format:    options.Format,
		}, nil
	}
	path = ensureExportExtension(path, options.Format)
	if err := database.CheckExportContext(ctx); err != nil {
		return database.ExportResult{
			Cancelled: true,
			Format:    options.Format,
		}, nil
	}
	job.status.Store(exportStatusRunning)

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

	progressWriter := &exportProgressWriter{writer: tempFile, job: job}
	stats, writeErr := write(ctx, progressWriter)
	if writeErr != nil {
		_ = tempFile.Close()
		if errors.Is(writeErr, context.Canceled) || ctx.Err() != nil {
			return database.ExportResult{
				Cancelled: true,
				Format:    options.Format,
			}, nil
		}
		return database.ExportResult{}, writeErr
	}
	if err := database.CheckExportContext(ctx); err != nil {
		_ = tempFile.Close()
		return database.ExportResult{
			Cancelled: true,
			Format:    options.Format,
		}, nil
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return database.ExportResult{}, fmt.Errorf("sync export file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return database.ExportResult{}, fmt.Errorf("close export file: %w", err)
	}
	if err := database.CheckExportContext(ctx); err != nil {
		return database.ExportResult{
			Cancelled: true,
			Format:    options.Format,
		}, nil
	}

	fileInfo, err := os.Stat(tempPath)
	if err != nil {
		return database.ExportResult{}, fmt.Errorf("inspect export file: %w", err)
	}
	if err := database.CheckExportContext(ctx); err != nil {
		return database.ExportResult{
			Cancelled: true,
			Format:    options.Format,
		}, nil
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
	ctx, job, err := s.startExportJob(request.JobID, request.ExpectedRows)
	if err != nil {
		return serviceError[database.ExportResult](err.Error())
	}
	defer s.finishExportJob(job)

	result, err := s.writeExport(ctx, job, request.SuggestedName, request.Options, func(
		ctx context.Context,
		writer io.Writer,
	) (database.ExportStats, error) {
		driver, release, err := s.driverFor(connectionID)
		if err != nil {
			return database.ExportStats{}, err
		}
		defer release()

		return driver.ExportTable(ctx, request, writer)
	})
	if err != nil {
		return serviceError[database.ExportResult](err.Error())
	}
	return response.BaseResponse[database.ExportResult]{Data: result}
}

func (s *Service) ExportQueryResults(
	request database.RowsExportRequest,
) response.BaseResponse[database.ExportResult] {
	if request.Options.Format == database.ExportFormatSQL {
		return serviceError[database.ExportResult](
			"SQL INSERT export requires a table source",
		)
	}

	expectedRows := request.ExpectedRows
	if expectedRows <= 0 {
		expectedRows = int64(len(request.Rows))
	}
	ctx, job, err := s.startExportJob(request.JobID, expectedRows)
	if err != nil {
		return serviceError[database.ExportResult](err.Error())
	}
	defer s.finishExportJob(job)

	result, err := s.writeExport(ctx, job, request.SuggestedName, request.Options, func(
		ctx context.Context,
		writer io.Writer,
	) (database.ExportStats, error) {
		return database.WriteExportRowsContext(
			ctx,
			writer,
			request.Columns,
			request.Rows,
			request.Options,
		)
	})
	if err != nil {
		return serviceError[database.ExportResult](err.Error())
	}
	return response.BaseResponse[database.ExportResult]{Data: result}
}
