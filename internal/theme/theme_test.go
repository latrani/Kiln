package theme

import (
	"strings"
	"testing"

	"github.com/latrani/Kiln/internal/str"
)

func mustBuild(t *testing.T, srcs ...string) *Theme {
	t.Helper()
	var chain []file
	for i, s := range srcs {
		f, err := parse("t"+string(rune('0'+i))+".toml", []byte(s))
		if err != nil {
			t.Fatal(err)
		}
		chain = append(chain, f)
	}
	th, err := build(chain)
	if err != nil {
		t.Fatal(err)
	}
	return th
}

func TestSGR(t *testing.T) {
	th := mustBuild(t, `
[palette]
ember = "#ff9f43"
[ui]
"status.error" = { fg = "red", bold = true }
"input.over_limit" = { fg = "bright-white", bg = "red" }
"link" = { underline = true }
"link.hover" = { fg = "bright-blue" }
"sidebar.attention" = { fg = "ember", bg = "#010203", faint = true, italic = true, reverse = true }
`)
	for _, c := range []struct {
		role Role
		want string
	}{
		{StatusError, "\x1b[1;31m"},
		{InputOverLimit, "\x1b[97;41m"},
		{LinkHover, "\x1b[4;94m"}, // inherits link's underline
		{SidebarAttention, "\x1b[2;3;7;38;2;255;159;67;48;2;1;2;3m"},
		{Sidebar, ""},
	} {
		if got := th.SGR(c.role); got != c.want {
			t.Errorf("SGR(%s) = %q, want %q", c.role, got, c.want)
		}
	}
	if got := th.Paint(Sidebar, "x"); got != "x" {
		t.Errorf("an unstyled role should leave text alone, got %q", got)
	}
	if got := th.Paint(StatusError, "x"); got != "\x1b[1;31mx"+Reset {
		t.Errorf("Paint = %q", got)
	}
}

func TestCascadeAndOverride(t *testing.T) {
	th := mustBuild(t,
		`[ui]
"sidebar" = { fg = "#111111", bg = "#222222" }
"sidebar.active" = { bold = true }`,
		`[ui]
"sidebar.active" = { bold = false, reverse = true }`)
	if got := th.SGR(SidebarActive); got != "\x1b[7;38;2;17;17;17;48;2;34;34;34m" {
		t.Errorf("SGR = %q: want the parent's colors, bold turned back off, reverse on", got)
	}
}

func TestNestedTablesAndDottedKeys(t *testing.T) {
	a := mustBuild(t, "[ui.sidebar]\nfg = \"red\"\n[ui.sidebar.active]\nbold = true\n")
	b := mustBuild(t, "[ui]\n\"sidebar\" = { fg = \"red\" }\n\"sidebar.active\" = { bold = true }\n")
	if a.SGR(SidebarActive) != b.SGR(SidebarActive) || a.SGR(SidebarActive) == "" {
		t.Errorf("nested %q, quoted %q", a.SGR(SidebarActive), b.SGR(SidebarActive))
	}
}

func TestCSS(t *testing.T) {
	th := mustBuild(t, "[ui]\nexport = { fg = \"#d8d8d8\", bg = \"black\" }\n")
	if fg, bg := th.CSS(Export); fg != "#d8d8d8" || bg != "#000000" {
		t.Errorf("CSS = %q, %q", fg, bg)
	}
}

func TestBuildErrors(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"[ui]\n\"sidebar.activ\" = { bold = true }\n", str.ThemeUnknownRole("t0.toml", "sidebar.activ")},
		{"[ui]\nsidebar = { colour = \"red\" }\n", str.ThemeUnknownField("t0.toml", "sidebar", "colour")},
		{"[ui]\nsidebar = { fg = \"#12345\" }\n", str.ThemeBadColor("t0.toml", "sidebar", "#12345")},
		{"[ui]\nsidebar = { fg = \"nope\" }\n", str.ThemeBadColor("t0.toml", "sidebar", "nope")},
		{"[ui]\nsidebar = { bold = \"yes\" }\n", str.ThemeBadField("t0.toml", "sidebar", "bold", str.ThemeWantBool())},
		{"[palette]\nred = \"#ff0000\"\n", str.ThemePaletteNameTaken("t0.toml", "red")},
		{"[palette]\nember = \"orange\"\n", str.ThemeBadColor("t0.toml", str.ThemePaletteEntry("ember"), "orange")},
		{"colors = 1\n", str.ThemeUnknownKey("t0.toml", "colors")},
	} {
		f, err := parse("t0.toml", []byte(c.src))
		if err == nil {
			_, err = build([]file{f})
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: err = %v, want %q", c.src, err, c.want)
		}
	}
}
