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

func TestNameChars(t *testing.T) {
	for _, ok := range []string{"", "Kit", "O'Brien", "Kit.Fox", "a$b", "(Rook)"} {
		if err := NameChars(ok); err != nil {
			t.Errorf("NameChars(%q) = %v, want ok", ok, err)
		}
	}
	for _, bad := range []string{"Kit Fox", "a=b", "a&b", "a|b", "!Kit", "*Kit", "#Kit", "$Kit", "Kït", "a\tb"} {
		if NameChars(bad) == nil {
			t.Errorf("NameChars(%q) accepted", bad)
		}
	}
}

func TestCheckName(t *testing.T) {
	for _, bad := range []string{"", "me", "HERE", "home", "nil"} {
		if CheckName(bad) == nil {
			t.Errorf("CheckName(%q) accepted", bad)
		}
	}
	if err := CheckName("Kit"); err != nil {
		t.Errorf("CheckName(Kit) = %v", err)
	}
}

func TestCharID(t *testing.T) {
	for name, want := range map[string]string{"Kit": "Kit", "O'Brien": "O_Brien", "Kit.Fox": "Kit_Fox", "Ash-2_x": "Ash-2_x"} {
		if got := CharID(name); got != want {
			t.Errorf("CharID(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestAddCharacter(t *testing.T) {
	dir := t.TempDir()
	world := "# fm\nhost = \"h\"\nport = 1\n\n[[characters]]\nname = \"Kit\"\n\n[[highlight]]\nmatch = { pattern = \"x\" }\n"
	write(t, dir, map[string]string{"worlds/fm.toml": world})
	id, err := AddCharacter(dir, "fm", "O'Brien")
	if err != nil {
		t.Fatal(err)
	}
	if id != "O_Brien" {
		t.Errorf("id = %q", id)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "worlds", "fm.toml"))
	if !strings.HasPrefix(string(b), world) {
		t.Errorf("existing content changed:\n%s", b)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("file no longer loads: %v\n%s", err, b)
	}
	ch, ok := cfg.Find("fm", "O_Brien")
	if !ok || ch.Name != "O'Brien" {
		t.Fatalf("new character = %+v, %v\n%s", ch, ok, b)
	}
	if len(ch.Rules.Highlight) != 1 {
		t.Errorf("world rules = %+v; the character must not swallow them", ch.Rules.Highlight)
	}

	// A plain name needs no id line.
	if _, err := AddCharacter(dir, "fm", "Rook"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(dir, "worlds", "fm.toml"))
	if !strings.HasSuffix(string(b), "[[characters]]\nname = \"Rook\"\n") {
		t.Errorf("plain name block:\n%s", b)
	}
}

func TestAddCharacterErrors(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"worlds/fm.toml": "host = \"h\"\nport = 1\n\n[[characters]]\nname = \"O'Brien\"\nid = \"O_Brien\"\n"})
	for _, name := range []string{"", "me", "Kit Fox", "o.brien"} {
		if _, err := AddCharacter(dir, "fm", name); err == nil {
			t.Errorf("AddCharacter(%q) accepted", name)
		}
	}
	if _, err := AddCharacter(dir, "missing", "Kit"); err == nil {
		t.Error("missing world accepted")
	}
	if _, err := AddCharacter(dir, "../evil", "Kit"); err == nil {
		t.Error("bad world id accepted")
	}
}

func TestAddWorld(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	if err := AddWorld(dir, "fm", "muck.example.org", 8899, true); err != nil {
		t.Fatal(err)
	}
	if _, err := AddCharacter(dir, "fm", "Kit"); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, ok := cfg.Find("fm", "Kit")
	if !ok || kit.Host != "muck.example.org" || kit.Port != 8899 || !kit.TLS {
		t.Fatalf("kit = %+v, %v", kit, ok)
	}
	if len(kit.Rules.Classify) == 0 {
		t.Error("the fuzzball pack isn't used")
	}
}

func TestAddWorldWithoutFuzzballPack(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"worlds/.keep": ""})
	if err := AddWorld(dir, "fm", "h", 23, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err != nil {
		t.Fatalf("world uses a pack that isn't there: %v", err)
	}
}

func TestAddWorldErrors(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"worlds/Fm.toml": "host = \"h\"\nport = 1\n"})
	for _, c := range []struct {
		id, host string
		port     int
	}{
		{"", "h", 1}, {"a b", "h", 1}, {"../x", "h", 1}, {"fm", "h", 1},
		{"new", "", 1}, {"new", "a b", 1}, {"new", "h", 0}, {"new", "h", 65536},
	} {
		if err := AddWorld(dir, c.id, c.host, c.port, false); err == nil {
			t.Errorf("AddWorld(%q, %q, %d) accepted", c.id, c.host, c.port)
		}
	}
}
