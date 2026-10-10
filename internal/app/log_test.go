package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/kilntest"
	"github.com/latrani/Kiln/internal/logstore"
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
	if g.Cursor != g.Lines[len(g.Lines)-1] || a.Char("fm/kit").Log != g || a.OpenLog("fm/kit") != g {
		t.Error("cursor not on the newest line, or the log isn't the character's")
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
	effs := g.SetFind("09-22 7")
	if !g.Searching || g.FindStatus() != str.BrowseSearching("09-22 7", DayLabel("2026-09-23")) {
		t.Fatalf("searching %v, status %q", g.Searching, g.FindStatus())
	}
	drain(a, effs)
	if g.Searching || g.Cursor == nil || g.Cursor.Entry.Text != "09-22 79" {
		t.Errorf("searching %v, cursor %v", g.Searching, g.Cursor)
	}
}

func TestStopSearchLeavesTheCursor(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	cursor := g.Cursor
	effs := g.SetFind("nowhere")
	if !g.StopSearch() || g.Searching || g.Cursor != cursor || g.Status != str.BrowseSearchCancelled() {
		t.Fatalf("stop: searching %v, status %q", g.Searching, g.Status)
	}
	drain(a, effs) // the day still arrives, to nothing
	if g.Cursor != cursor {
		t.Error("the cancelled search moved the cursor")
	}
}

func TestGotoDateShowsTheDaysFirstLine(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	var shown *LogLine
	drain(a, g.GotoDate("2026-09-22", func(l *LogLine) { shown = l }))
	if shown == nil || shown != g.Cursor || shown.Entry.Text != "09-22 0" {
		t.Errorf("shown %v, cursor %v", shown, g.Cursor)
	}
	g.GotoDate("2026/09/22", nil)
	if g.Status != str.BrowseDateFormat() {
		t.Errorf("bad date: %q", g.Status)
	}
}

func TestSelectionIsReceivedShownUnexcludedLines(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	l, _ := a.LogLayout(a.Char("fm/kit").Ch)
	kilntest.WriteLog(t, l, logDay, "a", "> b", "c", "d")
	g := a.OpenLog("fm/kit")
	g.SetCursor(g.Lines[0])
	g.Mark()
	g.SetCursor(g.Lines[3])
	g.Mark()
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
