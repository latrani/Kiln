package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/kilntest"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/scene"
	"github.com/latrani/Kiln/internal/str"
)

var logDay = time.Date(2026, 9, 24, 9, 0, 0, 0, time.Local)

// logApp is sessionApp with Kit open and its logs written: each day of
// days is that many minutes of "<day> <n>" lines, the last day today.
func logApp(t *testing.T, days ...int) *App {
	t.Helper()
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	l, _ := a.LogLayout(a.Char("fm/kit").Ch)
	for i, n := range days {
		start := logDay.AddDate(0, 0, i-len(days)+1)
		var lines []string
		for j := range n {
			lines = append(lines, fmt.Sprintf("%s %d", start.Format("01-02"), j))
		}
		kilntest.WriteLog(t, l, start, lines...)
	}
	return a
}

// drain performs Run effects as a front end would until none are left.
func drain(a *App, effs []Effect) {
	for len(effs) > 0 {
		var next []Effect
		for _, e := range effs {
			if r, ok := e.(Run); ok {
				if msg, ok := r.Func().(LogOlderMsg); ok {
					next = append(next, a.HandleLogOlder(msg)...)
				}
			}
		}
		effs = next
	}
}

func TestOpenLogLoadsWholeDaysUpFront(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	if len(g.Lines) != 300 || g.HistDone {
		t.Fatalf("loaded %d lines, done %v; want two whole days", len(g.Lines), g.HistDone)
	}
	if a.Char("fm/kit").Log != g || a.OpenLog("fm/kit") != g {
		t.Error("the log isn't the character's")
	}
}

func TestOlderRunsWhatWaitedWhenTheDayArrives(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	ran := 0
	effs := g.Older(func() []Effect { ran++; return nil })
	if len(effs) != 1 || !g.Loading {
		t.Fatalf("Older gave %v, loading %v", effs, g.Loading)
	}
	if again := g.Older(func() []Effect { ran += 10; return nil }); again != nil {
		t.Error("a second read started while one is in flight")
	}
	drain(a, effs)
	if len(g.Lines) != 450 || !g.HistDone || g.Loading || ran != 10 {
		t.Errorf("lines %d, done %v, loading %v, ran %d (the newer request replaces the older)", len(g.Lines), g.HistDone, g.Loading, ran)
	}
}

func TestLogDropsAStaleOlderRead(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	effs := g.Older(nil)
	a.CloseLog("fm/kit")
	fresh := a.OpenLog("fm/kit")
	n := len(fresh.Lines)
	drain(a, effs)
	if len(fresh.Lines) != n {
		t.Errorf("a read for the closed log landed in the new one: %d → %d", n, len(fresh.Lines))
	}
	effs = fresh.Older(nil)
	a.Close("fm/kit")
	drain(a, effs) // the character is gone: nothing to do, and no panic
}

func TestLogRelinesAPageReadUnderOldRules(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	effs := g.Older(nil)
	c := a.Char("fm/kit")
	c.Ch.Rules.Classify = append(c.Ch.Rules.Classify, config.ClassifyRule{Tag: "early", Pattern: `^09-22 `})
	if err := c.compile(); err != nil {
		t.Fatal(err)
	}
	drain(a, effs)
	if l := g.Lines[0]; len(l.Tags) != 1 || l.Tags[0] != "early" {
		t.Errorf("the oldest day wasn't classified again: %q tagged %q", l.Entry.Text, l.Tags)
	}
	if !runOf(logSeqs(g)) {
		t.Error("the relined day broke the numbering")
	}
}

func TestLogLiveDedupesAtMillisecondPrecision(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	l, _ := a.LogLayout(a.Char("fm/kit").Ch)
	base := logDay.Add(123456789 * time.Nanosecond) // not a whole millisecond
	w := logstore.NewWriter(l)
	var sent []logstore.Entry
	for i, text := range []string{"one", "two", "three"} {
		e := logstore.Entry{Time: base.Add(time.Duration(i) * time.Second), Dir: logstore.In, Text: text}
		w.Append(e)
		sent = append(sent, e)
	}
	w.Close()
	g := a.OpenLog("fm/kit")
	for _, e := range append(sent, logstore.Entry{Time: base.Add(5 * time.Second), Dir: logstore.In, Text: "four"}) {
		msg := lineMsg("fm/kit", s, e.Text)
		msg.Ev.Entry = e
		a.Handle(msg) // the same burst arrives as events after the log opened
	}
	var got []string
	for _, l := range g.Lines {
		got = append(got, l.Entry.Text)
	}
	if strings.Join(got, ",") != "one,two,three,four" {
		t.Errorf("lines = %q", got)
	}
}

