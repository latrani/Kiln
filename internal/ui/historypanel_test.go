package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/glyph"
	"github.com/latrani/Kiln/internal/str"
)

// sent is a harness with Kit connected, look and say hi sent.
func sent(t *testing.T) *harness {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	for _, s := range []string{"look", "say hi"} {
		h.typeText(s)
		h.enter()
	}
	return h
}

func TestHistoryPanelKeys(t *testing.T) {
	h := sent(t)
	h.typeText("zz")
	h.key("up") // the top row: the panel, the draft its query
	if !h.m.a.History().IsOpen() || !strings.Contains(h.screen(), str.ViewHistoryNoMatch()) {
		t.Fatalf("up didn't open the panel over its query:\n%s", h.screen())
	}
	if l := h.m.layout(); !strings.HasPrefix(xansi.Strip(l.inRows[len(l.inRows)-1]), glyph.Search+"zz") {
		t.Errorf("query row %q; want it marked %s, not %s", xansi.Strip(l.inRows[len(l.inRows)-1]), glyph.Search, gutterMark)
	}
	h.press(tea.KeyBackspace, 0)
	h.press(tea.KeyBackspace, 0)
	if s := h.screen(); !strings.Contains(s, " look") || !strings.Contains(s, gutterMark+"say hi") {
		t.Errorf("cleared query: want both, say hi selected:\n%s", s)
	}
	h.key("up")
	h.key("enter")
	if h.m.a.History().IsOpen() || h.m.input().Value() != "look" || len(h.conn("fm/kit").Sent()) != 3 { // the login, look, say hi
		t.Errorf("enter: open %v, input %q, sent %q; want look to edit, nothing sent", h.m.a.History().IsOpen(), h.m.input().Value(), h.conn("fm/kit").Sent())
	}
	h.key("up")
	h.typeText("x")
	h.key("esc")
	if h.m.a.History().IsOpen() || h.m.input().Value() != "look" {
		t.Errorf("esc: open %v, input %q; want the draft back", h.m.a.History().IsOpen(), h.m.input().Value())
	}
	h.key("up")
	h.key("down") // past the newest
	if h.m.a.History().IsOpen() || h.m.input().Value() != "look" {
		t.Errorf("down past the newest: open %v, input %q", h.m.a.History().IsOpen(), h.m.input().Value())
	}
}

func TestHistoryPanelClicks(t *testing.T) {
	h := sent(t)
	l := h.m.layout()
	chipX := func() int { _, c := h.m.statusBar(l.rw); return l.sw + 1 + c[0] }
	if _, c := h.m.statusBar(l.rw); c[1] == 0 || !strings.Contains(xansi.Strip(h.m.View().Content), str.ViewHistoryButton()) {
		t.Fatal("no History chip in the statusline")
	}
	h.m.Update(tea.MouseClickMsg{X: chipX(), Y: h.m.height - 1, Button: tea.MouseLeft})
	if !h.m.a.History().IsOpen() {
		t.Fatal("the chip didn't open the panel")
	}
	l = h.m.layout()
	row := -1
	for y, i := range l.hist {
		if i == 0 { // look
			row = y
		}
	}
	h.m.Update(tea.MouseClickMsg{X: l.sw + 3, Y: l.top + l.sbH + 1 + row, Button: tea.MouseLeft})
	if h.m.a.History().IsOpen() || h.m.input().Value() != "look" {
		t.Errorf("clicked look: open %v, input %q", h.m.a.History().IsOpen(), h.m.input().Value())
	}
	h.m.Update(tea.MouseClickMsg{X: chipX(), Y: h.m.height - 1, Button: tea.MouseLeft})
	h.m.Update(tea.MouseClickMsg{X: chipX(), Y: h.m.height - 1, Button: tea.MouseLeft})
	if h.m.a.History().IsOpen() {
		t.Error("the chip didn't close the panel")
	}
}

func TestHistoryPanelHeight(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.key("up")
	if l := h.m.layout(); len(l.hist) != 1 || !strings.Contains(h.screen(), str.ViewHistoryNone()) {
		t.Errorf("no history: %d rows; want one, the hint:\n%s", len(l.hist), h.screen())
	}
	h.key("esc")
	for _, s := range []string{"look", "say hi"} {
		h.typeText(s)
		h.enter()
	}
	h.key("up")
	if l := h.m.layout(); len(l.hist) != 2 {
		t.Errorf("two lines sent: %d rows; want two:\n%s", len(l.hist), h.screen())
	}
}
