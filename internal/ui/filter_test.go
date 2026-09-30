package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/scene"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/theme"
)

// panelSide is the sidebar column's rows, trimmed.
func panelSide(h *harness) []string {
	var out []string
	for _, row := range strings.Split(h.screen(), "\n") {
		out = append(out, strings.TrimSpace(strings.SplitN(row, "│", 2)[0]))
	}
	return out
}

func buttons() string { return str.FilterHide() + " " + str.FilterOnly() }

func TestPanelOpensAndCloses(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	side := panelSide(h)
	want := []string{str.FilterUntagged(), buttons(), "", glyphOpen + " page", buttons(), "in", buttons(), "", "self", buttons(), "", str.FilterAddText()}
	for i, w := range want {
		if side[i] != w {
			t.Fatalf("panel row %d = %q, want %q:\n%s", i, side[i], w, h.screen())
		}
	}
	h.key("f")
	if h.br().panel != nil || panelSide(h)[0] != "fm" {
		t.Errorf("f should close the panel:\n%s", h.screen())
	}
	h.key("f")
	h.key("esc")
	if h.br() == nil || h.br().panel != nil {
		t.Error("Esc should close the panel and stay in log mode")
	}
	// The chip toggles it too (TestTopBarChipsToggle).
}

func TestPanelLightsTheItemsOwnState(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	h.kitFilter().PressOnly(scene.Item{Name: "self"}, h.br().items())
	s := h.drawn()
	on := func(label string) string { return theme.Paint(theme.FilterButtonOn, label) }
	if strings.Count(s, on(str.FilterHide())) != 3 || strings.Count(s, on(str.FilterOnly())) != 1 {
		t.Errorf("want Hide lit on Untagged, page and page/in, Only on self:\n%q", s)
	}
}

func TestPanelKeys(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	b, f := h.br(), h.kitFilter()
	page, in, self := scene.Item{Name: "page"}, scene.Item{Name: "page/in"}, scene.Item{Name: "self"}
	h.key("down") // past Untagged
	if b.panel.sel.item != page {
		t.Fatalf("starts on %v", b.panel.sel)
	}
	h.key("h")
	if !f.Hidden(page) || !f.Hidden(in) || strings.Contains(h.screen(), "Mira pages") {
		t.Errorf("h on page should hide it and page/in:\n%s", h.screen())
	}
	h.keys("down", "h") // page/in: unhide clears page, keeps nothing else hidden
	if f.Hidden(page) || f.Hidden(in) {
		t.Errorf("unhiding page/in: page %v, page/in %v", f.Hidden(page), f.Hidden(in))
	}
	h.keys("down", "o") // self: Only
	if o, _ := f.Only(); o != self || !f.Hidden(page) {
		t.Errorf("o on self: Only %v, page hidden %v", o, f.Hidden(page))
	}
	h.key("h") // Hide on the Only item: just self hidden
	if _, ok := f.Only(); ok || !f.Hidden(self) || f.Hidden(page) {
		t.Errorf("h on the Only item should hide just it")
	}
	h.keys("up", "up") // back to page
	h.key("left")      // collapse
	if !h.m.chars["fm/kit"].collapsed["page"] || slices.Contains(panelSide(h), "in") {
		t.Errorf("← should collapse page:\n%s", h.screen())
	}
	h.key("right")
	if h.m.chars["fm/kit"].collapsed["page"] {
		t.Error("→ should expand page")
	}
}

func TestCollapsedParentMarksFilterBelow(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	h.keys("down", "down", "h", "up", "left") // hide page/in, collapse page
	if got := panelSide(h)[3]; got != glyphCollapsed+" page "+glyphFiltered {
		t.Errorf("collapsed page row = %q", got)
	}
}

func TestPanelClicks(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	f := h.kitFilter()
	click := func(x, y int) { h.m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}) }
	// Rows: 0 Untagged, 1 buttons, 2 blank, 3 ▼ page, 4 buttons,
	// 5 in, 6 buttons, 7 blank, 8 self, 9 buttons.
	// Buttons for depth 0 start at column 2: "  Hide Only".
	hideX, onlyX := 2, 2+len(str.FilterHide())+1
	click(hideX, 9)
	if !f.Hidden(scene.Item{Name: "self"}) {
		t.Error("clicking self's Hide")
	}
	click(onlyX, 4)
	if o, _ := f.Only(); o != (scene.Item{Name: "page"}) {
		t.Error("clicking page's Only")
	}
	click(1, 3) // the ▼
	if !h.m.chars["fm/kit"].collapsed["page"] {
		t.Error("clicking ▼ should collapse page")
	}
	click(4, 8-2) // rows moved up two: self's name is now row 6
	if h.br().panel.sel.item != (scene.Item{Name: "self"}) {
		t.Errorf("clicking a name selects it: %v", h.br().panel.sel)
	}
}

