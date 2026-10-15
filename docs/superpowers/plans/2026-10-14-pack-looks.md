# Pack Looks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A pack carries `[palette]` and `[tags]` for its own tags, layered under the theme; the fuzzball pack takes over the page and whisper looks from the built-in theme.

**Architecture:** A theme is a chain of parsed files merged base first (`internal/theme/theme.go` `build`). Pack looks become files *prepended* to that chain, world and character looks stay appended. `Theme.Stack(below, above)` builds that chain; `Theme.Best` falls back to fewer layers when some don't resolve. `config.Character` gains `PackLooks` beside `Looks`, and the three places that build a character's theme (TUI, `kiln` render, web API) call `Best`.

**Tech Stack:** Go, BurntSushi TOML, the existing theme/config/rules packages.

**Spec:** `docs/superpowers/specs/2026-10-14-pack-looks-design.md`

## Global Constraints

- Order, lowest first: packs (in `use` order) → theme (built-in, then the person's) → world → character. Palette names merge in the same order.
- Pack looks use world-file syntax: `[palette]`, `[palette.light]`, `[palette.dark]`, `[tags]`; colors `#rrggbb`, a palette name, a terminal color, or `default`.
- Lines look the same as before for a character using the fuzzball pack: the TUI goldens (`internal/ui/testdata/golden`) must pass unchanged, never `-update`d.
- The built-in theme's `[tags]` keeps only `self` and `highlight`.
- Pre-1.0: clean break, no migration for old `packs/fuzzball.toml` files.
- Strings: no new user-facing strings expected; any that appear go in `internal/str/locales/en.toml` with a `Strings:` trailer.
- Names in tests: Kit, Rook, Ash; world `fm`. (Existing tests use "Mira" in server lines; leave those.)
- Indi's uncommitted edits to `internal/config/defaults/packs/fuzzball.toml` (system and ooc rules, comments removed) are his: never overwrite or revert them. Task 4 appends below them.

## Review Focus

1. A pack whose look names a color nobody defines: loading still works, the error names `packs/<id>.toml`, and the character draws with the theme alone (Task 1 `TestBestFallsBack`, Task 2 `TestPackBadColorNamesThePack`).
2. A world look that doesn't resolve on a character whose pack's does: the pack's looks stay (pages keep their color), and the error is still reported (Task 1 `TestBestFallsBack`, Task 3 `TestBadWorldLookKeepsPackLooks`).
3. Two packs styling the same tag: the later one in `use` wins (Task 1 `TestStackTwoPacksLaterWins`).
4. A theme defining a palette name a pack also defines: the theme's color is used by the pack's tags (Task 1 `TestStackOrder`).
5. Light appearance on the web: a pack's `[palette.light]` reaches the `Theme` event's `Light` looks for that character (Task 3 `TestPackLooksReachTheWeb`).

---

### Task 1: `Theme.Stack` and `Theme.Best`

**Files:**
- Modify: `internal/theme/theme.go` (replace `With`, ~line 229)
- Test: `internal/theme/theme_test.go`

**Interfaces:**
- Produces: `func (t *Theme) Stack(below, above []Layer) (*Theme, error)`; `func (t *Theme) Best(below, above []Layer) (*Theme, error)`. `With` is removed.

- [ ] **Step 1: Write the failing tests** (append to `internal/theme/theme_test.go`, package `theme`)

```go
// layer is a Layer from TOML tables, as a world or pack file gives them.
func layer(t *testing.T, where, src string) Layer {
	t.Helper()
	var raw map[string]any
	if _, err := toml.Decode(src, &raw); err != nil {
		t.Fatal(err)
	}
	p, _ := raw["palette"].(map[string]any)
	tg, _ := raw["tags"].(map[string]any)
	l, err := ParseLayer(where, p, tg)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func tagSGR(t *testing.T, th *Theme, tag string) string {
	t.Helper()
	_, ts, _ := th.Tag(tag)
	return ts.Style.SGR()
}

// Packs go under the theme, worlds over it; palette names resolve
// across all of them, the theme's beating a pack's. (say: a tag the
// built-in theme doesn't style.)
func TestStackOrder(t *testing.T) {
	pack := layer(t, "packs/p.toml", "[palette]\nsea = \"#111111\"\n[tags]\nooc = { fg = \"sea\", bold = true }\nsay = { fg = \"#222222\" }\n")
	world := layer(t, "worlds/fm.toml", "[tags]\nsay = { fg = \"#333333\" }\n")
	base, err := FromTOML("[palette]\nsea = \"#444444\"\n[tags]\nsay = { fg = \"#555555\", italic = true }\n")
	if err != nil {
		t.Fatal(err)
	}
	th, err := base.Stack([]Layer{pack}, []Layer{world})
	if err != nil {
		t.Fatal(err)
	}
	if got := tagSGR(t, th, "ooc"); got != "\x1b[1;38;2;68;68;68m" {
		t.Errorf("ooc = %q: the pack's tag, in the theme's sea", got)
	}
	if got := tagSGR(t, th, "say"); got != "\x1b[3;38;2;51;51;51m" {
		t.Errorf("say = %q: the world's color over the theme's italic", got)
	}
	th, err = base.Stack([]Layer{pack}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := tagSGR(t, th, "say"); got != "\x1b[3;38;2;85;85;85m" {
		t.Errorf("say = %q: the theme's beats the pack's", got)
	}
}

func TestStackTwoPacksLaterWins(t *testing.T) {
	a := layer(t, "packs/a.toml", "[tags]\nooc = { fg = \"#111111\", bold = true }\n")
	b := layer(t, "packs/b.toml", "[tags]\nooc = { fg = \"#222222\" }\n")
	th, err := Builtin().Stack([]Layer{a, b}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := tagSGR(t, th, "ooc"); got != "\x1b[1;38;2;34;34;34m" {
		t.Errorf("ooc = %q: b's color, a's bold", got)
	}
}

// A pack's [palette.light] applies on a light background.
func TestStackPackLightPalette(t *testing.T) {
	pack := layer(t, "packs/p.toml", "[palette]\nsea = \"#111111\"\n[palette.light]\nsea = \"#0a0b0c\"\n[tags]\nooc = { fg = \"sea\" }\n")
	th, err := BuiltinFor(Light).Stack([]Layer{pack}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := tagSGR(t, th, "ooc"); got != "\x1b[38;2;10;11;12m" {
		t.Errorf("light ooc = %q", got)
	}
}

// Best keeps the most that resolves: everything, else the packs' looks,
// else the theme alone; the error says what didn't.
func TestBestFallsBack(t *testing.T) {
	good := layer(t, "packs/p.toml", "[tags]\nooc = { fg = \"#111111\" }\n")
	badPack := layer(t, "packs/q.toml", "[tags]\nooc = { fg = \"nowhere\" }\n")
	badWorld := layer(t, "worlds/fm.toml", "[tags]\npage = { fg = \"nowhere\" }\n")
	b := Builtin()
	th, err := b.Best([]Layer{good}, []Layer{badWorld})
	if err == nil || tagSGR(t, th, "ooc") != "\x1b[38;2;17;17;17m" {
		t.Errorf("a bad world look: err %v, ooc %q; want the pack's looks kept and an error", err, tagSGR(t, th, "ooc"))
	}
	th, err = b.Best([]Layer{badPack}, nil)
	if err == nil || th != b {
		t.Errorf("a bad pack look: err %v; want the theme itself and an error", err)
	}
	if th, err := b.Best(nil, nil); err != nil || th != b {
		t.Error("no layers should be the theme itself")
	}
}
```

Add `"github.com/BurntSushi/toml"` to the test file's imports if it isn't there.

- [ ] **Step 2: Run to see them fail**

Run: `go test ./internal/theme -run 'TestStack|TestBest'`
Expected: build failure, `base.Stack undefined`.

- [ ] **Step 3: Implement** — replace `With` in `internal/theme/theme.go`:

```go
// Layer is a pack's, world's or character's own [palette] and [tags]:
// a pack's go under the theme it's used with, a world's and character's
// over it (see Theme.Stack).
type Layer struct{ f file }
```

(update the existing `Layer` comment to that), and replace the `With` function with:

```go
// Stack is t with below under it and above on top, each in order,
// merged field by field: the packs' looks under a theme, a world's and
// a character's over it. Palette names resolve across all of them.
func (t *Theme) Stack(below, above []Layer) (*Theme, error) {
	if len(below) == 0 && len(above) == 0 {
		return t, nil
	}
	chain := make([]file, 0, len(below)+len(t.chain)+len(above))
	for _, l := range below {
		chain = append(chain, l.f)
	}
	chain = append(chain, t.chain...)
	for _, l := range above {
		chain = append(chain, l.f)
	}
	return build(chain, t.ap)
}

// Best is Stack, or where that doesn't resolve (a color nobody
// defines), t with below alone, or t itself: the most that does. err
// says why the whole stack didn't.
func (t *Theme) Best(below, above []Layer) (*Theme, error) {
	th, err := t.Stack(below, above)
	if err == nil {
		return th, nil
	}
	if len(above) > 0 {
		if th, berr := t.Stack(below, nil); berr == nil {
			return th, err
		}
	}
	return t, err
}
```

- [ ] **Step 4: Fix callers so the tree builds.** `With` had three callers; change each to `Stack(nil, <layers>)` for now (Task 3 moves them to `Best`):
  - `internal/ui/model.go:322`: `theme.Active().Stack(nil, cs.Ch.Looks)`
  - `cmd/kiln/render.go:38`: `theme.Active().Stack(nil, ch.Looks)`
  - `internal/webapi/theme.go:78-79`: `lk.dark.Stack(nil, ch.Looks)`, `lk.light.Stack(nil, ch.Looks)`
  - and any test calling `.With(` (`grep -rn '\.With(' internal cmd web`): `.With(ls...)` → `.Stack(nil, ls)`.

- [ ] **Step 5: Run**

Run: `go vet ./... && go test ./internal/theme ./internal/ui ./internal/webapi ./cmd/kiln`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/theme internal/ui internal/webapi cmd/kiln
git commit -m "theme: Stack puts layers under a theme as well as over it; Best falls back"
```

---

### Task 2: Packs carry looks

**Files:**
- Modify: `internal/config/config.go` (`Character` ~line 78, `Load` ~232, `loadWorld`/`loadWorldData` ~326-420, `loadPack` ~426, `decodeBytes` ~480)
- Modify: `internal/config/edit.go:519` (the `loadWorldData` call)
- Test: `internal/config/pack_test.go`

**Interfaces:**
- Consumes: `theme.ParseLayer`, `theme.Layer`, `(*theme.Theme).Stack` (Task 1).
- Produces: `config.Character.PackLooks []theme.Layer` — the looks of the packs the character's world uses, in `use` order, to go under the theme.

- [ ] **Step 1: Write the failing tests** (append to `internal/config/pack_test.go`)

```go
// packWorld writes a pack named extra and a world fm using it, and
// returns Kit.
func packWorld(t *testing.T, pack string) config.Character {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{"worlds", "packs"} {
		os.MkdirAll(filepath.Join(dir, sub), 0o700)
	}
	os.WriteFile(filepath.Join(dir, "packs", "extra.toml"), []byte(pack), 0o600)
	os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte("host = \"h\"\nport = 1\nuse = [\"extra\"]\n[[characters]]\nname = \"Kit\"\n"), 0o600)
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, _ := cfg.Find("fm", "Kit")
	return kit
}

