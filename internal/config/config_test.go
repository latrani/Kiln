package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write creates files under dir from a map of relative path → content.
func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadResolvesInheritance(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"config.toml": "[defaults]\nmax_line_bytes = 1000\n",
		"packs/p.toml": `
[[classify]]
tag = "page"
pattern = '^\S+ pages: '
`,
		"worlds/fm.toml": `
host = "example.org"
port = 8899
tls = true
use = ["p"]
login = "connect {name} {password}"

[[classify]]
tag = "ooc"
pattern = '^OOC'

[characters.kit]
name = "Kit"
aliases = ["Kitty"]

[characters.rook]
name = "Rook"
max_line_bytes = 500
login = "co {name} {password}"

[[characters.rook.classify]]
tag = "mine"
pattern = 'Rook'
`,
	})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, ok := cfg.Find("fm", "kit")
	if !ok {
		t.Fatal("kit not found")
	}
	if kit.Name != "Kit" || kit.Aliases[0] != "Kitty" || kit.Host != "example.org" || kit.Port != 8899 || !kit.TLS {
		t.Errorf("kit basics wrong: %+v", kit)
	}
	if kit.TLSTrust != "pin" {
		t.Errorf("TLSTrust = %q, want default pin", kit.TLSTrust)
	}
	if kit.MaxLineBytes != 1000 {
		t.Errorf("kit MaxLineBytes = %d, want 1000 from [defaults]", kit.MaxLineBytes)
	}
	if kit.NewlineMode != "batch" {
		t.Errorf("kit NewlineMode = %q, want built-in batch", kit.NewlineMode)
	}
	if kit.Login != "connect {name} {password}" {
		t.Errorf("kit Login = %q, want world's", kit.Login)
	}
	if tags := classifyTags(kit.Rules); tags != "page,ooc" {
		t.Errorf("kit classify tags = %s, want pack then world: page,ooc", tags)
	}

	rook, _ := cfg.Find("fm", "rook")
	if rook.MaxLineBytes != 500 || rook.Login != "co {name} {password}" {
		t.Errorf("rook overrides lost: %+v", rook)
	}
	if tags := classifyTags(rook.Rules); tags != "page,ooc,mine" {
		t.Errorf("rook classify tags = %s, want page,ooc,mine", tags)
	}
	if tags := classifyTags(kit.Rules); strings.Contains(tags, "mine") {
		t.Error("rook's rule leaked into kit")
	}
}

func classifyTags(r Rules) string {
	var tags []string
	for _, c := range r.Classify {
		tags = append(tags, c.Tag)
	}
	return strings.Join(tags, ",")
}

func TestLoadEmptyDirIsEmptyConfig(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil || len(cfg.Worlds) != 0 {
		t.Errorf("Load(empty) = %+v, %v", cfg, err)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := []struct {
		name, world, wantErr string
	}{
		{"missing host", "port = 1\n", "host is required"},
		{"bad port", "host = \"h\"\nport = 0\n", "port must be"},
		{"missing name", "host = \"h\"\nport = 1\n[characters.kit]\n", "name is required"},
		{"unknown key", "host = \"h\"\nport = 1\nhots = \"typo\"\n", `unknown key "hots"`},
		{"unknown pack", "host = \"h\"\nport = 1\nuse = [\"nope\"]\n", `unknown pack "nope"`},
		{"bad regex", "host = \"h\"\nport = 1\n[[classify]]\ntag = \"x\"\npattern = '('\n[characters.kit]\nname = \"Kit\"\n", "classify rule 1"},
		{"empty highlight match", "host = \"h\"\nport = 1\n[[highlight]]\nattention = true\n[characters.kit]\nname = \"Kit\"\n", "match needs tags or pattern"},
		{"bad trust", "host = \"h\"\nport = 1\ntls_trust = \"yolo\"\n", "tls_trust"},
		{"bad newline mode", "host = \"h\"\nport = 1\nnewline_mode = \"x\"\n[characters.kit]\nname = \"Kit\"\n", "newline_mode"},
		{"bad char id", "host = \"h\"\nport = 1\n[characters.\"a/b\"]\nname = \"X\"\n", "id may only use"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, map[string]string{"worlds/w.toml": c.world})
			_, err := Load(dir)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err = %v, want containing %q", err, c.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "w.toml") {
				t.Errorf("err %q should name the file", err)
			}
		})
	}
}

func TestEnsureDefaultsWritesStarterFilesOnce(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"config.toml", "packs/fuzzball.toml"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("%s not written: %v", rel, err)
		}
	}
	// User edits must survive a second run.
	os.WriteFile(filepath.Join(dir, "config.toml"), []byte("# mine\n"), 0o600)
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	if string(b) != "# mine\n" {
		t.Errorf("EnsureDefaults overwrote config.toml: %q", b)
	}
}

func TestStarterPackLoads(t *testing.T) {
	dir := t.TempDir()
	EnsureDefaults(dir)
	write(t, dir, map[string]string{"worlds/fm.toml": "host = \"h\"\nport = 1\nuse = [\"fuzzball\"]\n[characters.kit]\nname = \"Kit\"\n"})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, _ := cfg.Find("fm", "kit")
	if len(kit.Rules.Classify) == 0 || len(kit.Rules.Highlight) == 0 {
		t.Errorf("fuzzball pack rules missing: %+v", kit.Rules)
	}
	if kit.MaxLineBytes != 2047 {
		t.Errorf("MaxLineBytes = %d, want 2047", kit.MaxLineBytes)
	}
}

func TestAutoconnectInherits(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"worlds/a.toml": "host = \"h\"\nport = 1\nautoconnect = true\n[characters.kit]\nname = \"Kit\"\n[characters.rook]\nname = \"Rook\"\nautoconnect = false\n",
		"worlds/b.toml": "host = \"h\"\nport = 1\n[characters.ash]\nname = \"Ash\"\n",
	})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		world, char string
		want        bool
	}{{"a", "kit", true}, {"a", "rook", false}, {"b", "ash", false}} {
		ch, _ := cfg.Find(c.world, c.char)
		if ch.Autoconnect != c.want {
			t.Errorf("%s/%s Autoconnect = %v, want %v", c.world, c.char, ch.Autoconnect, c.want)
		}
	}
}
