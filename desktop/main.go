// Command desktop is Kiln in a native window: the web build's page, in
// a Wails webview, over the TUI's core (internal/native) on this
// machine, with the TUI's config and data.
package main

import (
	"context"
	"log"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/str"
)

// The page's bindings to Kiln's methods, committed under web/ui. After
// changing a method, regenerate them with the pinned wails3
// (bindings_test.go fails until then).
//
//go:generate wails3 generate bindings -ts -silent -d ../web/ui/src/bindings

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
	page, err := pageFS()
	if err != nil {
		return err
	}
	notes := &wailsNotes{ns: notifications.New()}
	k := &Kiln{notes: notes, cfgDir: cfgDir, dataDir: dataDir}
	notes.ns.OnNotificationResponse(func(r notifications.NotificationResult) {
		// Off the thread it came on, which may be the main thread: a
		// click asks the loop and the window.
		if r.Error == nil && r.Response.ActionIdentifier == notifications.DefaultActionIdentifier {
			go k.clicked(r.Response.UserInfo)
		}
	})
	a := application.New(application.Options{
		Name: str.WebAppName(),
		// Notifications first: services shut down last first, so Kiln's
		// loop ends before they do.
		Services:   []application.Service{application.NewService(notes), application.NewService(k)},
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
		// 88 columns of scrollback, 80 of log mode's beside its 8-column
		// gutter, at the page's 9.6px character: the gutters (12px), a
		// 22-column sidebar and its divider, with overlay scrollbars.
		// The height keeps Wails' 4:3 (800 by 600).
		Width:  1080,
		Height: 810,
		// Shown once it's put where it was left (placer).
		Hidden: true,
	})}
	w.win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if !k.quitGate() {
			e.Cancel()
		}
	})
	k.win, k.dialogs = w, w
	p := newPlacer(w.win, dataDir, func() []application.Rect {
		var areas []application.Rect
		for _, s := range a.Screen.GetAll() {
			areas = append(areas, s.WorkArea)
		}
		return areas
	})
	// Once the page is up, the screens are known. Should it never come
	// up, the window shows anyway, where it opens.
	w.win.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) { p.place() })
	a.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		time.AfterFunc(5*time.Second, p.place)
	})
	for _, e := range []events.WindowEventType{events.Common.WindowDidMove, events.Common.WindowDidResize, events.Common.WindowMaximise, events.Common.WindowUnMaximise} {
		w.win.OnWindowEvent(e, func(*application.WindowEvent) { p.moved() })
	}
	if err := watchPage(a, w.win); err != nil {
		return err
	}
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
func (w wailsWindow) Focus()                   { w.win.UnMinimise(); w.win.Focus() }

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

// wailsNotes is Wails' notifications service, started as one of the
// app's. If it can't start (macOS: the app isn't bundled; Linux: no
// session bus) the app runs without notifications, and calls nothing
// of the service's but its click hook: only its start checks that it
// can run.
type wailsNotes struct {
	ns *notifications.NotificationService
	ok atomic.Bool // started
}

func (n *wailsNotes) ServiceStartup(ctx context.Context, opts application.ServiceOptions) error {
	if n.ns.ServiceStartup(ctx, opts) == nil {
		n.ok.Store(true)
	}
	return nil
}

func (n *wailsNotes) ServiceShutdown() error {
	if n.ok.Load() {
		return n.ns.ServiceShutdown()
	}
	return nil
}

// Send waits up to 5s for macOS to take the notification.
//
//wails:ignore
func (n *wailsNotes) Send(id, title, body string, data map[string]any) error {
	if !n.ok.Load() {
		return errNoNotifications
	}
	return n.ns.SendNotification(notifications.NotificationOptions{ID: id, Title: title, Body: body, Data: data})
}

// Allowed is always true on Windows and Linux.
//
//wails:ignore
func (n *wailsNotes) Allowed() (bool, error) {
	if !n.ok.Load() {
		return false, errNoNotifications
	}
	return n.ns.CheckNotificationAuthorization()
}

// Ask waits for macOS's question to be answered, up to 3 minutes.
//
//wails:ignore
func (n *wailsNotes) Ask() (bool, error) {
	if !n.ok.Load() {
		return false, errNoNotifications
	}
	return n.ns.RequestNotificationAuthorization()
}
