package app

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/latrani/Kiln/internal/str"
)

// state is Kiln's own state, kept in Deps.StateFile between runs: what a
// person chose about the screen, not config. Fields it doesn't know are
// kept as they were.
type state struct {
	fields map[string]json.RawMessage
}

const sidebarCollapsedKey = "sidebar_collapsed" //str:ok a file format's field

// loadState reads the state file; a missing one is empty, and one that
// can't be read is empty too, with the statusline saying why.
func (a *App) loadState() {
	a.state.fields = map[string]json.RawMessage{}
	if a.d.StateFile == "" {
		return
	}
	b, err := os.ReadFile(a.d.StateFile)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err == nil {
		err = json.Unmarshal(b, &a.state.fields)
	}
	if err == nil && a.state.fields == nil { // null: not an object
		err = errors.New(string(b))
	}
	if err != nil {
		a.state.fields = map[string]json.RawMessage{}
		a.SetStatus(true, str.StatusStateUnreadable(err))
		return
	}
	json.Unmarshal(a.state.fields[sidebarCollapsedKey], &a.sidebarCollapsed)
}

// saveState writes the state file; a failure says so in the statusline.
func (a *App) saveState() {
	if a.d.StateFile == "" {
		return
	}
	v, _ := json.Marshal(a.sidebarCollapsed)
	a.state.fields[sidebarCollapsedKey] = v
	b, _ := json.MarshalIndent(a.state.fields, "", "  ")
	if err := writeWhole(a.d.StateFile, append(b, '\n')); err != nil {
		a.SetStatus(true, str.StatusStateNotSaved(err))
	}
}

// writeWhole writes b to path whole or not at all: a new file beside it,
// renamed over it, as the config editor and the password file do.
func writeWhole(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".state-*") //str:ok a temp file's name
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}

// SidebarCollapsed reports whether the sidebar is collapsed in wide
// mode, as a person last chose; it's saved between runs.
func (a *App) SidebarCollapsed() bool { return a.sidebarCollapsed }

// SetSidebarCollapsed collapses the sidebar, or not, and saves it. It
// changes even when it can't be saved.
func (a *App) SetSidebarCollapsed(on bool) {
	a.sidebarCollapsed = on
	a.saveState()
}

// Activity is the collapsed sidebar's badge: the open characters' unread
// counts summed, and whether any has an unread priority line, as their
// rows show.
func (a *App) Activity() (unread int, priority bool) {
	for _, c := range a.chars {
		unread += c.Unread
		priority = priority || c.Priority && c.Unread > 0
	}
	return unread, priority
}
