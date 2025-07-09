package main

import (
	"context"
	"embed"
	"rollingthunder/internal/db"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/build
var assets embed.FS

func main() {
	// Create an instance of the app structure
	db := db.NewService()

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
		Bind: []interface{}{
			db,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
