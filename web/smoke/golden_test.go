package smoke

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"image/png"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/latrani/Kiln/internal/app"
	kconfig "github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/glyph"
	"github.com/latrani/Kiln/internal/kilntest"
	"github.com/latrani/Kiln/internal/logstore"
	kscene "github.com/latrani/Kiln/internal/scene"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
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

	mu      sync.Mutex
	conns   map[string]*kilntest.Conn
	missing []string // shots with no golden yet; see shoot
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
		pw = maps.Clone(kilntest.GoldenPasswords)
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
	ran := make(chan struct{}) // closed when the loop's Run returns
	var ranOnce sync.Once      // the page should open the bridge once; never close ran twice
	mux := http.NewServeMux()
	mux.Handle("/kiln/", http.StripPrefix("/kiln/", http.FileServer(http.Dir(page))))
	// What the wasm's kiln answers at once (web/cmd/kiln-web), which the
	// golden page asks for synchronously (golden.tsx): the editor's field
	// check, the input's over-limit ranges.
	mux.HandleFunc("/check", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(webapi.CheckField(r.FormValue("field"), r.FormValue("value"))))
	})
	mux.HandleFunc("/over", func(w http.ResponseWriter, r *http.Request) {
		max, _ := strconv.Atoi(r.FormValue("max"))
		rs := webapi.OverRanges(r.FormValue("text"), max, r.FormValue("mode"))
		if rs == nil {
			rs = [][2]int{}
		}
		json.NewEncoder(w).Encode(rs)
	})
	mux.HandleFunc("/bridge", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		c.SetReadLimit(64 << 20)
		l := webapi.New(a, func(b []byte) { c.Write(ctx, websocket.MessageText, b) })
		go func() {
			defer ranOnce.Do(func() { close(ran) })
			l.Run(ctx, webapi.Options{Autoconnect: true, ConfigDir: dir})
		}()
		select {
		case loops <- l:
		case <-ctx.Done():
			return
		}
		for {
			_, b, err := c.Read(ctx)
			if err != nil {
				return
			}
			l.Act(b)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		cancel()
		srv.Close()
		// Run hangs up every session as it ends: wait for it, so it's
		// done before the test's directories go.
		select {
		case <-ran:
		case <-time.After(5 * time.Second):
		}
	})
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
		waitFor(t, s.ev, `document.querySelector('.bottom input[type="password"]')`, "the password field")
	}
	// Kit's login can be sent before the app has taken the connect in,
	// which is when it stamps "Connected since": wait for that, or the
	// stamp can land after the clock moves on.
	waitFor(t, s.ev, `document.body.textContent.includes(`+jsString(upTo(str.ViewConnectedSince(mark)))+`)`, "Kit connected")
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

// done ends a scenario: skipped, once it has taken all its shots, if
// any had no golden yet. (Not a Cleanup: a skip there can turn into a
// bare FAIL.)
func (s *scene) done() {
	s.t.Helper()
	if len(s.missing) > 0 {
		s.t.Skipf("no golden yet for %s: run with -update to make them", strings.Join(s.missing, ", "))
	}
}

