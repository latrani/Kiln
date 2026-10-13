# Web UI, step 1: the API sends data, the page styles it — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Everything the page needs to draw Kiln comes from Go as data: lines as marked spans, the theme as data, display text for dates and times, cleaned notification text, live field checks, and every catalog string and shared glyph as a typed TypeScript module. No page yet; the TUI looks and behaves exactly as before.

**Architecture:**
- **Spans.** `webapi.Line` loses `Text` (with the server's escape codes) and gets `Plain` and `Spans`. A span is its text, the server's style (`ansi.SpanStyle`: colors as `"ansi:N"` for the terminal's sixteen or `"#rrggbb"`, plus bold/faint/italic/underline/reverse), the styled tag names over it **in the order their styles fold** (`rules.TagRuns`, which `Highlighter.Runs` is rebuilt on, so the TUI and the web can't disagree), and `Link` (the URL). `Input.Prompt` goes out as spans too.
- **Folding is the page's.** To draw a span, start from its server style and lay each named tag's style on top, in order: a set color replaces, flags add up (`theme.Style.Over`). A test does that fold in Go and checks it gives, byte for byte, what the TUI paints.
- **`Theme` event (webapi).** Every role's and tag's style as data (`theme.Look`), for dark and light, plus each character's own tag table where its `[palette]`/`[tags]` change it, plus the `appearance` setting. Sent after `Start` and whenever the config changes. The loop keeps a highlighter per character (built on the dark theme: which tags have styles, and their scope, don't depend on the appearance).
- **Display text.** Lines get `DayLabel` and `Clock`; a `history_end` line's span text is its catalog message; a `day` line's is its label. `Char` gets `ConnectedSince` and `LastSeen`. The TUI's formatting moves into `internal/app` (`TimeOfDay`, `Clock`, `ConnectedSince`) and both front ends call it.
- **Links** (`urlRE`, `trimURL`) move from `internal/ui/sbmouse.go` to `internal/app` (`app.Links`).
- **Notify:** the `Notify` effect goes to the page with `Body` cleaned (`notify.Clean`, split out of `notify.Message`).
- **`check(field, value)`:** `app.CheckField` (the `Accept*` filter for that field) and `webapi.CheckField`, exported from the wasm as `kiln.check`.
- **Glyphs** move into a new leaf package, `internal/glyph`; the TUI uses its constants (no visual change).
- **TypeScript strings.** `internal/str/gen` also writes `web/ui/src/str.gen.ts`: a function per catalog entry (same parameter order as Go's), plurals by `Intl.PluralRules`, and `glyph`. `web/static/strings.json` stays until step 2's page replaces `kiln.js`.
- **Time zone.** The wasm takes the browser's IANA zone (`Intl…resolvedOptions().timeZone`) with embedded tzdata, so days and times are right across daylight-saving changes (Go's wasm otherwise fixes the offset at start-up).

**Tech Stack:** Go; Node's built-in TypeScript type stripping for the generated module's test (`node --test`, Node ≥ 22.18).

**Spec:** `docs/superpowers/specs/2026-10-12-web-ui-design.md`, "What changes in the API", "Scrollback lines" (spans and `textContent`), "Accessibility" (attention lines). Issue #141.

**Rulings (deviations from the spec, for review):**
1. **Tag order is per span, not per stylesheet.** The spec has one CSS rule per tag and the cascade layering them. But the TUI folds tags in each line's own order (whole-line styles first, then match-scope ones), which a fixed stylesheet can't follow, and a character's own looks give it its own tag table. So Go sends each span's styled tag names in fold order, and the page folds their styles (from the `Theme` event) over the server style. Light/dark still needs no round trip (the page holds both), and a theme change still resends no lines.
2. **The TS module has every catalog entry**, not just `[web]`: the page shows many strings the TUI does (form labels, hints, state names, plurals). esbuild drops the unused ones.
3. **The browser's time zone** is new here (not in the spec). Without it, display text for older days is off by an hour across a daylight-saving change.
4. **#141's JSON items are already done** (part 2, step 4a: names for `Kind`/`Op`, `DisallowUnknownFields`, `TestEffectFieldsDontCollide`, `omitzero` on `ConnectedAt`). Nothing to do but tick them in #141.

## Global Constraints

- The TUI doesn't change: golden screens are NOT updated; existing tests change only where a moved function is called from its new home.
- Strings: every person-facing string from the catalog (`internal/str/locales/en.toml`); no catalog changes are expected in this step. If one is needed, add it, run `go generate ./internal/str`, and add a `Strings:` trailer to that commit.
- Colors as data are exactly `""`, `"#rrggbb"` (lowercase) or `"ansi:N"` with N in 0–15, in `ansi.SpanStyle` and `theme.Look` alike.
- Offsets into a line's plain text are bytes, always on rune boundaries.
- Before each commit: `go vet ./... && go test ./...`, `(cd web && go vet ./... && go test ./... && GOOS=js GOARCH=wasm go vet ./...)`, `node --test web/static/*.test.mjs web/ui/src/*.test.ts`, and `web/build.sh`. No `-race`.
- End each commit with the session's attribution lines.

## Review Focus

1. **A server reset inside a tag's run** (`\x1b[0;31m` mid-match): the web's folded style must match what the TUI paints on both sides of it. → Task 7's equivalence cases.
2. **Multi-byte text at a boundary** (an accent or emoji right before a tag match, a link, or a color change): spans split on rune boundaries and `Plain` equals the joined span texts. → Task 7's `café ☕ Kit` case and `TestSpansJoinToPlainAndMarkLinks`.
3. **A character whose own looks don't resolve** (its `[tags]` names an undefined palette color): its spans use the theme's own tags, its `Theme` entry is left out, and the status line says why, once, as the TUI does. → Task 8's `TestBadLookFallsBackAndSaysSo`.
4. **A broken theme file**: at start, the built-in theme and a status error; after a config change, the previous theme stays and the status says why. → Task 8's `TestBrokenThemeKeepsWhatItHad`.
5. **Escape-code parsing that disagrees with `ansi.Strip`** (a malformed or non-SGR sequence): tag spans index `ansi.Strip`'s plain text, so `ansi.Spans`' text must join to exactly that, or tags land on the wrong bytes. → Task 1's `TestSpansJoinToStrip`.

---

### Task 1: `ansi.Spans` keeps the sixteen, faint and reverse

**Files:** Modify `internal/ansi/spans.go`, `internal/ansi/spans_test.go`, `internal/scene/scene.go` (`spanCSS`), `internal/scene/scene_test.go` (if it pins colors)

