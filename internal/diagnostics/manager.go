package diagnostics

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	settingsVersion = 1
	maxReportFiles  = 10
)

type Settings struct {
	Enabled           bool `json:"enabled"`
	IncludeSystemInfo bool `json:"includeSystemInfo"`
}

type FrontendReport struct {
	Message string `json:"message"`
	Stack   string `json:"stack,omitempty"`
	Source  string `json:"source,omitempty"`
}

type ExportResult struct {
	Path  string `json:"path"`
	Files int    `json:"files"`
}

type settingsEnvelope struct {
	Version  int      `json:"version"`
	Settings Settings `json:"settings"`
}

type reportEnvelope struct {
	Version    int               `json:"version"`
	Kind       string            `json:"kind"`
	OccurredAt string            `json:"occurredAt"`
	Message    string            `json:"message"`
	Stack      string            `json:"stack,omitempty"`
	System     map[string]string `json:"system,omitempty"`
}

type Manager struct {
	settingsPath string
	reportsDir   string
	mu           sync.Mutex
}

func NewManager() *Manager {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		configDirectory = "."
	}
	cacheDirectory, err := os.UserCacheDir()
	if err != nil {
		cacheDirectory = configDirectory
	}
	return NewManagerAt(
		filepath.Join(configDirectory, "RollingThunder", "diagnostics.json"),
		filepath.Join(cacheDirectory, "RollingThunder", "diagnostics"),
	)
}

func NewManagerAt(settingsPath string, reportsDir string) *Manager {
	return &Manager{
		settingsPath: settingsPath,
		reportsDir:   reportsDir,
	}
}

func (manager *Manager) Settings() (Settings, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.loadSettings()
}

func (manager *Manager) loadSettings() (Settings, error) {
	data, err := os.ReadFile(manager.settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Settings{}, nil
		}
		return Settings{}, fmt.Errorf("read diagnostics settings: %w", err)
	}
	var envelope settingsEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Settings{}, fmt.Errorf("decode diagnostics settings: %w", err)
	}
	if envelope.Version != settingsVersion {
		return Settings{}, fmt.Errorf(
			"unsupported diagnostics settings version %d",
			envelope.Version,
		)
	}
	return envelope.Settings, nil
}

func atomicWrite(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(directory, ".diagnostics-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func (manager *Manager) UpdateSettings(settings Settings) (Settings, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	data, err := json.MarshalIndent(settingsEnvelope{
		Version:  settingsVersion,
		Settings: settings,
	}, "", "  ")
	if err != nil {
		return Settings{}, err
	}
	if err := atomicWrite(manager.settingsPath, data); err != nil {
		return Settings{}, fmt.Errorf("save diagnostics settings: %w", err)
	}
	return settings, nil
}

var (
	connectionURLPattern = regexp.MustCompile(`(?i)\b(postgres(?:ql)?|mysql|mariadb)://[^@\s]+@`)
	credentialPattern    = regexp.MustCompile(`(?i)(["']?(?:password|passwd|pwd|token|secret)["']?\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;}]+)`)
	bearerPattern        = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	privateKeyPattern    = regexp.MustCompile(`(?s)-----BEGIN [^-]*PRIVATE KEY-----.*?-----END [^-]*PRIVATE KEY-----`)
	quotedValuePattern   = regexp.MustCompile(`'([^']|'')*'`)
)

func Redact(value string) string {
	redacted := connectionURLPattern.ReplaceAllString(value, "$1://[credentials]@")
	redacted = credentialPattern.ReplaceAllString(redacted, "$1[redacted]")
	redacted = bearerPattern.ReplaceAllString(redacted, "Bearer [redacted]")
	redacted = privateKeyPattern.ReplaceAllString(redacted, "[private key redacted]")
	redacted = quotedValuePattern.ReplaceAllString(redacted, "'[value]'")
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		redacted = strings.ReplaceAll(redacted, home, "$HOME")
	}
	return redacted
}

func (manager *Manager) RecordFrontend(report FrontendReport) (bool, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	settings, err := manager.loadSettings()
	if err != nil {
		return false, err
	}
	if !settings.Enabled {
		return false, nil
	}
	source := strings.TrimSpace(report.Source)
	if source == "" {
		source = "frontend"
	}
	return true, manager.writeReport(
		"frontend",
		source+": "+report.Message,
		report.Stack,
		settings,
	)
}

// RecordCrash writes a small local-only, redacted crash report even when
// diagnostics are disabled. It is never uploaded and excludes application
// state, queries, rows, credentials, and connection profiles.
func (manager *Manager) RecordCrash(message string, stack string) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	settings, _ := manager.loadSettings()
	return manager.writeReport("crash", message, stack, settings)
}

