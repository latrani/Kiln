package feed

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/kilntest"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
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
	writeWorld(t, dir, fmWorld)
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

func writeWorld(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func open(t *testing.T, a *app.App, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if c, err := a.Open(k); c == nil || err != nil {
			t.Fatalf("Open(%s) = %v, %v", k, c, err)
		}
	}
}

// attach gives k a session, so session messages for it are taken.
func attach(a *app.App, k string) *session.Session {
	c := a.Char(k)
	c.Sess = session.New(session.Options{Char: c.Ch, Log: kilntest.NopLog{}})
	return c.Sess
}

func line(a *app.App, k string, s *session.Session, text string) {
	a.Handle(app.SessionMsg{Key: k, Sess: s, OK: true, Ev: session.Event{Kind: session.EventLine, Entry: logstore.Entry{Time: day, Dir: logstore.In, Text: text}}})
}

// kinds names evs' types, in order.
func kinds(evs []Event) []string {
	var out []string
	for _, e := range evs {
		out = append(out, strings.TrimPrefix(reflect.TypeOf(e).String(), "feed."))
	}
	return out
}

func find[T Event](evs []Event) (T, bool) {
	for _, e := range evs {
		if v, ok := e.(T); ok {
			return v, true
		}
	}
	var zero T
	return zero, false
}

func TestFirstTurnSendsEverything(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	f := New(a)
	evs := f.Turn(nil)
	want := []string{"Worlds", "Order", "Char", "Active", "Status", "Confirm", "SaveQuestion"}
	if got := kinds(evs); !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds %v, want %v", got, want)
	}
	if w, _ := find[Worlds](evs); len(w.Worlds) != 1 || w.Worlds[0].ID != "fm" || len(w.Worlds[0].Characters) != 2 {
		t.Errorf("worlds %+v", w)
	}
	if o, _ := find[Order](evs); !reflect.DeepEqual(o.Keys, []string{"fm/kit"}) {
		t.Errorf("order %+v", o)
	}
	if c, _ := find[Char](evs); c.Key != "fm/kit" || c.Name != "Kit" || c.World != "fm" {
		t.Errorf("char %+v", c)
	}
	if act, _ := find[Active](evs); act.Key != "fm/kit" {
		t.Errorf("active %+v", act)
	}
}

func TestNothingNewNothingSent(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	f := New(a)
	f.Turn(nil)
	if evs := f.Turn(nil); len(evs) != 0 {
		t.Errorf("a turn with nothing new sent %v", kinds(evs))
	}
}

func TestClosedAndStatus(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit", "fm/rook")
	f := New(a)
	f.Turn(nil)
	a.Close("fm/rook")
	a.SetStatus(true, "oops") //str:ok: a test status
	evs := f.Turn(nil)
	if got := kinds(evs); !reflect.DeepEqual(got, []string{"Order", "Closed", "Status"}) {
		t.Fatalf("kinds %v", got)
	}
	if c, _ := find[Closed](evs); c.Key != "fm/rook" {
		t.Errorf("closed %+v", c)
	}
	if s, _ := find[Status](evs); s.Text != "oops" || !s.Err {
		t.Errorf("status %+v", s)
	}
}

func TestEffectsComeLastWithoutRun(t *testing.T) {
	a, _ := newApp(t)
	f := New(a)
	f.Turn(nil)
	effs := []app.Effect{app.Run{Func: func() any { return nil }}, app.Copy{Text: "x"}, app.Quit{}}
	evs := f.Turn(effs)
	if got := kinds(evs); !reflect.DeepEqual(got, []string{"Effect", "Effect"}) {
		t.Fatalf("kinds %v", got)
	}
	if e := evs[0].(Effect); e.Effect != (app.Copy{Text: "x"}) {
		t.Errorf("first effect %+v", e)
	}
}

func TestAllSendsEverythingAgain(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	f := New(a)
	first := f.Turn(nil)
	if again := f.All(); !reflect.DeepEqual(kinds(again), kinds(first)) {
		t.Errorf("All sent %v, the first turn %v", kinds(again), kinds(first))
	}
	if evs := f.Turn(nil); len(evs) != 0 {
		t.Errorf("a turn after All sent %v", kinds(evs))
	}
}

// The feed is plain Go: no terminal, no UI package.
func TestFeedDependsOnNoFrontEnd(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, dep := range strings.Fields(string(out)) {
		if strings.HasSuffix(dep, "/internal/ui") || strings.Contains(dep, "bubbletea") || strings.Contains(dep, "lipgloss") {
			t.Errorf("feed depends on %s", dep)
		}
	}
}
