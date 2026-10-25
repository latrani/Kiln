package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/glyph"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/scene"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/theme"
)

// panelSide is the sidebar column's rows, trimmed of spaces and the
// tree's lines.
func panelSide(h *harness) []string {
	rows := strings.Split(h.screen(), "\n")
	w := slices.Index([]rune(rows[0]), '│') // the divider, past the title
	var out []string
	for _, row := range rows[sideTitleH:] {
		r := []rune(row)
		if w >= 0 {
			r = r[:min(w, len(r))]
		} else if i := slices.Index(r, '│'); i >= 0 {
			r = r[:i]
		}
		side := strings.TrimLeft(string(r), " "+glyphTreeLine)
		if t := strings.TrimLeft(side, glyphTreeBranch+glyphTreeLast); t != side {
			side = strings.TrimPrefix(t, glyph.Rule)
		}
		out = append(out, strings.TrimSpace(side))
	}
	return out
}

// unfold opens every parent the panel has met, for tests that aren't
// about folding: parents start folded.
func unfold(h *harness) {
	h.br().Entries()
	clear(h.m.chars["fm/kit"].Collapsed)
}

func buttons() string { return str.FilterHide() + str.Separator() + str.FilterOnly() }

// The groups' rows, open, as panelSide trims them.
func catsRow() string  { return glyphOpen + " " + str.FilterCategories() }
func flagsRow() string { return glyphOpen + " " + str.FilterFlags() }

func TestPanelOpensAndCloses(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	side := panelSide(h)
	// Parents start folded; → opens one.
	want := []string{catsRow(), buttons(), glyphCollapsed + " page", buttons(), classify.SentTag, buttons(), flagsRow(), buttons(), "self", buttons(), "", str.FilterAddText()}
	for i, w := range want {
		if side[i] != w {
			t.Fatalf("panel row %d = %q, want %q:\n%s", i, side[i], w, h.screen())
		}
	}
	h.keys("down", "right")
	want = []string{catsRow(), buttons(), glyphOpen + " page", buttons(), "in", buttons(), classify.SentTag, buttons(), flagsRow(), buttons(), "self", buttons(), "", str.FilterAddText()}
	for i, w := range want {
		if side := panelSide(h); side[i] != w {
			t.Fatalf("opened: panel row %d = %q, want %q:\n%s", i, side[i], w, h.screen())
		}
	}
	h.key("f")
	if h.br().panelFocus || panelSide(h)[0] != catsRow() {
		t.Errorf("f should give the log focus, the panel staying:\n%s", h.screen())
	}
	h.key("f")
	h.key("esc")
	if h.br() == nil || h.br().panelFocus {
		t.Error("Esc should give the log focus and stay in log mode")
	}
}

func TestPanelLightsTheItemsOwnState(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	unfold(h)
	h.kitFilter().PressOnly(scene.Item{Name: "self"}, h.br().Items())
	s := h.drawn()
	on := func(label string) string { return theme.Paint(theme.FilterButtonOn, label) }
	if strings.Count(s, on(str.FilterHide())) != 1 || strings.Count(s, on(str.FilterOnly())) != 1 {
		t.Errorf("want Hide lit on just sent, by default (self is a flag: its Only lights no category), Only on self:\n%q", s)
	}
	h.kitFilter().PressOnly(scene.Item{Name: "page/in"}, h.br().Items())
	if s := h.drawn(); strings.Count(s, on(str.FilterHide())) != 1 || strings.Count(s, on(str.FilterOnly())) != 2 {
		t.Errorf("want Hide lit on sent, Only on self and page/in (Categories and page are its ancestors, so stay unlit):\n%q", s)
	}
}

