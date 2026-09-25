package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
)

var day24 = time.Date(2026, 9, 24, 21, 0, 0, 0, time.Local)

// writeLog puts entries in fm/kit's log, one minute apart from start.
func (h *harness) writeLog(start time.Time, lines ...string) {
	h.t.Helper()
	w := logstore.NewWriter(h.m.d.LogRoot, "fm", "kit")
	defer w.Close()
	for i, l := range lines {
		dir := logstore.In
		if strings.HasPrefix(l, "> ") {
			dir, l = logstore.Out, strings.TrimPrefix(l, "> ")
		}
		if err := w.Append(logstore.Entry{Time: start.Add(time.Duration(i) * time.Minute), Dir: dir, Text: l}); err != nil {
			h.t.Fatal(err)
		}
	}
}

func (h *harness) key(s string) tea.Cmd {
	var k tea.KeyPressMsg
	switch s {
	case "esc":
		k = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		k = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "up":
		k = tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		k = tea.KeyPressMsg{Code: tea.KeyDown}
	case "home":
		k = tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		k = tea.KeyPressMsg{Code: tea.KeyEnd}
	case "space":
		k = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "ctrl+b":
		k = tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl}
	default:
		r := []rune(s)[0]
		k = tea.KeyPressMsg{Code: r, Text: s}
	}
	_, cmd := h.m.Update(k)
	return h.drainLoads(cmd)
}

// drainLoads runs history loads the way Bubble Tea would, feeding each
// result back in, until browse stops loading. Other commands are returned.
func (h *harness) drainLoads(cmd tea.Cmd) tea.Cmd {
	for cmd != nil {
		if b := h.br(); b == nil || !b.loading {
			break
		}
		_, cmd = h.m.Update(cmd())
	}
	return cmd
}

func (h *harness) keys(ss ...string) {
	for _, s := range ss {
		h.key(s)
	}
}

func (h *harness) br() *browse { return h.m.chars["fm/kit"].browse }

var scene1 = []string{
	"Rook says, \"Evening!\"",
	"Sable waves a paw.",
	"Mira pages: you around?",
	"> :grins.",
	"Kit grins.",
	"Rook says, \"The lighthouse is dark.\"",
	"Rook yawns.",
}

