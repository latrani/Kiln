package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

const spWorld = "host = \"sp.test\"\nport = 1\n\n[[characters]]\nid = \"ash\"\nname = \"Ash\"\naliases = [\"Cinder\"]\n"

// sideRows is the sidebar's text, trimmed, down to the first blank row.
func sideRows(h *harness) []string {
	var out []string
	for _, row := range strings.Split(h.screen(), "\n") {
		s := strings.TrimSpace(strings.SplitN(row, "│", 2)[0])
		if s == "" {
			break
		}
		out = append(out, s)
	}
	return out
}

func TestPickerOpensAndConnects(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.press('o', tea.ModCtrl)
	if got := strings.Join(sideRows(h), "|"); got != "fm|Rook|sp|Ash" {
		t.Errorf("picker rows = %q, want unopened characters only", got)
	}
	if s := h.screen(); !strings.Contains(s, "│Filter:   · Enter to connect · Esc to close") {
		t.Errorf("no filter prompt:\n%s", s)
	}
	h.enter()
	rook := h.m.chars["fm/rook"]
	if h.m.picker != nil || rook == nil || rook.sess == nil || h.m.active != "fm/rook" {
		t.Fatalf("Enter should open, connect and switch to rook:\n%s", h.screen())
	}
}

func TestPickerFilter(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.press('o', tea.ModCtrl)
	for filter, want := range map[string]string{
		"cin": "sp|Ash",  // alias
		"SP":  "sp|Ash",  // world, any case
		"ro":  "fm|Rook", // name
		"zzz": noMatches, // nothing
		"":    "fm|Rook|sp|Ash",
	} {
		h.m.picker.form.fields[0].in.SetValue("")
		h.typeText(filter)
		if got := strings.Join(sideRows(h), "|"); got != want {
			t.Errorf("filter %q: rows = %q, want %q", filter, got, want)
		}
	}
	h.m.picker.form.fields[0].in.SetValue("")
	h.typeText("zzz")
	h.enter() // nothing to open
	if h.m.picker == nil || len(h.m.order) != 1 {
		t.Errorf("Enter with no matches changed something: order %v", h.m.order)
	}
}

func TestPickerArrowsSkipHeaders(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.press('o', tea.ModCtrl)
	if h.m.picker.sel != "fm/rook" {
		t.Fatalf("sel = %q, want the first character", h.m.picker.sel)
	}
	h.press(tea.KeyDown, 0)
	h.press(tea.KeyDown, 0) // clamps at the last
	if h.m.picker.sel != "sp/ash" {
		t.Errorf("sel = %q after ↓↓", h.m.picker.sel)
	}
	h.press(tea.KeyUp, 0)
	if h.m.picker.sel != "fm/rook" {
		t.Errorf("sel = %q after ↑", h.m.picker.sel)
	}
	h.typeText("a") // "a" matches Ash only (not Rook): the highlight follows
	if h.m.picker.sel != "sp/ash" {
		t.Errorf("sel = %q after filtering", h.m.picker.sel)
	}
}

func TestPickerClicks(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	click := func(y int) tea.Cmd {
		_, cmd := h.m.Update(tea.MouseClickMsg{X: 4, Y: y, Button: tea.MouseLeft})
		return cmd
	}
	click(2) // rows: fm, Kit, + Add connection
	if h.m.picker == nil {
		t.Fatalf("clicking %s didn't open the picker:\n%s", addLabel, h.screen())
	}
	click(0) // a world header: nothing
	if h.m.picker == nil {
		t.Fatal("clicking a header closed the picker")
	}
	if click(1) == nil || h.m.chars["fm/rook"] == nil || h.m.picker != nil {
		t.Errorf("clicking Rook should open and connect it:\n%s", h.screen())
	}
}

func TestPickerEscAndReload(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.press('o', tea.ModCtrl)
	h.typeText("ro")
	h.press(tea.KeyEscape, 0)
	if h.m.picker != nil || h.m.chars["fm/rook"] != nil {
		t.Fatal("Esc should close the picker and open nothing")
	}
	h.press('o', tea.ModCtrl)
	h.typeText("ro")
	noRook := strings.Replace(fmWorld, "[[characters]]\nid = \"rook\"\nname = \"Rook\"\n", "", 1)
	os.WriteFile(filepath.Join(h.dir, "worlds", "fm.toml"), []byte(noRook), 0o600)
	h.m.Update(reloadMsg{})
	if h.m.picker == nil || h.m.picker.form.value(0) != "ro" {
		t.Fatal("reload should keep the picker and its filter")
	}
	if h.m.picker.sel != "" || !strings.Contains(h.screen(), noMatches) {
		t.Errorf("sel = %q; the removed character should be gone:\n%s", h.m.picker.sel, h.screen())
	}
	h.enter()
	if h.m.chars["fm/rook"] != nil {
		t.Error("Enter opened a character that's no longer configured")
	}
}

