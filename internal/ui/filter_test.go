package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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
	want := []string{str.FilterTitle(), "", "▼ page", buttons(), "in", buttons(), "", "self", buttons(), "", str.FilterAddText()}
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
	// The chip toggles it too.
	l := h.m.layout()
	h.screen()
	h.m.Update(tea.MouseClickMsg{X: l.sw + 1 + h.br().filterChip[0] + 1, Y: 0, Button: tea.MouseLeft})
	if h.br().panel == nil {
		t.Fatal("clicking the Filter chip should open the panel")
	}
	h.m.Update(tea.MouseClickMsg{X: l.sw + 1 + h.br().filterChip[0] + 1, Y: 0, Button: tea.MouseLeft})
	if h.br().panel != nil {
		t.Error("clicking it again should close the panel")
	}
}

func TestPanelLightsTheItemsOwnState(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	h.kitFilter().PressOnly(scene.Item{Name: "self"}, h.br().items())
	s := h.drawn()
	on := func(label string) string { return theme.Paint(theme.FilterButtonOn, label) }
	if strings.Count(s, on(str.FilterHide())) != 2 || strings.Count(s, on(str.FilterOnly())) != 1 {
		t.Errorf("want Hide lit on page and page/in, Only on self:\n%q", s)
	}
}
