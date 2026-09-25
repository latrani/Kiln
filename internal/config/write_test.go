package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestAppendHighlightAfterCharacterTable(t *testing.T) {
	dir := t.TempDir()
	world := "# my world\nhost = \"h\"\nport = 1\n\n[[characters]]\nid = \"kit\"\nname = \"Kit\"\n"
	write(t, dir, map[string]string{"worlds/fm.toml": world})
	if err := AppendHighlight(dir, "fm", `the "lighthouse" (old)`); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "worlds", "fm.toml"))
	if !strings.HasPrefix(string(b), world) {
		t.Errorf("existing content changed:\n%s", b)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("appended file no longer loads: %v\n%s", err, b)
	}
	kit, _ := cfg.Find("fm", "kit")
	if len(kit.Rules.Highlight) != 1 {
		t.Fatalf("rules = %+v (the rule must land at world level, not inside the last [[characters]])", kit.Rules.Highlight)
	}
	r := kit.Rules.Highlight[0]
	re := regexp.MustCompile(r.Match.Pattern)
	if !re.MatchString(`I saw THE "LIGHTHOUSE" (OLD) glow`) || re.MatchString("the lighthouse old") {
		t.Errorf("pattern %q should match the text literally, case-insensitively", r.Match.Pattern)
	}
	if r.Style != HighlightStyle {
		t.Errorf("style = %+v", r.Style)
	}
}

func TestAppendHighlightKeepsSpacingAndEscapesDEL(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"worlds/fm.toml": "host = \"h\"\nport = 1\n\n[[characters]]\nid = \"kit\"\nname = \"Kit\"\n"})
	if err := AppendHighlight(dir, "fm", "a  b\x7fc"); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("appended file no longer loads: %v", err)
	}
	kit, _ := cfg.Find("fm", "kit")
	re := regexp.MustCompile(kit.Rules.Highlight[0].Match.Pattern)
	if !re.MatchString("A  B\x7fC") || re.MatchString("a b\x7fc") {
		t.Errorf("pattern %q should keep both spaces and the DEL", re)
	}
}

func TestAppendHighlightErrors(t *testing.T) {
	dir := t.TempDir()
	if err := AppendHighlight(dir, "fm", "   "); err == nil {
		t.Error("empty text accepted")
	}
	if err := AppendHighlight(dir, "../evil", "x"); err == nil {
		t.Error("bad world id accepted")
	}
	if err := AppendHighlight(dir, "missing", "x"); err == nil {
		t.Error("missing world file accepted")
	}
}
