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
	want := []string{"Worlds", "Order", "Char", "Input", "Input", "History", "Presence", "Active", "Status", "Confirm", "SaveQuestion", "Lines"}
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
	if again := f.All(); kinds(again)[0] != "Start" || !reflect.DeepEqual(kinds(again)[1:], kinds(first)) {
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

// writeLog puts lines in fm/kit's log, a minute apart from start.
func writeLog(t *testing.T, a *app.App, start time.Time, lines ...string) {
	t.Helper()
	l, _ := a.LogLayout(a.Char("fm/kit").Ch)
	kilntest.WriteLog(t, l, start, lines...)
}

func linesOf(evs []Event) []Lines {
	var out []Lines
	for _, e := range evs {
		if l, ok := e.(Lines); ok {
			out = append(out, l)
		}
	}
	return out
}

func seqs(ls []app.Line) []int {
	var out []int
	for _, l := range ls {
		out = append(out, l.Seq)
	}
	return out
}

func TestOneLineOneChange(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit", "fm/rook")
	s := attach(a, "fm/rook")
	f := New(a)
	f.Turn(nil)
	line(a, "fm/rook", s, "Hello")
	evs := f.Turn(nil)
	if got := kinds(evs); !reflect.DeepEqual(got, []string{"Char", "Lines"}) {
		t.Fatalf("kinds %v", got)
	}
	if c := evs[0].(Char); c.Key != "fm/rook" || c.Unread != 1 {
		t.Errorf("char %+v", c)
	}
	if l := evs[1].(Lines); l.Key != "fm/rook" || l.Op != Append || len(l.Lines) != 1 || l.Lines[0].Seq != 0 {
		t.Errorf("lines %+v", l)
	}
}

func TestScrollbackPrependsReplacesAndResets(t *testing.T) {
	a, dir := newApp(t)
	many := make([]string, app.HistoryLines+50)
	for i := range many {
		many[i] = "[Wiki] edited"
	}
	ch, _ := a.Find("fm/kit")
	l, _ := a.LogLayout(ch)
	kilntest.WriteLog(t, l, day.AddDate(0, 0, -1), many...)
	open(t, a, "fm/kit")
	f := New(a)
	first := linesOf(f.Turn(nil))
	if len(first) != 1 || first[0].Op != Reset || first[0].Lines[len(first[0].Lines)-1].Seq != -1 {
		t.Fatalf("first: %+v", first)
	}
	msg := a.RequestOlder("fm/kit")[0].(app.Run).Func().(app.OlderMsg)
	page, _ := a.HandleOlder(msg)
	got := linesOf(f.Turn(nil))
	if len(got) != 1 || got[0].Op != Prepend || len(got[0].Lines) != len(page) || got[0].Lines[0].Seq != page[0].Seq {
		t.Fatalf("after an older page: %+v", got)
	}
	writeWorld(t, dir, fmWorld+"\n[[classify]]\ntag = \"wiki\"\npattern = '^\\[Wiki\\]'\n")
	if _, ok := a.Reload(); !ok {
		t.Fatal(a.Status().Text)
	}
	got = linesOf(f.Turn(nil))
	all := a.Char("fm/kit").Lines
	if len(got) != 1 || got[0].Op != Replace || len(got[0].Lines) != len(all) || got[0].Lines[0].Seq != all[0].Seq {
		t.Fatalf("after a rules change: %d events, op %v", len(got), got)
	}
	if tags := got[0].Lines[1].TagNames(); len(tags) == 0 || tags[0] != "wiki" {
		t.Errorf("replaced line tags %v", tags)
	}
}

func TestReopenResets(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	line(a, "fm/kit", s, "one")
	line(a, "fm/kit", s, "two")
	f := New(a)
	f.Turn(nil)
	a.Close("fm/kit")
	open(t, a, "fm/kit")
	s = attach(a, "fm/kit")
	line(a, "fm/kit", s, "again")
	got := linesOf(f.Turn(nil))
	if len(got) != 1 || got[0].Op != Reset || len(got[0].Lines) != 1 || got[0].Lines[0].Seq != 0 {
		t.Errorf("after reopening: %+v", got)
	}
}

func TestSinceMatchesWhatWasSent(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	f := New(a)
	var sentLines []app.Line
	epoch := 0
	take := func() {
		for _, l := range linesOf(f.Turn(nil)) {
			sentLines, epoch = append(sentLines, l.Lines...), l.Epoch
		}
	}
	take()
	for _, text := range []string{"a", "b", "c", "d"} {
		line(a, "fm/kit", s, text)
		take()
	}
	for n := -1; n < 4; n++ {
		var want []int
		for _, l := range sentLines {
			if l.Seq > n {
				want = append(want, l.Seq)
			}
		}
		got, ok := f.Since("fm/kit", epoch, n)
		if !ok || !reflect.DeepEqual(seqs(got), want) {
			t.Errorf("Since(%d) = %v, %v; sent after it %v", n, seqs(got), ok, want)
		}
	}
}
func TestLogMode(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	writeLog(t, a, day.AddDate(0, 0, -1), "one", "two", "three")
	f := New(a)
	f.Turn(nil)
	g := a.OpenLog("fm/kit")
	evs := f.Turn(nil)
	if got := kinds(evs); !reflect.DeepEqual(got, []string{"Log", "LogLines", "Filter"}) {
		t.Fatalf("opening: kinds %v", got)
	}
	if l, _ := find[Log](evs); !l.Open || l.Start != nil {
		t.Errorf("log %+v", l)
	}
	if ll, _ := find[LogLines](evs); ll.Op != Reset || len(ll.Lines) != len(g.Lines) {
		t.Errorf("log lines %+v", ll)
	}
	g.Mark(g.Lines[0])
	g.Mark(g.Lines[2])
	g.ToggleExclude(g.Lines[1])
	evs = f.Turn(nil)
	l, _ := find[Log](evs)
	if l.Start == nil || *l.Start != g.Lines[0].Seq || *l.End != g.Lines[2].Seq || !reflect.DeepEqual(l.Excluded, []int{g.Lines[1].Seq}) {
		t.Errorf("range %+v", l)
	}
	g.SetFind("t", nil, nil)
	l, _ = find[Log](f.Turn(nil))
	if !reflect.DeepEqual(l.Matches, []int{g.Lines[1].Seq, g.Lines[2].Seq}) {
		t.Errorf("matches %v", l.Matches)
	}
	g.AddText("one")
	evs = f.Turn(nil)
	l, _ = find[Log](evs)
	if !reflect.DeepEqual(l.Hidden, []int{g.Lines[1].Seq, g.Lines[2].Seq}) {
		t.Errorf("hidden %v", l.Hidden)
	}
	if fl, _ := find[Filter](evs); len(fl.Entries) == 0 || !fl.Entries[len(fl.Entries)-1].Only {
		t.Errorf("filter %+v; want the text row with Only", fl)
	}
	a.CloseLog("fm/kit")
	evs = f.Turn(nil)
	if l, _ := find[Log](evs); l.Open || len(evs) != 1 {
		t.Errorf("closing: %v, %+v", kinds(evs), l)
	}
}

// A reload that changes nothing about a character's tags doesn't send
// its lines again.
func TestReloadWithoutRetagSendsNoLines(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	line(a, "fm/kit", s, "one")
	f := New(a)
	f.Turn(nil)
	if _, ok := a.Reload(); !ok {
		t.Fatal(a.Status().Text)
	}
	if got := linesOf(f.Turn(nil)); len(got) != 0 {
		t.Errorf("a reload that retags nothing sent %+v", got)
	}
}

// Since names its numbering: lines after N from a numbering that has
// since restarted aren't what the page thinks.
func TestSinceKnowsTheNumberingRestarted(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	for _, text := range []string{"a", "b", "c"} {
		line(a, "fm/kit", s, text)
	}
	f := New(a)
	first := linesOf(f.Turn(nil))
	epoch := first[0].Epoch
	if got, ok := f.Since("fm/kit", epoch, 1); !ok || len(got) != 1 || got[0].Seq != 2 {
		t.Fatalf("Since in the same numbering: %v, %v", got, ok)
	}
	a.Close("fm/kit")
	open(t, a, "fm/kit")
	s = attach(a, "fm/kit")
	for _, text := range []string{"w", "x", "y", "z"} {
		line(a, "fm/kit", s, text)
	}
	again := linesOf(f.Turn(nil))
	if len(again) != 1 || again[0].Epoch == epoch {
		t.Fatalf("reopened: %+v; want a Reset in a new numbering", again)
	}
	if _, ok := f.Since("fm/kit", epoch, 2); ok {
		t.Error("Since from the old numbering should say it restarted")
	}
}

// What the TUI draws from the core reaches the page too.
func TestPresenceIdleTextConnectedAtAndStore(t *testing.T) {
	a, _ := newApp(t)
	f := New(a)
	evs := f.All()
	if p, ok := find[Presence](evs); !ok || p.State != app.PresenceUnknown {
		t.Errorf("presence %+v, %v", p, ok)
	}
	a.SetInput("/open fm/kit") // nothing open: the idle input
	a.Focus(true)
	evs = f.Turn(nil)
	if i, _ := find[Input](evs); i.Key != "" || i.Text != "/open fm/kit" {
		t.Errorf("idle input %+v", i)
	}
	if p, _ := find[Presence](evs); p.State != app.PresenceHere {
		t.Errorf("presence after focus %+v", p)
	}
	open(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	a.Handle(app.SessionMsg{Key: "fm/kit", Sess: s, OK: true, Ev: session.Event{Kind: session.EventState, State: session.Connected}})
	if c, _ := find[Char](f.Turn(nil)); !c.ConnectedAt.Equal(day) {
		t.Errorf("connected at %v", c.ConnectedAt)
	}
	if q, _ := find[SaveQuestion](f.All()); q.Store != a.PasswordStore() {
		t.Errorf("save question store %q, want %q", q.Store, a.PasswordStore())
	}
}

func TestLogSaysHowManyAreSelected(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	writeLog(t, a, day.AddDate(0, 0, -1), "one", "two", "three")
	f := New(a)
	g := a.OpenLog("fm/kit")
	g.Mark(g.Lines[0])
	g.Mark(g.Lines[2])
	g.ToggleExclude(g.Lines[1])
	if l, _ := find[Log](f.Turn(nil)); l.Selected != len(g.Selection()) || l.Selected != 2 {
		t.Errorf("selected %d, want %d", l.Selected, len(g.Selection()))
	}
}

func TestInputIsNotEchoed(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	f := New(a)
	f.Turn(nil)
	a.SetInput("typed by the page")
	f.Told("fm/kit", 1)
	if evs := f.Turn(nil); len(evs) != 0 {
		t.Errorf("the page's own text came back: %v", kinds(evs))
	}
	a.SetInput("") // the core clearing it, as Submit does
	if i, ok := find[Input](f.Turn(nil)); !ok || i.Text != "" || i.Key != "fm/kit" {
		t.Errorf("a core change: %+v, %v", i, ok)
	}
}

func TestCharCarriesTheLimit(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	c, _ := find[Char](New(a).Turn(nil))
	if c.MaxLineBytes != a.Char("fm/kit").Ch.MaxLineBytes || c.NewlineMode != a.Char("fm/kit").Ch.NewlineMode {
		t.Errorf("char %+v", c)
	}
}

// A line older than any the core holds can't be caught up from: the page
// would miss what's between.
func TestSinceRefusesAGap(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	line(a, "fm/kit", s, "a")
	f := New(a)
	epoch := linesOf(f.Turn(nil))[0].Epoch
	if _, ok := f.Since("fm/kit", epoch, -100); ok {
		t.Error("Since from before the oldest line held should refuse")
	}
	if got, ok := f.Since("fm/kit", epoch, -1); !ok || len(got) != 1 {
		t.Errorf("Since from just before the oldest: %v, %v", got, ok)
	}
}

// An empty list goes out as [], not null: a page can map over it.
func TestEmptyOrderIsAnEmptyList(t *testing.T) {
	a, _ := newApp(t)
	o, _ := find[Order](New(a).Turn(nil))
	if o.Keys == nil {
		t.Error("Order.Keys is nil")
	}
}

func TestCharSaysWhenAsPeopleRead(t *testing.T) {
	a, _ := newApp(t)
	f := New(a)
	open(t, a, "fm/kit", "fm/rook")
	f.All()
	s := attach(a, "fm/kit")
	a.Handle(app.SessionMsg{Key: "fm/kit", Sess: s, OK: true, Ev: session.Event{Kind: session.EventState, State: session.Connected}})
	line(a, "fm/kit", s, "hi")
	var kit, rook Char
	for _, e := range f.All() {
		if c, ok := e.(Char); ok && c.Key == "fm/kit" {
			kit = c
		} else if ok {
			rook = c
		}
	}
	if kit.ConnectedSince != app.ConnectedSince(day, day) || kit.LastSeen != app.Clock(day, day) {
		t.Errorf("Kit says %q, %q", kit.ConnectedSince, kit.LastSeen)
	}
	if rook.ConnectedSince != "" || rook.LastSeen != "" {
		t.Errorf("Rook says %q, %q", rook.ConnectedSince, rook.LastSeen)
	}
}

func TestWorldsAreInThePickersOrderWithAliases(t *testing.T) {
	a, dir := newApp(t)
	writeWorld(t, dir, fmWorld+"\n[[characters]]\nid = \"ash\"\nname = \"ash\"\naliases = [\"cinder\"]\n")
	if err := os.WriteFile(filepath.Join(dir, "worlds", "Bay.toml"), []byte("host = \"b.test\"\nport = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.Reload(); !ok {
		t.Fatal(a.Status().Text)
	}
	w, _ := find[Worlds](New(a).Turn(nil))
	var ids []string
	for _, x := range w.Worlds {
		ids = append(ids, x.ID)
	}
	if !reflect.DeepEqual(ids, []string{"Bay", "fm"}) {
		t.Errorf("worlds %v, want Bay before fm (case-insensitive)", ids)
	}
	fm := w.Worlds[1].Characters
	if len(fm) != 3 || fm[0].Name != "ash" || fm[1].Name != "Kit" || fm[2].Name != "Rook" {
		t.Errorf("fm's characters %+v, want by name, case-insensitive", fm)
	}
	if !reflect.DeepEqual(fm[0].Aliases, []string{"cinder"}) {
		t.Errorf("ash's aliases %v", fm[0].Aliases)
	}
}

func TestLogCarriesTheExportFormat(t *testing.T) {
	a, dir := newApp(t)
	p := filepath.Join(dir, "config.toml")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, append([]byte("export_format = \"html\"\n"), b...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.Reload(); !ok {
		t.Fatal(a.Status().Text)
	}
	open(t, a, "fm/kit")
	f := New(a)
	a.OpenLog("fm/kit")
	if l, _ := find[Log](f.Turn(nil)); l.ExportFormat != "html" {
		t.Errorf("ExportFormat %q, want html", l.ExportFormat)
	}
}