func TestBrowseOpensAndCloses(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	s := h.screen()
	for _, want := range []string{"BROWSE Kit · Thu Sep 24 → today", "── Thu Sep 24 ──", "21:00", "Rook says", "21:06", "m mark"} {
		if !strings.Contains(s, want) {
			t.Errorf("screen missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "\n> ") || strings.Contains(s, "│> ") {
		t.Error("normal input still shown in browse mode")
	}
	h.key("esc")
	if h.br() != nil || strings.Contains(h.screen(), "BROWSE") {
		t.Errorf("esc did not close browse:\n%s", h.screen())
	}
}

func TestBrowseKeepsStatusline(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	rows := strings.Split(h.screen(), "\n")
	if len(rows) != 24 {
		t.Fatalf("screen has %d rows, want 24", len(rows))
	}
	last := func() string {
		rows := strings.Split(h.screen(), "\n")
		return strings.TrimSpace(strings.SplitN(rows[len(rows)-1], "│", 2)[1])
	}
	if got := last(); got != "BROWSE · 0 selected · fm/Kit 🔒 · disconnected · 21:14" {
		t.Errorf("statusline = %q", got)
	}
	if !strings.Contains(rows[len(rows)-2], "m mark") {
		t.Errorf("action bar should sit just above the statusline:\n%s", h.screen())
	}
	h.keys("m", "up", "m")
	if got := last(); !strings.HasPrefix(got, "BROWSE · 2 selected · ") {
		t.Errorf("statusline = %q", got)
	}
	h.m.setStatus(true, "Rook: log write failed: disk full") // e.g. another character's event
	if got := last(); !strings.Contains(got, "log write failed") {
		t.Errorf("status message hidden in browse: %q", got)
	}
	// The body still ends above the action bar: the newest line is visible.
	h.key("end")
	if !strings.Contains(h.screen(), "Rook yawns.") {
		t.Errorf("newest line cut off:\n%s", h.screen())
	}
}

func TestBrowseMarkExcludeExport(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	exportDir := t.TempDir()
	h.m.exportDir = exportDir
	h.key("ctrl+b")
	// Cursor starts on the last line (Rook yawns). Mark 21:01..21:05.
	h.keys("up", "m")                   // Rook says lighthouse = end
	h.keys("up", "up", "up", "up", "m") // Sable = start (marks may go either way)
	h.keys("down", "space")             // exclude the page
	b := h.br()
	var got []string
	for _, e := range b.selection() {
		got = append(got, e.Text)
	}
	want := "Sable waves a paw.|Kit grins.|Rook says, \"The lighthouse is dark.\""
	if strings.Join(got, "|") != want {
		t.Errorf("selection = %q, want %q (page excluded, sent line dropped)", strings.Join(got, "|"), want)
	}
	s := h.screen()
	if !strings.Contains(s, glyphExcluded+" Mira pages") || !strings.Contains(s, glyphSelected+" Sable") {
		t.Errorf("gutter glyphs missing:\n%s", s)
	}

	h.keys("e", "h")
	if !strings.Contains(h.screen(), "save as: ") || !strings.Contains(h.screen(), "fm Kit.html") {
		t.Fatalf("filename prompt missing:\n%s", h.screen())
	}
	h.key("enter")
	path := filepath.Join(exportDir, "2026-09-24 2101 fm Kit.html")
	b2, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("export not written: %v\n%s", err, h.screen())
	}
	if !strings.Contains(string(b2), "Sable waves a paw.") || strings.Contains(string(b2), "Mira") {
		t.Errorf("export content wrong:\n%s", b2)
	}
	h.keys("e", "h", "enter")
	if !strings.Contains(h.screen(), "exists; pick another name") {
		t.Errorf("overwrite not refused:\n%s", h.screen())
	}
}

func TestBrowseExportNeedsRange(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	h.key("e")
	if !strings.Contains(h.screen(), "mark a range with m") {
		t.Errorf("screen:\n%s", h.screen())
	}
	if cmd := h.key("c"); cmd != nil {
		t.Error("copy without range should do nothing")
	}
}

func TestBrowseCopy(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	h.keys("m", "up", "m")
	if cmd := h.key("c"); cmd == nil {
		t.Fatal("copy returned no command")
	}
	if !strings.Contains(h.screen(), "copied 2 lines") {
		t.Errorf("screen:\n%s", h.screen())
	}
}

func TestBrowseChips(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	s := h.screen()
	if !strings.Contains(s, "tags: 1[page] 2[say] 3[self]") {
		t.Fatalf("chips:\n%s", s)
	}
	h.key("1") // page → only
	s = h.screen()
	if !strings.Contains(s, "1[+page]") || strings.Contains(s, "Sable") || !strings.Contains(s, "Mira pages") {
		t.Errorf("only-page filter wrong:\n%s", s)
	}
	h.key("1") // page → hide
	s = h.screen()
	if !strings.Contains(s, "1[−page]") || strings.Contains(s, "Mira pages") || !strings.Contains(s, "Sable") {
		t.Errorf("hide-page filter wrong:\n%s", s)
	}
	h.key("1") // neutral
	if !strings.Contains(h.screen(), "Mira pages") {
		t.Error("neutral chip should show everything")
	}
}

func TestBrowseHidingCursorLineKeepsPlace(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	h.keys("home", "down", "down") // Mira pages
	b := h.br()
	if b.cursor.e.Text != "Mira pages: you around?" {
		t.Fatalf("cursor on %q", b.cursor.e.Text)
	}
	h.keys("1", "1") // page → only → hide
	if got := b.cursor.e.Text; got != "> :grins." && got != ":grins." {
		t.Errorf("cursor moved to %q, want the next line", got)
	}
	h.keys("1", "home", "down", "down", "down", "down") // neutral; Kit grins. (self)
	h.keys("3", "3")                                    // self → only → hide
	if got := b.cursor.e.Text; got != "Rook says, \"The lighthouse is dark.\"" {
		t.Errorf("cursor moved to %q, want the next visible line", got)
	}
	// With nothing visible after it, fall back to the previous line.
	h.keys("3", "end", "1") // self neutral; Rook yawns; page → only
	if got := b.cursor.e.Text; got != "Mira pages: you around?" {
		t.Errorf("cursor moved to %q, want the previous visible line", got)
	}
}

func TestBrowseChipNumbersStayStable(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, "Rook says, \"Hi, Kit.\"")
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.key("ctrl+b")
	if s := h.screen(); !strings.Contains(s, "tags: 1[say] 2[self]") {
		t.Fatalf("chips:\n%s", s)
	}
	h.conn("fm/kit").lines <- "Mira pages: you around?"
	h.settle("fm/kit", func() bool { return strings.Contains(h.screen(), "Mira pages") })
	if s := h.screen(); !strings.Contains(s, "tags: 1[say] 2[self] 3[page]") {
		t.Errorf("new tag renumbered chips:\n%s", s)
	}
	h.key("1")
	if b := h.br(); b.chips["say"] == 0 || b.chips["page"] != 0 {
		t.Errorf("1 should still target say: %v", b.chips)
	}
}

