// Command desktop is Kiln in a native window: the web build's page, in
// a Wails webview, over the TUI's core (internal/native) on this
// machine, with the TUI's config and data.
package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/str"
)

// The page, built into frontend/dist. all: keeps the embed compiling
// while only .gitkeep is there.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if err := run(); err != nil {
		log.Fatal(str.CliError(err))
	}
}

func run() error {
	cfgDir, err := config.Dir()
	if err != nil {
		return err
	}
	dataDir, err := config.DataDir()
	if err != nil {
		return err
	}
	page, err := fs.Sub(assets, "frontend/dist") //str:ok
	if err != nil {
		return err
	}
	k := &Kiln{cfgDir: cfgDir, dataDir: dataDir}
	a := application.New(application.Options{
		Name:     str.WebAppName(),
		Services: []application.Service{application.NewService(k)},
		Assets:   application.AssetOptions{Handler: application.AssetFileServerFS(page)},
		Mac:      application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	w := wailsWindow{a: a, win: a.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: str.WebAppName(),
		URL:   "/",
	})}
	k.win, k.dialogs = w, w
	return a.Run()
}

// wailsWindow is the window and dialogs Kiln uses, through the Wails app.
type wailsWindow struct {
	a   *application.App
	win *application.WebviewWindow // the one window, the dialogs' parent
}

func (w wailsWindow) Emit(name, data string)   { w.a.Event.Emit(name, data) }
func (w wailsWindow) OpenURL(url string) error { return w.a.Browser.OpenURL(url) }
func (w wailsWindow) Quit()                    { w.a.Quit() }

func (w wailsWindow) SaveFile(name, dir string) (string, error) {
	d := w.a.Dialog.SaveFile().SetFilename(name).AttachToWindow(w.win)
	if dir != "" {
		d.SetDirectory(dir)
	}
	return d.PromptForSingleSelection()
}
