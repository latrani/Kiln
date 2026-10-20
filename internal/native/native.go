// Package native is Kiln's core as a front end on this machine runs it:
// the TUI and the desktop app build theirs here, so a fix to one reaches
// the other.
package native

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/secrets"
	"github.com/latrani/Kiln/internal/session"
)

// Options are where Kiln's files are, and the window's size.
type Options struct {
	ConfigDir, DataDir string
	Size               func() (w, h int) // the dial's starting window size; nil: none, the front end reports it once known
}

// Core is the core's deps as the TUI's: conn.Dial with known hosts,
// secrets for passwords, a log writer per layout, state.json, and the
// config watcher. Close closes the writers and the watcher.
type Core struct {
	Deps    app.Deps
	Changes <-chan struct{}
	Close   func()
}

// New makes the core and starts watching the config. Call
// config.EnsureDefaults first, as config.Watch needs.
func New(o Options) (*Core, error) {
	watcher, err := config.Watch(o.ConfigDir)
	if err != nil {
		return nil, err
	}
	hosts := KnownHosts(o.DataDir)
	var mu sync.Mutex
	writers := map[logstore.Layout]*logstore.Writer{}
	return &Core{
		Deps: app.Deps{
			ConfigDir:  o.ConfigDir,
			LogRoot:    filepath.Join(o.DataDir, "logs"),       //str:ok
			StateFile:  filepath.Join(o.DataDir, "state.json"), //str:ok
			KnownHosts: hosts,
			Load:       config.Load,
			Dial: func(ctx context.Context, ch config.Character) (session.LineConn, error) {
				var w, h int
				if o.Size != nil {
					w, h = o.Size()
				}
				return conn.Dial(ctx, conn.Options{
					Host: ch.Host, Port: ch.Port, TLS: ch.TLS, TLSTrust: ch.TLSTrust,
					KnownHosts: hosts, Width: w, Height: h,
				})
			},
			NewLog: func(l logstore.Layout) session.Appender {
				mu.Lock()
				defer mu.Unlock()
				if writers[l] == nil { // a changed log_dir or log_name gets a new writer
					writers[l] = logstore.NewWriter(l)
				}
				return writers[l]
			},
			Password: func(store, world, char string) (string, error) {
				return secrets.Open(store, o.DataDir).Get(world, char)
			},
			SavePassword: func(store, world, char, password string) error {
				return secrets.Open(store, o.DataDir).Set(world, char, password)
			},
			DeletePassword: func(store, world, char string) error {
				return secrets.Open(store, o.DataDir).Delete(world, char)
			},
		},
		Changes: watcher.Changes(),
		Close: sync.OnceFunc(func() {
			watcher.Close()
			mu.Lock()
			defer mu.Unlock()
			for _, w := range writers {
				w.Close()
			}
		}),
	}, nil
}

// KnownHosts is the servers' certificates Kiln has accepted, in dataDir.
func KnownHosts(dataDir string) conn.KnownHosts {
	return conn.KnownHosts{Path: filepath.Join(dataDir, "known_hosts")} //str:ok
}
