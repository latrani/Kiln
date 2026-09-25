package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
	h.press('e', tea.ModCtrl)
	e := h.m.picker.edit
	if e == nil || e.kind != editWorld {
		t.Fatalf("no world editor:\n%s", h.screen())
	}
	s := h.screen()
	if !strings.Contains(s, "│Host: muck.test") || !strings.Contains(s, "► "+extraLabel) || strings.Contains(s, "Max bytes") {
		t.Errorf("editor should show the basics, extras folded:\n%s", s)
	}
	h.focusOn(extraLabel)
	h.enter() // unfold
	if s := h.screen(); !strings.Contains(s, "▼ "+extraLabel) || !strings.Contains(s, "Local echo") {
		t.Fatalf("extras didn't unfold:\n%s", s)
	}
	h.focusOn("Max bytes")
	h.press(tea.KeyBackspace, 0)
	h.press(tea.KeyBackspace, 0)
	h.typeText("400")
	h.focusOn("Local echo")
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
	h.focusOn(extraLabel)
	h.enter()
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
	if !strings.Contains(h.screen(), "forgot fm/kit's saved password") {
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
	if !strings.Contains(h.screen(), "Enter again to delete") || h.m.chars["fm/kit"] == nil {
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
	h.press('e', tea.ModCtrl)
	h.focusOn(delWorldLabel)
	h.enter()
	h.enter()
	if !strings.Contains(h.screen(), "still has 1 character") {
		t.Fatalf("a world with characters was deleted, or no reason given:\n%s", h.screen())
	}
	h.press(tea.KeyEscape, 0)
	h.m.picker.sel = "fm/rook"
	h.press('e', tea.ModCtrl)
	h.focusOn(delCharLabel)
	h.enter()
	h.enter()
	h.m.picker.sel = worldSel("fm")
	h.press('e', tea.ModCtrl)
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
	if s := h.screen(); !strings.Contains(s, "[ "+delWorldLabel+" ]") || strings.Contains(s, "│Host:") {
		t.Errorf("the form should scroll to the focused button:\n%s", s)
	}
	h.focusOn("Host")
	if s := h.screen(); !strings.Contains(s, "│Host:") {
		t.Errorf("and back up:\n%s", s)
	}
}
