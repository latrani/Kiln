//go:build js

// Command kiln-web is Kiln's core in a browser tab: built for GOOS=js,
// it runs the web API (internal/webapi) for the page, keeps files in the
// page's memfs (the page saves them to IndexedDB: persist.mjs), and
// reaches MUCKs through kiln-relay. The page starts it (web/ui/src/boot.ts); see
// docs/web-dev.md.
package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"syscall/js"
	"time"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/secrets"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/webapi"
	"github.com/latrani/Kiln/web/backup"
	"github.com/latrani/Kiln/web/resume"
	"github.com/latrani/Kiln/web/tz"
	"github.com/latrani/Kiln/web/wsdial"
)

// page is what kiln.start hands over.
type page struct {
	relay   string
	onBatch js.Value
	saving  bool
}

func main() {
	// The browser's own zone, before anything reads the time.
	if name := js.Global().Get("Intl").Call("DateTimeFormat").Call("resolvedOptions").Get("timeZone"); name.Type() == js.TypeString { //str:ok
		if loc := tz.Zone(name.String()); loc != nil {
			time.Local = loc
		}
	}
	started := make(chan page, 1)
	k := js.Global().Get("Object").New() //str:ok
	// JS callbacks must not block: start only hands off to main.
	k.Set("start", js.FuncOf(func(_ js.Value, a []js.Value) any { //str:ok
		started <- page{relay: a[0].String(), onBatch: a[1], saving: len(a) > 2 && a[2].Bool()}
		return nil
	}))
	k.Set("overLimit", js.FuncOf(func(_ js.Value, a []js.Value) any { //str:ok
		return webapi.OverLimit(a[0].String(), a[1].Int(), a[2].String())
	}))
	k.Set("over", js.FuncOf(func(_ js.Value, a []js.Value) any { //str:ok
		out := []any{}
		for _, r := range webapi.OverRanges(a[0].String(), a[1].Int(), a[2].String()) {
			out = append(out, []any{r[0], r[1]})
		}
		return out
	}))
	k.Set("check", js.FuncOf(func(_ js.Value, a []js.Value) any { //str:ok
		return webapi.CheckField(a[0].String(), a[1].String())
	}))
	// The page calls wake when it comes back into view, when a suspended
	// tab's WebSockets can look open but be dead. dropLinks is for tests.
	k.Set("wake", js.FuncOf(func(js.Value, []js.Value) any { wsdial.Wake(); return nil }))         //str:ok
	k.Set("dropLinks", js.FuncOf(func(js.Value, []js.Value) any { wsdial.DropAll(); return nil })) //str:ok
	k.Set("maxBackup", backup.MaxFile)                                                             //str:ok
	js.Global().Set("kiln", k)                                                                     //str:ok
	js.Global().Call("kilnReady")                                                                  //str:ok
	p := <-started
	if err := run(k, p); err != nil {
		js.Global().Get("console").Call("error", str.CliError(err)) //str:ok
	}
}

func run(k js.Value, p page) error {
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
	// Held connections to pick back up in this tab, saved beside the logs.
	held := resume.Open(filepath.Join(dataDir, "relay.json"), p.saving) //str:ok
	k.Set("synced", js.FuncOf(func(_ js.Value, a []js.Value) any { held.Synced(a[0].String()); return nil })) //str:ok
	k.Set("tokens", js.FuncOf(func(js.Value, []js.Value) any { //str:ok
		var out []any
		for _, t := range wsdial.Tokens() {
			out = append(out, t)
		}
		return out
	}))
	k.Set("detachLinks", js.FuncOf(func(js.Value, []js.Value) any { wsdial.DetachAll(); return nil })) //str:ok
	k.Set("closeLinks", js.FuncOf(func(js.Value, []js.Value) any { wsdial.CloseAll(); return nil }))   //str:ok
	go func() {
		for range time.Tick(time.Second) {
			held.Sync()
		}
	}()
	var mu sync.Mutex
	writers := map[logstore.Layout]*logstore.Writer{}
	a := app.New(app.Deps{
		ConfigDir: cfgDir,
		LogRoot:   filepath.Join(dataDir, "logs"),       //str:ok
		StateFile: filepath.Join(dataDir, "state.json"), //str:ok
		Load:      config.Load,
		Dial: func(ctx context.Context, ch config.Character) (session.LineConn, error) {
			// The relay does TLS, trusting the world as its config says, so
			// the world file's tls only says what to ask for. A held
			// connection saved for the character is picked back up.
			c, err := held.Dial(ctx, p.relay, app.Key(ch.World, ch.ID), ch)
			if err != nil {
				return nil, err
			}
			return c, nil
		},
		NewLog: func(l logstore.Layout) session.Appender {
			mu.Lock()
			defer mu.Unlock()
			if writers[l] == nil {
				writers[l] = logstore.NewWriter(l)
			}
			return writers[l]
		},
		// No saved passwords in the browser: Kiln asks. (Part 3 adds a
		// login form the browser's password manager can fill.)
		Password: func(string, string, string) (string, error) { return "", secrets.ErrNotFound },
	})
	a.ApplyConfig(cfg)
	l := webapi.New(a, func(b []byte) { p.onBatch.Invoke(string(b)) })
	k.Set("act", js.FuncOf(func(_ js.Value, v []js.Value) any { //str:ok
		l.Act([]byte(v[0].String()))
		return nil
	}))
	k.Set("storageFailed", js.FuncOf(func(js.Value, []js.Value) any { //str:ok
		l.Say(true, str.WebNotSaving())
		return nil
	}))
	if !p.saving {
		l.Say(true, str.WebNotSaving())
	}
	l.Run(context.Background(), webapi.Options{
		Autoconnect: true, Resume: held.Keys(), Drafts: filepath.Join(dataDir, "drafts.json"), ConfigDir: cfgDir, //str:ok
		Backup:  func() ([]byte, error) { return backup.Zip(cfgDir) },
		Restore: func(data []byte) (int, error) { return backup.Unzip(data, cfgDir) },
	})
	return nil
}
