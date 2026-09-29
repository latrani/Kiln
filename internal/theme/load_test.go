package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/latrani/Kiln/internal/str"
)

func writeTheme(t *testing.T, dir, name, body string) {
	t.Helper()
	os.MkdirAll(filepath.Join(dir, "themes"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, "themes", name+".toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuiltinIsTodaysLook(t *testing.T) {
	b := Builtin()
	for _, c := range []struct {
		role Role
		want string
	}{
		{ScrollbackEcho, "\x1b[2m"},
		{SidebarActive, "\x1b[7m"},
		{StatusError, "\x1b[31m"},
		{LinkHover, "\x1b[4;94m"},
		{InputOverLimit, "\x1b[97;41m"},
		{SidebarAttention, "\x1b[1;38;2;255;209;102m"},
		{Sidebar, ""},
	} {
		if got := b.SGR(c.role); got != c.want {
			t.Errorf("builtin %s = %q, want %q", c.role, got, c.want)
		}
	}
}

func TestLoadWithoutAFileIsBuiltin(t *testing.T) {
	th, err := Load(t.TempDir())
	if err != nil || th.SGR(StatusError) != Builtin().SGR(StatusError) {
		t.Errorf("Load = %v, %v", th, err)
	}
}

func TestExtendsDefaultIsBuiltin(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "default", "extends = \"default\"\n[ui]\n\"status.error\" = { fg = \"#ff0000\" }\n")
	th, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if th.SGR(StatusError) != "\x1b[38;2;255;0;0m" || th.SGR(SidebarActive) != "\x1b[7m" {
		t.Errorf("want the override on top of the built-in: %q, %q", th.SGR(StatusError), th.SGR(SidebarActive))
	}
}

func TestExtendsAnotherTheme(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "ember", "extends = \"default\"\n[palette]\nember = \"#ff9f43\"\n")
	writeTheme(t, dir, "default", "extends = \"ember\"\n[ui]\n\"status.error\" = { fg = \"ember\" }\n")
	th, err := Load(dir)
	if err != nil || th.SGR(StatusError) != "\x1b[38;2;255;159;67m" {
		t.Errorf("Load = %q, %v", th.SGR(StatusError), err)
	}
}

func TestExtendsCycle(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "default", "extends = \"a\"\n") // default → a → b → a
	writeTheme(t, dir, "a", "extends = \"b\"\n")
	writeTheme(t, dir, "b", "extends = \"a\"\n")
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), str.ThemeExtendsLoop("b.toml", "a")) {
		t.Errorf("err = %v", err)
	}
}

func TestLoadErrors(t *testing.T) {
	for _, c := range []struct{ body, want string }{
		{"extends = \"nope\"\n", str.ThemeNoTheme("nope")},
		{"[ui\n", "themes/default.toml"},
		{"[ui]\n\"sidebar.nope\" = { bold = true }\n", str.ThemeUnknownRole("default.toml", "sidebar.nope")},
	} {
		dir := t.TempDir()
		writeTheme(t, dir, "default", c.body)
		th, err := Load(dir)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: err = %v, want %q", c.body, err, c.want)
		}
		if th == nil || th.SGR(StatusError) != Builtin().SGR(StatusError) {
			t.Errorf("%q: a broken theme should fall back to the built-in", c.body)
		}
	}
}

func TestActive(t *testing.T) {
	defer SetActive(Builtin())
	if Paint(StatusError, "x") != "\x1b[31mx"+Reset {
		t.Errorf("the active theme starts as the built-in")
	}
	th := mustBuild(t, "[ui]\n\"status.error\" = { fg = \"green\" }\n")
	SetActive(th)
	if SGR(StatusError) != "\x1b[32m" {
		t.Errorf("SetActive didn't take: %q", SGR(StatusError))
	}
}
