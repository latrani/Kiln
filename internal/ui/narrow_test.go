package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/str"
)

// narrowHarness is collapseHarness at 60×24: narrow.
func narrowHarness(t *testing.T) *harness {
	h := collapseHarness(t)
	h.m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	return h
}

// screenRow is screen row y, trimmed on the right.
func screenRow(h *harness, y int) string {
	return strings.TrimRight(strings.Split(h.screen(), "\n")[y], " ")
}

// rowOf is the screen row starting (after spaces) with s; -1 for none.
func rowOf(h *harness, s string) int {
	for y, r := range strings.Split(h.screen(), "\n") {
		if strings.HasPrefix(strings.TrimSpace(r), s) {
			return y
		}
	}
	return -1
}

func TestNarrowStartsClosed(t *testing.T) {
	h := narrowHarness(t)
	if h.m.layout().sw != 0 || !strings.HasPrefix(topLine(h), menuButton) {
		t.Errorf("sw %d, top %q; want no column and ≡", h.m.layout().sw, topLine(h))
	}
}

func TestNarrowOpenAndPick(t *testing.T) {
	h := narrowHarness(t)
	h.key("ctrl+b")
	if r := screenRow(h, 0); !strings.HasPrefix(r, str.ViewConnectionsTitle()) || !strings.HasSuffix(r, collapseButton) || len([]rune(strings.Split(h.screen(), "\n")[0])) != 60 {
		t.Fatalf("row 0 %q; want the title across the screen, ‹ at its right", r)
	}
	if h.m.layout().rw != 60 {
		t.Errorf("rw %d; the pane keeps its width under the sidebar", h.m.layout().rw)
	}
	h.click(3, rowOf(h, "× Rook"))
	if h.m.a.Active() != "fm/rook" || h.m.narrowOpen {
		t.Fatalf("clicked Rook: active %s, open %v", h.m.a.Active(), h.m.narrowOpen)
	}
	h.key("ctrl+b")
	h.click(3, rowOf(h, "fm"))
	if h.m.a.Active() != worldSel("fm") || h.m.narrowOpen {
		t.Errorf("clicked the world: active %s, open %v", h.m.a.Active(), h.m.narrowOpen)
	}
	h.m.switchTo("fm/kit")
	h.key("ctrl+b")
	h.key("esc")
	if h.m.narrowOpen {
		t.Error("esc didn't close it")
	}
	h.key("ctrl+b")
	h.click(59, 0) // ‹
	if h.m.narrowOpen {
		t.Error("‹ didn't close it")
	}
}

func TestNarrowPicker(t *testing.T) {
	h := narrowHarness(t)
	h.key("ctrl+o")
	if sideTitleText(h) != str.PickerTitle() || h.m.layout().sw != 0 {
		t.Fatalf("the picker's list isn't showing across: %q", screenRow(h, 0))
	}
	h.key("esc")
	if h.m.narrowOpen || h.m.narrowShown() {
		t.Error("esc out of the picker left it open")
	}
}

func TestNarrowLogMode(t *testing.T) {
	h := narrowHarness(t)
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	if h.m.narrowShown() {
		t.Fatal("log mode opened with the panel")
	}
	h.key("f")
	if !h.m.narrowShown() || sideTitleText(h) != str.FilterTitle() || !h.br().panelFocus {
		t.Fatalf("f: shown %v, title %q", h.m.narrowShown(), screenRow(h, 0))
	}
	if !strings.Contains(h.screen(), firstPart(str.BrowseHintsNoneAway())) {
		t.Errorf("the log's bar isn't there under the panel:\n%s", h.screen())
	}
	h.key("esc")
	if h.m.narrowShown() || h.br().panelFocus {
		t.Error("esc in the panel didn't close it")
	}
	h.key("ctrl+b")
	if !h.m.narrowShown() || !h.br().panelFocus {
		t.Error("ctrl+b didn't open the panel with focus")
	}
	h.key("ctrl+b")
	if h.m.narrowShown() {
		t.Error("ctrl+b didn't close it")
	}
	item := h.br().Panel.Sel.Item
	h.key("h")
	if !h.kitFilter().Hidden(item) {
		t.Error("h from the log didn't reach the hidden panel")
	}
}

