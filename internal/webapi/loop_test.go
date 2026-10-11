package webapi

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/kilntest"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
)

const fmWorld = `host = "muck.test"
port = 8888
tls = true
use = ["fuzzball"]

[[characters]]
id = "kit"
name = "Kit"

[[characters]]
id = "rook"
name = "Rook"
`

var day = time.Date(2026, 9, 24, 21, 0, 0, 0, time.Local)

// newApp is an App over fm (Kit, Rook) with fake connections, and its
// config dir.
func newApp(t *testing.T) (*app.App, string) {
	t.Helper()
	dir := t.TempDir()
	if err := config.EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte(fmWorld), 0o600); err != nil {
		t.Fatal(err)
	}
	a := app.New(app.Deps{
		ConfigDir: dir,
		Load:      config.Load,
		LogRoot:   filepath.Join(dir, "logs"),
		Dial: func(context.Context, config.Character) (session.LineConn, error) {
			return kilntest.NewConn(), nil
		},
		NewLog:   func(logstore.Layout) session.Appender { return kilntest.NopLog{} },
		Password: func(string, string, string) (string, error) { return "", errors.New("none") },
		Now:      func() time.Time { return day },
	})
	if _, ok := a.Reload(); !ok {
		t.Fatal(a.Status().Text)
	}
	return a, dir
}

// harness runs a Loop and collects its batches.
type harness struct {
	t       *testing.T
	a       *app.App
	dir     string
	l       *Loop
	batches chan []map[string]any
}

func start(t *testing.T) *harness {
	t.Helper()
	a, dir := newApp(t)
	return startOn(t, a, dir)
}

func startOn(t *testing.T, a *app.App, dir string) *harness {
	t.Helper()
	h := &harness{t: t, a: a, dir: dir, batches: make(chan []map[string]any, 100)}
	h.l = New(a, func(b []byte) { h.batches <- decode(t, b) })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go h.l.Run(ctx, false)
	h.next() // the first batch: everything
	return h
}

// next is the next batch, or fails after a second.
func (h *harness) next() []map[string]any {
	h.t.Helper()
	select {
	case b := <-h.batches:
		return b
	case <-time.After(time.Second):
		h.t.Fatal("no batch")
		return nil
	}
}

// act sends an action and returns its turn's batch.
func (h *harness) act(js string) []map[string]any {
	h.t.Helper()
	h.l.Act([]byte(js))
	return h.next()
}

func types(b []map[string]any) []string {
	var out []string
	for _, e := range b {
		out = append(out, e["type"].(string))
	}
	return out
}

// event is b's first event of type typ, nil if none.
func event(b []map[string]any, typ string) map[string]any {
	for _, e := range b {
		if e["type"] == typ {
			return e
		}
	}
	return nil
}

func TestFirstBatchIsEverything(t *testing.T) {
	a, _ := newApp(t)
	got := make(chan []map[string]any, 1)
	l := New(a, func(b []byte) { got <- decode(t, b) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go l.Run(ctx, false)
	if b := <-got; types(b)[0] != "Start" {
		t.Errorf("first batch %v", types(b))
	}
}

func TestOpenSubmitAndInput(t *testing.T) {
	h := start(t)
	b := h.act(`{"Do":"open","Key":"fm/kit"}`)
	if !slices.Contains(types(b), "Char") || !slices.Contains(types(b), "Active") {
		t.Fatalf("open: %v", types(b))
	}
	b = h.act(`{"Do":"input","Key":"fm/kit","Text":"hello"}`)
	if slices.Contains(types(b), "Input") {
		t.Errorf("the page's own text came back: %v", b)
	}
	b = h.act(`{"Do":"submit","Text":"/notify off"}`) // a command: no connection needed
	var cleared bool
	for _, e := range b {
		if e["type"] == "Input" && e["Key"] == "fm/kit" && e["Text"] == "" {
			cleared = true
		}
	}
	if !cleared {
		t.Errorf("submit should clear the input: %v", b)
	}
	b = h.act(`{"Do":"older","Text":"draft"}`)
	if in := event(b, "Input"); in == nil || in["Text"] != "/notify off" {
		t.Errorf("history: %v", b)
	}
}

func TestBadActionsSaySo(t *testing.T) {
	h := start(t)
	want := upTo(str.StatusBadAction(errors.New(mark)))
	for _, js := range []string{`not json`, `{"Do":"frobnicate"}`} {
		b := h.act(js)
		if st := event(b, "Status"); st == nil || st["Err"] != true || !strings.HasPrefix(st["Text"].(string), want) {
			t.Errorf("%s: %v", js, b)
		}
	}
	h.act(`{"Do":"close","Key":"nobody/here"}`) // ignored, as the TUI would
	h.act(`{"Do":"mark","Key":"nobody/here","Seq":3}`)
}

func TestRunWorkComesBackOnTheLoop(t *testing.T) {
	h := start(t)
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	h.act(`{"Do":"submit","Text":""}`) // connect: a Run waiting on the session
	deadline := time.After(2 * time.Second)
	for {
		select {
		case b := <-h.batches:
			if c := event(b, "Char"); c != nil && c["State"] == float64(session.Connected) {
				return
			}
		case <-deadline:
			t.Fatal("never connected")
		}
	}
}

func TestQuitEndsTheLoop(t *testing.T) {
	a, _ := newApp(t)
	got := make(chan []map[string]any, 10)
	l := New(a, func(b []byte) { got <- decode(t, b) })
	done := make(chan struct{})
	go func() { l.Run(context.Background(), false); close(done) }()
	<-got
	l.Act([]byte(`{"Do":"quit"}`))
	b := <-got
	if last := b[len(b)-1]; last["type"] != "Effect" || last["effect"] != "Quit" {
		t.Errorf("last batch %v", types(b))
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("the loop kept running")
	}
}

func TestSinceOrEverything(t *testing.T) {
	h := start(t)
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	h.a.Char("fm/kit") // opened on the loop; read-only here
	b := h.act(`{"Do":"since","Key":"fm/kit","Epoch":999,"Seq":0}`)
	if types(b)[0] != "Start" {
		t.Errorf("a stale epoch should get everything: %v", types(b))
	}
}

func TestOverLimit(t *testing.T) {
	if OverLimit("12345", 5, "batch") || !OverLimit("123456", 5, "batch") || !OverLimit("123\n45", 5, "flatten") {
		t.Error("OverLimit")
	}
}

// webapi is plain Go: no js, no terminal, no UI package.
func TestWebapiDependsOnNoFrontEnd(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, dep := range strings.Fields(string(out)) {
		if dep == "syscall/js" || strings.HasSuffix(dep, "/internal/ui") || strings.Contains(dep, "bubbletea") || strings.Contains(dep, "lipgloss") {
			t.Errorf("webapi depends on %s", dep)
		}
	}
}

func TestIdleInputIsNotEchoed(t *testing.T) {
	h := start(t)
	if b := h.act(`{"Do":"input","Key":"","Text":"/open fm/kit"}`); slices.Contains(types(b), "Input") {
		t.Errorf("the idle input came back: %v", b)
	}
	if h.a.IdleText() != "/open fm/kit" {
		t.Errorf("idle text %q", h.a.IdleText())
	}
}
