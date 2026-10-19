package main

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/native"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/webapi"
)

// batchEvent is the Wails event each of the loop's batches goes to the
// page as, its data the batch's JSON as a string.
const batchEvent = "kiln:batch" //str:ok: an event name

// window is what Kiln needs of the Wails app, so tests fake it.
type window interface {
	Emit(name, data string)
	OpenURL(url string) error
	Quit()
}

// dialogs are the OS dialogs Kiln opens, so tests fake them.
type dialogs interface {
	// SaveFile asks where to save a file named name, starting in dir ("":
	// the OS's choice). "" is a cancel.
	SaveFile(name, dir string) (path string, err error)
	// ConfirmQuit asks whether to quit with n characters connected:
	// true for Quit, false for Cancel.
	ConfirmQuit(n int) bool
}

// Kiln is the service the page calls: Kiln's core, the TUI's (see
// internal/native), driven by the web API's loop. Its exported methods
// are the page's bindings.
type Kiln struct {
	win             window
	dialogs         dialogs
	cfgDir, dataDir string

	mu     sync.Mutex
	loop   *webapi.Loop  // nil until Start
	cancel func()        // stops the loop
	done   chan struct{} // closed once the loop has ended and the core is closed

	quitMu   sync.Mutex
	quitting bool // the quit is answered: every gate lets it through
	asking   bool // the quit question is up
}

// Start builds the core and runs the loop, once the page's listeners
// are set. A second call does nothing.
func (k *Kiln) Start() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.loop != nil {
		return nil
	}
	if err := config.EnsureDefaults(k.cfgDir); err != nil {
		return err
	}
	cfg, err := config.Load(k.cfgDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(k.dataDir, 0o700); err != nil {
		return err
	}
	// No Size: the page reports the pane's once it's laid out.
	core, err := native.New(native.Options{ConfigDir: k.cfgDir, DataDir: k.dataDir})
	if err != nil {
		return err
	}
	a := app.New(core.Deps)
	a.ApplyConfig(cfg)
	l := webapi.New(a, func(b []byte) { k.win.Emit(batchEvent, string(b)) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	k.loop, k.cancel, k.done = l, cancel, done
	go func() {
		l.Run(ctx, webapi.Options{
			Autoconnect: true, ConfigDir: k.cfgDir, Changes: core.Changes,
			Drafts: filepath.Join(k.dataDir, "drafts.json"), //str:ok
		})
		core.Close()
		close(done)
		if ctx.Err() == nil { // a /quit, not the app shutting down
			k.win.Quit()
		}
	}()
	return nil
}

// ServiceShutdown stops the loop as the app quits, hanging up every
// session, and waits for the core to close. Wails calls it.
func (k *Kiln) ServiceShutdown() error {
	k.mu.Lock()
	cancel, done := k.cancel, k.done
	k.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	return nil
}

// Act hands the loop one action from the page, as JSON (the wasm
// main's act). Before Start it does nothing.
func (k *Kiln) Act(action string) {
	if l := k.running(); l != nil {
		l.Act([]byte(action))
	}
}

// Check is what to say about a settings field as value is typed, ""
// when it's fine: webapi.CheckField.
func (k *Kiln) Check(field, value string) string {
	return webapi.CheckField(field, value)
}

// Over is where text runs past maxLineBytes: webapi.OverRanges, empty
// rather than nil so the page gets a list.
func (k *Kiln) Over(text string, maxLineBytes int, newlineMode string) [][2]int {
	if r := webapi.OverRanges(text, maxLineBytes, newlineMode); r != nil {
		return r
	}
	return [][2]int{}
}

// SaveAs asks where to save data with the OS's Save dialog, offering
// name and starting in export_dir when the config sets one, and writes
// it there. A cancelled dialog is not an error.
func (k *Kiln) SaveAs(name string, data []byte) error {
	path, err := k.dialogs.SaveFile(name, k.exportDir())
	if err != nil || path == "" {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// OpenURL opens an http or https link in the default browser; any
// other is refused.
func (k *Kiln) OpenURL(link string) error {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" { //str:ok
		return errors.New(str.DesktopNotALink(link))
	}
	return k.win.OpenURL(link)
}

func (k *Kiln) running() *webapi.Loop {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.loop
}

// exportDir is the running config's export_dir, read on the loop; ""
// when it sets none or the loop isn't running.
func (k *Kiln) exportDir() string {
	k.mu.Lock()
	l, done := k.loop, k.done
	k.mu.Unlock()
	if l == nil {
		return ""
	}
	got := make(chan string, 1)
	l.Call(func(a *app.App) { got <- a.Config().ExportDir })
	select {
	case dir := <-got:
		return dir
	case <-done:
		return ""
	}
}

// gateWait is how long quitGate waits for the loop to say who's
// connected before asking off the main thread instead.
const gateWait = 250 * time.Millisecond

// quitGate is whether a quit (Cmd+Q, the app menu's Quit, the system's)
// or the window's closing (its close button, Alt+F4) goes ahead: Wails'
// ShouldQuit and WindowClosing hook. Once a quit is answered every gate
// lets it through, so the closing and quitting that follow it don't ask
// again.
//
// With nothing connected the quit goes ahead. Otherwise it's held while
// shouldQuit asks, off the gate: ShouldQuit runs on the main thread,
// which the question needs and which the loop may be waiting on. Quit
// quits again, and that passes. While the question is up, other gates
// hold without asking.
func (k *Kiln) quitGate() bool {
	k.quitMu.Lock()
	defer k.quitMu.Unlock()
	if k.quitting {
		return true
	}
	if k.asking {
		return false
	}
	if n, ok := k.connected(time.After(gateWait)); ok && n == 0 {
		k.quitting = true
		return true
	}
	k.asking = true
	go func() {
		yes := k.shouldQuit()
		k.quitMu.Lock()
		k.asking, k.quitting = false, yes
		k.quitMu.Unlock()
		if yes {
			k.win.Quit()
		}
	}()
	return false
}

// shouldQuit is true when no character is connected, and otherwise asks,
// true for Quit. It waits for the answer, so it never runs on the main
// thread.
func (k *Kiln) shouldQuit() bool {
	n, _ := k.connected(nil)
	return n == 0 || k.dialogs.ConfirmQuit(n)
}

func (k *Kiln) isAsking() bool {
	k.quitMu.Lock()
	defer k.quitMu.Unlock()
	return k.asking
}

// connected is how many characters are connected, read on the loop; 0
// when it isn't running. ok is false if timeout fired first (nil: wait
// for the loop).
func (k *Kiln) connected(timeout <-chan time.Time) (n int, ok bool) {
	k.mu.Lock()
	l, done := k.loop, k.done
	k.mu.Unlock()
	if l == nil {
		return 0, true
	}
	got := make(chan int, 1)
	l.Call(func(a *app.App) {
		n := 0
		for _, key := range a.Order() {
			if a.Char(key).State == session.Connected {
				n++
			}
		}
		got <- n
	})
	select {
	case n := <-got:
		return n, true
	case <-done:
		return 0, true
	case <-timeout:
		return 0, false
	}
}