func TestPanelKeys(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	unfold(h)
	b, f := h.br(), h.kitFilter()
	page, in, self := scene.Item{Name: "page"}, scene.Item{Name: "page/in"}, scene.Item{Name: "self"}
	h.key("down") // past Categories
	if b.Panel.Sel.Item != page {
		t.Fatalf("starts on %v", b.Panel.Sel)
	}
	h.key("h")
	if !f.Hidden(page) || !f.Hidden(in) || strings.Contains(h.screen(), "Mira pages") {
		t.Errorf("h on page should hide it and page/in:\n%s", h.screen())
	}
	h.keys("down", "h") // page/in: unhide clears page, keeps nothing else hidden
	if f.Hidden(page) || f.Hidden(in) {
		t.Errorf("unhiding page/in: page %v, page/in %v", f.Hidden(page), f.Hidden(in))
	}
	h.keys("down", "down", "down", "o") // past sent and Flags to self: Only, a flag's, lighting no category
	if !f.Only(self) || f.Hidden(page) {
		t.Errorf("o on self: Only %v, page hidden %v", f.Only(self), f.Hidden(page))
	}
	h.key("h") // Hide on the Only item: just self hidden
	if f.Only(self) || !f.Hidden(self) || f.Hidden(page) {
		t.Errorf("h on the Only item should hide just it")
	}
	h.keys("up", "up", "up", "up") // back to page
	h.key("left")                  // collapse
	if !h.m.chars["fm/kit"].Collapsed[scene.Item{Name: "page"}] || slices.Contains(panelSide(h), "in") {
		t.Errorf("← should collapse page:\n%s", h.screen())
	}
	h.key("right")
	if h.m.chars["fm/kit"].Collapsed[scene.Item{Name: "page"}] {
		t.Error("→ should expand page")
	}
}

func TestCollapsedParentMarksFilterBelow(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	unfold(h)
	h.keys("down", "down", "h", "up", "left") // hide page/in, collapse page
	if got := panelSide(h)[2]; got != glyphCollapsed+" page "+glyphFiltered {
		t.Errorf("collapsed page row = %q", got)
	}
}

// Enter and Space fold a tag as ←/→ do, toggling it; on a tag with no
// children they do nothing.
func TestPanelFoldToggles(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	collapsed := func() bool { return h.m.chars["fm/kit"].Collapsed[scene.Item{Name: "page"}] }
	h.key("down") // page, folded
	for _, step := range []struct {
		key  string
		want bool
	}{{"enter", false}, {"space", true}, {"right", false}, {"right", false}, {"space", true}} {
		h.key(step.key)
		if collapsed() != step.want {
			t.Fatalf("after %s, page folded = %v:\n%s", step.key, collapsed(), h.screen())
		}
	}
	if h.br().prompt != promptNone {
		t.Errorf("Enter on a tag opened a prompt")
	}
}

func TestPanelClicks(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	unfold(h)
	f := h.kitFilter()
	click := func(x, y int) { h.m.Update(tea.MouseClickMsg{X: x, Y: sideTitleH + y, Button: tea.MouseLeft}) }
	// Rows: 0 ▼ Categories, 1 buttons, 2 ├─▼ page, 3 buttons, 4 │ └─in,
	// 5 buttons, 6 └─sent, 7 buttons, 8 ▼ Flags, 9 buttons, 10 └─self,
	// 11 buttons. Hide and Only sit a column in from the name: self's at
	// 3, page's (past its ▼) at 5.
	only := xansi.StringWidth(str.FilterHide() + str.Separator())
	click(2, 11) // left of Hide: nothing
	if f.Hidden(scene.Item{Name: "self"}) {
		t.Error("clicking left of self's Hide hid it")
	}
	click(3, 11)
	if !f.Hidden(scene.Item{Name: "self"}) {
		t.Errorf("clicking self's Hide:\n%s", h.screen())
	}
	click(5+only, 3)
	if !f.Only(scene.Item{Name: "page"}) {
		t.Error("clicking page's Only")
	}
	click(2, 2) // the ▼
	if !h.m.chars["fm/kit"].Collapsed[scene.Item{Name: "page"}] {
		t.Error("clicking ▼ should collapse page")
	}
	click(4, 10-2) // rows moved up two: self's name is now row 8
	if h.br().Panel.Sel.Item != (scene.Item{Name: "self"}) {
		t.Errorf("clicking a name selects it: %v", h.br().Panel.Sel)
	}
}

