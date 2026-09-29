# Theming Phase 4: Choosing Themes, Light and Dark Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `theme` and `appearance` settings pick the theme and its light or dark palette. Auto-detection asks the terminal, at startup and on focus-in. The default glaze theme gets a light palette, and log mode's older days restyle when the theme changes mid-read (#89).

**Architecture:** `internal/theme` builds a theme *for an appearance*: each file in the chain adds its `[palette]` and then its `[palette.dark]` or `[palette.light]`. `theme.Load` takes the theme's name and the appearance. `internal/config` reads the global `theme` and `appearance`. `internal/ui` keeps the detected appearance, asks the terminal with Bubble Tea's `RequestBackgroundColor`, and reloads through the existing `loadTheme` path when the answer flips it. Log mode's `olderMsg` gets the same stale-theme check that `sbOlderMsg` already has.

**Tech Stack:** Go 1.27, Bubble Tea v2 (`charm.land/bubbletea/v2` v2.0.10: `tea.RequestBackgroundColor`, `tea.BackgroundColorMsg.IsDark()`, `tea.FocusMsg`), BurntSushi TOML, Kiln's `internal/str` catalog.

**Spec:** `docs/superpowers/specs/2026-09-28-theming-design.md` (sections: Choosing a theme, Light and dark, Phases → 4)

## Global Constraints

- **`theme` and `appearance` are global**, in config.toml only. They're not inheritable, and worlds never carry `[ui]`.
- **`theme`** defaults to `"default"`. It names `themes/<name>.toml`, and `default` falls back to the built-in when that file doesn't exist. A name that isn't a bare file name (`../x`, `x.toml`, `""`) is an error.
- **`appearance`** is `"auto"` (the default), `"dark"` or `"light"`. Anything else is a config error.
- **Palette resolution:** for each file in the `extends` chain, base first, its `[palette]` entries and then its `[palette.<appearance>]` entries. Later wins. World and character `[palette]` layers do the same, after the theme.
- **`[palette.dark]` and `[palette.light]` are the only sub-tables a `[palette]` may have.** Any other table value is `theme.palette_not_color`.
- **Detection:**
  - With `auto`, Kiln asks at startup, on every focus-in, and after every config reload.
  - Until an answer arrives, or if none ever does, it's dark.
  - A pinned `dark` or `light` never asks, and ignores any answer that arrives anyway.
  - `kiln tail` never asks: `light` means light, anything else means dark.
- **Restyling** goes through `loadTheme`, which already skips it when the new theme `Equal`s the active one. Two appearances whose palettes resolve the same really do draw the same, so `Equal` needs no appearance comparison.
- **The dark look must not change.** The golden screens stay identical in every task.
- **User-facing text goes through `internal/str`.** Tests build expected text from `str` functions. Commits that add strings carry a `Strings:` trailer. Config values (`auto`, `dark`, `light`) are vocabulary.
- Fixtures use Kit, Rook, Ash, Mira and world `fm`. Smoke tests use a scratch `XDG_CONFIG_HOME`/`XDG_DATA_HOME`.
- Commits end with the session's `Co-Authored-By` and `Claude-Session` lines.

## Review Focus

- **A terminal that never answers** (mosh) must leave Kiln dark and fully working, with no hang and no error. Pinned in Task 4 (`TestNoAnswerStaysDark`).
- **An answer arriving with a pinned appearance** must change nothing. Pinned in Task 4 (`TestPinnedAppearanceIgnoresAnswer`).
- **A focus-in whose answer matches the current appearance** must not restyle, which would drop a live selection. Pinned in Task 4 (`TestSameAnswerKeepsSelection`).
- **`theme = "nope"`** with no such file: statusline error, built-in theme, Kiln keeps working. Pinned in Task 4 (`TestMissingNamedTheme`).
- **A theme change while log mode is paging older days** must not leave that day in old colors. Pinned in Task 5 (`TestBrowseOlderDayAfterThemeChange`).

---

### Task 1: Palettes per appearance, and loading a theme by name

