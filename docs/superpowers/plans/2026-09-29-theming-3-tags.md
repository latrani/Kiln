# Theming Phase 3: Tags, Behavior and Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Server lines get their look from the theme's `[tags]` and their behavior from `attention` and `quiet` tag lists. `[[highlight]]` is gone, `/highlight` writes a `highlight` classify tag, and Indi's own config is rewritten to the new form (#88).

**Architecture:** `internal/theme` learns a `[tags]` table (styles with `scope`, falling back up slashes) and *layers*: a world's or character's own `[palette]` and `[tags]`, built on top of the active theme with `Theme.With`. `internal/rules` shrinks to one job: given a line's classify tags, fold the tag styles into styled runs and work out attention and quiet from the lists. `internal/style` becomes a leaf package that draws runs of SGR over server text, which breaks the import cycle theme → style → rules → theme. `internal/config` drops `[[highlight]]` and carries `attention`, `quiet` and the per-world/per-character look tables.

**Tech Stack:** Go 1.27, Bubble Tea v2, BurntSushi TOML, Kiln's `internal/str` catalog.

**Spec:** `docs/superpowers/specs/2026-09-28-theming-design.md` (sections: Three layers, Theme files, Choosing a theme (the world/character tables only), A full example, Migration, Phases → 3, Core tags)

## Global Constraints

