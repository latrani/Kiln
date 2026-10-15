# Color Names Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Catppuccin's fourteen accents and Kiln's glazes are color names every theme has; `ember`, `lavender` and `beacon` retire; the web's terminal colors are generated from the same file.

**Architecture:** Two embedded base files, `catppuccin.toml` and `glaze.toml`, are parsed once and put under every chain `build` resolves (not stored in `Theme.chain`, so `Stack` doesn't add them twice). `default.toml` keeps only `[ui]` and `[tags]`. `catppuccin.toml`'s `[terminal]` table is read separately into `Terminal(ap)`, and a generator writes it as `web/ui/src/terminal.gen.ts`.

**Tech Stack:** Go (embed, BurntSushi TOML), TypeScript (Preact page), `go generate`.

**Spec:** `docs/superpowers/specs/2026-10-14-color-names-design.md`

## Global Constraints

- Layer order, lowest first: `catppuccin.toml` → `glaze.toml` → packs → theme → world → character; later layers redefine names.
- Dark is Macchiato, light is Latte, values from catppuccin/palette `palette.json`:
  - Macchiato: rosewater #f4dbd6, flamingo #f0c6c6, pink #f5bde6, mauve #c6a0f6, red #ed8796, maroon #ee99a0, peach #f5a97f, yellow #eed49f, green #a6da95, teal #8bd5ca, sky #91d7e3, sapphire #7dc4e4, blue #8aadf4, lavender #b7bdf8; text #cad3f5, base #24273a, surface1 #494d64, subtext1 #b8c0e0, surface2 #5b6078, subtext0 #a5adcb.
  - Latte: rosewater #dc8a78, flamingo #dd7878, pink #ea76cb, mauve #8839ef, red #d20f39, maroon #e64553, peach #fe640b, yellow #df8e1d, green #40a02b, teal #179299, sky #04a5e5, sapphire #209fb5, blue #1e66f5, lavender #7287fd; text #4c4f69, base #eff1f5, surface1 #bcc0cc, subtext1 #5c5f77, surface2 #acb0be, subtext0 #6c6f85.
- Only the fourteen accents are palette names. `[terminal]` names are not.
- `catppuccin.LICENSE` is Catppuccin's MIT license verbatim (`gh api repos/catppuccin/palette/license --jq .content | base64 -d`).
- No user-facing strings change. File names in messages are paths (`//str:ok`).
- The TUI goldens change once, on purpose (`-update`), and the commit says so.
- Pre-1.0: no aliases for `ember`, `lavender`, `beacon`.

## Review Focus

1. A theme with no `extends` names `mauve` and `celadon-ink`: both resolve, dark and light (Task 1 `TestEveryThemeHasTheNames`).
2. `Stack` on a theme twice-built (a character's theme from `Active()`): the base files aren't doubled, and a world's `mauve` still beats the base (Task 1 `TestBaseNamesRedefinable`).
3. `kiln theme show` of a one-file theme: the output loads and looks the same, and lists the base names (Task 3).
4. The page before Go runs (loading, locked): it draws with the generated terminal colors, identical to today's (Task 2's values check against today's `theme.ts`).
5. A file naming `beacon` after this: the "isn't a color" error, nothing else lost (Task 1 `TestRetiredNamesAreGone`).

---

### Task 1: The base layers, and the retired names

> **Revised (Indi, mid-run):** shipped names are namespaced by file: `catppuccin.mauve`, `glaze.celadon-ink`, `glaze.alarm`. `parseBase` prefixes each palette key with the file's namespace; `[terminal]` names accents in full (`catppuccin.red`); the built-in theme's colors gain `glaze.`, `beacon` → `catppuccin.yellow`, the pack's `page`/`whisper` → `catppuccin.peach`/`catppuccin.mauve`. Tests below name the namespaced forms. See the spec's Names section.

**Files:**
- Create: `internal/theme/catppuccin.toml`, `internal/theme/catppuccin.LICENSE`, `internal/theme/glaze.toml`
- Modify: `internal/theme/load.go` (embed and parse the bases), `internal/theme/theme.go` (`build` puts the bases under every chain; `parseTables` callers), `internal/theme/default.toml` (palette out; `beacon` → `yellow`), `internal/config/defaults/packs/fuzzball.toml` (`ember` → `peach`, `lavender` → `mauve`)
- Test: `internal/theme/load_test.go`, then the pins listed in Step 6

**Interfaces:**
- Produces: `baseFiles() []file` (catppuccin then glaze, parsed once); `parseBase(name string, src []byte) (file, map[string]any, error)` returning the file and the raw `terminal` table (nil for glaze). `build(chain, ap)` resolves `baseFiles()` + `chain`; `Theme.chain` stays `chain`.

- [ ] **Step 1: Write the failing tests** (append to `internal/theme/load_test.go`)

```go
// Every theme has the color names, dark and light, even one that
// extends nothing.
func TestEveryThemeHasTheNames(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "bare", "[tags]\na = { fg = \"mauve\" }\nb = { fg = \"celadon-ink\" }\n")
	for _, c := range []struct {
		ap     Appearance
		mauve  string
		celadon string
	}{{Dark, "\x1b[38;2;198;160;246m", "\x1b[38;2;160;227;225m"}, {Light, "\x1b[38;2;136;57;239m", "\x1b[38;2;29;114;111m"}} {
		th, err := Load(dir, "bare", c.ap)
		if err != nil {
			t.Fatal(err)
		}
		if _, ts, _ := th.Tag("a"); ts.Style.SGR() != c.mauve {
			t.Errorf("%v mauve = %q, want %q", c.ap, ts.Style.SGR(), c.mauve)
		}
		if _, ts, _ := th.Tag("b"); ts.Style.SGR() != c.celadon {
			t.Errorf("%v celadon-ink = %q, want %q", c.ap, ts.Style.SGR(), c.celadon)
		}
	}
}

// A later layer redefines a base name, and stacking a theme again
// doesn't put the bases back over it.
func TestBaseNamesRedefinable(t *testing.T) {
	th, err := FromTOML("[palette]\nmauve = \"#010203\"\n[tags]\na = { fg = \"mauve\" }\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, ts, _ := th.Tag("a"); ts.Style.SGR() != "\x1b[38;2;1;2;3m" {
		t.Errorf("theme's mauve = %q", ts.Style.SGR())
	}
	world := layer(t, "worlds/fm.toml", "[palette]\nmauve = \"#040506\"\n")
	st, err := th.Stack(nil, []Layer{world})
	if err != nil {
		t.Fatal(err)
	}
	if _, ts, _ := st.Tag("a"); ts.Style.SGR() != "\x1b[38;2;4;5;6m" {
		t.Errorf("world's mauve = %q", ts.Style.SGR())
	}
}

// ember, lavender (Kiln's) and beacon are gone: naming them is an error.
func TestRetiredNamesAreGone(t *testing.T) {
	for _, n := range []string{"ember", "beacon"} {
		if _, err := FromTOML("[tags]\na = { fg = \"" + n + "\" }\n"); err == nil {
			t.Errorf("%s still resolves", n)
		}
	}
	th, err := FromTOML("[tags]\na = { fg = \"lavender\" }\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, ts, _ := th.Tag("a"); ts.Style.SGR() != "\x1b[38;2;183;189;248m" {
		t.Errorf("lavender = %q, want Catppuccin's", ts.Style.SGR())
	}
}
```

(`layer` is the helper in `theme_test.go`; same package.)

- [ ] **Step 2: Run to see them fail**

Run: `go test ./internal/theme -run 'TestEveryThemeHasTheNames|TestBaseNamesRedefinable|TestRetiredNamesAreGone'`
Expected: FAIL — `mauve` isn't a color; `ember` still resolves.

- [ ] **Step 3: Write the base files**

`internal/theme/catppuccin.toml`:

```toml
# Catppuccin's accents, the color names every theme has: Macchiato for a
# dark background, Latte for a light one. From catppuccin/palette
# (palette.json); MIT licensed, see catppuccin.LICENSE beside this file.
# Only the accents: Catppuccin's text and surface shades assume its own
# background, and server text sits on the terminal's.

[palette]
rosewater = "#f4dbd6"
flamingo  = "#f0c6c6"
pink      = "#f5bde6"
mauve     = "#c6a0f6"
red       = "#ed8796"
maroon    = "#ee99a0"
peach     = "#f5a97f"
yellow    = "#eed49f"
green     = "#a6da95"
teal      = "#8bd5ca"
sky       = "#91d7e3"
sapphire  = "#7dc4e4"
blue      = "#8aadf4"
lavender  = "#b7bdf8"

[palette.light]
rosewater = "#dc8a78"
flamingo  = "#dd7878"
pink      = "#ea76cb"
mauve     = "#8839ef"
red       = "#d20f39"
maroon    = "#e64553"
peach     = "#fe640b"
yellow    = "#df8e1d"
green     = "#40a02b"
teal      = "#179299"
sky       = "#04a5e5"
sapphire  = "#209fb5"
blue      = "#1e66f5"
lavender  = "#7287fd"

# The web's stand-in for a terminal's own colors, which a browser
# doesn't have (Catppuccin's terminal ports). Not palette names: only
# what isn't an accent is written out; the rest name accents, and the
# brights repeat the normals.
[terminal]
fg           = "#cad3f5" # text
bg           = "#24273a" # base
black        = "#494d64" # surface1
white        = "#b8c0e0" # subtext1
bright-black = "#5b6078" # surface2
bright-white = "#a5adcb" # subtext0
red          = "red"
green        = "green"
yellow       = "yellow"
blue         = "blue"
magenta      = "pink"
cyan         = "teal"

[terminal.light]
fg           = "#4c4f69" # text
bg           = "#eff1f5" # base
black        = "#bcc0cc" # surface1
white        = "#5c5f77" # subtext1
bright-black = "#acb0be" # surface2
bright-white = "#6c6f85" # subtext0
```

(`[terminal.light]` gives only the hex six; the accent names resolve in the light palette.)

`internal/theme/catppuccin.LICENSE`: run `gh api repos/catppuccin/palette/license --jq .content | base64 -d > internal/theme/catppuccin.LICENSE`, then check it begins `MIT License` and `Copyright (c) 2021 Catppuccin`.

`internal/theme/glaze.toml`: move the glaze and `alarm` entries out of `internal/theme/default.toml`'s `[palette]` and `[palette.light]`, with their comments (the "Each part of the screen has a glaze…" paragraph and the light palette's lead comment), under this header:

```toml
# Kiln's glazes, for its own parts of the screen, and alarm: color
# names every theme has, beside Catppuccin's (catppuccin.toml).
```

Delete `ember`, `lavender` and `beacon` from `default.toml` (both palettes), leaving it no `[palette]` tables.

- [ ] **Step 4: Put the bases under every chain**

`internal/theme/load.go`:

```go
//go:embed catppuccin.toml
var catppuccinSrc []byte

//go:embed glaze.toml
var glazeSrc []byte

// bases are the color names every theme has, under whatever it's built
// from: Catppuccin's accents, then Kiln's glazes. terminal is
// catppuccin.toml's [terminal] table, for Terminal.
var bases, terminalRaw = func() ([]file, map[string]any) {
	ctp, term, err := parseBase("catppuccin.toml", catppuccinSrc) //str:ok: a path
	if err != nil {
		panic(err)
	}
	glaze, _, err := parseBase("glaze.toml", glazeSrc) //str:ok: a path
	if err != nil {
		panic(err)
	}
	return []file{ctp, glaze}, term
}()

// parseBase reads a base file: a palette, and catppuccin.toml's
// [terminal] table, returned as it's written.
func parseBase(name string, data []byte) (file, map[string]any, error) {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return file{name: name}, nil, err
	}
	term, _ := raw["terminal"].(map[string]any)
	delete(raw, "terminal")
	f, err := parseTables(name, raw)
	return f, term, err
}
```

(add the `github.com/BurntSushi/toml` import). In `internal/theme/theme.go` `build`, iterate the bases first without storing them:

```go
func build(chain []file, ap Appearance) (*Theme, error) {
	...
	for _, f := range append(slices.Clone(bases), chain...) {
```

and keep `t := &Theme{..., chain: chain, ...}` as is. Update `build`'s comment: "…for ap, over the bases (every theme's color names): each file adds…".

