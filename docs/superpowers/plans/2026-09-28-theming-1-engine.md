# Theming Phase 1: Theme Engine and Chrome Roles Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every color and attribute Kiln draws comes from a theme file of named roles, with the built-in default theme reproducing today's look exactly, painted areas ready for backgrounds, a dimmed backdrop behind modals, and live reload (#86).

**Architecture:** A new `internal/theme` package parses theme TOML (palette, `[ui]` roles, `extends`), cascades roles down their dots, and resolves each known role to an SGR string once. The active theme sits behind an atomic pointer, since some rendering runs off the UI goroutine. `internal/ui` replaces every hard-coded escape (`style.Dim`, `reverse`, `bold`, `red`, link and over-limit codes) with `theme.Paint(role, text)`, and wraps each painted area's rows in `theme.Fill`, which pads rows and re-applies the area's colors after resets. Golden cell-grid tests captured before any change pin today's look.

**Tech Stack:** Go 1.27, Bubble Tea v2 (`charm.land/bubbletea/v2`), BurntSushi TOML, `github.com/charmbracelet/x/ansi` for widths, Kiln's `internal/str` catalog for user-facing text.

**Spec:** `docs/superpowers/specs/2026-09-28-theming-design.md` (sections: The rule, Theme files, Roles, Areas, The backdrop, Phases → 1)

## Global Constraints

- **The rule:** anything the server wrote sits on the terminal's background. The scrollback and log mode's body are never given an area background. Server ANSI colors are never remapped.
- **Colors** are `"#rrggbb"`, a palette name, or one of the 16 terminal names: `black`, `red`, `green`, `yellow`, `blue`, `magenta`, `cyan`, `white`, and `bright-` plus each of those. Palette names can't be a terminal name.
- **Style fields** are exactly `fg`, `bg`, `bold`, `faint`, `italic`, `underline`, `reverse`.
- **Roles cascade:** `a.b.c` inherits every field of `a.b`, which inherits from `a`, and overrides the fields it sets. Theme files merge field by field along `extends`.
- **Role names are API.** Use exactly the names in Task 2's `roles.go`.
- **The built-in default theme reproduces today's look.** The golden screens from Task 1 must stay identical through Tasks 2–9 and 11–13. Only Task 10 (the backdrop) updates them, and only the picker and editor screens.
- **No config setting picks a theme in this phase.** Kiln loads `themes/default.toml` from the config dir if it exists, else the built-in. `extends = "default"` inside `themes/default.toml` means the built-in.
- **User-facing text goes through `internal/str`** (`locales/en.toml`, then `go generate ./internal/str`). Tests build expected text from `str` functions, never copies. Commits that add strings carry a `Strings:` trailer.
- **Color downsampling needs no code.** Bubble Tea v2's renderer converts truecolor to the terminal's profile (`cursed_renderer.setColorProfile`).
- Fixtures use Kit, Rook, Ash and world `fm`. Smoke tests use scratch `XDG_CONFIG_HOME`/`XDG_DATA_HOME`, never the real setup.
- No emoji. Glyphs are one cell wide.
- Commits end with the session's `Co-Authored-By` and `Claude-Session` lines.

## Review Focus

