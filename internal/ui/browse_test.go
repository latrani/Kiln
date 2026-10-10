package ui

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/kilntest"
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
	kilntest.WriteLog(h.t, logstore.Layout{Root: h.m.d.LogRoot, World: "fm", Char: "kit", CharName: "Kit"}, start, lines...)
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
	case "ctrl+c":
		k = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
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
		if b := h.br(); b == nil || !b.Loading {
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
	if top := rightRow(h, 0); !strings.HasPrefix(top, "fm"+str.Separator()+"Kit") {
		t.Errorf("top bar = %q", top)
	}
	for _, want := range []string{"── Thu Sep 24 ──", "21:00", "Rook says", "21:06", firstPart(str.BrowseHints())} {
		if !strings.Contains(s, want) {
			t.Errorf("screen missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "\n> ") || strings.Contains(s, "│> ") {
		t.Error("normal input still shown in browse mode")
	}
	h.key("esc")
	if h.br() != nil || strings.Contains(h.screen(), firstPart(str.BrowseHints())) {
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
	last := func() string {
		rows := strings.Split(h.screen(), "\n")
		return strings.TrimSpace(strings.SplitN(rows[len(rows)-1], "│", 2)[1])
	}
	if got := last(); got != str.ViewSelected(0) {
		t.Errorf("statusline = %q", got)
	}
	if !strings.Contains(rows[len(rows)-3], firstPart(str.BrowseHints())) {
		t.Errorf("action bar should sit a rule above the statusline:\n%s", h.screen())
	}
	h.keys("m", "up", "m")
	if got := last(); got != str.BrowseLinesInRange(2) {
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
	h := newHarness(t, map[string]string{"fm": echoWorld})
	h.writeLog(day24, scene1...)
	exportDir := t.TempDir()
	h.m.a.Config().ExportDir = exportDir
	h.key("ctrl+l")
	// Cursor starts on the last line (Rook yawns). Mark 21:01..21:05.
	h.keys("up", "m")                   // Rook says lighthouse = end
	h.keys("up", "up", "up", "up", "m") // Sable = start (marks may go either way)
	h.keys("down", "space")             // exclude the page
	b := h.br()
	var got []string
	for _, e := range b.Selection() {
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
	if cmd, _ := h.br().key(tea.KeyPressMsg{Code: 'c', Text: "c"}, h.m.browseBodyH()); cmd != nil {
		t.Error("copy without range should do nothing") // no clipboard write; the message's timer is the model's
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
	h := newHarness(t, map[string]string{"fm": echoWorld})
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

// Find goes newest first: from the cursor (at the newest line on
// opening) back through older lines, n further back, N forward again,
// stopping at either end rather than wrapping.
func TestBrowseFind(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("/")
	h.typeText("rook")
	h.key("enter")
	b := h.br()
	if b.Cursor.Entry.Text != scene1[6] || !strings.Contains(h.screen(), str.BrowseFindStatus("rook", 1, 3)) {
		t.Errorf("first match should be the newest: %q\n%s", b.Cursor.Entry.Text, h.screen())
	}
	h.key("n")
	if b.Cursor.Entry.Text != scene1[5] {
		t.Errorf("n went to %q", b.Cursor.Entry.Text)
	}
	h.key("n")
	h.key("n") // past the oldest
	if b.Cursor.Entry.Text != scene1[0] || !strings.Contains(h.screen(), str.BrowseNoOlderMatches("rook")) {
		t.Errorf("n past the oldest match should stay, and say so: %q\n%s", b.Cursor.Entry.Text, h.screen())
	}
	h.key("N")
	if b.Cursor.Entry.Text != scene1[5] {
		t.Errorf("N went to %q", b.Cursor.Entry.Text)
	}
	h.keys("N", "N") // past the newest
	if b.Cursor.Entry.Text != scene1[6] || !strings.Contains(h.screen(), str.BrowseNoNewerMatches("rook")) {
		t.Errorf("N past the newest match should stay, and say so: %q\n%s", b.Cursor.Entry.Text, h.screen())
	}
	if !strings.Contains(h.m.View().Content, theme.SGR(theme.LogFind)+"Rook") {
		t.Error("matches not highlighted")
	}
	if !strings.Contains(h.screen(), "Sable") {
		t.Error("find must not hide lines")
	}
	h.key("/")
	h.press('u', tea.ModCtrl) // clear the last term
	h.typeText("nobody")
	h.key("enter")
	if !strings.Contains(h.screen(), str.BrowseNoMatches("nobody")) {
		t.Errorf("a term with no matches should say so:\n%s", h.screen())
	}
}

// Find reads in older days for a match not yet loaded, and its count
// says when older history may hold more.
func TestBrowseFindSearchesOlderHistory(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24.AddDate(0, 0, -1), "Rook says, \"lighthouse\"", "Sable waves.")
	var lines []string
	for i := range app.LogInitialLines + 20 {
		lines = append(lines, fmt.Sprintf("Sable says, \"line %d\"", i))
	}
	lines[10] = "Rook says, \"lighthouse again\""
	h.writeLog(day24, lines...)
	h.key("ctrl+l")
	b := h.br()
	if b.HistDone {
		t.Fatal("the older day shouldn't be loaded yet")
	}
	h.key("/")
	h.typeText("lighthouse")
	h.key("enter")
	if b.Cursor.Entry.Text != lines[10] || !strings.Contains(h.screen(), str.BrowseFindStatusMore("lighthouse", 1, 1)) {
		t.Errorf("first match, with more history to search: %q\n%s", b.Cursor.Entry.Text, h.screen())
	}
	h.key("n")
	if b.Cursor.Entry.Text != "Rook says, \"lighthouse\"" || !strings.Contains(h.screen(), str.BrowseFindStatus("lighthouse", 2, 2)) {
		t.Errorf("n should read in the older day for its match: %q\n%s", b.Cursor.Entry.Text, h.screen())
	}
}

// While find reads older days, the top bar says so; a move that asks
// for older lines itself takes over.
func TestBrowseFindSaysWhileSearching(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24.AddDate(0, 0, -2), scene1...)
	h.writeLog(day24.AddDate(0, 0, -1), scene1...)
	lines := make([]string, app.LogInitialLines+20)
	for i := range lines {
		lines[i] = fmt.Sprintf("Sable says, \"line %d\"", i)
	}
	h.writeLog(day24, lines...)
	h.key("ctrl+l")
	b := h.br()
	b.Find = "nowhere"
	today := app.DayLabel(day24.Format("2006-01-02"))
	cmd := b.run(b.FindOlder(true))
	if cmd == nil || b.FindStatus() != str.BrowseSearching("nowhere", today) {
		t.Fatalf("searching: status %q", b.FindStatus())
	}
	b.Older(nil) // as moving up past the top does
	if b.FindStatus() == str.BrowseSearching("nowhere", today) {
		t.Error("the search should give way")
	}

	// Esc stops a search, staying in log mode with the cursor where it was.
	h.m.Update(cmd()) // the read Older(nil) took over
	cursor := b.Cursor
	b.FindOlder(true)
	h.press(tea.KeyEscape, 0) // not h.key: the read stays in flight, undrained
	if h.br() != b || b.Searching || b.Cursor != cursor {
		t.Fatalf("Esc should stop the search and stay:\n%s", h.screen())
	}
	if want := str.BrowseSearchCancelled(); !strings.Contains(h.screen(), want) {
		t.Errorf("want %q:\n%s", want, h.screen())
	}
	h.key("esc")
	if h.br() != nil {
		t.Error("with no search, Esc leaves log mode")
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
	if cmd == nil || !b.Loading {
		t.Fatal("Home should start a background load")
	}
	if len(b.Lines) != 300 || b.Cursor.Entry.Text != "day 23 line 0" {
		t.Errorf("Update must not load synchronously: %d lines, cursor %q", len(b.Lines), b.Cursor.Entry.Text)
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
	if len(b.Lines) != 450 || cmd == nil {
		t.Fatalf("after one day: %d lines, next cmd %v", len(b.Lines), cmd != nil)
	}
	h.drainLoads(cmd)
	if b.Loading || !b.HistDone || b.Cursor.Entry.Text != "day 21 line 0" {
		t.Errorf("loading=%v done=%v cursor=%q", b.Loading, b.HistDone, b.Cursor.Entry.Text)
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
	n := len(fresh.Lines)
	h.m.Update(cmd()) // the old browse's day arrives late
	if len(fresh.Lines) != n {
		t.Errorf("stale load changed the new browse: %d → %d lines", n, len(fresh.Lines))
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
	if len(b.Lines) != 300 {
		t.Errorf("initially loaded %d lines, want 300 (two days)", len(b.Lines))
	}
	if b.HistDone {
		t.Error("older days should remain to page in")
	}
	h.key("g")
	h.typeText("2026-09-21")
	h.key("enter")
	if b.Cursor.Entry.Text != "day 21 line 0" {
		t.Errorf("date jump landed on %q", b.Cursor.Entry.Text)
	}
	if !b.HistDone || b.Lines[0].Day != "2026-09-21" {
		t.Errorf("after loading everything: done %v, oldest %s", b.HistDone, b.Lines[0].Day)
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
	start, end := b.Start, b.End
	h.key("home") // loads everything older
	if b.Start != start || b.End != end || len(b.Selection()) != 2 {
		t.Error("marks moved when older history loaded")
	}
}

func TestBrowseLiveLinesArrive(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.key("ctrl+l")
	h.conn("fm/kit").Feed("Brand new line")
	h.settle("fm/kit", func() bool { return strings.Contains(h.screen(), "Brand new line") })
	if h.br().Cursor.Entry.Text != "Brand new line" {
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
			if bl != nil && bl.Entry.Text == text {
				return i + 2
			}
		}
		t.Fatalf("%q not on screen", text)
		return 0
	}
	x := l.sw + 1 + 10
	h.m.Update(tea.MouseClickMsg{X: x, Y: rowOf(scene1[1]), Button: tea.MouseLeft})
	if b.Cursor.Entry.Text != scene1[1] {
		t.Errorf("click moved cursor to %q", b.Cursor.Entry.Text)
	}
	h.m.Update(tea.MouseClickMsg{X: x, Y: rowOf(scene1[4]), Button: tea.MouseLeft, Mod: tea.ModShift})
	if b.Start == nil || b.End == nil || b.Start.Entry.Text != scene1[1] || b.End.Entry.Text != scene1[4] {
		t.Fatalf("shift-click range wrong")
	}
	h.screen()
	h.m.Update(tea.MouseClickMsg{X: l.sw + 1 + browsePrefixW - 2, Y: rowOf(scene1[2]), Button: tea.MouseLeft})
	if !b.Excluded[b.Lines[2]] {
		t.Error("gutter click did not exclude")
	}
}

func TestBrowseMouseSelectsLikeFinder(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": echoWorld})
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
			if bl == b.Lines[i] {
				h.m.Update(tea.MouseClickMsg{X: x, Y: row + 2, Button: tea.MouseLeft, Mod: mod})
				h.screen()
				return
			}
		}
		t.Fatalf("line %d not on screen", i)
	}
	span := func() (int, int) { return b.Index(b.Start), b.Index(b.End) }

	click(2, false)
	if s, e := span(); b.End == nil || s != 2 || e != 2 || b.Cursor != b.Lines[2] {
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
	if !b.Excluded[b.Lines[3]] {
		t.Fatal("shift-click inside the range should exclude")
	}
	if s, e := span(); s != 1 || e != 4 {
		t.Errorf("excluding changed the range to %d–%d", s, e)
	}
	click(3, true)
	if b.Excluded[b.Lines[3]] {
		t.Error("shift-click on an excluded line should bring it back")
	}
	click(3, true)
	click(0, false) // a new selection forgets old exclusions
	click(4, true)
	if s, e := span(); s != 0 || e != 4 || b.Excluded[b.Lines[3]] {
		t.Errorf("new range %d–%d, line 3 excluded %v", s, e, b.Excluded[b.Lines[3]])
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
	if h.m.chars["fm/rook"].browse != nil || strings.Contains(h.screen(), firstPart(str.BrowseHints())) {
		t.Error("rook should not be in browse mode")
	}
	h.press(tea.KeyUp, tea.ModCtrl)
	if h.br() == nil || !strings.Contains(h.screen(), firstPart(str.BrowseHints())) {
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

func TestBrowseFindIsFastOnLargeHistories(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.key("ctrl+l")
	b := h.br()
	for i := 0; i < 50000; i++ {
		text := fmt.Sprintf("a line %d", i)
		if i == 0 {
			text = "the line" // the only match, at the far end
		}
		b.Lines = append(b.Lines, &app.LogLine{Line: app.Line{Entry: logstore.Entry{Time: day24, Dir: logstore.In, Text: text}, Day: "2026-09-24"}, Lower: text})
	}
	b.Find = "the"
	start := time.Now()
	for i := 0; i < 10; i++ {
		b.Cursor = b.Lines[len(b.Lines)-1]
		b.FindOlder(false) // the whole way back every time
		b.FindStatus()
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
	if h.br().Find != "lighthouse" {
		t.Errorf("paste into find prompt: find = %q", h.br().Find)
	}
}

func TestSaveExpandsHomeAndRelativePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.m.a.Config().ExportDir = filepath.Join(home, "scenes")
	h.key("ctrl+l")
	h.keys("m", "up", "m")
	b := h.br()
	b.format = "plain"
	b.save("~/Desktop/a.txt")
	if _, err := os.Stat(filepath.Join(home, "Desktop", "a.txt")); err != nil {
		t.Errorf("~ not expanded: %v (%s)", err, b.Status)
	}
	b.save("b.txt")
	if _, err := os.Stat(filepath.Join(home, "scenes", "b.txt")); err != nil {
		t.Errorf("relative name not placed in export_dir: %v (%s)", err, b.Status)
	}
	b.save("  ")
	if !strings.Contains(b.Status, "file name") {
		t.Errorf("empty name status = %q", b.Status)
	}
}

func TestSaveWithoutExportDirRefusesRelativePath(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.m.a.Config().ExportDir = ""
	h.key("ctrl+l")
	h.keys("m", "up", "m")
	b := h.br()
	b.format = "plain"
	b.save("scene.txt")
	if !strings.Contains(b.Status, "no export_dir") {
		t.Errorf("status = %q", b.Status)
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
	if h.br().cfg().ExportDir != dir {
		t.Errorf("open browse exportDir = %q, want %q", h.br().cfg().ExportDir, dir)
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
	l, ok := h.m.a.LogLayout(h.m.chars["fm/kit"].Ch)
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

func (h *harness) kitFilter() *scene.Filter { return &h.m.chars["fm/kit"].Filter }

func TestBrowseFilterHidesAndOnly(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	b := h.br()
	f := h.kitFilter()
	f.PressOnly(scene.Item{Name: "page"}, b.Items())
	b.Refilter()
	s := h.screen()
	if strings.Contains(s, "Sable") || !strings.Contains(s, "Mira pages") {
		t.Errorf("only page:\n%s", s)
	}
	f.PressOnly(scene.Item{Name: "page"}, b.Items()) // clean slate
	f.ToggleHide(scene.Item{Name: "page"}, b.Items())
	b.Refilter()
	s = h.screen()
	if strings.Contains(s, "Mira pages") || !strings.Contains(s, "Sable") {
		t.Errorf("hide page:\n%s", s)
	}
}

func TestBrowseHidingCursorLineKeepsPlace(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": echoWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.keys("home", "down", "down") // Mira pages
	b := h.br()
	f := h.kitFilter()
	if b.Cursor.Entry.Text != "Mira pages: you around?" {
		t.Fatalf("cursor on %q", b.Cursor.Entry.Text)
	}
	f.ToggleHide(scene.Item{Name: "page"}, b.Items())
	b.Refilter()
	if got := b.Cursor.Entry.Text; got != "> :grins." && got != ":grins." {
		t.Errorf("cursor moved to %q, want the next line", got)
	}
	f.ToggleHide(scene.Item{Name: "page"}, b.Items()) // shown again
	h.keys("end")                                     // Rook yawns, the last line
	f.PressOnly(scene.Item{Name: "page"}, b.Items())
	b.Refilter()
	if got := b.Cursor.Entry.Text; got != "Mira pages: you around?" {
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
	f.PressOnly(scene.Item{Name: "whisper"}, b.Items())
	h.conn("fm/kit").Feed("Mira pages: you around?")
	h.settle("fm/kit", func() bool { return slices.Contains(b.Items(), scene.Item{Name: "page"}) })
	if !f.Hidden(scene.Item{Name: "page"}) || strings.Contains(h.screen(), "Mira pages") {
		t.Errorf("a tag arriving under Only whisper should arrive hidden:\n%s", h.screen())
	}
}

func TestFilterOutlastsLogMode(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.kitFilter().ToggleHide(scene.Item{Name: "page"}, h.br().Items())
	h.key("esc")
	h.key("ctrl+l")
	if strings.Contains(h.screen(), "Mira pages") {
		t.Errorf("filter lost on leaving log mode:\n%s", h.screen())
	}
}

// Ctrl+L (or the Log chip) hides log mode as you left it: back on the
// next Ctrl+L with the filter panel, cursor and marks, even after
// looking at another world, and caught up on what arrived meanwhile.
// Esc closes it, and the next one starts fresh.
func TestLogModeKeptAcrossToggle(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.writeLog(day24, scene1...)
	h.open("sp/ash")
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.key("ctrl+l")
	h.keys("up", "up", "m")
	h.key("f")
	h.key("down")
	b := h.br()
	cursor, sel := b.Cursor, b.Panel.Sel
	h.key("ctrl+l")
	if h.br() != nil {
		t.Fatal("Ctrl+L should leave log mode")
	}
	h.m.switchTo("sp/ash") // another world
	h.conn("fm/kit").Feed("Brand new line")
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].Unread > 0 })
	h.m.switchTo("fm/kit")
	h.key("ctrl+l")
	if h.br() != b || b.Panel == nil || b.Panel.Sel != sel || b.Cursor != cursor || b.Start == nil {
		t.Fatalf("log mode not as left:\n%s", h.screen())
	}
	if b.Last().Entry.Text != "Brand new line" {
		t.Errorf("hidden log mode missed a live line: last %q", b.Last().Entry.Text)
	}
	h.key("esc") // the panel
	h.key("esc") // log mode
	h.key("ctrl+l")
	if h.br() == b || h.br().Panel != nil || h.br().Start != nil {
		t.Errorf("after Esc, log mode should start fresh:\n%s", h.screen())
	}
}

// The Log chip hides log mode as Ctrl+L does, and /log brings it back
// as Ctrl+L does.
func TestLogModeKeptAcrossChipAndCommand(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.typeText("/log")
	h.enter()
	h.key("f")
	b := h.br()
	l := h.m.layout()
	_, logc, _, _ := h.m.topBar(l.rw)
	h.m.Update(tea.MouseClickMsg{X: l.sw + 1 + logc[0] + 1, Y: 0, Button: tea.MouseLeft})
	if h.br() != nil {
		t.Fatalf("the Log chip should leave log mode:\n%s", h.screen())
	}
	h.typeText("/log")
	h.enter()
	if h.br() != b || b.Panel == nil {
		t.Errorf("/log should bring log mode back as the chip left it:\n%s", h.screen())
	}
}

// The top row of log mode always shows the date of the line at the top,
// mid-day too, and scrolling never hides the cursor under it.
func TestLogDateSticksToTop(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	var lines []string
	for i := range 80 {
		lines = append(lines, fmt.Sprintf("Rook says, \"line %d\"", i))
	}
	h.writeLog(day24, lines...)
	h.writeLog(day24.AddDate(0, 0, 1), lines...)
	h.key("ctrl+l")
	divider := func(day time.Time) string { return "── " + app.DayLabel(day.Format("2006-01-02")) + " ──" }
	firstRow := func() string { return strings.SplitN(strings.Split(h.screen(), "\n")[topH], "│", 2)[1] }
	if got := firstRow(); !strings.Contains(got, divider(day24.AddDate(0, 0, 1))) {
		t.Errorf("mid-day, the top row should be the day: %q\n%s", got, h.screen())
	}
	b := h.br()
	for range 100 { // up across the day boundary, a line at a time
		h.key("up")
		if h.screen(); !slices.Contains(b.rowLines, b.Cursor) {
			t.Fatalf("cursor scrolled out of view:\n%s", h.screen())
		}
	}
	if got := firstRow(); !strings.Contains(got, divider(day24)) {
		t.Errorf("back on the 24th, the top row should say so: %q\n%s", got, h.screen())
	}
}

// Whatever you do in log mode, the body never ends in blank rows, and the
// cursor stays in view: moves, pages, the wheel, filtering, resizing.
func TestLogBodyStaysFull(t *testing.T) {
	for seed := range int64(3) {
		h := newHarness(t, map[string]string{"fm": fmWorld})
		r := rand.New(rand.NewSource(seed))
		var lines []string
		for i := range 150 {
			who := []string{"Rook says, ", "Mira pages: ", "Sable says, "}[r.Intn(3)]
			lines = append(lines, fmt.Sprintf("%s%q", who, strings.Repeat("word ", r.Intn(30))+fmt.Sprint(i)))
		}
		h.writeLog(day24.AddDate(0, 0, -2), lines[:30]...)
		h.writeLog(day24.AddDate(0, 0, -1), lines[30:70]...)
		h.writeLog(day24, lines[70:]...)
		h.key("ctrl+l")
		b := h.br()
		keys := []string{"up", "down", "pgup", "pgdn", "home", "end", "wu", "wd", "wu", "wd", "wu", "wd", "filter", "resize"}
		for step := range 400 {
			l := h.m.layout()
			k := keys[r.Intn(len(keys))]
			switch k {
			case "wu":
				h.drainLoads(h.m.handleWheel(tea.MouseWheelMsg{X: l.sw + 5, Y: 5, Button: tea.MouseWheelUp}))
			case "wd":
				h.drainLoads(h.m.handleWheel(tea.MouseWheelMsg{X: l.sw + 5, Y: 5, Button: tea.MouseWheelDown}))
			case "pgup":
				h.press(tea.KeyPgUp, 0)
			case "pgdn":
				h.press(tea.KeyPgDown, 0)
			case "filter":
				h.kitFilter().ToggleHide(scene.Item{Name: "page"}, b.Items())
				b.Refilter()
			case "resize":
				h.m.Update(tea.WindowSizeMsg{Width: 60 + r.Intn(60), Height: 12 + r.Intn(20)})
			default:
				h.key(k)
			}
			s := h.screen()
			rows := strings.Split(s, "\n")
			bodyEnd := len(rows) - 5
			last := strings.TrimSpace(strings.SplitN(rows[bodyEnd], "│", 2)[1])
			if last == "" {
				t.Fatalf("seed %d step %d after %s: blank bottom row\n%s", seed, step, k, s)
			}
			if !slices.Contains(b.rowLines, b.Cursor) {
				t.Fatalf("seed %d step %d after %s: cursor out of view\n%s", seed, step, k, s)
			}
		}
	}
}

// The wheel scrolls log mode's view a line at a time, as it scrolls the
// scrollback, leaving the cursor be until it would go out of sight; then
// it's dragged along at the edge.
func TestLogWheelScrolls(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	var lines []string
	for i := range 80 {
		lines = append(lines, fmt.Sprintf("Rook says, \"line %d\"", i))
	}
	h.writeLog(day24, lines...)
	h.key("ctrl+l")
	b := h.br()
	l := h.m.layout()
	wheel := func(button tea.MouseButton) {
		h.drainLoads(h.m.handleWheel(tea.MouseWheelMsg{X: l.sw + 5, Y: 5, Button: button}))
		h.screen()
	}
	h.screen()
	bottom := b.Cursor
	top := b.top
	wheel(tea.MouseWheelUp)
	if b.top == top || b.top.Entry.Text != lines[slices.IndexFunc(lines, func(s string) bool { return s == top.Entry.Text })-1] {
		t.Errorf("one notch up should scroll one line: top %q, was %q", b.top.Entry.Text, top.Entry.Text)
	}
	if b.Cursor == bottom {
		t.Error("the cursor on the newest line, now scrolled out of view, should be dragged up with it")
	}
	if b.rowLines[len(b.rowLines)-1] != b.Cursor {
		t.Errorf("dragged to the bottom edge, the cursor should be the last line shown:\n%s", h.screen())
	}
	h.keys("up", "up") // off the edge: wheel leaves it alone now
	cursor := b.Cursor
	wheel(tea.MouseWheelUp)
	if b.Cursor != cursor {
		t.Errorf("a cursor in view shouldn't move: %q", b.Cursor.Entry.Text)
	}
	for range 100 {
		wheel(tea.MouseWheelUp)
	}
	if b.top != b.Visible()[0] || b.Cursor != b.rowLines[len(b.rowLines)-1] {
		t.Errorf("scrolled to the oldest line, the cursor is the last line shown: top %q cursor %q", b.top.Entry.Text, b.Cursor.Entry.Text)
	}
	h.key("home") // the cursor on the top edge
	wheel(tea.MouseWheelDown)
	if b.Cursor != b.top {
		t.Errorf("scrolling down, the cursor is dragged along at the top edge: top %q cursor %q", b.top.Entry.Text, b.Cursor.Entry.Text)
	}
	for range 100 {
		wheel(tea.MouseWheelDown)
	}
	if last := b.rowLines[len(b.rowLines)-1]; last != b.Last() {
		t.Errorf("scrolled to the end, the newest line should be at the bottom:\n%s", h.screen())
	}
}

// One wheel notch scrolls scroll_lines: rows in the scrollback, lines in
// log mode.
func TestWheelScrollsScrollLines(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	var lines []string
	for i := range 80 {
		lines = append(lines, fmt.Sprintf("Rook says, \"line %d\"", i))
	}
	h.writeLog(day24, lines...)
	cs := h.m.chars["fm/kit"]
	for _, c := range []struct{ setting, want int }{{config.DefaultScrollLines, 1}, {4, 4}} {
		h.m.a.Config().ScrollLines = c.setting
		l := h.m.layout()
		cs.sb.ToBottom()
		h.m.handleWheel(tea.MouseWheelMsg{X: l.sw + 5, Y: l.top + 1, Button: tea.MouseWheelUp})
		if cs.sb.offset != c.want {
			t.Errorf("scroll_lines %d: scrollback scrolled %d rows, want %d", c.setting, cs.sb.offset, c.want)
		}
		h.key("ctrl+l")
		b := h.br()
		h.screen()
		top := b.Index(b.top)
		h.drainLoads(h.m.handleWheel(tea.MouseWheelMsg{X: l.sw + 5, Y: 5, Button: tea.MouseWheelUp}))
		h.screen()
		if got := top - b.Index(b.top); got != c.want {
			t.Errorf("scroll_lines %d: log mode scrolled %d lines, want %d", c.setting, got, c.want)
		}
		h.key("esc")
	}
}

// Too few lines to fill log mode's body sit at its bottom, as in the
// scrollback.
func TestLogShortHistorySitsAtBottom(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	s := h.screen()
	if rows := strings.Split(s, "\n"); !strings.Contains(rows[len(rows)-5], scene1[6]) {
		t.Errorf("the newest line should be at the bottom:\n%s", s)
	}
}

// With local_echo off, the log view leaves out what you sent, like the
// scrollback does; the lines stay loaded, so turning it on brings them back.
func TestBrowseRespectsLocalEcho(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	if s := h.screen(); strings.Contains(s, "21:03") { // the sent line's time
		t.Errorf("sent line shown with local_echo off:\n%s", s)
	}
	b := h.br()
	for _, l := range b.Visible() {
		if l.Entry.Dir == logstore.Out {
			t.Errorf("visible sent line %q", l.Entry.Text)
		}
	}
	b.Cursor = b.LastVisible()
	for range scene1 {
		h.key("up")
	}
	if b.Cursor.Entry.Dir == logstore.Out {
		t.Error("cursor landed on a sent line")
	}

	h.m.chars["fm/kit"].Ch.LocalEcho = true
	if s := h.screen(); !strings.Contains(s, "21:03") {
		t.Errorf("sent line missing with local_echo on:\n%s", s)
	}
}

// The log mode prompt edits like the input box and forms do (#97).
func TestBrowsePromptEditKeys(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("/")
	h.typeText("say rook")
	h.press(tea.KeyBackspace, tea.ModAlt) // delete word: "say "
	h.press('a', tea.ModCtrl)             // to the start
	h.press('d', tea.ModCtrl)             // delete forward: "ay "
	if got := h.br().pin.Value(); got != "ay " {
		t.Errorf("after word delete, home and delete: %q", got)
	}
	h.press('k', tea.ModCtrl) // kill to end
	if got := h.br().pin.Value(); got != "" {
		t.Errorf("after kill to end: %q", got)
	}
}

// Ctrl+L closes log mode as it opened it, even with a prompt open (#98).
func TestBrowseKeyToggles(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	if h.br() == nil {
		t.Fatal("Ctrl+L didn't open log mode")
	}
	h.key("ctrl+l")
	if h.br() != nil {
		t.Errorf("Ctrl+L should close log mode:\n%s", h.screen())
	}
	h.key("ctrl+l")
	h.key("/")
	h.key("ctrl+l")
	if h.br() != nil {
		t.Errorf("Ctrl+L should close log mode from a prompt:\n%s", h.screen())
	}
}

// Saving says how it went on log mode's status, so the next log-mode key
// clears it like any log message.
func TestSaveFileStatusIsTheLogs(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.a.Config().ExportDir = t.TempDir()
	h.writeLog(day24, "a", "b")
	h.key("ctrl+l")
	b := h.br()
	b.SetCursor(b.Lines[0])
	b.Mark()
	b.SetCursor(b.Lines[1])
	b.Mark()
	b.format = "plain"
	b.ClearStatus()
	b.save("scene.txt")
	if want := str.BrowseSaved(filepath.Join(h.m.a.Config().ExportDir, "scene.txt")); b.Status != want {
		t.Errorf("log status %q, want %q", b.Status, want)
	}
}