// A pack's [palette] and [tags] go under the theme, light variants and all.
func TestPackLooks(t *testing.T) {
	kit := packWorld(t, "[palette]\nsea = \"#123456\"\n[palette.light]\nsea = \"#0a0b0c\"\n[tags]\n\"ooc\" = { fg = \"sea\" }\n\n[[classify]]\ntag = \"ooc\"\npattern = '^OOC'\n")
	if len(kit.PackLooks) != 1 || len(kit.Looks) != 0 {
		t.Fatalf("pack looks %d, own looks %d", len(kit.PackLooks), len(kit.Looks))
	}
	for _, c := range []struct {
		th   *theme.Theme
		want string
	}{{theme.Builtin(), "\x1b[38;2;18;52;86m"}, {theme.BuiltinFor(theme.Light), "\x1b[38;2;10;11;12m"}} {
		th, err := c.th.Stack(kit.PackLooks, kit.Looks)
		if err != nil {
			t.Fatal(err)
		}
		if _, ts, _ := th.Tag("ooc"); ts.Style.SGR() != c.want {
			t.Errorf("ooc = %q, want %q", ts.Style.SGR(), c.want)
		}
	}
}

// A pack's look that names no color loads, and says which pack when it's
// put on a theme.
func TestPackBadColorNamesThePack(t *testing.T) {
	kit := packWorld(t, "[tags]\n\"ooc\" = { fg = \"nowhere\" }\n")
	_, err := theme.Builtin().Stack(kit.PackLooks, nil)
	want := str.ThemeBadColor(filepath.Join("packs", "extra.toml"), str.ThemeTagEntry("ooc"), "nowhere")
	if err == nil || err.Error() != want {
		t.Errorf("err %v, want %q", err, want)
	}
}

