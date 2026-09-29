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

// TestBuiltinLook pins a few of the built-in theme's roles; the golden
// screens in internal/ui pin the rest.
func TestBuiltinLook(t *testing.T) {
	b := Builtin()
	for _, c := range []struct {
		role Role
		want string
	}{
		{ScrollbackEcho, "\x1b[2m"},
		{Sidebar, "\x1b[38;2;160;227;225m"},
		{SidebarActive, "\x1b[7;38;2;160;227;225m"},
		{SidebarAdd, "\x1b[1;38;2;192;237;235;48;2;11;61;59m"},
		{RuleForm, "\x1b[38;2;205;181;162m"},
		{FormButton, "\x1b[1;38;2;237;211;192;48;2;61;32;11m"},
		{FormButtonSecondary, "\x1b[38;2;237;211;192;48;2;61;32;11m"},
		{PickerAdd, "\x1b[38;2;237;211;192;48;2;61;32;11m"},
		{StatusError, "\x1b[31m"},
		{LinkHover, "\x1b[4;94m"},
		{InputOverLimit, "\x1b[38;2;217;38;38;48;2;42;9;9m"},
		{SidebarAttention, "\x1b[1;38;2;255;209;102m"},
		{Scrollback, ""},
	} {
		if got := b.SGR(c.role); got != c.want {
			t.Errorf("builtin %s = %q, want %q", c.role, got, c.want)
		}
	}
	for _, c := range []struct {
		tag, want string
		match     bool
	}{
		{"page/in", "\x1b[1;38;2;255;159;67m", false},
		{"whisper/in", "\x1b[3;38;2;195;155;211m", false},
		{"self", "\x1b[1m", false},
		{"highlight", "\x1b[1;38;2;255;209;102m", true},
	} {
		if _, ts, _ := b.Tag(c.tag); ts.Style.SGR() != c.want || ts.Match != c.match {
			t.Errorf("builtin tag %s = %q, match %v; want %q, match %v", c.tag, ts.Style.SGR(), ts.Match, c.want, c.match)
		}
	}
}

func TestLoadWithoutAFileIsBuiltin(t *testing.T) {
	th, err := Load(t.TempDir(), "default", Dark)
	if err != nil || th.SGR(StatusError) != Builtin().SGR(StatusError) {
		t.Errorf("Load = %v, %v", th, err)
	}
}

func TestExtendsDefaultIsBuiltin(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "default", "extends = \"default\"\n[ui]\n\"status.error\" = { fg = \"#ff0000\" }\n")
	th, err := Load(dir, "default", Dark)
	if err != nil {
		t.Fatal(err)
	}
	if th.SGR(StatusError) != "\x1b[38;2;255;0;0m" || th.SGR(SidebarActive) != Builtin().SGR(SidebarActive) {
		t.Errorf("want the override on top of the built-in: %q, %q", th.SGR(StatusError), th.SGR(SidebarActive))
	}
}

func TestExtendsAnotherTheme(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "ember", "extends = \"default\"\n[palette]\nember = \"#ff9f43\"\n")
	writeTheme(t, dir, "default", "extends = \"ember\"\n[ui]\n\"status.error\" = { fg = \"ember\" }\n")
	th, err := Load(dir, "default", Dark)
	if err != nil || th.SGR(StatusError) != "\x1b[38;2;255;159;67m" {
		t.Errorf("Load = %q, %v", th.SGR(StatusError), err)
	}
}

func TestExtendsCycle(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "default", "extends = \"a\"\n") // default → a → b → a
	writeTheme(t, dir, "a", "extends = \"b\"\n")
	writeTheme(t, dir, "b", "extends = \"a\"\n")
	_, err := Load(dir, "default", Dark)
	if err == nil || !strings.Contains(err.Error(), str.ThemeExtendsLoop("themes/b.toml", "a")) {
		t.Errorf("err = %v", err)
	}
}

func TestLoadErrors(t *testing.T) {
	for _, c := range []struct{ body, want string }{
		{"extends = \"nope\"\n", str.ThemeNoTheme("nope")},
		{"[ui\n", "themes/default.toml"},
		{"[ui]\n\"sidebar.nope\" = { bold = true }\n", str.ThemeUnknownRole("themes/default.toml", "sidebar.nope")},
	} {
		dir := t.TempDir()
		writeTheme(t, dir, "default", c.body)
		th, err := Load(dir, "default", Dark)
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

func TestLoadByName(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "ember", "extends = \"default\"\n[ui]\n\"status.error\" = { fg = \"#ff0000\" }\n")
	th, err := Load(dir, "ember", Dark)
	if err != nil || th.SGR(StatusError) != "\x1b[38;2;255;0;0m" {
		t.Errorf("Load(ember) = %q, %v", th.SGR(StatusError), err)
	}
	if _, err := Load(dir, "nope", Dark); err == nil || err.Error() != str.ThemeNoTheme("nope") {
		t.Errorf("missing theme: err = %v", err)
	}
	if th, err := Load(dir, "../x", Light); err == nil || err.Error() != str.ThemeBadName("../x") || th != BuiltinFor(Light) {
		t.Errorf("bad name: %v, %v", th, err)
	}
}

func TestBuiltinForLight(t *testing.T) {
	if BuiltinFor(Dark) != Builtin() {
		t.Error("Builtin is the dark built-in")
	}
	if BuiltinFor(Light) == Builtin() {
		t.Error("the light built-in should be its own theme")
	}
}
func TestBuiltinLightLook(t *testing.T) {
	b := BuiltinFor(Light)
	for _, c := range []struct {
		role Role
		want string
	}{
		{Sidebar, "\x1b[38;2;29;114;111m"},
		{SidebarAdd, "\x1b[1;38;2;18;84;82;48;2;188;230;228m"},
		{RuleInput, "\x1b[38;2;180;162;116m"},
		{InputOverLimit, "\x1b[38;2;165;29;29;48;2;249;220;220m"},
		{SidebarAttention, "\x1b[1;38;2;164;116;4m"},
	} {
		if got := b.SGR(c.role); got != c.want {
			t.Errorf("light %s = %q, want %q", c.role, got, c.want)
		}
	}
	if _, ts, _ := b.Tag("page/in"); ts.Style.SGR() != "\x1b[1;38;2;194;90;10m" {
		t.Errorf("light page/in = %q", ts.Style.SGR())
	}
	if Builtin().SGR(InputOverLimit) != "\x1b[38;2;217;38;38;48;2;42;9;9m" {
		t.Error("the dark over-limit changed")
	}
}