- **A broken theme file** (a typo'd role, a bad color, a TOML syntax error) must never crash Kiln or blank the screen. At startup Kiln uses the built-in theme and says why in the statusline; on reload it keeps the previous theme and says why. Pinned in Task 3 (`TestLoadErrors`) and Task 11 (`TestThemeReloadErrorKeepsTheme`).
- **`extends` loops** (a → b → a) must be an error, not a hang, and `themes/default.toml` saying `extends = "default"` must mean the built-in, not itself. Pinned in Task 3 (`TestExtendsDefaultIsBuiltin`, `TestExtendsCycle`).
- **Resets inside a painted area** (a styled segment's `ESC[0m` in the middle of a sidebar or input row) must not leave terminal-colored holes, and the padding must be area-colored. Pinned in Task 4 (`TestFillReassertsAfterReset`) and Task 6 (`TestInputAreaHasNoHoles`).
- **Narrow panes** (width 0, or content wider than the pane) must not panic or overflow in `Fill`. Pinned in Task 4 (`TestFillNarrow`).
- **Editing the theme while scrolled up** must keep the scroll position and not crash on a live selection. Pinned in Task 11 (`TestThemeReloadKeepsScroll`).

---

### Task 1: Golden screens of today's look

Capture what the screen looks like now, cell by cell with each cell's style, so the later tasks can prove they changed nothing visible. Different SGR bytes that produce the same cells (like `ESC[7mESC[1m` and `ESC[1;7m`) compare equal.

**Files:**
- Create: `internal/ui/golden_test.go`
- Create: `internal/ui/testdata/golden/*.txt` (generated)

**Interfaces:**
- Produces: `func assertGolden(t *testing.T, name, content string)`, and `go test ./internal/ui -run TestGolden -update` rewrites the files. Later tasks run `go test ./internal/ui -run TestGolden` and must see PASS.

- [ ] **Step 1: Write the cell interpreter and golden helper**

```go
package ui

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/ansi"
)

var update = flag.Bool("update", false, "rewrite golden screens in testdata/golden")

// sgrState is what a terminal would draw a cell with.
type sgrState struct {
	bold, faint, italic, underline, reverse bool
	fg, bg                                  string
}

func (s sgrState) key() string {
	var parts []string
	for _, f := range []struct {
		on   bool
		name string
	}{{s.bold, "bold"}, {s.faint, "faint"}, {s.italic, "italic"}, {s.underline, "underline"}, {s.reverse, "reverse"}} {
		if f.on {
			parts = append(parts, f.name)
		}
	}
	if s.fg != "" {
		parts = append(parts, "fg="+s.fg)
	}
	if s.bg != "" {
		parts = append(parts, "bg="+s.bg)
	}
	return strings.Join(parts, ",")
}

// apply folds one SGR parameter list into s.
func (s *sgrState) apply(params string) {
	ps := strings.Split(params, ";")
	for i := 0; i < len(ps); i++ {
		n, _ := strconv.Atoi(ps[i])
		switch {
		case ps[i] == "" || n == 0:
			*s = sgrState{}
		case n == 1:
			s.bold = true
		case n == 2:
			s.faint = true
		case n == 3:
			s.italic = true
		case n == 4:
			s.underline = true
		case n == 7:
			s.reverse = true
		case n == 22:
			s.bold, s.faint = false, false
		case n == 23:
			s.italic = false
		case n == 24:
			s.underline = false
		case n == 27:
			s.reverse = false
		case n >= 30 && n <= 37:
			s.fg = fmt.Sprintf("ansi%d", n-30)
		case n >= 90 && n <= 97:
			s.fg = fmt.Sprintf("ansi%d", n-90+8)
		case n == 39:
			s.fg = ""
		case n >= 40 && n <= 47:
			s.bg = fmt.Sprintf("ansi%d", n-40)
		case n >= 100 && n <= 107:
			s.bg = fmt.Sprintf("ansi%d", n-100+8)
		case n == 49:
			s.bg = ""
		case (n == 38 || n == 48) && i+1 < len(ps):
			var c string
			if ps[i+1] == "2" && i+4 < len(ps) {
				r, _ := strconv.Atoi(ps[i+2])
				g, _ := strconv.Atoi(ps[i+3])
				b, _ := strconv.Atoi(ps[i+4])
				c, i = fmt.Sprintf("#%02x%02x%02x", r, g, b), i+4
			} else if ps[i+1] == "5" && i+2 < len(ps) {
				c, i = "idx"+ps[i+2], i+2
			}
			if n == 38 {
				s.fg = c
			} else {
				s.bg = c
			}
		}
	}
}

// cells describes content as rows of text, each followed by its styled
// runs ("  @3-8 bold,reverse"). Two contents that draw the same are equal.
func cells(content string) string {
	var out strings.Builder
	var st sgrState
	for _, row := range strings.Split(content, "\n") {
		var text strings.Builder
		var runs []string
		col, runStart, runKey := 0, 0, ""
		flush := func() {
			if runKey != "" && col > runStart {
				runs = append(runs, fmt.Sprintf("@%d-%d %s", runStart, col-1, runKey))
			}
		}
		for i := 0; i < len(row); {
			if row[i] == 0x1b {
				j := ansi.EscapeEnd(row, i)
				seq := row[i:j]
				if len(seq) >= 3 && seq[1] == '[' && seq[len(seq)-1] == 'm' {
					st.apply(seq[2 : len(seq)-1])
				}
				i = j
				continue
			}
			r, size := []rune(row[i:])[0], len(string([]rune(row[i:])[0]))
			if k := st.key(); k != runKey {
				flush()
				runStart, runKey = col, k
			}
			text.WriteRune(r)
			col++
			i += size
		}
		flush()
		out.WriteString(text.String() + "\n")
		if len(runs) > 0 {
			out.WriteString("  " + strings.Join(runs, "; ") + "\n")
		}
	}
	return out.String()
}

// assertGolden compares content, as cells, with testdata/golden/<name>.txt.
func assertGolden(t *testing.T, name, content string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".txt")
	got := cells(content)
	if *update {
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("screen %s drew differently.\ngot:\n%s\nwant:\n%s", name, got, want)
	}
}

func (h *harness) raw() string { return h.m.View().Content }

func TestGoldenCellsSeeThroughBytes(t *testing.T) {
	a := cells("\x1b[7m\x1b[1mhi\x1b[0m")
	b := cells("\x1b[1;7mhi\x1b[m")
	if a != b {
		t.Errorf("same cells, different result:\n%s\n%s", a, b)
	}
}
```

- [ ] **Step 2: Write the golden screens**

Append to `internal/ui/golden_test.go`. Each scene is deterministic: the harness clock is fixed and lines are fed one at a time. The `withEcho` world turns on local echo so sent lines show.

```go
// goldenWorld has local echo on, so sent lines show in the scrollback.
var goldenWorld = strings.Replace(fmWorld, "max_line_bytes = 20", "max_line_bytes = 20\nlocal_echo = true", 1)

func goldenHarness(t *testing.T) *harness {
	h := newHarness(t, map[string]string{"fm": goldenWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.advance(time.Minute)
	return h
}

func TestGoldenMain(t *testing.T) {
	h := goldenHarness(t)
	h.open("fm/rook")
	h.line("Rook says, \"Evening, Kit.\"")
	h.line("See https://kiln.test/map for the way")
	h.typeText(":waves.")
	h.enter()
	h.m.chars["fm/rook"].unread, h.m.chars["fm/rook"].attention = 3, true
	assertGolden(t, "main", h.raw())
}

func TestGoldenPicker(t *testing.T) {
	h := goldenHarness(t)
	h.press('o', tea.ModCtrl)
	assertGolden(t, "picker", h.raw())
}

func TestGoldenEditor(t *testing.T) {
	h := goldenHarness(t)
	h.typeText("/edit world")
	h.enter()
	assertGolden(t, "editor", h.raw())
	h.focusOn(portLabel)
	h.typeText("x") // rejected: shows the field's error
	assertGolden(t, "editor-error", h.raw())
}

func TestGoldenLog(t *testing.T) {
	h := goldenHarness(t)
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.keys("m", "up", "m") // a range
	h.key("1")             // a chip on
	h.key("/")
	h.typeText("Rook")
	h.key("enter") // find
	assertGolden(t, "log", h.raw())
}

func TestGoldenInput(t *testing.T) {
	h := goldenHarness(t)
	h.typeText("this line runs past twenty bytes")
	in := h.m.input()
	in.StartSelect(gutterWidth, 0) // select the first four cells, as a mouse drag would
	in.DragTo(gutterWidth+4, 0)
	h.m.setStatus(true, str.StatusNotSent(errors.New("x")))
	assertGolden(t, "input", h.raw())
}

func TestGoldenScrolled(t *testing.T) {
	h := goldenHarness(t)
	for i := 0; i < 40; i++ {
		h.advance(2 * pageGap)
		h.line(fmt.Sprintf("line %d https://kiln.test/%d", i, i))
	}
	h.press(tea.KeyPgUp, 0)
	l := h.m.layout()
	h.m.Update(tea.MouseMotionMsg{X: l.sw + 1 + len("line 30 h"), Y: l.sbH - 1})
	assertGolden(t, "scrolled", h.raw())
}

func TestGoldenPassword(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": goldenWorld})
	delete(h.pw, "fm/kit")
	h.init()
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].needPW })
	h.typeText("s3cret")
	assertGolden(t, "password", h.raw())
}
```

Add `"errors"`, `"time"` and `"github.com/latrani/Kiln/internal/str"` to the imports.

- [ ] **Step 3: Generate the goldens and read them**

Run: `go test ./internal/ui -run 'TestGolden' -update && go test ./internal/ui -run 'TestGolden' -v`
Expected: PASS. Then open each file in `internal/ui/testdata/golden/` and check that it shows what its name says: runs with `faint`, `reverse`, `bold`, `fg=ansi1` (the error red), `fg=ansi12` (hover blue), and `fg=ansi15,bg=ansi1` (over limit). A scene that shows none of its styles is set up wrong; fix the scene, not the golden.

- [ ] **Step 4: Run the whole suite**

Run: `go test ./... && go vet ./...`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/golden_test.go internal/ui/testdata
git commit -m "test(ui): golden cell grids of today's screens"
```

---

### Task 2: The theme package

**Files:**
- Create: `internal/theme/roles.go`
- Create: `internal/theme/theme.go`
- Create: `internal/theme/theme_test.go`
- Modify: `internal/str/locales/en.toml` (a `[theme]` section), then `go generate ./internal/str`

**Interfaces:**
- Produces (used by every later task):
  - `type Role string`, plus the consts in `roles.go` and `var Roles []Role` (every role, parents first)
  - `const Reset = "\x1b[0m"`
  - `type Theme struct{…}` with `func (t *Theme) SGR(r Role) string`, `func (t *Theme) Paint(r Role, text string) string` (returns text unchanged when r draws nothing), and `func (t *Theme) CSS(r Role) (fg, bg string)` (hex strings, "" for unset)
  - `func parse(name string, data []byte) (file, error)` and `func build(chain []file) (*Theme, error)` (package-internal, for Task 3)

- [ ] **Step 1: Write roles.go**

```go
package theme

// Role names a part of the screen that a theme styles. Role names are
// API: theme files refer to them. A role inherits every style field of
// its parent (the name up to its last dot) and overrides what it sets.
type Role string

// The roles. Their names are theme vocabulary, not text for people.
//
//str:ok
const (
	Sidebar             Role = "sidebar" // area
	SidebarWorld        Role = "sidebar.world"
	SidebarChar         Role = "sidebar.char"
	SidebarActive       Role = "sidebar.active"
	SidebarUnread       Role = "sidebar.unread"
	SidebarAttention    Role = "sidebar.attention"
	SidebarConnecting   Role = "sidebar.connecting"
	SidebarDisconnected Role = "sidebar.disconnected"
	SidebarAdd          Role = "sidebar.add"
	SidebarMore         Role = "sidebar.more"

	Picker              Role = "picker"
	PickerWorld         Role = "picker.world"
	PickerWorldSelected Role = "picker.world.selected"
	PickerSelected      Role = "picker.selected"
	PickerAdd           Role = "picker.add"

	Divider Role = "divider"
	Rule    Role = "rule"

	Scrollback           Role = "scrollback" // never an area: server text keeps the terminal background
	ScrollbackDay        Role = "scrollback.day"
	ScrollbackHistoryEnd Role = "scrollback.history_end"
	ScrollbackLoading    Role = "scrollback.loading"
	ScrollbackEcho       Role = "scrollback.echo"
	ScrollbackSys        Role = "scrollback.sys"
	ScrollbackPill       Role = "scrollback.pill"
	ScrollbackSelection  Role = "scrollback.selection"
	ScrollbackInactive   Role = "scrollback.inactive"
	Link                 Role = "link"
	LinkHover            Role = "link.hover"

	Input          Role = "input" // area
	InputHint      Role = "input.hint"
	InputOverLimit Role = "input.over_limit"
	InputSelection Role = "input.selection"

	Status      Role = "status" // area
	StatusLog   Role = "status.log"
	StatusError Role = "status.error"
	StatusClock Role = "status.clock"

	Form      Role = "form"
	FormLabel Role = "form.label"
	FormHint  Role = "form.hint"
	FormError Role = "form.error"
	FormFocus Role = "form.focus"
	FormTitle Role = "form.title"

	Log           Role = "log"
	LogHeader     Role = "log.header" // area
	LogTitle      Role = "log.header.title"
	LogChip       Role = "log.header.chip"
	LogChipOn     Role = "log.header.chip.on"
	LogTime       Role = "log.time"
	LogCursor     Role = "log.cursor"
	LogSelected   Role = "log.selected"
	LogExcluded   Role = "log.excluded"
	LogFind       Role = "log.find"
	LogDay        Role = "log.day"
	LogLoading    Role = "log.loading"
	LogBar        Role = "log.bar" // area
	LogHints      Role = "log.bar.hints"
	LogError      Role = "log.bar.error"

	Export Role = "export" // the HTML export's page colors
)

// Roles is every role, each after its parent.
var Roles = []Role{
	Sidebar, SidebarWorld, SidebarChar, SidebarActive, SidebarUnread, SidebarAttention,
	SidebarConnecting, SidebarDisconnected, SidebarAdd, SidebarMore,
	Picker, PickerWorld, PickerWorldSelected, PickerSelected, PickerAdd,
	Divider, Rule,
	Scrollback, ScrollbackDay, ScrollbackHistoryEnd, ScrollbackLoading, ScrollbackEcho, ScrollbackSys,
	ScrollbackPill, ScrollbackSelection, ScrollbackInactive, Link, LinkHover,
	Input, InputHint, InputOverLimit, InputSelection,
	Status, StatusLog, StatusError, StatusClock,
	Form, FormLabel, FormHint, FormError, FormFocus, FormTitle,
	Log, LogHeader, LogTitle, LogChip, LogChipOn, LogTime, LogCursor, LogSelected, LogExcluded,
	LogFind, LogDay, LogLoading, LogBar, LogHints, LogError,
	Export,
}

// parent is r's parent role, or "" for a top-level one.
func (r Role) parent() Role {
	for i := len(r) - 1; i >= 0; i-- {
		if r[i] == '.' {
			return r[:i]
		}
	}
	return ""
}
```

- [ ] **Step 2: Add the error strings**

Append to `internal/str/locales/en.toml`, then run `go generate ./internal/str`:

```toml

[theme]
parse = "themes/{file}: {err}"
unknown_key = "themes/{file}: unknown key {key:%q} (a theme has extends, palette and ui)"
unknown_role = "themes/{file}: unknown role {role:%q}"
unknown_field = "themes/{file}: {role}: unknown setting {field:%q}"
bad_field = "themes/{file}: {role}: {field} must be {want}"
bad_color = "themes/{file}: {where}: {color:%q} isn't a color (use #rrggbb, a palette name, or a terminal color like bright-blue)"
palette_name_taken = "themes/{file}: palette name {name:%q} is a terminal color"
no_theme = "no theme {name:%q} (no themes/{name}.toml)"
extends_loop = "themes/{file}: extends loops back to {name:%q}"
palette_entry = "palette {name}"
want_color = "a color"
want_bool = "true or false"
want_theme = "a theme name"
```

- [ ] **Step 3: Write the failing tests**

`internal/theme/theme_test.go`:

```go
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
```

- [ ] **Step 4: Run to see it fail**

Run: `go test ./internal/theme`
Expected: FAIL, since `parse`, `build` and `Theme` are undefined.

- [ ] **Step 5: Write theme.go**

```go
// Package theme is Kiln's look: a palette of named colors, and the styles
// of the screen's parts (roles), from theme files in TOML. See the spec,
// docs/superpowers/specs/2026-09-28-theming-design.md.
package theme

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/latrani/Kiln/internal/str"
)

// Reset clears all SGR attributes.
const Reset = "\x1b[0m"

// terminalColors are the 16 colors a terminal's own theme sets, by the
// names a theme uses for them, in ANSI order.
//
//str:ok
var terminalColors = []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
	"bright-black", "bright-red", "bright-green", "bright-yellow", "bright-blue", "bright-magenta", "bright-cyan", "bright-white"}

// xterm's defaults for the 16, for the HTML export, which can't ask the
// terminal.
var terminalHex = []string{"#000000", "#cd0000", "#00cd00", "#cdcd00", "#0000ee", "#cd00cd", "#00cdcd", "#e5e5e5",
	"#7f7f7f", "#ff0000", "#00ff00", "#ffff00", "#5c5cff", "#ff00ff", "#00ffff", "#ffffff"}

