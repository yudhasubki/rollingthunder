package db

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"rollingthunder/pkg/application"
	"rollingthunder/pkg/response"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type openFileDialogFunc func(
	context.Context,
	wailsruntime.OpenDialogOptions,
) (string, error)

var defaultOpenFileDialog openFileDialogFunc = wailsruntime.OpenFileDialog
var defaultOpenDirectoryDialog openFileDialogFunc = wailsruntime.OpenDirectoryDialog

var sqliteFileFilters = []wailsruntime.FileFilter{
	{
		DisplayName: "SQLite databases (*.sqlite, *.sqlite3, *.db)",
		Pattern:     "*.sqlite;*.sqlite3;*.db",
	},
	{
		DisplayName: "All files",
		Pattern:     "*",
	},
}

func normalizeSQLiteDialogPath(path string, create bool) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve selected SQLite path: %w", err)
	}
	if create && filepath.Ext(absolute) == "" {
		absolute += ".sqlite3"
	}
	return filepath.Clean(absolute), nil
}

// ChooseSQLiteDatabaseFile opens the native file chooser. For create=true it
// returns a destination path; SQLite creates the file when the profile connects.
func (s *Service) ChooseSQLiteDatabaseFile(
	create bool,
) response.BaseResponse[string] {
	if s.ctx == nil {
		return serviceErrorWithCode[string](
			500,
			errorCodeDatabaseOperationFailed,
			"Application is not ready",
			"The native file picker is unavailable before application startup.",
			"Wait for Rolling Thunder to finish starting and try again.",
		)
	}

	var (
		path string
		err  error
	)
	if create {
		path, err = s.sqliteSaveDialog(s.ctx, wailsruntime.SaveDialogOptions{
			Title:                "Create SQLite database",
			DefaultFilename:      application.Identifier + ".sqlite3",
			Filters:              sqliteFileFilters,
			CanCreateDirectories: true,
		})
	} else {
		path, err = s.sqliteOpenDialog(s.ctx, wailsruntime.OpenDialogOptions{
			Title:                "Open SQLite database",
			Filters:              sqliteFileFilters,
			CanCreateDirectories: false,
			ResolvesAliases:      true,
		})
	}
	if err != nil {
		return serviceErrorWithCode[string](
			500,
			errorCodeDatabaseOperationFailed,
			"Could not choose SQLite database",
			err.Error(),
			"Check file permissions and try the native file picker again.",
		)
	}
	path, err = normalizeSQLiteDialogPath(path, create)
	if err != nil {
		return serviceErrorWithCode[string](
			400,
			errorCodeInvalidRequest,
			"Invalid SQLite path",
			err.Error(),
			"Choose another database file path.",
		)
	}
	return response.BaseResponse[string]{Data: path}
}
