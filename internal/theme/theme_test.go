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

func TestParseErrors(t *testing.T) {
	for _, c := range []struct{ body, want string }{
		{"palette = \"x\"\n", str.ThemeNotTable("t.toml", "palette")},
		{"ui = 5\n", str.ThemeNotTable("t.toml", "ui")},
		{"[ui]\nsidebar = \"red\"\n", str.ThemeRoleNotTable("t.toml", "sidebar")},
		{"[ui.sidebar]\nactive = \"red\"\n", str.ThemeRoleNotTable("t.toml", "sidebar.active")},
		{"extends = \"../x\"\n", str.ThemeBadExtends("t.toml", "../x")},
		{"extends = \"x.toml\"\n", str.ThemeBadExtends("t.toml", "x.toml")},
		{"extends = \"\"\n", str.ThemeBadExtends("t.toml", "")},
	} {
		_, err := parse("t.toml", []byte(c.body))
		if err == nil || err.Error() != c.want {
			t.Errorf("%q: err = %v, want %q", c.body, err, c.want)
		}
	}
}

func TestDottedAndNestedRoleMerge(t *testing.T) {
	for range 20 { // map order varies; the two spellings must merge whichever comes first
		th := mustBuild(t, "[ui]\n\"sidebar.active\" = { bold = true }\n[ui.sidebar.active]\nfg = \"red\"\n")
		if got := th.SGR(SidebarActive); got != "\x1b[1;31m" {
			t.Fatalf("SGR(sidebar.active) = %q, want both settings", got)
		}
	}
}

func TestEqual(t *testing.T) {
	a := mustBuild(t, "[ui]\n\"status.error\" = { fg = \"red\" }\n")
	b := mustBuild(t, "[palette]\nx = \"#000000\"\n[ui]\n\"status.error\" = { fg = \"red\" }\n")
	c := mustBuild(t, "[ui]\n\"status.error\" = { fg = \"green\" }\n")
	if !a.Equal(b) || a.Equal(c) {
		t.Errorf("Equal: same looks %v, different looks %v", a.Equal(b), a.Equal(c))
	}
}

func TestDefaultColorClearsInherited(t *testing.T) {
	th := mustBuild(t, "[ui]\nsidebar = { fg = \"red\", bg = \"#010203\" }\n\"sidebar.add\" = { bg = \"default\" }\n")
	if got := th.SGR(SidebarAdd); got != "\x1b[31m" {
		t.Errorf("SGR(sidebar.add) = %q, want the fg without the inherited bg", got)
	}
	if fg, bg := th.CSS(SidebarAdd); fg == "" || bg != "" {
		t.Errorf("CSS(sidebar.add) = %q, %q", fg, bg)
	}
	f, _ := parse("t.toml", []byte("[palette]\ndefault = \"#000000\"\n"))
	if _, err := build([]file{f}); err == nil || err.Error() != str.ThemePaletteNameTaken("t.toml", "default") {
		t.Errorf("a palette name default: err = %v", err)
	}
}
func TestTags(t *testing.T) {
	th := mustBuild(t, `
[palette]
ember = "#ff9f43"
[tags]
page = { fg = "ember", bold = true }
"page/in" = { italic = true }
highlight = { fg = "#ffd166", scope = "match" }
`)
	for _, c := range []struct {
		tag, styled, sgr string
		match            bool
	}{
		{"page", "page", "\x1b[1;38;2;255;159;67m", false},
		{"page/in", "page/in", "\x1b[3m", false}, // a tag's own style; parents don't cascade into it
		{"page/out", "page", "\x1b[1;38;2;255;159;67m", false},
		{"page/out/x", "page", "\x1b[1;38;2;255;159;67m", false},
		{"highlight", "highlight", "\x1b[38;2;255;209;102m", true},
	} {
		styled, ts, ok := th.Tag(c.tag)
		if !ok || styled != c.styled || ts.Style.SGR() != c.sgr || ts.Match != c.match {
			t.Errorf("Tag(%q) = %q, %q, match %v, %v; want %q, %q, match %v", c.tag, styled, ts.Style.SGR(), ts.Match, ok, c.styled, c.sgr, c.match)
		}
	}
	for _, tag := range []string{"pages", "whisper", "self"} {
		if _, _, ok := th.Tag(tag); ok {
			t.Errorf("Tag(%q) found a style; want none", tag)
		}
	}
}

func TestTagsMergeAlongExtends(t *testing.T) {
	th := mustBuild(t, "[tags]\npage = { bold = true, scope = \"match\" }\n", "[tags]\npage = { fg = \"red\" }\n")
	_, ts, _ := th.Tag("page")
	if ts.Style.SGR() != "\x1b[1;31m" || !ts.Match {
		t.Errorf("page = %q, match %v; want bold red, match kept", ts.Style.SGR(), ts.Match)
	}
}

func TestTagErrors(t *testing.T) {
	for _, c := range []struct{ body, want string }{
		{"tags = 1\n", str.ThemeNotTable("t.toml", "tags")},
		{"[tags]\npage = \"red\"\n", str.ThemeTagNotTable("t.toml", "page")},
		{"[tags]\npage = { scope = \"word\" }\n", str.ThemeBadScope("t.toml", "page")},
		{"[tags]\npage = { size = 3 }\n", str.ThemeUnknownField("t.toml", str.ThemeTagEntry("page"), "size")},
		{"[tags]\npage = { bold = \"yes\" }\n", str.ThemeBadField("t.toml", str.ThemeTagEntry("page"), "bold", str.ThemeWantBool())},
	} {
		_, err := parse("t.toml", []byte(c.body))
		if err == nil || err.Error() != c.want {
			t.Errorf("%q: err = %v, want %q", c.body, err, c.want)
		}
	}
	f, _ := parse("t.toml", []byte("[tags]\npage = { fg = \"nope\" }\n"))
	if _, err := build([]file{f}); err == nil || err.Error() != str.ThemeBadColor("t.toml", str.ThemeTagEntry("page"), "nope") {
		t.Errorf("unknown color: err = %v", err)
	}
}

func TestStyleOver(t *testing.T) {
	th := mustBuild(t, "[tags]\na = { fg = \"red\", bold = true }\nb = { fg = \"blue\", italic = true }\n")
	_, a, _ := th.Tag("a")
	_, b, _ := th.Tag("b")
	if got := a.Style.Over(b.Style).SGR(); got != "\x1b[1;3;34m" {
		t.Errorf("a.Over(b) = %q, want b's color and both attributes", got)
	}
	if got := (Style{}).Over(a.Style).SGR(); got != a.Style.SGR() {
		t.Errorf("zero.Over(a) = %q", got)
	}
}

func TestEqualSeesTags(t *testing.T) {
	a := mustBuild(t, "[tags]\npage = { bold = true }\n")
	b := mustBuild(t, "[tags]\npage = { bold = true, scope = \"match\" }\n")
	if a.Equal(b) || !a.Equal(mustBuild(t, "[tags]\npage = { bold = true }\n")) {
		t.Error("Equal must compare tag styles and scopes")
	}
}