var hexRE = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`) //str:ok

// color is a resolved color: a terminal color (ansi 0–15) or a hex one.
type color struct {
	ansi int // -1 for hex
	hex  string
}

func (c color) code(bg bool) string {
	if c.ansi >= 0 {
		base := 30
		if c.ansi >= 8 {
			base = 90 - 8
		}
		if bg {
			base += 10
		}
		return fmt.Sprint(base + c.ansi)
	}
	var r, g, b int
	fmt.Sscanf(c.hex[1:], "%02x%02x%02x", &r, &g, &b)
	lead := "38"
	if bg {
		lead = "48"
	}
	return fmt.Sprintf("%s;2;%d;%d;%d", lead, r, g, b)
}

func (c color) css() string {
	if c.ansi >= 0 {
		return terminalHex[c.ansi]
	}
	return strings.ToLower(c.hex)
}

// fileStyle is a role's style as a file writes it: unset fields are nil,
// so a role can inherit them, and a later file can turn a flag back off.
type fileStyle struct {
	fg, bg                                  *string
	bold, faint, italic, underline, reverse *bool
}

// file is one parsed theme file.
type file struct {
	name    string
	extends string
	palette map[string]string
	ui      map[Role]fileStyle
}

// style is a role's resolved style.
type style struct {
	fg, bg                                  *color
	bold, faint, italic, underline, reverse bool
}

func (s style) sgr() string {
	var codes []string
	for _, f := range []struct {
		on   bool
		code string
	}{{s.bold, "1"}, {s.faint, "2"}, {s.italic, "3"}, {s.underline, "4"}, {s.reverse, "7"}} {
		if f.on {
			codes = append(codes, f.code)
		}
	}
	if s.fg != nil {
		codes = append(codes, s.fg.code(false))
	}
	if s.bg != nil {
		codes = append(codes, s.bg.code(true))
	}
	if len(codes) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

// Theme is a loaded theme: every role's style, resolved.
type Theme struct {
	styles map[Role]style
	sgr    map[Role]string
}

// SGR is the escape sequence that starts drawing in r, or "" when r draws
// plainly.
func (t *Theme) SGR(r Role) string { return t.sgr[r] }

// Paint draws text in r and resets after it, or returns text unchanged
// when r draws plainly.
func (t *Theme) Paint(r Role, text string) string {
	if s := t.sgr[r]; s != "" {
		return s + text + Reset
	}
	return text
}

// CSS is r's colors as CSS hex colors, "" where r sets none.
func (t *Theme) CSS(r Role) (fg, bg string) {
	s := t.styles[r]
	if s.fg != nil {
		fg = s.fg.css()
	}
	if s.bg != nil {
		bg = s.bg.css()
	}
	return fg, bg
}

var styleFields = []string{"fg", "bg", "bold", "faint", "italic", "underline", "reverse"}

// parse reads one theme file. name is its file name, for messages.
func parse(name string, data []byte) (file, error) {
	f := file{name: name, palette: map[string]string{}, ui: map[Role]fileStyle{}}
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return f, errors.New(str.ThemeParse(name, err))
	}
	for k, v := range raw {
		switch k {
		case "extends":
			s, ok := v.(string)
			if !ok {
				return f, errors.New(str.ThemeBadField(name, "extends", "extends", str.ThemeWantTheme()))
			}
			f.extends = s
		case "palette":
			tbl, _ := v.(map[string]any)
			for pk, pv := range tbl {
				s, ok := pv.(string)
				if !ok {
					return f, errors.New(str.ThemeBadColor(name, str.ThemePaletteEntry(pk), fmt.Sprint(pv)))
				}
				f.palette[pk] = s
			}
		case "ui":
			tbl, _ := v.(map[string]any)
			if err := flatten(name, "", tbl, f.ui); err != nil {
				return f, err
			}
		default:
			return f, errors.New(str.ThemeUnknownKey(name, k))
		}
	}
	return f, nil
}

// flatten reads [ui], written with quoted dotted keys ("sidebar.active"),
// nested tables ([ui.sidebar.active]), or both, into styles by role.
func flatten(name, prefix string, tbl map[string]any, out map[Role]fileStyle) error {
	var own fileStyle
	hasOwn := false
	for k, v := range tbl {
		role := k
		if prefix != "" {
			role = prefix + "." + k
		}
		if sub, ok := v.(map[string]any); ok {
			if err := flatten(name, role, sub, out); err != nil {
				return err
			}
			continue
		}
		if prefix == "" || !slices.Contains(styleFields, k) {
			return errors.New(str.ThemeUnknownField(name, prefix, k))
		}
		hasOwn = true
		switch k {
		case "fg", "bg":
			s, ok := v.(string)
			if !ok {
				return errors.New(str.ThemeBadField(name, prefix, k, str.ThemeWantColor()))
			}
			if k == "fg" {
				own.fg = &s
			} else {
				own.bg = &s
			}
		default:
			b, ok := v.(bool)
			if !ok {
				return errors.New(str.ThemeBadField(name, prefix, k, str.ThemeWantBool()))
			}
			switch k {
			case "bold":
				own.bold = &b
			case "faint":
				own.faint = &b
			case "italic":
				own.italic = &b
			case "underline":
				own.underline = &b
			case "reverse":
				own.reverse = &b
			}
		}
	}
	if hasOwn {
		out[Role(prefix)] = own
	}
	return nil
}

// build resolves a chain of files, base first: later files override
// earlier ones field by field, then roles inherit down their dots.
func build(chain []file) (*Theme, error) {
	palette := map[string]color{}
	merged := map[Role]fileStyle{}
	origin := map[Role]string{} // which file set a role last, for messages
	for _, f := range chain {
		for _, n := range slices.Sorted(maps.Keys(f.palette)) {
			if slices.Contains(terminalColors, n) {
				return nil, errors.New(str.ThemePaletteNameTaken(f.name, n))
			}
			c, ok := literal(f.palette[n])
			if !ok {
				return nil, errors.New(str.ThemeBadColor(f.name, str.ThemePaletteEntry(n), f.palette[n]))
			}
			palette[n] = c
		}
		for r, s := range f.ui {
			if !slices.Contains(Roles, r) {
				return nil, errors.New(str.ThemeUnknownRole(f.name, string(r)))
			}
			merged[r] = overlay(merged[r], s)
			origin[r] = f.name
		}
	}
	t := &Theme{styles: map[Role]style{}, sgr: map[Role]string{}}
	for _, r := range Roles {
		s := t.styles[r.parent()]
		fs := merged[r]
		for _, c := range []struct {
			src *string
			dst **color
		}{{fs.fg, &s.fg}, {fs.bg, &s.bg}} {
			if c.src == nil {
				continue
			}
			col, ok := resolve(*c.src, palette)
			if !ok {
				return nil, errors.New(str.ThemeBadColor(origin[r], string(r), *c.src))
			}
			*c.dst = &col
		}
		for _, c := range []struct {
			src *bool
			dst *bool
		}{{fs.bold, &s.bold}, {fs.faint, &s.faint}, {fs.italic, &s.italic}, {fs.underline, &s.underline}, {fs.reverse, &s.reverse}} {
			if c.src != nil {
				*c.dst = *c.src
			}
		}
		t.styles[r], t.sgr[r] = s, s.sgr()
	}
	return t, nil
}

// overlay is a with b's set fields on top.
func overlay(a, b fileStyle) fileStyle {
	for _, p := range []struct{ dst, src **string }{{&a.fg, &b.fg}, {&a.bg, &b.bg}} {
		if *p.src != nil {
			*p.dst = *p.src
		}
	}
	for _, p := range []struct{ dst, src **bool }{{&a.bold, &b.bold}, {&a.faint, &b.faint},
		{&a.italic, &b.italic}, {&a.underline, &b.underline}, {&a.reverse, &b.reverse}} {
		if *p.src != nil {
			*p.dst = *p.src
		}
	}
	return a
}

// literal reads a color written out: #rrggbb or a terminal color's name.
func literal(s string) (color, bool) {
	if hexRE.MatchString(s) {
		return color{ansi: -1, hex: s}, true
	}
	if i := slices.Index(terminalColors, s); i >= 0 {
		return color{ansi: i}, true
	}
	return color{}, false
}

// resolve reads a role's color: a literal, or a palette name.
func resolve(s string, palette map[string]color) (color, bool) {
	if c, ok := literal(s); ok {
		return c, true
	}
	c, ok := palette[s]
	return c, ok
}
```

- [ ] **Step 6: Run the tests**

Run: `go test ./internal/theme ./internal/str && go vet ./internal/theme`
Expected: PASS. `TestNoStrayStrings` passes: the role names, terminal color names and hex regexp are marked `//str:ok`, and every message goes through `str`.

- [ ] **Step 7: Commit**

```bash
git add internal/theme internal/str
git commit -m "feat(theme): theme files, roles, cascade and SGR

Strings: theme.parse, theme.unknown_key, theme.unknown_role, theme.unknown_field, theme.bad_field, theme.bad_color, theme.palette_name_taken, theme.no_theme, theme.extends_loop, theme.palette_entry, theme.want_color, theme.want_bool, theme.want_theme (new)"
```

---

### Task 3: The default theme, loading and the active theme

**Files:**
- Create: `internal/theme/default.toml`
- Create: `internal/theme/load.go`
- Create: `internal/theme/load_test.go`
- Modify: `internal/config/paths.go` (`EnsureDefaults` makes `themes/`)
- Modify: `internal/config/watch.go` (watch `themes/`)
- Modify: `internal/config/watch_test.go`

**Interfaces:**
- Consumes: `parse`, `build`, `Theme`, the roles (Task 2).
- Produces:
  - `func Builtin() *Theme`: the embedded default, always valid.
  - `func Load(dir string) (*Theme, error)`: `dir/themes/default.toml` if present (following `extends`), else the built-in. On error it returns `Builtin()` **and** the error.
  - `func Active() *Theme`, `func SetActive(t *Theme)`, and package-level `func Paint(r Role, text string) string` and `func SGR(r Role) string`, which use `Active()`. Safe to call from any goroutine.

- [ ] **Step 1: Write default.toml**

These are exactly today's codes: `style.Dim` is faint, `reverse` is reverse, `bold` is bold, the error red is terminal red, link hover is underline plus bright blue, over-limit is bright white on red, and the attention dot is `config.HighlightStyle`.

```toml
# Kiln's built-in theme. To change the look, make themes/default.toml in
# Kiln's config folder with extends = "default", and set only the roles
# you want different. Colors are "#rrggbb", a [palette] name, or a
# terminal color (red, bright-blue, ...). Styles take fg, bg, bold, faint,
# italic, underline and reverse. A role inherits from the role before its
# last dot: link.hover starts from link.

[ui]
"sidebar.world"     = { bold = true }
"sidebar.active"    = { reverse = true }
"sidebar.attention" = { fg = "#ffd166", bold = true }
"sidebar.add"       = { faint = true }
"sidebar.more"      = { faint = true }

"picker.world"          = { bold = true }
"picker.world.selected" = { reverse = true }
"picker.selected"       = { reverse = true }
"picker.add"            = { faint = true }

"divider" = { faint = true }
"rule"    = { faint = true }

"scrollback.day"         = { faint = true }
"scrollback.history_end" = { faint = true }
"scrollback.loading"     = { faint = true }
"scrollback.echo"        = { faint = true }
"scrollback.sys"         = { faint = true }
"scrollback.pill"        = { reverse = true }
"scrollback.selection"   = { reverse = true }
"scrollback.inactive"    = { faint = true }
"link"                   = { underline = true }
"link.hover"             = { fg = "bright-blue" }

"input.hint"       = { faint = true }
"input.over_limit" = { fg = "bright-white", bg = "red" }
"input.selection"  = { reverse = true }

"status.log"   = { bold = true }
"status.error" = { fg = "red" }

"form.label" = { faint = true }
"form.hint"  = { faint = true }
"form.error" = { fg = "red" }
"form.focus" = { reverse = true }
"form.title" = { bold = true }

"log.header.title"   = { bold = true }
"log.header.chip.on" = { reverse = true }
"log.time"           = { faint = true }
"log.cursor"         = { reverse = true }
"log.find"           = { reverse = true }
"log.day"            = { faint = true }
"log.loading"        = { faint = true }
"log.bar.hints"      = { faint = true }
"log.bar.error"      = { fg = "red" }

"export" = { fg = "#d8d8d8", bg = "#1b1b1f" }
```

