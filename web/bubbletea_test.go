//go:build !js

package web_test

import (
	"os"
	"regexp"
	"testing"
)

// TestBubbleTeaCopyMatchesRoot fails when the root module's Bubble Tea
// version drifts from the copy in third_party/bubbletea. The copy states
// the upstream version it is on its "upstream:" line in KILN.md (a replace
// hides it from go.mod), and the root go.mod and this module's require
// must both name that version. Plain text matching keeps it free of
// dependencies; the require pattern wants a "v" so the replace line, whose
// next field is "=>", never matches.
func TestBubbleTeaCopyMatchesRoot(t *testing.T) {
	copied := find(t, "third_party/bubbletea/KILN.md", `(?m)^upstream: charm\.land/bubbletea/v2 (v\S+)$`)
	root := find(t, "../go.mod", `(?m)^\s*(?:require\s+)?charm\.land/bubbletea/v2\s+(v\S+)`)
	web := find(t, "go.mod", `(?m)^\s*(?:require\s+)?charm\.land/bubbletea/v2\s+(v\S+)`)
	if root != copied || web != copied {
		t.Errorf("Bubble Tea: root go.mod %s, web/go.mod %s, third_party copy %s; recopy it (see KILN.md)", root, web, copied)
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
