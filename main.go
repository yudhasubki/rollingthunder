package main

import (
	"context"
	"embed"
	"fmt"
	"runtime/debug"

	"rollingthunder/internal/db"
	"rollingthunder/internal/diagnostics"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/build
var assets embed.FS

func main() {
	// Create an instance of the app structure
	diagnosticManager := diagnostics.NewManager()
	db := db.NewServiceWithDiagnostics(diagnosticManager)
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