func TestNarrowClicksLand(t *testing.T) {
	h := narrowHarness(t)
	h.key("ctrl+b")
	h.click(badgeX, rowOf(h, "× Rook")) // Rook isn't connected: its ×
	if h.m.chars["fm/rook"] != nil {
		t.Error("the × didn't close Rook")
	}
}

func TestNarrowResize(t *testing.T) {
	h := collapseHarness(t)
	h.key("ctrl+b") // wide, collapsed
	h.m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	if h.m.narrowShown() {
		t.Fatal("narrow started open")
	}
	h.key("ctrl+b")
	h.m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	if h.m.layout().sw != 0 || !h.m.a.SidebarCollapsed() {
		t.Errorf("wide again: sw %d, collapsed %v; want collapsed as left", h.m.layout().sw, h.m.a.SidebarCollapsed())
	}
	h.m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	if h.m.narrowShown() {
		t.Error("narrow again came back open")
	}
}

// The wheel over the open narrow panel scrolls the panel, not the log
// hidden under it.
func TestNarrowPanelWheel(t *testing.T) {
	h := narrowHarness(t)
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	h.m.Update(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelDown})
	if h.br().scrolled {
		t.Error("the wheel over the panel scrolled the hidden log")
	}
}

// The second click of a double-click on a narrow row lands on what's
// under it once the sidebar's closed: it does nothing.
func TestNarrowDoubleClickDoesNothingUnder(t *testing.T) {
	h := narrowHarness(t)
	h.key("ctrl+b")
	y := rowOf(h, "fm")
	h.click(3, y)
	h.screen()    // the overview, drawn: its parts are clickable
	h.click(3, y) // on the overview now, where Kit's part is
	if h.m.a.Active() != worldSel("fm") {
		t.Errorf("the second click switched to %s; want fm's overview still", h.m.a.Active())
	}
	h.advance(doubleClick + time.Millisecond)
	h.screen()
	h.click(3, y)
	if h.m.a.Active() == worldSel("fm") {
		t.Error("a later click on the overview did nothing")
	}
}

// Opening log mode, or becoming narrow, starts the narrow sidebar
// closed: whatever was open before doesn't come back.
func TestNarrowClosesOnLogModeAndResize(t *testing.T) {
	h := narrowHarness(t)
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	h.key("ctrl+l")
	h.key("ctrl+l") // back out
	if h.m.narrowShown() {
		t.Error("the sidebar came back open after log mode")
	}
	h.m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	h.key("ctrl+l")
	h.key("f") // wide: the panel focused
	h.m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	if h.m.narrowShown() || h.br().panelFocus {
		t.Error("becoming narrow opened the panel over the log")
	}
}

// The picker's add-character field and its wrapped error, and the
// scroll hints, at the narrow sidebar's full width.
func TestNarrowAddCharacterAndHints(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	h.press('o', tea.ModCtrl)
	h.press(tea.KeyDown, 0) // Rook, then fm's add row
	h.enter()
	if h.m.picker == nil || h.m.picker.edit == nil {
		t.Fatalf("no character editor:\n%s", h.screen())
	}
	h.typeText("Ash")
	if c := h.m.View().Cursor; c == nil || c.Y >= h.m.narrowH() || c.X != 2+len("Ash") {
		t.Errorf("cursor %+v; want it in the name field, in the open sidebar", c)
	}
	h.typeText(strings.Repeat("!", 3))
	if s := h.screen(); !strings.Contains(s, "Ash!!!") {
		t.Errorf("the name isn't drawn across:\n%s", s)
	}
	h2 := newHarness(t, map[string]string{"big": manyChars(30)})
	h2.openAll()
	h2.m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	h2.key("ctrl+b")
	if s := h2.screen(); !strings.Contains(s, "▼ ") || !strings.Contains(s, "more") {
		t.Errorf("no scroll hint in the open narrow sidebar:\n%s", s)
	}
}