// A pack still rejects keys that aren't rules or looks.
func TestPackUnknownKey(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "worlds"), 0o700)
	os.MkdirAll(filepath.Join(dir, "packs"), 0o700)
	os.WriteFile(filepath.Join(dir, "packs", "extra.toml"), []byte("[frob]\nx = 1\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte("host = \"h\"\nport = 1\nuse = [\"extra\"]\n[[characters]]\nname = \"Kit\"\n"), 0o600)
	_, err := config.Load(dir)
	if err == nil || !strings.Contains(err.Error(), str.ConfigUnknownKey("extra.toml", "frob")) {
		t.Errorf("err %v", err)
	}
}
```

Add `"github.com/latrani/Kiln/internal/str"` to the imports. If `str.ThemeBadColor`'s or `str.ConfigUnknownKey`'s argument order differs from the above, follow its doc comment in `internal/str/keys_gen.go`.

- [ ] **Step 2: Run to see them fail**

Run: `go test ./internal/config -run 'TestPack'`
Expected: build failure, `kit.PackLooks undefined`.

- [ ] **Step 3: Implement** in `internal/config/config.go`:

`Character`, next to `Looks`:

```go
	PackLooks    []theme.Layer // the [palette] and [tags] of the packs its world uses, in use order: under the theme
	Looks        []theme.Layer // the world's and then the character's own [palette] and [tags]: over the theme
