package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
)

// testConn is a scripted server connection.
type testConn struct {
	lines     chan string
	prompts   chan string
	mu        sync.Mutex
	sent      []string
	sizes     [][2]int
	closeOnce sync.Once
}

func newTestConn() *testConn {
	return &testConn{lines: make(chan string, 100), prompts: make(chan string, 1)}
}
func (c *testConn) Lines() <-chan string   { return c.lines }
func (c *testConn) Prompts() <-chan string { return c.prompts }
func (c *testConn) Err() error             { return nil }
func (c *testConn) Close() error           { c.closeOnce.Do(func() { close(c.lines) }); return nil }
func (c *testConn) Send(l string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, l)
	return nil
}
func (c *testConn) Resize(w, h int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sizes = append(c.sizes, [2]int{w, h})
	return nil
}
func (c *testConn) Sizes() [][2]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][2]int(nil), c.sizes...)
}
func (c *testConn) Sent() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.sent...)
}

type memLog struct{}

func (memLog) Append(logstore.Entry) error { return nil }

type harness struct {
	t       *testing.T
	m       *Model
	dir     string
	mu      sync.Mutex
	conns   map[string]*testConn
	dialErr map[string]error
	saved   map[string]string
	pw      map[string]string
}

const fmWorld = `host = "muck.test"
port = 8888
tls = true
use = ["fuzzball"]
login = "connect {name} {password}"
max_line_bytes = 20

[characters.kit]
name = "Kit"
autoconnect = true

[characters.rook]
name = "Rook"
`

func newHarness(t *testing.T, worlds map[string]string) *harness {
	t.Helper()
	dir := t.TempDir()
	if err := config.EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	for name, body := range worlds {
		os.WriteFile(filepath.Join(dir, "worlds", name+".toml"), []byte(body), 0o600)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, dir: dir, conns: map[string]*testConn{}, dialErr: map[string]error{},
		saved: map[string]string{}, pw: map[string]string{"fm/kit": "hunter2", "fm/rook": "pw"}}
	d := Deps{
		ConfigDir:  dir,
		LogRoot:    filepath.Join(dir, "logs"),
		KnownHosts: conn.KnownHosts{Path: filepath.Join(dir, "known_hosts")},
		Load:       config.Load,
		Dial: func(ctx context.Context, ch config.Character) (session.LineConn, error) {
			h.mu.Lock()
			defer h.mu.Unlock()
			k := key(ch.World, ch.ID)
			if err := h.dialErr[k]; err != nil {
				delete(h.dialErr, k)
				return nil, err
			}
			c := newTestConn()
			h.conns[k] = c
			return c, nil
		},
		NewLog: func(world, char string) session.Appender { return memLog{} },
		Password: func(world, char string) (string, error) {
			h.mu.Lock()
			defer h.mu.Unlock()
			if p, ok := h.pw[key(world, char)]; ok {
				return p, nil
			}
			return "", errors.New("not found")
		},
		SavePassword: func(world, char, pw string) error {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.saved[key(world, char)] = pw
			return nil
		},
		Now: func() time.Time { return time.Date(2026, 9, 24, 21, 14, 0, 0, time.Local) },
	}
	h.m = New(d, cfg)
	h.m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return h
}

func (h *harness) conn(k string) *testConn {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.conns[k]
}

// settle feeds session events for k into the model until pred holds.
func (h *harness) settle(k string, pred func() bool) {
	h.t.Helper()
	deadline := time.After(3 * time.Second)
	for !pred() {
		cs := h.m.chars[k]
		select {
		case ev, ok := <-cs.sess.Events():
			h.m.Update(eventMsg{key: k, sess: cs.sess, ev: ev, ok: ok})
		case <-deadline:
			h.t.Fatalf("timed out; screen:\n%s", h.screen())
		}
	}
}

// connected waits for the Connected state and, when the character has a
// saved password, for the auto-login line (sent just after the state
// change) so tests never race it.
func (h *harness) connected(k string) func() bool {
	return func() bool {
		if h.m.chars[k].state != session.Connected {
			return false
		}
		h.mu.Lock()
		_, hasPW := h.pw[k]
		h.mu.Unlock()
		c := h.conn(k)
		return !hasPW || (c != nil && len(c.Sent()) > 0)
	}
}

func (h *harness) screen() string { return ansi.Strip(h.m.View().Content) }

