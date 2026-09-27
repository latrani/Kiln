package rules_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/rules"
)

// The starter fuzzball pack tags both sides of a page conversation
// "page", and colors only the side you receive. (Your own echoes can
// still be bold through "self" when they contain your name.)
func TestStarterPackPages(t *testing.T) {
	dir := t.TempDir()
	if err := config.EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte("host = \"h\"\nport = 1\nuse = [\"fuzzball\"]\n[[characters]]\nname = \"Kit\"\n"), 0o600)
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, _ := cfg.Find("fm", "Kit")
	cls, err := classify.New(kit.Rules.Classify, kit.Name, kit.Aliases)
	if err != nil {
		t.Fatal(err)
	}
	hl, err := rules.New(kit.Rules.Highlight)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		line   string
		dir    string // "page/in" or "page/out"
		styled bool
	}{
		{"Mira pages: you around?", "page/in", true},
		{"In a page-pose to you, Mira grins.", "page/in", true},
		{"You sense that Mira is paging you from the Docks.", "page/in", true},
		{`You page, "yep!" to Mira.`, "page/out", false},
		{`You page-pose, "Kit grins." to Mira.`, "page/out", false},
	} {
		tags := cls.Classify(c.line)
		if !slices.Contains(tags, "page") || !slices.Contains(tags, c.dir) {
			t.Errorf("%q: tags %v, want page and %s", c.line, tags, c.dir)
		}
		res := hl.Apply(c.line, cls.Tags(c.line))
		if paged := res.Style.FG == "#ff9f43"; paged != c.styled {
			t.Errorf("%q: page color %v, want %v", c.line, paged, c.styled)
		}
	}
}
