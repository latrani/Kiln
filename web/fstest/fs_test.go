//go:build js

// Package fstest checks that Kiln's file-using packages work on memfs,
// the browser page's filesystem. Run with web/testdata/memfs_exec.sh.
package fstest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
)

func TestConfigOnMemfs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kiln")
	if err := config.EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	world := "host = \"example.org\"\nport = 8899\ntls = true\n\n[[characters]]\nid = \"kit\"\nname = \"Kit\"\n"
	if err := os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte(world), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Find("fm", "kit"); !ok {
		t.Errorf("fm/kit not loaded: %+v", cfg.Worlds)
	}
}

func TestKnownHostsOnMemfs(t *testing.T) {
	kh := conn.KnownHosts{Path: filepath.Join(t.TempDir(), "known_hosts")}
	if err := kh.Trust("example.org:8899", "sha256:abcd"); err != nil {
		t.Fatal(err)
	}
	if fp, ok, err := kh.Lookup("example.org:8899"); err != nil || !ok || fp != "sha256:abcd" {
		t.Errorf("Lookup = %q, %v, %v", fp, ok, err)
	}
}

func TestRenameReplacesOnMemfs(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "config.toml"), filepath.Join(dir, "config.toml.tmp")
	os.WriteFile(a, []byte("old"), 0o644)
	os.WriteFile(b, []byte("new"), 0o644)
	if err := os.Rename(b, a); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(a); string(got) != "new" {
		t.Errorf("got %q", got)
	}
}