func (h *harness) typeText(s string) {
	for _, r := range s {
		h.m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func (h *harness) press(code rune, mod tea.KeyMod) tea.Cmd {
	_, cmd := h.m.Update(tea.KeyPressMsg{Code: code, Mod: mod})
	return cmd
}

// drainScrollback runs scrollback history reads the way Bubble Tea
// would, feeding each result back in until no read is in flight.
func (h *harness) drainScrollback(cmd tea.Cmd) {
	for cmd != nil {
		msg, ok := cmd().(sbOlderMsg)
		if !ok {
			return
		}
		_, cmd = h.m.Update(msg)
	}
}

func (h *harness) enter() tea.Cmd { return h.press(tea.KeyEnter, 0) }

func (h *harness) init() {
	h.m.Init() // starts autoconnect sessions; the returned cmds (clock, waits) are not run
}

func TestLayoutShowsSidebarAndStatus(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	s := h.screen()
	for _, want := range []string{"▾ fm", "✕ Kit", "✕ Rook", "fm/Kit 🔒 · disconnected · 21:14", "> "} {
		if !strings.Contains(s, want) {
			t.Errorf("screen missing %q:\n%s", want, s)
		}
	}
	if rows := strings.Split(h.m.View().Content, "\n"); len(rows) != 24 {
		t.Errorf("screen has %d rows, want 24", len(rows))
	}
}

func TestTooSmall(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.Update(tea.WindowSizeMsg{Width: 20, Height: 5})
	if !strings.Contains(h.screen(), "Kiln needs at least") {
		t.Errorf("screen = %q", h.screen())
	}
}

func TestAutoconnectOnlyFlaggedCharacters(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	if h.m.chars["fm/rook"].sess != nil {
		t.Error("rook connected without autoconnect")
	}
	if got := h.conn("fm/kit").Sent(); len(got) != 1 || got[0] != "connect Kit hunter2" {
		t.Errorf("auto-login sent %q", got)
	}
	if s := h.screen(); !strings.Contains(s, "○ Kit") || !strings.Contains(s, "connected to muck.test:8888") {
		t.Errorf("screen:\n%s", s)
	}
}

func TestIncomingLinesHighlightAndBadges(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.press(tea.KeyDown, tea.ModCtrl) // look at rook; kit is now in the background
	c := h.conn("fm/kit")
	c.lines <- "Rook says, \"hi\""
	c.lines <- "Mira pages: \x1b]0;pwned\x07you around?"
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].unread == 2 })
	if !h.m.chars["fm/kit"].attention {
		t.Error("page did not set attention")
	}
	if s := h.screen(); !strings.Contains(s, "● Kit        2") {
		t.Errorf("sidebar badge missing:\n%s", s)
	}
	h.press(tea.KeyUp, tea.ModCtrl)
	s := h.screen()
	if strings.Contains(s, "● Kit") || !strings.Contains(s, "○ Kit") {
		t.Errorf("switching back should clear badges:\n%s", s)
	}
	if !strings.Contains(s, "Mira pages: you around?") || strings.Contains(h.m.View().Content, "pwned") {
		t.Errorf("page not shown sanitized:\n%s", s)
	}
	if !strings.Contains(h.m.View().Content, "\x1b[1;38;2;255;159;67mMira pages") {
		t.Error("page not styled by the fuzzball pack")
	}
}

func TestSendBatchAndFlatten(t *testing.T) {
	flat := strings.Replace(fmWorld, "max_line_bytes = 20", "max_line_bytes = 20\nnewline_mode = \"flatten\"", 1)
	for _, c := range []struct {
		world string
		want  []string
	}{{fmWorld, []string{"connect Kit hunter2", ":waves.", "say hi"}}, {flat, []string{"connect Kit hunter2", ":waves. say hi"}}} {
		h := newHarness(t, map[string]string{"fm": c.world})
		h.init()
		h.settle("fm/kit", h.connected("fm/kit"))
		h.typeText(":waves.")
		h.press(tea.KeyEnter, tea.ModShift)
		h.typeText("say hi")
		h.enter()
		if got := h.conn("fm/kit").Sent(); strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("sent %q, want %q", got, c.want)
		}
		if !strings.Contains(h.screen(), "> :waves.") {
			t.Errorf("echo missing:\n%s", h.screen())
		}
		if !h.m.chars["fm/kit"].in.Empty() {
			t.Error("input not cleared")
		}
	}
}

