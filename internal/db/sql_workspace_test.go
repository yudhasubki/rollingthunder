package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestOpenAndSaveSQLFileUsesBackendGrant(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "report.sql")
	if err := os.WriteFile(path, []byte("SELECT 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	service := NewService()
	service.Start(context.Background())
	service.sqlOpenDialog = func(
		context.Context,
		wailsruntime.OpenDialogOptions,
	) (string, error) {
		return path, nil
	}

	opened := service.OpenSQLFile()
	if len(opened.Errors) > 0 {
		t.Fatalf("OpenSQLFile() errors = %+v", opened.Errors)
	}
	if opened.Data.Content != "SELECT 1;\n" || opened.Data.Token == "" {
		t.Fatalf("OpenSQLFile() = %+v", opened.Data)
	}

	saved := service.SaveSQLFile(SaveSQLFileRequest{
		Token:   opened.Data.Token,
		Content: "SELECT 2;\n",
	})
	if len(saved.Errors) > 0 {
		t.Fatalf("SaveSQLFile() errors = %+v", saved.Errors)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "SELECT 2;\n" {
		t.Fatalf("saved content = %q", content)
	}
}

func TestSaveSQLFileWithUnknownTokenRequiresPicker(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "safe.sql")
	service := NewService()
	service.Start(context.Background())
	service.saveDialog = func(
		context.Context,
		wailsruntime.SaveDialogOptions,
	) (string, error) {
		return path, nil
	}

	saved := service.SaveSQLFile(SaveSQLFileRequest{
		Token:         "frontend-crafted-path",
		Content:       "SELECT current_timestamp;\n",
		SuggestedName: "safe",
	})
	if len(saved.Errors) > 0 {
		t.Fatalf("SaveSQLFile() errors = %+v", saved.Errors)
	}
	if saved.Data.Path != path || saved.Data.Token == "" {
		t.Fatalf("SaveSQLFile() = %+v", saved.Data)
	}
}

func TestSQLWorkspaceRejectsOversizedDocument(t *testing.T) {
	service := NewService()
	service.Start(context.Background())
	result := service.SaveSQLFile(SaveSQLFileRequest{
		Content: string(make([]byte, maxSQLFileBytes+1)),
	})
	if len(result.Errors) != 1 || result.Errors[0].Code != errorCodeSQLFileFailed {
		t.Fatalf("SaveSQLFile() = %+v", result)
	}
}