`internal/theme/default.toml`: `"sidebar.priority"`, `"status.presence.away"` and `"highlight"`: `fg = "beacon"` → `fg = "yellow"`. Its `[tags]` comment's "(ember, lavender)" → "(peach, mauve)". `internal/config/defaults/packs/fuzzball.toml`: `"page"` `fg = "ember"` → `"peach"`, `"whisper"` `fg = "lavender"` → `"mauve"`.

- [ ] **Step 5: Run the new tests**

Run: `go test ./internal/theme -run 'TestEveryThemeHasTheNames|TestBaseNamesRedefinable|TestRetiredNamesAreGone'`
Expected: PASS.

- [ ] **Step 6: Update the pins whose values moved, and the goldens**

Run: `go test ./... 2>&1 | grep -E '^(--- FAIL|FAIL)|_test.go:'`. Expected failures, each a pin of an old value, updated to the new one (compute SGRs as `\x1b[38;2;R;G;Bm` from the hex):
  - `internal/theme/load_test.go` `TestBuiltinLook`: `SidebarPriority` → `"\x1b[1;38;2;238;212;159m"`, `highlight` → `"\x1b[1;38;2;238;212;159m"`; the "names the colors packs use" loop: `ember`, `lavender` → `peach`, `mauve`. `TestBuiltinLightLook`: `SidebarPriority` → `"\x1b[1;38;2;223;142;29m"`.
  - `internal/config/pack_test.go` `TestStarterPackPages`: dark `page/in` → `"\x1b[1;38;2;245;169;127m"`, `whisper/in` → `"\x1b[3;38;2;198;160;246m"`, light `page/in` → `"\x1b[1;38;2;254;100;11m"`.
  - `internal/webapi/theme_test.go` `TestCharLooksAndSpans`: Rook's `page` FG `"#ff9f43"` → `"#f5a97f"`.
  - `internal/theme/show_test.go` `TestShowMergedLoadsTheSame` uses `ember` as a theme's own palette name: it defines it, so it still loads; leave it unless it fails.
  - Any other failure naming `ember`/`lavender`/`beacon` as a color: switch to `peach`/`mauve`/`yellow` with the new values. Any other failure: a bug in this task — debug it.