func TestOverLimitNeedsConfirm(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld}) // max_line_bytes = 20
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.typeText("this line is definitely too long")
	h.enter()
	if n := len(h.conn("fm/kit").Sent()); n != 1 {
		t.Fatalf("sent %d lines before confirm", n)
	}
	if !strings.Contains(h.screen(), "over 20 bytes") {
		t.Errorf("no warning:\n%s", h.screen())
	}
	h.enter()
	if n := len(h.conn("fm/kit").Sent()); n != 2 {
		t.Errorf("confirm Enter did not send")
	}
}

func TestFlattenChecksJoinedLength(t *testing.T) {
	flat := strings.Replace(fmWorld, "max_line_bytes = 20", "max_line_bytes = 20\nnewline_mode = \"flatten\"", 1)
	h := newHarness(t, map[string]string{"fm": flat})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.typeText("say one two")
	h.press(tea.KeyEnter, tea.ModShift)
	h.typeText("three four") // each line fits; joined is 22 bytes
	h.enter()
	if n := len(h.conn("fm/kit").Sent()); n != 1 {
		t.Fatalf("sent %d lines before confirm", n)
	}
	if !strings.Contains(h.screen(), "over 20 bytes") {
		t.Errorf("no warning:\n%s", h.screen())
	}
}

func TestSlashCommandsAndEscape(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.typeText("//me waves")
	h.enter()
	if got := h.conn("fm/kit").Sent(); got[len(got)-1] != "/me waves" {
		t.Errorf("sent %q", got)
	}
	h.typeText("/bogus")
	h.enter()
	if !strings.Contains(h.screen(), "unknown command /bogus") {
		t.Errorf("screen:\n%s", h.screen())
	}
	h.typeText("/disconnect")
	h.enter()
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].state == session.Disconnected })
	h.typeText("/connect")
	h.enter()
	h.settle("fm/kit", h.connected("fm/kit"))
}

func TestHighlightKeepsSpacing(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.typeText("/highlight   the  old   lighthouse ")
	h.enter()
	cfg, err := config.Load(h.dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, _ := cfg.Find("fm", "kit")
	rs := kit.Rules.Highlight
	if len(rs) == 0 || rs[len(rs)-1].Match.Pattern != "(?i)the  old   lighthouse" {
		t.Errorf("rules = %+v", rs)
	}
}

func TestNotConnectedStatus(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("hello")
	h.enter()
	if !strings.Contains(h.screen(), "Kit is not connected (/connect)") {
		t.Errorf("screen:\n%s", h.screen())
	}
	if h.m.chars["fm/kit"].in.Value() != "hello" {
		t.Error("input lost")
	}
}

func TestPasswordPromptAndSave(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	delete(h.pw, "fm/kit")
	h.init()
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].needPW })
	h.typeText("s3cret")
	s := h.screen()
	if !strings.Contains(s, "> ••••••") || strings.Contains(s, "s3cret") {
		t.Errorf("password not masked:\n%s", s)
	}
	h.enter()
	if got := h.conn("fm/kit").Sent(); len(got) != 1 || got[0] != "connect Kit s3cret" {
		t.Errorf("sent %q", got)
	}
	if !strings.Contains(h.screen(), "connect Kit ***") {
		t.Errorf("redacted echo missing:\n%s", h.screen())
	}
	h.m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if h.saved["fm/kit"] != "s3cret" {
		t.Errorf("saved = %q", h.saved)
	}
	if len(h.m.chars["fm/kit"].in.history) != 0 {
		t.Error("password leaked into input history")
	}
}

func TestPasswordPromptKeepsDraft(t *testing.T) {
	for _, skip := range []bool{false, true} {
		h := newHarness(t, map[string]string{"fm": fmWorld})
		delete(h.pw, "fm/kit")
		h.init()
		h.typeText("half a pose")
		cs := h.m.chars["fm/kit"]
		h.settle("fm/kit", func() bool { return cs.needPW })
		if v := cs.in.Value(); v != "" {
			t.Fatalf("password prompt starts with %q", v)
		}
		h.typeText("s3cret")
		if skip {
			h.press(tea.KeyEscape, 0)
		} else {
			h.enter()
			if got := h.conn("fm/kit").Sent(); len(got) != 1 || got[0] != "connect Kit s3cret" {
				t.Errorf("sent %q", got)
			}
		}
		if v := cs.in.Value(); v != "half a pose" || cs.needPW {
			t.Errorf("skip=%v: input = %q, needPW = %v; want the draft back", skip, v, cs.needPW)
		}
	}
}

