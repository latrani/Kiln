package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/scene"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

var day24 = time.Date(2026, 9, 24, 21, 0, 0, 0, time.Local)

// writeLog puts entries in fm/kit's log, one minute apart from start.
func (h *harness) writeLog(start time.Time, lines ...string) {
	h.t.Helper()
	w := logstore.NewWriter(logstore.Layout{Root: h.m.d.LogRoot, World: "fm", Char: "kit", CharName: "Kit"})
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
	case "left":
		k = tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		k = tea.KeyPressMsg{Code: tea.KeyRight}
	case "delete":
		k = tea.KeyPressMsg{Code: tea.KeyDelete}
	case "ctrl+l":
		k = tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl}
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
	h.key("ctrl+l")
	s := h.screen()
	for _, want := range []string{str.BrowseLog() + " Kit" + str.Separator() + str.BrowseToToday("Thu Sep 24"), "── Thu Sep 24 ──", "21:00", "Rook says", "21:06", firstPart(str.BrowseHints())} {
		if !strings.Contains(s, want) {
			t.Errorf("screen missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "\n> ") || strings.Contains(s, "│> ") {
		t.Error("normal input still shown in browse mode")
	}
	h.key("esc")
	if h.br() != nil || strings.Contains(h.screen(), "LOG") {
		t.Errorf("esc did not close browse:\n%s", h.screen())
	}
}

func TestBrowseKeepsStatusline(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	rows := strings.Split(h.screen(), "\n")
	if len(rows) != 24 {
		t.Fatalf("screen has %d rows, want 24", len(rows))
	}
	sep := str.Separator()
	last := func() string {
		rows := strings.Split(h.screen(), "\n")
		return strings.TrimSpace(strings.SplitN(rows[len(rows)-1], "│", 2)[1])
	}
	if got := last(); !strings.HasPrefix(got, "fm/Kit  "+str.ViewLogButton()+" "+sep+str.StateDisconnected()+sep+str.ViewSelected(0)+" ") || !strings.HasSuffix(got, " 21:14") {
		t.Errorf("statusline = %q", got)
	}
	if !strings.Contains(rows[len(rows)-2], firstPart(str.BrowseHints())) {
		t.Errorf("action bar should sit just above the statusline:\n%s", h.screen())
	}
	h.keys("m", "up", "m")
	if got := last(); !strings.Contains(got, sep+str.ViewSelected(2)+" ") {
		t.Errorf("statusline = %q", got)
	}
	msg := str.StatusLogWriteFailed("R", errors.New("x")) // e.g. another character's event
	h.m.setStatus(true, msg)
	if got := last(); !strings.Contains(got, msg) {
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
	h.m.cfg.ExportDir = exportDir
	h.key("ctrl+l")
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
	if !strings.Contains(h.screen(), str.BrowseSavePrompt()) || !strings.Contains(h.screen(), "fm Kit.html") {
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
	if !strings.Contains(h.screen(), str.BrowseFileExists(filepath.Base(path))) {
		t.Errorf("overwrite not refused:\n%s", h.screen())
	}
}

func TestBrowseExportNeedsRange(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
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
	h.key("ctrl+l")
	h.keys("m", "up", "m")
	if cmd := h.key("c"); cmd == nil {
		t.Fatal("copy returned no command")
	}
	if !strings.Contains(h.screen(), "copied 2 lines") {
		t.Errorf("screen:\n%s", h.screen())
	}
}

// t says which tags the cursor's line has, and which style each takes.
func TestBrowseLineTags(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("t") // Rook yawns.
	if !strings.Contains(h.screen(), str.BrowseNoLineTags()) {
		t.Errorf("want no tags:\n%s", h.screen())
	}
	h.keys("up", "up", "t") // Kit grins.
	if want := str.BrowseLineTags("self"); !strings.Contains(h.screen(), want) {
		t.Errorf("want %q:\n%s", want, h.screen())
	}
	h.keys("up", "up", "t") // Mira pages
	want := str.BrowseLineTags("page" + str.Separator() + str.BrowseTagAs("page/in", "page"))
	if !strings.Contains(h.screen(), want) {
		t.Errorf("want %q:\n%s", want, h.screen())
	}
}

func TestBrowseFind(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
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
	if !strings.Contains(h.m.View().Content, theme.SGR(theme.LogFind)+"Rook") {
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
	h.key("ctrl+l")
	b := h.br()
	_, cmd := h.m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	if cmd == nil || !b.loading {
		t.Fatal("Home should start a background load")
	}
	if len(b.lines) != 300 || b.cursor.e.Text != "day 23 line 0" {
		t.Errorf("Update must not load synchronously: %d lines, cursor %q", len(b.lines), b.cursor.e.Text)
	}
	if s := h.screen(); !strings.Contains(s, str.BrowseLoading()) || strings.Contains(s, "⋯") {
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
	h.key("ctrl+l")
	_, cmd := h.m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	if cmd == nil {
		t.Fatal("no load started")
	}
	h.key("esc")
	h.key("ctrl+l") // a new browse
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
	h.key("ctrl+l")
	b := h.br()
	if len(b.lines) != 300 {
		t.Errorf("initially loaded %d lines, want 300 (two days)", len(b.lines))
	}
	if !strings.Contains(h.screen(), "…"+str.BrowseToToday("Wed Sep 23")) {
		t.Errorf("header should show more history exists:\n%s", h.screen())
	}
	h.key("g")
	h.typeText("2026-09-21")
	h.key("enter")
	if b.cursor.e.Text != "day 21 line 0" {
		t.Errorf("date jump landed on %q", b.cursor.e.Text)
	}
	if !strings.Contains(h.screen(), str.BrowseToToday("Mon Sep 21")) || strings.Contains(h.screen(), "…Mon") {
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
	h.key("ctrl+l")
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
	h.key("ctrl+l")
	h.conn("fm/kit").lines <- "Brand new line"
	h.settle("fm/kit", func() bool { return strings.Contains(h.screen(), "Brand new line") })
	if h.br().cursor.e.Text != "Brand new line" {
		t.Error("cursor at the bottom should follow live lines")
	}
}

func TestBrowseMouse(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.screen()
	b := h.br()
	l := h.m.layout()
	rowOf := func(text string) int {
		for i, bl := range b.rowLines {
			if bl != nil && bl.e.Text == text {
				return i + 2
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
}

func TestBrowseMouseSelectsLikeFinder(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.screen()
	b := h.br()
	x := h.m.layout().sw + 1 + 10
	click := func(i int, shift bool) {
		t.Helper()
		var mod tea.KeyMod
		if shift {
			mod = tea.ModShift
		}
		for row, bl := range b.rowLines {
			if bl == b.lines[i] {
				h.m.Update(tea.MouseClickMsg{X: x, Y: row + 2, Button: tea.MouseLeft, Mod: mod})
				h.screen()
				return
			}
		}
		t.Fatalf("line %d not on screen", i)
	}
	span := func() (int, int) { return b.index(b.start), b.index(b.end) }

	click(2, false)
	if s, e := span(); b.end == nil || s != 2 || e != 2 || b.cursor != b.lines[2] {
		t.Fatalf("click should select just its line, got %d–%d", s, e)
	}
	click(4, true)
	if s, e := span(); s != 2 || e != 4 {
		t.Fatalf("shift-click below should extend to %d–%d, got %d–%d", 2, 4, s, e)
	}
	click(1, true)
	if s, e := span(); s != 1 || e != 4 {
		t.Fatalf("shift-click above should extend to %d–%d, got %d–%d", 1, 4, s, e)
	}
	click(3, true)
	if !b.excluded[b.lines[3]] {
		t.Fatal("shift-click inside the range should exclude")
	}
	if s, e := span(); s != 1 || e != 4 {
		t.Errorf("excluding changed the range to %d–%d", s, e)
	}
	click(3, true)
	if b.excluded[b.lines[3]] {
		t.Error("shift-click on an excluded line should bring it back")
	}
	click(3, true)
	click(0, false) // a new selection forgets old exclusions
	click(4, true)
	if s, e := span(); s != 0 || e != 4 || b.excluded[b.lines[3]] {
		t.Errorf("new range %d–%d, line 3 excluded %v", s, e, b.excluded[b.lines[3]])
	}
}

func TestHighlightCommand(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("/highlight the lighthouse")
	h.enter()
	if !strings.Contains(h.screen(), str.StatusHighlightAdded("the lighthouse")) {
		t.Fatalf("screen:\n%s", h.screen())
	}
	h.m.Update(reloadMsg{})
	cs := h.m.chars["fm/kit"]
	text, _ := cs.render(logstore.Entry{Dir: logstore.In, Text: "Rook: The Lighthouse is dark."})
	_, ts, _ := theme.Active().Tag(config.HighlightTag)
	if !strings.Contains(text, ts.Style.SGR()+"The Lighthouse"+style.Reset+" is dark.") {
		t.Errorf("new highlight not drawn on just the match: %q", text)
	}
	h.typeText("/highlight")
	h.enter()
	if !strings.Contains(h.screen(), str.ConfigNothingToHighlight()) {
		t.Errorf("screen:\n%s", h.screen())
	}
}

func TestBrowseCommandAndSwitching(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook")
	h.typeText("/log")
	h.enter()
	if h.br() == nil || !strings.Contains(h.screen(), str.BrowseNoLogs()) {
		t.Fatalf("screen:\n%s", h.screen())
	}
	h.press(tea.KeyDown, tea.ModCtrl) // switch to Rook: normal view
	if strings.Contains(h.screen(), "LOG") {
		t.Error("rook should not be in browse mode")
	}
	h.press(tea.KeyUp, tea.ModCtrl)
	if !strings.Contains(h.screen(), "LOG Kit") {
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
		if !strings.Contains(got, theme.SGR(theme.LogFind)+c.needle) {
			t.Errorf("highlightFind(%q, %q) = %q: match not highlighted", c.plain, c.needle, got)
		}
	}
}

func TestBrowseLiveDedupeAtMillisecondPrecision(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	base := day24.Add(123456789 * time.Nanosecond) // not a whole millisecond
	w := logstore.NewWriter(logstore.Layout{Root: h.m.d.LogRoot, World: "fm", Char: "kit", CharName: "Kit"})
	var sent []logstore.Entry
	for i, text := range []string{"one", "two", "three"} {
		e := logstore.Entry{Time: base.Add(time.Duration(i) * time.Second), Dir: logstore.In, Text: text}
		w.Append(e)
		sent = append(sent, e)
	}
	w.Close()
	h.key("ctrl+l")
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

func TestBrowseFindIsFastOnLargeHistories(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.key("ctrl+l")
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
	h.key("ctrl+l")
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
	h.m.cfg.ExportDir = filepath.Join(home, "scenes")
	h.key("ctrl+l")
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
	h.m.cfg.ExportDir = ""
	h.key("ctrl+l")
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
	h.key("ctrl+l")
	dir := t.TempDir()
	os.WriteFile(filepath.Join(h.dir, "config.toml"), []byte("export_dir = \""+dir+"\"\n"), 0o600)
	h.m.Update(reloadMsg{})
	if h.br().exportDir != dir {
		t.Errorf("open browse exportDir = %q, want %q", h.br().exportDir, dir)
	}
}

func TestExportNameAndFormatSettings(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(h.dir, "config.toml"), []byte("export_dir = \""+dir+"\"\nexport_name = \"{world}/{name} {date}\"\nexport_format = \"html\"\n"), 0o600)
	h.m.Update(reloadMsg{})
	h.key("ctrl+l")
	h.keys("m", "up", "m", "e")
	if !strings.Contains(h.screen(), "enter html") {
		t.Errorf("preselected format not offered:\n%s", h.screen())
	}
	h.keys("enter", "enter")
	if _, err := os.Stat(filepath.Join(dir, "fm", "Kit 2026-09-24.html")); err != nil {
		t.Errorf("export not at the templated name: %v\n%s", err, h.screen())
	}
}

func TestLogDirAndNameSettings(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	logs := filepath.Join(t.TempDir(), "Mucks")
	os.WriteFile(filepath.Join(h.dir, "config.toml"),
		[]byte("log_dir = \""+logs+"/{world}/{name}/%Y/%m\"\nlog_name = \"%Y-%m-%d.%H.%M.%S\"\n"), 0o600)
	h.m.Update(reloadMsg{})
	l, ok := h.m.logLayout(h.m.chars["fm/kit"].ch)
	if !ok {
		t.Fatal("no log layout")
	}
	w := logstore.NewWriter(l)
	w.Append(logstore.Entry{Time: day24, Dir: logstore.In, Text: "from my own log folder"})
	w.Close()
	want := filepath.Join(logs, "fm", "Kit", "2026", "09", "2026-09-24.21.00.00.log")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("log not at %s: %v", want, err)
	}
	h.key("ctrl+l")
	if !strings.Contains(h.screen(), "from my own log folder") {
		t.Errorf("browse didn't read it back:\n%s", h.screen())
	}
}

// The statusline's Log chip sits after the character's name in both
// modes: clicking it opens log mode, and in log mode goes back.
func TestStatusLogChip(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	l := h.m.layout()
	chipX := l.sw + 1 + len("fm/Kit ") + 1 // on the label, past the chip's leading space
	click := func(x int) {
		h.m.Update(tea.MouseClickMsg{X: x, Y: h.m.height - 1, Button: tea.MouseLeft})
	}
	click(chipX - 2) // the name: nothing
	if h.m.cur().browse != nil {
		t.Fatal("clicking the name opened log mode")
	}
	click(chipX)
	if h.m.cur().browse == nil {
		t.Fatalf("clicking %s didn't open log mode:\n%s", str.ViewLogButton(), h.screen())
	}
	click(chipX)
	if h.m.cur().browse != nil {
		t.Fatalf("clicking %s in log mode didn't go back:\n%s", str.ViewLogButton(), h.screen())
	}

	// A name too long to leave room for the chip: nothing to click.
	h.m.chars["fm/kit"].ch.Name = strings.Repeat("K", h.m.width)
	for x := l.sw + 1; x < h.m.width; x++ {
		click(x)
		if h.m.cur().browse != nil {
			t.Fatalf("a click at %d opened log mode with the chip cut off:\n%s", x, h.screen())
		}
	}
}

// kitFilter is fm/kit's log filter.
func (h *harness) kitFilter() *scene.Filter { return &h.m.chars["fm/kit"].filter }

func TestBrowseFilterHidesAndOnly(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	b := h.br()
	f := h.kitFilter()
	f.PressOnly(scene.Item{Name: "page"}, b.items())
	b.refilter()
	s := h.screen()
	if strings.Contains(s, "Sable") || !strings.Contains(s, "Mira pages") {
		t.Errorf("only page:\n%s", s)
	}
	f.PressOnly(scene.Item{Name: "page"}, b.items()) // clean slate
	f.ToggleHide(scene.Item{Name: "page"}, b.items())
	b.refilter()
	s = h.screen()
	if strings.Contains(s, "Mira pages") || !strings.Contains(s, "Sable") {
		t.Errorf("hide page:\n%s", s)
	}
}

func TestBrowseHidingCursorLineKeepsPlace(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.keys("home", "down", "down") // Mira pages
	b := h.br()
	f := h.kitFilter()
	if b.cursor.e.Text != "Mira pages: you around?" {
		t.Fatalf("cursor on %q", b.cursor.e.Text)
	}
	f.ToggleHide(scene.Item{Name: "page"}, b.items())
	b.refilter()
	if got := b.cursor.e.Text; got != "> :grins." && got != ":grins." {
		t.Errorf("cursor moved to %q, want the next line", got)
	}
	f.ToggleHide(scene.Item{Name: "page"}, b.items()) // shown again
	h.keys("end")                                     // Rook yawns, the last line
	f.PressOnly(scene.Item{Name: "page"}, b.items())
	b.refilter()
	if got := b.cursor.e.Text; got != "Mira pages: you around?" {
		t.Errorf("cursor moved to %q, want the previous visible line", got)
	}
}

func TestFilterNewTagUnderOnlyArrivesHidden(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, "Rook whispers, \"Hi, Kit.\"")
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.key("ctrl+l")
	b := h.br()
	f := h.kitFilter()
	f.PressOnly(scene.Item{Name: "whisper"}, b.items())
	h.conn("fm/kit").lines <- "Mira pages: you around?"
	h.settle("fm/kit", func() bool { return slices.Contains(b.items(), scene.Item{Name: "page"}) })
	if !f.Hidden(scene.Item{Name: "page"}) || strings.Contains(h.screen(), "Mira pages") {
		t.Errorf("a tag arriving under Only whisper should arrive hidden:\n%s", h.screen())
	}
}

func TestFilterOutlastsLogMode(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.kitFilter().ToggleHide(scene.Item{Name: "page"}, h.br().items())
	h.key("esc")
	h.key("ctrl+l")
	if strings.Contains(h.screen(), "Mira pages") {
		t.Errorf("filter lost on leaving log mode:\n%s", h.screen())
	}
}

func TestFilterChipOnHeaderRow(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	rows := strings.Split(h.screen(), "\n")
	head := strings.SplitN(rows[0], "│", 2)[1]
	if !strings.HasSuffix(strings.TrimRight(head, " "), " "+str.FilterTitle()) {
		t.Errorf("header row = %q, want the Filter chip at its end", head)
	}
	if strings.Contains(h.screen(), "1[") {
		t.Errorf("numbered chips still drawn:\n%s", h.screen())
	}
}