func TestBrowseFind(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	h.keys("home") // cursor to the oldest line
	h.key("/")
	h.typeText("rook")
	h.key("enter")
	b := h.br()
	if b.cursor.e.Text != scene1[0] || !strings.Contains(h.screen(), "find: rook 1/3") {
		t.Errorf("first match wrong: %q\n%s", b.cursor.e.Text, h.screen())
	}
	h.key("n")
	if b.cursor.e.Text != scene1[5] {
		t.Errorf("n went to %q", b.cursor.e.Text)
	}
	h.key("N")
	if b.cursor.e.Text != scene1[0] {
		t.Errorf("N went to %q", b.cursor.e.Text)
	}
	if !strings.Contains(h.m.View().Content, reverse+"Rook") {
		t.Error("matches not highlighted")
	}
	if !strings.Contains(h.screen(), "Sable") {
		t.Error("find must not hide lines")
	}
}

func TestBrowseLoadsOlderDaysOffTheUIGoroutine(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	for d := 21; d <= 24; d++ {
		lines := make([]string, 150)
		for i := range lines {
			lines[i] = fmt.Sprintf("day %d line %d", d, i)
		}
		h.writeLog(time.Date(2026, 9, d, 8, 0, 0, 0, time.Local), lines...)
	}
	h.key("ctrl+b")
	b := h.br()
	_, cmd := h.m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	if cmd == nil || !b.loading {
		t.Fatal("Home should start a background load")
	}
	if len(b.lines) != 300 || b.cursor.e.Text != "day 23 line 0" {
		t.Errorf("Update must not load synchronously: %d lines, cursor %q", len(b.lines), b.cursor.e.Text)
	}
	if !strings.Contains(h.screen(), "loading older history") {
		t.Errorf("no loading row:\n%s", h.screen())
	}
	// A second Home while loading doesn't start another read.
	if _, cmd2 := h.m.Update(tea.KeyPressMsg{Code: tea.KeyHome}); cmd2 != nil {
		t.Error("second load started while one is in flight")
	}
	// The first day arrives; Home keeps paging until history runs out.
	_, cmd = h.m.Update(cmd())
	if len(b.lines) != 450 || cmd == nil {
		t.Fatalf("after one day: %d lines, next cmd %v", len(b.lines), cmd != nil)
	}
	h.drainLoads(cmd)
	if b.loading || !b.histDone || b.cursor.e.Text != "day 21 line 0" {
		t.Errorf("loading=%v done=%v cursor=%q", b.loading, b.histDone, b.cursor.e.Text)
	}
	if strings.Contains(h.screen(), "loading older history") {
		t.Errorf("loading row left behind:\n%s", h.screen())
	}
}