func TestPanelClicksIgnoredDuringPrompt(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.keys("up", "s", "up", "up", "s") // a range
	h.key("e")                         // export's format prompt, the panel beside it
	h.screen()
	h.m.Update(tea.MouseClickMsg{X: 2, Y: sideTitleH + 3, Button: tea.MouseLeft}) // page's Hide
	h.key("p")                                                                    // must not panic
	if h.kitFilter().Hidden(scene.Item{Name: "page"}) {
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
	side, listH := panelSide(h), h.br().Panel.listH
	// Categories, 12 tags and sent × 2 rows, a blank, + Text: 30 rows; all
	// but one of the list's rows show, above the hint, which is its last.
	if want := str.ViewMoreBelow(30 - (listH - 1)); side[listH-1] != want {
		t.Fatalf("the list's last row = %q, want %q:\n%s", side[listH-1], want, h.screen())
	}
	for range 40 {
		h.key("down")
	}
	if !h.br().Panel.Sel.Add {
		t.Fatalf("down should reach + Text: %v", h.br().Panel.Sel)
	}
	if !slices.Contains(panelSide(h), str.FilterAddText()) {
		t.Errorf("+ Text not scrolled into view:\n%s", h.screen())
	}
	before := h.br().Panel.top
	h.m.Update(tea.MouseWheelMsg{X: 1, Y: 5, Button: tea.MouseWheelUp})
	if h.br().Panel.top >= before {
		t.Errorf("wheel up: top %d, was %d", h.br().Panel.top, before)
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
	if !slices.Contains(panelSide(h), glyphRemove+" "+str.FilterTextItem("lighthouse")) {
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
	if top := h.br().Panel.top; top != 9 {
		t.Errorf("three wheel-downs of 3: top = %d, want 9", top)
	}
}

// filterState is each panel item's Hide and Only, to tell whether a
// press changed any.
func filterState(h *harness) string {
	var b strings.Builder
	f := h.kitFilter()
	for _, it := range h.br().Items() {
		fmt.Fprintf(&b, "%v:%v/%v ", it, f.Hidden(it), f.Only(it))
	}
	return b.String()
}

func TestPanelClickOnBottomHintScrolls(t *testing.T) {
	h := manyTagsHarness(t)
	sel, before := h.br().Panel.Sel, filterState(h)
	h.m.Update(tea.MouseClickMsg{X: 3, Y: sideTitleH + h.br().Panel.listH - 1, Button: tea.MouseLeft})
	h.screen()
	if h.br().Panel.top == 0 || h.br().Panel.Sel != sel || filterState(h) != before {
		t.Errorf("clicking ▼ more: top %d, sel %v, filter %s, was %s", h.br().Panel.top, h.br().Panel.Sel, filterState(h), before)
	}
}

// A tag first seen in an earlier log-mode session lights under a filter
// set since.
func TestFilterRelightsAcrossSessions(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.screen() // session 1 sees page
	h.key("esc")
	h.kitFilter().PressOnly(scene.Item{Name: classify.SentTag}, scene.TagItems([]string{classify.SentTag})) // page not in the list
	h.key("ctrl+l")
	h.screen()
	if !h.kitFilter().Hidden(scene.Item{Name: "page"}) {
		t.Error("page should light under Only sent in a new session")
	}
}

func TestPanelIsFastOnLargeHistories(t *testing.T) {
	scales(t, "20 frames with the panel open", func(n int) time.Duration {
		h := newHarness(t, map[string]string{"fm": fmWorld})
		h.key("ctrl+l")
		b := h.br()
		for i := range n {
			tag := fmt.Sprintf("p%d/c%d", i%10, i%4)
			b.Lines = append(b.Lines, &app.LogLine{Line: app.Line{Seq: logSeqAfter(b.Lines), Entry: logstore.Entry{Time: day24, Dir: logstore.In, Text: "x"}, Day: "2026-09-24"}, Tags: []string{tag}, Lower: "x"})
		}
		b.Cursor = b.Lines[len(b.Lines)-1]
		h.key("f")
		for i := range 5 {
			h.br().Fold(scene.Item{Name: fmt.Sprintf("p%d", i)}, true)
		}
		for range 60 {
			h.key("down") // to + Text
		}
		start := time.Now()
		for range 20 {
			h.screen()
		}
		return time.Since(start)
	})
}

// The panel takes log mode's own keys: Ctrl+C backs out of it, j/k move.
func TestPanelLogModeKeys(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	unfold(h)
	h.keys("down", "j") // past Categories, then j
	if h.br().Panel.Sel.Item != (scene.Item{Name: "page/in"}) {
		t.Errorf("j should move down: %v", h.br().Panel.Sel)
	}
	h.key("k")
	if h.br().Panel.Sel.Item != (scene.Item{Name: "page"}) {
		t.Errorf("k should move up: %v", h.br().Panel.Sel)
	}
	h.key("ctrl+c")
	if h.br() == nil || h.br().panelFocus {
		t.Error("Ctrl+C should give the log focus and stay in log mode")
	}
}

// Only a parent folds: ← on a leaf leaves nothing behind for children
// that arrive later.
func TestLeftOnLeafDoesNotFold(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	h.keys("down", "down", "down", "down", "left") // past page, sent and Flags to self
	if h.m.chars["fm/kit"].Collapsed[scene.Item{Name: "self"}] {
		t.Error("← on self (no children) recorded a fold")
	}
}

// Categories is the panel's first row, open, with its own Hide and
// Only: Hide leaves the lines with no category, Only just the others.
func TestPanelCategoriesRow(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	if side := panelSide(h); side[0] != catsRow() || side[1] != buttons() {
		t.Fatalf("first rows = %q, %q:\n%s", side[0], side[1], h.screen())
	}
	if h.br().Panel.Sel.Item != (scene.Item{Group: scene.CategoryGroup}) {
		t.Errorf("the panel should open on Categories: %v", h.br().Panel.Sel)
	}
	h.key("h")
	if s := h.screen(); !strings.Contains(s, "Sable waves a paw.") || strings.Contains(s, "Mira pages") {
		t.Errorf("Hide on Categories should show just lines with no category:\n%s", s)
	}
	h.key("o")
	if s := h.screen(); strings.Contains(s, "Sable waves a paw.") || !strings.Contains(s, "Mira pages") {
		t.Errorf("Only on Categories should show just lines with one:\n%s", s)
	}
}

// A tag with a filter set is listed even when none of its lines is
// loaded, so what hides the log is always on screen.
func TestPanelListsFilteredTagsNotLoaded(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.kitFilter().PressOnly(scene.Item{Name: "whisper/in"}, h.br().Items())
	h.key("f")
	unfold(h)
	side := panelSide(h)
	if !slices.Contains(side, glyphOpen+" whisper") || !slices.Contains(side, "in") {
		t.Errorf("whisper/in (Only, no lines loaded) not listed:\n%s", h.screen())
	}
}

// A text row starts with a ×, and clicking it removes the row.
func TestPanelTextRowRemovesWithClickOnX(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	b := h.br()
	for range 10 {
		h.key("down") // to + Text
	}
	h.key("enter")
	h.typeText("Rook")
	h.key("enter")
	if !slices.Contains(panelSide(h), glyphRemove+" "+str.FilterTextItem("Rook")) {
		t.Fatalf("no × on the text row:\n%s", h.screen())
	}
	rowOf := func() int {
		for i, r := range b.panelRows(0) {
			if r.kind == prName && r.sel.Item.Text {
				return i
			}
		}
		return -1
	}
	y := rowOf()
	b.panelClick(3, y, 100) // on the name, not the ×: selects it, removes nothing
	if len(h.kitFilter().Texts()) != 1 {
		t.Error("clicking a text row's name removed it")
	}
	b.panelClick(0, y, 100) // the ×
	if n := len(h.kitFilter().Texts()); n != 0 {
		t.Errorf("clicking × left %d text rows", n)
	}
}

// Every parent folds the first time the panel sees it, however deep, and
// stays as the reader leaves it.
func TestNestedParentsStartFolded(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	b, cs := h.br(), h.m.chars["fm/kit"]
	items := []scene.Item{{Name: "a"}, {Name: "a/b"}, {Name: "a/b/c"}, {Name: "z"}}
	b.EntriesOf(items)
	if !cs.Collapsed[scene.Item{Name: "a"}] || !cs.Collapsed[scene.Item{Name: "a/b"}] || cs.Collapsed[scene.Item{Name: "a/b/c"}] || cs.Collapsed[scene.Item{Name: "z"}] {
		t.Errorf("collapsed = %v, want a and a/b only", cs.Collapsed)
	}
	var names []string
	for _, e := range b.entriesOf(items) {
		if !e.Add {
			names = append(names, e.Item.Name)
		}
	}
	if strings.Join(names, " ") != "a z" {
		t.Errorf("listed %v, want just a and z", names)
	}
	delete(cs.Collapsed, scene.Item{Name: "a"}) // the reader opens a
	b.EntriesOf(items)
	if cs.Collapsed[scene.Item{Name: "a"}] || !cs.Collapsed[scene.Item{Name: "a/b"}] {
		t.Errorf("after opening a: %v, a should stay open", cs.Collapsed)
	}
}

// The highlight stops at the top, and removing the last text row leaves
// it on + Text.
func TestPanelHighlightStaysPut(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	h.key("up")
	if h.br().Panel.Sel.Item != (scene.Item{Group: scene.CategoryGroup}) {
		t.Errorf("up at the top moved to %v", h.br().Panel.Sel)
	}
	for range 10 {
		h.key("down") // to + Text
	}
	h.key("enter")
	h.typeText("lighthouse")
	h.key("enter") // the new row is highlighted
	h.key("x")
	if !h.br().Panel.Sel.Add {
		t.Errorf("after removing the last text row: %v; want + Text", h.br().Panel.Sel)
	}
}

// + Text becomes the field the text is typed in, as + Character does,
// its keys in the action bar; Enter adds it, Esc puts + Text back.
func TestPanelAddTextInline(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	for range 10 {
		h.key("down") // to + Text
	}
	h.key("enter")
	side := panelSide(h)
	if slices.Contains(side, str.FilterAddText()) || !slices.Contains(side, str.FilterField()) || logBar(h) != str.FilterFieldHint() {
		t.Fatalf("Enter on + Text should open the field in its place, hint %q, keys in the bar:\n%s", str.FilterField(), h.screen())
	}
	h.typeText("Rook")
	row := slices.Index(panelSide(h), "Rook")
	if row < 0 {
		t.Fatalf("typed text not in the panel:\n%s", h.screen())
	}
	if c := h.m.View().Cursor; c == nil || c.Y != sideTitleH+row || c.X != 1+len("Rook") {
		t.Errorf("cursor %v; want after Rook in the field, row %d", c, sideTitleH+row)
	}
	h.key("esc")
	if !slices.Contains(panelSide(h), str.FilterAddText()) || len(h.kitFilter().Texts()) != 0 {
		t.Errorf("Esc should put + Text back, adding nothing:\n%s", h.screen())
	}
	h.key("enter")
	h.typeText("Rook")
	h.key("enter")
	if !slices.Contains(panelSide(h), glyphRemove+" "+str.FilterTextItem("Rook")) || !slices.Contains(panelSide(h), str.FilterAddText()) {
		t.Errorf("Enter should add the text row and put + Text back:\n%s", h.screen())
	}
}
