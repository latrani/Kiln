package main

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// bindings is where the page's bindings to Kiln are committed (see the
// go:generate line in main.go).
const bindings = "../web/ui/src/bindings" //str:ok: a path

// TestBindingsAreFresh regenerates the page's bindings with the pinned
// wails3 and fails if they differ from what's committed, as
// internal/str's test does for str.gen.ts.
func TestBindingsAreFresh(t *testing.T) {
	wails := wails3(t)
	if out, err := exec.Command(wails, "version").CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "v3.0.0-beta.32" { //str:ok: the pinned version
		t.Fatalf("wails3 is %q, not v3.0.0-beta.32: go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.32", out) //str:ok: a developer's message
	}
	dir := t.TempDir()
	cmd := exec.Command(wails, "generate", "bindings", "-ts", "-silent", "-d", dir) //str:ok: main.go's go:generate, into dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("wails3 generate bindings: %v\n%s", err, out) //str:ok: a developer's message
	}
	want, got := tree(t, dir), tree(t, bindings)
	for f, b := range want {
		if !bytes.Equal(got[f], b) {
			t.Errorf("%s is stale: run go generate in desktop/", filepath.Join(bindings, f)) //str:ok: a developer's message
		}
	}
	for f := range got {
		if _, ok := want[f]; !ok {
			t.Errorf("%s isn't generated any more: run go generate in desktop/", filepath.Join(bindings, f)) //str:ok: a developer's message
		}
	}
}

// wails3 is the wails3 CLI: on the PATH, or where go install puts it.
func wails3(t *testing.T) string {
	if p, err := exec.LookPath("wails3"); err == nil { //str:ok
		return p
	}
	out, err := exec.Command("go", "env", "GOPATH").Output() //str:ok
	if err == nil {
		p := filepath.Join(strings.TrimSpace(string(out)), "bin", "wails3") //str:ok
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Fatal("no wails3: go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.32") //str:ok: a developer's message
	return ""
}

// tree is every file under dir, by its path in dir.
func tree(t *testing.T, dir string) map[string][]byte {
	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		files[rel] = b
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