**Files:**
- Modify: `internal/theme/theme.go`, `internal/theme/load.go`
- Modify: `internal/str/locales/en.toml` (then `go generate ./internal/str`)
- Test: `internal/theme/theme_test.go`, `internal/theme/load_test.go`

**Interfaces:**
- Produces:
  - `type Appearance int` with `const ( Dark Appearance = iota; Light )`.
  - `func Load(dir, name string, ap Appearance) (*Theme, error)`. On any error it returns `BuiltinFor(ap)` and the error.
  - `func Builtin() *Theme` (dark, as today) and `func BuiltinFor(ap Appearance) *Theme`.
  - `func FromTOML(src string) (*Theme, error)` (dark, as today) and `func FromTOMLFor(src string, ap Appearance) (*Theme, error)`.
  - `(*Theme).With` keeps the theme's appearance.
  - `file.appearance map[Appearance]map[string]string`: the per-appearance palettes.

- [ ] **Step 1: Strings**

In `en.toml` `[theme]`, add:

```toml
bad_name = "theme {name:%q} isn't a theme name (a file name in themes/, without .toml)"
```

Run `go generate ./internal/str`.

- [ ] **Step 2: Write the failing tests**

Append to `internal/theme/theme_test.go`:

```go
func mustBuildFor(t *testing.T, ap Appearance, srcs ...string) *Theme {
	t.Helper()
	var chain []file
	for i, s := range srcs {
		f, err := parse("t"+string(rune('0'+i))+".toml", []byte(s))
		if err != nil {
			t.Fatal(err)
		}
		chain = append(chain, f)
	}
	th, err := build(chain, ap)
	if err != nil {
		t.Fatal(err)
	}
	return th
}

func TestPalettePerAppearance(t *testing.T) {
	src := `
[palette]
ink = "#aaaaaa"
[palette.light]
ink = "#111111"
[ui]
sidebar = { fg = "ink" }
[tags]
page = { fg = "ink" }
`
	dark, light := mustBuildFor(t, Dark, src), mustBuildFor(t, Light, src)
	if dark.SGR(Sidebar) != "\x1b[38;2;170;170;170m" || light.SGR(Sidebar) != "\x1b[38;2;17;17;17m" {
		t.Errorf("sidebar: dark %q, light %q", dark.SGR(Sidebar), light.SGR(Sidebar))
	}
	if _, ts, _ := light.Tag("page"); ts.Style.SGR() != "\x1b[38;2;17;17;17m" {
		t.Errorf("light page = %q", ts.Style.SGR())
	}
}

// Along extends, each file adds its [palette] then its appearance's: a
// later file's plain [palette] beats an earlier file's [palette.light].
func TestAppearancePaletteOrder(t *testing.T) {
	th := mustBuildFor(t, Light,
		"[palette]\nink = \"#aaaaaa\"\n[palette.light]\nink = \"#111111\"\n",
		"[palette]\nink = \"#222222\"\n[ui]\nsidebar = { fg = \"ink\" }\n")
	if th.SGR(Sidebar) != "\x1b[38;2;34;34;34m" {
		t.Errorf("sidebar = %q, want the later file's plain palette", th.SGR(Sidebar))
	}
}

func TestLayersFollowAppearance(t *testing.T) {
	base := mustBuildFor(t, Light, "")
	l, err := ParseLayer("worlds/fm.toml", map[string]any{"x": "#aaaaaa", "light": map[string]any{"x": "#111111"}}, map[string]any{"page": map[string]any{"fg": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	th, err := base.With(l)
	if err != nil {
		t.Fatal(err)
	}
	if _, ts, _ := th.Tag("page"); ts.Style.SGR() != "\x1b[38;2;17;17;17m" {
		t.Errorf("a light theme's layer should use its light palette: %q", ts.Style.SGR())
	}
}

func TestPaletteSubTableErrors(t *testing.T) {
	for _, body := range []string{"[palette.dusk]\nx = \"#000000\"\n", "[palette.light]\nx = 1\n"} {
		_, err := parse("t.toml", []byte(body))
		want := str.ThemePaletteNotColor("t.toml", "dusk")
		if strings.Contains(body, "light") {
			want = str.ThemePaletteNotColor("t.toml", "x")
		}
		if err == nil || err.Error() != want {
			t.Errorf("%q: err = %v, want %q", body, err, want)
		}
	}
}
```

