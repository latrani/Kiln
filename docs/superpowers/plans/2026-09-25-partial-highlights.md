# Partial Highlights Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Highlight rules with `scope = "match"` style only the matched part of a line (the classify tag's spans, or the rule's own pattern matches), drawn over the server's colors.

**Architecture:** `classify` gains `Tags`, which returns each tag with the byte spans that produced it (`Classify` stays as the names-only view). `rules.Apply` takes those tags and, when a match-scope rule applies, returns the line cut into runs, each with its folded style. `style.Highlight` draws a result over server text, restoring the server's SGR state where a styled run ends. The UI's `renderLine` and `kiln tail` both draw through it.

**Tech Stack:** Go 1.27, `regexp`, `unicode/utf8`.

**Spec:** `docs/superpowers/specs/2026-09-25-partial-highlights-design.md`

## Global Constraints

- No new dependencies.
- Test fixtures use only the suite's names: characters Kit, Rook, Ash, Mira, Zoë; worlds `fm`. Never real players' names.
- Commit straight to `main`; never push or open PRs. Every commit message ends with:
  `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01X5RzCDj8AoLniuQdFBZC4E`
- Never run kiln against the real `~/.config/kiln`; any manual run uses scratch `XDG_CONFIG_HOME`/`XDG_DATA_HOME`.
- `scope` values: absent or `"line"` (whole line, today's behavior) and `"match"`. Error text: `highlight rule N: scope must be "line" or "match"`.
- A span is a byte range `[Start, End)` of the plain (ANSI-stripped) text; empty matches never make spans.
- Folding is today's: later non-empty colors win; bold, italic and underline accumulate; attention is per line.
- Run `gofmt -w` on changed files, then `gofmt -l .` (prints nothing) and `go test -race ./...` before each commit.

## Review Focus

- Server text holding non-SGR escapes (an OSC title in `kiln tail`, which isn't sanitized): spans must still land on the right characters. (Test in Task 4.)
- A styled span that runs to the end of the line followed by a trailing server reset: the style is closed and the line ends reset. (Task 4.)
- Overlapping spans for one tag (two classify rules matching `PAGE` and `PAGE:`): one merged run, not a split or a gap. (Task 3.)
- A regex-special alias like `(Ash)`: its `self` span covers exactly `(Ash)`. (Task 2.)
- A match-scope rule whose tag is on the line with no spans (a zero-width classify pattern like `^`): no styling, but its attention still counts. (Task 3.)

## File Structure

- `internal/config/config.go`: `HighlightRule.Scope`, validation. Test: `config_test.go`.
- `internal/classify/classify.go`: `Span`, `Tag`, `Tags`; `Classify` rebuilt on `Tags`; `self` spans. Test: `classify_test.go`.
- `internal/rules/rules.go`: `Run`, `Result.Runs`, `Apply(plain, []classify.Tag)`. Test: `rules_test.go`.
- `internal/ansi/strip.go`: exported `EscapeEnd`.
- `internal/style/style.go`: `Highlight`. Test: `style_test.go`.
- `internal/ui/model.go` (`renderLine`), `cmd/kiln/main.go` and `cmd/kiln/render.go`: callers. Tests: `internal/ui/model_test.go`, `cmd/kiln/render_test.go`.
- `README.md`.

---

### Task 1: `scope` in the config

**Files:**
- Modify: `internal/config/config.go` (`HighlightRule`; `validate`)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.HighlightRule.Scope string` (toml `scope`; "" means line).

- [ ] **Step 1: Write the failing tests**

In `config_test.go`, add to the `TestLoadErrors` cases:

```go
		{"bad scope", "host = \"h\"\nport = 1\n[[highlight]]\nmatch = { pattern = 'x' }\nscope = \"word\"\n[[characters]]\nname = \"Kit\"\n", `highlight rule 1: scope must be "line" or "match"`},
```

and append:

```go
func TestHighlightScope(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"worlds/fm.toml": `host = "h"
port = 1

[[highlight]]
match = { tags = ["page"] }
scope = "match"

[[highlight]]
match = { tags = ["page"] }
scope = "line"

[[highlight]]
match = { tags = ["page"] }

[[characters]]
name = "Kit"
`})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, _ := cfg.Find("fm", "Kit")
	var got []string
	for _, r := range kit.Rules.Highlight {
		got = append(got, r.Scope)
	}
	if want := []string{"match", "line", ""}; !reflect.DeepEqual(got, want) {
		t.Errorf("scopes = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/config -run 'TestLoadErrors|TestHighlightScope'`
Expected: FAIL: `bad scope` gets `unknown key "scope"`; `TestHighlightScope` fails to load with the same error.

- [ ] **Step 3: Implement**

In `config.go`, change `HighlightRule` to:

```go
// HighlightRule styles matching lines and optionally flags them for
// attention. Scope "match" styles only the matched text (see
// rules.Apply); "" or "line" styles the whole line.
type HighlightRule struct {
	Match     Match  `toml:"match"`
	Style     Style  `toml:"style"`
	Attention bool   `toml:"attention"`
	Scope     string `toml:"scope"`
}
```

In `validate`, inside the `for i, r := range ch.Rules.Highlight` loop, after the pattern check:

```go
		if r.Scope != "" && r.Scope != "line" && r.Scope != "match" {
			return fmt.Errorf(`highlight rule %d: scope must be "line" or "match"`, i+1)
		}
```

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/config`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/config
git commit -m "feat(config): highlight rules take a scope

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01X5RzCDj8AoLniuQdFBZC4E"
```

---

### Task 2: Classify records spans

**Files:**
- Modify: `internal/classify/classify.go`
- Test: `internal/classify/classify_test.go`

**Interfaces:**
- Produces:
  - `type Span struct{ Start, End int }`
  - `type Tag struct{ Name string; Spans []Span }`
  - `func (c *Classifier) Tags(plain string) []Tag`: same tags, same order as `Classify` (rule order, no duplicates, `self` last). A tag's spans: every non-empty match of each of its rules, in rule order, then position. `Spans` is nil when there are none.
  - `Classify(plain string) []string` unchanged in behavior (now built on `Tags`).

- [ ] **Step 1: Write the failing tests**

Append to `classify_test.go`:

```go
func TestTagsSpans(t *testing.T) {
	c, err := New([]config.ClassifyRule{
		{Tag: "page", Pattern: `^PAGE:`},
		{Tag: "page", Pattern: `pages: `},
		{Tag: "ooc", Pattern: `OOC`},
		{Tag: "start", Pattern: `^`},
	}, "Kit", []string{"Kitty"})
	if err != nil {
		t.Fatal(err)
	}
	// PAGE: 0-5, "pages: " 11-18, OOC 18-21, Kit 22-25, Kit 27-30, kitty 32-37
	got := c.Tags("PAGE: Mira pages: OOC Kit, Kit! kitty")
	want := []Tag{
		{Name: "page", Spans: []Span{{0, 5}, {11, 18}}},
		{Name: "ooc", Spans: []Span{{18, 21}}},
		{Name: "start"}, // zero-width: tagged, but no spans
		{Name: SelfTag, Spans: []Span{{22, 25}, {27, 30}, {32, 37}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Tags = %+v\nwant   %+v", got, want)
	}
}

func TestSelfSpansCoverOnlyTheName(t *testing.T) {
	cases := []struct {
		name    string
		aliases []string
		in      string
		want    []Span
	}{
		{"Kit", nil, "KitKit Kit", []Span{{7, 10}}},        // not a whole word, then a whole word
		{"Kit", nil, "Kitë Kit", []Span{{6, 9}}},           // ë continues the word
		{"Zoë", nil, "hi ZOË!", []Span{{3, 7}}},            // multi-byte, any case
		{"Kit", []string{"(Ash)"}, "hello (Ash) here", []Span{{6, 11}}}, // literal regex specials
		{"Kit", nil, "Rook waves.", nil},
	}
	for _, tc := range cases {
		c, err := New(nil, tc.name, tc.aliases)
		if err != nil {
			t.Fatal(err)
		}
		var got []Span
		for _, tag := range c.Tags(tc.in) {
			if tag.Name == SelfTag {
				got = tag.Spans
			}
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: self spans = %v, want %v", tc.in, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/classify`
Expected: build failure: `c.Tags undefined`, `undefined: Tag`, `undefined: Span`.

- [ ] **Step 3: Implement**

Replace everything in `classify.go` from `// Classifier holds` to the end with:

```go
// Classifier holds one character's compiled classify rules.
type Classifier struct {
	rules []rule
	names *regexp.Regexp // the character's names, longest first; nil if none
}

type rule struct {
	tag string
	re  *regexp.Regexp
}

// Span is a byte range [Start, End) of a line's plain text.
type Span struct{ Start, End int }

// Tag is a tag on a line and where in the line it matched.
type Tag struct {
	Name  string
	Spans []Span // nil if every match was empty (e.g. a bare `^`)
}

// New compiles rules plus the built-in self rule for name and aliases.
func New(rules []config.ClassifyRule, name string, aliases []string) (*Classifier, error) {
	c := &Classifier{}
	for _, r := range rules {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, err
		}
		c.rules = append(c.rules, rule{r.Tag, re})
	}
	var names []string
	for _, n := range append([]string{name}, aliases...) {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}
	if len(names) > 0 {
		// Longest first, so "Kitty" is tried before "Kit".
		slices.SortStableFunc(names, func(a, b string) int { return len(b) - len(a) })
		for i, n := range names {
			names[i] = regexp.QuoteMeta(n)
		}
		c.names = regexp.MustCompile(`(?i)(?:` + strings.Join(names, "|") + `)`)
	}
	return c, nil
}

// Classify returns the tags for plain (ANSI-stripped) text, in rule order,
// without duplicates, with SelfTag last.
func (c *Classifier) Classify(plain string) []string {
	var names []string
	for _, t := range c.Tags(plain) {
		names = append(names, t.Name)
	}
	return names
}

// Tags is Classify with where each tag matched: every non-empty match of
// each of its rules, in rule order, then by position.
func (c *Classifier) Tags(plain string) []Tag {
	var tags []Tag
	at := map[string]int{} // tag name → index in tags
	for _, r := range c.rules {
		if !r.re.MatchString(plain) {
			continue
		}
		i, ok := at[r.tag]
		if !ok {
			i = len(tags)
			at[r.tag] = i
			tags = append(tags, Tag{Name: r.tag})
		}
		for _, m := range r.re.FindAllStringIndex(plain, -1) {
			if m[0] < m[1] {
				tags[i].Spans = append(tags[i].Spans, Span{m[0], m[1]})
			}
		}
	}
	if _, ok := at[SelfTag]; !ok {
		if spans := c.selfSpans(plain); spans != nil {
			tags = append(tags, Tag{Name: SelfTag, Spans: spans})
		}
	}
	return tags
}

// selfSpans finds each whole-word mention of the character's names.
// Word boundaries are Unicode-aware (Go's \b is ASCII-only, which would
// miss names like "Zoë") and checked by hand, so a span covers just the
// name and back-to-back mentions are each found.
func (c *Classifier) selfSpans(plain string) []Span {
	if c.names == nil {
		return nil
	}
	var spans []Span
	for _, m := range c.names.FindAllStringIndex(plain, -1) {
		before, _ := utf8.DecodeLastRuneInString(plain[:m[0]])
		after, _ := utf8.DecodeRuneInString(plain[m[1]:])
		if m[0] > 0 && isWordRune(before) || m[1] < len(plain) && isWordRune(after) {
			continue
		}
		spans = append(spans, Span{m[0], m[1]})
	}
	return spans
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}
```

Update the imports to `"regexp"`, `"slices"`, `"strings"`, `"unicode"`, `"unicode/utf8"`, and `config`.

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/classify`
Expected: `ok` (the existing `TestClassify`, `TestSelfNamesAreLiteralAndUnicode` and `TestNoNamesNoSelf` still pass).

- [ ] **Step 5: Run everything and commit**

Run: `gofmt -l . && go vet ./... && go test -race ./...`
Expected: all `ok`.

```bash
git add internal/classify
git commit -m "feat(classify): tags remember where they matched

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01X5RzCDj8AoLniuQdFBZC4E"
```

---

### Task 3: Highlight runs

**Files:**
- Modify: `internal/rules/rules.go`
- Modify: `internal/ui/model.go` (`renderLine`: pass `cls.Tags(plain)`)
- Modify: `cmd/kiln/main.go` (tail: pass `cls.Tags(plain)`)
- Test: `internal/rules/rules_test.go`

**Interfaces:**
- Consumes: `classify.Tag`, `classify.Span`, `Classifier.Tags` (Task 2); `HighlightRule.Scope` (Task 1).
- Produces:
  - `type Run struct { Start, End int; Style config.Style; Styled bool }`
  - `Result` gains `Runs []Run`: nil when only whole-line rules applied (then `Style` is the line's style, as today); otherwise runs cover `[0, len(plain))` in order, adjacent runs never equal, and `Style` is zero.
  - `func (h *Highlighter) Apply(plain string, tags []classify.Tag) Result`

- [ ] **Step 1: Write the failing tests**

In `rules_test.go`:

1. Add imports `"reflect"` and `"github.com/latrani/Kiln/internal/classify"`.
2. Add helpers above `TestApply`:

```go
// line is a whole-line Result.
func line(s config.Style, styled, attention bool) Result {
	return Result{Style: s, Styled: styled, Attention: attention}
}

// tagged makes span-less tags from names.
func tagged(names ...string) []classify.Tag {
	var tags []classify.Tag
	for _, n := range names {
		tags = append(tags, classify.Tag{Name: n})
	}
	return tags
}
```

3. In `TestApply`, change the case field `tags []string` to `tags []classify.Tag`; in the **cases** (not the rules passed to `New`, whose `Match.Tags` stay `[]string`), replace each `[]string{...}` tag list with `tagged(...)`; replace each positional `Result{A, B, C}` with `line(A, B, C)`; and change the comparison to `if got := h.Apply(c.plain, c.tags); !reflect.DeepEqual(got, c.want) {`.
4. Append:

```go
func TestApplyMatchScope(t *testing.T) {
	blue := config.Style{FG: "#2053ff", Bold: true}
	pageSpans := []classify.Tag{{Name: "page", Spans: []classify.Span{{Start: 0, End: 5}}}}
	const plain = "PAGE: hi Kit" // PAGE: 0-5, Kit 9-12

	t.Run("match and line rules fold per character", func(t *testing.T) {
		h, _ := New([]config.HighlightRule{
			{Match: config.Match{Tags: []string{"page"}}, Style: blue, Scope: "match", Attention: true},
			{Match: config.Match{Pattern: `Kit`}, Style: config.Style{Underline: true}, Scope: "match"},
			{Match: config.Match{Tags: []string{"page"}}, Style: config.Style{Italic: true}}, // whole line
		})
		want := Result{Styled: true, Attention: true, Runs: []Run{
			{Start: 0, End: 5, Style: config.Style{FG: "#2053ff", Bold: true, Italic: true}, Styled: true},
			{Start: 5, End: 9, Style: config.Style{Italic: true}, Styled: true},
			{Start: 9, End: 12, Style: config.Style{Italic: true, Underline: true}, Styled: true},
		}}
		if got := h.Apply(plain, pageSpans); !reflect.DeepEqual(got, want) {
			t.Errorf("Apply = %+v\nwant    %+v", got, want)
		}
	})

	t.Run("unstyled gaps", func(t *testing.T) {
		h, _ := New([]config.HighlightRule{{Match: config.Match{Tags: []string{"page"}}, Style: blue, Scope: "match"}})
		want := Result{Styled: true, Runs: []Run{
			{Start: 0, End: 5, Style: blue, Styled: true},
			{Start: 5, End: 12},
		}}
		if got := h.Apply(plain, pageSpans); !reflect.DeepEqual(got, want) {
			t.Errorf("Apply = %+v", got)
		}
	})

	t.Run("a pattern's matches beat its tags' spans", func(t *testing.T) {
		h, _ := New([]config.HighlightRule{{Match: config.Match{Tags: []string{"page"}, Pattern: `hi`}, Style: blue, Scope: "match"}})
		want := Result{Styled: true, Runs: []Run{{Start: 0, End: 6}, {Start: 6, End: 8, Style: blue, Styled: true}, {Start: 8, End: 12}}}
		if got := h.Apply(plain, pageSpans); !reflect.DeepEqual(got, want) {
			t.Errorf("Apply = %+v", got)
		}
	})

	t.Run("overlapping spans merge", func(t *testing.T) {
		h, _ := New([]config.HighlightRule{{Match: config.Match{Tags: []string{"page"}}, Style: blue, Scope: "match"}})
		tags := []classify.Tag{{Name: "page", Spans: []classify.Span{{Start: 0, End: 4}, {Start: 0, End: 5}}}}
		want := Result{Styled: true, Runs: []Run{{Start: 0, End: 5, Style: blue, Styled: true}, {Start: 5, End: 12}}}
		if got := h.Apply(plain, tags); !reflect.DeepEqual(got, want) {
			t.Errorf("Apply = %+v", got)
		}
	})

	t.Run("no spans: no style, attention still counts", func(t *testing.T) {
		h, _ := New([]config.HighlightRule{{Match: config.Match{Tags: []string{"start"}}, Style: blue, Scope: "match", Attention: true}})
		if got := h.Apply(plain, tagged("start")); !reflect.DeepEqual(got, Result{Attention: true}) {
			t.Errorf("Apply = %+v", got)
		}
	})
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/rules`
Expected: build failure: `undefined: Run`, `unknown field Runs`, and `Apply` argument type mismatch.

- [ ] **Step 3: Implement**

Replace `rules.go` below the package doc with:

```go
import (
	"regexp"
	"slices"

	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/config"
)

// Highlighter holds one character's compiled highlight rules.
type Highlighter struct {
	rules []compiled
}

type compiled struct {
	tags      []string
	re        *regexp.Regexp // nil = no pattern constraint
	style     config.Style
	attention bool
	match     bool // scope = "match": style only the rule's spans
}

// Run is a stretch of a line's plain text and the style it gets.
type Run struct {
	Start, End int
	Style      config.Style
	Styled     bool // false: left as the server sent it
}

// Result is the combined effect of every rule that applies to a line.
type Result struct {
	Style     config.Style // the whole line's style, when Runs is nil
	Styled    bool         // some rule styled some of the line
	Attention bool
	Runs      []Run // with match-scope rules: the line, cut into styled runs
}

// New compiles rules.
func New(rules []config.HighlightRule) (*Highlighter, error) {
	h := &Highlighter{}
	for _, r := range rules {
		c := compiled{tags: r.Match.Tags, style: r.Style, attention: r.Attention, match: r.Scope == "match"}
		if r.Match.Pattern != "" {
			re, err := regexp.Compile(r.Match.Pattern)
			if err != nil {
				return nil, err
			}
			c.re = re
		}
		h.rules = append(h.rules, c)
	}
	return h, nil
}

// hit is a rule that applies to a line, with its spans (nil: the whole line).
type hit struct {
	style config.Style
	spans []classify.Span
}

// Apply evaluates every rule against a line's plain text and tags. A rule
// applies when the line has one of its tags (if any) and matches its
// pattern (if any). Each character's style folds the rules that cover it,
// in order: later non-empty colors override earlier ones, and
// bold/italic/underline/attention accumulate. A whole-line rule covers
// every character; a match-scope rule covers its pattern's matches, or
// else its tags' spans.
func (h *Highlighter) Apply(plain string, tags []classify.Tag) Result {
	var res Result
	var hits []hit
	partial := false
	for _, r := range h.rules {
		if len(r.tags) > 0 && !slices.ContainsFunc(tags, func(t classify.Tag) bool { return slices.Contains(r.tags, t.Name) }) {
			continue
		}
		if r.re != nil && !r.re.MatchString(plain) {
			continue
		}
		res.Attention = res.Attention || r.attention
		if !r.match {
			hits = append(hits, hit{style: r.style})
			continue
		}
		if spans := r.spans(plain, tags); len(spans) > 0 {
			hits = append(hits, hit{style: r.style, spans: spans})
			partial = true
		}
	}
	if len(hits) == 0 {
		return res
	}
	res.Styled = true
	if !partial {
		for _, h := range hits {
			res.Style = fold(res.Style, h.style)
		}
		return res
	}
	cuts := []int{0, len(plain)}
	for _, h := range hits {
		for _, s := range h.spans {
			cuts = append(cuts, s.Start, s.End)
		}
	}
	slices.Sort(cuts)
	cuts = slices.Compact(cuts)
	for i := 0; i+1 < len(cuts); i++ {
		run := Run{Start: cuts[i], End: cuts[i+1]}
		for _, h := range hits {
			if h.spans == nil || covers(h.spans, run.Start, run.End) {
				run.Style, run.Styled = fold(run.Style, h.style), true
			}
		}
		if n := len(res.Runs); n > 0 && res.Runs[n-1].Style == run.Style && res.Runs[n-1].Styled == run.Styled {
			res.Runs[n-1].End = run.End
			continue
		}
		res.Runs = append(res.Runs, run)
	}
	return res
}

// spans is where a match-scope rule styles: its pattern's non-empty
// matches, or else the spans of its tags on the line.
func (r compiled) spans(plain string, tags []classify.Tag) []classify.Span {
	var out []classify.Span
	if r.re != nil {
		for _, m := range r.re.FindAllStringIndex(plain, -1) {
			if m[0] < m[1] {
				out = append(out, classify.Span{Start: m[0], End: m[1]})
			}
		}
		return out
	}
	for _, t := range tags {
		if slices.Contains(r.tags, t.Name) {
			out = append(out, t.Spans...)
		}
	}
	return out
}

// covers reports whether [start, end) lies inside one of spans. Runs are
// cut at every span edge, so a run is wholly inside a span or outside it.
func covers(spans []classify.Span, start, end int) bool {
	return slices.ContainsFunc(spans, func(s classify.Span) bool { return s.Start <= start && end <= s.End })
}

// fold layers b over a: b's non-empty colors win, attributes accumulate.
func fold(a, b config.Style) config.Style {
	if b.FG != "" {
		a.FG = b.FG
	}
	if b.BG != "" {
		a.BG = b.BG
	}
	a.Bold = a.Bold || b.Bold
	a.Italic = a.Italic || b.Italic
	a.Underline = a.Underline || b.Underline
	return a
}
```

- [ ] **Step 4: Update the callers**

`internal/ui/model.go` `renderLine`: change `res := hl.Apply(plain, cls.Classify(plain))` to `res := hl.Apply(plain, cls.Tags(plain))`.

`cmd/kiln/main.go` (tail's event loop): change `res = hl.Apply(plain, cls.Classify(plain))` to `res = hl.Apply(plain, cls.Tags(plain))`.

(Runs aren't drawn yet; Task 4 does that. Until then a match-scope rule draws nothing, and existing configs, which have no `scope`, are unaffected.)

- [ ] **Step 5: Run to verify**

Run: `go test ./internal/rules`
Expected: `ok`.

Run: `gofmt -l . && go vet ./... && go test -race ./...`
Expected: all `ok`.

- [ ] **Step 6: Commit**

```bash
git add internal/rules internal/ui/model.go cmd/kiln/main.go
git commit -m "feat(rules): match-scope highlight rules style runs of a line

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01X5RzCDj8AoLniuQdFBZC4E"
```

---

### Task 4: Drawing runs

**Files:**
- Modify: `internal/ansi/strip.go` (export `EscapeEnd`)
- Modify: `internal/style/style.go` (`Highlight`)
- Modify: `internal/ui/model.go` (`renderLine`)
- Modify: `cmd/kiln/render.go`
- Test: `internal/style/style_test.go`, `internal/ui/model_test.go`, `cmd/kiln/render_test.go`

**Interfaces:**
- Consumes: `rules.Result`, `rules.Run` (Task 3).
- Produces:
  - `func ansi.EscapeEnd(s string, i int) int`
  - `func style.Highlight(text string, res rules.Result) string`: `text` is server text whose `ansi.Strip` is the plain text `res` was computed on.

- [ ] **Step 1: Write the failing tests**

Append to `internal/style/style_test.go` (add imports `"github.com/latrani/Kiln/internal/rules"`):

```go
func TestHighlight(t *testing.T) {
	blue := config.Style{FG: "#2053ff"}
	b := SGR(blue)
	styled := func(start, end int, s config.Style) rules.Run {
		return rules.Run{Start: start, End: end, Style: s, Styled: true}
	}
	plain := func(start, end int) rules.Run { return rules.Run{Start: start, End: end} }
	runs := func(rs ...rules.Run) rules.Result { return rules.Result{Styled: true, Runs: rs} }
	cases := []struct {
		name, text string
		res        rules.Result
		want       string
	}{
		{"prefix, then the server's color comes back", "\x1b[32mPAGE: hi",
			runs(styled(0, 5, blue), plain(5, 9)),
			"\x1b[32m" + b + "PAGE:" + Reset + "\x1b[32m hi" + Reset},
		{"a server reset inside a span doesn't end it", "\x1b[1mPA\x1b[0mGE: hi",
			runs(styled(0, 5, blue), plain(5, 9)),
			"\x1b[1m" + b + "PA\x1b[0m" + b + "GE:" + Reset + " hi" + Reset},
		{"a span in the middle", "hi Kit!",
			runs(plain(0, 3), styled(3, 6, config.Style{Underline: true}), plain(6, 7)),
			"hi \x1b[4mKit" + Reset + "!" + Reset},
		{"multi-byte", "hi Zoë!",
			runs(plain(0, 3), styled(3, 7, config.Style{Underline: true}), plain(7, 8)),
			"hi \x1b[4mZoë" + Reset + "!" + Reset},
		{"non-SGR escapes don't shift spans", "\x1b]0;title\x07PAGE: hi",
			runs(styled(0, 5, blue), plain(5, 9)),
			"\x1b]0;title\x07" + b + "PAGE:" + Reset + " hi" + Reset},
		{"span to the end, then a trailing reset", "PAGE:\x1b[0m",
			runs(styled(0, 5, blue)),
			b + "PAGE:\x1b[0m" + b + Reset},
		{"unstyled", "\x1b[31mhi", rules.Result{}, "\x1b[31mhi" + Reset},
		{"whole line", "hi", rules.Result{Styled: true, Style: config.Style{Italic: true}}, "\x1b[3mhi" + Reset},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Highlight(c.text, c.res); got != c.want {
				t.Errorf("Highlight = %q\nwant        %q", got, c.want)
			}
		})
	}
}
```

Append to `internal/ui/model_test.go` (add imports for `classify`, `rules`, `logstore`, `style` and `config` if missing):

```go
func TestRenderLineMatchScope(t *testing.T) {
	cls, _ := classify.New([]config.ClassifyRule{{Tag: "page", Pattern: `^PAGE:`}}, "Kit", nil)
	blue := config.Style{FG: "#2053ff", Bold: true}
	hl, _ := rules.New([]config.HighlightRule{{Match: config.Match{Tags: []string{"page"}}, Style: blue, Scope: "match", Attention: true}})
	got, attn := renderLine(cls, hl, logstore.Entry{Dir: logstore.In, Text: "PAGE: Mira says hi"})
	if want := style.SGR(blue) + "PAGE:" + style.Reset + " Mira says hi" + style.Reset; got != want || !attn {
		t.Errorf("renderLine = %q, %v; want %q, true", got, attn, want)
	}
}
```

In `cmd/kiln/render_test.go`, add a case:

```go
		{"partial", in("PAGE: hi"),
			rules.Result{Styled: true, Attention: true, Runs: []rules.Run{
				{Start: 0, End: 5, Style: config.Style{Bold: true}, Styled: true}, {Start: 5, End: 8}}},
			"» \x1b[1mPAGE:\x1b[0m hi\x1b[0m"},
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/style ./internal/ui ./cmd/kiln`
Expected: `internal/style` fails to build (`undefined: Highlight`); after that exists, `TestRenderLineMatchScope` and the `partial` render case fail (runs not drawn).

- [ ] **Step 3: Export `EscapeEnd`**

In `internal/ansi/strip.go`, after `skipEscape`:

```go
// EscapeEnd returns the index just past the escape sequence that starts
// at s[i] (which must be ESC), exactly as Strip skips it.
func EscapeEnd(s string, i int) int { return skipEscape(s, i) }
```

- [ ] **Step 4: Write `Highlight`**

In `internal/style/style.go` (add imports `"github.com/latrani/Kiln/internal/ansi"` and `"github.com/latrani/Kiln/internal/rules"`):

```go
// Highlight draws res over server text: the whole line in res.Style, or
// each styled run in its style. Where a run ends mid-line, the server's
// own SGR state is restored; a server reset inside a run re-applies the
// run's style. text's ANSI-stripped form must be the plain text res was
// computed on. The result ends with Reset.
func Highlight(text string, res rules.Result) string {
	if !res.Styled {
		return text + Reset
	}
	if res.Runs == nil {
		return Apply(text, res.Style)
	}
	var b, server strings.Builder // server: its SGR since its last reset
	runs, ri, pos := res.Runs, 0, 0
	ours := "" // our SGR, while inside a styled run
	for i := 0; i < len(text); {
		if text[i] == 0x1b {
			j := ansi.EscapeEnd(text, i)
			seq := text[i:j]
			b.WriteString(seq)
			if params, ok := sgrParams(seq); ok {
				if first, _, _ := strings.Cut(params, ";"); first == "" || first == "0" {
					server.Reset()
					if params != "" && params != "0" {
						server.WriteString(seq) // e.g. ESC[0;31m: reset, then red
					}
					b.WriteString(ours)
				} else {
					server.WriteString(seq)
				}
			}
			i = j
			continue
		}
		for ri < len(runs) && pos >= runs[ri].End {
			if ours != "" {
				b.WriteString(Reset + server.String())
				ours = ""
			}
			ri++
		}
		if ours == "" && ri < len(runs) && runs[ri].Styled {
			ours = SGR(runs[ri].Style)
			b.WriteString(ours)
		}
		b.WriteByte(text[i])
		i++
		pos++
	}
	return b.String() + Reset
}

// sgrParams returns the parameters of an SGR sequence (ESC [ … m).
func sgrParams(seq string) (string, bool) {
	if len(seq) < 3 || seq[1] != '[' || seq[len(seq)-1] != 'm' {
		return "", false
	}
	return seq[2 : len(seq)-1], true
}
```

- [ ] **Step 5: Draw through it**

`internal/ui/model.go` `renderLine`: replace

```go
	res := hl.Apply(plain, cls.Tags(plain))
	if !res.Styled {
		return text + style.Reset, res.Attention
	}
	return style.Apply(text, res.Style), res.Attention
```

with

```go
	res := hl.Apply(plain, cls.Tags(plain))
	return style.Highlight(text, res), res.Attention
```

`cmd/kiln/render.go`: replace the last three statements

```go
	if !res.Styled {
		return marker + e.Text + style.Reset
	}
	return style.Apply(marker+e.Text, res.Style)
```

with

```go
	if res.Runs != nil {
		return marker + style.Highlight(e.Text, res) // spans index e.Text, not the marker
	}
	if !res.Styled {
		return marker + e.Text + style.Reset
	}
	return style.Apply(marker+e.Text, res.Style)
```

- [ ] **Step 6: Run to verify they pass**

Run: `go test ./internal/style ./internal/ui ./cmd/kiln`
Expected: `ok` for all three.

Run: `gofmt -l . && go vet ./... && go test -race ./...`
Expected: all `ok`.

- [ ] **Step 7: Commit**

```bash
git add internal/ansi internal/style internal/ui/model.go internal/ui/model_test.go cmd/kiln
git commit -m "feat: draw match-scope highlights over the server's colors

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01X5RzCDj8AoLniuQdFBZC4E"
```

---

### Task 5: README

**Files:**
- Modify: `README.md` (the `### Rules` section)

- [ ] **Step 1: Document `scope`**

After the paragraph starting "Tags are worked out when lines are shown", add:

````markdown
By default a highlight styles the whole line. With `scope = "match"` it styles only the part that matched: its own `pattern`'s matches, or else the text its tags' classify rules matched. So a server that prefixes pages with `PAGE:` can color just the prefix, and `self` can bold just your name:

```toml
[[classify]]
tag = "page"
pattern = '^PAGE:'

[[highlight]]
match = { tags = ["page"] }
style = { fg = "#2053ff", bold = true }
scope = "match"                      # just "PAGE:"; attention still marks the line
attention = true

[[highlight]]
match = { tags = ["self"] }
style = { bold = true }
scope = "match"                      # just your name, wherever it appears
```
````

- [ ] **Step 2: Check it**

Run: `grep -n 'scope = "match"' README.md`
Expected: the two example lines.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: highlight scope

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01X5RzCDj8AoLniuQdFBZC4E"
```
