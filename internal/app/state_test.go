package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/latrani/Kiln/internal/str"
)

// stateApp is an App saving its state in file.
func stateApp(file string) *App { return New(Deps{StateFile: file}) }

func TestSidebarCollapsedSaved(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.json")
	a := stateApp(file)
	if a.SidebarCollapsed() {
		t.Fatal("starts collapsed")
	}
	a.SetSidebarCollapsed(true)
	if !stateApp(file).SidebarCollapsed() {
		t.Error("a new App didn't read it back collapsed")
	}
}

func TestStateFileKeepsOtherFields(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(file, []byte(`{"other": 1}`), 0o600)
	stateApp(file).SetSidebarCollapsed(true)
	var got map[string]any
	b, _ := os.ReadFile(file)
	if err := json.Unmarshal(b, &got); err != nil || got["other"] != 1.0 || got["sidebar_collapsed"] != true {
		t.Errorf("file = %s (%v)", b, err)
	}
}

func TestStateFileUnreadable(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(file, []byte("not json"), 0o600)
	a := stateApp(file)
	want := strings.SplitN(str.StatusStateUnreadable(errors.New("\x00")), "\x00", 2)[0]
	if a.SidebarCollapsed() || !a.Status().Err || !strings.HasPrefix(a.Status().Text, want) {
		t.Errorf("collapsed %v, status %+v", a.SidebarCollapsed(), a.Status())
	}
}

func TestStateFileMissing(t *testing.T) {
	a := stateApp(filepath.Join(t.TempDir(), "state.json"))
	if a.Status().Text != "" {
		t.Errorf("status %q for a missing file", a.Status().Text)
	}
}

func TestStateFileUnwritable(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "afile"), nil, 0o600)
	a := stateApp(filepath.Join(dir, "afile", "state.json")) // under a file: can't be made
	a.SetSidebarCollapsed(true)
	want := strings.SplitN(str.StatusStateNotSaved(errors.New("\x00")), "\x00", 2)[0]
	if !a.SidebarCollapsed() || !a.Status().Err || !strings.HasPrefix(a.Status().Text, want) {
		t.Errorf("collapsed %v, status %+v; want it changed anyway, and said", a.SidebarCollapsed(), a.Status())
	}
}

func TestActivity(t *testing.T) {
	a, _ := testApp(t)
	openAll(t, a, "fm/kit", "fm/rook")
	a.Char("fm/kit").Unread = 3
	rook := a.Char("fm/rook")
	rook.Unread, rook.Priority = 2, true
	if n, p := a.Activity(); n != 5 || !p {
		t.Errorf("Activity = %d, %v; want 5, true", n, p)
	}
	rook.Unread = 0
	if n, p := a.Activity(); n != 3 || p {
		t.Errorf("Activity = %d, %v; want 3, false (no unread priority line)", n, p)
	}
}
