package app

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
)

// writeLog puts lines in fm/kit's log under a's log root, one a minute
// from start; "> " starts a sent line.
func writeLog(t *testing.T, a *App, start time.Time, lines ...string) {
	t.Helper()
	w := logstore.NewWriter(logstore.Layout{Root: a.d.LogRoot, World: "fm", Char: "kit", CharName: "Kit"})
	defer w.Close()
	for i, l := range lines {
		dir := logstore.In
		if s, ok := strings.CutPrefix(l, "> "); ok {
			dir, l = logstore.Out, s
		}
		if err := w.Append(logstore.Entry{Time: start.Add(time.Duration(i) * time.Minute), Dir: dir, Text: l}); err != nil {
			t.Fatal(err)
		}
	}
}

func kinds(ls []*Line) []Kind {
	var ks []Kind
	for _, l := range ls {
		ks = append(ks, l.Kind)
	}
	return ks
}

var day23 = time.Date(2026, 9, 23, 20, 0, 0, 0, time.Local)

func TestOpenPreloadsTheTailOfHistory(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	writeLog(t, a, day23, "one", "> sent", "two")
	openAll(t, a, "fm/kit")
	kit := a.Char("fm/kit")
	if got, want := kinds(kit.Lines), []Kind{Day, Server, Server, HistoryEnd}; !slices.Equal(got, want) {
		t.Errorf("preloaded %v, want %v (sent lines hidden with local_echo off)", got, want)
	}
	if end := kit.Lines[len(kit.Lines)-1]; !end.Entry.Time.Equal(day23.Add(2 * time.Minute)) {
		t.Errorf("history ends at %v", end.Entry.Time)
	}
	if kit.More {
		t.Error("More with everything preloaded")
	}
	if at, ok := a.LastTime("fm/kit"); !ok || !at.Equal(day23.Add(2*time.Minute)) {
		t.Errorf("LastTime = %v, %v; want the last line's, not the end marker's or a divider's", at, ok)
	}
}

func TestPagingOlderHistory(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	lines := make([]string, HistoryLines+50)
	for i := range lines {
		lines[i] = "line"
	}
	writeLog(t, a, day23, lines...)
	openAll(t, a, "fm/kit")
	kit := a.Char("fm/kit")
	if !kit.More {
		t.Fatal("not More with history past the preload")
	}
	effs := a.RequestOlder("fm/kit")
	if len(effs) != 1 || !kit.Loading {
		t.Fatalf("RequestOlder = %v, loading %v", effs, kit.Loading)
	}
	if again := a.RequestOlder("fm/kit"); again != nil {
		t.Error("a second read started while one was in flight")
	}
	before := len(kit.Lines)
	msg := effs[0].(Run).Func().(OlderMsg)
	got, ok := a.HandleOlder(msg)
	if !ok || len(got) == 0 || len(kit.Lines) != before+len(got) || kit.Lines[0] != got[0] {
		t.Fatalf("HandleOlder = %d lines, %v; lines %d → %d", len(got), ok, before, len(kit.Lines))
	}
	if kit.Loading || kit.More {
		t.Errorf("after the last page: loading %v more %v", kit.Loading, kit.More)
	}
	a.Preload("fm/kit") // a new reader: a page from the old one is stale
	if _, ok := a.HandleOlder(msg); ok {
		t.Error("took a page from a replaced reader")
	}
}

// A page read under rules that have since changed is made again under
// the current ones when it arrives.
func TestOlderPageAfterARulesChange(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	lines := make([]string, HistoryLines+50)
	for i := range lines {
		lines[i] = "[Wiki] edited"
	}
	writeLog(t, a, day23, lines...)
	openAll(t, a, "fm/kit")
	msg := a.RequestOlder("fm/kit")[0].(Run).Func().(OlderMsg)
	dir := filepath.Dir(a.d.LogRoot) // sessionApp's log root is <config dir>/logs
	writeWorlds(t, dir, map[string]string{"fm": fmWorld + "\n[[classify]]\ntag = \"wiki\"\npattern = '^\\[Wiki\\]'\n"})
	a.ApplyConfig(load(t, dir))
	got, _ := a.HandleOlder(msg)
	for _, l := range got {
		if l.Kind == Server && !slices.Contains(l.TagNames(), "wiki") {
			t.Fatalf("%q arrived with the old rules' tags", l.Entry.Text)
		}
	}
	for _, l := range a.Char("fm/kit").Lines {
		if l.Kind == Server && !slices.Contains(l.TagNames(), "wiki") {
			t.Fatalf("a preloaded line kept the old rules' tags after the reload")
		}
	}
}

func TestSessionLinesAndPrompt(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	kit := a.Char("fm/kit")
	a.Handle(SessionMsg{Key: "fm/kit", Sess: s, OK: true, Ev: session.Event{Kind: session.EventPrompt, Entry: logstore.Entry{Text: "Name?\x07 "}}})
	if kit.Prompt != "Name? " {
		t.Errorf("Prompt = %q, want it sanitized", kit.Prompt)
	}
	ev, _, _ := a.Handle(SessionMsg{Key: "fm/kit", Sess: s, OK: true, Ev: session.Event{Kind: session.EventLine, Entry: logstore.Entry{Dir: logstore.Out, Text: "hi"}}})
	if ev.Shown || kit.Prompt == "" || len(kit.Lines) != 0 {
		t.Errorf("a sent line with local_echo off: shown %v prompt %q lines %d", ev.Shown, kit.Prompt, len(kit.Lines))
	}
	ev, _, _ = a.Handle(lineMsg("fm/kit", s, "Welcome!"))
	if !ev.Shown || kit.Prompt != "" || len(kit.Lines) != 1 || kit.Lines[0] != ev.Line {
		t.Errorf("a server line: shown %v prompt %q lines %d", ev.Shown, kit.Prompt, len(kit.Lines))
	}
	if l := a.Echo("fm/kit", logstore.Entry{Dir: logstore.Out, Text: "connect Kit ******"}); l != nil {
		t.Error("echoed a sent line with local_echo off")
	}
}

func TestEchoWithLocalEcho(t *testing.T) {
	echo := strings.Replace(fmWorld, "tls = true\n", "tls = true\nlocal_echo = true\n", 1)
	a := sessionApp(t, map[string]string{"fm": echo})
	openAll(t, a, "fm/kit")
	l := a.Echo("fm/kit", logstore.Entry{Dir: logstore.Out, Text: ":waves."})
	if kit := a.Char("fm/kit"); l == nil || l.Kind != Echo || kit.Lines[len(kit.Lines)-1] != l {
		t.Errorf("Echo = %+v", l)
	}
}