func TestPromptShown(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.conn("fm/kit").prompts <- "Name: "
	h.settle("fm/kit", func() bool { return strings.Contains(h.screen(), "Name: ") })
}

func TestPreloadsHistory(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "logs")
	w := logstore.NewWriter(root, "fm", "kit")
	ts := time.Date(2026, 9, 23, 20, 0, 0, 0, time.Local)
	w.Append(logstore.Entry{Time: ts, Dir: logstore.In, Text: "yesterday's news"})
	w.Close()
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.d.LogRoot = root
	h.m.preload(h.m.chars["fm/kit"])
	s := h.screen()
	if !strings.Contains(s, "yesterday's news") || !strings.Contains(s, "history ends Wed Sep 23 20:00") {
		t.Errorf("screen:\n%s", s)
	}
}

func TestReloadAddsCharactersAndKeepsOldOnError(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	p := filepath.Join(h.dir, "worlds", "fm.toml")
	os.WriteFile(p, []byte(fmWorld+"\n[characters.ash]\nname = \"Ash\"\n"), 0o600)
	h.m.Update(reloadMsg{})
	if !strings.Contains(h.screen(), "Ash") {
		t.Errorf("new character missing:\n%s", h.screen())
	}
	os.WriteFile(p, []byte("host = \n"), 0o600)
	h.m.Update(reloadMsg{})
	s := h.screen()
	if !strings.Contains(s, "config not reloaded") || !strings.Contains(s, "Ash") {
		t.Errorf("screen:\n%s", s)
	}
}

func TestRemovedConnectedCharacterLeavesOnDisconnect(t *testing.T) {
	sp := "host = \"sp.test\"\nport = 1\n\n[characters.ash]\nname = \"Ash\"\n"
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": sp})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	cs := h.m.chars["fm/kit"]
	sess := cs.sess

	noKit := strings.Replace(fmWorld, "[characters.kit]\nname = \"Kit\"\nautoconnect = true\n", "", 1)
	os.WriteFile(filepath.Join(h.dir, "worlds", "fm.toml"), []byte(noKit), 0o600)
	h.m.Update(reloadMsg{})
	if !cs.orphan || h.m.chars["fm/kit"] != cs {
		t.Fatal("connected character should stay as an orphan")
	}
	if got := strings.Join(h.m.order, " "); got != "fm/rook fm/kit sp/ash" {
		t.Errorf("order = %s, want the orphan grouped under fm", got)
	}
	if n := strings.Count(h.screen(), "▾ fm"); n != 1 {
		t.Errorf("fm header shown %d times:\n%s", n, h.screen())
	}

	h.conn("fm/kit").Close() // server drops the connection
	deadline := time.After(3 * time.Second)
	for h.m.chars["fm/kit"] != nil {
		select {
		case ev, ok := <-sess.Events():
			h.m.Update(eventMsg{key: "fm/kit", sess: sess, ev: ev, ok: ok})
		case <-deadline:
			t.Fatal("orphan not dropped after disconnect")
		}
	}
	if slices.Contains(h.m.order, "fm/kit") {
		t.Errorf("order = %v", h.m.order)
	}
	for { // the session is stopped: it must not redial and log in again
		select {
		case _, ok := <-sess.Events():
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("orphan session still running")
		}
	}
}

func manyChars(n int) string {
	w := "host = \"big.test\"\nport = 1\n"
	for i := range n {
		w += fmt.Sprintf("\n[characters.c%02d]\nname = \"C%02d\"\n", i, i)
	}
	return w
}

