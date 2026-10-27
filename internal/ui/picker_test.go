package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/str"
)

const spWorld = "host = \"sp.test\"\nport = 1\n\n[[characters]]\nid = \"ash\"\nname = \"Ash\"\naliases = [\"Cinder\"]\n"

// sideRows is the sidebar's text, trimmed, down to the first blank row.
func sideRows(h *harness) []string {
	var out []string
	for _, row := range strings.Split(h.screen(), "\n")[listY(h, 0):] {
		s := untree(strings.SplitN(row, "│", 2)[0])
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
	if got := strings.Join(sideRows(h), "|"); got != "fm|× Kit|Rook|"+addCharLabel+"|sp|Ash|"+addCharLabel+"|"+addWorldLabel {
		t.Errorf("picker rows = %q, want every character, open or not", got)
	}
	if h.m.picker.sel != "fm/kit" {
		t.Errorf("sel = %q, want what's showing", h.m.picker.sel)
	}
	if got := strings.TrimSpace(screenSide(h, sideTitleH)); got != str.PickerFilter() {
		t.Errorf("under the title = %q, want the filter, empty, saying what it is", got)
	}
	if c := h.m.View().Cursor; c == nil || c.X != 1 || c.Y != sideTitleH {
		t.Errorf("cursor = %v, want in the filter", c)
	}
	if s := h.screen(); !strings.Contains(s, "│"+pickerHint) {
		t.Errorf("the input area should say the picker's keys:\n%s", s)
	}
	h.typeText("roo")
	if got := strings.TrimSpace(screenSide(h, sideTitleH)); got != "roo" {
		t.Errorf("the filter = %q, want what's typed", got)
	}
	h.enter()
	rook := h.m.chars["fm/rook"]
	if h.m.picker != nil || rook == nil || rook.Sess == nil || h.m.a.Active() != "fm/rook" {
		t.Fatalf("Enter should open, connect and switch to rook:\n%s", h.screen())
	}
}

// Enter on an open character shows it, connecting nothing; on a world
// with one open, its overview.
func TestPickerJumpsToOpen(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.open("sp/ash")
	h.press('o', tea.ModCtrl)
	h.typeText("ash")
	h.enter()
	if h.m.picker != nil || h.m.a.Active() != "sp/ash" || h.m.chars["sp/ash"].Sess != nil {
		t.Fatalf("Enter on open Ash: active %q, picker open %v, connected %v", h.m.a.Active(), h.m.picker != nil, h.m.chars["sp/ash"].Sess != nil)
	}
	h.press('o', tea.ModCtrl)
	if h.m.picker.sel != "sp/ash" {
		t.Errorf("sel = %q, want what's showing", h.m.picker.sel)
	}
	h.typeText("fm")
	if h.m.picker.sel != worldSel("fm") {
		t.Fatalf("sel = %q, want fm's own row, the first match", h.m.picker.sel)
	}
	h.enter()
	if h.m.picker != nil || h.m.a.Active() != worldSel("fm") {
		t.Errorf("Enter on fm: active %q, picker open %v; want its overview", h.m.a.Active(), h.m.picker != nil)
	}
	h.press('o', tea.ModCtrl)
	if h.m.picker.sel != worldSel("fm") {
		t.Errorf("sel = %q, want the overview showing", h.m.picker.sel)
	}
	h.press('o', tea.ModCtrl)
	if h.m.picker != nil {
		t.Error("Ctrl+O again should close the picker")
	}
}

// A world with nothing open has no overview: Enter on it opens its
// editor.
func TestPickerWorldWithNothingOpen(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.press('o', tea.ModCtrl)
	h.typeText("sp")
	h.enter()
	if h.m.picker == nil || h.m.picker.edit == nil || h.m.picker.edit.kind != editWorld {
		t.Errorf("Enter on sp should open its editor:\n%s", h.screen())
	}
}

func TestPickerFilter(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.press('o', tea.ModCtrl)
	for filter, want := range map[string]string{
		"SP":  "sp|Ash|" + addCharLabel + "|" + addWorldLabel,        // a world, any case
		"ro":  "fm|Rook|" + addCharLabel + "|" + addWorldLabel,       // a name
		"fmr": "fm|Rook|" + addCharLabel + "|" + addWorldLabel,       // in order, across the world and the name
		"fm/": "fm|× Kit|Rook|" + addCharLabel + "|" + addWorldLabel, // the / between them
		"rf":  addWorldLabel,                                         // out of order
		"cin": addWorldLabel,                                         // an alias isn't on screen
		"zzz": addWorldLabel,                                         // nothing
		"":    "fm|× Kit|Rook|" + addCharLabel + "|sp|Ash|" + addCharLabel + "|" + addWorldLabel,
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
	if h.m.picker == nil || h.m.picker.edit == nil || len(h.m.a.Order()) != 1 {
		t.Errorf("Enter with no matches should only open the world editor: order %v", h.m.a.Order())
	}
}

func TestPickerArrows(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.press('o', tea.ModCtrl)
	if h.m.picker.sel != "fm/kit" {
		t.Fatalf("sel = %q, want what's showing", h.m.picker.sel)
	}
	for range 4 { // Rook, fm's add row, sp's header, Ash
		h.press(tea.KeyDown, 0)
	}
	if h.m.picker.sel != "sp/ash" {
		t.Errorf("sel = %q after ↓×4", h.m.picker.sel)
	}
	for range 4 {
		h.press(tea.KeyDown, 0) // clamps at the last
	}
	if h.m.picker.sel != addWorldSel {
		t.Errorf("sel = %q after ↓×4 more", h.m.picker.sel)
	}
	for range 6 {
		h.press(tea.KeyUp, 0) // through the add rows and a header
	}
	if h.m.picker.sel != "fm/kit" {
		t.Errorf("sel = %q after ↑×6", h.m.picker.sel)
	}
	h.press(tea.KeyUp, 0)
	h.press(tea.KeyUp, 0) // clamps at the first header
	if h.m.picker.sel != worldSel("fm") {
		t.Errorf("sel = %q, want fm's header", h.m.picker.sel)
	}
	h.press(tea.KeyDown, 0)
	h.typeText("a") // "a" matches Ash first: the highlight goes there
	if h.m.picker.sel != "sp/ash" {
		t.Errorf("sel = %q after filtering", h.m.picker.sel)
	}
	h.press(tea.KeyUp, 0)
	h.press(tea.KeyLeft, 0) // the filter's cursor moves, its text doesn't: the highlight stays
	if h.m.picker.sel != worldSel("sp") {
		t.Errorf("sel = %q after ↑ and ←, want sp's header", h.m.picker.sel)
	}
	h.press(tea.KeyRight, 0)
	h.press(tea.KeyBackspace, 0) // the filter, emptied: back to what's showing
	if h.m.picker.sel != "fm/kit" {
		t.Errorf("sel = %q with the filter emptied, want what's showing", h.m.picker.sel)
	}
}

func TestMatch(t *testing.T) {
	for _, c := range []struct {
		s, f string
		at   []int
		ok   bool
	}{
		{"fm/Rook", "fmr", []int{0, 1, 3}, true},
		{"fm/Rook", "FM/", []int{0, 1, 2}, true},
		{"fm/Rook", "", nil, true},
		{"fm/Rook", "rf", nil, false},
		{"fm/Kit", "fmk", []int{0, 1, 3}, true},
		{"fm/Mira", "mr", []int{3, 5}, true},                      // Mira's M, where the name starts
		{"sp/AshSable", "ss", []int{0, 6}, true},                  // S, where a word starts, over the s in Ash
		{"Tapestries/Indi", "tapsi", []int{0, 1, 2, 4, 11}, true}, // I, where the name starts
	} {
		at, _, ok := match(c.s, c.f)
		if ok != c.ok || !slices.Equal(at, c.at) {
			t.Errorf("match(%q, %q) = %v, %v; want %v, %v", c.s, c.f, at, ok, c.at, c.ok)
		}
	}
}

// The highlight goes to the best match, not the first: letters where
// words start count most, and the filter's first letter most of all.
func TestPickerPrefersWordStarts(t *testing.T) {
	fm := "host = \"fm.test\"\nport = 1\n"
	for _, n := range []string{"Ash", "Kit", "Mira", "Rook"} {
		fm += fmt.Sprintf("\n[[characters]]\nid = %q\nname = %q\n", strings.ToLower(n), n)
	}
	sp := "host = \"sp.test\"\nport = 1\n\n[[characters]]\nid = \"sable\"\nname = \"Sable\"\n"
	h := newHarness(t, map[string]string{"fm": fm, "sp": sp})
	h.press('o', tea.ModCtrl)
	for filter, want := range map[string]string{
		"fmr": "fm/rook",      // Rook's R starts its name; Mira's r doesn't
		"s":   worldSel("sp"), // sp starts with s; Ash only has one
		"f/m": "fm/mira",      // Mira's M, not the m in fm
		"fm":  worldSel("fm"), // fm itself, before its characters, which tie it
	} {
		h.m.picker.form.fields[0].in.SetValue("")
		h.typeText(filter)
		if h.m.picker.sel != want {
			t.Errorf("filter %q: sel = %q, want %q", filter, h.m.picker.sel, want)
		}
	}
}

// /open with a filter jumps to its first match, as Ctrl+O, typing it
// and Enter would; with no match the picker stays up, filtered.
func TestOpenCommandJumps(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.typeText("/open fmr")
	h.enter()
	if cs := h.m.chars["fm/rook"]; h.m.picker != nil || cs == nil || cs.Sess == nil || h.m.a.Active() != "fm/rook" {
		t.Fatalf("/open fmr should open, connect and show Rook:\n%s", h.screen())
	}
	h.typeText("/open zzz")
	h.enter()
	if h.m.picker == nil || h.m.picker.form.value(0) != "zzz" || h.m.picker.edit != nil {
		t.Errorf("/open zzz should leave the picker up, filtered:\n%s", h.screen())
	}
	h.press(tea.KeyEscape, 0)
	h.typeText("/open")
	h.enter()
	if h.m.picker == nil || h.m.picker.form.value(0) != "" {
		t.Error("/open alone should open the picker")
	}
}

// The first match is the same row whatever's open: the list's order
// doesn't change as characters open and close.
func TestPickerFirstMatchIsStable(t *testing.T) {
	two := fmWorld + "\n[[characters]]\nid = \"mira\"\nname = \"Mira\"\n"
	h := newHarness(t, map[string]string{"fm": two})
	h.press('o', tea.ModCtrl)
	h.typeText("i")
	if h.m.picker.sel != "fm/kit" { // Kit's i and Mira's tie: the first
		t.Errorf("i: sel = %q", h.m.picker.sel)
	}
	h.press(tea.KeyEscape, 0)
	h.open("fm/mira")
	h.press('o', tea.ModCtrl)
	h.typeText("i")
	if h.m.picker.sel != "fm/kit" {
		t.Errorf("i with Mira open: sel = %q, still Kit", h.m.picker.sel)
	}
	h.typeText("r")
	if h.m.picker.sel != "fm/mira" {
		t.Errorf("ir: sel = %q", h.m.picker.sel)
	}
}

func TestPickerClicks(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	click := func(y int) tea.Cmd {
		_, cmd := h.m.Update(tea.MouseClickMsg{X: 4, Y: listY(h, y), Button: tea.MouseLeft})
		return cmd
	}
	click(3) // rows: fm, Kit, a gap, + Connection
	if h.m.picker == nil {
		t.Fatalf("clicking %s didn't open the picker:\n%s", addLabel, h.screen())
	}
	if click(2) == nil || h.m.chars["fm/rook"] == nil || h.m.picker != nil { // rows: fm, Kit, Rook, + Character
		t.Errorf("clicking Rook should open and connect it:\n%s", h.screen())
	}
	h.press('o', tea.ModCtrl)
	click(0)
	if h.m.picker != nil || h.m.a.Active() != worldSel("fm") {
		t.Errorf("clicking fm should show its overview: active %q", h.m.a.Active())
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
	h.typeText("/log")
	h.enter()
	h.press('o', tea.ModCtrl)
	if h.m.picker != nil {
		t.Fatal("picker opened over browse mode")
	}
	// Shown in full on an 80-column screen, for Ctrl+O and for a click on
	// + Open connection alike.
	if s := h.screen(); !strings.Contains(s, browseBlocksPicker) {
		t.Errorf("Ctrl+O: status not shown in full:\n%s", s)
	}
	h.m.a.ClearStatus()
	h.m.openPicker()
	if s := h.screen(); h.m.picker != nil || !strings.Contains(s, browseBlocksPicker) {
		t.Errorf("click: status not shown in full:\n%s", s)
	}
}

func TestSwitchingIntoBrowseClosesPicker(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook")
	h.m.switchTo("fm/rook")
	h.typeText("/log")
	h.enter()
	h.m.switchTo("fm/kit")
	h.press('o', tea.ModCtrl)
	h.press(tea.KeyDown, tea.ModCtrl) // to rook, who is browsing
	if h.m.a.Active() != "fm/rook" || h.m.picker != nil {
		t.Errorf("active = %q, picker open = %v; browse would hide the filter", h.m.a.Active(), h.m.picker != nil)
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
		return slices.IndexFunc(h.m.pickerRows(), func(r sidebarRow) bool { return selKey(r) == h.m.picker.sel })
	}
	start, page := rowOf(), max(1, h.m.sidebarView().avail-1)
	h.press(tea.KeyPgDown, 0)
	if got := rowOf(); got <= start || got > start+page {
		t.Errorf("PgDn: row %d → %d, want within one page (%d rows)", start, got, page)
	}
	if sv, got := h.m.sidebarView(), rowOf(); got < sv.top || got >= sv.top+sv.avail {
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
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].NeedPW })
	h.typeText("s3cret")
	h.enter()
	if !h.m.asking() {
		t.Fatal("no save-password question")
	}
	h.press('o', tea.ModCtrl)
	if h.m.picker != nil || !strings.Contains(h.screen(), questionBlocksPicker) {
		t.Errorf("Ctrl+O: picker opened or no status:\n%s", h.screen())
	}
	h.m.a.ClearStatus()
	h.m.Update(tea.MouseClickMsg{X: 4, Y: sideTitleH + 3, Button: tea.MouseLeft}) // rows: fm, Kit, a gap, + Connection
	if h.m.picker != nil || !strings.Contains(h.screen(), questionBlocksPicker) {
		t.Errorf("click: picker opened or no status:\n%s", h.screen())
	}
	if !h.m.asking() || len(h.saved) != 0 {
		t.Error("the question should still be waiting")
	}
}

func TestPickerAddRows(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.press('o', tea.ModCtrl)
	want := "fm|× Kit|Rook|" + addCharLabel + "|sp|Ash|" + addCharLabel + "|" + addWorldLabel
	if got := strings.Join(sideRows(h), "|"); got != want {
		t.Errorf("rows = %q, want %q", got, want)
	}
	h.m.picker.sel = "sp/ash"
	h.press(tea.KeyUp, 0)
	h.press(tea.KeyUp, 0) // past sp's header
	if h.m.picker.sel != addCharSel("fm") {
		t.Errorf("sel = %q after ↑↑, want fm's add row", h.m.picker.sel)
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
	for _, want := range []string{"│World: ", "│Host:  ", "│Port:  ", "│TLS:   [ ]", "│ Save "} {
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
	h.press(tea.KeyDown, 0) // past the collapsed extras
	h.press(tea.KeyDown, 0) // to Save
	h.enter()
	if h.m.picker == nil || h.m.picker.edit != nil {
		t.Fatalf("saving should go back to the picker:\n%s", h.screen())
	}
	if _, err := os.Stat(filepath.Join(h.dir, "worlds", "newworld.toml")); err != nil {
		t.Fatal(err)
	}
	var w *config.World
	for i := range h.m.a.Config().Worlds {
		if h.m.a.Config().Worlds[i].ID == "newworld" {
			w = &h.m.a.Config().Worlds[i]
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

// The save reports every problem; the form shows the first, as before.
func TestEditorShowsTheFirstProblem(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	toAddWorld(h)
	h.typeText("fm") // taken
	h.press(tea.KeyTab, 0)
	h.typeText("h")
	h.press(tea.KeyTab, 0) // no port
	h.press(tea.KeyTab, 0)
	h.press(tea.KeyDown, 0)
	h.press(tea.KeyDown, 0) // to Save
	h.enter()
	s := h.screen()
	if h.m.picker.edit == nil || !strings.Contains(s, str.ConfigPortRange()) || strings.Contains(s, str.ConfigWorldExists("fm")) {
		t.Errorf("want only the port's problem:\n%s", s)
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
	h.press(tea.KeyDown, 0)
	h.press(tea.KeyDown, 0) // to Save
	h.enter()
	if h.m.picker.edit == nil || !strings.Contains(h.screen(), str.ConfigWorldExists("fm")) {
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
	h.press(tea.KeyDown, 0)
	h.press(tea.KeyDown, 0) // from Kit, past Rook, to fm's add row
	h.enter()
	if h.m.picker.edit == nil {
		t.Fatalf("no character editor:\n%s", h.screen())
	}
	// The name goes where + Character was (rows: fm, Kit, Rook, it, + World),
	// showing what to type while empty; the input area keeps only the hint.
	if got := sideText(h, 3); got != str.PickerName() {
		t.Errorf("the empty name field = %q, want its hint", got)
	}
	if got := sideText(h, 4); got != addWorldLabel {
		t.Errorf("row under the name field = %q", got)
	}
	if s := h.screen(); !strings.Contains(s, "│"+addCharHint) || strings.Contains(s, str.PickerName()+": ") {
		t.Errorf("the input area should hold just the hint:\n%s", s)
	}
	cursorAt := func(x, y int) {
		t.Helper()
		if c := h.m.View().Cursor; c == nil || c.X != x || c.Y != y {
			t.Errorf("cursor = %+v, want (%d, %d)", c, x, y)
		}
	}
	cursorAt(2, listY(h, 3))
	h.typeText("O'Br ")
	if got := sideText(h, 3); got != "O'Br" {
		t.Errorf("the name field = %q", got)
	}
	cursorAt(6, listY(h, 3))
	var problem []string
	for _, r := range h.m.pickerRows() {
		if r.kind == rowAddCharError {
			problem = append(problem, r.text)
		}
	}
	if got, want := strings.Join(problem, " "), str.ConfigNameHasSpaces(); got != want {
		t.Errorf("the refused space is explained as %q, want %q", got, want)
	}
	if got := sideText(h, 4); got != problem[0] {
		t.Errorf("row under the name field = %q, want the problem", got)
	}
	h.m.Update(tea.MouseClickMsg{X: 4, Y: listY(h, 4), Button: tea.MouseLeft})
	if e := h.m.addingChar(); e == nil || e.form.value(0) != "O'Br" {
		t.Error("a click on the problem should leave the name be")
	}
	h.typeText("ien")
	if strings.Contains(h.screen(), problem[0]) {
		t.Errorf("a good edit should clear the problem:\n%s", h.screen())
	}
	h.enter()
	cs := h.m.chars["fm/O_Brien"]
	if h.m.picker != nil || cs == nil || cs.Sess == nil || h.m.a.Active() != "fm/O_Brien" {
		t.Fatalf("saving should open, connect and switch to O'Brien:\n%s", h.screen())
	}
	if cs.Ch.Name != "O'Brien" {
		t.Errorf("name = %q", cs.Ch.Name)
	}
}

func TestPickerAddCharacterScrolls(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.press('o', tea.ModCtrl)
	h.press(tea.KeyDown, 0)
	h.press(tea.KeyDown, 0) // from Kit, past Rook, to fm's add row
	h.enter()
	name := strings.Repeat("Kit", 15)
	h.typeText(name)
	sw := SidebarWidth(h.m.width)
	got := sideText(h, 3)
	if !strings.HasSuffix(name, got) || len(got) != sw-3 {
		t.Errorf("the name field = %q, want the last %d of %q", got, sw-3, name)
	}
	if c := h.m.View().Cursor; c == nil || c.X != sw-1 || c.Y != listY(h, 3) {
		t.Errorf("cursor = %+v, want the field's last column", c)
	}
	h.press(tea.KeyHome, 0)
	if got := sideText(h, 3); !strings.HasPrefix(name, got) {
		t.Errorf("after Home the name field = %q, want the start of the name", got)
	}
}

func TestPickerClickAddRows(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	click := func(y int) {
		h.m.Update(tea.MouseClickMsg{X: 4, Y: listY(h, y), Button: tea.MouseLeft})
	}
	h.press('o', tea.ModCtrl) // rows: fm, Kit, Rook, + Character, + World
	click(3)
	if e := h.m.picker.edit; e == nil || e.world != "fm" {
		t.Fatalf("clicking %s didn't open fm's character editor", addCharLabel)
	}
	click(4) // another row leaves the editor for that row's
	if e := h.m.picker.edit; e == nil || e.world != "" {
		t.Fatalf("clicking %s didn't open the world editor", addWorldLabel)
	}
	click(0) // a header shows its world's overview, leaving the editor
	if h.m.picker != nil || h.m.a.Active() != worldSel("fm") {
		t.Errorf("clicking a header should show its world's overview: active %q", h.m.a.Active())
	}
}

func TestNoCharactersPointsAtPicker(t *testing.T) {
	h := newHarness(t, nil)
	if s := h.screen(); !strings.Contains(s, "│"+str.ViewNoCharacters()) {
		t.Errorf("empty config doesn't say how to add a character:\n%s", s)
	}
}

func TestPickerPageFromAddRow(t *testing.T) {
	worlds := map[string]string{}
	for i := range 20 {
		worlds[fmt.Sprintf("w%02d", i)] = fmt.Sprintf("host = \"w.test\"\nport = 1\n\n[[characters]]\nid = \"c%02d\"\nname = \"C%02d\"\n", i, i)
	}
	h := newHarness(t, worlds)
	h.press('o', tea.ModCtrl)
	h.m.picker.sel = addWorldSel
	h.press(tea.KeyPgUp, 0)
	if h.m.picker.sel == addWorldSel {
		t.Fatal("PgUp from + World didn't move")
	}
	h.m.picker.sel = "w00/c00"
	h.press(tea.KeyPgDown, 0)
	if h.m.picker.sel == "w00/c00" {
		t.Error("PgDn didn't move")
	}
}

func TestOpenCommand(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("/open")
	h.enter()
	if h.m.picker == nil {
		t.Fatalf("/open with a character open didn't open the picker:\n%s", h.screen())
	}
	h.press(tea.KeyEscape, 0)

	empty := newHarness(t, nil)
	empty.typeText("/open")
	empty.enter()
	if empty.m.picker == nil {
		t.Fatalf("/open with nothing open didn't open the picker:\n%s", empty.screen())
	}
}

func TestOpenConnectionLabel(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	if s := h.screen(); !strings.Contains(s, addLabel) || strings.Contains(s, "Add connection") {
		t.Errorf("sidebar CTA:\n%s", s)
	}
}