func TestBrowseDropsLoadForClosedBrowse(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24.AddDate(0, 0, -1), "old line")
	h.writeLog(day24, make([]string, 250)...)
	h.key("ctrl+b")
	_, cmd := h.m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	if cmd == nil {
		t.Fatal("no load started")
	}
	h.key("esc")
	h.key("ctrl+b") // a new browse
	fresh := h.br()
	n := len(fresh.lines)
	h.m.Update(cmd()) // the old browse's day arrives late
	if len(fresh.lines) != n {
		t.Errorf("stale load changed the new browse: %d → %d lines", n, len(fresh.lines))
	}
}

func TestBrowsePagesOlderDaysAndDateJump(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	for d := 21; d <= 24; d++ {
		lines := make([]string, 150)
		for i := range lines {
			lines[i] = fmt.Sprintf("day %d line %d", d, i)
		}
		h.writeLog(time.Date(2026, 9, d, 8, 0, 0, 0, time.Local), lines...)
	}
	h.key("ctrl+b")
	b := h.br()
	if len(b.lines) != 300 {
		t.Errorf("initially loaded %d lines, want 300 (two days)", len(b.lines))
	}
	if !strings.Contains(h.screen(), "…Wed Sep 23 → today") {
		t.Errorf("header should show more history exists:\n%s", h.screen())
	}
	h.key("g")
	h.typeText("2026-09-21")
	h.key("enter")
	if b.cursor.e.Text != "day 21 line 0" {
		t.Errorf("date jump landed on %q", b.cursor.e.Text)
	}
	if !strings.Contains(h.screen(), "Mon Sep 21 → today") || strings.Contains(h.screen(), "…Mon") {
		t.Errorf("header after loading everything:\n%s", h.screen())
	}
	h.key("g")
	h.typeText("yesterday")
	h.key("enter")
	if !strings.Contains(h.screen(), "dates look like") {
		t.Errorf("bad date not reported:\n%s", h.screen())
	}
}

func TestBrowseMarksSurvivePaging(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(time.Date(2026, 9, 23, 8, 0, 0, 0, time.Local), make([]string, 250)...)
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	b := h.br()
	h.keys("m", "up", "m")
	start, end := b.start, b.end
	h.key("home") // loads everything older
	if b.start != start || b.end != end || len(b.selection()) != 2 {
		t.Error("marks moved when older history loaded")
	}
}

func TestBrowseLiveLinesArrive(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.key("ctrl+b")
	h.conn("fm/kit").lines <- "Brand new line"
	h.settle("fm/kit", func() bool { return strings.Contains(h.screen(), "Brand new line") })
	if h.br().cursor.e.Text != "Brand new line" {
		t.Error("cursor at the bottom should follow live lines")
	}
}

func TestBrowseMouse(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	h.screen()
	b := h.br()
	l := h.m.layout()
	rowOf := func(text string) int {
		for i, bl := range b.rowLines {
			if bl != nil && bl.e.Text == text {
				return i + 3
			}
		}
		t.Fatalf("%q not on screen", text)
		return 0
	}
	x := l.sw + 1 + 10
	h.m.Update(tea.MouseClickMsg{X: x, Y: rowOf(scene1[1]), Button: tea.MouseLeft})
	if b.cursor.e.Text != scene1[1] {
		t.Errorf("click moved cursor to %q", b.cursor.e.Text)
	}
	h.m.Update(tea.MouseClickMsg{X: x, Y: rowOf(scene1[4]), Button: tea.MouseLeft, Mod: tea.ModShift})
	if b.start == nil || b.end == nil || b.start.e.Text != scene1[1] || b.end.e.Text != scene1[4] {
		t.Fatalf("shift-click range wrong")
	}
	h.screen()
	h.m.Update(tea.MouseClickMsg{X: l.sw + 1 + browsePrefixW - 2, Y: rowOf(scene1[2]), Button: tea.MouseLeft})
	if !b.excluded[b.lines[2]] {
		t.Error("gutter click did not exclude")
	}
	h.m.Update(tea.MouseClickMsg{X: l.sw + 1 + b.chipSpans[0].from, Y: 1, Button: tea.MouseLeft})
	if b.chips["page"] == 0 {
		t.Error("chip click did not cycle")
	}
}

