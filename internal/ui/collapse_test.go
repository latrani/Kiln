package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/glyph"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/theme"
)

// click is a left click at screen (x, y).
func (h *harness) click(x, y int) {
	h.m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
}

// collapseHarness has Kit and Rook open, Kit active.
func collapseHarness(t *testing.T) *harness {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook")
	h.m.switchTo("fm/kit")
	return h
}

// topLine is screen row 0, stripped.
func topLine(h *harness) string { return strings.Split(h.screen(), "\n")[0] }

func TestCollapseByChevron(t *testing.T) {
	h := collapseHarness(t)
	sw := h.m.layout().sw
	if !strings.HasSuffix(strings.TrimRight(screenSide(h, 0), " "), glyph.Collapse) {
		t.Fatalf("no ‹ at the title's right: %q", screenSide(h, 0))
	}
	h.click(sw-1, 0)
	if !h.m.a.SidebarCollapsed() || h.m.layout().sw != 0 || !strings.HasPrefix(topLine(h), glyph.Menu) {
		t.Fatalf("after ‹: collapsed %v, sw %d, top %q", h.m.a.SidebarCollapsed(), h.m.layout().sw, topLine(h))
	}
	h.click(0, 0) // the ≡
	if h.m.a.SidebarCollapsed() || h.m.layout().sw == 0 {
		t.Errorf("≡ didn't bring it back")
	}
}

func TestCtrlBToggles(t *testing.T) {
	h := collapseHarness(t)
	h.key("ctrl+b")
	if !h.m.a.SidebarCollapsed() {
		t.Fatal("ctrl+b didn't collapse")
	}
	h.key("ctrl+b")
	if h.m.a.SidebarCollapsed() {
		t.Error("ctrl+b didn't expand")
	}
}

func TestBadge(t *testing.T) {
	h := collapseHarness(t)
	h.key("ctrl+b")
	h.m.chars["fm/kit"].Unread = 3
	rook := h.m.chars["fm/rook"]
	rook.Unread, rook.Priority = 2, true
	if want := glyph.Menu + " " + glyph.Priority + "5 fm"; !strings.HasPrefix(topLine(h), want) {
		t.Errorf("top %q; want it to start %q", topLine(h), want)
	}
	if drawn := h.m.View().Content; !strings.Contains(drawn, theme.Paint(theme.SidebarPriority, glyph.Priority)) || !strings.Contains(drawn, theme.Paint(theme.SidebarUnread, "5")) {
		t.Error("the badge isn't in the rows' roles")
	}
	rook.Priority = false
	if want := glyph.Menu + " 5 fm"; !strings.HasPrefix(topLine(h), want) {
		t.Errorf("no priority: top %q; want %q", topLine(h), want)
	}
	h.m.chars["fm/kit"].Unread, rook.Unread = 0, 0
	if want := glyph.Menu + " fm"; !strings.HasPrefix(topLine(h), want) {
		t.Errorf("nothing unread: top %q; want %q", topLine(h), want)
	}
}

func TestCollapsedClicksLand(t *testing.T) {
	h := collapseHarness(t)
	h.key("ctrl+b")
	l := h.m.layout()
	if l.rx != 0 || l.rw != h.m.width {
		t.Fatalf("collapsed layout rx %d rw %d; want the whole width", l.rx, l.rw)
	}
	_, logc, _, _ := h.m.topBar(l.rw)
	h.click(l.rx+logc[0], 0)
	if h.m.chars["fm/kit"].browse == nil {
		t.Error("the Log chip, clicked where it's drawn, didn't open log mode")
	}
	h.key("esc")
	_, c := h.m.statusBar(l.rw)
	h.click(l.rx+c.main[0], h.m.height-1)
	if !h.m.a.History().IsOpen() {
		t.Error("the History chip, clicked where it's drawn, didn't open the panel")
	}
}