Append to `internal/theme/load_test.go`:

```go
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
```

Existing calls change: every `Load(dir)` in `load_test.go` becomes `Load(dir, "default", Dark)`, and `mustBuild` in `theme_test.go` calls `build(chain, Dark)`.

- [ ] **Step 3: Run them to watch them fail**

Run: `go test ./internal/theme`
Expected: build failure (`Appearance`, `BuiltinFor` undefined; `build` and `Load` take the wrong number of arguments).

- [ ] **Step 4: Implement**

In `theme.go`:

```go
// Appearance is whether the terminal's background is dark or light,
// which picks a theme's [palette.dark] or [palette.light].
type Appearance int

const (
	Dark Appearance = iota
	Light
)

// appearanceKeys are the [palette] sub-tables, by appearance.
var appearanceKeys = map[string]Appearance{"dark": Dark, "light": Light} //str:ok: theme vocabulary
```

Add `appearance map[Appearance]map[string]string` to `file`, initialized in `parseTables`. The palette case becomes:

```go
		case "palette":
			tbl, ok := v.(map[string]any)
			if !ok {
				return f, errors.New(str.ThemeNotTable(name, k))
			}
			for pk, pv := range tbl {
				if sub, ok := pv.(map[string]any); ok {
					ap, known := appearanceKeys[pk]
					if !known {
						return f, errors.New(str.ThemePaletteNotColor(name, pk))
					}
					m := map[string]string{}
					for sk, sv := range sub {
						s, ok := sv.(string)
						if !ok {
							return f, errors.New(str.ThemePaletteNotColor(name, sk))
						}
						m[sk] = s
					}
					f.appearance[ap] = m
					continue
				}
				s, ok := pv.(string)
				if !ok {
					return f, errors.New(str.ThemePaletteNotColor(name, pk))
				}
				f.palette[pk] = s
			}
```

`Theme` gains `ap Appearance`. `build(chain []file, ap Appearance)` adds, per file, `f.palette` and then `f.appearance[ap]` through the same checked loop (name taken, literal color). Factor that loop into `addPalette(f.name, entries, palette) error` and call it twice. Set `t.ap = ap`. `With` calls `build(chain, t.ap)`.

In `load.go`:

```go
var builtins = func() map[Appearance]*Theme {
	f, err := parse(themePath("default"), builtinSrc)
	if err != nil {
		panic(err)
	}
	out := map[Appearance]*Theme{}
	for _, ap := range []Appearance{Dark, Light} {
		t, err := build([]file{f}, ap)
		if err != nil {
			panic(err)
		}
		out[ap] = t
	}
	return out
}()

// Builtin is the theme Kiln ships with, for a dark terminal.
func Builtin() *Theme { return builtins[Dark] }

// BuiltinFor is the theme Kiln ships with, for ap.
func BuiltinFor(ap Appearance) *Theme { return builtins[ap] }

// Load reads dir/themes/<name>.toml, following extends, for ap. With no
// such file, "default" is the built-in. On an error it returns the
// built-in theme too, so there's always something to draw with.
func Load(dir, name string, ap Appearance) (*Theme, error) {
	if !validName(name) {
		return builtins[ap], errors.New(str.ThemeBadName(name))
	}
	chain, err := chainFor(dir, name, nil)
	if err != nil {
		return builtins[ap], err
	}
	t, err := build(chain, ap)
	if err != nil {
		return builtins[ap], err
	}
	return t, nil
}
```

`active` starts as `builtins[Dark]`. `FromTOML(src)` becomes `FromTOMLFor(src, Dark)`, and `FromTOMLFor` builds `[]file{base, f}` with `ap`.

Other callers (`internal/ui/model.go`, `cmd/kiln/main.go`) call `theme.Load(dir, "default", theme.Dark)` for now; Task 4 passes the real values.

- [ ] **Step 5: Run the tests**