func TestHighlightCommand(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("/highlight the lighthouse")
	h.enter()
	if !strings.Contains(h.screen(), `added highlight for "the lighthouse"`) {
		t.Fatalf("screen:\n%s", h.screen())
	}
	h.m.Update(reloadMsg{})
	cs := h.m.chars["fm/kit"]
	text, _ := cs.render(logstore.Entry{Dir: logstore.In, Text: "Rook: The Lighthouse is dark."})
	if !strings.Contains(text, "\x1b[1;38;2;255;209;102m") {
		t.Errorf("new rule not applied: %q", text)
	}
	h.typeText("/highlight")
	h.enter()
	if !strings.Contains(h.screen(), "nothing to highlight") {
		t.Errorf("screen:\n%s", h.screen())
	}
}

func TestBrowseCommandAndSwitching(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook")
	h.typeText("/browse")
	h.enter()
	if h.br() == nil || !strings.Contains(h.screen(), "no logs yet") {
		t.Fatalf("screen:\n%s", h.screen())
	}
	h.press(tea.KeyDown, tea.ModCtrl) // switch to Rook: normal view
	if strings.Contains(h.screen(), "BROWSE") {
		t.Error("rook should not be in browse mode")
	}
	h.press(tea.KeyUp, tea.ModCtrl)
	if !strings.Contains(h.screen(), "BROWSE Kit") {
		t.Error("kit's browse state was lost")
	}
	_ = session.Connected
}

func TestPromptWindow(t *testing.T) {
	cases := []struct {
		in       string
		col, w   int
		want     string
		wantCurX int
	}{
		{"short", 5, 20, "short", 5},
		{"/a/very/long/path/Scene.html", 28, 12, "…Scene.html", 11},
		{"/a/very/long/path/Scene.html", 0, 12, "/a/very/long", 0}, // cursor at start: text may fill every cell
		{"日本語日本語", 6, 7, "…本語", 5},                                 // "…" + 4 cells + the cursor's cell = 6 ≤ 7
	}
	for _, c := range cases {
		got, x := promptWindow([]rune(c.in), c.col, c.w)
		if got != c.want || x != c.wantCurX {
			t.Errorf("promptWindow(%q, %d, %d) = %q, %d; want %q, %d", c.in, c.col, c.w, got, x, c.want, c.wantCurX)
		}
	}
}

func TestHighlightFindSurvivesCaseFolding(t *testing.T) {
	for _, c := range []struct{ plain, needle string }{
		{"ȺȺȺȺȺȺ x", "x"},   // Ⱥ lowercases to a longer encoding
		{"İİİ foo", "foo"},  // İ lowercases to a different length
		{"KELVIN K x", "x"}, // Kelvin sign
		{"Straße STRASSE", "ß"},
	} {
		got := highlightFind(c.plain, c.needle)
		if !utf8.ValidString(got) {
			t.Errorf("highlightFind(%q, %q) = %q: invalid UTF-8", c.plain, c.needle, got)
		}
		if ansi.Strip(got) != c.plain {
			t.Errorf("highlightFind(%q, %q) changed the text: %q", c.plain, c.needle, ansi.Strip(got))
		}
		if !strings.Contains(got, reverse+c.needle) {
			t.Errorf("highlightFind(%q, %q) = %q: match not highlighted", c.plain, c.needle, got)
		}
	}
}

func TestBrowseLiveDedupeAtMillisecondPrecision(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	base := day24.Add(123456789 * time.Nanosecond) // not a whole millisecond
	w := logstore.NewWriter(h.m.d.LogRoot, "fm", "kit")
	var sent []logstore.Entry
	for i, text := range []string{"one", "two", "three"} {
		e := logstore.Entry{Time: base.Add(time.Duration(i) * time.Second), Dir: logstore.In, Text: text}
		w.Append(e)
		sent = append(sent, e)
	}
	w.Close()
	h.key("ctrl+b")
	b := h.br()
	for _, e := range sent { // the same burst arrives as events after browse opened
		b.appendLive(e)
	}
	b.appendLive(logstore.Entry{Time: base.Add(5 * time.Second), Dir: logstore.In, Text: "four"})
	var got []string
	for _, l := range b.lines {
		got = append(got, l.e.Text)
	}
	if strings.Join(got, ",") != "one,two,three,four" {
		t.Errorf("lines = %q", got)
	}
}