func TestFindReadsOlderDaysUntilAMatch(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	var found *LogLine
	effs := g.SetFind("09-22 7", nil, func(l *LogLine) { found = l })
	if !g.Searching || g.FindStatus(nil) != str.BrowseSearching("09-22 7", DayLabel("2026-09-23")) {
		t.Fatalf("searching %v, status %q", g.Searching, g.FindStatus(nil))
	}
	drain(a, effs)
	if g.Searching || found == nil || found.Entry.Text != "09-22 79" {
		t.Errorf("searching %v, found %v", g.Searching, found)
	}
}

func TestStopSearchFindsNothing(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	called := false
	effs := g.SetFind("nowhere", nil, func(*LogLine) { called = true })
	if !g.StopSearch() || g.Searching || g.Status != str.BrowseSearchCancelled() {
		t.Fatalf("stop: searching %v, status %q", g.Searching, g.Status)
	}
	drain(a, effs) // the day still arrives, to nothing
	if called {
		t.Error("the cancelled search found something")
	}
}

func TestGotoDayShowsTheDaysFirstLine(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	var shown *LogLine
	drain(a, g.GotoTime("2026-09-22", func(l *LogLine) { shown = l }))
	if shown == nil || shown.Entry.Text != "09-22 0" {
		t.Errorf("shown %v", shown)
	}
	for _, bad := range []string{"2026/09/22", "2026-09-22 9", "2026-09-22 09:00:00", "yesterday"} {
		g.Status = ""
		g.GotoTime(bad, nil)
		if g.Status != str.BrowseDateFormat() {
			t.Errorf("%q: status %q", bad, g.Status)
		}
	}
}

// A day and time goes to the last line in or before that minute,
// paging in the day it's on; one before every line, to the oldest.
func TestGotoTimeShowsTheLineAtOrBefore(t *testing.T) {
	a := logApp(t, 150, 150, 150) // a line a minute from 09:00
	g := a.OpenLog("fm/kit")
	var shown *LogLine
	show := func(l *LogLine) { shown = l }
	for _, c := range []struct{ when, want string }{
		{"2026-09-22 09:10", "09-22 10"},     // its own minute
		{" 2026-09-22   09:10 ", "09-22 10"}, // spacing doesn't matter
		{"2026-09-23 12:00", "09-23 149"},    // after the day's last line: that line
		{"2026-09-22 08:00", "09-22 0"},      // before everything: the oldest
	} {
		shown = nil
		drain(a, g.GotoTime(c.when, show))
		if shown == nil || shown.Entry.Text != c.want {
			t.Errorf("%q went to %v, want %q", c.when, shown, c.want)
		}
	}
	if l := g.Lines[10]; WhenOf(l) != "2026-09-22 09:10" {
		t.Errorf("WhenOf = %q", WhenOf(l))
	}
}

func TestSelectionIsReceivedShownUnexcludedLines(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	l, _ := a.LogLayout(a.Char("fm/kit").Ch)
	kilntest.WriteLog(t, l, logDay, "a", "> b", "c", "d")
	g := a.OpenLog("fm/kit")
	g.Mark(g.Lines[0])
	g.Mark(g.Lines[3])
	g.ToggleExclude(g.Lines[2])
	var got []string
	for _, e := range g.Selection() {
		got = append(got, e.Text)
	}
	if strings.Join(got, ",") != "a,d" || g.Status != str.BrowseLinesInRange(4) {
		t.Errorf("selection %q, status %q", got, g.Status)
	}
	g.ToggleExclude(nil)
	if !g.StatusErr {
		t.Error("excluding nothing didn't say why not")
	}
}

