package main

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/latrani/Kiln/internal/str"
)

func TestManifestListsFilesAsSlashPaths(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "worlds"), 0o755)
	os.WriteFile(filepath.Join(dir, "config.toml"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), nil, 0o644)
	got, err := manifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"config.toml", "worlds/fm.toml"}; !slices.Equal(got, want) {
		t.Errorf("manifest = %q, want %q", got, want)
	}
}

func TestRunNeedsAllow(t *testing.T) {
	if err := run(nil, io.Discard); err == nil || err.Error() != str.RelayNeedAllow() {
		t.Errorf("err = %v", err)
	}
}

func TestDevNeedsDirs(t *testing.T) {
	if err := run([]string{"-allow", "x.toml", "-dev"}, io.Discard); err == nil || err.Error() != str.RelayDevNeedsDirs() {
		t.Errorf("err = %v", err)
	}
}
