package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestChooseOracleTNSFileReturnsReviewedAliases(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "tnsnames.ora")
	if err := os.WriteFile(path, []byte(`
		APP = (DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=db.internal)(PORT=1521))
			(CONNECT_DATA=(SERVICE_NAME=app.internal)))
		REPORTING = (DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=report.internal)(PORT=1521))
			(CONNECT_DATA=(SERVICE_NAME=report.internal)))
	`), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService()
	service.ctx = context.Background()
	service.oracleTNSOpenDialog = func(
		context.Context,
		wailsruntime.OpenDialogOptions,
	) (string, error) {
		return path, nil
	}

	selected := service.ChooseOracleTNSFile()
	if len(selected.Errors) > 0 {
		t.Fatalf("ChooseOracleTNSFile() = %+v", selected)
	}
	if selected.Data.Path != path ||
		strings.Join(selected.Data.Aliases, ",") != "APP,REPORTING" {
		t.Fatalf("selection = %+v", selected.Data)
	}
}

func TestChooseOracleTNSFileRejectsInvalidConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tnsnames.ora")
	if err := os.WriteFile(path, []byte("APP = invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService()
	service.ctx = context.Background()
	service.oracleTNSOpenDialog = func(
		context.Context,
		wailsruntime.OpenDialogOptions,
	) (string, error) {
		return path, nil
	}

	selected := service.ChooseOracleTNSFile()
	if len(selected.Errors) == 0 {
		t.Fatalf("invalid tnsnames.ora was accepted: %+v", selected)
	}
}

func TestChooseOracleWalletDirectoryReportsPasswordRequirements(t *testing.T) {
	for _, test := range []struct {
		name             string
		file             string
		hasAutoLogin     bool
		passwordRequired bool
	}{
		{
			name:         "auto login",
			file:         "cwallet.sso",
			hasAutoLogin: true,
		},
		{
			name:             "password protected",
			file:             "ewallet.p12",
			passwordRequired: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := t.TempDir()
			if err := os.WriteFile(
				filepath.Join(path, test.file),
				[]byte("wallet"),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
			service := NewService()
			service.ctx = context.Background()
			service.oracleWalletDialog = func(
				context.Context,
				wailsruntime.OpenDialogOptions,
			) (string, error) {
				return path, nil
			}

			selected := service.ChooseOracleWalletDirectory()
			if len(selected.Errors) > 0 {
				t.Fatalf(
					"ChooseOracleWalletDirectory() = %+v",
					selected,
				)
			}
			if selected.Data.Path != path ||
				selected.Data.HasAutoLogin != test.hasAutoLogin ||
				selected.Data.PasswordRequired != test.passwordRequired {
				t.Fatalf("selection = %+v", selected.Data)
			}
		})
	}
}
