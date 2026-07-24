package diagnostics

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	root := t.TempDir()
	return NewManagerAt(
		filepath.Join(root, "config", "diagnostics.json"),
		filepath.Join(root, "cache", "reports"),
	)
}

func TestDiagnosticsAreOptInAndSettingsArePrivate(t *testing.T) {
	manager := newTestManager(t)
	settings, err := manager.Settings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Enabled {
		t.Fatal("diagnostics must default to disabled")
	}
	recorded, err := manager.RecordFrontend(FrontendReport{Message: "hidden"})
	if err != nil || recorded {
		t.Fatalf("RecordFrontend() = %v, %v", recorded, err)
	}
	paths, err := manager.reportPaths()
	if err != nil || len(paths) != 0 {
		t.Fatalf("reports while disabled = %v, %v", paths, err)
	}

	updated, err := manager.UpdateSettings(Settings{
		Enabled:           true,
		IncludeSystemInfo: true,
	})
	if err != nil || !updated.Enabled {
		t.Fatalf("UpdateSettings() = %+v, %v", updated, err)
	}
	info, err := os.Stat(manager.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("settings permissions = %o", info.Mode().Perm())
	}
	directoryInfo, err := os.Stat(filepath.Dir(manager.settingsPath))
	if err != nil {
		t.Fatal(err)
	}
	if directoryInfo.Mode().Perm() != 0o700 {
		t.Fatalf("settings directory permissions = %o", directoryInfo.Mode().Perm())
	}
}

func TestDiagnosticReportsRedactCredentialsPathsAndValues(t *testing.T) {
	manager := newTestManager(t)
	if _, err := manager.UpdateSettings(Settings{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	recorded, err := manager.RecordFrontend(FrontendReport{
		Message: `postgres://user:secret@db.internal/app password=hunter2 ` +
			`{"token":"api-token-value"} Authorization: Bearer bearer-token ` +
			`value 'customer-data'`,
		Stack: filepath.Join(home, "Project", "app.ts") + ":12",
	})
	if err != nil || !recorded {
		t.Fatalf("RecordFrontend() = %v, %v", recorded, err)
	}
	paths, err := manager.reportPaths()
	if err != nil || len(paths) != 1 {
		t.Fatalf("report paths = %v, %v", paths, err)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("report permissions = %o", info.Mode().Perm())
	}
	content := string(data)
	for _, secret := range []string{
		"user:secret",
		"hunter2",
		"api-token-value",
		"bearer-token",
		"customer-data",
		home,
	} {
		if secret != "" && strings.Contains(content, secret) {
			t.Fatalf("report contains sensitive value %q: %s", secret, content)
		}
	}
	if !strings.Contains(content, "[credentials]") ||
		!strings.Contains(content, "[redacted]") ||
		!strings.Contains(content, "$HOME") {
		t.Fatalf("redaction markers missing: %s", content)
	}
}

func TestDiagnosticRotationExportAndClear(t *testing.T) {
	manager := newTestManager(t)
	if _, err := manager.UpdateSettings(Settings{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < maxReportFiles+3; index++ {
		if _, err := manager.RecordFrontend(FrontendReport{
			Message: "test report",
		}); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := manager.reportPaths()
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != maxReportFiles {
		t.Fatalf("rotated reports = %d, want %d", len(paths), maxReportFiles)
	}

	exportPath := filepath.Join(t.TempDir(), "diagnostics.zip")
	if err := os.WriteFile(exportPath, []byte("previous archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	exported, err := manager.Export(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if exported.Files != maxReportFiles || exported.Path != exportPath {
		t.Fatalf("Export() = %+v", exported)
	}
	archive, err := zip.OpenReader(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.File) != maxReportFiles+1 {
		t.Fatalf("archive files = %d", len(archive.File))
	}
	_ = archive.Close()
	info, err := os.Stat(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("archive permissions = %o", info.Mode().Perm())
	}

	if err := manager.Clear(); err != nil {
		t.Fatal(err)
	}
	paths, err = manager.reportPaths()
	if err != nil || len(paths) != 0 {
		t.Fatalf("reports after clear = %v, %v", paths, err)
	}
}

func TestLocalCrashReportExistsWhileDiagnosticsDisabled(t *testing.T) {
	manager := newTestManager(t)
	if err := manager.RecordCrash("panic: boom", "stack"); err != nil {
		t.Fatal(err)
	}
	paths, err := manager.reportPaths()
	if err != nil || len(paths) != 1 {
		t.Fatalf("crash reports = %v, %v", paths, err)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"system"`) {
		t.Fatalf("disabled diagnostics included system info: %s", data)
	}
}

func TestReplaceArchiveFileRejectsDirectoryDestination(t *testing.T) {
	root := t.TempDir()
	tempPath := filepath.Join(root, "diagnostics.tmp")
	if err := os.WriteFile(tempPath, []byte("new archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(root, "diagnostics.zip")
	if err := os.Mkdir(targetPath, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := replaceArchiveFile(tempPath, targetPath); err == nil {
		t.Fatal("replaceArchiveFile() unexpectedly replaced a directory")
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("directory destination was modified")
	}
}