func TestCopyAndExportAreEffects(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	l, _ := a.LogLayout(a.Char("fm/kit").Ch)
	kilntest.WriteLog(t, l, logDay, "a", "b")
	g := a.OpenLog("fm/kit")
	if effs := g.Copy(); effs != nil || g.Status != str.BrowseMarkRange() {
		t.Errorf("copy with no range: %v, %q", effs, g.Status)
	}
	if g.ExportReady() || g.Status != str.BrowseMarkRange() {
		t.Error("export ready with no range")
	}
	g.Mark(g.Lines[0])
	g.Mark(g.Lines[1])
	if effs := g.Copy(); len(effs) != 1 || effs[0] != (Copy{Text: scene.Plain(g.Selection())}) || g.Status != str.BrowseCopied(2) {
		t.Errorf("copy: %v, %q", effs, g.Status)
	}
	if name, ok := g.ExportFileName("txt", ""); !ok || name == "" || strings.Contains(name, "/") {
		t.Errorf("default download name %q, %v", name, ok)
	}
	if effs := g.Export("txt", "  "); effs != nil || g.Status != str.BrowseNoFileName() {
		t.Errorf("empty name: %v, %q", effs, g.Status)
	}
	effs := g.Export("txt", " scene.txt ")
	if len(effs) != 1 {
		t.Fatalf("export: %v", effs)
	}
	if s, ok := effs[0].(SaveFile); !ok || s.Key != "fm/kit" || s.Name != "scene.txt" || string(s.Data) != scene.Render("txt", g.Selection(), g.Title()) {
		t.Errorf("SaveFile = %+v", effs[0])
	}
}

func TestFindStepsFromTheLineItsGiven(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	l, _ := a.LogLayout(a.Char("fm/kit").Ch)
	kilntest.WriteLog(t, l, logDay, "x 1", "y", "x 2", "x 3")
	g := a.OpenLog("fm/kit")
	var found *LogLine
	at := func(l *LogLine) { found = l }
	g.SetFind("x", nil, at) // nowhere yet: from the newest
	if found != g.Lines[3] {
		t.Fatalf("found %v; want the newest match", found)
	}
	g.FindOlder(g.Lines[3], false, at)
	if found != g.Lines[2] {
		t.Errorf("older than x 3: %v", found)
	}
	if got := g.FindNewer(g.Lines[0]); got != g.Lines[2] {
		t.Errorf("newer than x 1: %v", got)
	}
	if got := g.FindNewer(g.Lines[3]); got != nil || g.Status != str.BrowseNoNewerMatches("x") {
		t.Errorf("newer than the newest: %v, %q", got, g.Status)
	}
	if s := g.FindStatus(g.Lines[2]); s != str.BrowseFindStatus("x", 2, 3) {
		t.Errorf("status at x 2: %q", s)
	}
}

func TestMarkWithNoLineDoesNothing(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	g := a.OpenLog("fm/kit")
	g.Mark(nil)
	if g.Start != nil || g.Status != "" {
		t.Errorf("start %v, status %q", g.Start, g.Status)
	}
}

// logSeqs is g's line numbers, in order.
func logSeqs(g *Log) []int {
	var out []int
	for _, l := range g.Lines {
		out = append(out, l.Seq)
	}
	return out
}

func TestLogNumbersFollowTheLines(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	if got := logSeqs(g); !runOf(got) || got[len(got)-1] != -1 {
		t.Fatalf("loaded %d lines numbered %v…; want one run ending at -1", len(got), got[:3])
	}
	lowest := g.Lines[0].Seq
	drain(a, g.Older(nil))
	if got := logSeqs(g); !runOf(got) || g.Lines[0].Seq >= lowest {
		t.Errorf("after an older day: lowest %d (was %d), run %v", g.Lines[0].Seq, lowest, runOf(got))
	}
	g.appendLive(logstore.Entry{Time: time.Now(), Dir: logstore.In, Text: "live"})
	if l := g.Last(); l.Seq != 0 {
		t.Errorf("a live line: %d; want 0", l.Seq)
	}
}

func TestNewRangeOrdersItsEnds(t *testing.T) {
	a := logApp(t, 3)
	g := a.OpenLog("fm/kit")
	g.NewRange(g.Lines[2], g.Lines[0])
	if g.Start != g.Lines[0] || g.End != g.Lines[2] || len(g.Selection()) != 3 {
		t.Errorf("start %v end %v", g.Start, g.End)
	}
}

func TestLineTagsSaysWhoseStyleEachTakes(t *testing.T) {
	styled := func(tag string) (string, bool) {
		switch tag {
		case "page":
			return "page", true
		case "page/ooc":
			return "page", true
		}
		return "", false
	}
	if got := LineTags(nil, styled); got != "" {
		t.Errorf("no tags: %q", got)
	}
	want := str.BrowseLineTags(strings.Join([]string{"page", str.BrowseTagAs("page/ooc", "page"), str.BrowseTagUnstyled("x")}, str.ListSeparator()))
	if got := LineTags([]string{"page", "page/ooc", "x"}, styled); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
