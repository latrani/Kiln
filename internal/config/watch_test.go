package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchReportsTomlChangesOnce(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	w, err := Watch(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	p := filepath.Join(dir, "worlds", "fm.toml")
	for i := 0; i < 3; i++ { // a burst of saves
		os.WriteFile(p, []byte("host = \"h\"\nport = 1\n"), 0o600)
	}
	select {
	case <-w.Changes():
	case <-time.After(3 * time.Second):
		t.Fatal("no change reported")
	}
	select {
	case <-w.Changes():
		t.Error("burst reported more than once")
	case <-time.After(2 * Debounce):
	}
}

func TestWatchIgnoresNonToml(t *testing.T) {
	dir := t.TempDir()
	EnsureDefaults(dir)
	w, err := Watch(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	os.WriteFile(filepath.Join(dir, "worlds", ".fm.toml.swp"), []byte("x"), 0o600)
	select {
	case <-w.Changes():
		t.Error("reported a non-.toml change")
	case <-time.After(3 * Debounce):
	}
}