Run: `go build ./... && go test ./internal/theme ./internal/str && go test ./internal/ui -run TestGolden`
Expected: PASS, goldens unchanged.

- [ ] **Step 6: Commit**

```bash
git add internal/theme internal/str internal/ui/model.go cmd/kiln
git commit -m "feat(theme): palettes per appearance; load a theme by name

Strings: theme.bad_name (new)"
```

---

### Task 2: `theme` and `appearance` settings

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/str/locales/en.toml`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `Config.Theme string` (default `"default"`) and `Config.Appearance string` (`"auto"`, `"dark"` or `"light"`; default `"auto"`).

- [ ] **Step 1: Strings**

In `[config]`:

```toml
bad_appearance = 'config.toml: appearance must be "auto", "dark" or "light"'
```

Run `go generate ./internal/str`.

- [ ] **Step 2: Write the failing test**

```go
func TestThemeAndAppearance(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil || cfg.Theme != "default" || cfg.Appearance != "auto" {
		t.Fatalf("defaults: %q %q %v", cfg.Theme, cfg.Appearance, err)
	}
	write(t, dir, map[string]string{"config.toml": "theme = \"ember\"\nappearance = \"light\"\n"})
	if cfg, err = Load(dir); err != nil || cfg.Theme != "ember" || cfg.Appearance != "light" {
		t.Errorf("set: %q %q %v", cfg.Theme, cfg.Appearance, err)
	}
	write(t, dir, map[string]string{"config.toml": "appearance = \"dusk\"\n"})
	if _, err := Load(dir); err == nil || err.Error() != str.ConfigBadAppearance() {
		t.Errorf("bad appearance: %v", err)
	}
}
```

Run: `go test ./internal/config -run TestThemeAndAppearance`
Expected: build failure (`cfg.Theme` undefined).

- [ ] **Step 3: Implement**

`globalFile` gains `Theme string \`toml:"theme"\`` and `Appearance string \`toml:"appearance"\``, and `Config` gains:

```go
	Theme         string        // themes/<name>.toml; "default" for the built-in
	Appearance    string        // "auto", "dark" or "light"
```

In `Load`, after the export-format check:

```go
	themeName := g.Theme
	if themeName == "" {
		themeName = "default"
	}
	appearance := g.Appearance
	switch appearance {
	case "":
		appearance = "auto"
	case "auto", "dark", "light":
	default:
		return nil, errors.New(str.ConfigBadAppearance())
	}
```

and set both in the `Config` literal. The theme name isn't validated here: `theme.Load` reports a bad name, and a broken theme must never stop the config from loading (the same rule as today's broken `themes/default.toml`).

- [ ] **Step 4: Run and commit**

Run: `go test ./internal/config ./internal/str`
Expected: PASS.

```bash
git add internal/config internal/str
git commit -m "feat(config): theme and appearance settings

Strings: config.bad_appearance (new)"
```

---

### Task 3: The default theme's light palette

**Files:**
- Modify: `internal/theme/default.toml`
- Test: `internal/theme/load_test.go`

**Interfaces:**
- Consumes: Task 1's `BuiltinFor`.

- [ ] **Step 1: Write the failing test**

```go
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
```

Run: `go test ./internal/theme -run TestBuiltinLightLook`
Expected: FAIL (the light built-in draws the dark colors).

- [ ] **Step 2: Add the palette**

In `default.toml`, add to `[palette]` (dark values, for the over-limit mark):

```toml
alarm      = "#d92626"
alarm-deep = "#2a0909"
```

change `"input.over_limit"` to `{ fg = "alarm", bg = "alarm-deep" }`, update the header comment to mention `[palette.light]`, and add after `[palette]`:

