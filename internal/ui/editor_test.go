package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/str"
)

func (h *harness) worldFile(id string) string {
	h.t.Helper()
	b, err := os.ReadFile(filepath.Join(h.dir, "worlds", id+".toml"))
	if err != nil {
		h.t.Fatal(err)
	}
	return string(b)
}

// focusOn moves the open editor's focus to the field labeled label.
func (h *harness) focusOn(label string) {
	h.t.Helper()
	f := h.m.picker.edit.form
	i := f.field(label)
	if i < 0 {
		h.t.Fatalf("no field %q", label)
	}
	for range len(f.fields) {
		if f.focus == i {
			return
		}
		h.press(tea.KeyTab, 0)
	}
	h.t.Fatalf("can't reach %q (hidden?):\n%s", label, h.screen())
}

func TestEditWorldFromPicker(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.press('o', tea.ModCtrl)
	h.press(tea.KeyUp, 0) // fm's header
	h.press('t', tea.ModCtrl)
	e := h.m.picker.edit
	if e == nil || e.kind != editWorld {
		t.Fatalf("no world editor:\n%s", h.screen())
	}
	s := h.screen()
	if !strings.Contains(s, "│Host: muck.test") || !strings.Contains(s, "► "+extraLabel) || strings.Contains(s, maxBytesLabel) {
		t.Errorf("editor should show the basics, extras folded:\n%s", s)
	}
	h.focusOn(extraLabel)
	h.enter() // unfold
	if s := h.screen(); !strings.Contains(s, "▼ "+extraLabel) || !strings.Contains(s, reconnectLabel) {
		t.Fatalf("extras didn't unfold:\n%s", s)
	}
	h.focusOn(maxBytesLabel)
	h.press(tea.KeyBackspace, 0)
	h.press(tea.KeyBackspace, 0)
	h.typeText("400")
	h.focusOn(echoLabel)
	h.press(' ', 0) // default → on
	h.focusOn(saveLabel)
	h.enter()
	if h.m.picker == nil || h.m.picker.edit != nil {
		t.Fatalf("save should go back to the picker:\n%s", h.screen())
	}
	got := h.worldFile("fm")
	want := strings.Replace(fmWorld, "max_line_bytes = 20\n", "max_line_bytes = 400\nlocal_echo = true\n", 1)
	if got != want {
		t.Errorf("file =\n%s\nwant\n%s", got, want)
	}
	if kit := h.m.chars["fm/kit"]; kit.ch.MaxLineBytes != 400 || !kit.ch.LocalEcho {
		t.Errorf("open character not updated: %+v", kit.ch)
	}
}

func TestAddWorldWithExtras(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	toAddWorld(h)
	h.typeText("sp")
	h.press(tea.KeyTab, 0)
	h.typeText("sp.test")
	h.press(tea.KeyTab, 0)
	h.typeText("7")
	h.focusOn(extraLabel)
	h.enter()
	h.focusOn("Autoconnect")
	h.press(' ', 0)
	h.press(' ', 0) // default → on → off
	h.focusOn(saveLabel)
	h.enter()
	if got := h.worldFile("sp"); !strings.Contains(got, "host = \"sp.test\"") || !strings.Contains(got, "autoconnect = false\n") {
		t.Errorf("file:\n%s", got)
	}
}

// Ctrl+T edits the active character from the main view, as /edit does,
// and leaves Ctrl+E to the input's end of line.
func TestEditKeyFromMainView(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.press('t', tea.ModCtrl)
	if p := h.m.picker; p == nil || p.edit == nil || p.edit.kind != editChar || p.edit.char != "kit" {
		t.Fatalf("Ctrl+T should edit the active character:\n%s", h.screen())
	}
	h.press(tea.KeyEscape, 0)
	if h.m.picker != nil {
		t.Errorf("Esc should close the picker it came with:\n%s", h.screen())
	}
	h.typeText("abc")
	h.press(tea.KeyHome, 0)
	h.press('e', tea.ModCtrl)
	h.typeText("d")
	if h.m.picker != nil || h.m.input().Value() != "abcd" {
		t.Errorf("Ctrl+E should go to the end of the line, input = %q", h.m.input().Value())
	}
}

