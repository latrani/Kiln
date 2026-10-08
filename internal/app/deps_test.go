package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The core never touches the terminal: no Bubble Tea or lipgloss in its
// dependencies, and no painting.
func TestNoTerminal(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range strings.Fields(string(out)) {
		if strings.Contains(p, "bubbletea") || strings.Contains(p, "lipgloss") {
			t.Errorf("internal/app depends on %s", p)
		}
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range []string{"theme.Paint(", "theme.Fill(", ".Paint(", ".Fill("} {
			if strings.Contains(string(src), call) {
				t.Errorf("%s paints (%s)", f, call)
			}
		}
	}
}
