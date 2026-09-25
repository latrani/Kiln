package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/config"
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
	if got := strings.Join(sideRows(h), "|"); got != "fm|Rook|"+addCharLabel+"|sp|Ash|"+addCharLabel+"|"+addWorldLabel {
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
		"cin": "sp|Ash|" + addCharLabel + "|" + addWorldLabel,  // alias
		"SP":  "sp|Ash|" + addCharLabel + "|" + addWorldLabel,  // world, any case
		"ro":  "fm|Rook|" + addCharLabel + "|" + addWorldLabel, // name
		"zzz": addWorldLabel,                                   // nothing
		"":    "fm|Rook|" + addCharLabel + "|sp|Ash|" + addCharLabel + "|" + addWorldLabel,
	} {
		h.m.picker.form.fields[0].in.SetValue("")
		h.typeText(filter)
		if got := strings.Join(sideRows(h), "|"); got != want {
			t.Errorf("filter %q: rows = %q, want %q", filter, got, want)
		}
	}
	h.m.picker.form.fields[0].in.SetValue("")
	h.typeText("zzz")
	h.enter() // + World
	if h.m.picker == nil || h.m.picker.edit == nil || len(h.m.order) != 1 {
		t.Errorf("Enter with no matches should only open the world editor: order %v", h.m.order)
	}
}