- [ ] **Step 2: Write the failing tests**

`internal/theme/load_test.go`:

```go
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
```

- [ ] **Step 3: Run to see it fail**

Run: `go test ./internal/theme`
Expected: FAIL, since `Builtin`, `Load`, `SetActive` and friends are undefined.

- [ ] **Step 4: Write load.go**

```go
package theme

import (
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"

	"github.com/latrani/Kiln/internal/str"
)

//go:embed default.toml
var builtinSrc []byte

var builtin = func() *Theme {
	f, err := parse("default.toml", builtinSrc)
	if err != nil {
		panic(err)
	}
	t, err := build([]file{f})
	if err != nil {
		panic(err)
	}
	return t
}()

// Builtin is the theme Kiln ships with.
func Builtin() *Theme { return builtin }

// Load reads dir/themes/default.toml, following extends, or returns the
// built-in theme when there's no such file. On an error it returns the
// built-in theme too, so there's always something to draw with.
func Load(dir string) (*Theme, error) {
	chain, err := chainFor(dir, "default", nil)
	if err != nil {
		return builtin, err
	}
	t, err := build(chain)
	if err != nil {
		return builtin, err
	}
	return t, nil
}

// chainFor is the files theme name is built from, base first. seen is the
// user files already on the chain: a file extending its own name means the
// built-in (only default has one).
func chainFor(dir, name string, seen []string) ([]file, error) {
	builtinFile := func() ([]file, error) {
		if name != "default" {
			return nil, errors.New(str.ThemeNoTheme(name))
		}
		f, _ := parse("default.toml", builtinSrc)
		return []file{f}, nil
	}
	if slices.Contains(seen, name) {
		if name == seen[len(seen)-1] {
			return builtinFile()
		}
		return nil, errors.New(str.ThemeExtendsLoop(seen[len(seen)-1]+".toml", name))
	}
	data, err := os.ReadFile(filepath.Join(dir, "themes", name+".toml"))
	if errors.Is(err, os.ErrNotExist) {
		return builtinFile()
	}
	if err != nil {
		return nil, err
	}
	f, err := parse(name+".toml", data)
	if err != nil {
		return nil, err
	}
	if f.extends == "" {
		return []file{f}, nil
	}
	base, err := chainFor(dir, f.extends, append(seen, name))
	if err != nil {
		return nil, err
	}
	return append(base, f), nil
}

var active atomic.Pointer[Theme]

func init() { active.Store(builtin) }

// Active is the theme to draw with.
func Active() *Theme { return active.Load() }

// SetActive makes t the theme to draw with.
func SetActive(t *Theme) { active.Store(t) }

// Paint is Active().Paint.
func Paint(r Role, text string) string { return Active().Paint(r, text) }

// SGR is Active().SGR.
func SGR(r Role) string { return Active().SGR(r) }
```

Note the cycle rule in `chainFor`: a name already on the chain is the built-in only when it is the file that just asked for it (`default.toml` saying `extends = "default"`). Any other repeat is a loop.

- [ ] **Step 5: Make and watch the themes folder**

In `internal/config/paths.go`, change the loop in `EnsureDefaults`:

```go
	for _, sub := range []string{"worlds", "packs", "themes"} {
```

In `internal/config/watch.go`, update `Watch`'s comment and list:

```go
// Watch starts watching dir, dir/worlds, dir/packs and dir/themes. Call
// EnsureDefaults first so they all exist.
func Watch(dir string) (*Watcher, error) {
	...
	for _, d := range []string{dir, filepath.Join(dir, "worlds"), filepath.Join(dir, "packs"), filepath.Join(dir, "themes")} {
```

Add to `internal/config/watch_test.go`:

```go
func TestWatchSeesThemes(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	w, err := Watch(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	os.WriteFile(filepath.Join(dir, "themes", "default.toml"), []byte("extends = \"default\"\n"), 0o644)
	select {
	case <-w.Changes():
	case <-time.After(2 * time.Second):
		t.Fatal("no change reported for a theme file")
	}
}
```

- [ ] **Step 6: Run the tests**

Run: `go test ./internal/theme ./internal/config ./internal/str && go vet ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/theme internal/config
git commit -m "feat(theme): built-in default theme, loading, and the active theme"
```

---

### Task 4: Painted areas

**Files:**
- Modify: `internal/style/style.go` (add `Reassert`)
- Modify: `internal/style/style_test.go`
- Create: `internal/theme/fill.go`
- Create: `internal/theme/fill_test.go`

