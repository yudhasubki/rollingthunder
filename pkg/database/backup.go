package database

import (
	"context"
	"io"
	"strings"
)

type BackupFormat string

const (
	BackupFormatSQLiteNative    BackupFormat = "sqlite"
	BackupFormatPostgresCustom  BackupFormat = "postgres_custom"
	BackupFormatMySQLSQL        BackupFormat = "mysql_sql"
	BackupFormatOracleDataPump  BackupFormat = "oracle_datapump"
	BackupFormatSQLServerNative BackupFormat = "sqlserver_native"
)

type BackupDirectory struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

type BackupCapabilities struct {
	Available         bool              `json:"available"`
	Engine            string            `json:"engine"`
	Format            BackupFormat      `json:"format"`
	Extension         string            `json:"extension"`
	BackupTool        string            `json:"backupTool"`
	RestoreTool       string            `json:"restoreTool"`
	RestoreReady      bool              `json:"restoreReady"`
	BuiltIn           bool              `json:"builtIn"`
	Message           string            `json:"message,omitempty"`
	SupportsScope     bool              `json:"supportsScope"`
	RequiresDirectory bool              `json:"requiresDirectory"`
	ServerSideFiles   bool              `json:"serverSideFiles"`
	Directories       []BackupDirectory `json:"directories"`
}

type BackupRequest struct {
	ConnectionID string `json:"connectionId"`
	JobID        string `json:"jobId"`
	Schema       string `json:"schema,omitempty"`
	Directory    string `json:"directory,omitempty"`
	ServerPath   string `json:"serverPath,omitempty"`
	SchemaOnly   bool   `json:"schemaOnly"`
	DataOnly     bool   `json:"dataOnly"`
}

func (request BackupRequest) Validate() error {
	if request.ConnectionID == "" {
		return ErrBackupConnectionRequired
	}
	if request.SchemaOnly && request.DataOnly {
		return ErrBackupScopeConflict
	}
	if strings.ContainsAny(request.Directory, "\x00\r\n") {
		return ErrBackupDirectoryInvalid
	}
	if strings.ContainsAny(request.ServerPath, "\x00\r\n") {
		return ErrBackupServerPathInvalid
	}
	return nil
}

type BackupResult struct {
	Path      string       `json:"path"`
	Bytes     int64        `json:"bytes"`
	Format    BackupFormat `json:"format"`
	Cancelled bool         `json:"cancelled"`
}

type RestoreFileSelection struct {
	Token  string       `json:"token"`
	Name   string       `json:"name"`
	Size   int64        `json:"size"`
	Format BackupFormat `json:"format"`
}

type RestorePreviewRequest struct {
	ConnectionID string `json:"connectionId"`
	Token        string `json:"token"`
	Schema       string `json:"schema,omitempty"`
	Directory    string `json:"directory,omitempty"`
	ServerPath   string `json:"serverPath,omitempty"`
}

type RestorePreview struct {
	ConnectionID  string       `json:"connectionId"`
	Database      string       `json:"database"`
	Engine        string       `json:"engine"`
	File          string       `json:"file"`
	Size          int64        `json:"size"`
	Format        BackupFormat `json:"format"`
	Schema        string       `json:"schema,omitempty"`
	Directory     string       `json:"directory,omitempty"`
	Destructive   bool         `json:"destructive"`
	Transactional bool         `json:"transactional"`
	Warnings      []string     `json:"warnings"`
	Fingerprint   string       `json:"fingerprint"`
}

type ApplyRestoreRequest struct {
	Restore     RestorePreviewRequest `json:"restore"`
	Fingerprint string                `json:"fingerprint"`
	JobID       string                `json:"jobId"`
}

type RestoreResult struct {
	Restored    bool   `json:"restored"`
	Fingerprint string `json:"fingerprint"`
	Cancelled   bool   `json:"cancelled"`
}

type MaintenanceProgress struct {
	JobID       string `json:"jobId"`
	Kind        string `json:"kind"`
	Status      string `json:"status"`
	ElapsedMS   int64  `json:"elapsedMs"`
	Cancellable bool   `json:"cancellable"`
}

type NativeBackupDriver interface {
	BackupDatabase(ctx context.Context, path string) error
	RestoreDatabase(ctx context.Context, path string) error
}

// StreamingBackupDriver is implemented by engines whose native backup files
// are staged by the database server. The service owns the local file handles,
// while the driver owns server-side job lifecycle and staging cleanup.
type StreamingBackupDriver interface {
	GetBackupDirectories(ctx context.Context) ([]BackupDirectory, error)
	BackupDatabaseToWriter(
		ctx context.Context,
		writer io.Writer,
		request BackupRequest,
	) error
	RestoreDatabaseFromReader(
		ctx context.Context,
		reader io.Reader,
		request RestorePreviewRequest,
	) error
}

// ServerBackupMetadata identifies a native backup that remains on the
// database server. Identity must change whenever the selected backup set
// changes, allowing the service to reject stale restore confirmations without
// treating a remote server path as a local application file.
type ServerBackupMetadata struct {
	Path       string `json:"path"`
	Database   string `json:"database"`
	Bytes      int64  `json:"bytes"`
	Position   int    `json:"position"`
	FinishedAt string `json:"finishedAt,omitempty"`
	Identity   string `json:"identity"`
}

// ServerSideBackupDriver is implemented by engines whose native backup and
// restore commands operate on paths visible to the database server, not paths
// on the Rolling Thunder desktop host.
type ServerSideBackupDriver interface {
	GetBackupDirectories(ctx context.Context) ([]BackupDirectory, error)
	BackupDatabaseToServer(
		ctx context.Context,
		request BackupRequest,
	) (ServerBackupMetadata, error)
	InspectServerBackup(
		ctx context.Context,
		path string,
	) (ServerBackupMetadata, error)
	RestoreDatabaseFromServer(
		ctx context.Context,
		path string,
	) error
}

type backupError string

func (err backupError) Error() string { return string(err) }

const (
	ErrBackupConnectionRequired backupError = "backup connection is required"
	ErrBackupScopeConflict      backupError = "schema-only and data-only cannot both be enabled"
	ErrBackupDirectoryInvalid   backupError = "backup directory contains invalid characters"
	ErrBackupServerPathInvalid  backupError = "server backup path contains invalid characters"
)