// shoot checks the page, light and dark, against
// testdata/golden/<name>-<scheme>.png. A shot with no golden yet is
// noted, and the scenario goes on.
func (s *scene) shoot(name string) {
	s.t.Helper()
	for _, scheme := range []string{"light", "dark"} {
		if !checkShot(s.t, filepath.Join("testdata", "golden", name+"-"+scheme+".png"), s.shot(scheme), *update) {
			s.missing = append(s.missing, name+"-"+scheme)
		}
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

// The nine scenarios, each the TUI's (internal/ui/golden_test.go): the
// same data, reached as a person would on the page, and waited for
// before the shot.

// goHas waits until ok holds of Go's state, read on the loop.
func (s *scene) goHas(ok func(a *app.App) bool, what string) {
	s.t.Helper()
	for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		got := make(chan bool, 1)
		s.l.Call(func(a *app.App) { got <- ok(a) })
		if <-got {
			return
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("waiting for %s in Go: gave up", what)
		}
	}
}

// show has the server send Kit line, and waits for it on the page.
func (s *scene) show(line string) {
	s.t.Helper()
	s.conn("fm/kit").Feed(line)
	waitFor(s.t, s.ev, `[...document.querySelectorAll("main .line")].some(l => l.textContent.includes(`+jsString(line)+`))`, line)
}

// typeIn types text into the focused field.
func (s *scene) typeIn(text string) {
	s.t.Helper()
	devtools(s.t, s.ws, "Input.insertText", map[string]any{"text": text})
}

// letter presses a letter key, as log mode's and the panel's keys take it.
func (s *scene) letter(k string) {
	s.t.Helper()
	devtools(s.t, s.ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": k, "text": k})
	devtools(s.t, s.ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": k})
}

// writeLog puts the log scene in Kit's log.
func (s *scene) writeLog() {
	kilntest.WriteLog(s.t, logstore.Layout{Root: s.logs, World: "fm", Char: "kit", CharName: "Kit"}, kilntest.LogDay, kilntest.LogScene...)
}

const input = `document.querySelector(".bottom textarea")`

func TestGoldenMain(t *testing.T) {
	s := goldenScene(t, nil, nil)
	// Rook opened but not connected, Kit still showing, as the TUI's open.
	s.ev(`kilnPage.act({Do: "open", Key: "fm/rook"})`)
	waitFor(t, s.ev, `document.querySelector(".side").textContent.includes("Rook")`, "Rook in the sidebar")
	s.show(kilntest.MainLines[0])
	s.show(kilntest.MainLines[1])
	waitFor(t, s.ev, `document.activeElement === `+input, "the input")
	s.typeIn(kilntest.MainTyped)
	pressKey(t, s.ws, "Enter", 13)
	waitFor(t, s.ev, `[...document.querySelectorAll("main .line")].some(l => l.textContent.includes(`+jsString(kilntest.MainTyped)+`))`, "the echo")
	s.l.Call(func(a *app.App) { c := a.Char("fm/rook"); c.Unread, c.Priority = 3, true })
	waitFor(t, s.ev, `[...document.querySelectorAll(".side li")].find(l => l.textContent.includes("Rook"))?.textContent.trim().endsWith("3")`, "Rook's unread")
	s.shoot("main")
	s.done()
}

func TestGoldenPicker(t *testing.T) {
	s := goldenScene(t, nil, nil)
	s.show(kilntest.MainLines[0])
	ctrlO(t, s.ws)
	waitFor(t, s.ev, `document.activeElement === document.querySelector('.bottom input[role="combobox"]')`, "the picker")
	s.shoot("picker")
	s.done()
}

func TestGoldenPickerAdd(t *testing.T) {
	s := goldenScene(t, nil, nil)
	s.show(kilntest.MainLines[0])
	ctrlO(t, s.ws)
	waitFor(t, s.ev, `document.activeElement === document.querySelector('.bottom input[role="combobox"]')`, "the picker")
	pressKey(t, s.ws, "ArrowDown", 40) // Rook, then fm's add row, as the TUI's
	pressKey(t, s.ws, "Enter", 13)
	waitFor(t, s.ev, `document.activeElement === document.querySelector("#kiln-name")`, "the name field")
	s.typeIn("Ash ") // the space won't do: the problem shows under the name
	waitFor(t, s.ev, `document.querySelector("#kiln-name-note").textContent !== ""`, "the name's problem")
	s.shoot("picker-add")
	s.done()
}

func TestGoldenEditor(t *testing.T) {
	s := goldenScene(t, nil, nil)
	waitFor(t, s.ev, `document.activeElement === `+input, "the input")
	s.typeIn("/edit world")
	pressKey(t, s.ws, "Enter", 13)
	port := `document.getElementById([...document.querySelectorAll(".bottom label")].find(l => l.textContent.startsWith(` + jsString(str.EditorPort()) + `)).htmlFor)`
	waitFor(t, s.ev, port, "the world's editor")
	s.shoot("editor")
	// The port is the relay's, fixed on the web, so the error is on a
	// field the page can edit: an x after More settings' max line bytes.
	s.ev(`document.querySelector(".editor details").open = true`)
	mb := `document.getElementById("ed-max_line_bytes")`
	waitFor(t, s.ev, mb+`.offsetParent !== null`, "the extras shown")
	s.ev(mb + `.focus()`)
	s.typeIn("x")
	waitFor(t, s.ev, `document.getElementById("ed-max_line_bytes-err")`, "the field's error")
	s.shoot("editor-error")
	s.done()
}

func TestGoldenLog(t *testing.T) {
	s := goldenScene(t, nil, func(s *scene) { s.writeLog() })
	ctrlL(t, s.ws)
	rows := `[...document.querySelectorAll(".lrow")]`
	waitFor(t, s.ev, rows+`.length === `+strconv.Itoa(len(kilntest.LogScene)), "log mode, the scene")
	// The TUI's s, ↑, s: the last two lines.
	s.ev(`document.querySelector(".log").focus()`)
	s.letter("s")
	waitFor(t, s.ev, rows+`.at(-1).querySelector(".gutter").textContent === `+jsString(glyph.Selected), "the last line")
	click(t, s.ws, s.ev, rows+`.at(-2)`)
	s.letter("s")
	waitFor(t, s.ev, `[...document.querySelectorAll(".lrow .gutter")].filter(g => g.textContent === `+jsString(glyph.Selected)+`).length === 2`, "the range")
	// The Filter chip on, as the TUI's: Only on page, set directly.
	s.l.Call(func(a *app.App) { a.Char("fm/kit").Log.PressOnly(kscene.Item{Name: "page"}) })
	waitFor(t, s.ev, rows+`.length < `+strconv.Itoa(len(kilntest.LogScene)), "Only on page")
	s.ev(`document.querySelector(".log").focus()`)
	s.letter("/")
	waitFor(t, s.ev, `document.activeElement === document.querySelector("#kiln-logprompt")`, "find")
	s.typeIn(kilntest.FindTerm)
	pressKey(t, s.ws, "Enter", 13)
	waitFor(t, s.ev, `document.querySelector("mark")`, "found")
	s.shoot("log")
	s.done()
}

func TestGoldenInput(t *testing.T) {
	s := goldenScene(t, nil, nil)
	waitFor(t, s.ev, `document.activeElement === `+input, "the input")
	s.typeIn(kilntest.InputText)
	// What's past max_line_bytes is marked, as the TUI's: Go's kiln.over,
	// in input.over_limit (theme.ts's roleClass).
	waitFor(t, s.ev, `document.querySelector(".bottom .r-input-over-limit")`, "the overflow marked")
	s.ev(input + `.setSelectionRange(0, 4)`) // the first four, as the TUI's drag
	// Typing dismisses a status, so Go has what's typed first.
	s.goHas(func(a *app.App) bool { return a.Text() == kilntest.InputText }, "the typing")
	notSent := str.StatusNotSent(errors.New("x"))
	s.l.Call(func(a *app.App) { a.SetStatus(true, notSent) })
	waitFor(t, s.ev, `document.body.textContent.includes(`+jsString(notSent)+`)`, "the status")
	s.shoot("input")
	s.done()
}

func TestGoldenScrolled(t *testing.T) {
	s := goldenScene(t, nil, nil)
	for i := 0; i < 40; i++ {
		s.clock.Advance(2 * time.Second)
		s.show(kilntest.ScrolledLine(i))
		// Read as it comes, as the TUI's MarkSeen: scrolled to the end, so
		// the pager holds nothing. (Not a wheel: in headless Chrome a wheel
		// past the end moves the whole viewport.)
		s.ev(`(e => { e.scrollTop = e.scrollHeight; })(document.querySelector("main .scroll"))`)
		waitFor(t, s.ev, `(e => e.scrollTop + e.clientHeight >= e.scrollHeight - 1)(document.querySelector("main .scroll"))`, "at the end")
	}
	pressKey(t, s.ws, "PageUp", 33)
	// Scrolled up, with nothing new below: the pager's ▼ more.
	waitFor(t, s.ev, `document.querySelector("main").textContent.includes(`+jsString(str.ViewPillMore())+`)`, "scrolled up")
	// The bottom row's link, hovered, as the TUI's.
	link := `[...document.querySelectorAll("main a")].filter(a => (r => (r.top + r.bottom) / 2)(a.getBoundingClientRect()) < document.querySelector("main .scroll").getBoundingClientRect().bottom).at(-1)`
	waitFor(t, s.ev, link, "the bottom row's link")
	r := s.ev(`JSON.stringify((r => ({x: r.left + 2, y: r.top + r.height / 2}))(` + link + `.getBoundingClientRect()))`)
	var p struct{ X, Y float64 }
	if err := json.Unmarshal([]byte(mustUnquote(t, r)), &p); err != nil {
		t.Fatal(err)
	}
	devtools(t, s.ws, "Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": p.X, "y": p.Y})
	s.shoot("scrolled")
	s.done()
}

func TestGoldenPassword(t *testing.T) {
	pw := maps.Clone(kilntest.GoldenPasswords)
	delete(pw, "fm/kit")
	s := goldenScene(t, pw, nil)
	field := `document.querySelector('.bottom input[type="password"]')`
	waitFor(t, s.ev, `document.activeElement === `+field, "the password field")
	s.typeIn(kilntest.PasswordTyped)
	s.shoot("password")
	s.done()
}

func TestGoldenLogFilter(t *testing.T) {
	s := goldenScene(t, nil, func(s *scene) { s.writeLog() })
	ctrlL(t, s.ws)
	waitFor(t, s.ev, `document.activeElement === document.querySelector(".log") && document.querySelector(".lrow")`, "log mode")
	s.letter("f")
	sel := `document.querySelector(".filter-panel .fsel")`
	waitFor(t, s.ev, `document.activeElement === `+sel, "the panel")
	// The TUI's keys: past Categories, page (folded), sent and Flags to
	// self, h hides it, then ↓ to + Text, where o does nothing.
	pressKey(t, s.ws, "ArrowDown", 40)
	pressKey(t, s.ws, "ArrowDown", 40)
	pressKey(t, s.ws, "ArrowDown", 40)
	pressKey(t, s.ws, "ArrowDown", 40)
	s.letter("h")
	waitFor(t, s.ev, `document.querySelectorAll('.filter-panel [aria-pressed="true"]').length === 1`, "self hidden")
	pressKey(t, s.ws, "ArrowDown", 40)
	s.letter("o")
	waitFor(t, s.ev, `document.activeElement === `+sel+` && `+sel+`.textContent === `+jsString(str.FilterAddText()), "+ Text")
	s.shoot("log-filter")
	s.done()
}

func TestGoldenHistory(t *testing.T) {
	s := goldenScene(t, nil, func(s *scene) {
		kilntest.WriteLog(s.t, logstore.Layout{Root: s.logs, World: "fm", Char: "kit", CharName: "Kit"}, kilntest.HistoryDay, kilntest.HistoryLogged...)
	})
	waitFor(t, s.ev, `document.activeElement === `+input, "the input")
	for _, line := range kilntest.HistorySent { // sent as the TUI's, a minute apart
		s.l.Call(func(a *app.App) {
			a.SetInput(line)
			a.Submit()
			if a.Confirming() { // over max_line_bytes: Enter again sends it anyway
				a.Submit()
			}
			s.clock.Advance(time.Minute) // in the loop's turn: the next send waits for it
		})
	}
	last := kilntest.HistorySent[len(kilntest.HistorySent)-1]
	waitFor(t, s.ev, `[...document.querySelectorAll("main .line")].some(l => l.textContent.includes(`+jsString(last)+`))`, "the last echo")
	pressKey(t, s.ws, "ArrowUp", 38)
	waitFor(t, s.ev, `document.querySelector(".history [aria-selected=true]")?.textContent.includes(`+jsString(last)+`)`, "the panel, the newest selected")
	s.shoot("history")
	s.done()
}
