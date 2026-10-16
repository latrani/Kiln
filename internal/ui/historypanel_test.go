package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/glyph"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/theme"
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
	chipX := func() int { _, c := h.m.statusBar(l.rw); return l.sw + 1 + c.main[0] }
	if _, c := h.m.statusBar(l.rw); c.main[1] == 0 || !strings.Contains(xansi.Strip(h.m.View().Content), str.ViewHistoryButton()) {
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

// bottom is the statusline's text.
func (h *harness) bottom() string {
	rows := strings.Split(h.screen(), "\n")
	return strings.TrimRight(rows[len(rows)-1], " ")
}

func TestHistoryScopeChips(t *testing.T) {
	h := sent(t)
	pair := chipText(str.ViewScopeSession()) + chipText(str.ViewScopeCharacter())
	if strings.Contains(h.bottom(), pair) {
		t.Fatalf("scope chips with the panel down: %q", h.bottom())
	}
	h.key("up")
	if want := pair + str.Separator() + chipText(str.ViewHistoryButton()); !strings.HasSuffix(h.bottom()+" ", want) {
		t.Errorf("statusline %q; want it to end %q", h.bottom(), want)
	}
	if bar := h.m.View().Content; !strings.Contains(bar, theme.Paint(theme.StatusScopeOn, chipText(str.ViewScopeCharacter()))) ||
		!strings.Contains(bar, theme.Paint(theme.StatusScope, chipText(str.ViewScopeSession()))) {
		t.Error("want Character lit, Session not")
	}
	h.key("esc")
	if strings.Contains(h.bottom(), pair) {
		t.Errorf("scope chips after closing: %q", h.bottom())
	}
}

func TestHistoryScopeArrows(t *testing.T) {
	h := sent(t)
	h.open("fm/rook")
	h.m.switchTo("fm/kit")
	h.m.chars["fm/rook"].Unread = 1
	h.key("up")
	for _, step := range []struct {
		key  string
		want app.Scope
	}{{"left", app.ScopeSession}, {"left", app.ScopeSession}, {"right", app.ScopeCharacter}, {"right", app.ScopeCharacter}} { // no looping
		h.key(step.key)
		if h.m.a.HistoryScope() != step.want || !h.m.a.History().IsOpen() {
			t.Fatalf("%s: scope %v, open %v; want %v", step.key, h.m.a.HistoryScope(), h.m.a.History().IsOpen(), step.want)
		}
	}
	h.press(tea.KeyTab, 0)
	if h.m.a.HistoryScope() != app.ScopeCharacter || h.m.a.Active() != "fm/rook" {
		t.Errorf("Tab with the panel up: scope %v, active %s; want the unread one, scope as it was", h.m.a.HistoryScope(), h.m.a.Active())
	}
}

func TestHistoryScopeClick(t *testing.T) {
	h := sent(t)
	h.key("up")
	l := h.m.layout()
	click := func(span [2]int) {
		h.m.Update(tea.MouseClickMsg{X: l.sw + 1 + span[0], Y: h.m.height - 1, Button: tea.MouseLeft})
	}
	_, c := h.m.statusBar(l.rw)
	click(c.session)
	if h.m.a.HistoryScope() != app.ScopeSession || !h.m.a.History().IsOpen() {
		t.Errorf("clicked Session: scope %v, open %v", h.m.a.HistoryScope(), h.m.a.History().IsOpen())
	}
	_, c = h.m.statusBar(l.rw)
	click(c.character)
	if h.m.a.HistoryScope() != app.ScopeCharacter {
		t.Errorf("clicked Character: scope %v", h.m.a.HistoryScope())
	}
}

func TestHistoryScopeNarrow(t *testing.T) {
	h := sent(t)
	h.key("up")
	l := h.m.layout()
	hist := xansi.StringWidth(chipText(str.ViewHistoryButton()))
	all := hist + xansi.StringWidth(chipText(str.ViewScopeSession())+chipText(str.ViewScopeCharacter())+str.Separator())
	w := hist + 2 + (all-hist)/2 // room for History beside a message, not the switch too
	line, c := h.m.statusBar(w)
	if c.main[1] == 0 || c.session != ([2]int{}) || c.character != ([2]int{}) || strings.Contains(xansi.Strip(line), str.ViewScopeSession()) {
		t.Errorf("at %d of %d: %q, chips %+v; want History alone", w, l.rw, xansi.Strip(line), c)
	}
}

func TestHistoryPlaceholder(t *testing.T) {
	h := sent(t)
	h.key("up")
	last := func() string {
		l := h.m.layout()
		return strings.TrimRight(xansi.Strip(l.inRows[len(l.inRows)-1]), " ")
	}
	if got, want := last(), glyph.Search+str.ViewHistoryPlaceholder(); got != want {
		t.Errorf("empty query row %q; want %q", got, want)
	}
	h.typeText("x")
	if strings.Contains(last(), str.ViewHistoryPlaceholder()) {
		t.Errorf("placeholder with a query: %q", last())
	}
}

func TestHistoryChipNothingOpen(t *testing.T) {
	h := newHarness(t, map[string]string{"sp": spWorld}) // nothing autoconnects
	if h.m.cur() != nil {
		t.Fatal("something's open")
	}
	h.key("up")
	if strings.Contains(h.bottom(), str.ViewHistoryButton()) || h.m.a.History().IsOpen() {
		t.Errorf("nothing open: statusline %q, panel open %v", h.bottom(), h.m.a.History().IsOpen())
	}
	h.open("sp/ash") // not connected
	if !strings.Contains(h.bottom(), str.ViewHistoryButton()) {
		t.Errorf("disconnected character: statusline %q; want History", h.bottom())
	}
}

func TestHistoryScopeArrowsElsewhere(t *testing.T) {
	h := sent(t)
	h.open("fm/rook")
	h.m.switchTo("fm/kit")
	h.m.chars["fm/rook"].Unread = 1
	h.key("up")
	h.key("ctrl+l") // log mode over the panel
	h.key("left")
	if h.m.a.HistoryScope() != app.ScopeCharacter || h.m.a.Active() != "fm/kit" {
		t.Errorf("← in log mode: scope %v, active %s; want both as they were", h.m.a.HistoryScope(), h.m.a.Active())
	}
	h.key("ctrl+l") // out of log mode, back to the panel
	if !h.m.historyShows() {
		t.Fatal("the panel isn't showing")
	}
	h.m.openPicker()
	if h.m.picker == nil {
		t.Fatal("no picker")
	}
	h.key("left")
	if h.m.a.HistoryScope() != app.ScopeCharacter {
		t.Errorf("← in the picker: scope %v; the picker's fields have it", h.m.a.HistoryScope())
	}
}