func TestPickerArrowsSkipHeaders(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.press('o', tea.ModCtrl)
	if h.m.picker.sel != "fm/rook" {
		t.Fatalf("sel = %q, want the first character", h.m.picker.sel)
	}
	h.press(tea.KeyDown, 0)
	h.press(tea.KeyDown, 0)
	if h.m.picker.sel != "sp/ash" {
		t.Errorf("sel = %q after ↓↓", h.m.picker.sel)
	}
	for range 3 {
		h.press(tea.KeyDown, 0) // clamps at the last
	}
	if h.m.picker.sel != addWorldSel {
		t.Errorf("sel = %q after ↓↓↓", h.m.picker.sel)
	}
	for range 4 {
		h.press(tea.KeyUp, 0) // through the add rows
	}
	if h.m.picker.sel != "fm/rook" {
		t.Errorf("sel = %q after ↑↑↑↑", h.m.picker.sel)
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
	if h.m.picker.sel != addWorldSel || strings.Contains(h.screen(), "Rook") {
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

func TestPickerAddRows(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.open("fm/rook") // with Kit autoconnected, fm has nobody left to offer
	h.press('o', tea.ModCtrl)
	want := "fm|" + addCharLabel + "|sp|Ash|" + addCharLabel + "|" + addWorldLabel
	if got := strings.Join(sideRows(h), "|"); got != want {
		t.Errorf("rows = %q, want %q", got, want)
	}
	if h.m.picker.sel != "sp/ash" {
		t.Errorf("sel = %q, want the first character", h.m.picker.sel)
	}
	h.press(tea.KeyUp, 0)
	if h.m.picker.sel != addCharSel("fm") {
		t.Errorf("sel = %q after ↑, want fm's add row", h.m.picker.sel)
	}
	h.typeText("zzz")
	if got := strings.Join(sideRows(h), "|"); got != addWorldLabel {
		t.Errorf("no matches: rows = %q", got)
	}
	if h.m.picker.sel != addWorldSel {
		t.Errorf("sel = %q, want %q", h.m.picker.sel, addWorldSel)
	}
}

// toAddWorld opens the picker's world editor.
func toAddWorld(h *harness) {
	h.press('o', tea.ModCtrl)
	h.typeText("zzz") // leaves only + World
	h.enter()
	if h.m.picker.edit == nil {
		h.t.Fatalf("no world editor:\n%s", h.screen())
	}
}

func TestPickerAddWorld(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	toAddWorld(h)
	s := h.screen()
	for _, want := range []string{"│World: ", "│Host:  ", "│Port:  ", "│TLS:   [ ]", "│[ Save ]"} {
		if !strings.Contains(s, want) {
			t.Errorf("editor is missing %q:\n%s", want, s)
		}
	}
	h.typeText("new world") // the space is refused
	h.enter()               // Enter moves on; it doesn't save
	if h.m.picker.edit == nil {
		t.Fatal("Enter in a field saved (or closed) the editor")
	}
	h.typeText("muck.example.org")
	h.press(tea.KeyTab, 0)
	h.typeText("88a99") // so is the letter
	h.press(tea.KeyTab, 0)
	h.press(' ', 0)
	h.enter() // to [ Save ]
	h.enter()
	if h.m.picker == nil || h.m.picker.edit != nil {
		t.Fatalf("saving should go back to the picker:\n%s", h.screen())
	}
	if _, err := os.Stat(filepath.Join(h.dir, "worlds", "newworld.toml")); err != nil {
		t.Fatal(err)
	}
	var w *config.World
	for i := range h.m.cfg.Worlds {
		if h.m.cfg.Worlds[i].ID == "newworld" {
			w = &h.m.cfg.Worlds[i]
		}
	}
	if w == nil {
		t.Fatal("the new world isn't loaded")
	}
	if h.m.picker.form.value(0) != "" || h.m.picker.sel != addCharSel("newworld") {
		t.Errorf("filter %q, sel %q; want the new world's add row", h.m.picker.form.value(0), h.m.picker.sel)
	}
	if !strings.Contains(strings.Join(sideRows(h), "|"), "newworld|"+addCharLabel) {
		t.Errorf("rows = %q", sideRows(h))
	}
}

func TestPickerAddWorldErrorKeepsEditor(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	toAddWorld(h)
	h.typeText("fm")
	h.press(tea.KeyTab, 0)
	h.typeText("h")
	h.press(tea.KeyTab, 0)
	h.typeText("23")
	h.press(tea.KeyTab, 0)
	h.enter() // to [ Save ]
	h.enter()
	if h.m.picker.edit == nil || !strings.Contains(h.screen(), "already exists") {
		t.Errorf("a taken id should keep the editor open and say why:\n%s", h.screen())
	}
	h.press(tea.KeyEscape, 0)
	if h.m.picker == nil || h.m.picker.edit != nil || h.m.picker.form.value(0) != "zzz" {
		t.Error("Esc should go back to the picker, filter kept")
	}
}

func TestPickerAddCharacterConnects(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.press('o', tea.ModCtrl)
	h.press(tea.KeyDown, 0) // Rook, then fm's add row
	h.enter()
	if h.m.picker.edit == nil {
		t.Fatalf("no character editor:\n%s", h.screen())
	}
	if s := h.screen(); !strings.Contains(s, "│Name: ") {
		t.Errorf("no name prompt:\n%s", s)
	}
	h.typeText("O'Br ")
	if !strings.Contains(h.screen(), "spaces") {
		t.Errorf("the refused space isn't explained:\n%s", h.screen())
	}
	h.typeText("ien")
	h.enter()
	cs := h.m.chars["fm/O_Brien"]
	if h.m.picker != nil || cs == nil || cs.sess == nil || h.m.active != "fm/O_Brien" {
		t.Fatalf("saving should open, connect and switch to O'Brien:\n%s", h.screen())
	}
	if cs.ch.Name != "O'Brien" {
		t.Errorf("name = %q", cs.ch.Name)
	}
}

func TestPickerClickAddRows(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	click := func(y int) {
		h.m.Update(tea.MouseClickMsg{X: 4, Y: y, Button: tea.MouseLeft})
	}
	h.press('o', tea.ModCtrl) // rows: fm, Rook, + Character, + World
	click(2)
	if e := h.m.picker.edit; e == nil || e.world != "fm" {
		t.Fatalf("clicking %s didn't open fm's character editor", addCharLabel)
	}
	click(3) // another row leaves the editor for that row's
	if e := h.m.picker.edit; e == nil || e.world != "" {
		t.Fatalf("clicking %s didn't open the world editor", addWorldLabel)
	}
	click(0) // a header leaves the editor
	if h.m.picker == nil || h.m.picker.edit != nil {
		t.Error("clicking a header should close the editor, not the picker")
	}
}

func TestNoCharactersPointsAtPicker(t *testing.T) {
	h := newHarness(t, nil)
	if s := h.screen(); !strings.Contains(s, "│"+noCharacters) {
		t.Errorf("empty config doesn't say how to add a character:\n%s", s)
	}
}
