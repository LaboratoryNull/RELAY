package main

import (
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Wails 2.15 formats the GTK background colour through the process locale.
	// On systems without en_US.UTF-8 it may produce an invalid value such as
	// rgba(12, 14, 18, 1,0). Keep numeric formatting locale-independent while
	// preserving the user's language and other regional settings.
	// Create an instance of the app structure
	app := NewApp()
	app.restoreLocale = makeGTKColourLocaleSafe()

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "Relay — SSH Manager",
		Width:     1280,
		Height:    800,
		MinWidth:  920,
		MinHeight: 620,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 12, G: 14, B: 18, A: 255},
		// Keep WebKit's GTK drop target enabled: Wails receives Linux file paths
		// through that target and forwards them as the wails:file-drop event.
		DragAndDrop: &options.DragAndDrop{EnableFileDrop: true},
		OnStartup:   app.startup,
		OnShutdown:  app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

// makeGTKColourLocaleSafe works around a Wails 2.15 Linux bug: its GTK CSS
// formatter assumes that en_US.UTF-8 is installed. LC_ALL has precedence over
// LC_NUMERIC, so temporarily remove it while GTK initialises, then restore the
// exact user environment from App.startup.
func makeGTKColourLocaleSafe() func() {
	lcAll, hadLCAll := os.LookupEnv("LC_ALL")
	lcNumeric, hadLCNumeric := os.LookupEnv("LC_NUMERIC")
	_ = os.Unsetenv("LC_ALL")
	_ = os.Setenv("LC_NUMERIC", "C")

	return func() {
		if hadLCAll {
			_ = os.Setenv("LC_ALL", lcAll)
		} else {
			_ = os.Unsetenv("LC_ALL")
		}
		if hadLCNumeric {
			_ = os.Setenv("LC_NUMERIC", lcNumeric)
		} else {
			_ = os.Unsetenv("LC_NUMERIC")
		}
	}
}
