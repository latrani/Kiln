package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/glyph"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/theme"
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
	if got := sideTitleText(h); got != str.FilterTitle() {
		t.Errorf("sidebar title = %q; want the filter panel's", got)
	}
	if side := panelSide(h); side[0] != catsRow() {
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
	h.m.Update(tea.MouseClickMsg{X: 3, Y: sideTitleH, Button: tea.MouseLeft}) // Categories' name
	if !h.br().panelFocus {
		t.Fatal("a click in the panel should focus it")
	}
	h.click(l.sw+10, l.top+2)
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

// panelFooter is the filter panel's footer, its rows joined with the
// separator; "" with no rule above one.
func panelFooter(h *harness) string {
	side := panelSide(h)
	for i := len(side) - 1; i > 0; i-- { // the footer's rule: the last one
		if r := side[i]; r != "" && strings.Trim(r, glyph.Rule) == "" {
			var keys []string
			for _, k := range side[i+1:] {
				if k != "" {
					keys = append(keys, k)
				}
			}
			return strings.Join(keys, str.Separator())
		}
	}
	return ""
}

// logBar is log mode's action bar, trimmed.
func logBar(h *harness) string { return rightRow(h, h.m.height-3) }

// keys joins hint keys as a footer shows them.
func keys(ks ...string) string { return strings.Join(ks, str.Separator()) }

func TestLogFooters(t *testing.T) {
	h := logHarness(t)
	h.key("f")
	h.keys("down", "down") // past Categories and page to sent: no fold, add or remove
	h.key("f")
	if logBar(h) != str.BrowseHintsNone() || panelFooter(h) != keys(str.FilterKeyHide(), str.FilterKeyOnly()) {
		t.Errorf("log focused: bar %q, panel footer %q", logBar(h), panelFooter(h))
	}
	drawn := h.m.View().Content
	if !strings.Contains(drawn, theme.Paint(theme.LogHints, str.BrowseHintsNone())) || !strings.Contains(drawn, theme.Paint(theme.FilterHintsAway, fit(keys(str.FilterKeyHide(), str.FilterKeyOnly()), h.m.layout().sw))) {
		t.Error("log focused: want the log's hints plain, the panel's faded")
	}
	h.key("f")
	if logBar(h) != str.BrowseHintsNoneAway() || panelFooter(h) != keys(str.FilterKeyHide(), str.FilterKeyOnly(), str.FilterKeyLog()) {
		t.Errorf("panel focused: bar %q, panel footer %q", logBar(h), panelFooter(h))
	}
	if !strings.Contains(h.m.View().Content, theme.Paint(theme.LogHintsAway, str.BrowseHintsNoneAway())) {
		t.Error("panel focused: want the log's hints faded")
	}
	h.key("up") // page, which has tags under it
	if want := keys(str.FilterKeyHide(), str.FilterKeyOnly(), str.FilterKeyFold(), str.FilterKeyLog()); panelFooter(h) != want {
		t.Errorf("on a parent: footer %q, want %q", panelFooter(h), want)
	}
	h.key("f")
	if want := keys(str.FilterKeyHide(), str.FilterKeyOnly(), str.FilterKeyFoldAway()); panelFooter(h) != want {
		t.Errorf("on a parent, the log focused: footer %q, want %q", panelFooter(h), want)
	}
	h.key("f")
	h.key("up") // Categories, a group, folds too
	if want := keys(str.FilterKeyHide(), str.FilterKeyOnly(), str.FilterKeyFold(), str.FilterKeyLog()); panelFooter(h) != want {
		t.Errorf("on a group: footer %q, want %q", panelFooter(h), want)
	}
	h.br().AddText("lighthouse")
	for !h.br().Panel.Sel.Item.Text {
		h.key("down")
	}
	if want := keys(str.FilterKeyHide(), str.FilterKeyOnly(), str.FilterKeyRemove(), str.FilterKeyLog()); panelFooter(h) != want {
		t.Errorf("on a text: footer %q, want %q", panelFooter(h), want)
	}
	h.key("down") // + Text
	if want := keys(str.FilterKeyAdd(), str.FilterKeyLog()); panelFooter(h) != want {
		t.Errorf("on + Text: footer %q, want %q", panelFooter(h), want)
	}
	h.key("s")
	if logBar(h) != str.BrowseHintsSelectingAway() {
		t.Errorf("selecting, panel focused: bar %q", logBar(h))
	}
}

func TestLogModeTitleClickFocusesPanel(t *testing.T) {
	h := logHarness(t)
	h.m.Update(tea.MouseClickMsg{X: 3, Y: 0, Button: tea.MouseLeft})
	if !h.br().panelFocus {
		t.Error("a click on the panel's title should focus the panel")
	}
}

func TestFooterRows(t *testing.T) {
	sep := str.Separator()
	got := footerRows("a b"+sep+"c d"+sep+"e", 5)
	if strings.Join(got, "|") != "a b|c d|e" {
		t.Errorf("footerRows = %q", got)
	}
	if got := footerRows("a b"+sep+"e", 20); len(got) != 1 || got[0] != "a b"+sep+"e" {
		t.Errorf("both fit on one row: %q", got)
	}
	if got := footerRows("abcdefgh", 4); len(got) != 1 || got[0] != "abc…" {
		t.Errorf("too wide an item: %q", got)
	}
}

func TestLogModeShortScreen(t *testing.T) {
	h := logHarness(t)
	h.m.Update(tea.WindowSizeMsg{Width: MinWidth, Height: MinHeight})
	h.key("f") // narrow: the panel is open while it has the keys
	rows := strings.Split(h.screen(), "\n")
	if len(rows) != MinHeight {
		t.Fatalf("%d rows; want %d", len(rows), MinHeight)
	}
	if side := panelSide(h); side[0] != catsRow() {
		t.Errorf("at %dx%d the panel lost its list:\n%s", MinWidth, MinHeight, h.screen())
	}
}
