// Command desktop is Kiln in a native window: the web build's page, in
// a Wails webview, over the TUI's core (internal/native) on this
// machine, with the TUI's config and data.
package main

import (
	"embed"
	"io/fs"
	"log"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/str"
)

// The page's bindings to Kiln's methods, committed under web/ui. After
// changing a method, regenerate them with the pinned wails3
// (bindings_test.go fails until then).
//
//go:generate wails3 generate bindings -ts -silent -d ../web/ui/src/bindings

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
		Name:       str.WebAppName(),
		Services:   []application.Service{application.NewService(k)},
		Assets:     application.AssetOptions{Handler: application.AssetFileServerFS(page)},
		Mac:        application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		ShouldQuit: k.quitGate,
	})
	if runtime.GOOS == "darwin" { //str:ok
		a.Menu.Set(macMenu())
	}
	w := wailsWindow{a: a, win: a.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: str.WebAppName(),
		URL:   "/",
	})}
	w.win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if !k.quitGate() {
			e.Cancel()
		}
	})
	k.win, k.dialogs = w, w
	return a.Run()
}

// macMenu is the system's App, Edit and Window menus, and no Kiln ones.
// Without an Edit menu, Cmd+C, V, X and A do nothing in the webview.
func macMenu() *application.Menu {
	m := application.NewMenu()
	m.AddRole(application.AppMenu)
	m.AddRole(application.EditMenu)
	m.AddRole(application.WindowMenu)
	return m
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
	path, err := d.PromptForSingleSelection()
	if runtime.GOOS == "windows" && windowsCancel(err) { //str:ok
		return "", nil
	}
	return path, err
}

// ConfirmQuit asks on a sheet (macOS) or message box over the window,
// waiting for the answer, so it's never called on the main thread: the
// dialog is shown there, and on macOS and Linux answers later. Quit is
// the default (Return), Cancel the cancel (Esc).
func (w wailsWindow) ConfirmQuit(n int) bool {
	quit, cancel := str.DesktopQuit(), str.DesktopCancel()
	if runtime.GOOS == "windows" { //str:ok
		// Wails' Windows question is a Yes/No message box, whatever the
		// labels, and it finds a button's OnClick by matching its label
		// to the one pressed (dialogs_windows.go). Windows draws its own
		// Yes and No.
		quit, cancel = "Yes", "No" //str:ok: Wails' Windows button names
	}
	answer := make(chan bool, 1)
	d := w.a.Dialog.Question().
		SetTitle(str.DesktopQuitTitle()).
		SetMessage(str.DesktopQuitMessage(n)).
		AttachToWindow(w.win)
	// Cancel first: Wails adds macOS's buttons last first, which puts
	// the last, Quit, on the right.
	d.AddButton(cancel).SetAsCancel().OnClick(func() { answer <- false })
	d.AddButton(quit).SetAsDefault().OnClick(func() { answer <- true })
	d.Show()
	return <-answer
}

// windowsCancel is whether err is Wails' Windows Save dialog saying it
// was cancelled. There a cancel is an error, not "" as on macOS and
// Linux: dialogs_windows.go returns ShowAndGetResult's error, and its
// internal cfd package (go-common-file-dialog, cfd/errors.go) gives
// ErrorCancelled for one. Being internal, it can only be matched by its
// message.
func windowsCancel(err error) bool {
	return err != nil && err.Error() == "cancelled by user" //str:ok: Wails' cfd.ErrorCancelled
}