func TestPanelClicksIgnoredDuringPrompt(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.keys("up", "m", "up", "up", "m", "f") // a range, then the panel
	h.key("esc")                            // close the panel
	h.key("e")                              // export's format prompt
	h.br().togglePanel()                    // panel open under the prompt
	h.screen()
	h.m.Update(tea.MouseClickMsg{X: 2, Y: 3, Button: tea.MouseLeft}) // page's Hide
	h.key("p")                                                       // must not panic
	if h.kitFilter().Active() {
		t.Error("clicks should be ignored while a prompt is open")
	}
}

func TestPanelScrolls(t *testing.T) {
	var lines []string
	for i := range 12 {
		lines = append(lines, fmt.Sprintf("[t%02d] note", i))
	}
	h := newHarness(t, map[string]string{"fm": fmWorld + manyTags(12)})
	h.writeLog(day24, lines...)
	h.key("ctrl+l")
	h.key("f")
	side := panelSide(h)
	// 12 tags × 3 rows + "+ Text" = 37 rows; 23 show above the hint.
	if want := str.ViewMoreBelow(37 - 23); side[len(side)-1] != want {
		t.Fatalf("last row = %q, want %q:\n%s", side[len(side)-1], want, h.screen())
	}
	for range 40 {
		h.key("down")
	}
	if !h.br().panel.sel.add {
		t.Fatalf("down should reach + Text: %v", h.br().panel.sel)
	}
	if !slices.Contains(panelSide(h), str.FilterAddText()) {
		t.Errorf("+ Text not scrolled into view:\n%s", h.screen())
	}
	before := h.br().panel.top
	h.m.Update(tea.MouseWheelMsg{X: 1, Y: 5, Button: tea.MouseWheelUp})
	if h.br().panel.top >= before {
		t.Errorf("wheel up: top %d, was %d", h.br().panel.top, before)
	}
}

// manyTags adds n classify rules to a world: "[tNN] …" gets tag tNN.
func manyTags(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "\n[[classify]]\ntag = \"t%02d\"\npattern = '^\\[t%02d\\]'\n", i, i)
	}
	return b.String()
}

func TestPanelTextRows(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	for range 10 {
		h.key("down") // to + Text
	}
	h.key("enter")
	if h.br().prompt != promptFilterText {
		t.Fatalf("Enter on + Text should ask for a term:\n%s", h.screen())
	}
	h.typeText("lighthouse")
	h.key("enter")
	s := h.screen()
	if !strings.Contains(s, "The lighthouse is dark") || strings.Contains(s, "Sable") {
		t.Errorf("a new text row takes Only:\n%s", s)
	}
	if !slices.Contains(panelSide(h), str.FilterTextItem("lighthouse")) {
		t.Errorf("no text row:\n%s", s)
	}
	// An exact repeat adds nothing; a different term is a second row.
	h.keys("down", "enter") // + Text
	h.typeText("lighthouse")
	h.key("enter")
	h.keys("down", "enter")
	h.typeText("Rook")
	h.key("enter")
	if got := h.kitFilter().Texts(); len(got) != 2 {
		t.Errorf("texts = %v, want lighthouse and Rook", got)
	}
	h.key("up")     // Rook (the new row is highlighted) → lighthouse
	h.key("x")      // removes lighthouse; the highlight moves down onto Rook
	h.key("delete") // removes Rook
	if n := len(h.kitFilter().Texts()); n != 0 {
		t.Errorf("x and Delete should remove text rows, %d left", n)
	}
	if !strings.Contains(h.screen(), "Sable") {
		t.Errorf("removing the Only text row should show lines again:\n%s", h.screen())
	}
}

// manyTagsHarness opens log mode on 12 tags and the filter panel.
func manyTagsHarness(t *testing.T) *harness {
	var lines []string
	for i := range 12 {
		lines = append(lines, fmt.Sprintf("[t%02d] note", i))
	}
	h := newHarness(t, map[string]string{"fm": fmWorld + manyTags(12)})
	h.writeLog(day24, lines...)
	h.key("ctrl+l")
	h.key("f")
	h.screen()
	return h
}