- **The rule:** server text sits on the terminal's background. Tag styles may use any color, background included (they're Kiln's styling of server lines), and `style.Highlight` re-applies them after every server reset.
- **Tag styles** take the theme style fields (`fg`, `bg`, `bold`, `faint`, `italic`, `underline`, `reverse`) plus `scope = "line" | "match"` (default `"line"`). Colors are `#rrggbb`, a palette name, a terminal color name, or `default`.
- **Tags fall back up their slashes:** a line tag `page/in` uses the most specific styled name among `page/in`, `page`.
- **Folding:** whole-line styles first, then match-scope styles on top of the text they cover. Later colors win; attributes add up.
- **Layers:** a world's `[palette]`/`[tags]` sit over its theme, and a character's sit over its world's. Layers merge into the theme field by field, like `extends`, so a layer's `{ fg = … }` over the theme's `{ bold = true }` gives both.
- **`attention` and `quiet`** are lists of tag names. They're inheritable, and they *add up* along the chain: `[defaults]` → packs (in `use` order) → world → character, the way rule lists do. A list entry `x` matches a line tag `x` or any tag under it (`x/…`). Quiet wins over attention.
- **`[[highlight]]` is removed, with no shim.** A file that still has it fails with the existing unknown-key error. Don't add a dedicated message.
- **Worlds and characters get `[palette]` and `[tags]` only in this phase.** A per-world `[ui]` (the chrome following the active character's world) comes with #89, where the per-world `theme` setting lands.
- **User-facing text goes through `internal/str`.** Add or edit entries in `internal/str/locales/en.toml`, run `go generate ./internal/str`, call the generated function. Delete entries that nothing uses any more. Tests build expected text from `str` functions. Commits that add or change strings carry a `Strings:` trailer.
- **Config values are vocabulary**: `line`, `match`, `attention`, `quiet`, `highlight`, tag names. They stay verbatim and aren't translated.
- **Fixtures** use Kit, Rook, Ash, Mira and world `fm`. Smoke tests use a scratch `XDG_CONFIG_HOME`/`XDG_DATA_HOME`.
- **Golden screens** (`internal/ui/testdata/golden`) must not change in Tasks 1–6. Tags only restyle server lines, and the goldens' server lines keep their look because the default theme's `[tags]` reproduce the old fuzzball pack colors exactly.
- Commits end with the session's `Co-Authored-By` and `Claude-Session` lines.

## Review Focus

- **A world's `[tags]` naming a color that doesn't exist** (`fg = "beacon"` with no `beacon` in any palette) must not break the character. Kiln reports it in the statusline as `fm/kit: …` and draws that character's lines with the theme's own tag styles. Pinned in Task 5 (`TestBadWorldLookFallsBackToTheme`).
- **Editing the theme's `[tags]`** must restyle lines already in the scrollback, the same way a `[ui]` change does. Pinned in Task 5 (`TestThemeTagChangeRestyles`).
- **`attention = ["page"]` must catch `page/in` but not `pages`** (a bare prefix isn't a parent). Pinned in Task 3 (`TestListsMatchTagAndChildren`).
- **A tag in both `attention` and `quiet`** (or a line with one tag from each) is quiet, never attention. Pinned in Task 3 (`TestQuietWinsOverAttention`).
- **A config that still has `[[highlight]]`** fails to load, naming the file and the key, and nothing panics. Pinned in Task 4 (`TestHighlightIsGone`).

---

### Task 1: `[tags]` in theme files

**Files:**
- Modify: `internal/theme/theme.go`, `internal/theme/load.go`
- Modify: `internal/str/locales/en.toml` (then `go generate ./internal/str`)
- Test: `internal/theme/theme_test.go`, `internal/theme/load_test.go`

**Interfaces:**
- Produces:
  - `type Style struct` (exported, resolved): `func (s Style) SGR() string`, `func (s Style) Over(b Style) Style`.
  - `type TagStyle struct { Style Style; Match bool }`. `Match` is true for `scope = "match"`.
  - `func (t *Theme) Tag(name string) (styled string, ts TagStyle, ok bool)`: the most specific styled name up `name`'s slashes, and its style.
  - `Theme.Equal` also compares tag styles.
  - The theme error messages take the file's full path (`themes/x.toml`, or later `worlds/fm.toml`) as `{file}`.

- [ ] **Step 1: Reword the theme strings to take a full path, and add the tag strings**

In `internal/str/locales/en.toml`, `[theme]` section, drop the hard-coded `themes/` from each entry and add three new ones. Keep every entry's placeholders the same.

```toml
[theme]
parse = "{file}: {err}"
unknown_key = "{file}: unknown key {key:%q} (a theme has extends, palette, ui and tags)"
unknown_role = "{file}: unknown role {role:%q}"
unknown_field = "{file}: {role}: unknown setting {field:%q}"
bad_field = "{file}: {role}: {field} must be {want}"
bad_color = "{file}: {where}: {color:%q} isn't a color (use #rrggbb, a palette name, a terminal color like bright-blue, or default)"
palette_name_taken = "{file}: palette name {name:%q} is a terminal color"
no_theme = "no theme {name:%q} (no themes/{name}.toml)"
extends_loop = "{file}: extends loops back to {name:%q}"
not_table = "{file}: {key} must be a table, like [{key}]"
role_not_table = "{file}: {role} needs a table of settings, like {role} = {{ fg = … }}"
bad_extends = "{file}: extends {name:%q} isn't a theme name (a file name in themes/, without .toml)"
tag_not_table = "{file}: tag {tag:%q} needs a table of settings, like {{ fg = … }}"
bad_scope = '{file}: tag {tag:%q}: scope must be "line" or "match"'
tag_entry = "tag {name}"
palette_entry = "palette {name}"
want_color = "a color"
want_bool = "true or false"
want_theme = "a theme name"
```

(Keep any other `[theme]` entries that exist, unchanged.) Run `go generate ./internal/str`.

- [ ] **Step 2: Pass full paths**

In `load.go`, every `parse(...)` call and message uses the full path. The built-in is `"themes/default.toml"` too, since it stands in for that file.

```go
// in the builtin var and FromTOML:
f, err := parse("themes/default.toml", builtinSrc)
// in chainFor:
		return nil, errors.New(str.ThemeExtendsLoop("themes/"+seen[len(seen)-1]+".toml", name))
	...
	f, err := parse("themes/"+name+".toml", data)
// builtinFile:
		f, _ := parse("themes/default.toml", builtinSrc)
// FromTOML's test file:
	f, err := parse("themes/test.toml", []byte(src))
```

Update the existing tests that build expected messages. For example, `TestLoadErrors` becomes `str.ThemeUnknownRole("themes/default.toml", "sidebar.nope")`, and `TestExtendsCycle` becomes `str.ThemeExtendsLoop("themes/b.toml", "a")`. `mustBuild` in `theme_test.go` names its files `t0.toml` etc.; leave that alone, since those names only have to match the test's own expectations. Run `go test ./internal/theme`: PASS.

- [ ] **Step 3: Write the failing tests for `[tags]`**

Append to `internal/theme/theme_test.go`:

```go
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
```

- [ ] **Step 4: Run them to watch them fail**

Run: `go test ./internal/theme -run 'TestTags|TestTagErrors|TestStyleOver|TestEqualSeesTags'`
Expected: build failure (`th.Tag undefined`, `Style` undefined).

- [ ] **Step 5: Implement**

In `theme.go`:

1. Add the parsed form and hold it in `file`:

```go
// tagFileStyle is a tag's style as a file writes it.
type tagFileStyle struct {
	fileStyle
	scope *string // "line" or "match"; nil: unset
}

type file struct {
	name    string
	extends string
	palette map[string]string
	ui      map[Role]fileStyle
	tags    map[string]tagFileStyle
}
```

2. Pull the field reading out of `flatten` into a helper that both `[ui]` and `[tags]` use. `where` is the role or `str.ThemeTagEntry(tag)`, for messages:

```go
// readStyle reads a style's fields from tbl. Fields that aren't style
// fields are returned in rest, for the caller to judge.
func readStyle(name, where string, tbl map[string]any) (s fileStyle, rest map[string]any, err error) {
	for k, v := range tbl {
		switch k {
		case "fg", "bg":
			c, ok := v.(string)
			if !ok {
				return s, nil, errors.New(str.ThemeBadField(name, where, k, str.ThemeWantColor()))
			}
			if k == "fg" {
				s.fg = &c
			} else {
				s.bg = &c
			}
		case "bold", "faint", "italic", "underline", "reverse":
			b, ok := v.(bool)
			if !ok {
				return s, nil, errors.New(str.ThemeBadField(name, where, k, str.ThemeWantBool()))
			}
			*map[string]**bool{"bold": &s.bold, "faint": &s.faint, "italic": &s.italic, "underline": &s.underline, "reverse": &s.reverse}[k] = &b
		default:
			if rest == nil {
				rest = map[string]any{}
			}
			rest[k] = v
		}
	}
	return s, rest, nil
}
```

`flatten` keeps its own recursion (sub-tables are roles) and its `role_not_table` check, but collects a role's non-table values into a map and hands them to `readStyle`. Any `rest` is an error: `str.ThemeUnknownField(name, prefix, k)` for the first key of `rest` in sorted order. `TestParseErrors` and `TestDottedAndNestedRoleMerge` must still pass unchanged.

3. In `parse`, add the `tags` key (and set `f.tags = map[string]tagFileStyle{}` with the others):

```go
		case "tags":
			tbl, ok := v.(map[string]any)
			if !ok {
				return f, errors.New(str.ThemeNotTable(name, k))
			}
			for tag, tv := range tbl {
				fields, ok := tv.(map[string]any)
				if !ok {
					return f, errors.New(str.ThemeTagNotTable(name, tag))
				}
				where := str.ThemeTagEntry(tag)
				s, rest, err := readStyle(name, where, fields)
				if err != nil {
					return f, err
				}
				ts := tagFileStyle{fileStyle: s}
				for _, k := range slices.Sorted(maps.Keys(rest)) {
					sc, ok := rest[k].(string)
					if k != "scope" {
						return f, errors.New(str.ThemeUnknownField(name, where, k))
					}
					if !ok || sc != "line" && sc != "match" {
						return f, errors.New(str.ThemeBadScope(name, tag))
					}
					ts.scope = &sc
				}
				f.tags[tag] = ts
			}
```

(A `[tags]` table keyed with dots, like `"page.in"`, is just a tag name with a dot in it. Only `/` is a tag separator. Note that a bare `page.in = {…}` key without quotes parses as a nested table, which then fails the `tag_not_table` check; that's fine.)

4. Export the resolved style, and add the tags to `Theme`:

```go
// Style is a resolved style, for tag styles, which fold together.
type Style struct{ s style }

// SGR is the escape sequence that starts drawing in s, or "".
func (s Style) SGR() string { return s.s.sgr() }

// Over is s with b on top: b's colors win, attributes add up.
func (s Style) Over(b Style) Style {
	if b.s.fg != nil {
		s.s.fg = b.s.fg
	}
	if b.s.bg != nil {
		s.s.bg = b.s.bg
	}
	s.s.bold = s.s.bold || b.s.bold
	s.s.faint = s.s.faint || b.s.faint
	s.s.italic = s.s.italic || b.s.italic
	s.s.underline = s.s.underline || b.s.underline
	s.s.reverse = s.s.reverse || b.s.reverse
	return s
}

// TagStyle is how lines with a tag are drawn: the whole line, or with
// Match only the text the tag's pattern matched.
type TagStyle struct {
	Style Style
	Match bool
}

type Theme struct {
	styles map[Role]style
	sgr    map[Role]string
	tags   map[string]TagStyle
	chain  []file // what it was built from, base first; see With
}

// Tag is how a line tag is drawn: the style of the most specific styled
// name up its slashes (page/in, then page), and that name.
func (t *Theme) Tag(name string) (styled string, ts TagStyle, ok bool) {
	for n := name; ; {
		if ts, ok := t.tags[n]; ok {
			return n, ts, true
		}
		i := strings.LastIndexByte(n, '/')
		if i < 0 {
			return "", TagStyle{}, false
		}
		n = n[:i]
	}
}
```

5. In `build`, merge tags along the chain (field by field, scope last-set wins, remember the origin file for messages) and resolve them after the roles. Keep the chain on the theme:

```go
	mergedTags := map[string]tagFileStyle{}
	tagOrigin := map[string]string{}
	for _, f := range chain {
		// (after the palette and ui loops that are already there)
		for tag, s := range f.tags {
			m := mergedTags[tag]
			m.fileStyle = overlay(m.fileStyle, s.fileStyle)
			if s.scope != nil {
				m.scope = s.scope
			}
			mergedTags[tag], tagOrigin[tag] = m, f.name
		}
	}
	t := &Theme{styles: map[Role]style{}, sgr: map[Role]string{}, tags: map[string]TagStyle{}, chain: chain}
	// ... roles as before ...
	for _, tag := range slices.Sorted(maps.Keys(mergedTags)) {
		fs := mergedTags[tag]
		s, err := resolveStyle(fs.fileStyle, palette, tagOrigin[tag], str.ThemeTagEntry(tag))
		if err != nil {
			return nil, err
		}
		t.tags[tag] = TagStyle{Style: Style{s}, Match: fs.scope != nil && *fs.scope == "match"}
	}
```

Extract the role loop's color and flag resolution into `resolveStyle(fs fileStyle, palette map[string]color, origin, where string) (style, error)`, starting from a zero `style`. The role loop then calls it and layers the result over the parent. A role's parent inheritance stays as it is: resolve the role's own fields over `t.styles[r.parent()]`, so give `resolveStyle` a `base style` parameter and pass the parent there, or `style{}` for tags. It returns `str.ThemeBadColor(origin, where, value)` for an unknown color, and handles `default` as before.

6. `Equal` compares tags too:

```go
func (t *Theme) Equal(o *Theme) bool {
	return maps.Equal(t.sgr, o.sgr) && maps.EqualFunc(t.tags, o.tags, func(a, b TagStyle) bool {
		return a.Match == b.Match && a.Style.SGR() == b.Style.SGR()
	})
}
```

- [ ] **Step 6: Run the theme tests**

Run: `go test ./internal/theme ./internal/str`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/theme internal/str
git commit -m "feat(theme): [tags]: styles for classify tags, with scope

Strings: theme.tag_not_table, theme.bad_scope, theme.tag_entry (new);
theme.* (every file message takes the file's path)"
```

---

### Task 2: Layers: a world's and character's own palette and tags

**Files:**
- Modify: `internal/theme/theme.go`
- Test: `internal/theme/theme_test.go`

**Interfaces:**
- Consumes: Task 1's `file`, `parse`, `build`, `Theme.chain`, `Tag`.
- Produces:
  - `type Layer struct` (opaque) and `func ParseLayer(where string, palette, tags map[string]any) (Layer, error)`. `where` is the file for messages (`worlds/fm.toml`, or a character's `str.ConfigCharacterId(...)`).
  - `func (t *Theme) With(layers ...Layer) (*Theme, error)`: t with the layers on top, in order. With no layers it returns t itself.

- [ ] **Step 1: Write the failing tests**

```go
func TestLayersOverTheme(t *testing.T) {
	base := mustBuild(t, "[palette]\nember = \"#ff9f43\"\n[tags]\npage = { fg = \"ember\", bold = true }\n")
	world, err := ParseLayer("worlds/fm.toml",
		map[string]any{"beacon": "#ffd166"},
		map[string]any{"highlight": map[string]any{"fg": "beacon", "scope": "match"}, "page": map[string]any{"italic": true}})
	if err != nil {
		t.Fatal(err)
	}
	char, err := ParseLayer("kit", nil, map[string]any{"highlight": map[string]any{"fg": "ember"}})
	if err != nil {
		t.Fatal(err)
	}
	th, err := base.With(world, char)
	if err != nil {
		t.Fatal(err)
	}
	if _, ts, _ := th.Tag("page"); ts.Style.SGR() != "\x1b[1;3;38;2;255;159;67m" {
		t.Errorf("page = %q, want the theme's color and bold with the world's italic", ts.Style.SGR())
	}
	if _, ts, _ := th.Tag("highlight"); ts.Style.SGR() != "\x1b[38;2;255;159;67m" || !ts.Match {
		t.Errorf("highlight = %q, match %v; want the character's color over the world's, the world's scope", ts.Style.SGR(), ts.Match)
	}
	if _, _, ok := base.Tag("highlight"); ok {
		t.Error("With changed the base theme")
	}
	if same, _ := base.With(); same != base {
		t.Error("With() should return the theme itself")
	}
}

func TestLayerErrors(t *testing.T) {
	if _, err := ParseLayer("worlds/fm.toml", nil, map[string]any{"page": "red"}); err == nil || err.Error() != str.ThemeTagNotTable("worlds/fm.toml", "page") {
		t.Errorf("err = %v", err)
	}
	l, err := ParseLayer("worlds/fm.toml", nil, map[string]any{"page": map[string]any{"fg": "beacon"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Builtin().With(l); err == nil || err.Error() != str.ThemeBadColor("worlds/fm.toml", str.ThemeTagEntry("page"), "beacon") {
		t.Errorf("unknown color: err = %v", err)
	}
}
```

- [ ] **Step 2: Run them to watch them fail**

Run: `go test ./internal/theme -run 'TestLayer'`
Expected: build failure (`ParseLayer undefined`).

- [ ] **Step 3: Implement**

```go
// Layer is a world's or character's own [palette] and [tags], drawn over
// the theme it uses (see Theme.With).
type Layer struct{ f file }

// ParseLayer reads a layer's tables, as a world or character file gives
// them. where names the file for messages. Colors are checked when the
// layer is put on a theme, since they may name the theme's palette.
func ParseLayer(where string, palette, tags map[string]any) (Layer, error) {
	raw := map[string]any{}
	if palette != nil {
		raw["palette"] = palette
	}
	if tags != nil {
		raw["tags"] = tags
	}
	f, err := parseTables(where, raw)
	return Layer{f}, err
}

// With is t with layers on top, in order, merged field by field.
func (t *Theme) With(layers ...Layer) (*Theme, error) {
	if len(layers) == 0 {
		return t, nil
	}
	chain := slices.Clone(t.chain)
	for _, l := range layers {
		chain = append(chain, l.f)
	}
	return build(chain)
}
```

Split `parse` into two steps: `parse(name, data)` does `toml.Unmarshal` and calls `parseTables(name, raw)`, which holds the existing `switch k` over `extends`, `palette`, `ui` and `tags`. Config only ever hands a layer `palette` and `tags`, because its strict decoder rejects any other key.

- [ ] **Step 4: Run the theme tests**

Run: `go test ./internal/theme`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/theme
git commit -m "feat(theme): layers: a world's or character's own palette and tags"
```

---

### Task 3: `rules` folds tag styles; `style` draws runs

**Files:**
- Modify: `internal/style/style.go`, `internal/style/style_test.go`
- Rewrite: `internal/rules/rules.go`, `internal/rules/rules_test.go`
- Delete: `internal/rules/pack_test.go` (Task 6 replaces it)
- Modify: `cmd/kiln/render.go`, `cmd/kiln/render_test.go`

**Interfaces:**
- Consumes: Task 1's `theme.Theme.Tag`, `theme.Style.SGR`, `theme.Style.Over`.
- Produces:
  - `style.Run{Start, End int; SGR string}` (`SGR == ""`: left as the server sent it), `func style.Highlight(text string, runs []style.Run) string`, and `func style.Apply(text, sgr string) string`. `style.SGR(config.Style)` is removed. `style` imports only `internal/ansi`.
  - `rules.New(th *theme.Theme, attention, quiet []string) *Highlighter` (no error), `(*Highlighter).Apply(plain string, tags []classify.Tag) rules.Result`, and `rules.Result{Runs []style.Run; Attention, Quiet bool}`. `Runs == nil` means unstyled.

- [ ] **Step 1: Write the failing `rules` tests**

Replace `internal/rules/rules_test.go`:

```go
package rules

import (
	"testing"

	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

func mustTheme(t *testing.T, tags string) *theme.Theme {
	t.Helper()
	th, err := theme.FromTOML("[tags]\n" + tags)
	if err != nil {
		t.Fatal(err)
	}
	return th
}

// tag is a tag with spans.
func tag(name string, spans ...classify.Span) classify.Tag { return classify.Tag{Name: name, Spans: spans} }

func sgr(t *testing.T, th *theme.Theme, names ...string) string {
	t.Helper()
	var s theme.Style
	for _, n := range names {
		_, ts, ok := th.Tag(n)
		if !ok {
			t.Fatalf("no style for %s", n)
		}
		s = s.Over(ts.Style)
	}
	return s.SGR()
}

func TestWholeLineTags(t *testing.T) {
	th := mustTheme(t, `"page/in" = { fg = "#ff9f43", bold = true }
self = { italic = true }`)
	h := New(th, nil, nil)
	plain := "Mira pages: hi Kit"
	if got := h.Apply("Rook waves.", nil); got.Runs != nil {
		t.Errorf("untagged line styled: %+v", got)
	}
	got := h.Apply(plain, []classify.Tag{tag("page"), tag("page/in"), tag("self", classify.Span{Start: 15, End: 18})})
	want := []style.Run{{Start: 0, End: len(plain), SGR: sgr(t, th, "page/in", "self")}}
	if len(got.Runs) != 1 || got.Runs[0] != want[0] {
		t.Errorf("Runs = %+v, want %+v", got.Runs, want)
	}
}

func TestMatchScopeOnTopOfLine(t *testing.T) {
	th := mustTheme(t, `highlight = { fg = "#ffd166", scope = "match" }
page = { italic = true }`)
	h := New(th, nil, nil)
	// The match-scope tag comes first on the line but still draws on top.
	got := h.Apply("Mira pages: the lighthouse", []classify.Tag{tag("highlight", classify.Span{Start: 16, End: 26}), tag("page", classify.Span{Start: 0, End: 11})})
	want := []style.Run{
		{Start: 0, End: 16, SGR: sgr(t, th, "page")},
		{Start: 16, End: 26, SGR: sgr(t, th, "page", "highlight")},
	}
	if len(got.Runs) != 2 || got.Runs[0] != want[0] || got.Runs[1] != want[1] {
		t.Errorf("Runs = %+v, want %+v", got.Runs, want)
	}
}

func TestMatchScopeWithoutSpans(t *testing.T) {
	th := mustTheme(t, `highlight = { fg = "#ffd166", scope = "match" }`)
	if got := New(th, nil, nil).Apply("x", []classify.Tag{tag("highlight")}); got.Runs != nil {
		t.Errorf("a match-scope tag with no spans styled the line: %+v", got.Runs)
	}
}

func TestListsMatchTagAndChildren(t *testing.T) {
	h := New(mustTheme(t, ""), []string{"page"}, nil)
	for _, c := range []struct {
		tag  string
		want bool
	}{{"page", true}, {"page/in", true}, {"page/in/x", true}, {"pages", false}, {"whisper", false}} {
		if got := h.Apply("x", []classify.Tag{tag(c.tag)}).Attention; got != c.want {
			t.Errorf("attention for %q = %v, want %v", c.tag, got, c.want)
		}
	}
}

func TestQuietWinsOverAttention(t *testing.T) {
	h := New(mustTheme(t, ""), []string{"self", "spam"}, []string{"spam"})
	for _, tags := range [][]classify.Tag{{tag("spam")}, {tag("self"), tag("spam")}} {
		res := h.Apply("x", tags)
		if !res.Quiet || res.Attention {
			t.Errorf("%v: quiet %v attention %v, want quiet only", tags, res.Quiet, res.Attention)
		}
	}
}
```

- [ ] **Step 2: Write the failing `style` tests**

In `internal/style/style_test.go`, delete `TestSGR` (its job moved to the theme) and change the rest to the new signatures. Runs carry SGR strings now:

```go
	under := "\x1b[4m"
	styled := func(start, end int, sgr string) Run { return Run{Start: start, End: end, SGR: sgr} }
	plain := func(start, end int) Run { return Run{Start: start, End: end} }
```

Each case's `res rules.Result` becomes `runs []Run`: `runs(plain(0, 3), styled(3, 6, under), plain(6, 7))` becomes `[]Run{plain(0, 3), styled(3, 6, under), plain(6, 7)}`. The unstyled case is `nil`. The whole-line case is `[]Run{styled(0, 2, "\x1b[3m")}` with the same expected output as before. `Apply("x", "")` is `"x\x1b[0m"`, and `Apply("\x1b[1mMira\x1b[0m pages", "\x1b[3m")` keeps its current expectation. Drop the `config` and `rules` imports.

- [ ] **Step 3: Run them to watch them fail**

Run: `go test ./internal/rules ./internal/style`
Expected: build failures (`New` has the wrong arguments, `Run` is undefined in `style`).

- [ ] **Step 4: Implement `style`**

In `internal/style/style.go`: the package comment becomes `// Package style draws styled runs over server text, keeping the server's own SGR.` Remove `SGR` and `hexRGB` and the `config` and `rules` imports, and add:

```go
// Run is a stretch [Start, End) of a line's plain text, drawn in SGR, or
// as the server sent it when SGR is "".
type Run struct {
	Start, End int
	SGR        string
}

// Apply wraps text (which may contain the server's own SGR sequences) in
// sgr, re-applying it after every server SGR inside text, and ends with
// Reset.
func Apply(text, sgr string) string {
	if sgr == "" {
		return text + Reset
	}
	return Highlight(text, []Run{{Start: 0, End: len(text), SGR: sgr}})
}
```

`Highlight(text string, runs []Run)`: `if len(runs) == 0 { return text + Reset }`, drop the `res.Runs == nil` branch, and in the loop use `runs[ri].SGR != ""` / `ours = runs[ri].SGR` in place of `Styled` / `SGR(runs[ri].Style)`. Everything else stays.

- [ ] **Step 5: Implement `rules`**

Replace `internal/rules/rules.go`:

```go
// Package rules works out how a classified line looks and behaves: its
// tags' styles from the theme, and attention and quiet from tag lists.
package rules

import (
	"cmp"
	"slices"
	"strings"

	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

// Highlighter is one character's look and behavior for its lines.
type Highlighter struct {
	th               *theme.Theme
	attention, quiet []string
}

// Result is how a line is drawn and what it asks for.
type Result struct {
	Runs      []style.Run // nil: the line as the server sent it
	Attention bool        // never with Quiet
	Quiet     bool        // not unread, no attention, no notification
}

// New makes a Highlighter drawing tags in th's styles. A line asks for
// attention (or is quiet) when one of its tags is in the list, or is
// under one: "page" covers "page/in".
func New(th *theme.Theme, attention, quiet []string) *Highlighter {
	return &Highlighter{th: th, attention: attention, quiet: quiet}
}

// in reports whether tag is in list or under one of its entries.
func in(list []string, tag string) bool {
	return slices.ContainsFunc(list, func(x string) bool { return tag == x || strings.HasPrefix(tag, x+"/") })
}

// hit is one styled tag name on a line, with the spans of the line's
// tags that fall back to it.
type hit struct {
	ts    theme.TagStyle
	spans []classify.Span
}

// Apply works out plain's runs and behavior from its tags. Each tag
// takes the style of the most specific styled name up its slashes; tags
// that land on the same name count once. Whole-line styles fold first,
// in tag order, then match-scope ones on top of the text they matched.
// Later colors win and attributes add up. Quiet wins over attention.
func (h *Highlighter) Apply(plain string, tags []classify.Tag) Result {
	var res Result
	var hits []hit
	at := map[string]int{}
	for _, t := range tags {
		res.Attention = res.Attention || in(h.attention, t.Name)
		res.Quiet = res.Quiet || in(h.quiet, t.Name)
		name, ts, ok := h.th.Tag(t.Name)
		if !ok {
			continue
		}
		if i, ok := at[name]; ok {
			hits[i].spans = append(hits[i].spans, t.Spans...)
			continue
		}
		at[name] = len(hits)
		hits = append(hits, hit{ts: ts, spans: slices.Clone(t.Spans)})
	}
	if res.Quiet {
		res.Attention = false
	}
	hits = slices.DeleteFunc(hits, func(h hit) bool { return h.ts.Match && len(h.spans) == 0 })
	if len(hits) == 0 {
		return res
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return cmp.Compare(b2i(a.ts.Match), b2i(b.ts.Match)) })
	cuts := []int{0, len(plain)}
	for _, h := range hits {
		if h.ts.Match {
			for _, s := range h.spans {
				cuts = append(cuts, s.Start, s.End)
			}
		}
	}
	slices.Sort(cuts)
	cuts = slices.Compact(cuts)
	for i := 0; i+1 < len(cuts); i++ {
		var st theme.Style
		for _, h := range hits {
			if !h.ts.Match || covers(h.spans, cuts[i], cuts[i+1]) {
				st = st.Over(h.ts.Style)
			}
		}
		run := style.Run{Start: cuts[i], End: cuts[i+1], SGR: st.SGR()}
		if n := len(res.Runs); n > 0 && res.Runs[n-1].SGR == run.SGR {
			res.Runs[n-1].End = run.End
			continue
		}
		res.Runs = append(res.Runs, run)
	}
	if len(res.Runs) == 1 && res.Runs[0].SGR == "" {
		res.Runs = nil // styles that set nothing
	}
	return res
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// covers reports whether [start, end) lies inside one of spans. Runs are
// cut at every span edge, so a run is wholly inside a span or outside it.
func covers(spans []classify.Span, start, end int) bool {
	return slices.ContainsFunc(spans, func(s classify.Span) bool { return s.Start <= start && end <= s.End })
}
```

Delete `internal/rules/pack_test.go`.

- [ ] **Step 6: Update `cmd/kiln/render.go`**

```go
// render formats one entry for plain terminal output. Styled lines are
// drawn in their tags' styles; attention lines get a "» " marker, which
// is never styled. Sent lines are dimmed and sys lines are prefixed with
// "* ". Text is sanitized first, so res must be computed on
// ansi.Strip(ansi.Sanitize(…)).
func render(e logstore.Entry, res rules.Result) string {
	e.Text = ansi.Sanitize(e.Text)
	switch e.Dir {
	case logstore.Out:
		return theme.Paint(theme.ScrollbackEcho, "> "+e.Text)
	case logstore.Sys:
		return theme.Paint(theme.ScrollbackSys, "* "+e.Text)
	}
	marker := ""
	if res.Attention {
		marker = "» "
	}
	return marker + style.Highlight(e.Text, res.Runs) // runs index e.Text, not the marker
}
```

In `cmd/kiln/render_test.go`, drop the `config` import. Cases become runs: the whole-line page is `rules.Result{Runs: []style.Run{{Start: 0, End: len("Mira pages: hi"), SGR: "\x1b[1;38;2;255;159;67m"}}, Attention: true}` with want `"» \x1b[1;38;2;255;159;67mMira pages: hi\x1b[0m"` (the marker is now outside the style). Adjust the other cases the same way. Delete the "bad color ignored" case, since colors are resolved by the theme now. In `cmd/kiln/main.go` (tail mode), build the highlighter from the loaded theme:

```go
	th := theme.Active()
	if lt, err := th.With(ch.Looks...); err != nil {
		fmt.Fprintln(os.Stderr, "*", ch.World+"/"+ch.ID+": "+err.Error()) //str:ok: a key and a catalog message
	} else {
		th = lt
	}
	hl := rules.New(th, ch.Rules.Attention, ch.Rules.Quiet)
```

(`ch.Looks`, `Rules.Attention` and `Rules.Quiet` arrive in Task 4. Until then, write `rules.New(theme.Active(), nil, nil)` here and in `internal/ui` so the build passes, and let Task 4 put the real values in.)

- [ ] **Step 7: Make it build, then run everything that doesn't depend on Task 4**

In `internal/ui/model.go`, `compile` becomes:

```go
func (cs *charState) compile() error {
	cls, err := classify.New(cs.ch.Rules.Classify, cs.ch.Name, cs.ch.Aliases)
	if err != nil {
		return err
	}
	cs.cls, cs.hl = cls, rules.New(theme.Active(), nil, nil)
	return nil
}
```

and `renderLine` ends `return style.Highlight(text, res.Runs), res`. Fix `TestRenderLineMatchScope` in `model_test.go` to build its highlighter from `theme.FromTOML("[tags]\npage = { fg = \"#2053ff\", bold = true, scope = \"match\" }\n")` with `attention = []string{"page"}`, and expect `"\x1b[1;38;2;32;83;255mPAGE:" + style.Reset + " Mira says hi" + style.Reset` with `res.Attention`.

Run: `go build ./... && go test ./internal/rules ./internal/style ./cmd/...`
Expected: PASS. (`internal/ui` and `internal/config` still have `[[highlight]]` fixtures; Task 4 fixes them.)

- [ ] **Step 8: Commit**

```bash
git add internal/rules internal/style internal/ui/model.go internal/ui/model_test.go cmd/kiln
git commit -m "refactor(rules): tags take their styles from the theme; style draws runs"
```

---

### Task 4: Config: `attention`, `quiet`, looks; `[[highlight]]` gone

**Files:**
- Modify: `internal/config/config.go`, `internal/config/write.go`
- Modify: `internal/str/locales/en.toml` (delete the highlight-rule entries)
- Test: `internal/config/config_test.go`, `internal/config/write_test.go`, `internal/config/edit_test.go`

**Interfaces:**
- Consumes: Task 2's `theme.ParseLayer`.
- Produces:
  - `config.Rules{Classify []ClassifyRule; Attention, Quiet []string}`. `HighlightRule`, `Match` and `Style` are removed.
  - `config.Character.Looks []theme.Layer`: the world's layer, then the character's, each present only if it has a `[palette]` or `[tags]`.
  - `config.AppendHighlight(dir, world, text)` writes a `[[classify]]` rule with `tags = ["highlight"]`. `HighlightStyle` is removed.
  - `const config.HighlightTag = "highlight"`.

- [ ] **Step 1: Write the failing tests**

In `internal/config/config_test.go`:

```go
func TestAttentionAndQuietAddUp(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "worlds"), 0o700)
	os.MkdirAll(filepath.Join(dir, "packs"), 0o700)
	os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[defaults]\nattention = [\"self\"]\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "packs", "p.toml"), []byte("attention = [\"page/in\", \"self\"]\nquiet = [\"spam\"]\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte(`host = "h"
port = 1
use = ["p"]
attention = ["whisper/in"]

[[characters]]
name = "Kit"
quiet = ["wiki"]
`), 0o600)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, _ := cfg.Find("fm", "Kit")
	if want := []string{"self", "page/in", "whisper/in"}; !slices.Equal(kit.Rules.Attention, want) {
		t.Errorf("attention = %v, want %v", kit.Rules.Attention, want)
	}
	if want := []string{"spam", "wiki"}; !slices.Equal(kit.Rules.Quiet, want) {
		t.Errorf("quiet = %v, want %v", kit.Rules.Quiet, want)
	}
}

func TestWorldAndCharacterLooks(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "worlds"), 0o700)
	os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte(`host = "h"
port = 1

[palette]
beacon = "#ffd166"

[tags]
highlight = { fg = "beacon", scope = "match" }

[[characters]]
name = "Kit"
tags = { highlight = { bold = true } }

[[characters]]
name = "Rook"
`), 0o600)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, _ := cfg.Find("fm", "Kit")
	rook, _ := cfg.Find("fm", "Rook")
	if len(kit.Looks) != 2 || len(rook.Looks) != 1 {
		t.Fatalf("looks: kit %d, rook %d; want 2 and 1", len(kit.Looks), len(rook.Looks))
	}
	th, err := theme.Builtin().With(kit.Looks...)
	if err != nil {
		t.Fatal(err)
	}
	if _, ts, _ := th.Tag("highlight"); ts.Style.SGR() != "\x1b[1;38;2;255;209;102m" || !ts.Match {
		t.Errorf("kit's highlight = %q, match %v", ts.Style.SGR(), ts.Match)
	}
}

func TestBadWorldTagsFailLoad(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "worlds"), 0o700)
	os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte("host = \"h\"\nport = 1\n[tags]\npage = \"red\"\n"), 0o600)
	if _, err := Load(dir); err == nil || err.Error() != str.ThemeTagNotTable(filepath.Join("worlds", "fm.toml"), "page") {
		t.Errorf("err = %v", err)
	}
}

func TestHighlightIsGone(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "worlds"), 0o700)
	os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte("host = \"h\"\nport = 1\n[[highlight]]\nmatch = { pattern = 'x' }\n"), 0o600)
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "fm.toml") || !strings.Contains(err.Error(), "highlight") {
		t.Errorf("err = %v, want the unknown-key error naming fm.toml and highlight", err)
	}
}
```

(`TestHighlightIsGone` matches on the file and key names, not on catalog text, which is why the literals are allowed here. Add `//str:ok` on that line if `TestTestsReadTheCatalog` flags it.)

In `internal/config/write_test.go`, rewrite the three `AppendHighlight` tests to check the classify rule it now writes:

```go
	if len(kit.Rules.Classify) != 1 {
		t.Fatalf("rules = %+v (the rule must land at world level, not inside the last [[characters]])", kit.Rules.Classify)
	}
	r := kit.Rules.Classify[0]
	if !slices.Equal(r.AllTags(), []string{HighlightTag}) {
		t.Errorf("tags = %v, want [%s]", r.AllTags(), HighlightTag)
	}
	if !regexp.MustCompile(r.Pattern).MatchString(`THE "LIGHTHOUSE" (OLD)`) {
		t.Errorf("pattern %q should match case-insensitively", r.Pattern)
	}
```

`TestAppendHighlightKeepsSpacingAndEscapesDEL` reads `kit.Rules.Classify[0].Pattern`. At `write_test.go:104`, the fixture's `[[highlight]]\nmatch = { pattern = \"x\" }` becomes `[[classify]]\ntag = \"x\"\npattern = \"x\"`, and the check reads `ch.Rules.Classify`.

In `internal/config/edit_test.go`, the fixture's `[[characters.highlight]]` block (line 26) and the `# added by /highlight` `[[highlight]]` block (line 34) become `[[characters.classify]]` and `[[classify]]` rules with a `tag` and a `pattern`. Line 172's check expects `"# added by /highlight\n[[classify]]"`. In `config_test.go`, delete the "empty highlight match" and "bad scope" cases and `TestHighlightScope`. Lines 200 and 327–347 switch from highlight rules to classify rules the same way.

- [ ] **Step 2: Run them to watch them fail**

Run: `go test ./internal/config`
Expected: build failure (`Rules.Attention`, `Character.Looks`, `HighlightTag` undefined).

- [ ] **Step 3: Implement**

In `config.go`:

- Delete `Style`, `Match`, `HighlightRule` and the highlight loop in `validate`.
- Change `Rules` and `appendRules`:

```go
// Rules is what packs, worlds and characters say about lines: how to tag
// them, and which tags ask for attention or are quiet. Along the chain
// the classify rules append, and the tag lists add up.
type Rules struct {
	Classify  []ClassifyRule `toml:"classify"`
	Attention []string       `toml:"attention"`
	Quiet     []string       `toml:"quiet"`
}

func appendRules(a, b Rules) Rules {
	return Rules{
		Classify:  append(append([]ClassifyRule(nil), a.Classify...), b.Classify...),
		Attention: union(a.Attention, b.Attention),
		Quiet:     union(a.Quiet, b.Quiet),
	}
}

// union is a then b's new entries, in order.
func union(a, b []string) []string {
	out := slices.Clone(a)
	for _, s := range b {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}
```

- `[defaults]` takes the lists. `globalFile.Defaults` becomes a `defaultsFile`, and `loadGlobal` returns its `Rules` part as well:

```go
type defaultsFile struct {
	settings
	Attention []string `toml:"attention"`
	Quiet     []string `toml:"quiet"`
}
```

`loadGlobal` returns `(globalFile, settings, Rules, error)`, with the rules being `Rules{Attention: g.Defaults.Attention, Quiet: g.Defaults.Quiet}`. `Load` passes them to `loadWorld`, and `loadWorldData` starts `worldRules` from them before the packs.

- Looks. Add to both `worldFile` and `charFile`:

```go
	Palette map[string]any `toml:"palette"`
	Tags    map[string]any `toml:"tags"`
```

and to `Character`: `Looks []theme.Layer // the world's and then the character's own [palette] and [tags]`. In `loadWorldData`, after the TLS checks:

```go
	var worldLooks []theme.Layer
	if wf.Palette != nil || wf.Tags != nil {
		l, err := theme.ParseLayer(rel, wf.Palette, wf.Tags)
		if err != nil {
			return World{}, err
		}
		worldLooks = append(worldLooks, l)
	}
```

and per character, after `where` is final:

```go
		looks := slices.Clone(worldLooks)
		if cf.Palette != nil || cf.Tags != nil {
			l, err := theme.ParseLayer(where, cf.Palette, cf.Tags)
			if err != nil {
				return World{}, err
			}
			looks = append(looks, l)
		}
```

and set `Looks: looks` in the `Character` literal. (`internal/theme` must not import `internal/config`. It doesn't.)

In `write.go`: delete `HighlightStyle` and add `const HighlightTag = "highlight" //str:ok: config vocabulary`. `AppendHighlight`'s doc and rule become:

```go
// AppendHighlight adds a literal-text, case-insensitive classify rule,
// tagging matching lines "highlight", to the end of worlds/<world>.toml.
// The theme's [tags] say how highlight looks. Appending is safe because
// TOML table headers are absolute: "[[classify]]" always means the
// world's top-level list, even after a [characters.x] table. Existing
// content and comments are kept.
...
	rule := fmt.Sprintf("\n# %s\n[[classify]]\ntags = [%q]\npattern = %s\n", //str:ok
		str.ConfigAddedByHighlight(), HighlightTag, pattern)
```

In `en.toml`, delete `highlight_needs_match`, `highlight_bad_pattern` and `bad_highlight_scope`, and regenerate.

Back in `internal/ui/model.go` and `cmd/kiln/main.go`, put the real values into `rules.New` (see Task 5 for the UI's version).

- [ ] **Step 4: Run the config tests**

Run: `go generate ./internal/str && go test ./internal/config ./internal/str`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config internal/str cmd/kiln
git commit -m "feat(config): attention and quiet tag lists, world and character looks; [[highlight]] is gone

Strings: config.highlight_needs_match, config.highlight_bad_pattern,
config.bad_highlight_scope (removed)"
```

---

### Task 5: The UI draws tags from each character's looks

**Files:**
- Modify: `internal/ui/model.go`
- Test: `internal/ui/theme_test.go`, `internal/ui/model_test.go`, `internal/ui/notify_test.go`

**Interfaces:**
- Consumes: Task 2's `Theme.With`, Task 3's `rules.New`, Task 4's `Character.Looks` and `Rules.Attention`/`Quiet`.
- Produces: `charState.compile()` always leaves `cls` and `hl` usable when the classify rules compile. If the looks don't resolve, it falls back to the active theme alone and returns the error for the caller to show.

- [ ] **Step 1: Write the failing tests**

Append to `internal/ui/theme_test.go`:

```go
// Editing the theme's [tags] restyles lines already on screen.
func TestThemeTagChangeRestyles(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.show("Mira pages: you around?")
	writeUserTheme(t, h, "extends = \"default\"\n[tags]\n\"page/in\" = { fg = \"#0a0b0c\" }\n")
	h.m.Update(reloadMsg{})
	if !strings.Contains(h.drawn(), "\x1b[1;38;2;10;11;12m") {
		t.Error("the page wasn't restyled with the theme's new page/in color (bold from the built-in)")
	}
}

// A world's look naming a color nobody defines is reported, and the
// character still draws with the theme's own tag styles.
func TestBadWorldLookFallsBackToTheme(t *testing.T) {
	world := fmWorld + "\n[tags]\n\"page/in\" = { fg = \"beacon\" }\n"
	h := newHarness(t, map[string]string{"fm": world})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	if !strings.Contains(h.m.status, "fm/kit: "+upTo(str.ThemeBadColor(mark, "", ""))) {
		t.Errorf("status = %q, want the bad color reported", h.m.status)
	}
	h.show("Mira pages: you around?")
	_, ts, _ := theme.Active().Tag("page/in")
	if !strings.Contains(h.drawn(), ts.Style.SGR()+"Mira pages") {
		t.Error("the page should draw in the theme's own page/in style")
	}
}
```

(`upTo`/`mark` cut the catalog text at the placeholder; see `internal/ui/catalog_test.go`. If `str.ThemeBadColor`'s first placeholder isn't the one to cut at, cut with `after` instead, so the test only relies on the catalog.)

In `model_test.go` and `notify_test.go`, the three quiet fixtures (`model_test.go:1165`, `notify_test.go:157`) become a classify tag plus `quiet`:

```go
	world := fmWorld + "\nquiet = [\"wiki\"]\n\n[[classify]]\ntag = \"wiki\"\npattern = '^\\[Wiki\\]'\n"
```

and `"\nquiet = [\"chatter\"]\n\n[[classify]]\ntag = \"chatter\"\npattern = '^Rook'\n"`. The `quiet = […]` line must come before any `[[…]]` table so it stays top-level. Since `fmWorld` ends in a `[[characters]]` table, the world-level keys have to be spliced in before it:

```go
	world := strings.Replace(fmWorld, "max_line_bytes = 20", "max_line_bytes = 20\nquiet = [\"wiki\"]", 1) +
		"\n[[classify]]\ntag = \"wiki\"\npattern = '^\\[Wiki\\]'\n"
```

`TestHighlightKeepsSpacing` (`model_test.go:429`) reads `kit.Rules.Classify` and finds the rule tagged `config.HighlightTag`.

- [ ] **Step 2: Run them to watch them fail**

Run: `go test ./internal/ui -run 'TestThemeTagChangeRestyles|TestBadWorldLookFallsBackToTheme|TestQuiet|TestHighlightKeepsSpacing'`
Expected: FAIL (the tags aren't drawn from the theme, no status).

- [ ] **Step 3: Implement**

In `internal/ui/model.go`:

```go
// compile builds cs's classifier, and its highlighter from the active
// theme with the character's own looks on top. Looks that don't resolve
// (a color nobody defines) leave the theme's own tag styles in use; the
// error is returned for the caller to show.
func (cs *charState) compile() error {
	cls, err := classify.New(cs.ch.Rules.Classify, cs.ch.Name, cs.ch.Aliases)
	if err != nil {
		return err
	}
	th, lookErr := theme.Active().With(cs.ch.Looks...)
	if lookErr != nil {
		th = theme.Active()
	}
	cs.cls, cs.hl = cls, rules.New(th, cs.ch.Rules.Attention, cs.ch.Rules.Quiet)
	return lookErr
}
```

`styleInputs` adds the looks:

```go
func styleInputs(ch config.Character) any {
	return struct {
		rules   config.Rules
		looks   []theme.Layer
		name    string
		aliases []string
	}{ch.Rules, ch.Looks, ch.Name, ch.Aliases}
}
```

`loadTheme` recompiles every character before restyling, since each highlighter holds the theme it was built on:

```go
	theme.SetActive(th)
	for _, k := range m.order {
		cs := m.chars[k]
		if err := cs.compile(); err != nil {
			m.setStatus(true, k+": "+err.Error())
		}
		render := func(e logstore.Entry) string { text, _ := cs.render(e); return text }
		cs.sb.Rerender(render)
		if cs.browse != nil {
			cs.browse.restyle(render)
		}
	}
```

(Iterate `m.order`, not the map, so which error ends up in the status is deterministic. `loadTheme` runs before `m.themeErr` is reported, and a theme error set later in the same update still wins, which is right: a broken theme is the bigger problem.)

- [ ] **Step 4: Run all the UI tests, goldens included**

Run: `go test ./internal/ui`
Expected: PASS, with the golden screens unchanged. If `TestGoldenMain` fails on the "Rook says, "Evening, Kit."" line, Task 6's default `[tags]` aren't in yet: this task's goldens rely on the default theme styling `self` bold. Move Step 1 of Task 6 (the default theme's `[tags]`) into this task, and commit it here.

- [ ] **Step 5: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): lines draw their tags from the theme and the character's looks"
```

---

### Task 6: The default theme's tags, the fuzzball pack, `/highlight`, docs

**Files:**
- Modify: `internal/theme/default.toml`
- Modify: `internal/config/defaults/packs/fuzzball.toml`
- Create: `internal/config/pack_test.go` (replaces the deleted `internal/rules/pack_test.go`)
- Modify: `README.md`
- Test: `internal/ui/browse_test.go` (`TestHighlightCommand`)

**Interfaces:**
- Consumes: everything above.
- Produces: nothing new in code.

- [ ] **Step 1: The default theme styles the core tags the old pack colored, and `highlight`**

Add to `internal/theme/default.toml`, after the `[palette]` swatches:

```toml
ember    = "#ff9f43"
lavender = "#c39bd3"
beacon   = "#ffd166"
```

and at the end:

```toml
# How lines with classify tags are drawn. A tag falls back up its
# slashes: whisper/in uses whisper's style if it has none of its own.
# scope = "match" styles only the text the tag's pattern matched.
[tags]
"page/in"    = { fg = "ember", bold = true }
"whisper/in" = { fg = "lavender", italic = true }
"self"       = { bold = true }
"highlight"  = { fg = "beacon", bold = true, scope = "match" }
```

`sidebar.attention` keeps its own `#ffd166`: it's chrome, and it can now say `fg = "beacon"`. Make that change so both follow one swatch.

- [ ] **Step 2: Write the failing pack test**

Create `internal/config/pack_test.go`:

```go
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
// conversation, and only the side you receive asks for attention and
// takes the default theme's color.
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
	hl := rules.New(theme.Builtin(), kit.Rules.Attention, kit.Rules.Quiet)
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
		{`You page-pose, "Kit grins." to Mira.`, "page/out", false},
	} {
		kind, _, _ := strings.Cut(c.dir, "/")
		tags := cls.Classify(c.line)
		if !slices.Contains(tags, kind) || !slices.Contains(tags, c.dir) {
			t.Errorf("%q: tags %v, want %s and %s", c.line, tags, kind, c.dir)
		}
		res := hl.Apply(c.line, cls.Tags(c.line))
		if res.Attention != c.in || (res.Runs != nil) != c.in {
			t.Errorf("%q: attention %v, styled %v; want both %v", c.line, res.Attention, res.Runs != nil, c.in)
		}
	}
}
```

Run: `go test ./internal/config -run TestStarterPackPages`
Expected: FAIL. The embedded pack still has `[[highlight]]`, so `Load` errors on the unknown key.

- [ ] **Step 3: Move the pack to the new form**

In `internal/config/defaults/packs/fuzzball.toml`, delete the three `[[highlight]]` blocks. Change the header comment and add the list:

```toml
# Starter rules for Fuzzball MUCKs (built-in commands plus the stock
# cmd-page and cmd-whisper programs). Servers customize these heavily;
# edit freely. The pack says what lines are and which ones ask for your
# attention; how they look is up to the theme's [tags].

attention = ["page/in", "whisper/in", "self"]
```

(`attention` goes above the first `[[classify]]`, so it stays a top-level key.) The page comment's "while the highlight below colors only what you receive" becomes "while only what you receive asks for attention."

- [ ] **Step 4: Run the pack, `/highlight` and golden tests**

`TestHighlightCommand` (`browse_test.go:526`) should still pass: the status text is unchanged. Add a check that a line containing the text now draws in the theme's `highlight` style:

```go
	h.show("the lighthouse is dark")
	_, ts, _ := theme.Active().Tag(config.HighlightTag)
	if !strings.Contains(h.drawn(), ts.Style.SGR()+"lighthouse") {
		t.Error("the new highlight didn't draw after the reload")
	}
```

(If `h.show` can't be used in that test because Kit isn't connected, settle the connection first the way `goldenHarness` does. The config watcher doesn't run in tests, so send `reloadMsg{}` after the command.)

Run: `go test ./...`
Expected: PASS, goldens unchanged.

- [ ] **Step 5: Docs**

In `README.md`:

- The rules section around line 246 ("Classify rules tag lines, and highlight rules style them") becomes three short parts: classify rules tag lines; the theme's `[tags]` style them (fallback up slashes, `scope`, folding); `attention` and `quiet` lists say how they behave, add up along the chain, and cover a tag's children. Replace every `[[highlight]]` example with the new form. `quiet` keeps its current explanation (not unread, no `▼ new`, no badge, no notification, wins over attention).
- A world or character can carry `[palette]` and `[tags]`, over its theme. Show a world file with the spec's `beacon` example.
- `/highlight <text>` adds a `[[classify]]` rule tagging matches `highlight`, and the theme styles it.
- A **Core tags** subsection for pack authors, with the spec's table (`page`, `page/in`, `page/out`, `whisper`, …, `self`) and the sentence about `say` and `pose` not being core.
- The Themes section's style-field sentence mentions that `[tags]` styles also take `scope`, and its example gains a `[tags]` table.

- [ ] **Step 6: Commit**

```bash
git add internal/theme/default.toml internal/config README.md internal/ui/browse_test.go
git commit -m "feat: the fuzzball pack and default theme in the new form; docs for tags"
```

---

### Task 7: Rewrite Indi's config

Not code, and not run by a subagent. Done in the main session with Indi watching, after Tasks 1–6 are merged. Nothing is written without Indi seeing the diff first.

**Files (outside the repo):** `~/.config/kiln/packs/default.toml`, `packs/fuzzball.toml`, `packs/prepends.toml`, and a new `themes/default.toml`.

- [ ] **Step 1: Back up**

```bash
cp -R ~/.config/kiln ~/.config/kiln.bak-$(date +%Y%m%d-%H%M)
```

- [ ] **Step 2: Convert each `[[highlight]]`**

For each block: its `match.tags` (or a new classify tag, if it matches by `pattern`) gets a `[tags]` entry in `themes/default.toml` (`extends = "default"`) with its `style` and `scope`. Where it has `attention = true` or `quiet = true`, the tag goes into the pack's `attention` or `quiet` list. Then delete the block. Known today:

- `packs/default.toml`: `self`, bold, attention. The built-in theme already styles `self` bold, so this becomes just `attention = ["self"]`.
- `packs/prepends.toml`: `page` (`#2053ff`, bold, match, attention), `watchfor` (`#de4400`, bold, match), `whisper` (`#00a5d9`, bold, match, attention), `system` (`#02a700`, bold). These become `[tags]` entries in `themes/default.toml` and `attention = ["page", "whisper"]` in the pack.
- `packs/fuzzball.toml`: replace it with the new embedded pack if Indi hasn't customized it (`diff` it against the old embedded version from git first). Otherwise convert its blocks like the others.

- [ ] **Step 3: Show the diff, then load it**

`diff -ru ~/.config/kiln.bak-* ~/.config/kiln`, reviewed with Indi. Then check it loads with a scratch data dir: `XDG_DATA_HOME=$(mktemp -d) go run ./cmd/kiln --check` if a check flag exists; otherwise `go test`-style: a tiny `go run` that calls `config.Load(os.ExpandEnv("$HOME/.config/kiln"))` and `theme.Load(...)` and prints errors. Indi syncs the files to the remote themselves.

---

## Self-review notes

- **Spec coverage:** `[tags]` and `scope` (Task 1), fallback up slashes (Task 1), folding order (Task 3), world/character `[palette]`/`[tags]` (Tasks 2, 4, 5), attention/quiet lists (Tasks 3, 4), the fuzzball pack (Task 6), `/highlight` (Tasks 4, 6), `HighlightStyle` gone (Task 4), core tags documented (Task 6), no shim (Task 4), live restyle (Task 5). The world/character `[ui]` table is deferred to #89 on purpose (see Global Constraints).
- **Folding within layers:** the spec says a world's `[tags]` "come after the theme's, and later colors win while attributes add up". Layers merge field by field, which gives exactly that, plus the ability to switch an attribute off with `false`.
