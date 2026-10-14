package smoke

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/latrani/Kiln/internal/app"
	kconfig "github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/kilntest"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/webapi"
)

var update = flag.Bool("update", false, "rewrite the screenshot goldens in testdata/golden")

// goldenClock is the scenarios' clock, which they move on.
type goldenClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *goldenClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *goldenClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// scene is one golden scenario's run: Go natively over the shared data
// (internal/kilntest), the golden page in Chrome, and the two joined by
// a WebSocket.
type scene struct {
	t     *testing.T
	l     *webapi.Loop // reach the app only through l.Call
	ws    *websocket.Conn
	ev    func(string) string
	logs  string // the log root
	clock *goldenClock

	mu    sync.Mutex
	conns map[string]*kilntest.Conn
}

// conn is the fake connection Dial gave k.
func (s *scene) conn(k string) *kilntest.Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conns[k]
}

// goldenPage builds the golden page (src/golden.tsx) into a directory.
func goldenPage(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	root, _ := filepath.Abs("../..")
	ui := filepath.Join(root, "web", "ui")
	if _, err := os.Stat(filepath.Join(ui, "node_modules")); err != nil {
		t.Skip("run npm ci in web/ui first")
	}
	build := exec.Command("node", "build.mjs", dir, "golden")
	build.Dir = ui
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the golden page: %v\n%s", err, out)
	}
	return dir
}

// goldenScene starts a scenario as the TUI's goldenHarness does: the
// golden page in Chrome at 960×600 with Go across the bridge, Kit
// connected (logged in when pw has Kit's password; nil pw is
// GoldenPasswords) and the clock a minute on. setup runs first, before
// Kit connects (writing a log). Presence is pinned at here, since the
// headless tab's focus would otherwise decide it.
func goldenScene(t *testing.T, pw map[string]string, setup func(s *scene)) *scene {
	t.Helper()
	chrome := chromePath(t)
	if pw == nil {
		pw = kilntest.GoldenPasswords
	}
	page := goldenPage(t)
	dir := t.TempDir()
	if err := kconfig.EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte(kilntest.GoldenWorld), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &scene{t: t, logs: filepath.Join(dir, "logs"), clock: &goldenClock{now: kilntest.GoldenStart}, conns: map[string]*kilntest.Conn{}}
	a := app.New(app.Deps{
		ConfigDir: dir,
		Load:      kconfig.Load,
		LogRoot:   s.logs,
		Dial: func(_ context.Context, ch kconfig.Character) (session.LineConn, error) {
			c := kilntest.NewConn()
			s.mu.Lock()
			s.conns[app.Key(ch.World, ch.ID)] = c
			s.mu.Unlock()
			return c, nil
		},
		NewLog: func(logstore.Layout) session.Appender { return kilntest.NopLog{} },
		Password: func(_, world, char string) (string, error) {
			if p, ok := pw[app.Key(world, char)]; ok {
				return p, nil
			}
			return "", errors.New("none") //str:ok: a test's fake keychain
		},
		Now: s.clock.Now,
	})
	if _, ok := a.Reload(); !ok {
		t.Fatal(a.Status().Text)
	}
	if setup != nil {
		setup(s)
	}

	ctx, cancel := context.WithCancel(context.Background())
	loops := make(chan *webapi.Loop, 1)
	mux := http.NewServeMux()
	mux.Handle("/kiln/", http.StripPrefix("/kiln/", http.FileServer(http.Dir(page))))
	mux.HandleFunc("/bridge", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		c.SetReadLimit(64 << 20)
		l := webapi.New(a, func(b []byte) { c.Write(ctx, websocket.MessageText, b) })
		go l.Run(ctx, webapi.Options{Autoconnect: true, ConfigDir: dir})
		loops <- l
		for {
			_, b, err := c.Read(ctx)
			if err != nil {
				return
			}
			l.Act(b)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(func() { cancel(); srv.Close() })
	bridge := "ws" + strings.TrimPrefix(srv.URL, "http") + "/bridge"
	s.ws, _ = openTab(t, chrome, srv.URL+"/kiln/?bridge="+url.QueryEscape(bridge))
	devtools(t, s.ws, "Emulation.setDeviceMetricsOverride", map[string]any{"width": 960, "height": 600, "deviceScaleFactor": 1, "mobile": false})
	s.ev = evaluator(t, s.ws)
	select {
	case s.l = <-loops:
	case <-time.After(20 * time.Second):
		t.Fatal("the golden page never opened the bridge")
	}

	if _, ok := pw["fm/kit"]; ok {
		deadline := time.Now().Add(20 * time.Second)
		for c := s.conn("fm/kit"); c == nil || len(c.Sent()) == 0; c = s.conn("fm/kit") {
			if time.Now().After(deadline) {
				t.Fatal("Kit never logged in")
			}
			time.Sleep(50 * time.Millisecond)
		}
	} else {
		waitFor(t, s.ev, `document.querySelector('.bottom input[type="password"]')`, "the password form")
	}
	s.clock.Advance(time.Minute)
	s.ev(`kilnPage.act({Do: "here"})`)
	waitFor(t, s.ev, `document.querySelector("main .scroll")`, "the main screen")
	return s
}

// shot screenshots the page in scheme ("light" or "dark"), once its
// fonts are in and the next frame is drawn.
func (s *scene) shot(scheme string) []byte {
	s.t.Helper()
	devtools(s.t, s.ws, "Emulation.setEmulatedMedia", map[string]any{"features": []map[string]string{{"name": "prefers-color-scheme", "value": scheme}}})
	s.ev(`document.fonts.ready.then(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(() => r(true)))))`)
	var r struct{ Data string }
	if err := json.Unmarshal(devtools(s.t, s.ws, "Page.captureScreenshot", map[string]any{"format": "png"}), &r); err != nil {
		s.t.Fatal(err)
	}
	b, err := base64.StdEncoding.DecodeString(r.Data)
	if err != nil {
		s.t.Fatal(err)
	}
	return b
}

// shoot checks the page, light and dark, against
// testdata/golden/<name>-<scheme>.png.
func (s *scene) shoot(name string) {
	s.t.Helper()
	for _, scheme := range []string{"light", "dark"} {
		checkShot(s.t, filepath.Join("testdata", "golden", name+"-"+scheme+".png"), s.shot(scheme), *update)
	}
}

// TestGoldenStable draws main's start twice in each scheme and expects
// the same pixels: fonts loaded, no caret, a fixed clock. Without it a
// golden could fail for nothing.
func TestGoldenStable(t *testing.T) {
	s := goldenScene(t, nil, nil)
	for _, scheme := range []string{"light", "dark"} {
		a, b := s.shot(scheme), s.shot(scheme)
		ga, err := png.Decode(bytes.NewReader(a))
		if err != nil {
			t.Fatal(err)
		}
		gb, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		if n := differs(ga, gb); n > 0 {
			t.Errorf("%s: %d pixels differ between two shots of the same screen", scheme, n)
		}
	}
}