func TestSidebarScrolls(t *testing.T) {
	h := newHarness(t, map[string]string{"big": manyChars(30)}) // 31 rows, 24 high
	rows := func() []string { return strings.Split(h.screen(), "\n") }
	side := func(y int) string { return strings.TrimSpace(strings.SplitN(rows()[y], "│", 2)[0]) }
	if side(0) != "▾ big" || side(22) != "✕ C21" || side(23) != "▾ 8 more" {
		t.Fatalf("top of list:\n%s", h.screen())
	}

	// Ctrl+↓ past the bottom scrolls the active character into view.
	for range 25 {
		h.press(tea.KeyDown, tea.ModCtrl)
	}
	if h.m.active != "big/c25" || side(22) != "✕ C25" || side(0) != "▴ 5 more" {
		t.Fatalf("active %s not in view:\n%s", h.m.active, h.screen())
	}
	if side(23) != "▾ 4 more" {
		t.Errorf("bottom row = %q", side(23))
	}

	// The wheel scrolls freely; re-rendering doesn't snap back to active.
	h.m.Update(tea.MouseWheelMsg{X: 1, Y: 5, Button: tea.MouseWheelDown})
	h.m.Update(tea.MouseWheelMsg{X: 1, Y: 5, Button: tea.MouseWheelDown})
	if side(23) != "✕ C29" || side(0) != "▴ 8 more" {
		t.Errorf("wheel down to the end:\n%s", h.screen())
	}
	h.m.Update(tea.MouseWheelMsg{X: 1, Y: 5, Button: tea.MouseWheelUp})
	top := side(1)
	h.screen()
	if side(1) != top {
		t.Error("sidebar snapped back after scrolling")
	}

	// Clicks map through the scroll offset; the hint scrolls a page.
	h.m.Update(tea.MouseClickMsg{X: 3, Y: 1, Button: tea.MouseLeft})
	if want := "big/c" + strings.TrimPrefix(top, "✕ C"); h.m.active != want {
		t.Errorf("clicked %q, active = %s", top, h.m.active)
	}
	h.m.Update(tea.MouseClickMsg{X: 3, Y: 0, Button: tea.MouseLeft})
	if side(0) != "▾ big" {
		t.Errorf("clicking ▴ should page up to the top:\n%s", h.screen())
	}
}

func TestSidebarNoHintsWhenItFits(t *testing.T) {
	h := newHarness(t, map[string]string{"big": manyChars(23)}) // 24 rows, 24 high
	if s := h.screen(); strings.Contains(s, "more") || !strings.Contains(s, "C22") {
		t.Errorf("screen:\n%s", s)
	}
}

func TestResizeIsDebouncedAndReported(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld}) // 80×24: right pane 63 wide
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	c := h.conn("fm/kit")
	if got := c.Sizes(); len(got) != 1 || got[0] != [2]int{63, 24} {
		t.Fatalf("sizes at connect = %v, want [[63 24]]", got)
	}
	// A drag: three sizes in quick succession. Every tick fires, but only
	// the last one reports.
	var cmds []tea.Cmd
	for _, w := range []int{100, 110, 120} {
		_, cmd := h.m.Update(tea.WindowSizeMsg{Width: w, Height: 40})
		cmds = append(cmds, cmd)
	}
	for _, cmd := range cmds {
		h.m.Update(cmd())
	}
	if got := c.Sizes(); len(got) != 2 || got[1] != [2]int{120 - 22 - 1, 40} {
		t.Errorf("sizes = %v, want one more report of [97 40]", got)
	}
}

func TestScrollPill(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	c := h.conn("fm/kit")
	for i := 0; i < 60; i++ {
		c.lines <- "filler"
	}
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].sb.Len() >= 61 })
	h.press(tea.KeyPgUp, 0)
	if !strings.Contains(h.screen(), "▼ more") {
		t.Errorf("no pill:\n%s", h.screen())
	}
	c.lines <- "fresh"
	h.settle("fm/kit", func() bool {
		sb := &h.m.chars["fm/kit"].sb
		return strings.Contains(sb.lines[sb.Len()-1].text, "fresh")
	})
	if !strings.Contains(h.screen(), " new ") {
		t.Errorf("pill should count new lines:\n%s", h.screen())
	}
	l := h.m.layout()
	h.m.Update(tea.MouseClickMsg{X: 79, Y: l.sbH - 1, Button: tea.MouseLeft})
	if h.m.chars["fm/kit"].sb.Scrolled() || !strings.Contains(h.screen(), "fresh") {
		t.Errorf("pill click did not jump to live:\n%s", h.screen())
	}
}

func TestSidebarClick(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.Update(tea.MouseClickMsg{X: 3, Y: 2, Button: tea.MouseLeft}) // row 2 = Rook
	if h.m.active != "fm/rook" {
		t.Errorf("active = %q", h.m.active)
	}
	h.m.Update(tea.MouseClickMsg{X: 3, Y: 0, Button: tea.MouseLeft}) // collapse fm
	if s := h.screen(); !strings.Contains(s, "▸ fm") || strings.Contains(s, "Kit") && strings.Contains(s, "✕ Kit") {
		t.Errorf("collapse failed:\n%s", s)
	}
}