**Interfaces:**
- Produces: `type SpanStyle struct { FG, BG string \`json:",omitempty"\`; Bold, Faint, Italic, Underline, Reverse bool \`json:",omitempty"\` }` with FG/BG `""`, `"#rrggbb"` or `"ansi:N"`; `func Hex(c string) string` (an `"ansi:N"` as xterm's hex, anything else unchanged).

- [ ] **Step 1: Update the tests.** In `TestSpans`, the 16-color cases now keep the index, and new cases cover faint, reverse and SGR 22:

```go
{"16 color", "\x1b[31mred\x1b[39m plain", []Span{{"red", SpanStyle{FG: "ansi:1"}}, {" plain", SpanStyle{}}}},
{"bright bg", "\x1b[101mx", []Span{{"x", SpanStyle{BG: "ansi:9"}}}},
{"256 below 16 is the sixteen", "\x1b[38;5;3mx", []Span{{"x", SpanStyle{FG: "ansi:3"}}}},
{"256 color", "\x1b[38;5;208mx", []Span{{"x", SpanStyle{FG: "#ff8700"}}}},
{"combined params", "\x1b[1;3;4;32mx\x1b[22;23;24mY", []Span{{"x", SpanStyle{FG: "ansi:2", Bold: true, Italic: true, Underline: true}}, {"Y", SpanStyle{FG: "ansi:2"}}}},
{"faint and reverse", "\x1b[2;7mx\x1b[27my\x1b[22mz", []Span{{"x", SpanStyle{Faint: true, Reverse: true}}, {"y", SpanStyle{Faint: true}}, {"z", SpanStyle{}}}},
{"22 ends bold and faint", "\x1b[1;2mx\x1b[22my", []Span{{"x", SpanStyle{Bold: true, Faint: true}}, {"y", SpanStyle{}}}},
```

Add:

```go
func TestHex(t *testing.T) {
	for in, want := range map[string]string{"ansi:1": "#cd0000", "ansi:15": "#ffffff", "#ff9f43": "#ff9f43", "": ""} {
		if got := Hex(in); got != want {
			t.Errorf("Hex(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSpansJoinToStrip: tags index Strip's text, so Spans must cover
// exactly the same bytes, whatever the escapes.
func TestSpansJoinToStrip(t *testing.T) {
	for _, s := range []string{"a\x1b[31mb\x1b[0mc", "a\x1b]0;t\x07b\x1b[2Jc", "x\x1b[", "\x1b[38;5mx", "café \x1b[1m☕\x1b[0m Kit", "a\x1bPdcs\x1b\\b"} {
		var b strings.Builder
		for _, sp := range Spans(s) {
			b.WriteString(sp.Text)
		}
		if b.String() != Strip(s) {
			t.Errorf("Spans(%q) joins to %q, Strip gives %q", s, b.String(), Strip(s))
		}
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/ansi` — FAIL (`ansi:1` vs `#cd0000`, no `Faint`, no `Hex`). If `TestSpansJoinToStrip` fails on a case, make `Spans` skip escapes exactly as `Strip` does (both should use `EscapeEnd`/`skipEscape` the same way); don't change `Strip`.

- [ ] **Step 3: Implement.** In `applySGR`: `2` sets `Faint`; `22` clears `Bold` and `Faint`; `7` sets `Reverse`, `27` clears it; 30–37/90–97/40–47/100–107 set `fmt.Sprintf("ansi:%d", i)`; `color256(n)` returns `"ansi:N"` for `n < 16`. Add json tags to `SpanStyle`. Add:

```go
// Hex is c as a hex color: one of the terminal's sixteen ("ansi:N") as
// xterm's default for it, anything else as it is.
func Hex(c string) string {
	if n, ok := strings.CutPrefix(c, "ansi:"); ok { //str:ok: a color reference
		if i, err := strconv.Atoi(n); err == nil && i >= 0 && i < 16 {
			return palette16[i]
		}
	}
	return c
}
```

- [ ] **Step 4: Keep the HTML export identical.** `scene.spanCSS` uses `ansi.Hex(s.FG)` / `ansi.Hex(s.BG)`. It ignores `Faint` and `Reverse` (as it ignored them before). Run `go test ./internal/scene ./internal/ansi` — PASS.

- [ ] **Step 5: Commit** `ansi: spans keep the terminal's sixteen by index, and faint and reverse`.

### Task 2: `rules.TagRuns`, with `Runs` built on it

**Files:** Modify `internal/rules/rules.go`, `internal/rules/rules_test.go`

**Interfaces:**
- Produces: `type TagRun struct { Start, End int; Names []string }`; `func (h *Highlighter) TagRuns(plain string, tags []classify.Tag) []TagRun` — nil when no tag on the line has a style; otherwise runs covering `[0, len(plain))` in order, each with the styled tag names (after the up-the-slashes lookup) whose styles apply there, **in fold order** (whole-line ones in tag order, then match-scope ones in tag order), adjacent runs with equal `Names` merged. `Runs` is unchanged in behavior.

- [ ] **Step 1: Write the failing test.**

```go
func TestTagRunsNameStylesInFoldOrder(t *testing.T) {
	th := mustTheme(t, `"page/in" = { fg = "#ff9f43" }
self = { scope = "match", bold = true }
url = { scope = "match", underline = true }
quiet = { faint = true }`)
	h := New(th)
	plain := "Mira pages: Kit, see https://x.test"
	got := h.TagRuns(plain, []classify.Tag{
		tag("self", classify.Span{Start: 12, End: 15}),
		tag("page/in/mira"), // falls back to page/in
		tag("url", classify.Span{Start: 21, End: 35}),
		tag("quiet"),
	})
	want := []TagRun{
		{0, 12, []string{"page/in", "quiet"}},
		{12, 15, []string{"page/in", "quiet", "self"}},
		{15, 21, []string{"page/in", "quiet"}},
		{21, 35, []string{"page/in", "quiet", "url"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("TagRuns = %+v\nwant %+v", got, want)
	}
	if h.TagRuns(plain, []classify.Tag{tag("unstyled")}) != nil {
		t.Error("a line with no styled tag should have no runs")
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/rules -run TagRuns` — FAIL (undefined).

- [ ] **Step 3: Implement.** Give `hit` a `name string` (the styled name `h.th.Tag` returns). Move the body of `Runs` up to the cut loop into `TagRuns`; per cut, collect `h.name` for each hit that is whole-line or covers the cut, in the sorted hits' order. Then:

```go
func (h *Highlighter) Runs(plain string, tags []classify.Tag) []style.Run {
	trs := h.TagRuns(plain, tags)
	if trs == nil {
		return nil
	}
	var runs []style.Run
	for _, tr := range trs {
		var st theme.Style
		for _, n := range tr.Names {
			_, ts, _ := h.th.Tag(n)
			st = st.Over(ts.Style)
		}
		run := style.Run{Start: tr.Start, End: tr.End, SGR: st.SGR()}
		if n := len(runs); n > 0 && runs[n-1].SGR == run.SGR {
			runs[n-1].End = run.End
			continue
		}
		runs = append(runs, run)
	}
	if len(runs) == 1 && runs[0].SGR == "" {
		return nil // styles that set nothing
	}
	return runs
}
```

`h.th.Tag(n)` on a styled name returns that name itself, so the lookup is exact. Doc-comment `TagRuns` with the fold-order rule (it's what the page relies on).

- [ ] **Step 4: Run** `go test ./internal/rules ./internal/ui` — PASS, goldens untouched.

- [ ] **Step 5: Commit** `rules: TagRuns names each run's styled tags in fold order; Runs folds them`.

### Task 3: links move into the core

**Files:** Create `internal/app/links.go`, `internal/app/links_test.go`; modify `internal/ui/sbmouse.go`; move `trimURL`'s tests from `internal/ui` (if any, e.g. `sbmouse_test.go`) to `links_test.go`

**Interfaces:**
- Produces: `func Links(plain string) [][2]int` — byte ranges of the web links in plain, trailing sentence punctuation and unmatched closing brackets trimmed.

- [ ] **Step 1: Write the failing test** (`internal/app/links_test.go`):

```go
func TestLinks(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"See https://kiln.test/map for the way", []string{"https://kiln.test/map"}},
		{"(https://x.test/a_(b)) and http://y.test.", []string{"https://x.test/a_(b)", "http://y.test"}},
		{"café https://x.test/☕?q=1!", []string{"https://x.test/☕?q=1"}},
		{"no links here", nil},
	} {
		var got []string
		for _, r := range Links(c.in) {
			got = append(got, c.in[r[0]:r[1]])
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("Links(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/app -run TestLinks` — FAIL (undefined).

- [ ] **Step 3: Move** `urlRE` and `trimURL` (with their comments) into `internal/app/links.go` and add:

```go
// Links is where plain's web links are, as byte ranges, each without
// trailing sentence punctuation or a closing bracket it doesn't open.
func Links(plain string) [][2]int {
	var out [][2]int
	for _, m := range urlRE.FindAllStringIndex(plain, -1) {
		out = append(out, [2]int{m[0], m[0] + len(trimURL(plain[m[0]:m[1]]))})
	}
	return out
}
```

In `sbmouse.go`'s `plainRows`: `l.links = app.Links(l.plain)`; delete `urlRE`/`trimURL` there and fix imports (`regexp` may go).

- [ ] **Step 4: Run** `go test ./internal/app ./internal/ui` — PASS.

- [ ] **Step 5: Commit** `app: finding links moves into the core`.

### Task 4: display text for times moves into the core

**Files:** Create `internal/app/clock.go`, `internal/app/clock_test.go`; modify `internal/app/app.go` (`Now`), `internal/ui/view.go` (`connectedSince`, `clock`), `internal/ui/browse.go:345`

**Interfaces:**
- Produces: `func (a *App) Now() time.Time` (the clock `Deps.Now` gives); `func TimeOfDay(t time.Time) string` (`15:04`, local); `func Clock(t, now time.Time) string` (time of day, with the day first when t isn't today); `func ConnectedSince(at, now time.Time) string` (the statusline's "Connected since …").

- [ ] **Step 1: Write the failing test** (`clock_test.go`; expected text from the catalog):

```go
func TestClockAndConnectedSince(t *testing.T) {
	now := time.Date(2026, 9, 24, 21, 30, 0, 0, time.Local)
	today := time.Date(2026, 9, 24, 21, 14, 0, 0, time.Local)
	before := time.Date(2026, 9, 23, 8, 5, 0, 0, time.Local)
	if got := TimeOfDay(today); got != "21:14" {
		t.Errorf("TimeOfDay = %q", got)
	}
	if got := Clock(today, now); got != "21:14" {
		t.Errorf("Clock(today) = %q", got)
	}
	if got, want := Clock(before, now), before.Format(str.DateDayTime()); got != want {
		t.Errorf("Clock(yesterday) = %q, want %q", got, want)
	}
	if got, want := ConnectedSince(today, now), str.ViewConnectedSince("21:14"); got != want {
		t.Errorf("ConnectedSince(today) = %q, want %q", got, want)
	}
	if got, want := ConnectedSince(before, now), str.ViewConnectedSinceDay(before.Format(str.DateDay()), "08:05"); got != want {
		t.Errorf("ConnectedSince(yesterday) = %q, want %q", got, want)
	}
}
```

(`"21:14"` here is a time layout's output, not catalog text; check `TestTestsReadTheCatalog` doesn't flag it.)

- [ ] **Step 2: Run** `go test ./internal/app -run Clock` — FAIL.

- [ ] **Step 3: Implement** by moving the bodies of `Model.connectedSince` and `Model.clock` (view.go:183, :462) into `ConnectedSince`/`Clock`, with `TimeOfDay` as `t.Local().Format("15:04") //str:ok: a time layout`. `Now` is `return a.d.Now()`. The TUI calls them: `connectedSince(cs)` → `app.ConnectedSince(cs.ConnectedAt, m.d.Now())`, `m.clock(t)` → `app.Clock(t, m.d.Now())` (delete both methods), browse.go's `l.Entry.Time.Local().Format("15:04")` → `app.TimeOfDay(l.Entry.Time)`.

- [ ] **Step 4: Run** `go test ./internal/app ./internal/ui` — PASS, goldens untouched.

- [ ] **Step 5: Commit** `app: times as people read them move into the core`.

### Task 5: `internal/glyph`

**Files:** Create `internal/glyph/glyph.go`; modify `internal/ui/keys.go`, `internal/ui/input.go` (`gutterMark`), `internal/ui/paint.go`, `internal/ui/browse.go:339`, `internal/ui/form.go:331-354`, `internal/ui/sidebar.go` (`attentionMark`, the ` × ` lead)

**Interfaces:**
- Produces (all `string` consts, each one cell wide):

```go
// Package glyph is the symbols Kiln draws, in one place so every front
// end draws the same ones. Each is one cell wide and in WGL4 where it
// can be (docs: the UI characters rubric). None is text for people.
//
//str:ok
package glyph

const (
	Echo       = "›" // before a line you sent
	Sys        = "*" // before Kiln's own lines
	Rule       = "─" // rules, and around a day's label
	Open       = "▼" // an open disclosure
	Collapsed  = "►" // a shut one
	Filtered   = "•" // a shut parent with a filter set under it
	Remove     = "×" // removes or closes what it's on
	Selected   = "▌" // a selected line in log mode
	Excluded   = "░" // a line taken out of the selection
	Attention  = "●" // something wants you
	ChoicePrev = "◄" // a choice field's arrows, while focused
	ChoiceNext = "►"
	Add        = "+" // before an add row
	Menu       = "≡" // the web's sidebar menu (part 4)
)
```

(If the `//str:ok` on the package clause isn't honored by the lint, put it on the `const` block as `roles.go` does.)

Also `var All = [][2]string{{"Echo", Echo}, {"Sys", Sys}, …}`: every constant by name, in order, for the TypeScript generator (Task 11).

- [ ] **Step 1: Write the test** (`internal/glyph/glyph_test.go`): every entry of `All` is exactly one rune and `xansi.StringWidth` 1, and `All` names every constant in `glyph.go` (parse the file with `go/parser` and compare the const names), so a new glyph can't be left out of the page.

- [ ] **Step 2: Run** `go test ./internal/glyph` — FAIL (no package), then create `glyph.go` — PASS.

- [ ] **Step 3: Point the TUI at it.** `keys.go`'s `glyphOpen`… become `= glyph.Open` etc.; `gutterMark = glyph.Echo`; paint.go `"* "` → `glyph.Sys + " "`, `"── "+…+" ──"` → `rule2 + " " + … + " " + rule2` with `rule2 := glyph.Rule + glyph.Rule` (same in browse.go:339); form.go `"► "`/`"▼ "` → `glyph.Collapsed+" "`/`glyph.Open+" "`, `"◄ "+v+" ►"` → `glyph.ChoicePrev+" "+v+" "+glyph.ChoiceNext`; sidebar `"●"` → `glyph.Attention`, `" × "` → `" "+glyph.Remove+" "`. Leave `strings.Repeat("─", w)` rules as `strings.Repeat(glyph.Rule, w)`. Don't touch the password mask (`•` in input.go: the browser draws its own).

- [ ] **Step 4: Run** `go test ./internal/ui ./internal/str` — PASS, goldens untouched.

- [ ] **Step 5: Commit** `glyph: the symbols Kiln draws, in one place`.

### Task 6: the theme's styles as data

**Files:** Modify `internal/theme/theme.go`, `internal/theme/theme_test.go`

**Interfaces:**
- Produces:

```go
// Look is a resolved style as data, for a front end that draws it
// itself. Colors are "#rrggbb", "ansi:N" (one of the terminal's
// sixteen) or "" for none.
type Look struct {
	FG, BG                                  string `json:",omitempty"`
	Bold, Faint, Italic, Underline, Reverse bool   `json:",omitempty"`
}

// TagLook is a tag's Look, and whether it's for the text the tag matched.
type TagLook struct {
	Look
	Match bool `json:",omitempty"`
}

// Looks is every role's and tag's style, as data.
func (t *Theme) Looks() (roles map[Role]Look, tags map[string]TagLook)
```

- [ ] **Step 1: Write the theme test** (`theme_test.go`):

```go
func TestLooks(t *testing.T) {
	th, err := FromTOML(`[palette]
accent = "#FF9F43"
[ui]
"sidebar.active" = { fg = "accent", bold = true }
[tags]
page = { fg = "red", reverse = true }
self = { scope = "match", underline = true }`)
	if err != nil {
		t.Fatal(err)
	}
	roles, tags := th.Looks()
	if got := roles[SidebarActive]; got.FG != "#ff9f43" || !got.Bold {
		t.Errorf("sidebar.active %+v", got)
	}
	if got := tags["page"]; got.FG != "ansi:1" || !got.Reverse || got.Match {
		t.Errorf("page %+v", got)
	}
	if got := tags["self"]; !got.Underline || !got.Match {
		t.Errorf("self %+v", got)
	}
	if len(roles) != len(Roles) {
		t.Errorf("%d roles, want %d", len(roles), len(Roles))
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/theme -run TestLooks` — FAIL.

- [ ] **Step 3: Implement** `color.ref()` (`fmt.Sprintf("ansi:%d", c.ansi)` or `strings.ToLower(c.hex)`), `style.look()`, and `Looks()` over `Roles` and `t.tags`.

- [ ] **Step 4: Run** `go test ./internal/theme` — PASS. **Commit** `theme: styles as data`.


### Task 7: lines and prompts as spans; clean notifications

**Files:** Create `internal/webapi/spans.go`, `internal/webapi/spans_test.go`; modify `internal/webapi/encode.go`, `internal/webapi/encode_test.go`, `internal/notify/notify.go`, `internal/notify/notify_test.go`

**Interfaces:**
- Consumes: `ansi.Spans`, `ansi.SpanStyle` (Task 1), `rules.TagRuns` (Task 2), `app.Links` (Task 3), `app.TimeOfDay` (Task 4), `(*theme.Theme).Looks` (Task 6, in the test).
- Produces:

```go
// Span is a run of a line's text with one look: the server's style, the
// styled tags over it in the order their styles fold (see
// rules.TagRuns), and the link it's part of.
type Span struct {
	Text string
	ansi.SpanStyle
	Tags []string `json:",omitempty"`
	Link string   `json:",omitempty"`
}

// Line (replacing Text):
	DayLabel string // the day as people read it; "" for none
	Clock    string // the time of day, "15:04"; "" for none
	Plain    string // the text, no styling: the spans' texts joined
	Spans    []Span

func spansOf(text string, hl *rules.Highlighter, tags []classify.Tag) []Span
func Encode(evs []any, hl func(key string) *rules.Highlighter) ([]byte, error) // hl may be nil, or return nil: no tag styles
func notify.Clean(line string) string
```

- [ ] **Step 1: Write the equivalence test** (`spans_test.go`): the page's fold, done in Go, must give per byte what the TUI paints.

```go
// fold is how the page draws a span: its server style, each tag's style
// on top in order, a set color replacing, flags adding up.
func fold(sp Span, tags map[string]theme.TagLook) ansi.SpanStyle {
	st := sp.SpanStyle
	for _, n := range sp.Tags {
		l := tags[n].Look
		if l.FG != "" {
			st.FG = l.FG
		}
		if l.BG != "" {
			st.BG = l.BG
		}
		st.Bold, st.Faint, st.Italic = st.Bold || l.Bold, st.Faint || l.Faint, st.Italic || l.Italic
		st.Underline, st.Reverse = st.Underline || l.Underline, st.Reverse || l.Reverse
	}
	return st
}

// perByte is each byte's style, from spans with Text.
func perByte[T any](spans []T, text func(T) string, style func(T) ansi.SpanStyle) []ansi.SpanStyle {
	var out []ansi.SpanStyle
	for _, s := range spans {
		for range len(text(s)) {
			out = append(out, style(s))
		}
	}
	return out
}

func TestSpansDrawWhatTheTUIPaints(t *testing.T) {
	th, err := theme.FromTOML(`[tags]
page = { fg = "#ff9f43", bold = true }
"page/in" = { italic = true }
self = { scope = "match", bg = "blue", reverse = true }
quiet = { faint = true, fg = "default" }`)
	if err != nil {
		t.Fatal(err)
	}
	hl := rules.New(th)
	_, looks := th.Looks()
	for _, c := range []struct {
		name, text string
		tags       []classify.Tag
	}{
		{"plain", "Rook waves.", nil},
		{"server colors only", "\x1b[31mred\x1b[0m plain", nil},
		{"whole line over server color", "\x1b[32mMira\x1b[0m pages: hi", []classify.Tag{{Name: "page/in"}}},
		{"match over part", "Rook says, \"Evening, Kit.\"", []classify.Tag{{Name: "self", Spans: []classify.Span{{Start: 20, End: 23}}}}},
		{"reset inside a match", "Kit \x1b[0;31mKit\x1b[0m Kit", []classify.Tag{{Name: "self", Spans: []classify.Span{{Start: 4, End: 7}}}}},
		{"line and match overlap", "\x1b[1mKit\x1b[22m pages", []classify.Tag{{Name: "page"}, {Name: "self", Spans: []classify.Span{{Start: 0, End: 3}}}}},
		{"default clears nothing", "\x1b[33mquiet\x1b[0m", []classify.Tag{{Name: "quiet"}}},
		{"multibyte before a match", "café ☕ Kit", []classify.Tag{{Name: "self", Spans: []classify.Span{{Start: len("café ☕ "), End: len("café ☕ Kit")}}}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			plain := ansi.Strip(c.text)
			painted := style.Highlight(c.text, hl.Runs(plain, c.tags))
			want := perByte(ansi.Spans(painted), func(s ansi.Span) string { return s.Text }, func(s ansi.Span) ansi.SpanStyle { return s.Style })
			got := perByte(spansOf(c.text, hl, c.tags), func(s Span) string { return s.Text }, func(s Span) ansi.SpanStyle { return fold(s, looks) })
			if !reflect.DeepEqual(got, want) {
				t.Errorf("web %v\nTUI %v", got, want)
			}
		})
	}
}
```


Plus:

```go
func TestSpansJoinToPlainAndMarkLinks(t *testing.T) {
	text := "See \x1b[4mhttps://kiln.test/map\x1b[0m, café"
	sp := spansOf(text, nil, nil)
	var b strings.Builder
	var link string
	for _, s := range sp {
		b.WriteString(s.Text)
		if s.Link != "" {
			link += s.Text
		}
	}
	if b.String() != ansi.Strip(text) {
		t.Errorf("joined %q", b.String())
	}
	if link != "https://kiln.test/map" {
		t.Errorf("link text %q", link)
	}
	for _, s := range sp {
		if s.Link != "" && s.Link != "https://kiln.test/map" {
			t.Errorf("span %q links to %q", s.Text, s.Link)
		}
	}
}
```

And in `encode_test.go`, `TestEncodeLines`: a `feed.Lines` with one `app.Server` line (`Entry{Time: day, Text: "\x1b[31mhi\x1b[0m"}`), one `app.Day` line (`Day: "2026-09-24"`) and one `app.HistoryEnd` line encode with `Plain`, `Spans`, no `Text`; the day line's `Plain` is `app.DayLabel("2026-09-24")`, the history line's is `str.ScrollbackHistoryEnds(day.Format(str.DateDayTime()))`, the server line's `Clock` is `app.TimeOfDay(day)` and its span has `"FG":"ansi:1"`. `TestEncodePrompt`: a `feed.Input` with `Prompt: "\x1b[1m> \x1b[0m"` encodes `Prompt` as spans (`[{"Text":"> ","Bold":true}]`). `TestNotifyIsClean`: a `feed.Effect{app.Notify{Title: "Kit", Body: "\x1b[31mMira\x07 pages "}}` encodes `"Body":"Mira pages"`.

- [ ] **Step 2: Run** `go test ./internal/webapi` — FAIL (undefined `spansOf`, Encode arity).

- [ ] **Step 3: Implement `spansOf`:**

```go
// spansOf splits text (sanitized, the server's escape codes in it) into
// spans: cut wherever the server's style, the styled tags (with hl; nil
// for none) or a link change, and merged where none does.
func spansOf(text string, hl *rules.Highlighter, tags []classify.Tag) []Span {
	type seg struct {
		start, end int
		st         ansi.SpanStyle
	}
	var segs []seg
	var b strings.Builder
	for _, s := range ansi.Spans(text) {
		segs = append(segs, seg{b.Len(), b.Len() + len(s.Text), s.Style})
		b.WriteString(s.Text)
	}
	plain := b.String()
	var runs []rules.TagRun
	if hl != nil {
		runs = hl.TagRuns(plain, tags)
	}
	links := app.Links(plain)
	cuts := []int{0, len(plain)}
	for _, s := range segs {
		cuts = append(cuts, s.start)
	}
	for _, r := range runs {
		cuts = append(cuts, r.Start, r.End)
	}
	for _, l := range links {
		cuts = append(cuts, l[0], l[1])
	}
	slices.Sort(cuts)
	cuts = slices.Compact(cuts)
	var out []Span
	si, ri, li := 0, 0, 0
	for i := 0; i+1 < len(cuts); i++ {
		a, z := cuts[i], cuts[i+1]
		for si+1 < len(segs) && segs[si].end <= a {
			si++
		}
		for ri < len(runs) && runs[ri].End <= a {
			ri++
		}
		for li < len(links) && links[li][1] <= a {
			li++
		}
		sp := Span{Text: plain[a:z], SpanStyle: segs[si].st}
		if ri < len(runs) && runs[ri].Start <= a {
			sp.Tags = runs[ri].Names
		}
		if li < len(links) && links[li][0] <= a {
			sp.Link = plain[links[li][0]:links[li][1]]
		}
		if n := len(out); n > 0 && out[n-1].SpanStyle == sp.SpanStyle && out[n-1].Link == sp.Link && slices.Equal(out[n-1].Tags, sp.Tags) {
			out[n-1].Text += sp.Text
			continue
		}
		out = append(out, sp)
	}
	return out
}
```

`Line` and `lineOf(l app.Line, hl *rules.Highlighter) Line`:

```go
func lineOf(l app.Line, hl *rules.Highlighter) Line {
	out := Line{Seq: l.Seq, Kind: kindName(l.Kind), Day: l.Day, Time: l.Entry.Time, Out: l.Entry.Dir == logstore.Out,
		Tags: l.Tags, Attention: l.Attention, Quiet: l.Quiet}
	if l.Day != "" {
		out.DayLabel = app.DayLabel(l.Day)
	}
	if !l.Entry.Time.IsZero() {
		out.Clock = app.TimeOfDay(l.Entry.Time)
	}
	text, tags := l.Text(), l.Tags
	switch l.Kind {
	case app.Day:
		text = out.DayLabel
	case app.HistoryEnd:
		text = str.ScrollbackHistoryEnds(l.Entry.Time.Format(str.DateDayTime()))
	}
	if l.Kind != app.Server {
		hl, tags = nil, nil
	}
	out.Spans = spansOf(text, hl, tags)
	for _, s := range out.Spans {
		out.Plain += s.Text
	}
	return out
}
```

(Match `paint.go` exactly for `Day`/`HistoryEnd` text; the `──` and `* `/`›` decorations are the page's, from `glyph`.)

`Encode(evs, hl)`: `look := func(k string) *rules.Highlighter { if hl == nil { return nil }; return hl(k) }`; `Lines`/`LogLines` use `linesOf(v.Lines, look(v.Key))`; a new `case feed.Input:` encodes `struct{ Key, Text string; Ack int; Prompt []Span; NeedPW bool }` with `Prompt: spansOf(v.Prompt, nil, nil)` (nil when `v.Prompt == ""`); `case feed.Effect:` with an `app.Notify` sets `Body = notify.Clean(n.Body)` before marshalling. Existing callers of `Encode` (tests, `loop.send`) pass `nil` for now; Task 8 passes the loop's highlighters.

`notify.Clean` is the first half of `Message` (strip escapes, drop control characters, trim); `Message` calls it. Add a `notify_test` case for `Clean`.

- [ ] **Step 4: Run** `go test ./internal/webapi ./internal/notify ./internal/ui` — PASS.

- [ ] **Step 5: Commit** `webapi: lines and prompts go out as marked spans; notifications cleaned`.

### Task 8: the `Theme` event

**Files:** Create `internal/webapi/theme.go`, `internal/webapi/theme_test.go`; modify `internal/webapi/loop.go` (`Options.ConfigDir`, theme state, `send`, `events`, `open`), `web/cmd/kiln-web/main.go` (pass `ConfigDir`)

**Interfaces:**
- Consumes: `theme.Look`, `theme.TagLook`, `(*Theme).Looks` (Task 6).
- Produces (webapi):

```go
// Theme is how things look, as data, for dark and light backgrounds;
// Appearance is the setting (auto, light or dark).
type Theme struct {
	Appearance  string
	Dark, Light Looks
}

// Looks is one appearance's styles. Chars has, by character key, the tag
// table of each character whose own [palette] or [tags] change it.
type Looks struct {
	Roles map[theme.Role]theme.Look
	Tags  map[string]theme.TagLook
	Chars map[string]map[string]theme.TagLook `json:",omitempty"`
}

type Options struct {
	Autoconnect bool
	Drafts      string
	ConfigDir   string // where themes are read from; "" for the built-in only
}
```

- [ ] **Step 1: Write the webapi tests** (`theme_test.go`, using `newApp`/`startOn` from `loop_test.go`, with `Options{ConfigDir: dir}` — extend `startOn` to take `Options` or add `startWith(t, a, dir, opts)`):
  - `TestThemeComesAfterStart`: the first batch's types begin `Start`, `Theme`; `Theme.Appearance == "auto"` (the default config); `Dark.Roles["sidebar.active"]` and `Light.Roles["sidebar.active"]` are both present and differ (the built-in has a light palette).
  - `TestThemeFollowsTheConfig`: write `themes/mine.toml` (`extends = "kiln"`, `[tags] page = { fg = "#123456" }`) and set `theme = "mine"` in `config.toml`; send `{"Do":"submit","Text":"/reload","N":1}` (or whatever reloads — check `app` for the reload command; a settings save also reloads); that turn's batch has a `Theme` whose `Dark.Tags["page"].FG == "#123456"`. A turn without a config change has no `Theme`.
  - `TestCharLooksAndSpans`: give Kit `[tags] page = { fg = "#abcdef" }` in `worlds/fm.toml` (a character's own `[tags]`; check `config` for the exact table placement); open Kit; `Theme.Dark.Chars["fm/kit"]["page"].FG == "#abcdef"`, and Rook has no `Chars` entry. Feed Kit a line Kit's rules tag `page` (use the fuzzball pack's page pattern, or a `[[rules]]` entry in the world file) and check its span has `Tags: ["page"]`.
  - `TestBadLookFallsBackAndSaysSo` (Review Focus 3): Kit's `[tags] page = { fg = "nosuchcolor" }`; opening Kit puts `str.StatusCharError("fm/kit", …)` on the status line (compare up to the error with `upTo`/`mark`), there's no `Chars["fm/kit"]`, and Kit's lines still get spans (from the theme's own tags).
  - `TestBrokenThemeKeepsWhatItHad` (Review Focus 4): start with `theme = "missing"` → status is an error and `Theme.Dark.Roles` equals the built-in's (`theme.BuiltinFor(theme.Dark).Looks()`); then a valid `mine` + reload → `Theme` with `mine`'s colors; then break `mine.toml` (bad TOML) + reload → no `Theme` event that turn, status error.

- [ ] **Step 2: Run** `go test ./internal/webapi -run Theme` — FAIL.

- [ ] **Step 3: Implement** in `internal/webapi/theme.go`:

```go
// looks is the loop's theme state: the themes it last sent and each
// open character's highlighter.
type looks struct {
	cfg      *config.Config // the config they were made for
	dark     *theme.Theme   // nil until a theme has loaded
	light    *theme.Theme
	ev       Theme
	hl       map[string]*rules.Highlighter // by character key
	lookErrs map[string]error              // characters whose own looks didn't resolve
}

// load reads the theme for cfg from dir, for both appearances, and each
// character's own looks on top. A broken theme keeps the one loaded
// before (at start, the built-in); its error is returned for the status
// line. changed is false when nothing that's sent changed.
func (lk *looks) load(dir string, cfg *config.Config, chars []config.Character) (changed bool, err error)

// highlighter is k's, or the theme's own when k has none.
func (lk *looks) highlighter(k string) *rules.Highlighter
```

`load`: `theme.Load(dir, cfg.Theme, theme.Dark)` and `…Light`; on error with `lk.dark != nil`, keep the old ones and return the error; on error at start, use what `Load` returned (the built-in) and return the error. For each `ch` in `chars` with `len(ch.Looks) > 0`: `d, err := lk.dark.With(ch.Looks...)`, `l, _ := lk.light.With(ch.Looks...)`; on error record `lookErrs[key]` and use `rules.New(lk.dark)`; else `hl[key] = rules.New(d)` and put both `Looks()` tag tables in `Chars[key]`. Characters without looks use `rules.New(lk.dark)`. Build `ev` (`Appearance: cfg.Appearance`); `changed = !reflect.DeepEqual(ev, lk.ev)`.

In `loop.go`: `Loop` gets `lk looks`. `Run` loads before the first send. `events`: when `all`, put the `Theme` right after `Start`; otherwise, if `l.a.Config() != l.lk.cfg`, reload and, if changed, put the `Theme` first. A load error goes to `l.a.SetStatus(true, err.Error())` (as the TUI's `themeErr`; check what text the TUI shows for it and use the same). `l.open(k)`: if `lookErrs[k]` is set, `l.a.SetStatus(true, str.StatusCharError(k, err))`. `send` passes `l.lk.highlighter` to `Encode`. Delete the `nil` placeholder from Task 7. In `kiln-web`, pass `ConfigDir: cfgDir`.

- [ ] **Step 4: Run** `go test ./internal/webapi && (cd web && GOOS=js GOARCH=wasm go vet ./...)` — PASS.

- [ ] **Step 5: Commit** `webapi: the theme goes to the page as data; lines tagged with each character's own looks`.

### Task 9: display text in `Char`

**Files:** Modify `internal/feed/events.go`, `internal/feed/feed.go` (`char`), `internal/feed/feed_test.go`

**Interfaces:**
- Consumes: `app.ConnectedSince`, `app.Clock`, `(*App).Now` (Task 4); `(*App).LastTime` (existing).
- Produces: `Char.ConnectedSince string` ("" unless connected) and `Char.LastSeen string` (when its last line came, as `app.Clock` says it; "" for none).

- [ ] **Step 1: Write the failing test** (in `TestPresenceIdleTextConnectedAtAndStore` or a new `TestCharSaysWhenAsPeopleRead`): after Kit connects, `Char.ConnectedSince == app.ConnectedSince(day, day)`; after `line(a, "fm/kit", s, "hi")`, `Char.LastSeen == app.Clock(day, day)`; Rook (never connected, no lines) has both `""`.

- [ ] **Step 2: Run** `go test ./internal/feed` — FAIL.

- [ ] **Step 3: Implement.** `char(a *app.App, c *app.Char) Char` (update its call in `Turn`):

```go
now := a.Now()
if c.State == session.Connected && !c.ConnectedAt.IsZero() {
	ch.ConnectedSince = app.ConnectedSince(c.ConnectedAt, now)
}
if t, ok := a.LastTime(c.Key); ok {
	ch.LastSeen = app.Clock(t, now)
}
```

The 30-second tick already runs a turn, so a day rollover reaches the page within 30 seconds.

- [ ] **Step 4: Run** `go test ./internal/feed ./internal/webapi` — PASS. **Commit** `feed: characters say when, as people read it`.

### Task 10: `check(field, value)`

**Files:** Modify `internal/app/edit.go`, `internal/app/edit_test.go` (or create `check_test.go`), `internal/webapi/loop.go` (next to `OverLimit`), `internal/webapi/loop_test.go`, `web/cmd/kiln-web/main.go`

**Interfaces:**
- Produces: `func CheckField(field, value string) error` (app; the `Accept*` filter for that field's config key, nil for a field with no live check); `func CheckField(field, value string) string` (webapi; the message, "" when fine); `kiln.check(field, value)` (wasm, synchronous).

- [ ] **Step 1: Write the failing test:**

```go
func TestCheckField(t *testing.T) {
	for _, c := range []struct {
		field, value string
		want         error // nil: fine
	}{
		{"id", "pf-muck_2", nil},
		{"id", "pf muck", AcceptWorldID("pf muck")},
		{"port", "5555", nil},
		{"port", "55a", AcceptPort("55a")},
		{"host", "muck.test", nil},
		{"host", "muck test", AcceptHost("muck test")},
		{"use", "fuzzball, pfmuck", nil},
		{"max_line_bytes", "12x", AcceptByteCount("12x")},
		{"aliases", "Kit Kitsune", nil},
		{"tls", "anything", nil},
	} {
		got := CheckField(c.field, c.value)
		if (got == nil) != (c.want == nil) || got != nil && got.Error() != c.want.Error() {
			t.Errorf("CheckField(%q, %q) = %v, want %v", c.field, c.value, got, c.want)
		}
	}
}
```

Plus a webapi `TestCheckField`: `CheckField("port", "55a") == app.AcceptPort("55a").Error()`, `CheckField("port", "5555") == ""`.

- [ ] **Step 2: Run** — FAIL.

- [ ] **Step 3: Implement:**

```go
// liveChecks are the fields checked as they're typed, by config key.
var liveChecks = map[string]func(string) error{
	"id": AcceptWorldID, "port": AcceptPort, "host": AcceptHost, //str:ok: config keys
	"use": AcceptPacks, "max_line_bytes": AcceptByteCount, "aliases": AcceptAliases, //str:ok: config keys
}

// CheckField is what a front end says about field (a config key) as
// value is typed: the same rule the TUI applies to keystrokes. nil for a
// field with no live check.
func CheckField(field, value string) error {
	if f := liveChecks[field]; f != nil {
		return f(value)
	}
	return nil
}
```

Confirm the keys match the config keys `config.FieldError` uses (`id`, `port`, `host`, `use`, `max_line_bytes`, `aliases`). webapi's wraps it to a string. In `kiln-web/main.go`, next to `overLimit`: `k.Set("check", js.FuncOf(func(_ js.Value, a []js.Value) any { return webapi.CheckField(a[0].String(), a[1].String()) }))` with `//str:ok`.

- [ ] **Step 4: Run** `go test ./internal/app ./internal/webapi && (cd web && GOOS=js GOARCH=wasm go vet ./...)` — PASS. **Commit** `app, webapi: check a field as it's typed`.

### Task 11: the TypeScript strings and glyphs module

**Files:** Modify `internal/str/catalog/generate.go`, `internal/str/catalog/generate_test.go`, `internal/str/gen/main.go`, `internal/str/str_test.go`; create `web/ui/src/str.gen.ts` (generated), `web/ui/src/str.test.ts`; modify `.github/workflows/web.yml`, `docs/web-dev.md`

**Interfaces:**
- Produces: `func TS(en, goSrc []byte, glyphs [][2]string) ([]byte, error)` (catalog; `goSrc` is the `keys_gen.go` just written, for parameter order; `glyphs` are name/value pairs); in TS, one `export function` per entry named like Go's but starting lower-case (`browseSaved`), and `export const glyph = { echo: "›", … } as const`.

- [ ] **Step 1: Write the generator test** (`generate_test.go`):

```go
func TestTS(t *testing.T) {
	en := []byte(`separator = " · "
[browse]
saved = "saved {path}"
quoted = "no tag {name:%q} {{here}}"
copied = { one = "copied 1 line", other = "copied {n} lines" }
[x]
tick = "a ` + "`" + `tick` + "`" + ` and ${dollar}"
`)
	goSrc, err := Generate(en, nil)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := TS(en, goSrc, [][2]string{{"Echo", "›"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"export function browseSaved(path: unknown): string",
		"export function browseCopied(n: number): string",
		"export function browseQuoted(name: unknown): string",
		"export function separator(): string",
		`echo: "›"`,
	} {
		if !strings.Contains(string(ts), want) {
			t.Errorf("missing %q in:\n%s", want, ts)
		}
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/str/catalog -run TestTS` — FAIL.

- [ ] **Step 3: Implement `TS`.** Header: `// Code generated by go generate ./internal/str from locales/en.toml and internal/glyph. DO NOT EDIT.` then:

```ts
const plural = new Intl.PluralRules("en");
const q = (v: unknown): string => JSON.stringify(String(v)); // Go's %q, near enough for display
const c = (v: unknown): string => String.fromCodePoint(Number(v));
const e = (v: unknown): string => (v instanceof Error ? v.message : String(v));
```

For each key (sorted), parameters in the order `signatures(goSrc)` gives for `FuncName(key)` (so TS and Go agree); a parameter named like a JS reserved word gets `_` (as Go's keywords do). Types: `n` → `number`, everything else `unknown`. Each template becomes a template literal: literal text escaped (`\` → `\\`, `` ` `` → `` \` ``, `${` → `\${`), `{name}` → `${name}`, `{err}` → `${e(err)}`, `{name:%q}` → `${q(name)}`, `{name:%c}` → `${c(name)}`, any other verb is an error. A plural:

```ts
export function browseCopied(n: number): string {
  switch (plural.select(n)) {
    case "one": return `copied 1 line`;
    default: return `copied ${n} lines`;
  }
}
```

(one `case` per form other than `other`, which is `default`). A `/** key: "template" */` doc comment above each. Then the glyph object, keys lower-camel (`ChoicePrev` → `choicePrev`). In `gen/main.go`, after writing `keys_gen.go`, write `../../web/ui/src/str.gen.ts` from `catalog.TS(data, src, glyphs)`, with `glyph.All` as the glyphs.

- [ ] **Step 4: Staleness.** In `str_test.go`'s staleness test, also check `../../web/ui/src/str.gen.ts` against `catalog.TS(en, got, glyph.All)`, failing with `web/ui/src/str.gen.ts is stale: run go generate ./internal/str`.

- [ ] **Step 5: Generate and test in Node.** Run `go generate ./internal/str`. Write `web/ui/src/str.test.ts`:

```ts
import { test } from "node:test";
import assert from "node:assert/strict";
import * as str from "./str.gen.ts";

test("plurals pick their form", () => {
  // browse.lines_in_range has one and other forms: 1 and 2 must differ,
  // and only "other" carries the count.
  assert.notEqual(str.browseLinesInRange(1), str.browseLinesInRange(2).replace("2", "1"));
  assert.match(str.browseLinesInRange(7), /7/);
});

test("placeholders fill in", () => {
  assert.match(str.viewConnectedSince("21:14"), /21:14/);
});

test("glyphs are one character each", () => {
  for (const g of Object.values(str.glyph)) assert.equal([...g].length, 1);
});
```

Run `node --test web/ui/src/*.test.ts` — PASS.

- [ ] **Step 6: CI and docs.** In `.github/workflows/web.yml`, the node step becomes `node --test web/static/*.test.mjs web/ui/src/*.test.ts`. In `docs/web-dev.md`'s test list, the same command, and a line under it: `web/ui/src/str.gen.ts` is generated by `go generate ./internal/str`; never edit it.

- [ ] **Step 7: Run** `go test ./internal/str/... ./internal/glyph` and the node tests — PASS. **Commit** `str: the catalog and glyphs as a TypeScript module for the page`.

### Task 12: the browser's time zone

**Files:** Create `web/cmd/kiln-web/zone.go`, `web/cmd/kiln-web/zone_test.go` (or put the pure part in a non-js file so it tests natively); modify `web/cmd/kiln-web/main.go`

**Interfaces:**
- Produces: `func zone(name string) *time.Location` — the IANA zone `name` from the embedded tzdata, or nil if `name` is empty or unknown (keep Go's own `time.Local` then).

- [ ] **Step 1: Write the failing test** (native build; the file with `zone` has no `js` build tag):

```go
func TestZone(t *testing.T) {
	ny := zone("America/New_York")
	if ny == nil {
		t.Fatal("no New York")
	}
	// Across a daylight-saving change the offset differs: a fixed offset
	// (Go's wasm default) can't do that.
	_, winter := time.Date(2026, 1, 15, 12, 0, 0, 0, ny).Zone()
	_, summer := time.Date(2026, 7, 15, 12, 0, 0, 0, ny).Zone()
	if winter == summer {
		t.Errorf("same offset in winter and summer: %d", winter)
	}
	if zone("") != nil || zone("Not/AZone") != nil {
		t.Error("an empty or unknown zone should be nil")
	}
}
```

- [ ] **Step 2: Run** `(cd web && go test ./cmd/kiln-web -run TestZone)` — FAIL. (If `web/cmd/kiln-web` is `js`-only throughout, put `zone` in a small new package, `web/tz`, instead.)

- [ ] **Step 3: Implement:**

```go
import _ "time/tzdata" // the browser has no zoneinfo files

// zone is the IANA time zone name, or nil when there's none to use.
func zone(name string) *time.Location {
	if name == "" {
		return nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil
	}
	return loc
}
```

At the top of `main()` in `main.go`, before anything reads the time:

```go
tz := js.Global().Get("Intl").Call("DateTimeFormat").Call("resolvedOptions").Get("timeZone") //str:ok
if tz.Type() == js.TypeString {
	if loc := zone(tz.String()); loc != nil {
		time.Local = loc
	}
}
```

- [ ] **Step 4: Run** `(cd web && go test ./... && GOOS=js GOARCH=wasm go vet ./...)` and `web/build.sh` — PASS. Note the wasm's size before and after in the commit message (tzdata adds about 450KB). **Commit** `kiln-web: the browser's time zone, so days are right across daylight saving`.

### Task 13: wrap up

**Files:** `web/static/kiln.js` (the dev page, if it reads `Line.Text` or `Input.Prompt` as a string), `web/smoke/smoke_test.go` (if it reads `Text`)

- [ ] **Step 1:** Find every reader of the old shapes: `grep -rn '\.Text\b\|Prompt' web/static/kiln.js web/static/index.html web/smoke`. The dev page shows a line as `line.Plain` and a prompt as its spans' text joined; the smoke test looks for the echoed line in `Plain`.
- [ ] **Step 2:** Run the full set: `go vet ./... && go test ./...`, `(cd web && go vet ./... && go test ./... && GOOS=js GOARCH=wasm go vet ./... && GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...)`, `node --test web/static/*.test.mjs web/ui/src/*.test.ts`, `web/build.sh`, and `(cd web && KILN_SMOKE=1 go test ./smoke -v)`. All PASS; goldens unchanged (`git status internal/ui/testdata` clean).
- [ ] **Step 3: Commit** `web: the dev page and smoke test read lines' new shape`.
- [ ] **Step 4:** A fresh review of the whole branch against `main` with this plan's Review Focus (per the project's workflow), fixes for anything Critical or Important with a failing test first, then a PR that says: what the page gets now; the four Rulings above; the wasm size change; and that #141's JSON items were already done (tick them).
