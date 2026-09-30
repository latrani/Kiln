package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/str"
)

// rightRow is screen row y's right pane, trimmed.
func rightRow(h *harness, y int) string {
	return strings.TrimSpace(strings.SplitN(strings.Split(h.screen(), "\n")[y], "│", 2)[1])
}

func chipText(label string) string { return " " + label + " " }

func TestTopBar(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	top := rightRow(h, 0)
	if !strings.HasPrefix(top, "fm"+str.Separator()+"Kit") || !strings.HasSuffix(top, strings.TrimSpace(chipText(str.ViewLogButton()))) {
		t.Errorf("normal top bar = %q", top)
	}
	h.key("ctrl+l")
	h.key("/")
	h.typeText("Mira")
	h.key("enter")
	top = rightRow(h, 0)
	want := chipText(str.ViewFilterButton()) + str.Separator() + chipText(str.ViewLogButton())
	if !strings.HasPrefix(top, "fm"+str.Separator()+"Kit") || !strings.Contains(top, str.BrowseFindStatus("Mira", 1, 1)) ||
		!strings.HasSuffix(top+" ", want) {
		t.Errorf("log top bar = %q, want name, find status, then %q", top, want)
	}
	if strings.Contains(h.screen(), "→") {
		t.Errorf("the date range should be gone:\n%s", h.screen())
	}
}

func TestTopBarChipsToggle(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	l := h.m.layout()
	click := func(x int) { h.m.Update(tea.MouseClickMsg{X: l.sw + 1 + x, Y: 0, Button: tea.MouseLeft}) }
	_, logc, _ := h.m.topBar(l.rw)
	click(logc[0] + 1)
	if h.br() == nil {
		t.Fatal("clicking Log should open log mode")
	}
	_, _, filt := h.m.topBar(l.rw)
	click(filt[0] + 1)
	if h.br().panel == nil {
		t.Fatal("clicking Filter should open the panel")
	}
	click(filt[0] + 1)
	if h.br().panel != nil {
		t.Error("clicking Filter again should close it")
	}
	_, logc, _ = h.m.topBar(l.rw)
	click(logc[0] + 1)
	if h.br() != nil {
		t.Error("clicking Log in log mode should close it")
	}
}

func TestTopBarNarrow(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.chars["fm/kit"].ch.Name = strings.Repeat("K", 80)
	h.m.Update(tea.WindowSizeMsg{Width: MinWidth, Height: 24})
	top := rightRow(h, 0)
	if !strings.HasSuffix(top, strings.TrimSpace(chipText(str.ViewLogButton()))) || !strings.Contains(top, "…") {
		t.Errorf("narrow top bar = %q, want the name cut before the chip", top)
	}
}

func TestClicksLandBelowTheTopBar(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.screen()
	b := h.br()
	l := h.m.layout()
	for row, bl := range b.rowLines {
		if bl != nil && bl.e.Text == scene1[1] {
			h.m.Update(tea.MouseClickMsg{X: l.sw + 11, Y: l.top + row, Button: tea.MouseLeft})
		}
	}
	if b.cursor.e.Text != scene1[1] {
		t.Errorf("a click on a log line's screen row selected %q", b.cursor.e.Text)
	}
	h.m.Update(tea.MouseClickMsg{X: 3, Y: 1, Button: tea.MouseLeft}) // the sidebar doesn't shift: row 1 is Kit
	if h.m.active != "fm/kit" {
		t.Errorf("sidebar click: active %s", h.m.active)
	}
}

func TestCursorBelowTheTopBar(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	l := h.m.layout()
	if c := h.m.View().Cursor; c == nil || c.Y != l.top+l.sbH+1 {
		t.Errorf("input cursor at %v, want row %d", c, l.top+l.sbH+1)
	}
}

// When the name leaves too little room, Filter goes before Log does, and
// a chip that isn't drawn has no columns to click.
func TestTopBarDropsFilterFirst(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	lw := len(chipText(str.ViewLogButton()))
	fw := len(chipText(str.ViewFilterButton()))
	w := lw + fw + 1 // no room for a name beside both
	line, logc, filt := h.m.topBar(w)
	if filt != [2]int{} || logc != [2]int{w - lw, w} || !strings.HasSuffix(ansi.Strip(line), chipText(str.ViewLogButton())) {
		t.Errorf("topBar(%d) = %q, log %v, filter %v; want Log alone", w, ansi.Strip(line), logc, filt)
	}
	if _, logc, filt := h.m.topBar(lw + 1); logc != [2]int{} || filt != [2]int{} {
		t.Errorf("too narrow for any chip: log %v, filter %v", logc, filt)
	}
}

// "no logs yet" only once there's nothing left to load.
func TestNoLogsYetWaitsForHistory(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.key("ctrl+l")
	b := h.br()
	b.histDone, b.loading = false, true // an older day on its way
	rows, _, _, _ := b.view(60, 20)
	if strings.Contains(ansi.Strip(strings.Join(rows, "\n")), str.BrowseNoLogs()) {
		t.Error("no logs yet shown while history is still loading")
	}
}
