package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

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
	case "space":
		k = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "ctrl+b":
		k = tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl}
	default:
		r := []rune(s)[0]
		k = tea.KeyPressMsg{Code: r, Text: s}
	}
	_, cmd := h.m.Update(k)
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
	if strings.Contains(s, "> ") && strings.Contains(s, "fm/Kit 🔒") {
		t.Error("normal input/statusline still shown in browse mode")
	}
	h.key("esc")
	if h.br() != nil || !strings.Contains(h.screen(), "fm/Kit 🔒") {
		t.Errorf("esc did not close browse:\n%s", h.screen())
	}
}

func TestBrowseMarkExcludeExport(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
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

	exportDir := t.TempDir()
	h.m.exportDir = exportDir
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