func TestPanelWheelScrollsAndStays(t *testing.T) {
	h := manyTagsHarness(t)
	for range 3 {
		h.m.Update(tea.MouseWheelMsg{X: 1, Y: 5, Button: tea.MouseWheelDown})
		h.screen() // a redraw must not pull the view back to the selection
	}
	if top := h.br().panel.top; top != 9 {
		t.Errorf("three wheel-downs of 3: top = %d, want 9", top)
	}
}

func TestPanelClickOnBottomHintScrolls(t *testing.T) {
	h := manyTagsHarness(t)
	sel := h.br().panel.sel
	h.m.Update(tea.MouseClickMsg{X: 3, Y: h.m.height - 1, Button: tea.MouseLeft})
	h.screen()
	if h.br().panel.top == 0 || h.br().panel.sel != sel || h.kitFilter().Active() {
		t.Errorf("clicking ▼ more: top %d, sel %v, filter active %v", h.br().panel.top, h.br().panel.sel, h.kitFilter().Active())
	}
}

// A tag first seen in an earlier log-mode session lights under a filter
// set since.
func TestFilterRelightsAcrossSessions(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.screen() // session 1 sees self
	h.key("esc")
	h.kitFilter().PressOnly(scene.Item{Name: "page"}, scene.TagItems([]string{"page"})) // self not in the list
	h.key("ctrl+l")
	h.screen()
	if !h.kitFilter().Hidden(scene.Item{Name: "self"}) {
		t.Error("self should light under Only page in a new session")
	}
}

func TestPanelIsFastOnLargeHistories(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.key("ctrl+l")
	b := h.br()
	for i := range 50000 {
		tag := fmt.Sprintf("p%d/c%d", i%10, i%4)
		b.lines = append(b.lines, &bline{e: logstore.Entry{Time: day24, Dir: logstore.In, Text: "x"}, tags: []string{tag}, text: "x", lower: "x", day: "2026-09-24"})
	}
	b.cursor = b.lines[len(b.lines)-1]
	h.key("f")
	for i := range 5 {
		h.br().setCollapsed(filterSel{item: scene.Item{Name: fmt.Sprintf("p%d", i)}}, true)
	}
	for range 60 {
		h.key("down") // to + Text
	}
	start := time.Now()
	for range 20 {
		h.screen()
	}
	if d := time.Since(start); d > 400*time.Millisecond {
		t.Errorf("20 frames with the panel open on 50k lines took %v", d)
	}
}

// The panel takes log mode's own keys: Ctrl+C backs out of it, j/k move.
func TestPanelLogModeKeys(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	h.keys("down", "j") // past Untagged, then j
	if h.br().panel.sel.item != (scene.Item{Name: "page/in"}) {
		t.Errorf("j should move down: %v", h.br().panel.sel)
	}
	h.key("k")
	if h.br().panel.sel.item != (scene.Item{Name: "page"}) {
		t.Errorf("k should move up: %v", h.br().panel.sel)
	}
	h.key("ctrl+c")
	if h.br() == nil || h.br().panel != nil {
		t.Error("Ctrl+C should close the panel and stay in log mode")
	}
}

// Only a parent folds: ← on a leaf leaves nothing behind for children
// that arrive later.
func TestLeftOnLeafDoesNotFold(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	h.keys("down", "down", "left") // self
	if h.m.chars["fm/kit"].collapsed["self"] {
		t.Error("← on self (no children) recorded a fold")
	}
}

// Untagged is the panel's first row, with its own Hide and Only.
func TestPanelUntaggedRow(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	if side := panelSide(h); side[0] != str.FilterUntagged() || side[1] != buttons() {
		t.Fatalf("first rows = %q, %q:\n%s", side[0], side[1], h.screen())
	}
	if h.br().panel.sel.item != (scene.Item{Untagged: true}) {
		t.Errorf("the panel should open on Untagged: %v", h.br().panel.sel)
	}
	h.key("o")
	s := h.screen()
	if !strings.Contains(s, "Sable waves a paw.") || strings.Contains(s, "Mira pages") {
		t.Errorf("Only on Untagged should show just untagged lines:\n%s", s)
	}
}

// A tag with a filter set is listed even when none of its lines is
// loaded, so what hides the log is always on screen.
func TestPanelListsFilteredTagsNotLoaded(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.kitFilter().PressOnly(scene.Item{Name: "whisper/in"}, h.br().items())
	h.key("f")
	side := panelSide(h)
	if !slices.Contains(side, glyphOpen+" whisper") || !slices.Contains(side, "in") {
		t.Errorf("whisper/in (Only, no lines loaded) not listed:\n%s", h.screen())
	}
}