Then: `go test ./internal/webapi -run TestFoldFixture -args -update` (the fixture's built-in `highlight` color), `(cd web/ui && npm test)`, and the TUI goldens: `go test ./internal/ui -run TestGolden -args -update`, then `git diff --stat internal/ui/testdata/golden` and read a few diffs: only colors that were ember, lavender or beacon change.

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/theme internal/config internal/ui/testdata/golden internal/webapi web/ui/src/testdata/fold.json
git commit -m "theme: Catppuccin's accents and the glazes are names every theme has

ember, lavender and beacon retire for peach, mauve and yellow. The TUI goldens change on purpose: pages, whispers, highlights, the priority dot and the away mark take Catppuccin's colors."
```

---

### Task 2: The web's terminal colors, generated

**Files:**
- Create: `internal/theme/terminal.go`, `internal/theme/gen/main.go`, `web/ui/src/terminal.gen.ts`
- Modify: `internal/theme/load.go` (a `//go:generate` line), `web/ui/src/theme.ts` (use the generated colors)
- Test: `internal/theme/terminal_test.go`

**Interfaces:**
- Consumes: `terminalRaw` (Task 1).
- Produces: `type TerminalColors struct{ FG, BG string; ANSI [16]string }`; `func Terminal(ap Appearance) TerminalColors`; `func TerminalTS() []byte`. TS: `export const terminalDark: Palette`, `export const terminalLight: Palette` with `Palette` = `{ fg: string; bg: string; ansi: string[] }`.

- [ ] **Step 1: Write the failing tests** (`internal/theme/terminal_test.go`)

```go
package theme

import (
	"bytes"
	"os"
	"testing"
)

// The web's terminal colors are today's (Catppuccin's terminal ports):
// accents resolved, the brights repeating the normals.
func TestTerminal(t *testing.T) {
	for _, c := range []struct {
		ap   Appearance
		want TerminalColors
	}{
		{Dark, TerminalColors{FG: "#cad3f5", BG: "#24273a", ANSI: [16]string{"#494d64", "#ed8796", "#a6da95", "#eed49f", "#8aadf4", "#f5bde6", "#8bd5ca", "#b8c0e0",
			"#5b6078", "#ed8796", "#a6da95", "#eed49f", "#8aadf4", "#f5bde6", "#8bd5ca", "#a5adcb"}}},
		{Light, TerminalColors{FG: "#4c4f69", BG: "#eff1f5", ANSI: [16]string{"#bcc0cc", "#d20f39", "#40a02b", "#df8e1d", "#1e66f5", "#ea76cb", "#179299", "#5c5f77",
			"#acb0be", "#d20f39", "#40a02b", "#df8e1d", "#1e66f5", "#ea76cb", "#179299", "#6c6f85"}}},
	} {
		if got := Terminal(c.ap); got != c.want {
			t.Errorf("%v: %+v\nwant %+v", c.ap, got, c.want)
		}
	}
}

func TestTerminalGeneratedIsFresh(t *testing.T) {
	got, err := os.ReadFile("../../web/ui/src/terminal.gen.ts")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, TerminalTS()) {
		t.Error("web/ui/src/terminal.gen.ts is stale: run go generate ./internal/theme")
	}
}
```

(The expected arrays are today's `theme.ts` `macchiato` and `latte`, so the page looks the same.)

- [ ] **Step 2: Run to see them fail**

Run: `go test ./internal/theme -run TestTerminal`
Expected: build failure, `Terminal undefined`.

- [ ] **Step 3: Implement** `internal/theme/terminal.go`:

```go
package theme

import (
	"fmt"
	"strings"
)

// TerminalColors is a terminal's own colors: its foreground and
// background, and the sixteen, as #rrggbb. The web draws with them,
// having no terminal (catppuccin.toml's [terminal]).
type TerminalColors struct {
	FG, BG string
	ANSI   [16]string
}

// terminalKeys are [terminal]'s names for the sixteen's normals, in
// ANSI order; the brights 9–14 repeat 1–6.
var terminalKeys = []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"} //str:ok: file vocabulary

// Terminal is catppuccin.toml's [terminal] for ap: [terminal.light]
// over [terminal] on a light background, accent names resolved in that
// appearance's palette.
func Terminal(ap Appearance) TerminalColors {
	vals := map[string]string{}
	for k, v := range terminalRaw {
		if s, ok := v.(string); ok {
			vals[k] = s
		}
	}
	if ap == Light {
		if light, ok := terminalRaw["light"].(map[string]any); ok {
			for k, v := range light {
				vals[k] = v.(string)
			}
		}
	}
	palette := map[string]color{}
	for _, f := range bases {
		addPalette(f.name, f.palette, palette)
		addPalette(f.name, f.appearance[ap], palette)
	}
	hex := func(s string) string {
		c, ok := resolve(s, palette)
		if !ok {
			panic("catppuccin.toml [terminal]: " + s)
		}
		return c.css()
	}
	tc := TerminalColors{FG: hex(vals["fg"]), BG: hex(vals["bg"])}
	for i, k := range terminalKeys {
		tc.ANSI[i] = hex(vals[k])
		tc.ANSI[i+8] = tc.ANSI[i]
	}
	tc.ANSI[8], tc.ANSI[15] = hex(vals["bright-black"]), hex(vals["bright-white"])
	return tc
}

// TerminalTS is web/ui/src/terminal.gen.ts: Terminal for both
// appearances, for the page.
func TerminalTS() []byte {
	var b strings.Builder
	b.WriteString("// Code generated by go generate ./internal/theme from catppuccin.toml; DO NOT EDIT.\n") //str:ok: generated code
	b.WriteString("// Catppuccin (MIT): see internal/theme/catppuccin.LICENSE.\n\n")                       //str:ok: generated code
	b.WriteString("import type { Palette } from \"./theme.ts\";\n")                                           //str:ok: generated code
	for _, c := range []struct {
		name string
		ap   Appearance
	}{{"terminalDark", Dark}, {"terminalLight", Light}} {
		tc := Terminal(c.ap)
		q := make([]string, len(tc.ANSI))
		for i, a := range tc.ANSI {
			q[i] = fmt.Sprintf("%q", a)
		}
		fmt.Fprintf(&b, "\nexport const %s: Palette = {\n  fg: %q,\n  bg: %q,\n  ansi: [%s],\n};\n", c.name, tc.FG, tc.BG, strings.Join(q, ", ")) //str:ok: generated code
	}
	return []byte(b.String())
}
```

If `resolve` or `css` differ in name from the above, use the existing ones in `theme.go` (`resolve(name, palette) (color, bool)`, `color.css()`). `terminalRaw["light"]` is how TOML decodes `[terminal.light]` (a nested table under `terminal`).

`internal/theme/gen/main.go`:

```go
// Command gen writes web/ui/src/terminal.gen.ts from catppuccin.toml's
// [terminal]. Run it with go generate ./internal/theme.
package main

import (
	"fmt"
	"os"

	"github.com/latrani/Kiln/internal/theme"
)

func main() {
	if err := os.WriteFile("../../web/ui/src/terminal.gen.ts", theme.TerminalTS(), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}
```

In `internal/theme/load.go`, above the embeds: `//go:generate go run ./gen`.

Run: `go generate ./internal/theme`

- [ ] **Step 4: Use it in the page.** `web/ui/src/theme.ts`: delete the `macchiato` and `latte` constants and their comment; add

```ts
import { terminalDark, terminalLight } from "./terminal.gen.ts";

// The terminal's colors on the web: Catppuccin Macchiato (dark) and Latte
// (light), generated from internal/theme/catppuccin.toml.
export const macchiato: Palette = terminalDark;
export const latte: Palette = terminalLight;
```

(keeping the exported names, which `theme.test.ts` imports).

- [ ] **Step 5: Run**

Run: `go test ./internal/theme ./internal/str && (cd web/ui && npm run check && npm test)`
Expected: PASS (the `str` lint accepts the `//str:ok` lines; the page's theme tests see the same colors).

- [ ] **Step 6: Commit**

```bash
git add internal/theme web/ui/src/theme.ts web/ui/src/terminal.gen.ts
git commit -m "web: the terminal's colors come from catppuccin.toml, generated"
```

---

### Task 3: `theme show`, docs, release archive

**Files:**
- Modify: `internal/theme/show.go` (`Show`), `internal/theme/show_test.go`
- Modify: `docs/themes.md`, `docs/rules.md` (the "in the built-in theme's `ember` and `lavender`" sentence), `.goreleaser.yaml:27`

- [ ] **Step 1: Write the failing tests.** In `internal/theme/show_test.go`, replace `TestShowKilnIsTheSource` and `TestShowOneFileIsTheFile` with:

```go
// Show prints every theme merged with the bases under it, so the color
// names every theme has are listed; the result loads and looks the same.
func TestShowListsTheBaseNames(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "plain", "[ui]\n\"status\" = { fg = \"red\" }\n")
	for _, name := range []string{"kiln", "plain"} {
		got, err := Show(dir, name)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range []string{"mauve", "celadon-ink", "alarm"} {
			if !strings.Contains(got, "\n"+n+" ") {
				t.Errorf("Show(%s) doesn't list %s:\n%.300s", name, n, got)
			}
		}
		writeTheme(t, dir, "flat-"+name, got)
		for _, ap := range []Appearance{Dark, Light} {
			want, _ := Load(dir, name, ap)
			flat, err := Load(dir, "flat-"+name, ap)
			if err != nil {
				t.Fatalf("%v\n%s", err, got)
			}
			sameLook(t, flat, want, "self", "highlight")
		}
	}
}
```

Run: `go test ./internal/theme -run TestShow`
Expected: FAIL — `Show(kiln)` returns the source, which has no `mauve`.

- [ ] **Step 2: Implement.** In `Show`, drop the one-file shortcut and merge over the bases:

```go
	return merged(name, append(slices.Clone(bases), chain...)), nil
```

(replacing `if len(chain) == 1 { return string(chain[0].src), nil }` and `return merged(name, chain), nil`). Update `Show`'s comment: "Show is theme name as one file: the color names every theme has, then the files it's built from, merged into one that looks the same on either appearance."

Run: `go test ./internal/theme ./cmd/kiln`
Expected: PASS (fix any `cmd/kiln` test that pinned `theme show kiln` printing the source the same way: assert it lists `mauve`).

- [ ] **Step 3: Docs and release.**
  - `docs/themes.md`: after the paragraph about `[palette.light]`/`[palette.dark]`, add a "Color names" section: every theme has Catppuccin's fourteen accents (a table: name, dark, light — the values in this plan's Global Constraints) and Kiln's glazes (`celadon-*`, `cobalt-*`, `gold-*`, `kaki-*`, each `-deep`/`-mid`/`-line`/`-ink`/`-bright` where they exist, and `alarm`), whatever it extends; a theme, pack, world or character can redefine any of them; `kiln theme show` lists them. Credit: "Catppuccin's colors are MIT licensed ([catppuccin/palette](https://github.com/catppuccin/palette))." Replace any `ember` example colors in the file with `peach`.
  - `docs/rules.md`: "in the built-in theme's `ember` and `lavender`" → "in `peach` and `mauve`"; drop the sentence about a theme with no `extends` having to define the colors a pack names (every theme has them now).
  - `.goreleaser.yaml:27`: `files: [README.md, LICENSE, internal/theme/catppuccin.LICENSE]`.
  - `grep -rn 'ember\b\|"lavender"\|beacon' docs/*.md` (excluding `docs/superpowers`): update any color use left.

- [ ] **Step 4: Check and commit**

Run: `go test ./... && (cd web && go test ./...)`
Expected: PASS.

```bash
git add internal/theme docs .goreleaser.yaml cmd/kiln
git commit -m "theme show lists the color names every theme has; docs and the release carry Catppuccin's"
```