// Editing from the main view keeps the sidebar: the open characters,
// the edited one highlighted, not the picker's list of ones to open.
// A click there closes the editor and does what it does.
func TestEditKeepsSidebar(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook")
	h.press('t', tea.ModCtrl)
	if h.m.picker == nil || h.m.picker.edit == nil {
		t.Fatalf("no editor:\n%s", h.screen())
	}
	want := []string{"fm", "× Kit", "× Rook", "", addLabel}
	for y, w := range want {
		if got := strings.TrimSpace(sideRow(h, y)); got != w {
			t.Errorf("sidebar row %d = %q, want %q:\n%s", y, got, w, h.screen())
		}
	}
	h.m.Update(tea.MouseClickMsg{X: 4, Y: 2, Button: tea.MouseLeft})
	if h.m.picker != nil || h.m.active != "fm/rook" {
		t.Errorf("click on Rook: picker open %v, active %s", h.m.picker != nil, h.m.active)
	}
}

func TestEditCharacterCommand(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("/edit")
	h.enter()
	e := h.m.picker.edit
	if e == nil || e.kind != editChar || e.char != "kit" {
		t.Fatalf("/edit should edit the active character:\n%s", h.screen())
	}
	if !strings.Contains(h.screen(), "│Editing fm/Kit") {
		t.Errorf("the editor doesn't say whose it is:\n%s", h.screen())
	}
	h.focusOn("Aliases")
	h.typeText("Kitty, K")
	h.focusOn("Autoconnect")
	h.press(tea.KeyLeft, 0)
	h.press(tea.KeyLeft, 0) // on → default → off, backwards
	h.focusOn(saveLabel)
	h.enter()
	if h.m.picker != nil {
		t.Error("/edit's editor should close the picker when it's done")
	}
	got := h.worldFile("fm")
	if !strings.Contains(got, "id = \"kit\"\nname = \"Kit\"\nautoconnect = false\naliases = [\"Kitty\", \"K\"]\n") {
		t.Errorf("file:\n%s", got)
	}
	if kit := h.m.chars["fm/kit"]; strings.Join(kit.ch.Aliases, ",") != "Kitty,K" {
		t.Errorf("aliases = %q", kit.ch.Aliases)
	}
	h.typeText("/edit world")
	h.enter()
	if e := h.m.picker.edit; e == nil || e.kind != editWorld || e.world != "fm" {
		t.Error("/edit world should edit the active character's world")
	}
}

func TestForgetPassword(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.saved["fm/kit"] = "hunter2"
	h.typeText("/edit")
	h.enter()
	h.focusOn(forgetPWLabel)
	h.enter()
	if _, ok := h.saved["fm/kit"]; ok {
		t.Error("password not forgotten")
	}
	if !strings.Contains(h.screen(), str.EditorForgotPassword("fm", "kit")) {
		t.Errorf("no status:\n%s", h.screen())
	}
}

func TestDeleteCharacterAndWorld(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, "a line")
	h.saved["fm/kit"] = "hunter2"
	h.typeText("/edit")
	h.enter()
	h.focusOn(delCharLabel)
	h.enter()
	if !strings.Contains(h.screen(), upTo(str.EditorDeleteCharacterWarning(mark))) || h.m.chars["fm/kit"] == nil {
		t.Fatalf("the first Enter should only warn:\n%s", h.screen())
	}
	h.press(tea.KeyUp, 0)
	h.press(tea.KeyDown, 0) // moving away disarms it
	h.enter()
	if h.m.chars["fm/kit"] == nil {
		t.Fatal("deleted without a fresh confirmation")
	}
	h.enter()
	if h.m.chars["fm/kit"] != nil || strings.Contains(h.worldFile("fm"), "\"kit\"") {
		t.Fatalf("Kit not deleted:\n%s", h.worldFile("fm"))
	}
	if _, ok := h.saved["fm/kit"]; ok {
		t.Error("saved password kept")
	}
	if _, err := os.Stat(filepath.Join(h.m.d.LogRoot, "fm", "kit")); err != nil {
		t.Errorf("logs touched: %v", err)
	}
	if _, ok := h.m.find("fm/rook"); !ok {
		t.Fatal("deleted the wrong character")
	}

	h.press('o', tea.ModCtrl)
	h.m.picker.sel = worldSel("fm")
	h.press('t', tea.ModCtrl)
	h.focusOn(delWorldLabel)
	h.enter()
	h.enter()
	if !strings.Contains(h.screen(), str.ConfigWorldHasCharacters("fm")[:20]) { // the note is cut to the pane
		t.Fatalf("a world with characters was deleted, or no reason given:\n%s", h.screen())
	}
	h.press(tea.KeyEscape, 0)
	h.m.picker.sel = "fm/rook"
	h.press('t', tea.ModCtrl)
	h.focusOn(delCharLabel)
	h.enter()
	h.enter()
	h.m.picker.sel = worldSel("fm")
	h.press('t', tea.ModCtrl)
	h.focusOn(delWorldLabel)
	h.enter()
	h.enter()
	if _, err := os.Stat(filepath.Join(h.dir, "worlds", "fm.toml")); !os.IsNotExist(err) {
		t.Errorf("world file still there:\n%s", h.screen())
	}
	if len(h.m.cfg.Worlds) != 0 {
		t.Errorf("worlds = %v", h.m.cfg.Worlds)
	}
}