func TestPickerBlockedInBrowse(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("/browse")
	h.enter()
	h.press('o', tea.ModCtrl)
	if h.m.picker != nil {
		t.Fatal("picker opened over browse mode")
	}
	// Shown in full on an 80-column screen, for Ctrl+O and for a click on
	// + Add connection alike.
	if s := h.screen(); !strings.Contains(s, browseBlocksPicker) {
		t.Errorf("Ctrl+O: status not shown in full:\n%s", s)
	}
	h.m.status = ""
	h.m.openPicker()
	if s := h.screen(); h.m.picker != nil || !strings.Contains(s, browseBlocksPicker) {
		t.Errorf("click: status not shown in full:\n%s", s)
	}
}

func TestSwitchingIntoBrowseClosesPicker(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook")
	h.m.switchTo("fm/rook")
	h.typeText("/browse")
	h.enter()
	h.m.switchTo("fm/kit")
	h.press('o', tea.ModCtrl)
	h.press(tea.KeyDown, tea.ModCtrl) // to rook, who is browsing
	if h.m.active != "fm/rook" || h.m.picker != nil {
		t.Errorf("active = %q, picker open = %v; browse would hide the filter", h.m.active, h.m.picker != nil)
	}
}

func TestPickerPageStepsByRows(t *testing.T) {
	worlds := map[string]string{}
	for i := range 20 { // 40 rows: taller than the screen
		worlds[fmt.Sprintf("w%02d", i)] = fmt.Sprintf("host = \"w.test\"\nport = 1\n\n[[characters]]\nid = \"c%02d\"\nname = \"C%02d\"\n", i, i)
	}
	h := newHarness(t, worlds)
	h.press('o', tea.ModCtrl)
	rowOf := func() int {
		return slices.IndexFunc(h.m.pickerRows(), func(r sidebarRow) bool { return r.char == h.m.picker.sel })
	}
	start, page := rowOf(), max(1, h.m.sidebarView().avail-1)
	h.press(tea.KeyPgDown, 0)
	if got := rowOf(); got <= start || got > start+page {
		t.Errorf("PgDn: row %d → %d, want within one page (%d rows)", start, got, page)
	}
	if name := "C" + h.m.picker.sel[len(h.m.picker.sel)-2:]; !slices.Contains(sideRows(h), name) {
		t.Errorf("highlight %q scrolled out of view:\n%s", h.m.picker.sel, h.screen())
	}
	mid, page := rowOf(), max(1, h.m.sidebarView().avail-1) // the ▲ hint takes a row now
	h.press(tea.KeyPgUp, 0)
	if got := rowOf(); got >= mid || got < mid-page {
		t.Errorf("PgUp: row %d → %d, want within one page (%d rows)", mid, got, page)
	}
}

func TestPickerBlockedBySavePasswordQuestion(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	delete(h.pw, "fm/kit")
	h.init()
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].needPW })
	h.typeText("s3cret")
	h.enter()
	if h.m.mode != modeSavePassword {
		t.Fatal("no save-password question")
	}
	h.press('o', tea.ModCtrl)
	if h.m.picker != nil || !strings.Contains(h.screen(), questionBlocksPicker) {
		t.Errorf("Ctrl+O: picker opened or no status:\n%s", h.screen())
	}
	h.m.status = ""
	h.m.Update(tea.MouseClickMsg{X: 4, Y: 2, Button: tea.MouseLeft}) // rows: fm, Kit, + Add connection
	if h.m.picker != nil || !strings.Contains(h.screen(), questionBlocksPicker) {
		t.Errorf("click: picker opened or no status:\n%s", h.screen())
	}
	if h.m.mode != modeSavePassword || len(h.saved) != 0 {
		t.Error("the question should still be waiting")
	}
}
