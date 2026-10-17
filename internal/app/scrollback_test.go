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
	if got := seqs(kit.Lines); !runOf(got) || got[len(got)-1] != -1 {
		t.Errorf("after an older page: %v; want one run ending at -1", got)
	}
	a.Preload("fm/kit") // a new reader: a page from the old one is stale
	if _, ok := a.HandleOlder(msg); ok {
		t.Error("took a page from a replaced reader")
	}
}

// The input's history starts with what the logs say was sent, but
// logins; older days are read as its panel needs them.
func TestHistoryFromTheLogs(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	writeLog(t, a, day23.Add(-24*time.Hour), "> ancient")
	lines := []string{"> connect Kit ***", "> /sent"}
	for range HistoryLines {
		lines = append(lines, "line")
	}
	writeLog(t, a, day23, lines...)
	openAll(t, a, "fm/kit")
	kit := a.Char("fm/kit")
	if got := kit.History.Lines(); !slices.Equal(got, []string{"//sent"}) {
		t.Fatalf("history %q; want the preloaded day's sent line as typed, not the login", got)
	}
	effs := a.HistoryOpen()
	if len(effs) != 1 || a.HistoryOpen() != nil {
		t.Fatalf("HistoryOpen = %v; want one read, and no second while it's in flight", effs)
	}
	msg := effs[0].(Run).Func().(SentOlderMsg)
	if more := a.HandleSentOlder(msg); more != nil {
		t.Errorf("read on past the oldest day: %v", more)
	}
	if got := texts(kit.History.Shown()); !slices.Equal(got, []string{"ancient", "//sent"}) {
		t.Errorf("shown %q", got)
	}
	if s := kit.History.Shown()[0]; !s.At.Equal(day23.Add(-24 * time.Hour)) {
		t.Errorf("ancient sent at %v; want its logged time", s.At)
	}
	if a.HandleSentOlder(msg); len(kit.History.Lines()) != 2 {
		t.Error("took a day from a reader that's done")
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
	before := seqs(a.Char("fm/kit").Lines)
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
	if got := seqs(a.Char("fm/kit").Lines); !slices.Equal(got[len(got)-len(before):], before) || !runOf(got) {
		t.Errorf("numbers after the reload and the page: %v; the preloaded %v should keep theirs", got, before)
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

// "Last seen" is the last line that arrived: what you sent doesn't move
// it, whether it's from the log or sent just now.
func TestLastTimeIgnoresWhatYouSent(t *testing.T) {
	echo := strings.Replace(fmWorld, "tls = true\n", "tls = true\nlocal_echo = true\n", 1)
	a := sessionApp(t, map[string]string{"fm": echo})
	writeLog(t, a, day23, "one", "> sent")
	openAll(t, a, "fm/kit")
	if at, ok := a.LastTime("fm/kit"); !ok || !at.Equal(day23) {
		t.Errorf("after the preload: LastTime = %v, %v; want the received line's", at, ok)
	}
	a.Echo("fm/kit", logstore.Entry{Time: day23.Add(time.Hour), Dir: logstore.Out, Text: "say hi"})
	if at, _ := a.LastTime("fm/kit"); !at.Equal(day23) {
		t.Errorf("after sending: LastTime = %v; want it unmoved", at)
	}
}

// seqs is ls's numbers, in order.
func seqs(ls []*Line) []int {
	var out []int
	for _, l := range ls {
		out = append(out, l.Seq)
	}
	return out
}

// runOf reports whether ns counts up by one.
func runOf(ns []int) bool {
	for i := 1; i < len(ns); i++ {
		if ns[i] != ns[i-1]+1 {
			return false
		}
	}
	return true
}

func TestScrollbackNumbersFollowTheLines(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	writeLog(t, a, day23, "one", "two")
	openAll(t, a, "fm/kit")
	kit := a.Char("fm/kit")
	if got := seqs(kit.Lines); !slices.Equal(got, []int{-4, -3, -2, -1}) { // day, one, two, end of history
		t.Fatalf("preloaded %v", got)
	}
	s := attach(a, "fm/kit")
	a.Handle(lineMsg("fm/kit", s, "Welcome!"))
	a.Echo("fm/kit", logstore.Entry{Dir: logstore.Out, Text: "hi"}) // local_echo off: not a line
	kit.Ch.LocalEcho = true
	a.Echo("fm/kit", logstore.Entry{Dir: logstore.Out, Text: "hi"})
	if got := seqs(kit.Lines); !slices.Equal(got, []int{-4, -3, -2, -1, 0, 1}) {
		t.Errorf("after a line and an echo: %v", got)
	}
	a.Preload("fm/kit") // again: below everything so far
	got := seqs(kit.Lines)
	if !runOf(got) || got[len(got)-1] != -5 {
		t.Errorf("preloaded again: %v; want a run ending at -5", got)
	}
	a.Handle(lineMsg("fm/kit", s, "Again"))
	if l := kit.Lines[len(kit.Lines)-1]; l.Seq != 2 {
		t.Errorf("a live line after: %d; want 2", l.Seq)
	}
}

// In session scope the panel lists no logged lines, so it reads no
// older days; back in character scope it reads on as it needs them.
func TestSessionScopeReadsNoLogs(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	writeLog(t, a, day23.Add(-24*time.Hour), "> ancient")
	lines := []string{"> sent"}
	for range HistoryLines {
		lines = append(lines, "line")
	}
	writeLog(t, a, day23, lines...)
	openAll(t, a, "fm/kit")
	a.SwapHistoryScope()
	if effs := a.HistoryOpen(); effs != nil {
		t.Errorf("HistoryOpen in session scope = %v; want no log read", effs)
	}
	if effs := a.HistoryMore(); effs != nil {
		t.Errorf("HistoryMore in session scope = %v; want no log read", effs)
	}
	if effs := a.SwapHistoryScope(); len(effs) != 1 {
		t.Errorf("back to character scope: %v; want a read", effs)
	}
}
