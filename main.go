package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strings"

	"rollingthunder/internal/db"
	"rollingthunder/internal/diagnostics"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/build
var assets embed.FS

//go:embed wails.json
var embeddedWailsConfig []byte

type wailsMetadata struct {
	Info struct {
		ProductVersion string `json:"productVersion"`
	} `json:"info"`
}

func productVersionFromWailsConfig(contents []byte) (string, error) {
	var metadata wailsMetadata
	if err := json.Unmarshal(contents, &metadata); err != nil {
		return "", fmt.Errorf("parse wails.json: %w", err)
	}
	version := strings.TrimSpace(metadata.Info.ProductVersion)
	if version == "" {
		return "", fmt.Errorf("wails.json productVersion is empty")
	}
	return version, nil
}

func main() {
	// Create an instance of the app structure
	diagnosticManager := diagnostics.NewManager()
	appVersion, versionErr := productVersionFromWailsConfig(embeddedWailsConfig)
	if versionErr != nil {
		appVersion = "0.0.1"
	}
	db := db.NewServiceWithDiagnosticsAndVersion(
		diagnosticManager,
		appVersion,
	)
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = diagnosticManager.RecordCrash(
				fmt.Sprintf("Unhandled application panic (%T)", recovered),
				string(debug.Stack()),
			)
			panic(recovered)
		}
	}()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "⚡ Rolling Thunder",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup: func(ctx context.Context) {
			db.Start(ctx)
		},
		OnShutdown: func(ctx context.Context) {
			db.Shutdown(ctx)
		},
		Bind: []interface{}{
			db,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
