package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/theme"
)

// withTheme draws with src (on top of the built-in theme) for one test.
func withTheme(t *testing.T, src string) *theme.Theme {
	t.Helper()
	th, err := theme.FromTOML(src)
	if err != nil {
		t.Fatal(err)
	}
	theme.SetActive(th)
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	return th
}

func TestSidebarUsesTheme(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook")
	h.m.chars["fm/rook"].unread, h.m.chars["fm/rook"].attention = 2, true
	th := withTheme(t, `[ui]
sidebar = { bg = "#010203" }
"sidebar.world" = { fg = "#0a0b0c" }
"sidebar.active" = { fg = "#0d0e0f" }
"sidebar.attention" = { fg = "#101112" }
divider = { fg = "#131415" }`)
	s := h.drawn()
	for _, role := range []theme.Role{theme.Sidebar, theme.SidebarWorld, theme.SidebarActive, theme.SidebarAttention, theme.Divider} {
		if !strings.Contains(s, th.SGR(role)) {
			t.Errorf("screen doesn't draw %s", role)
		}
	}
}

func TestPickerUsesTheme(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	th := withTheme(t, `[ui]
"picker.world" = { fg = "#0a0b0c" }
"picker.selected" = { fg = "#0d0e0f" }
"picker.add" = { fg = "#101112" }`)
	h.press('o', tea.ModCtrl)
	s := h.drawn()
	for _, role := range []theme.Role{theme.PickerWorld, theme.PickerSelected, theme.PickerAdd} {
		if !strings.Contains(s, th.SGR(role)) {
			t.Errorf("picker doesn't draw %s", role)
		}
	}
}

func TestInputAreaUsesTheme(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	th := withTheme(t, `[ui]
input = { bg = "#010203" }
"input.over_limit" = { fg = "#0a0b0c" }
"input.hint" = { fg = "#0d0e0f" }`)
	s := h.drawn() // disconnected: the input area shows a hint
	if !strings.Contains(s, th.SGR(theme.InputHint)) {
		t.Error("hint not drawn in input.hint")
	}
	h.typeText("this line runs past twenty bytes")
	if !strings.Contains(h.drawn(), th.SGR(theme.InputOverLimit)) {
		t.Error("over-limit text not drawn in input.over_limit")
	}
}

// TestInputAreaHasNoHoles: every input row is painted edge to edge, and
// a styled stretch inside one doesn't leave the terminal's background
// after it.
func TestInputAreaHasNoHoles(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	th := withTheme(t, "[ui]\ninput = { bg = \"#010203\" }\n")
	h.typeText("this line runs past twenty bytes")
	base := th.SGR(theme.Input)
	l := h.m.layout()
	row := strings.Split(h.drawn(), "\n")[l.sbH+1]
	_, right, _ := strings.Cut(row, "│")
	right = strings.TrimPrefix(right, theme.Reset) // the divider's own reset
	if !strings.HasPrefix(right, base) {
		t.Errorf("input row doesn't start painted: %q", right)
	}
	for _, part := range strings.Split(right, theme.Reset)[1:] {
		if part != "" && !strings.HasPrefix(part, base) {
			t.Errorf("a reset inside the input row isn't followed by the area's colors: %q", right)
			break
		}
	}
}

func TestFormUsesTheme(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	th := withTheme(t, `[ui]
"form.label" = { fg = "#0a0b0c" }
"form.focus" = { fg = "#0d0e0f" }
"form.title" = { fg = "#101112" }
"form.error" = { fg = "#131415" }`)
	h.typeText("/edit world")
	h.enter()
	h.focusOn(saveLabel) // a button draws focus; a text field shows a cursor
	s := h.drawn()
	for _, role := range []theme.Role{theme.FormLabel, theme.FormFocus, theme.FormTitle} {
		if !strings.Contains(s, th.SGR(role)) {
			t.Errorf("editor doesn't draw %s", role)
		}
	}
	h.focusOn(portLabel)
	h.typeText("x")
	if !strings.Contains(h.drawn(), th.SGR(theme.FormError)) {
		t.Error("field error not drawn in form.error")
	}
}