```toml
# The same glazes for a light terminal: dark inks for text and lines,
# pale tints under chips, and tag colors dark enough for paper.
[palette.light]
celadon-deep   = "#bce6e4"
celadon-mid    = "#629391"
celadon-line   = "#74b4b2"
celadon-ink    = "#1d726f"
celadon-bright = "#125452"

cobalt-deep   = "#bccae6"
cobalt-mid    = "#627293"
cobalt-line   = "#7489b4"
cobalt-ink    = "#1d3972"
cobalt-bright = "#122854"

gold-mid  = "#938562"
gold-line = "#b4a274"
gold-ink  = "#725a1d"

kaki-deep   = "#e6cebc"
kaki-mid    = "#937762"
kaki-line   = "#b49074"
kaki-ink    = "#72421d"
kaki-bright = "#542f12"

ember    = "#c25a0a"
lavender = "#814096"
beacon   = "#a47404"

alarm      = "#a51d1d"
alarm-deep = "#f9dcdc"
```

- [ ] **Step 3: Run and commit**

Run: `go test ./internal/theme && go test ./internal/ui -run TestGolden`
Expected: PASS, goldens unchanged.

```bash
git add internal/theme/default.toml internal/theme/load_test.go
git commit -m "feat(theme): the glaze look for light terminals"
```

---

### Task 4: Choosing and detecting in the UI and `kiln tail`

**Files:**
- Modify: `internal/ui/model.go`, `cmd/kiln/main.go`
- Test: `internal/ui/theme_test.go`

**Interfaces:**
- Consumes: Task 1's `theme.Load(dir, name, ap)`, Task 2's `Config.Theme` and `Config.Appearance`.
- Produces: `Model.detected theme.Appearance` (the last answer, `theme.Dark` until one arrives) and `func (m *Model) appearance() theme.Appearance`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/ui/theme_test.go` (add `"image/color"` to the imports):

```go
var (
	lightBG = tea.BackgroundColorMsg{Color: color.RGBA{0xfb, 0xf8, 0xf1, 0xff}}
	darkBG  = tea.BackgroundColorMsg{Color: color.RGBA{0x31, 0x2a, 0x54, 0xff}}
)

func withAppearance(t *testing.T, h *harness, setting string) {
	t.Helper()
	os.WriteFile(filepath.Join(h.dir, "config.toml"), []byte("appearance = \""+setting+"\"\n"), 0o600)
	h.m.Update(reloadMsg{})
}

func TestLightAnswerRestyles(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.Update(lightBG)
	if theme.Active().SGR(theme.Sidebar) != theme.BuiltinFor(theme.Light).SGR(theme.Sidebar) {
		t.Error("a light answer should switch to the light palette")
	}
	h.m.Update(darkBG)
	if theme.Active().SGR(theme.Sidebar) != theme.Builtin().SGR(theme.Sidebar) {
		t.Error("a dark answer should switch back")
	}
}

func TestNoAnswerStaysDark(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	if theme.Active().SGR(theme.Sidebar) != theme.Builtin().SGR(theme.Sidebar) || h.m.status != "" {
		t.Errorf("with no answer: sidebar %q, status %q", theme.Active().SGR(theme.Sidebar), h.m.status)
	}
}

func TestPinnedAppearanceIgnoresAnswer(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	withAppearance(t, h, "light")
	light := theme.Active()
	if light.SGR(theme.Sidebar) != theme.BuiltinFor(theme.Light).SGR(theme.Sidebar) {
		t.Fatal("appearance = light should draw light")
	}
	h.m.Update(darkBG)
	if theme.Active() != light {
		t.Error("a pinned appearance must ignore the terminal's answer")
	}
	if cmd := h.m.Init(); cmdAsks(cmd) {
		t.Error("a pinned appearance must not ask")
	}
}

func TestSameAnswerKeepsSelection(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.show("line")
	sb := &h.m.chars["fm/kit"].sb
	sb.StartSelect(sbPos{line: 0})
	h.m.Update(darkBG) // already dark
	if sb.sel == nil {
		t.Error("an answer that doesn't change the appearance dropped the selection")
	}
}

func TestAsksOnStartAndFocus(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	if !cmdAsks(h.m.Init()) {
		t.Error("auto should ask at start")
	}
	_, cmd := h.m.Update(tea.FocusMsg{})
	if !cmdAsks(cmd) {
		t.Error("auto should ask again on focus-in")
	}
}

