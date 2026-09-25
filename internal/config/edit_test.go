package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// uglyWorld has comments everywhere, a multi-line array, inline tables,
// a character's own rules, and odd spacing, all of which edits must keep.
const uglyWorld = `# FurryMUCK, my main
host = "furrymuck.com"   # the big one
port = 8899
tls = true
use = [ "fuzzball" ]       # starter rules

# my characters
[[characters]]              # the main
name = "Kit"                # the in-game name
aliases = ["Kitty",
           "K"]

[[characters.highlight]]
match = { tags = ["page"] }
style = { fg = "#ff9f43", bold = true }

[[characters]]
name = "Rook"
autoconnect = true

# added by /highlight
[[highlight]]
match = { pattern = '(?i)lighthouse' }
style = { fg = "#ffd166", bold = true }
`

func editDir(t *testing.T, world string) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, map[string]string{"worlds/fm.toml": world, "packs/fuzzball.toml": ""})
	return dir
}

func readWorld(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "worlds", "fm.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestWriteWorldChangesOnlyWhatChanged(t *testing.T) {
	dir := editDir(t, uglyWorld)
	s, inh, err := ReadWorld(dir, "fm")
	if err != nil {
		t.Fatal(err)
	}
	if inh.MaxLineBytes != DefaultMaxLineBytes || s.Port != 8899 || s.Login != nil {
		t.Fatalf("read %+v, inherited %+v", s, inh)
	}
	if err := WriteWorld(dir, "fm", s); err != nil {
		t.Fatal(err)
	}
	if got := readWorld(t, dir); got != uglyWorld {
		t.Fatalf("an unchanged save changed the file:\n%s", got)
	}
	s.Port = 9999
	s.Use = nil // unset: its line goes, comment and all
	login, n := "connect {name} {password}", 4000
	s.Login, s.MaxLineBytes = &login, &n
	if err := WriteWorld(dir, "fm", s); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(uglyWorld, "port = 8899\ntls = true\nuse = [ \"fuzzball\" ]       # starter rules\n",
		"port = 9999\ntls = true\nlogin = \"connect {name} {password}\"\nmax_line_bytes = 4000\n", 1)
	if got := readWorld(t, dir); got != want {
		t.Errorf("file =\n%s\nwant\n%s", got, want)
	}
	if _, err := Load(dir); err != nil {
		t.Errorf("result doesn't load: %v", err)
	}
}

func TestWriteWorldKeepsTrailingComment(t *testing.T) {
	dir := editDir(t, uglyWorld)
	s, _, _ := ReadWorld(dir, "fm")
	s.Host = "furry.example"
	if err := WriteWorld(dir, "fm", s); err != nil {
		t.Fatal(err)
	}
	if got := readWorld(t, dir); !strings.Contains(got, "host = \"furry.example\"   # the big one\n") {
		t.Errorf("comment after the value lost:\n%s", got)
	}
}

func TestWriteWorldRejectsBadResult(t *testing.T) {
	dir := editDir(t, uglyWorld)
	s, _, _ := ReadWorld(dir, "fm")
	bad := "sideways"
	s.NewlineMode = &bad
	if err := WriteWorld(dir, "fm", s); err == nil {
		t.Fatal("saved an invalid newline_mode")
	}
	if got := readWorld(t, dir); got != uglyWorld {
		t.Error("a refused edit changed the file")
	}
}

func TestWriteCharacter(t *testing.T) {
	dir := editDir(t, uglyWorld)
	s, inh, err := ReadCharacter(dir, "fm", "Kit")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(s.Aliases, ",") != "Kitty,K" || s.LocalEcho != nil || inh.Autoconnect {
		t.Fatalf("read %+v, inherited %+v", s, inh)
	}
	on := true
	s.Aliases, s.LocalEcho = []string{"Kitty"}, &on
	if err := WriteCharacter(dir, "fm", "Kit", s); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(uglyWorld, "aliases = [\"Kitty\",\n           \"K\"]\n",
		"aliases = [\"Kitty\"]\nlocal_echo = true\n", 1)
	if got := readWorld(t, dir); got != want {
		t.Errorf("file =\n%s\nwant\n%s", got, want)
	}
	r, _, _ := ReadCharacter(dir, "fm", "Rook")
	r.Autoconnect = nil // back to inheriting
	if err := WriteCharacter(dir, "fm", "Rook", r); err != nil {
		t.Fatal(err)
	}
	if got := readWorld(t, dir); strings.Contains(got, "autoconnect") {
		t.Errorf("autoconnect not removed:\n%s", got)
	}
	if err := WriteCharacter(dir, "fm", "Nobody", r); err == nil {
		t.Error("wrote a character that doesn't exist")
	}
}

func TestWriteCharacterAddsAfterNameComment(t *testing.T) {
	dir := editDir(t, "host = \"h\"\nport = 1\n[[characters]]\nname = \"Ash\"  # hi\n[[characters.classify]]\ntag = \"x\"\npattern = \"y\"\n")
	on := true
	if err := WriteCharacter(dir, "fm", "Ash", CharacterSettings{Autoconnect: &on}); err != nil {
		t.Fatal(err)
	}
	want := "host = \"h\"\nport = 1\n[[characters]]\nname = \"Ash\"  # hi\nautoconnect = true\n[[characters.classify]]\ntag = \"x\"\npattern = \"y\"\n"
	if got := readWorld(t, dir); got != want {
		t.Errorf("file =\n%q\nwant\n%q", got, want)
	}
}

func TestDeleteCharacter(t *testing.T) {
	dir := editDir(t, uglyWorld)
	if err := DeleteCharacter(dir, "fm", "Kit"); err != nil {
		t.Fatal(err)
	}
	start := strings.Index(uglyWorld, "[[characters]]              # the main")
	end := strings.Index(uglyWorld, "[[characters]]\nname = \"Rook\"")
	want := uglyWorld[:start] + uglyWorld[end:]
	if got := readWorld(t, dir); got != want {
		t.Errorf("file =\n%s\nwant\n%s", got, want)
	}
	if err := DeleteCharacter(dir, "fm", "Rook"); err != nil {
		t.Fatal(err)
	}
	got := readWorld(t, dir)
	if strings.Contains(got, "[[characters") || strings.Contains(got, "Rook") || !strings.Contains(got, "# added by /highlight\n[[highlight]]") {
		t.Errorf("deleting the last character took the wrong lines:\n%s", got)
	}
}

func TestDeleteWorld(t *testing.T) {
	dir := editDir(t, uglyWorld)
	if err := DeleteWorld(dir, "fm"); err == nil || !strings.Contains(err.Error(), "2 character") {
		t.Fatalf("deleted a world with characters: %v", err)
	}
	DeleteCharacter(dir, "fm", "Kit")
	DeleteCharacter(dir, "fm", "Rook")
	if err := DeleteWorld(dir, "fm"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "worlds", "fm.toml")); !os.IsNotExist(err) {
		t.Error("world file still there")
	}
}