func TestChipClickDuringFormatPromptDoesNotCrash(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	h.keys("up", "m", "up", "up", "m") // Sable..Kit grins region
	h.key("e")
	h.screen()
	b := h.br()
	for _, c := range b.chipSpans {
		if c.tag == "page" { // clicking would make "page only" hide the whole selection
			h.m.Update(tea.MouseClickMsg{X: h.m.layout().sw + 1 + c.from, Y: 1, Button: tea.MouseLeft})
		}
	}
	h.key("p") // must not panic
	if b.chips["page"] != 0 {
		t.Error("clicks should be ignored while a prompt is open")
	}
}

func TestBrowseFindIsFastOnLargeHistories(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.key("ctrl+b")
	b := h.br()
	for i := 0; i < 50000; i++ {
		b.lines = append(b.lines, &bline{e: logstore.Entry{Time: day24, Dir: logstore.In, Text: fmt.Sprintf("the line %d", i)}, text: fmt.Sprintf("the line %d", i), day: "2026-09-24"})
	}
	b.cursor = b.lines[len(b.lines)-1]
	b.find = "the"
	start := time.Now()
	for i := 0; i < 10; i++ {
		b.jumpMatch(1, false) // wraps around every time from the end
		b.cursor = b.lines[len(b.lines)-1]
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("10 find jumps on 50k lines took %v", d)
	}
}

func TestPasteInBrowse(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	h.m.Update(tea.PasteMsg{Content: "stray"})
	if v := h.m.chars["fm/kit"].in.Value(); v != "" {
		t.Errorf("paste leaked into the chat draft: %q", v)
	}
	h.key("/")
	h.m.Update(tea.PasteMsg{Content: "lighthouse"})
	h.key("enter")
	if h.br().find != "lighthouse" {
		t.Errorf("paste into find prompt: find = %q", h.br().find)
	}
}

func TestSaveExpandsHomeAndRelativePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.m.exportDir = filepath.Join(home, "scenes")
	h.key("ctrl+b")
	h.keys("m", "up", "m")
	b := h.br()
	b.format = "plain"
	b.save("~/Desktop/a.txt")
	if _, err := os.Stat(filepath.Join(home, "Desktop", "a.txt")); err != nil {
		t.Errorf("~ not expanded: %v (%s)", err, b.status)
	}
	b.save("b.txt")
	if _, err := os.Stat(filepath.Join(home, "scenes", "b.txt")); err != nil {
		t.Errorf("relative name not placed in export_dir: %v (%s)", err, b.status)
	}
	b.save("  ")
	if !strings.Contains(b.status, "file name") {
		t.Errorf("empty name status = %q", b.status)
	}
}

func TestSaveWithoutExportDirRefusesRelativePath(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.m.exportDir = ""
	h.key("ctrl+b")
	h.keys("m", "up", "m")
	b := h.br()
	b.format = "plain"
	b.save("scene.txt")
	if !strings.Contains(b.status, "no export_dir") {
		t.Errorf("status = %q", b.status)
	}
	if _, err := os.Stat("scene.txt"); err == nil {
		os.Remove("scene.txt")
		t.Error("wrote into the working directory")
	}
}

func TestReloadUpdatesOpenBrowseExportDir(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.key("ctrl+b")
	dir := t.TempDir()
	os.WriteFile(filepath.Join(h.dir, "config.toml"), []byte("export_dir = \""+dir+"\"\n"), 0o600)
	h.m.Update(reloadMsg{})
	if h.br().exportDir != dir {
		t.Errorf("open browse exportDir = %q, want %q", h.br().exportDir, dir)
	}
}