func (manager *Manager) writeReport(
	kind string,
	message string,
	stack string,
	settings Settings,
) error {
	if err := os.MkdirAll(manager.reportsDir, 0o700); err != nil {
		return fmt.Errorf("create diagnostics directory: %w", err)
	}
	report := reportEnvelope{
		Version:    1,
		Kind:       kind,
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		Message:    Redact(message),
		Stack:      Redact(stack),
	}
	if settings.Enabled && settings.IncludeSystemInfo {
		report.System = map[string]string{
			"os":   runtime.GOOS,
			"arch": runtime.GOARCH,
			"go":   runtime.Version(),
			"cpus": fmt.Sprintf("%d", runtime.NumCPU()),
		}
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	name := fmt.Sprintf(
		"%s-%s.json",
		time.Now().UTC().Format("20060102T150405.000000000Z"),
		kind,
	)
	if err := atomicWrite(filepath.Join(manager.reportsDir, name), data); err != nil {
		return fmt.Errorf("write diagnostics report: %w", err)
	}
	return manager.rotate()
}

func (manager *Manager) reportPaths() ([]string, error) {
	entries, err := os.ReadDir(manager.reportsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".json") {
			paths = append(paths, filepath.Join(manager.reportsDir, entry.Name()))
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func (manager *Manager) rotate() error {
	paths, err := manager.reportPaths()
	if err != nil {
		return err
	}
	for len(paths) > maxReportFiles {
		if err := os.Remove(paths[0]); err != nil && !os.IsNotExist(err) {
			return err
		}
		paths = paths[1:]
	}
	return nil
}

func (manager *Manager) Clear() error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	paths, err := manager.reportPaths()
	if err != nil {
		return err
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func replaceArchiveFile(tempPath string, targetPath string) error {
	if err := os.Rename(tempPath, targetPath); err == nil {
		return nil
	} else if targetInfo, statErr := os.Stat(targetPath); statErr != nil {
		return err
	} else if !targetInfo.Mode().IsRegular() {
		return fmt.Errorf("diagnostics destination is not a regular file")
	}

	backup, err := os.CreateTemp(filepath.Dir(targetPath), ".diagnostics-backup-*.tmp")
	if err != nil {
		return err
	}
	backupPath := backup.Name()
	if err := backup.Close(); err != nil {
		_ = os.Remove(backupPath)
		return err
	}
	if err := os.Remove(backupPath); err != nil {
		return err
	}
	if err := os.Rename(targetPath, backupPath); err != nil {
		return err
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		if restoreErr := os.Rename(backupPath, targetPath); restoreErr != nil {
			return fmt.Errorf(
				"replace diagnostics archive: %w; restore previous archive: %v",
				err,
				restoreErr,
			)
		}
		return fmt.Errorf("replace diagnostics archive: %w", err)
	}
	_ = os.Remove(backupPath)
	return nil
}

func (manager *Manager) Export(path string) (ExportResult, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if filepath.Ext(path) == "" {
		path += ".zip"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return ExportResult{}, err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".diagnostics-export-*.tmp")
	if err != nil {
		return ExportResult{}, err
	}
	tempPath := file.Name()
	defer func() {
		_ = file.Close()
		_ = os.Remove(tempPath)
	}()
	if err := file.Chmod(0o600); err != nil {
		return ExportResult{}, err
	}
	archive := zip.NewWriter(file)
	manifest, err := archive.Create("README.txt")
	if err != nil {
		return ExportResult{}, err
	}
	if _, err := io.WriteString(
		manifest,
		"Rolling Thunder diagnostics\n\nReports are local, redacted, and never uploaded automatically. Review every file before sharing.\n",
	); err != nil {
		return ExportResult{}, err
	}
	paths, err := manager.reportPaths()
	if err != nil {
		return ExportResult{}, err
	}
	files := 0
	for _, reportPath := range paths {
		source, openErr := os.Open(reportPath)
		if openErr != nil {
			return ExportResult{}, openErr
		}
		destination, createErr := archive.Create(filepath.Join("reports", filepath.Base(reportPath)))
		if createErr == nil {
			_, createErr = io.Copy(destination, source)
		}
		_ = source.Close()
		if createErr != nil {
			return ExportResult{}, createErr
		}
		files++
	}
	if err := archive.Close(); err != nil {
		return ExportResult{}, err
	}
	if err := file.Sync(); err != nil {
		return ExportResult{}, err
	}
	if err := file.Close(); err != nil {
		return ExportResult{}, err
	}
	if err := replaceArchiveFile(tempPath, path); err != nil {
		return ExportResult{}, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return ExportResult{}, err
	}
	return ExportResult{Path: path, Files: files}, nil
}
