package config_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/theme"
)

// The starter fuzzball pack tags both sides of a page or whisper
// conversation, and only the side you receive asks for attention.
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
	hl := rules.New(theme.Builtin())
	judge := rules.Judge{Attention: kit.Rules.Attention, Quiet: kit.Rules.Quiet}
	for _, c := range []struct {
		line string
		dir  string // e.g. "page/in" or "whisper/out"
		in   bool
	}{
		{`Mira whispers, "psst"`, "whisper/in", true},
		{`Mira whispers "psst"`, "whisper/in", true},
		{`You whisper, "hi" to Mira.`, "whisper/out", false},
		{"Mira pages: you around?", "page/in", true},
		{"In a page-pose to you, Mira grins.", "page/in", true},
		{"You sense that Mira is paging you from the Docks.", "page/in", true},
		{`You page, "yep!" to Mira.`, "page/out", false},
		{`You page-pose, "Rook grins." to Mira.`, "page/out", false}, // naming Kit would tag it self
	} {
		kind, _, _ := strings.Cut(c.dir, "/")
		tags := cls.Classify(c.line)
		if !slices.Contains(tags, kind) || !slices.Contains(tags, c.dir) {
			t.Errorf("%q: tags %v, want %s and %s", c.line, tags, kind, c.dir)
		}
		ts := cls.Tags(c.line)
		attention, runs := judge.Of(ts).Attention, hl.Runs(c.line, ts)
		// Only pages and whispers to you ask for attention; the built-in
		// theme styles both ways alike.
		if attention != c.in || runs == nil {
			t.Errorf("%q: attention %v, styled %v; want attention %v, styled", c.line, attention, runs != nil, c.in)
		}
	}
}