func TestMissingNamedTheme(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	os.WriteFile(filepath.Join(h.dir, "config.toml"), []byte("theme = \"nope\"\n"), 0o600)
	h.m.Update(reloadMsg{})
	if !strings.Contains(h.m.status, str.ThemeNoTheme("nope")) {
		t.Errorf("status = %q", h.m.status)
	}
}

// cmdAsks reports whether cmd is, or is a batch holding, a
// background-color request. It compares functions, and only ever calls
// the top-level command (a tea.Batch's function just returns its list),
// so nothing else in the batch runs.
func cmdAsks(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	want := reflect.ValueOf(tea.RequestBackgroundColor).Pointer()
	if reflect.ValueOf(cmd).Pointer() == want {
		return true
	}
	b, ok := cmd().(tea.BatchMsg)
	return ok && slices.ContainsFunc(b, func(c tea.Cmd) bool { return c != nil && reflect.ValueOf(c).Pointer() == want })
}
```

(Add `"reflect"` and `"slices"` to the test imports if they aren't there. `cmdAsks` must only be called on a command known to be `askBackground`'s result or a `tea.Batch`, as these tests do: calling any other command would run it.)

Run: `go test ./internal/ui -run 'TestLightAnswerRestyles|TestNoAnswerStaysDark|TestPinnedAppearance|TestSameAnswer|TestAsksOnStartAndFocus|TestMissingNamedTheme'`
Expected: FAIL.

- [ ] **Step 2: Implement**

In `internal/ui/model.go`:

- Add a `detected theme.Appearance` field to `Model` (commented `// the terminal's last answer; Dark until one comes`), and:

```go
// appearance is the palette to draw with: the setting, or with auto the
// terminal's last answer (dark until one comes).
func (m *Model) appearance() theme.Appearance {
	switch m.cfg.Appearance {
	case "light":
		return theme.Light
	case "dark":
		return theme.Dark
	}
	return m.detected
}

// askBackground is the command that asks the terminal for its
// background, when the appearance is auto.
func (m *Model) askBackground() tea.Cmd {
	if m.cfg.Appearance != "auto" {
		return nil
	}
	return tea.RequestBackgroundColor
}
```

- `loadTheme` calls `theme.Load(m.d.ConfigDir, m.cfg.Theme, m.appearance())`.
- `Init` appends `m.askBackground()` to its commands (a nil command is fine in `tea.Batch`).
- In `update`:

```go
	case tea.BackgroundColorMsg:
		ap := theme.Light
		if msg.IsDark() {
			ap = theme.Dark
		}
		if ap != m.detected {
			m.detected = ap
			if m.cfg.Appearance == "auto" {
				m.loadTheme()
			}
		}
```

  The `tea.FocusMsg` case ends with `return m, m.askBackground()`, and the `reloadMsg` case returns `tea.Batch(m.watch(), m.askBackground())`.

`loadTheme` already skips the restyle when the new theme `Equal`s the active one, so an answer that doesn't change anything costs nothing.

In `cmd/kiln/main.go`, `tail`:

```go
	ap := theme.Dark
	if cfg.Appearance == "light" {
		ap = theme.Light
	}
	if th, err := theme.Load(cfgDir, cfg.Theme, ap); err != nil {
```

- [ ] **Step 3: Run the whole suite**

Run: `go vet ./... && go test ./...`
Expected: PASS, goldens unchanged.

- [ ] **Step 4: Commit**

```bash
git add internal/ui cmd/kiln
git commit -m "feat(ui): choose the theme and appearance; ask the terminal on start and focus"
```

---

### Task 5: Log mode's older days follow a theme change

**Files:**
- Modify: `internal/ui/browse.go`
- Test: `internal/ui/theme_test.go`

**Interfaces:**
- Consumes: `theme.Active()`, `charState.render`.

- [ ] **Step 1: Write the failing test**

Model it on `TestOlderHistoryArrivingAfterThemeChange` (already in `theme_test.go`), which does the same for the scrollback. Open log mode with three days of logs, get the `olderMsg` a paging key returns without delivering it, change the theme with `writeUserTheme` and `reloadMsg{}` (for example `"log.day" = { fg = "#0a0b0c" }`, and a `[tags]` `"page/in"` color, with a `Mira pages: …` line in each day), then deliver the message. Every loaded line's text must match what `cs.render(l.e)` gives now.