func TestTallFormScrolls(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.Update(tea.WindowSizeMsg{Width: 80, Height: 14})
	h.typeText("/edit world")
	h.enter()
	h.focusOn(extraLabel)
	h.enter()
	h.focusOn(delWorldLabel)
	if s := h.screen(); !strings.Contains(s, " "+delWorldLabel+" ") || strings.Contains(s, "│Host:") {
		t.Errorf("the form should scroll to the focused button:\n%s", s)
	}
	h.focusOn("Host")
	if s := h.screen(); !strings.Contains(s, "│Host:") {
		t.Errorf("and back up:\n%s", s)
	}
}

func TestEditCharacterNotify(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("/edit")
	h.enter()
	h.focusOn(notifyLabel)
	if !strings.Contains(h.screen(), str.EditorDefault("first")) {
		t.Errorf("Notify should show what it inherits:\n%s", h.screen())
	}
	h.press(tea.KeyLeft, 0) // default → none, backwards
	h.focusOn(saveLabel)
	h.enter()
	if got := h.worldFile("fm"); !strings.Contains(got, "name = \"Kit\"\nautoconnect = true\nnotify = \"none\"\n") {
		t.Errorf("file:\n%s", got)
	}
	if kit := h.m.chars["fm/kit"]; kit.ch.Notify != "none" {
		t.Errorf("Notify = %q", kit.ch.Notify)
	}
}

// Ctrl+T in the editor closes it without saving, like Esc (#98): back
// to the picker when it came from there, out of it from the main view.
func TestEditKeyToggles(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.press('t', tea.ModCtrl)
	h.focusOn(aliasesLabel)
	h.typeText("kitty")
	h.press('t', tea.ModCtrl)
	if h.m.picker != nil {
		t.Errorf("Ctrl+T should close the editor and the picker it came with:\n%s", h.screen())
	}
	h.press('o', tea.ModCtrl)
	h.m.picker.sel = worldSel("fm")
	h.press('t', tea.ModCtrl)
	h.press('t', tea.ModCtrl)
	if p := h.m.picker; p == nil || p.edit != nil {
		t.Errorf("Ctrl+T should go back to the picker:\n%s", h.screen())
	}
	h.press(tea.KeyEscape, 0)
	h.m.switchTo(worldSel("fm")) // and from a world's overview, back to it
	h.press('t', tea.ModCtrl)
	h.press('t', tea.ModCtrl)
	if h.m.picker != nil || h.m.active != worldSel("fm") {
		t.Errorf("Ctrl+T should go back to the overview: active %q\n%s", h.m.active, h.screen())
	}
	if got := h.worldFile("fm"); got != fmWorld {
		t.Errorf("Ctrl+T saved:\n%s", got)
	}
}

// Ctrl+T hides the editor with its edits kept, and brings them back the
// next time the same editor opens, from anywhere; Esc throws them away.
func TestEditKeyKeepsDraft(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	aliases := func() string {
		f := h.m.picker.edit.form
		return f.value(f.field(aliasesLabel))
	}
	h.press('t', tea.ModCtrl)
	h.focusOn(aliasesLabel)
	h.typeText("kitty")
	h.press('t', tea.ModCtrl)
	if !strings.Contains(h.screen(), str.StatusDraftKept()) {
		t.Errorf("hiding should say the edits are kept:\n%s", h.screen())
	}
	h.press('o', tea.ModCtrl) // from the picker this time
	h.m.picker.sel = "fm/kit"
	h.press('t', tea.ModCtrl)
	if got := aliases(); got != "kitty" {
		t.Fatalf("draft not back: aliases %q\n%s", got, h.screen())
	}
	h.typeText("cat") // the focus came back too
	if got := aliases(); got != "kittycat" {
		t.Errorf("aliases = %q", got)
	}
	h.press(tea.KeyEscape, 0) // out of the editor, then the picker
	h.press(tea.KeyEscape, 0)
	h.press('t', tea.ModCtrl)
	if got := aliases(); got != "" {
		t.Errorf("Esc should drop the draft: aliases %q", got)
	}
}