func TestPickerShowsWhileCollapsed(t *testing.T) {
	h := collapseHarness(t)
	h.key("ctrl+b")
	h.key("ctrl+o")
	if sideTitleText(h) != str.PickerTitle() {
		t.Fatalf("the picker's list isn't showing: %q", screenSide(h, 0))
	}
	if strings.Contains(screenSide(h, 0), glyph.Collapse) {
		t.Error("‹ on a sidebar shown only for the picker")
	}
	h.key("esc")
	if h.m.layout().sw != 0 || !h.m.a.SidebarCollapsed() {
		t.Errorf("after the picker: sw %d, collapsed %v; want collapsed again", h.m.layout().sw, h.m.a.SidebarCollapsed())
	}
}

func TestLogPanelShowsWhileFocused(t *testing.T) {
	h := collapseHarness(t)
	h.m.chars["fm/rook"].Unread = 2
	h.key("ctrl+b")
	h.key("ctrl+l")
	if want := glyph.Menu + " fm"; !strings.HasPrefix(topLine(h), want) {
		t.Errorf("log mode, collapsed: top %q; want the filter panel's ≡, no badge", topLine(h))
	}
	h.key("f")
	if sideTitleText(h) != str.FilterTitle() {
		t.Fatalf("f didn't show the panel: %q", screenSide(h, 0))
	}
	h.key("f")
	if h.m.layout().sw != 0 || !h.m.a.SidebarCollapsed() {
		t.Errorf("f back: sw %d, collapsed %v; want collapsed again", h.m.layout().sw, h.m.a.SidebarCollapsed())
	}
}

func TestCtrlBWithPanelFocused(t *testing.T) {
	h := collapseHarness(t)
	h.key("ctrl+l")
	h.key("f")
	h.key("ctrl+b")
	if !h.m.a.SidebarCollapsed() || h.m.chars["fm/kit"].browse.panelFocus || h.m.layout().sw != 0 {
		t.Errorf("collapsed %v, panel focus %v, sw %d; want collapsed with the log focused", h.m.a.SidebarCollapsed(), h.m.chars["fm/kit"].browse.panelFocus, h.m.layout().sw)
	}
}

// runAll runs cmd and feeds what it says back in, batches and all, one
// level deep: enough for a debounce tick.
func (h *harness) runAll(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			if c != nil {
				h.m.Update(c())
			}
		}
	default:
		h.m.Update(msg)
	}
}

// Collapsing changes the pane's width without the terminal changing:
// the server hears the new width, as it does after a resize.
func TestCollapseReportsWidth(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld}) // 80×24
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	c := h.conn("fm/kit")
	_, cmd := h.m.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	h.runAll(cmd)
	if got := c.Sizes(); got[len(got)-1] != [2]int{80, 24} {
		t.Errorf("sizes after collapsing = %v; want the whole width last", got)
	}
}

// ‹ and ≡ show only where they'd do something: not while the picker or
// an editor is up.
func TestToggleHiddenWhileModal(t *testing.T) {
	h := collapseHarness(t)
	h.key("ctrl+o") // expanded, the picker up
	if strings.Contains(screenSide(h, 0), glyph.Collapse) {
		t.Errorf("‹ with the picker up: %q", screenSide(h, 0))
	}
	h.key("esc")
	h.key("ctrl+b")
	h.key("ctrl+t") // collapsed, an editor up
	if h.m.picker == nil {
		t.Fatal("no editor")
	}
	if strings.HasPrefix(topLine(h), glyph.Menu) {
		t.Errorf("≡ with an editor up: %q", topLine(h))
	}
}

// The badge is part of ≡'s button: a click on it brings the sidebar back.
func TestBadgeClick(t *testing.T) {
	h := collapseHarness(t)
	h.m.chars["fm/rook"].Unread, h.m.chars["fm/rook"].Priority = 2, true
	h.key("ctrl+b")
	h.click(2, 0) // the ●
	if h.m.a.SidebarCollapsed() {
		t.Error("a click on the badge didn't bring the sidebar back")
	}
}
