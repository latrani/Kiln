package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/str"
)

// logHarness is Kit in log mode over scene1.
func logHarness(t *testing.T) *harness {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	return h
}

func TestLogModePanelAlwaysUp(t *testing.T) {
	h := logHarness(t)
	if got := strings.TrimSpace(screenSide(h, 0)); got != str.FilterTitle() {
		t.Errorf("sidebar title = %q; want the filter panel's", got)
	}
	if side := panelSide(h); side[0] != str.FilterUntagged() {
		t.Errorf("first panel row = %q", side[0])
	}
	if want := chipText(str.ViewPresenceUnknown()) + str.Separator() + chipText(str.ViewLogButton()); !strings.HasSuffix(rightRow(h, 0)+" ", want) {
		t.Errorf("top bar %q; want only presence and Log after the name", rightRow(h, 0))
	}
}

func TestLogModeFocus(t *testing.T) {
	h := logHarness(t)
	sel, cur := h.br().Panel.Sel, h.br().Cursor
	h.key("up")
	if h.br().Panel.Sel != sel || h.br().Cursor == cur {
		t.Fatal("up with the log focused should move the cursor, not the highlight")
	}
	h.key("f")
	cur = h.br().Cursor
	h.key("down")
	if h.br().Panel.Sel == sel || h.br().Cursor != cur {
		t.Fatal("down with the panel focused should move the highlight, not the cursor")
	}
	h.key("esc")
	if h.br() == nil || h.br().panelFocus {
		t.Fatal("esc in the panel should give the log focus, staying in log mode")
	}
	h.key("esc")
	if h.br() != nil {
		t.Error("esc in the log, nothing marked or found, should leave log mode")
	}
}

func TestLogModeKeysFromEitherSide(t *testing.T) {
	h := logHarness(t)
	h.key("f")
	h.key("s")
	if !h.br().Marking {
		t.Error("s with the panel focused should start select mode")
	}
	h.key("esc") // focus to the log
	h.key("esc") // drop the range
	item := h.br().Panel.Sel.Item
	h.key("h")
	if !h.kitFilter().Hidden(item) {
		t.Errorf("h with the log focused should hide the highlighted %v", item)
	}
}

func TestLogModeClickFocuses(t *testing.T) {
	h := logHarness(t)
	l := h.m.layout()
	h.m.Update(tea.MouseClickMsg{X: 3, Y: sideTitleH, Button: tea.MouseLeft}) // Untagged's name
	if !h.br().panelFocus {
		t.Fatal("a click in the panel should focus it")
	}
	h.m.Update(tea.MouseClickMsg{X: l.sw + 10, Y: l.top + 2, Button: tea.MouseLeft})
	if h.br().panelFocus {
		t.Error("a click in the log should focus it")
	}
}

func TestLogModePromptOwnsKeys(t *testing.T) {
	h := logHarness(t)
	h.key("f")
	for !h.br().Panel.Sel.Add {
		h.key("down")
	}
	h.key("enter")
	h.typeText("fhs")
	if h.br().pin.Value() != "fhs" || !h.br().panelFocus || h.br().Marking {
		t.Errorf("prompt %q, panel focus %v, marking %v; the prompt should take every key", h.br().pin.Value(), h.br().panelFocus, h.br().Marking)
	}
	h.key("esc")
	if h.br().prompt != promptNone || !h.br().panelFocus {
		t.Errorf("esc: prompt %v, panel focus %v; want the prompt gone and the panel still focused", h.br().prompt, h.br().panelFocus)
	}
}

func TestLogModeFindThenPanel(t *testing.T) {
	h := logHarness(t)
	h.key("/")
	h.typeText("Rook")
	h.key("enter")
	if h.br().Find == "" {
		t.Fatal("no find")
	}
	h.key("f")
	sel := h.br().Panel.Sel
	h.key("down")
	if !h.br().panelFocus || h.br().Panel.Sel == sel {
		t.Fatal("with find on, f then down should move the panel's highlight")
	}
	h.key("esc")
	if h.br().panelFocus || h.br().Find == "" {
		t.Errorf("esc in the panel: focus %v, find %q; want the log focused, find still on", h.br().panelFocus, h.br().Find)
	}
	h.key("esc")
	if h.br() == nil || h.br().Find != "" {
		t.Error("the next esc should end find, staying in log mode")
	}
}

func TestLogModeHideKeepsFocus(t *testing.T) {
	h := logHarness(t)
	h.keys("f", "down")
	sel := h.br().Panel.Sel
	h.keys("ctrl+l", "ctrl+l")
	if h.br() == nil || !h.br().panelFocus || h.br().Panel.Sel != sel {
		t.Errorf("back from ctrl+l: focus and highlight should be as left")
	}
}

func TestLogModeNoSwitching(t *testing.T) {
	h := logHarness(t)
	h.open("fm/rook")
	h.m.switchTo("fm/kit")
	h.m.chars["fm/rook"].Unread = 1
	h.press(tea.KeyTab, 0)
	h.press(tea.KeyTab, tea.ModShift)
	h.press(tea.KeyUp, tea.ModCtrl)
	h.press(tea.KeyDown, tea.ModCtrl)
	if h.m.a.Active() != "fm/kit" {
		t.Errorf("active %s; switching keys do nothing in log mode", h.m.a.Active())
	}
}

var _ = xansi.Strip
