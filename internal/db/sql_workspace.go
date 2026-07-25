package db

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"rollingthunder/pkg/response"

	"github.com/google/uuid"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	maxSQLFileBytes = 10 << 20
	defaultSQLName  = "query.sql"
)

var sqlFileFilters = []wailsruntime.FileFilter{
	{
		DisplayName: "SQL files (*.sql)",
		Pattern:     "*.sql",
	},
	{
		DisplayName: "Text files (*.txt)",
		Pattern:     "*.txt",
	},
}

type sqlFileGrant struct {
	path string
}

type SQLWorkspaceFile struct {
	Token      string    `json:"token"`
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Content    string    `json:"content"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

type SaveSQLFileRequest struct {
	Token         string `json:"token,omitempty"`
	Content       string `json:"content"`
	SaveAs        bool   `json:"saveAs"`
	SuggestedName string `json:"suggestedName,omitempty"`
}

func sqlWorkspaceError[T any](
	status int,
	title string,
	detail string,
	hint string,
) response.BaseResponse[T] {
	return serviceErrorWithCode[T](
		status,
		errorCodeSQLFileFailed,
		title,
		detail,
		hint,
	)
}

func validateSQLFile(path string) (string, os.FileInfo, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("selected path is not a regular file")
	}
	if info.Size() > maxSQLFileBytes {
		return "", nil, fmt.Errorf(
			"SQL file is %d bytes; the workspace limit is %d bytes",
			info.Size(),
			maxSQLFileBytes,
		)
	}
	return absolute, info, nil
}

func (s *Service) grantSQLFile(path string) string {
	token := uuid.NewString()
	s.sqlFileMu.Lock()
	s.sqlFiles[token] = sqlFileGrant{path: path}
	s.sqlFileMu.Unlock()
	return token
}

func (s *Service) sqlFilePath(token string) (string, bool) {
	s.sqlFileMu.RLock()
	grant, ok := s.sqlFiles[strings.TrimSpace(token)]
	s.sqlFileMu.RUnlock()
	return grant.path, ok
}

func (s *Service) OpenSQLFile() response.BaseResponse[SQLWorkspaceFile] {
	if s.ctx == nil {
		return sqlWorkspaceError[SQLWorkspaceFile](
			http.StatusServiceUnavailable,
			"Application is not ready",
			"The native file picker is unavailable before application startup.",
			"Wait for Rolling Thunder to finish starting and try again.",
		)
	}
	path, err := s.sqlOpenDialog(s.ctx, wailsruntime.OpenDialogOptions{
		Title:                "Open SQL file",
		Filters:              sqlFileFilters,
		CanCreateDirectories: false,
		ResolvesAliases:      true,
	})
	if err != nil {
		return sqlWorkspaceError[SQLWorkspaceFile](
			http.StatusInternalServerError,
			"Could not choose SQL file",
			err.Error(),
			"Check file permissions and try the native file picker again.",
		)
	}
	if strings.TrimSpace(path) == "" {
		return response.BaseResponse[SQLWorkspaceFile]{}
	}
	absolute, info, err := validateSQLFile(path)
	if err != nil {
		return sqlWorkspaceError[SQLWorkspaceFile](
			http.StatusBadRequest,
			"SQL file is unavailable",
			err.Error(),
			"Choose a readable SQL file smaller than 10 MiB.",
		)
	}
	content, err := os.ReadFile(absolute)
	if err != nil {
		return sqlWorkspaceError[SQLWorkspaceFile](
			http.StatusForbidden,
			"Could not read SQL file",
			err.Error(),
			"Check file permissions and choose the file again.",
		)
	}
	return response.BaseResponse[SQLWorkspaceFile]{
		Data: SQLWorkspaceFile{
			Token:      s.grantSQLFile(absolute),
			Name:       filepath.Base(absolute),
			Path:       absolute,
			Content:    string(content),
			ModifiedAt: info.ModTime(),
		},
	}
}

func suggestedSQLFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." {
		return defaultSQLName
	}
	if filepath.Ext(name) == "" {
		name += ".sql"
	}
	return name
}

func writeSQLFile(path string, content []byte) error {
	if len(content) > maxSQLFileBytes {
		return fmt.Errorf(
			"SQL document is %d bytes; the workspace limit is %d bytes",
			len(content),
			maxSQLFileBytes,
		)
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".rolling-thunder-sql-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	mode := os.FileMode(0o600)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replaceExportFile(temporaryPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *Service) SaveSQLFile(
	request SaveSQLFileRequest,
) response.BaseResponse[SQLWorkspaceFile] {
	if s.ctx == nil {
		return sqlWorkspaceError[SQLWorkspaceFile](
			http.StatusServiceUnavailable,
			"Application is not ready",
			"The native file picker is unavailable before application startup.",
			"Wait for Rolling Thunder to finish starting and try again.",
		)
	}
	if len([]byte(request.Content)) > maxSQLFileBytes {
		return sqlWorkspaceError[SQLWorkspaceFile](
			http.StatusRequestEntityTooLarge,
			"SQL document is too large",
			"SQL workspace files are limited to 10 MiB.",
			"Split the script into smaller files before saving.",
		)
	}

	token := strings.TrimSpace(request.Token)
	path, granted := s.sqlFilePath(token)
	if request.SaveAs || !granted {
		selected, err := s.saveDialog(s.ctx, wailsruntime.SaveDialogOptions{
			Title:                "Save SQL file",
			DefaultFilename:      suggestedSQLFilename(request.SuggestedName),
			Filters:              sqlFileFilters,
			CanCreateDirectories: true,
		})
		if err != nil {
			return sqlWorkspaceError[SQLWorkspaceFile](
				http.StatusInternalServerError,
				"Could not choose save location",
				err.Error(),
				"Check folder permissions and try the native file picker again.",
			)
		}
		if strings.TrimSpace(selected) == "" {
			return response.BaseResponse[SQLWorkspaceFile]{}
		}
		absolute, err := filepath.Abs(selected)
		if err != nil {
			return serviceError[SQLWorkspaceFile](err.Error())
		}
		path = absolute
		token = s.grantSQLFile(path)
	}

	if err := writeSQLFile(path, []byte(request.Content)); err != nil {
		return sqlWorkspaceError[SQLWorkspaceFile](
			http.StatusForbidden,
			"Could not save SQL file",
			err.Error(),
			"Check that the destination folder is writable and try again.",
		)
	}
	info, err := os.Stat(path)
	if err != nil {
		return serviceError[SQLWorkspaceFile](err.Error())
	}
	return response.BaseResponse[SQLWorkspaceFile]{
		Data: SQLWorkspaceFile{
			Token:      token,
			Name:       filepath.Base(path),
			Path:       path,
			Content:    request.Content,
			ModifiedAt: info.ModTime(),
		},
	}
}

func (s *Service) CloseSQLFile(token string) response.BaseResponse[bool] {
	token = strings.TrimSpace(token)
	if token == "" {
		return response.BaseResponse[bool]{Data: true}
	}
	s.sqlFileMu.Lock()
	delete(s.sqlFiles, token)
	s.sqlFileMu.Unlock()
	return response.BaseResponse[bool]{Data: true}
}
