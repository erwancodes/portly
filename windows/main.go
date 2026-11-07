//go:build windows

package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// The production build is created by `wails build` after the Vite frontend has
// produced frontend/dist. The placeholder in that directory keeps `go test`
// and editor tooling useful before the first frontend build.
//
//go:embed all:frontend/dist
var embeddedAssets embed.FS

func main() {
	assets, err := fs.Sub(embeddedAssets, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}

	app := NewApp()
	if err := wails.Run(&options.App{
		Title:            "Portly",
		Width:            1180,
		Height:           760,
		MinWidth:         940,
		MinHeight:        620,
		BackgroundColour: options.NewRGB(14, 14, 17),
		// The frontend draws its own title bar and window controls; the tray keeps Open and Quit.
		Frameless:         true,
		HideWindowOnClose: true,
		Windows: &windows.Options{
			Theme: windows.Dark,
		},
		AssetServer:   &assetserver.Options{Assets: assets},
		OnStartup:     app.startup,
		OnBeforeClose: app.beforeClose,
		OnShutdown:    app.shutdown,
		Bind:          []interface{}{app},
	}); err != nil {
		log.Fatal(err)
	}
}