```

A pack file and what it loads to:

```go
// packFile is a pack as written: rules, and looks for its tags.
type packFile struct {
	Rules
	Palette map[string]any `toml:"palette"`
	Tags    map[string]any `toml:"tags"`
}

// pack is a loaded pack: its rules, and its looks if it has any.
type pack struct {
	rules Rules
	looks []theme.Layer
}
```

`loadPack` returns a `pack`, caching by id:

```go
func loadPack(dir, id string, cache map[string]pack) (pack, error) {
	if p, ok := cache[id]; ok {
		return p, nil
	}
	if !idRE.MatchString(id) {
		return pack{}, errors.New(str.ConfigBadPackId(id))
	}
	rel := filepath.Join("packs", id+".toml")
	path := filepath.Join(dir, rel)
	if _, err := os.Stat(path); err != nil {
		return pack{}, errors.New(str.ConfigUnknownPack(id))
	}
	var pf packFile
	if err := decodeFile(path, &pf, false); err != nil {
		return pack{}, err
	}
	p := pack{rules: pf.Rules}
	if pf.Palette != nil || pf.Tags != nil {
		l, err := theme.ParseLayer(rel, pf.Palette, pf.Tags)
		if err != nil {
			return pack{}, err
		}
		p.looks = []theme.Layer{l}
	}
	cache[id] = p
	return p, nil
}
```

`decodeBytes`: packs' looks decode like a world's:

```go
	_, world := v.(*worldFile)
	_, pk := v.(*packFile) // worlds, their characters and packs have looks
	for _, k := range md.Undecoded() {
		if !(world || pk) || !inLooks(k) {
```

(update `inLooks`'s comment: "a world's, character's or pack's [tags] or [palette]").

In `loadWorldData`: `packs map[string]Rules` → `packs map[string]pack`, and the `use` loop:

```go
	worldRules := baseRules
	var packLooks []theme.Layer
	for _, p := range wf.Use {
		pk, err := loadPack(dir, p, packs)
		if err != nil {
			return World{}, fmt.Errorf("%s: %w", rel, err)
		}
		worldRules = appendRules(worldRules, pk.rules)
		packLooks = append(packLooks, pk.looks...)
	}
```

and in the `Character{...}` literal add `PackLooks: packLooks,`. Change `loadWorld`'s parameter type and `Load`'s `packs := map[string]Rules{}` to `map[string]pack{}`, and `internal/config/edit.go:519`'s `map[string]Rules{}` to `map[string]pack{}`.

- [ ] **Step 4: Run**

Run: `go vet ./internal/config && go test ./internal/config`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config
git commit -m "config: a pack carries [palette] and [tags] for its tags, under the theme"
```

---

### Task 3: Characters draw with their packs' looks

**Files:**
- Modify: `internal/ui/model.go` (`styleInputs` ~307, `compileLook` ~320)
- Modify: `cmd/kiln/render.go` (`lookTheme` ~36)
- Modify: `internal/webapi/theme.go` (`looks.load` loop ~72-95, the `Looks.Chars` comment ~23)
- Test: `internal/ui/theme_test.go`, `internal/webapi/theme_test.go`

**Interfaces:**
- Consumes: `Character.PackLooks` (Task 2), `(*theme.Theme).Best` (Task 1).

- [ ] **Step 1: Write the failing tests**

`internal/ui/theme_test.go` (package `ui`; `newHarness`, `fmWorld`, `h.show`, `h.drawn` exist):

```go
// writePack writes packs/<id>.toml into the harness's config dir.
func writePack(t *testing.T, h *harness, id, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(h.dir, "packs", id+".toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A pack's looks style its lines; a world look that doesn't resolve is
// reported and leaves them.
func TestBadWorldLookKeepsPackLooks(t *testing.T) {
	world := strings.Replace(fmWorld, `use = ["fuzzball"]`, `use = ["fuzzball", "extra"]`, 1) + "\n[tags]\n\"page/in\" = { fg = \"nowhere\" }\n"
	h := newHarness(t, map[string]string{"fm": world})
	writePack(t, h, "extra", "[tags]\n\"ooc\" = { fg = \"#0a0b0c\" }\n\n[[classify]]\ntag = \"ooc\"\npattern = '^OOC'\n")
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	if !strings.Contains(h.m.a.Status().Text, str.ThemeBadColor(filepath.Join("worlds", "fm.toml"), str.ThemeTagEntry("page/in"), "nowhere")) {
		t.Errorf("status = %q, want the bad color reported", h.m.a.Status().Text)
	}
	h.show("OOC: brb")
	if !strings.Contains(h.drawn(), "\x1b[38;2;10;11;12mOOC: brb") {
		t.Error("the pack's ooc look should stay when the world's doesn't resolve")
	}
}
```

If the harness exposes its config dir under another name than `h.dir`, use that (see `writeUserTheme` in the same file for how it writes into the config dir). If `newHarness` writes worlds before packs exist, write the pack before `h.init()` as above; if `EnsureDefaults` runs inside `newHarness`, the `packs` dir already exists.

`internal/webapi/theme_test.go` (package `webapi`; `newApp`, `startThemed`, `evLooks`, `writeFile` exist):

```go
// A character whose pack has looks gets its own tag table, light
// variants included.
func TestPackLooksReachTheWeb(t *testing.T) {
	a, dir := newApp(t)
	writeFile(t, filepath.Join(dir, "packs", "extra.toml"), "[palette]\nsea = \"#123456\"\n[palette.light]\nsea = \"#0a0b0c\"\n[tags]\n\"ooc\" = { fg = \"sea\" }\n")
	writeFile(t, filepath.Join(dir, "worlds", "fm.toml"), strings.Replace(kitLooks, `use = ["fuzzball"]`, `use = ["fuzzball", "extra"]`, 1))
	if _, ok := a.Reload(); !ok {
		t.Fatal(a.Status().Text)
	}
	h := startThemed(t, a, dir)
	ev := event(h.next(), "Theme")
	for ap, want := range map[string]string{"Dark": "#123456", "Light": "#0a0b0c"} {
		chars, _ := evLooks(t, ev, ap)["Chars"].(map[string]any)
		rook, _ := chars["fm/rook"].(map[string]any)
		if rook == nil || rook["ooc"].(map[string]any)["FG"] != want {
			t.Errorf("%s: Rook's tags %v, want ooc %s", ap, rook, want)
		}
	}
}
```

(`kitLooks` has a `%s` for Kit's `page/in` color; replace it too: wrap the call as `strings.Replace(strings.Replace(kitLooks, "%s", "#abcdef", 1), ...)`.)

- [ ] **Step 2: Run to see them fail**

Run: `go test ./internal/ui -run TestBadWorldLookKeepsPackLooks; go test ./internal/webapi -run TestPackLooksReachTheWeb`
Expected: FAIL — the ooc look missing (pack looks aren't applied yet).

- [ ] **Step 3: Implement**

`internal/ui/model.go`:

```go
// styleInputs is what a character's scrollback styling depends on: the
// rules, its packs' and own looks, and the name and aliases that
// classify its own lines.
func styleInputs(ch config.Character) any {
	return struct {
		rules     config.Rules
		packLooks []theme.Layer
		looks     []theme.Layer
		name      string
		aliases   []string
	}{ch.Rules, ch.PackLooks, ch.Looks, ch.Name, ch.Aliases}
}

// compileLook builds cs's highlighter from the active theme with the
// character's packs' looks under it and its own on top. Looks that
// don't resolve leave the most that does (theme.Best); either way the
// error is returned for the caller to show.
func (cs *charState) compileLook() error {
	th, err := theme.Active().Best(cs.Ch.PackLooks, cs.Ch.Looks)
	cs.hl = rules.New(th)
	return err
}
```

`cmd/kiln/render.go`:

```go
// lookTheme is the active theme with ch's packs' looks under it and its
// own on top. Looks that don't resolve are reported to w, and the most
// that does stands in.
func lookTheme(ch config.Character, w io.Writer) *theme.Theme {
	th, err := theme.Active().Best(ch.PackLooks, ch.Looks)
	if err != nil {
		fmt.Fprintln(w, theme.Paint(theme.StatusError, "* "+str.StatusCharError(ch.World+"/"+ch.ID, err)))
	}
	return th
}
```

`internal/webapi/theme.go`, the `Looks` comment: "Chars has, by character key, the tag table of each character whose packs or own [palette] or [tags] change it; the rest use Tags." The loop:

```go
	for _, ch := range chars {
		k := app.Key(ch.World, ch.ID)
		styled[k] = baseStyled
		if len(ch.PackLooks) == 0 && len(ch.Looks) == 0 {
			continue
		}
		d, derr := lk.dark.Best(ch.PackLooks, ch.Looks)
		l, lerr := lk.light.Best(ch.PackLooks, ch.Looks)
		if err := firstErr(derr, lerr); err != nil {
			lk.lookErrs[k] = err
		}
		if d == lk.dark && l == lk.light {
			continue // nothing resolved: the theme's own
		}
		lk.hl[k], styled[k] = rules.New(d), styledOf(d)
		for _, p := range []struct {
			th *theme.Theme
			to *Looks
		}{{d, &ev.Dark}, {l, &ev.Light}} {
			if p.to.Chars == nil {
				p.to.Chars = map[string]map[string]theme.TagLook{}
			}
			_, p.to.Chars[k] = p.th.Looks()
		}
	}
```

- [ ] **Step 4: Run, and update the tests whose premise changed**

Run: `go test ./internal/ui ./internal/webapi ./cmd/kiln`
Expected: the two new tests PASS. These existing ones change premise and need updating as follows (each still pins what it was about):
  - `internal/webapi/theme_test.go` `TestBadLookFallsBackAndSaysSo`: Kit's bad look now falls back to theme plus packs, so `Chars["fm/kit"]` is sent (fuzzball's looks are still the built-in theme's at this point, so the entry may equal the base). Replace the "a look that didn't resolve was sent" check with: if `chars["fm/kit"]` is present, its `page/in` has no `FG` of `"nosuchcolor"`. Keep the status and span-tag checks.
  - `TestBadWorldLookFallsBackToTheme` (`internal/ui/theme_test.go`) still passes as written; leave it.
  Any other failure here is a bug in this task, not a premise change: debug it.

- [ ] **Step 5: Commit**

```bash
git add internal/ui internal/webapi cmd/kiln
git commit -m "ui, webapi, kiln: characters draw with their packs' looks under the theme"
```

---

### Task 4: The fuzzball pack takes the page and whisper looks

**Files:**
- Modify: `internal/config/defaults/packs/fuzzball.toml` (append at the end; Indi's uncommitted edits above stay as they are)
- Modify: `internal/theme/default.toml` (lines 37-38, 69-70, 156-163)
- Modify: `internal/theme/load_test.go` (`TestBuiltinLook`, `TestBuiltinLightLook`)
- Modify: `internal/config/pack_test.go` (`TestStarterPackPages`)
- Modify: `internal/webapi/theme_test.go` (`TestCharLooksAndSpans`), `web/ui/src/testdata/fold.json` (regenerated)

- [ ] **Step 1: Move the pins first (failing tests).**

In `internal/config/pack_test.go` `TestStarterPackPages`, replace `hl := rules.New(theme.Builtin())` with Kit's own theme, and pin the looks that leave the built-in:

```go
	th, err := theme.Builtin().Stack(kit.PackLooks, kit.Looks)
	if err != nil {
		t.Fatal(err)
	}
	hl := rules.New(th)
```

and after the loop:

```go
	// The pack's looks, as the built-in theme had them: dark, then light.
	light, err := theme.BuiltinFor(theme.Light).Stack(kit.PackLooks, kit.Looks)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		th        *theme.Theme
		tag, want string
	}{
		{th, "page/in", "\x1b[1;38;2;255;159;67m"},
		{th, "whisper/in", "\x1b[3;38;2;195;155;211m"},
		{light, "page/in", "\x1b[1;38;2;194;90;10m"},
	} {
		if _, ts, _ := c.th.Tag(c.tag); ts.Style.SGR() != c.want {
			t.Errorf("%s = %q, want %q", c.tag, ts.Style.SGR(), c.want)
		}
	}
	if len(kit.PackLooks) == 0 {
		t.Error("the starter fuzzball pack has no looks")
	}
```

In `internal/theme/load_test.go`: in `TestBuiltinLook`'s tag table, remove the `page/in` and `whisper/in` rows and add after the loop:

```go
	for _, tag := range []string{"page", "whisper"} {
		if _, _, ok := b.Tag(tag); ok {
			t.Errorf("the built-in theme styles %s: that's the fuzzball pack's", tag)
		}
	}
```

In `TestBuiltinLightLook`, remove the `page/in` check.

- [ ] **Step 2: Run to see them fail**

Run: `go test ./internal/config -run TestStarterPackPages; go test ./internal/theme -run TestBuiltin`
Expected: FAIL — "the starter fuzzball pack has no looks"; "the built-in theme styles page".

- [ ] **Step 3: Move the looks.**

Append to `internal/config/defaults/packs/fuzzball.toml` (after whatever is there, one blank line first):

```toml
# How these lines look. A theme, world or character can restyle any of
# them; a tag inherits up its slashes, so page/in starts from page.
[palette]
ember    = "#ff9f43"
lavender = "#c39bd3"

# Darker, for a light background.
[palette.light]
ember    = "#c25a0a"
lavender = "#814096"

[tags]
"page"    = { fg = "ember", bold = true }
"whisper" = { fg = "lavender", italic = true }
```

In `internal/theme/default.toml`: delete the `ember` and `lavender` lines from `[palette]` and `[palette.light]`, delete the `page` and `whisper` lines from `[tags]`, and replace the comment above `[tags]` with:

```toml
# How lines with Kiln's own tags are drawn: self (lines that mention
# you) and highlight (/highlight). A pack styles the tags it gives lines
# (see packs/fuzzball.toml); a theme can restyle any tag. A tag inherits
# up its slashes, as a role does up its dots. scope = "match" styles
# only the text the tag's pattern matched.
```

- [ ] **Step 4: Run, update what this changes, regenerate the fold fixture**

Run: `go test ./internal/config ./internal/theme`
Expected: PASS.

Run: `go test ./internal/webapi -run TestFoldFixture -args -update && (cd web/ui && npm test)`
Expected: the fixture drops the built-in `whisper` look; the page's fold tests PASS.

Run: `go test ./...`
Expected: PASS, including every TUI golden **unchanged** (never `-update` them; a golden diff is a bug here). Known premise change: `internal/webapi/theme_test.go` `TestCharLooksAndSpans` checks "Rook has no looks of its own but got an entry"; Rook now has the fuzzball pack's looks. Replace that check with: Rook's entry has `page/in` without Kit's `#abcdef` (`chars["fm/rook"]["page/in"]` absent or its `FG` ≠ `"#abcdef"`), and Rook's `page` has `FG` `"#ff9f43"`.

Then: `(cd web && KILN_SMOKE=1 go test -count=1 ./smoke)`.
Expected: PASS (goldens without PNGs skip).

- [ ] **Step 5: Commit** (only the files this task touched; Indi's own rule edits stay out of this commit unless he has committed them first)

```bash
git add -p internal/config/defaults/packs/fuzzball.toml   # stage only the appended looks block
git add internal/theme internal/config/pack_test.go internal/webapi web/ui/src/testdata/fold.json
git commit -m "fuzzball pack: page and whisper looks move here from the built-in theme"
```

---

### Task 5: Docs

**Files:**
- Modify: `docs/rules.md` ("Core tags" section ~line 52, and the looks paragraph ~line 30)
- Modify: `docs/themes.md` (the paragraph at ~line 22)
- Modify: `internal/config/defaults/themes/default.toml` (header comment)

- [ ] **Step 1: `docs/rules.md`.** Replace the "Core tags" heading and its first paragraph with:

```markdown
## Tags

A pack's tags are its own: the fuzzball pack's are below, and a pack for another server can use other names (`tell` rather than `page`). Kiln itself adds two on any server: `self`, for lines that mention your character, and `highlight`, from `/highlight`.
```

Keep the table, retitled in its lead-in as the fuzzball pack's and Kiln's own, and drop "Themes can count on these tags". After the paragraph "How tags look lives in the theme's `[tags]`…", add:

```markdown
A pack can give its tags a look too, with `[palette]` (and `[palette.light]`, `[palette.dark]`) and `[tags]` as a world file has them; the fuzzball pack colors pages and whispers that way. A pack's looks go under everything else: your theme, then the world, then the character can each restyle a pack's tag, and a palette name your theme defines beats the pack's.
```

- [ ] **Step 2: `docs/themes.md`.** In the paragraph beginning "`[ui]` styles Kiln's own parts", after "`[tags]` styles lines from the server, by their [tags](rules.md)." add: "The built-in theme styles only Kiln's own tags (`self`, `highlight`); packs style theirs, and your theme can restyle any of them."

- [ ] **Step 3: `internal/config/defaults/themes/default.toml`.** Change "To see every role, tag and color the built-in sets" to "To see every role and color the built-in sets"; leave the commented `[tags] "page"` example (a theme can still restyle a pack's tag).

- [ ] **Step 4: Check and commit**

Run: `go test ./internal/config ./internal/str`
Expected: PASS.

```bash
git add docs/rules.md docs/themes.md internal/config/defaults/themes/default.toml
git commit -m "docs: packs style their own tags; the built-in theme only Kiln's"
```
