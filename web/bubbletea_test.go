//go:build !js

package web_test

import (
	"os"
	"regexp"
	"testing"
)

// pinnedOver is the Bubble Tea release the web build's pin sits on. web/go.mod
// replaces Bubble Tea with the commit from charmbracelet/bubbletea#1790
// (GOOS=js support), which is pinnedOver plus a few unreleased upstream
// fixes. .github/workflows/bubbletea-watch.yml opens an issue once a release
// ships #1790, and then the replace goes away.
const pinnedOver = "v2.0.10"

// TestBubbleTeaPinMatchesRoot fails when the root module's Bubble Tea moves
// off the release the pin sits on, so the browser build doesn't quietly run
// an older Bubble Tea than the terminal one. Bumping it: check whether the new
// release has #1790 (drop the replace) or re-pin onto it. Plain text matching
// keeps it free of dependencies; the require pattern wants a "v" so the
// replace line, whose next field is "=>", never matches.
func TestBubbleTeaPinMatchesRoot(t *testing.T) {
	root := find(t, "../go.mod", `(?m)^\s*(?:require\s+)?charm\.land/bubbletea/v2\s+(v\S+)`)
	web := find(t, "go.mod", `(?m)^\s*(?:require\s+)?charm\.land/bubbletea/v2\s+(v\S+)`)
	if root != pinnedOver || web != pinnedOver {
		t.Errorf("Bubble Tea: root go.mod %s, web/go.mod %s, pin is over %s; see pinnedOver", root, web, pinnedOver)
	}
}

func find(t *testing.T, path, pattern string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(pattern).FindSubmatch(b)
	if m == nil {
		t.Fatalf("%s: no Bubble Tea version", path)
	}
	return string(m[1])
}
