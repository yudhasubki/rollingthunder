package database

import "context"

type BackupFormat string

const (
	BackupFormatSQLiteNative   BackupFormat = "sqlite"
	BackupFormatPostgresCustom BackupFormat = "postgres_custom"
	BackupFormatMySQLSQL       BackupFormat = "mysql_sql"
)

type BackupCapabilities struct {
	Available     bool         `json:"available"`
	Engine        string       `json:"engine"`
	Format        BackupFormat `json:"format"`
	Extension     string       `json:"extension"`
	BackupTool    string       `json:"backupTool"`
	RestoreTool   string       `json:"restoreTool"`
	RestoreReady  bool         `json:"restoreReady"`
	BuiltIn       bool         `json:"builtIn"`
	Message       string       `json:"message,omitempty"`
	SupportsScope bool         `json:"supportsScope"`
}

type BackupRequest struct {
	ConnectionID string `json:"connectionId"`
	JobID        string `json:"jobId"`
	Schema       string `json:"schema,omitempty"`
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
}

type RestorePreview struct {
	ConnectionID  string       `json:"connectionId"`
	Database      string       `json:"database"`
	Engine        string       `json:"engine"`
	File          string       `json:"file"`
	Size          int64        `json:"size"`
	Format        BackupFormat `json:"format"`
	Schema        string       `json:"schema,omitempty"`
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

type backupError string

func (err backupError) Error() string { return string(err) }

const (
	ErrBackupConnectionRequired backupError = "backup connection is required"
	ErrBackupScopeConflict      backupError = "schema-only and data-only cannot both be enabled"
)