**Interfaces:**
- Consumes: `Theme.SGR` (Task 2).
- Produces:
  - `func style.Reassert(s, base string) string`: `s` with `base` written after every SGR sequence that contains a reset.
  - `func (t *Theme) Fill(r Role, row string, w int) string`, and package-level `func Fill(r Role, row string, w int) string`: row cut or padded to exactly `w` cells. With `r` unstyled it's exactly the old `fit`: `xansi.Truncate(row, w, "")` plus spaces. With `r` styled it's `base + Reassert(row, base) + padding + Reset`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/style/style_test.go`:

```go
func TestReassert(t *testing.T) {
	base := "\x1b[48;2;1;2;3m"
	for _, c := range []struct{ in, want string }{
		{"a\x1b[0mb", "a\x1b[0m" + base + "b"},
		{"a\x1b[mb", "a\x1b[m" + base + "b"},
		{"a\x1b[1;31mb", "a\x1b[1;31mb"},              // no reset: left alone
		{"a\x1b[0;31mb", "a\x1b[0;31m" + base + "b"},  // a reset, then red
		{"a\x1b[38;5;0mb", "a\x1b[38;5;0mb"},          // black, not a reset
		{"plain", "plain"},
	} {
		if got := Reassert(c.in, base); got != c.want {
			t.Errorf("Reassert(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

`internal/theme/fill_test.go`:

```go
package theme

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestFillUnstyledIsPlainFit(t *testing.T) {
	b := Builtin()
	if got := b.Fill(Sidebar, "abc", 6); got != "abc   " {
		t.Errorf("Fill = %q", got)
	}
	if got := b.Fill(Sidebar, "abcdef", 3); got != "abc" {
		t.Errorf("Fill = %q", got)
	}
}

func TestFillReassertsAfterReset(t *testing.T) {
	th := mustBuild(t, "[ui]\nsidebar = { bg = \"#010203\" }\n")
	base := th.SGR(Sidebar)
	row := "a" + th.Paint(StatusError, "b") + "c" // "b" ends in a reset
	got := th.Fill(Sidebar, row, 5)
	if !strings.HasPrefix(got, base) || !strings.Contains(got, Reset+base+"c") || !strings.HasSuffix(got, "  "+Reset) {
		t.Errorf("Fill = %q: want the area's background after every reset and under the padding", got)
	}
	if xansi.StringWidth(got) != 5 {
		t.Errorf("width = %d", xansi.StringWidth(got))
	}
}

func TestFillNarrow(t *testing.T) {
	th := mustBuild(t, "[ui]\nsidebar = { bg = \"#010203\" }\n")
	if got := th.Fill(Sidebar, "abc", 0); got != "" {
		t.Errorf("width 0 = %q", got)
	}
	if got := th.Fill(Sidebar, "abcdef", 2); xansi.StringWidth(got) != 2 {
		t.Errorf("cut = %q", got)
	}
	if got := th.Fill(Sidebar, "abc", -1); got != "" {
		t.Errorf("negative width = %q", got)
	}
}
```

- [ ] **Step 2: Run to see it fail**

Run: `go test ./internal/style ./internal/theme`
Expected: FAIL, since `Reassert` and `Fill` are undefined.

- [ ] **Step 3: Implement**

Add to `internal/style/style.go`:

```go
// Reassert writes base again after every SGR sequence in s that resets,
// so a painted area keeps its colors across the resets of what's drawn
// inside it.
func Reassert(s, base string) string {
	if base == "" || !strings.Contains(s, "\x1b[") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			i++
			continue
		}
		j := ansi.EscapeEnd(s, i)
		b.WriteString(s[i:j])
		if params, ok := sgrParams(s[i:j]); ok {
			if _, reset := afterReset(params); reset {
				b.WriteString(base)
			}
		}
		i = j
	}
	return b.String()
}
```

Write `internal/theme/fill.go`:

```go
package theme

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/style"
)

// Fill draws row as exactly w cells of area r: cut or padded, with r's
// colors under the padding and re-applied after every reset inside row,
// so the area has no holes. With r unstyled it's plain cut-and-pad.
func (t *Theme) Fill(r Role, row string, w int) string {
	if w <= 0 {
		return ""
	}
	row = xansi.Truncate(row, w, "")
	pad := strings.Repeat(" ", max(0, w-xansi.StringWidth(row)))
	base := t.SGR(r)
	if base == "" {
		return row + pad
	}
	return base + style.Reassert(row, base) + pad + Reset
}

// Fill is Active().Fill.
func Fill(r Role, row string, w int) string { return Active().Fill(r, row, w) }
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/style ./internal/theme && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/style internal/theme
git commit -m "feat(theme): painted areas: fill and re-assert after resets"
```

---

### Task 5: Sidebar and picker

**Files:**
- Modify: `internal/theme/load.go` (add `FromTOML`)
- Modify: `internal/ui/sidebar.go` (`sidebarLine`, `attentionMark`)
- Modify: `internal/ui/picker.go` (`pickerLine`)
- Modify: `internal/ui/view.go` (sidebar rows, the ▲/▼ hints and the divider in `View`)
- Test: `internal/ui/theme_test.go` (create)

**Interfaces:**
- Consumes: `theme.Paint`, `theme.Fill`, `theme.SetActive`, `theme.Builtin`, and the roles (Tasks 2–4).
- Produces: `func withTheme(t *testing.T, src string)` in `internal/ui/theme_test.go`, which builds a theme from TOML on top of the built-in, activates it, and restores the built-in after the test. Tasks 6–11 use it.

- [ ] **Step 1: Write the test helper and a failing test**

`withTheme` needs a way to build a theme from a string outside the package. Add to `internal/theme/load.go`:

```go
// FromTOML builds a theme from src on top of the built-in, for tests and
// tools.
func FromTOML(src string) (*Theme, error) {
	base, _ := parse("default.toml", builtinSrc)
	f, err := parse("test.toml", []byte(src))
	if err != nil {
		return nil, err
	}
	return build([]file{base, f})
}
```

`internal/ui/theme_test.go`:

```go
package ui

import (
	"strings"
	"testing"

	"github.com/latrani/Kiln/internal/theme"
)

// withTheme draws with src (on top of the built-in theme) for one test.
func withTheme(t *testing.T, src string) *theme.Theme {
	t.Helper()
	th, err := theme.FromTOML(src)
	if err != nil {
		t.Fatal(err)
	}
	theme.SetActive(th)
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	return th
}

func TestSidebarUsesTheme(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook")
	h.m.chars["fm/rook"].unread, h.m.chars["fm/rook"].attention = 2, true
	th := withTheme(t, `[ui]
sidebar = { bg = "#010203" }
"sidebar.world" = { fg = "#0a0b0c" }
"sidebar.active" = { fg = "#0d0e0f" }
"sidebar.attention" = { fg = "#101112" }
divider = { fg = "#131415" }`)
	s := h.raw()
	for _, role := range []theme.Role{theme.Sidebar, theme.SidebarWorld, theme.SidebarActive, theme.SidebarAttention, theme.Divider} {
		if !strings.Contains(s, th.SGR(role)) {
			t.Errorf("screen doesn't draw %s", role)
		}
	}
}

func TestPickerUsesTheme(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	th := withTheme(t, `[ui]
"picker.world" = { fg = "#0a0b0c" }
"picker.selected" = { fg = "#0d0e0f" }
"picker.add" = { fg = "#101112" }`)
	h.press('o', tea.ModCtrl)
	s := h.raw()
	for _, role := range []theme.Role{theme.PickerWorld, theme.PickerSelected, theme.PickerAdd} {
		if !strings.Contains(s, th.SGR(role)) {
			t.Errorf("picker doesn't draw %s", role)
		}
	}
}
```

Add `tea "charm.land/bubbletea/v2"` to the imports.

- [ ] **Step 2: Run to see it fail**

Run: `go test ./internal/ui -run 'SidebarUsesTheme|PickerUsesTheme'`
Expected: FAIL: the screen still draws hard-coded codes.

- [ ] **Step 3: Wire the sidebar**

In `internal/ui/sidebar.go`, replace `var attentionMark = …` with:

```go
// attentionMark is the "●" for a character with attention lines.
func attentionMark() string { return theme.Paint(theme.SidebarAttention, "●") }
```

And change `sidebarLine`:

```go
	switch r.kind {
	case rowWorld:
		return theme.Paint(theme.SidebarWorld, fitName(r.world, w))
	case rowAdd:
		return theme.Paint(theme.SidebarAdd, fitName(addLabel, w))
	}
	cs := m.chars[r.char]
	lead, leadRole := " ", theme.SidebarChar
	switch {
	case cs.state == session.Connecting:
		lead, leadRole = " … ", theme.SidebarConnecting
	case closable(cs):
		lead, leadRole = " × ", theme.SidebarDisconnected
	}
	activity, shown := "", ""
	if cs.unread > 0 {
		activity = fmt.Sprintf(" %d", cs.unread)
		shown = theme.Paint(theme.SidebarUnread, activity)
		if cs.attention {
			activity = " ●" + activity
			shown = " " + attentionMark() + shown
		}
	}
	name := fitName(lead+cs.ch.Name, w-xansi.StringWidth(activity))
	line := theme.Paint(leadRole, name) + shown
	if r.char == m.active {
		return theme.Paint(theme.SidebarActive, line)
	}
	return line
```

With the built-in theme, `sidebar.char`, `.connecting`, `.disconnected` and `.unread` draw nothing, so the cells match the goldens. `Paint(SidebarActive, line)` wraps segments that may end in a reset (the attention dot). That's the same as today's `reverse + line + Reset`, where the dot's reset ends reverse early, so the goldens hold.

- [ ] **Step 4: Wire the picker**

In `internal/ui/picker.go`, `pickerLine`:

```go
	case rowWorld:
		if selKey(r) == m.picker.sel {
			return theme.Paint(theme.PickerWorldSelected, fitName(r.world, w))
		}
		return theme.Paint(theme.PickerWorld, fitName(r.world, w))
	...
	switch {
	case selKey(r) == m.picker.sel:
		return theme.Paint(theme.PickerSelected, line)
	case r.kind != rowChar:
		return theme.Paint(theme.PickerAdd, line)
	}
```

`picker.world.selected` inherits `picker.world`'s bold and adds reverse, matching today's `reverse + bold`.

- [ ] **Step 5: Wire the sidebar's area, hints and divider in View**

In `internal/ui/view.go`, in the `for y := 0; y < m.height; y++` loop of `View`, draw every sidebar row through `theme.Fill(theme.Sidebar, …, l.sw)`, and the divider through `theme.Paint(theme.Divider, "│")`:

```go
		switch r, hint := sv.at(y); {
		case hint < 0:
			b.WriteString(theme.Fill(theme.Sidebar, theme.Paint(theme.SidebarMore, fit(str.ViewMoreAbove(sv.top), l.sw)), l.sw))
		case hint > 0:
			b.WriteString(theme.Fill(theme.Sidebar, theme.Paint(theme.SidebarMore, fit(str.ViewMoreBelow(len(sv.rows)-sv.top-sv.avail), l.sw)), l.sw))
		case r != nil && m.picker != nil:
			b.WriteString(theme.Fill(theme.Sidebar, m.pickerLine(*r, l.sw), l.sw))
		case r != nil:
			b.WriteString(theme.Fill(theme.Sidebar, m.sidebarLine(*r, l.sw), l.sw))
		default:
			b.WriteString(theme.Fill(theme.Sidebar, "", l.sw))
		}
		b.WriteString(theme.Paint(theme.Divider, "│"))
```

Add `"github.com/latrani/Kiln/internal/theme"` to each file's imports.

- [ ] **Step 6: Run the tests, goldens included**

Run: `go test ./internal/ui && go vet ./...`
Expected: PASS, including `TestGolden*` with **no** `-update`. A golden diff means a visible change: fix the code, not the golden.

- [ ] **Step 7: Commit**

```bash
git add internal/theme internal/ui
git commit -m "feat(ui): sidebar and picker draw from the theme"
```

---

### Task 6: The input area and forms

**Files:**
- Modify: `internal/ui/view.go` (`prompt`'s hints and password prompt; the input rows in `View`)
- Modify: `internal/ui/input.go` (`overLimit`, selection)
- Modify: `internal/ui/form.go` (`rows`, `fieldRow`)
- Test: `internal/ui/theme_test.go`

**Interfaces:**
- Consumes: `withTheme` (Task 5), `theme.Paint`, `theme.SGR`, `theme.Fill`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/ui/theme_test.go`:

```go
func TestInputAreaUsesTheme(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	th := withTheme(t, `[ui]
input = { bg = "#010203" }
"input.over_limit" = { fg = "#0a0b0c" }
"input.hint" = { fg = "#0d0e0f" }`)
	s := h.raw() // disconnected: the input area shows a hint
	if !strings.Contains(s, th.SGR(theme.InputHint)) {
		t.Error("hint not drawn in input.hint")
	}
	h.typeText("this line runs past twenty bytes")
	if !strings.Contains(h.raw(), th.SGR(theme.InputOverLimit)) {
		t.Error("over-limit text not drawn in input.over_limit")
	}
}

// TestInputAreaHasNoHoles: every input row is painted edge to edge, and
// a styled stretch inside one doesn't leave the terminal's background
// after it.
func TestInputAreaHasNoHoles(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	th := withTheme(t, "[ui]\ninput = { bg = \"#010203\" }\n")
	h.typeText("this line runs past twenty bytes")
	base := th.SGR(theme.Input)
	l := h.m.layout()
	row := strings.Split(h.raw(), "\n")[l.sbH+1]
	_, right, _ := strings.Cut(row, "│")
	right = strings.TrimPrefix(right, theme.Reset) // the divider's own reset
	if !strings.HasPrefix(right, base) {
		t.Errorf("input row doesn't start painted: %q", right)
	}
	for _, part := range strings.Split(right, theme.Reset)[1:] {
		if part != "" && !strings.HasPrefix(part, base) {
			t.Errorf("a reset inside the input row isn't followed by the area's colors: %q", right)
			break
		}
	}
}

func TestFormUsesTheme(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	th := withTheme(t, `[ui]
"form.label" = { fg = "#0a0b0c" }
"form.focus" = { fg = "#0d0e0f" }
"form.title" = { fg = "#101112" }
"form.error" = { fg = "#131415" }`)
	h.typeText("/edit world")
	h.enter()
	s := h.raw()
	for _, role := range []theme.Role{theme.FormLabel, theme.FormFocus, theme.FormTitle} {
		if !strings.Contains(s, th.SGR(role)) {
			t.Errorf("editor doesn't draw %s", role)
		}
	}
	h.focusOn(portLabel)
	h.typeText("x")
	if !strings.Contains(h.raw(), th.SGR(theme.FormError)) {
		t.Error("field error not drawn in form.error")
	}
}
```

- [ ] **Step 2: Run to see it fail**

Run: `go test ./internal/ui -run 'InputArea|FormUsesTheme'`
Expected: FAIL.

- [ ] **Step 3: Wire the input's own drawing**

In `internal/ui/input.go`, delete `const overLimit = …`. In `Render`, write the over-limit and selection styles from the theme:

```go
		if red {
			b.WriteString(theme.SGR(theme.InputOverLimit))
		}
		for _, c := range vr.cells {
			bytes += len(c.text)
			if !red && limit > 0 && bytes > limit {
				red = true
				b.WriteString(theme.SGR(theme.InputOverLimit))
			}
			switch {
			case masked:
				b.WriteString("•")
			case in.selected(vr.line, c.col):
				// The selection's style, then back to what the row was in.
				b.WriteString(theme.SGR(theme.InputSelection) + c.text + theme.Reset)
				if red {
					b.WriteString(theme.SGR(theme.InputOverLimit))
				}
			default:
				b.WriteString(c.text)
			}
		}
		if red {
			b.WriteString(theme.Reset)
		}
```

The selection used to end with `ESC[27m` (reverse off). It now ends with a full reset and puts the over-limit style back, which draws the same cells.

- [ ] **Step 4: Wire the prompts in view.go**

In `prompt`:

```go
	hint := func(s string) ([]string, int, int, bool) { return []string{theme.Paint(theme.InputHint, s)}, 0, 0, true }
	...
		text := theme.Paint(theme.InputHint, label) + strings.TrimPrefix(rows[0], gutterMark) + theme.Paint(theme.InputHint, str.ViewPasswordKeys())
```

In `View`, paint the input rows as an area where they're added to `right`:

```go
		for _, r := range l.inRows {
			right = append(right, theme.Fill(theme.Input, r, l.rw))
		}
```

This replaces `right = append(right, l.inRows...)`.

- [ ] **Step 5: Wire forms**

In `internal/ui/form.go`, `rows`:

```go
	note := theme.Paint(theme.FormHint, f.hint)
	if f.reject != "" {
		note = theme.Paint(theme.FormError, f.reject)
	}
	if len(f.fields) == 1 {
		text, col := f.fieldRow(0, len(f.fields[0].label))
		return []string{text + theme.Paint(theme.FormHint, " "+str.Separator()) + note}, 0, col
	}
	if f.title != "" {
		rows = append(rows, theme.Paint(theme.FormTitle, f.title))
	}
	...
	if f.reject == "" {
		note = theme.Paint(theme.FormHint, str.FormNextField(f.hint))
	}
	rows[len(rows)-1] += theme.Paint(theme.FormHint, " "+str.Separator()) + note
```

`fieldRow`: every `reverse + X + style.Reset` becomes `theme.Paint(theme.FormFocus, X)`, and every `style.Dim(label)` / `style.Dim(fl.hint)` becomes `theme.Paint(theme.FormLabel, label)` / `theme.Paint(theme.FormHint, fl.hint)`.

- [ ] **Step 6: Run the tests, goldens included**

Run: `go test ./internal/ui && go vet ./...`
Expected: PASS, goldens unchanged (the `editor`, `editor-error`, `input` and `password` scenes cover this task).

- [ ] **Step 7: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): input area and forms draw from the theme"
```

---

### Task 7: Statusline and rules

**Files:**
- Modify: `internal/ui/view.go` (`statusLine`, the two rules in `View`)
- Test: `internal/ui/theme_test.go`

**Interfaces:**
- Consumes: `withTheme`, `theme.Paint`, `theme.Fill`.

- [ ] **Step 1: Write the failing test**

```go
func TestStatusAndRulesUseTheme(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	th := withTheme(t, `[ui]
status = { bg = "#010203" }
"status.error" = { fg = "#0a0b0c" }
"status.clock" = { fg = "#0d0e0f" }
rule = { fg = "#101112" }`)
	h.m.setStatus(true, "x")
	s := h.raw()
	for _, role := range []theme.Role{theme.Status, theme.StatusError, theme.StatusClock, theme.Rule} {
		if !strings.Contains(s, th.SGR(role)) {
			t.Errorf("screen doesn't draw %s", role)
		}
	}
}
```

- [ ] **Step 2: Run to see it fail**

Run: `go test ./internal/ui -run StatusAndRules`
Expected: FAIL.

- [ ] **Step 3: Implement**

In `statusLine`:

```go
		if cs.browse != nil {
			name = theme.Paint(theme.StatusLog, str.BrowseLog()) + sep + str.ViewSelected(len(cs.browse.selection())) + sep + name
		}
	...
	if m.status != "" {
		msg := m.status
		if m.statusErr {
			msg = theme.Paint(theme.StatusError, msg)
		}
	...
	rw := xansi.StringWidth(right)
	right = theme.Paint(theme.StatusClock, right)
	if w <= rw {
		return fit(right, w)
	}
	return fitName(strings.Join(parts, sep), w-rw-1) + " " + right
```

In `View`, paint the statusline as an area and draw the rule from its role, both where the right column is assembled (the normal view and log mode):

```go
		rule := theme.Paint(theme.Rule, strings.Repeat("─", l.rw))
		...
		right = append(right, rule, theme.Fill(theme.Status, m.statusLine(l.rw), l.rw))
```

and in the log-mode branch:

```go
		right = append(rows, theme.Fill(theme.Status, m.statusLine(l.rw), l.rw))
```

- [ ] **Step 4: Run the tests, goldens included**

Run: `go test ./internal/ui && go vet ./...`
Expected: PASS, goldens unchanged.

- [ ] **Step 5: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): statusline and rules draw from the theme"
```

---

### Task 8: The scrollback

**Files:**
- Modify: `internal/ui/scrollback.go` (`sbLine` gets a role; `loadingRow`; `Rerender` repaints chrome lines)
- Modify: `internal/ui/model.go` (`renderLine`, `renderDays`, `preload`, the echo appends)
- Modify: `internal/ui/sbmouse.go` (link and selection styles)
- Modify: `internal/ui/view.go` (the pill)
- Modify: `cmd/kiln/render.go`
- Modify: `internal/style/style.go` (delete `Dim`)
- Test: `internal/ui/theme_test.go`, `internal/ui/scrollback_test.go`

**Interfaces:**
- Consumes: `theme.Paint`, `theme.SGR`, `withTheme`.
- Produces:
  - `func chromeLine(r theme.Role, raw string) sbLine`: a line Kiln wrote (day divider, history marker, echo of a sent line), drawn in `r`, and repainted by `Rerender`.
  - `Scrollback.Rerender(render)` also repaints chrome lines from the active theme (Task 11 relies on this).

- [ ] **Step 1: Write the failing tests**

Append to `internal/ui/theme_test.go`:

```go
func TestScrollbackUsesTheme(t *testing.T) {
	th := withTheme(t, `[ui]
"scrollback.sys" = { fg = "#0a0b0c" }
"scrollback.echo" = { fg = "#0d0e0f" }
"scrollback.pill" = { fg = "#101112" }
link = { fg = "#131415" }`)
	h := newHarness(t, map[string]string{"fm": goldenWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.line("see https://kiln.test/map")
	h.typeText(":waves.")
	h.enter()
	s := h.raw()
	for _, role := range []theme.Role{theme.ScrollbackSys, theme.ScrollbackEcho, theme.Link} {
		if !strings.Contains(s, th.SGR(role)) {
			t.Errorf("scrollback doesn't draw %s", role)
		}
	}
	h.press(tea.KeyPgUp, 0)
	if !strings.Contains(h.raw(), th.SGR(theme.ScrollbackPill)) {
		t.Error("pill not drawn in scrollback.pill")
	}
}
```

Append to `internal/ui/scrollback_test.go`:

```go
func TestRerenderRepaintsChromeLines(t *testing.T) {
	s := sb(20)
	s.AppendLine(chromeLine(theme.ScrollbackDay, "── Thu Sep 24 ──"))
	th := withTheme(t, "[ui]\n\"scrollback.day\" = { fg = \"#0a0b0c\" }\n")
	s.Rerender(func(logstore.Entry) string { return "" })
	if !strings.HasPrefix(s.lines[0].text, th.SGR(theme.ScrollbackDay)) {
		t.Errorf("chrome line not repainted: %q", s.lines[0].text)
	}
}
```

Add `"github.com/latrani/Kiln/internal/logstore"` and `"github.com/latrani/Kiln/internal/theme"` to `scrollback_test.go`'s imports.

- [ ] **Step 2: Run to see it fail**

Run: `go test ./internal/ui -run 'ScrollbackUsesTheme|RerenderRepaintsChromeLines'`
Expected: FAIL (`chromeLine` undefined).

- [ ] **Step 3: Chrome lines**

In `internal/ui/scrollback.go`, add the role to `sbLine` and the constructor, turn `loadingRow` into a function, and teach `Rerender` about chrome lines:

```go
type sbLine struct {
	text  string
	entry *logstore.Entry // what text was rendered from; nil for dividers and such
	role  theme.Role      // for a line Kiln wrote: its style, and raw its text
	raw   string
	...
}

// chromeLine is a line Kiln wrote, drawn in r (and redrawn when the theme
// changes).
func chromeLine(r theme.Role, raw string) sbLine {
	return sbLine{text: theme.Paint(r, raw), role: r, raw: raw}
}

// loadingRow sits above the oldest line while more history may exist.
func loadingRow() string { return theme.Paint(theme.ScrollbackLoading, str.ScrollbackLoading()) }
```

In `View`, `tail = append(tail, loadingRow)` becomes `tail = append(tail, loadingRow())`.

`Rerender`:

```go
// Rerender redraws every line from the current rules and theme: lines
// rendered from a log entry with render, and Kiln's own lines in their
// roles. The view keeps its offset; a selection is dropped, since its
// byte positions may no longer fit.
func (s *Scrollback) Rerender(render func(logstore.Entry) string) {
	for i, l := range s.lines {
		switch {
		case l.entry != nil:
			s.lines[i] = sbLine{text: render(*l.entry), entry: l.entry}
		case l.role != "":
			s.lines[i] = chromeLine(l.role, l.raw)
		}
	}
	s.sel, s.hover = nil, nil
}
```

- [ ] **Step 4: Use them in model.go**

- `renderLine`: `style.Dim(gutterMark + text)` → `theme.Paint(theme.ScrollbackEcho, gutterMark+text)`, and `style.Dim("* " + text)` → `theme.Paint(theme.ScrollbackSys, "* "+text)`.
- `renderDays`: `sbLine{text: style.Dim("── " + dayLabel(day) + " ──")}` → `chromeLine(theme.ScrollbackDay, "── "+dayLabel(day)+" ──")`.
- `preload`: `cs.sb.Append(style.Dim(str.ScrollbackHistoryEnds(…)))` → `cs.sb.AppendLine(chromeLine(theme.ScrollbackHistoryEnd, str.ScrollbackHistoryEnds(…)))`.
- The two echo appends in `submit` (`cs.sb.Append(style.Dim(gutterMark + e.Text))` and the `ansi.Sanitize` one) → `cs.sb.AppendLine(chromeLine(theme.ScrollbackEcho, gutterMark+e.Text))`, keeping each one's `ansi.Sanitize` where it had one.

- [ ] **Step 5: Links, selection and pill**

In `internal/ui/sbmouse.go`, delete the `linkSGR`/`hoverSGR` consts. In `linkRow`:

```go
		sgr := theme.SGR(theme.Link)
		if hoverOK && lk == hovered {
			sgr = theme.SGR(theme.LinkHover)
		}
```

In `highlightRow`, `style.Reset + reverse + pr[a:z] + style.Reset` → `theme.Reset + theme.Paint(theme.ScrollbackSelection, pr[a:z]) + theme.Reset`. The trailing reset stays: it ends the server's styling before the rest of the row, as today.

In `internal/ui/view.go`, the pill: `style.Reset + reverse + pill + style.Reset` → `theme.Reset + theme.Paint(theme.ScrollbackPill, pill) + theme.Reset`.

- [ ] **Step 6: `kiln tail`, and retire style.Dim**

In `cmd/kiln/render.go`:

```go
	case logstore.Out:
		return theme.Paint(theme.ScrollbackEcho, "> "+e.Text)
	case logstore.Sys:
		return theme.Paint(theme.ScrollbackSys, "* "+e.Text)
```

Delete `Dim` from `internal/style/style.go`. `go build ./...` must pass: nothing may still call it.

- [ ] **Step 7: Run the tests, goldens included**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: PASS, goldens unchanged (the `main` and `scrolled` scenes cover this task).

- [ ] **Step 8: Commit**

```bash
git add internal cmd
git commit -m "feat(ui): scrollback draws from the theme; Kiln's own lines restyle"
```

---

### Task 9: Log mode

**Files:**
- Modify: `internal/ui/browse.go` (`view`, `highlightFind`)
- Test: `internal/ui/theme_test.go`

**Interfaces:**
- Consumes: `withTheme`, `theme.Paint`, `theme.Fill`.

- [ ] **Step 1: Write the failing test**

```go
func TestLogModeUsesTheme(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	th := withTheme(t, `[ui]
"log.header" = { bg = "#010203" }
"log.header.title" = { fg = "#0a0b0c" }
"log.header.chip.on" = { fg = "#0d0e0f" }
"log.time" = { fg = "#101112" }
"log.cursor" = { fg = "#131415" }
"log.find" = { fg = "#161718" }
"log.day" = { fg = "#191a1b" }
"log.bar" = { bg = "#1c1d1e" }
"log.bar.hints" = { fg = "#1f2021" }`)
	h.key("ctrl+l")
	s := h.raw()
	for _, role := range []theme.Role{theme.LogHeader, theme.LogTitle, theme.LogTime, theme.LogCursor, theme.LogDay, theme.LogBar, theme.LogHints} {
		if !strings.Contains(s, th.SGR(role)) {
			t.Errorf("log mode doesn't draw %s", role)
		}
	}
	h.key("1")
	h.key("/")
	h.typeText("Rook")
	h.key("enter")
	s = h.raw()
	for _, role := range []theme.Role{theme.LogChipOn, theme.LogFind} {
		if !strings.Contains(s, th.SGR(role)) {
			t.Errorf("log mode doesn't draw %s", role)
		}
	}
}
```

- [ ] **Step 2: Run to see it fail**

Run: `go test ./internal/ui -run LogModeUsesTheme`
Expected: FAIL.

- [ ] **Step 3: Implement**

In `browse.go`'s `view`:

- Header row 1: `head := theme.Paint(theme.LogTitle, str.BrowseLog()+" "+b.cs.ch.Name) + str.Separator() + span`.
- Chips: `chipRow[:len(chipRow)-len(chip)] + " " + reverse + chip[1:] + style.Reset` → `chipRow[:len(chipRow)-len(chip)] + " " + theme.Paint(theme.LogChipOn, chip[1:])`, and neutral chips `theme.Paint(theme.LogChip, …)` the same way. The chip spans are measured before styling, as now.
- `rows = []string{head, chipRow, style.Dim(strings.Repeat("─", w))}` → `rows = []string{theme.Fill(theme.LogHeader, head, w), theme.Fill(theme.LogHeader, chipRow, w), theme.Paint(theme.Rule, strings.Repeat("─", w))}`.
- Loading row → `theme.Paint(theme.LogLoading, str.BrowseLoading())`. Day divider → `theme.Paint(theme.LogDay, "── "+dayLabel(l.day)+" ──")`.
- Timestamp: `reverse + ts + style.Reset` → `theme.Paint(theme.LogCursor, ts)`, and `style.Dim(ts)` → `theme.Paint(theme.LogTime, ts)`.
- Gutter glyphs: `gutter = theme.Paint(theme.LogSelected, glyphSelected)` and `theme.Paint(theme.LogExcluded, glyphExcluded)`.
- Action bar: the rule → `theme.Paint(theme.Rule, …)`. The status error `red + msg + style.Reset` → `theme.Paint(theme.LogError, msg)`. The hints → `theme.Paint(theme.LogHints, str.BrowseHints())`. Wrap each action-bar row (prompt, status, hints) in `theme.Fill(theme.LogBar, row, w)`. **Keep `curX` as it is:** `Fill` adds no columns before the text, so the cursor math holds.

In `highlightFind`: `reverse + plain[m[0]:m[1]] + style.Reset` → `theme.Paint(theme.LogFind, plain[m[0]:m[1]])`.

The log body rows are **not** filled: server text keeps the terminal's background (the rule).

- [ ] **Step 4: Run the tests, goldens included**

Run: `go test ./internal/ui && go vet ./...`
Expected: PASS, goldens unchanged (the `log` scene covers this task).

- [ ] **Step 5: Retire the last hard-coded styles**

Delete the `reverse`, `red` and `bold` consts from `internal/ui/view.go`. `go build ./...` must pass. If anything still uses them, it was missed: move it to its role (the spec's Roles table says which).

Run: `go build ./... && go test ./internal/ui`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): log mode draws from the theme; no hard-coded styles left"
```

---

### Task 10: The backdrop

**Files:**
- Modify: `internal/ui/view.go` (`View`)
- Test: `internal/ui/theme_test.go`
- Update: `internal/ui/testdata/golden/picker.txt`, `editor.txt`, `editor-error.txt`

**Interfaces:**
- Consumes: `theme.Paint`, `ansi.Strip`.
- Produces: `func (m *Model) modal() bool`: true while the picker or an editor owns the input area.

- [ ] **Step 1: Write the failing test**

```go
func TestBackdropBehindModals(t *testing.T) {
	th := withTheme(t, "[ui]\n\"scrollback.inactive\" = { fg = \"#0a0b0c\" }\n")
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.line("Rook pages: see https://kiln.test/map")
	inactive := th.SGR(theme.ScrollbackInactive)
	if strings.Contains(h.raw(), inactive) {
		t.Fatal("backdrop drawn with no modal open")
	}
	h.press('o', tea.ModCtrl)
	s := h.raw()
	if !strings.Contains(s, inactive) || strings.Contains(s, th.SGR(theme.Link)) {
		t.Errorf("behind the picker, the scrollback should be one plain inactive color:\n%q", s)
	}
	h.press(tea.KeyEscape, 0)
	if strings.Contains(h.raw(), inactive) {
		t.Error("backdrop outlived the picker")
	}
}
```

- [ ] **Step 2: Run to see it fail**

Run: `go test ./internal/ui -run Backdrop`
Expected: FAIL.

- [ ] **Step 3: Implement**

In `internal/ui/view.go`:

```go
// modal reports whether the picker or an editor owns the input area; the
// scrollback is then a backdrop.
func (m *Model) modal() bool { return m.picker != nil }
```

In `View`, right after `rows := cs.sb.View(l.sbH)`:

```go
		if m.modal() {
			for i, r := range rows {
				rows[i] = theme.Paint(theme.ScrollbackInactive, ansi.Strip(r))
			}
		}
```

It goes before the pill is drawn, so a pill (scrolled up behind a modal) still shows in its own style. Import `"github.com/latrani/Kiln/internal/ansi"` if `view.go` doesn't already.

- [ ] **Step 4: Update the three modal goldens, and only those**

Run: `go test ./internal/ui -run 'TestGolden(Picker|Editor)' -update && go test ./internal/ui`
Expected: PASS. Then `git diff --stat internal/ui/testdata` must list only `picker.txt`, `editor.txt` and `editor-error.txt`, and their diffs must only add `faint` runs over the scrollback rows.

- [ ] **Step 5: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): dim the scrollback behind the picker and editors"
```

---

### Task 11: Live reload

**Files:**
- Modify: `internal/ui/model.go` (load the theme in `New` and `reloadNow`; restyle)
- Modify: `internal/ui/browse.go` (restyle open log mode)
- Modify: `cmd/kiln/main.go` (`kiln tail` loads the theme)
- Modify: `internal/str/locales/en.toml` (`status.theme_not_loaded`), then `go generate ./internal/str`
- Test: `internal/ui/theme_test.go`

**Interfaces:**
- Consumes: `theme.Load`, `theme.SetActive`, `theme.Active`, `Scrollback.Rerender` (Task 8).
- Produces: `func (m *Model) loadTheme()`, and `func (b *browse) restyle(render func(logstore.Entry) string)`.

- [ ] **Step 1: Add the string**

In `internal/str/locales/en.toml`, `[status]` section:

```toml
theme_not_loaded = "theme not loaded: {err}"
```

Run `go generate ./internal/str`.

- [ ] **Step 2: Write the failing tests**

```go
func writeUserTheme(t *testing.T, h *harness, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(h.dir, "themes", "default.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestThemeReloads(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": goldenWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.typeText(":waves.")
	h.enter()
	writeUserTheme(t, h, "extends = \"default\"\n[ui]\n\"scrollback.echo\" = { fg = \"#0a0b0c\" }\n")
	h.m.Update(reloadMsg{})
	want := theme.Active().SGR(theme.ScrollbackEcho)
	if want != "\x1b[2;38;2;10;11;12m" {
		t.Fatalf("theme not reloaded: %q", want)
	}
	if !strings.Contains(h.raw(), want) {
		t.Error("an echo line already on screen wasn't restyled")
	}
}

func TestThemeReloadErrorKeepsTheme(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	writeUserTheme(t, h, "extends = \"default\"\n[ui]\n\"status.error\" = { fg = \"#0a0b0c\" }\n")
	h.m.Update(reloadMsg{})
	good := theme.Active()
	writeUserTheme(t, h, "[ui]\n\"status.eror\" = { fg = \"red\" }\n")
	h.m.Update(reloadMsg{})
	if theme.Active() != good {
		t.Error("a broken theme replaced a working one")
	}
	if !strings.Contains(h.screen(), upTo(str.StatusThemeNotLoaded(mark))) {
		t.Errorf("no word about the broken theme:\n%s", h.screen())
	}
}

func TestBrokenThemeAtStartUsesBuiltin(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	dir := t.TempDir()
	config.EnsureDefaults(dir)
	os.WriteFile(filepath.Join(dir, "themes", "default.toml"), []byte("[ui\n"), 0o644)
	cfg, _ := config.Load(dir)
	m := New(Deps{ConfigDir: dir, Load: config.Load, Now: func() time.Time { return time.Date(2026, 9, 24, 21, 14, 0, 0, time.Local) }}, cfg)
	if theme.Active().SGR(theme.StatusError) != theme.Builtin().SGR(theme.StatusError) || !strings.Contains(m.status, upTo(str.StatusThemeNotLoaded(mark))) {
		t.Errorf("want the built-in theme and a status, got %q", m.status)
	}
}

func TestThemeReloadKeepsScroll(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	for i := 0; i < 40; i++ {
		h.advance(2 * pageGap)
		h.line(fmt.Sprintf("line %d", i))
	}
	h.press(tea.KeyPgUp, 0)
	sb := &h.m.chars["fm/kit"].sb
	offset := sb.offset
	sb.StartSelect(sbPos{line: 5})
	writeUserTheme(t, h, "extends = \"default\"\n[ui]\n\"scrollback.sys\" = { fg = \"#0a0b0c\" }\n")
	h.m.Update(reloadMsg{})
	if !sb.Scrolled() || sb.offset != offset {
		t.Errorf("reload moved the view: offset %d, was %d", sb.offset, offset)
	}
	h.screen() // must not panic with the selection dropped
}
```

Add `"fmt"`, `"os"`, `"path/filepath"`, `"time"`, `"github.com/latrani/Kiln/internal/config"` and `"github.com/latrani/Kiln/internal/str"` to the test file's imports.

- [ ] **Step 3: Run to see it fail**

Run: `go test ./internal/ui -run 'ThemeReload|BrokenThemeAtStart'`
Expected: FAIL.

- [ ] **Step 4: Implement**

In `internal/ui/model.go`:

```go
// loadTheme reads the theme from the config folder and draws with it,
// restyling what's on screen. A broken theme is reported and changes
// nothing, except at start, when the built-in theme stands in.
func (m *Model) loadTheme() {
	th, err := theme.Load(m.d.ConfigDir)
	if err != nil {
		m.setStatus(true, str.StatusThemeNotLoaded(err))
		if m.themed {
			return // keep the theme we have
		}
	}
	m.themed = true
	if th == theme.Active() {
		return
	}
	theme.SetActive(th)
	for _, cs := range m.chars {
		cs.sb.Rerender(func(e logstore.Entry) string { text, _ := cs.render(e); return text })
		if cs.browse != nil {
			cs.browse.restyle(func(e logstore.Entry) string { text, _ := cs.render(e); return text })
		}
	}
}
```

Add a `themed bool // a theme has been loaded; see loadTheme` field to `Model`. Call `m.loadTheme()` in `New` right after `m.applyConfig(cfg)`, and in `reloadNow` right after `m.applyConfig(cfg)`. `th == theme.Active()` is a pointer comparison: `Load` returns a new `*Theme` every time it reads a file, and the shared built-in otherwise, so an unchanged built-in skips the restyle.

In `internal/ui/browse.go`, the lines keep their entries (`bline.e`) and rendered text (`bline.text`). Add:

```go
// restyle redraws every line's text with render, after a theme change.
func (b *browse) restyle(render func(logstore.Entry) string) {
	for _, l := range b.lines {
		l.text = render(l.e)
	}
}
```

`bline.text` is the field `makeLine` sets from `renderLine`.

In `cmd/kiln/main.go`, `tail`: at the start, load the theme and fall back quietly:

```go
	if th, err := theme.Load(cfgDir); err != nil {
		fmt.Fprintln(os.Stderr, "*", str.StatusThemeNotLoaded(err))
	} else {
		theme.SetActive(th)
	}
```

`tail` needs `cfgDir`: pass it from `run` (change the signature to `tail(ch config.Character, cfgDir, dataDir string, cfg *config.Config)` and the call site).

- [ ] **Step 5: Run the tests, goldens included**

Run: `go test ./... && go vet ./...`
Expected: PASS, goldens unchanged.

- [ ] **Step 6: Smoke test with a scratch config**

```bash
export XDG_CONFIG_HOME=$(mktemp -d) XDG_DATA_HOME=$(mktemp -d)
go build -o /tmp/kiln-theme ./cmd/kiln && /tmp/kiln-theme
```

With Kiln running, write `$XDG_CONFIG_HOME/kiln/themes/default.toml` containing `extends = "default"` and `[ui]` with `sidebar = { bg = "#23242e" }` and `status = { bg = "#23242e" }`. The sidebar and statusline should turn that color within a second, edge to edge. Then write `[ui` (broken). The statusline should say the theme wasn't loaded, and the colors should stay. Quit with Ctrl+C twice.

- [ ] **Step 7: Commit**

```bash
git add internal cmd
git commit -m "feat(ui): themes reload live; a broken theme is reported and ignored

Strings: status.theme_not_loaded (new)"
```

---

### Task 12: The HTML export's colors

**Files:**
- Modify: `internal/scene/scene.go` (`HTML`)
- Test: `internal/scene/scene_test.go`

**Interfaces:**
- Consumes: `theme.Active().CSS(theme.Export)`.

- [ ] **Step 1: Write the failing test**

```go
func TestHTMLUsesTheExportRole(t *testing.T) {
	th, err := theme.FromTOML("[ui]\nexport = { fg = \"#0a0b0c\", bg = \"#0d0e0f\" }\n")
	if err != nil {
		t.Fatal(err)
	}
	theme.SetActive(th)
	defer theme.SetActive(theme.Builtin())
	page := HTML(nil, "t")
	if !strings.Contains(page, "background: #0d0e0f") || !strings.Contains(page, "color: #0a0b0c") {
		t.Errorf("page:\n%s", page)
	}
}

func TestHTMLDefaultColorsUnchanged(t *testing.T) {
	page := HTML(nil, "t")
	if !strings.Contains(page, "background: #1b1b1f; color: #d8d8d8;") {
		t.Errorf("the built-in export colors changed:\n%s", page)
	}
}
```

Add `"github.com/latrani/Kiln/internal/theme"` to the imports.

- [ ] **Step 2: Run to see it fail**

Run: `go test ./internal/scene -run HTML`
Expected: `TestHTMLUsesTheExportRole` FAILS.

- [ ] **Step 3: Implement**

In `HTML`, build the body rule from the role. A role that sets no color leaves that property out:

```go
	fg, bg := theme.Active().CSS(theme.Export)
	var body []string
	if bg != "" {
		body = append(body, "background: "+bg) //str:ok
	}
	if fg != "" {
		body = append(body, "color: "+fg) //str:ok
	}
	body = append(body, "margin: 2rem") //str:ok
	fmt.Fprintf(&b, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>%s</title>
<style>
body { %s; }
pre { font: 14px/1.45 ui-monospace, SFMono-Regular, Menlo, monospace; white-space: pre-wrap; }
</style>
</head>
<body>
<pre>
`, html.EscapeString(title), strings.Join(body, "; ")) //str:ok
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/scene ./internal/str && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/scene
git commit -m "feat(scene): the HTML export takes its colors from the theme"
```

---

### Task 13: Guard and docs

**Files:**
- Create: `internal/ui/nostyle_test.go`
- Modify: `README.md` (a Themes section)
- Modify: `CLAUDE.md` (the theming rule)

- [ ] **Step 1: Write the guard test**

```go
package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var sgrRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// TestNoHardCodedStyles keeps escape codes out of the UI's drawing: every
// color and attribute comes from a theme role (internal/theme). It flags
// string literals holding an SGR sequence in this package and cmd/kiln.
func TestNoHardCodedStyles(t *testing.T) {
	var files []string
	for _, glob := range []string{"*.go", filepath.Join("..", "..", "cmd", "kiln", "*.go")} {
		m, _ := filepath.Glob(glob)
		files = append(files, m...)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if bl, ok := n.(*ast.BasicLit); ok && bl.Kind == token.STRING {
				if s, err := strconv.Unquote(bl.Value); err == nil && sgrRE.MatchString(s) {
					t.Errorf("%s: %s: draw it with theme.Paint and a role instead", path, bl.Value)
				}
			}
			return true
		})
	}
}
```

If a literal legitimately needs an escape that isn't a style (none should remain in these files), say so in review; don't weaken the test.

- [ ] **Step 2: Run it**

Run: `go test ./internal/ui -run NoHardCodedStyles`
Expected: PASS. A failure names a style that Tasks 5–9 missed: move it to its role.

- [ ] **Step 3: README**

Add a `### Themes` section after `### Log mode` in `README.md`:

```markdown
### Themes

Kiln's colors come from a theme. To change them, make `themes/default.toml` in Kiln's config folder, start it with `extends = "default"`, and set only what you want different. Kiln picks up changes as you save.

    extends = "default"

    [palette]
    panel = "#1f2029"

    [ui]
    "sidebar"        = { bg = "panel" }
    "sidebar.active" = { reverse = true }
    "status"         = { bg = "panel" }
    "status.error"   = { fg = "#ff6b6b", bold = true }

A style sets any of `fg`, `bg`, `bold`, `faint`, `italic`, `underline` and `reverse`. A color is `#rrggbb`, a name from `[palette]`, or one of the terminal's own sixteen (`red`, `bright-blue`, and so on). A role inherits from the one before its last dot (`link.hover` starts from `link`). Text from the server always sits on your terminal's background; the sidebar, input box, statusline and log mode's header and action bar can have their own. If a theme has a mistake, Kiln says so in the statusline and keeps the colors it had.

The roles are: `sidebar` (`.world`, `.char`, `.active`, `.unread`, `.attention`, `.connecting`, `.disconnected`, `.add`, `.more`), `picker` (`.world`, `.world.selected`, `.selected`, `.add`), `divider`, `rule`, `scrollback` (`.day`, `.history_end`, `.loading`, `.echo`, `.sys`, `.pill`, `.selection`, `.inactive`), `link` (`.hover`), `input` (`.hint`, `.over_limit`, `.selection`), `status` (`.log`, `.error`, `.clock`), `form` (`.label`, `.hint`, `.error`, `.focus`, `.title`), `log` (`.header`, `.header.title`, `.header.chip`, `.header.chip.on`, `.time`, `.cursor`, `.selected`, `.excluded`, `.find`, `.day`, `.loading`, `.bar`, `.bar.hints`, `.bar.error`), and `export` (the HTML export's page).
```

- [ ] **Step 4: CLAUDE.md**

Append to `CLAUDE.md`:

```markdown

## Colors and styles

Every color and attribute Kiln draws comes from a theme role
(`internal/theme/roles.go`), never an escape code in the UI:
`theme.Paint(theme.StatusError, msg)`. A new kind of thing on screen gets
a new role, added to `roles.go` (`Roles` too), the built-in
`internal/theme/default.toml`, and the README's role list.
`TestNoHardCodedStyles` fails on escape codes in `internal/ui` and
`cmd/kiln`. Painted areas (sidebar, input, statusline, log header and
action bar) go through `theme.Fill`. The scrollback and log body never do:
server text sits on the terminal's background. The golden screens in
`internal/ui/testdata/golden` pin the built-in look; update them with
`-update` only when a change to the look is intended, and say so in the
commit.
```

- [ ] **Step 5: Run everything**

Run: `go test ./... && go vet ./... && gofmt -l cmd internal`
Expected: all PASS, and `gofmt -l` prints nothing.

- [ ] **Step 6: Commit**

```bash
git add internal/ui README.md CLAUDE.md
git commit -m "docs: themes; test: no hard-coded styles in the UI"
```