func TestTrustAfterPinMismatch(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.dialErr["fm/kit"] = &conn.PinMismatchError{HostPort: "muck.test:8888", Pinned: "sha256:aa", Got: "sha256:bb"}
	h.init()
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].pin != nil })
	if !strings.Contains(h.screen(), "/trust to accept") {
		t.Errorf("screen:\n%s", h.screen())
	}
	h.typeText("/trust")
	h.enter()
	h.settle("fm/kit", h.connected("fm/kit"))
	if fp, ok, _ := h.m.d.KnownHosts.Lookup("muck.test:8888"); !ok || fp != "sha256:bb" {
		t.Errorf("pin = %q %v", fp, ok)
	}
}

func TestCtrlCClearsThenQuits(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("oops")
	if cmd := h.press('c', tea.ModCtrl); cmd != nil {
		t.Error("ctrl+c with text should not quit")
	}
	if !h.m.chars["fm/kit"].in.Empty() {
		t.Error("ctrl+c did not clear")
	}
	cmd := h.press('c', tea.ModCtrl)
	if cmd == nil {
		t.Fatal("ctrl+c on empty input should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("not a quit")
	}
}

func TestPasteInsertsMultiline(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.Update(tea.PasteMsg{Content: "one\ntwo"})
	if got := h.m.chars["fm/kit"].in.Value(); got != "one\ntwo" {
		t.Errorf("input = %q", got)
	}
}

func TestSavePasswordAnswerBindsToPromptingCharacter(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	delete(h.pw, "fm/kit")
	h.init()
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].needPW })
	h.typeText("s3cret")
	h.enter()
	h.m.Update(tea.MouseClickMsg{X: 3, Y: 2, Button: tea.MouseLeft}) // click Rook mid-question
	h.m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if h.saved["fm/kit"] != "s3cret" || h.saved["fm/rook"] != "" {
		t.Errorf("saved = %q, want only fm/kit", h.saved)
	}
}

func TestTypedPasswordNotInHistory(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.typeText("connect Kit hunter2")
	h.enter()
	h.typeText("say hi")
	h.enter()
	h.press(tea.KeyUp, 0)
	h.press(tea.KeyUp, 0)
	if v := h.m.chars["fm/kit"].in.Value(); strings.Contains(v, "hunter2") {
		t.Errorf("history recalled %q", v)
	}
	if strings.Contains(h.screen(), "hunter2") {
		t.Errorf("password on screen:\n%s", h.screen())
	}
}

func TestConnectWhenAlreadyConnected(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.typeText("/connect")
	h.enter()
	if !strings.Contains(h.screen(), "already connected") {
		t.Errorf("screen:\n%s", h.screen())
	}
}

func TestScrollbackPagesHistoryAcrossPartialDay(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "logs")
	w := logstore.NewWriter(root, "fm", "kit")
	for _, d := range []int{23, 24} {
		start := time.Date(2026, 9, d, 8, 0, 0, 0, time.Local)
		for i := 0; i < 150; i++ {
			w.Append(logstore.Entry{Time: start.Add(time.Duration(i) * time.Minute), Dir: logstore.In, Text: fmt.Sprintf("d%d-%03d", d, i)})
		}
	}
	w.Close()
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.d.LogRoot = root
	cs := h.m.chars["fm/kit"]
	cs.sb = Scrollback{}
	h.m.preload(cs)
	// The preload took day 24 plus the last 50 lines of day 23; scrolling
	// to the very top must bring in the rest of day 23 exactly once.
	for i := 0; i < 100; i++ {
		h.drainScrollback(h.press(tea.KeyPgUp, 0))
		h.screen()
	}
	if cs.sb.loading || cs.sb.more {
		t.Errorf("loading=%v more=%v after paging to the top", cs.sb.loading, cs.sb.more)
	}
	var got []string
	dividers := map[string]int{}
	for _, l := range cs.sb.lines {
		p := ansi.Strip(l.text)
		switch {
		case strings.HasPrefix(p, "── "):
			dividers[p]++
		case strings.HasPrefix(p, "d2"):
			got = append(got, p)
		}
	}
	if len(got) != 300 {
		t.Fatalf("history lines = %d, want 300", len(got))
	}
	for i, p := range got {
		want := fmt.Sprintf("d%d-%03d", 23+i/150, i%150)
		if p != want {
			t.Fatalf("line %d = %q, want %q (order or duplication broken)", i, p, want)
		}
	}
	if dividers["── Wed Sep 23 ──"] != 1 || dividers["── Thu Sep 24 ──"] != 1 {
		t.Errorf("day dividers = %v, want one per day", dividers)
	}
}