```go
func TestBrowseOlderDayAfterThemeChange(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := goldenHarness(t)
	h.writeLog(day24, scene1...)
	h.writeLog(day24.AddDate(0, 0, -1), scene1...)
	h.writeLog(day24.AddDate(0, 0, -2), scene1...)
	h.key("ctrl+l")
	b := h.m.chars["fm/kit"].browse
	var msg olderMsg
	for i := 0; i < 50 && msg.b == nil; i++ {
		if cmd := h.press(tea.KeyPgUp, 0); cmd != nil {
			if m, ok := cmd().(olderMsg); ok {
				msg = m // read under the old theme...
			}
		}
	}
	if msg.b == nil {
		t.Fatal("no older day was read")
	}
	writeUserTheme(t, h, "extends = \"default\"\n[tags]\n\"page/in\" = { fg = \"#0a0b0c\" }\n")
	h.m.Update(reloadMsg{}) // ...the theme changes...
	h.m.Update(msg)         // ...then the day arrives
	cs := h.m.chars["fm/kit"]
	for _, l := range b.lines {
		if want, _ := cs.render(l.e); l.text != want {
			t.Fatalf("a line kept its old style: %q, want %q", l.text, want)
		}
	}
}
```

(`goldenHarness`, `writeLog`, `day24` and `scene1` are in `golden_test.go` and `browse_test.go`. If `writeLog`'s signature takes a day in a different form, follow `TestBrowsePagesOlderDaysAndDateJump` for how it writes several days. If `PgUp` doesn't page older in log mode, use the key that test uses.)

Run: `go test ./internal/ui -run TestBrowseOlderDayAfterThemeChange`
Expected: FAIL (the day arrives in the old style).

- [ ] **Step 2: Implement**

`olderMsg` gains `theme *theme.Theme // the theme active when the read started`. `requestOlder` captures `th := theme.Active()` next to `h, cls, hl, key` and sets `msg.theme = th`. `receive` restyles a stale day before prepending it:

```go
func (b *browse) receive(msg olderMsg) tea.Cmd {
	b.loading = false
	if msg.theme != theme.Active() { // rendered in a theme since replaced
		for _, l := range msg.lines {
			l.text, _ = b.cs.render(l.e)
		}
	}
	b.prepend(msg)
	...
```

- [ ] **Step 3: Run and commit**

Run: `go test ./...`
Expected: PASS.

```bash
git add internal/ui
git commit -m "fix(ui): log mode's older days follow a theme change mid-read"
```

---

### Task 6: Docs

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Write**

- The settings table under Configuration gets two rows:
  - `theme` (config.toml): which `themes/<name>.toml` to use (default `"default"`: yours if you made one, else the built-in).
  - `appearance` (config.toml): `"auto"` (default: asks the terminal), `"dark"` or `"light"`.
- The Themes section:
  - A theme can have `[palette.dark]` and `[palette.light]` on top of `[palette]`, and roles and tags follow along. Show the spec's `ember` example with a `[palette.light]` override.
  - Auto asks the terminal at start and whenever its window comes back into focus, and assumes dark until it hears back.
  - Over mosh, the terminal can't be asked (eternal and tmux pass the question on), so set `appearance` by hand on a light terminal.
  - Themes are chosen with `theme = "name"`, and making `themes/default.toml` with `extends = "default"` still works as before.
- The Rules section's paragraph on world looks mentions that a world's `[palette]` can have `[palette.light]` too.

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: choosing themes, light and dark"
```

---

## Self-review notes

- **Spec coverage:** global `theme` (Tasks 2, 4), `appearance` and detection with the focus re-ask (Tasks 2, 4), `[palette.dark]`/`[palette.light]` including layers (Task 1), the default light palette (Task 3), the mosh note (Task 6), restyling (Task 4 via `loadTheme`, Task 5 for log mode).
