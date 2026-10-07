//go:build js

// Command kiln-web is Kiln in a browser tab: built for GOOS=js, drawing
// into xterm.js through package bridge, keeping files in the page's memfs
// (the page saves them to IndexedDB: persist.mjs), and reaching MUCKs
// through kiln-relay. web/static/kiln.js starts it; see
// docs/web-dev.md.
package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"syscall/js"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/secrets"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/ui"
	"github.com/latrani/Kiln/web/backup"
	"github.com/latrani/Kiln/web/bridge"
	"github.com/latrani/Kiln/web/wsdial"
)

type page struct {
	b          *bridge.Bridge
	cols, rows int
	relay      string
	// saving is whether the page is saving files; storageFailed fires when
	// it stops.
	saving        bool
	storageFailed chan struct{}
}

func main() {
	started := make(chan page, 1)
	k := js.Global().Get("Object").New() //str:ok
	// start is a JS callback, so it must not block: it sets everything up
	// and hands off to main.
	k.Set("start", js.FuncOf(func(_ js.Value, a []js.Value) any { //str:ok
		write := a[3]
		saving := len(a) > 4 && a[4].Bool()
		storageFailed := make(chan struct{}, 1)
		k.Set("storageFailed", js.FuncOf(func(js.Value, []js.Value) any { //str:ok
			select {
			case storageFailed <- struct{}{}:
			default:
			}
			return nil
		}))
		b := bridge.New(func(p []byte) {
			u := js.Global().Get("Uint8Array").New(len(p)) //str:ok
			js.CopyBytesToJS(u, p)
			write.Invoke(u)
		})
		k.Set("input", js.FuncOf(func(_ js.Value, v []js.Value) any { b.Input(v[0].String()); return nil }))            //str:ok
		k.Set("resize", js.FuncOf(func(_ js.Value, v []js.Value) any { b.Resize(v[0].Int(), v[1].Int()); return nil })) //str:ok
		started <- page{b: b, cols: a[0].Int(), rows: a[1].Int(), relay: a[2].String(), saving: saving, storageFailed: storageFailed}
		return nil
	}))
	js.Global().Set("kiln", k)    //str:ok
	js.Global().Call("kilnReady") //str:ok
	p := <-started
	if err := run(p); err != nil {
		p.b.Close()
		js.Global().Get("console").Call("error", str.CliError(err)) //str:ok
	}
}

func run(p page) error {
	cfgDir, err := config.Dir()
	if err != nil {
		return err
	}
	if err := config.EnsureDefaults(cfgDir); err != nil {
		return err
	}
	cfg, err := config.Load(cfgDir)
	if err != nil {
		return err
	}
	dataDir, err := config.DataDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	knownHosts := filepath.Join(dataDir, "known_hosts") //str:ok
	kh := conn.KnownHosts{Path: knownHosts}
	dial := wsdial.Dialer(p.relay)
	var mu sync.Mutex
	writers := map[logstore.Layout]*logstore.Writer{}
	m := ui.New(ui.Deps{
		ConfigDir:  cfgDir,
		LogRoot:    filepath.Join(dataDir, "logs"), //str:ok
		KnownHosts: kh,
		Load:       config.Load,
		Dial: func(ctx context.Context, ch config.Character) (session.LineConn, error) {
			// Roughly the right pane's size; the UI sends the exact size.
			return conn.Dial(ctx, conn.Options{
				Host: ch.Host, Port: ch.Port, TLS: ch.TLS, TLSTrust: ch.TLSTrust,
				KnownHosts: kh, Width: p.cols - ui.SidebarWidth(p.cols) - 1, Height: p.rows,
				DialContext: dial,
			})
		},
		NewLog: func(l logstore.Layout) session.Appender {
			mu.Lock()
			defer mu.Unlock()
			if writers[l] == nil {
				writers[l] = logstore.NewWriter(l)
			}
			return writers[l]
		},
		// No saved passwords in the browser: Kiln asks. (Sub-project 4 adds
		// a login form the browser's password manager can fill.)
		Password: func(string, string, string) (string, error) { return "", secrets.ErrNotFound },
		OpenURL: func(u string) error {
			js.Global().Call("open", u, "_blank", "noopener") //str:ok
			return nil
		},
		SaveFile: download,
		Backup: func() (string, error) {
			data, err := backup.Zip(cfgDir, knownHosts)
			if err != nil {
				return "", err
			}
			name := str.BackupFileName(time.Now().Format("2006-01-02")) //str:ok
			return name, download(name, data)
		},
		Restore: func() (int, error) {
			data, err := pickFile()
			if err != nil || data == nil {
				return 0, err
			}
			return backup.Unzip(data, cfgDir, knownHosts)
		},
	}, cfg)
	prog := tea.NewProgram(m, p.b.Options(p.cols, p.rows)...)
	p.b.Attach(prog)
	notSaving := ui.StatusMsg{Text: str.WebNotSaving(), Err: true}
	go func() {
		if !p.saving {
			prog.Send(notSaving)
		}
		for range p.storageFailed {
			prog.Send(notSaving)
		}
	}()
	_, err = prog.Run()
	return err
}
