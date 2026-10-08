# Headless core, step 1: lines — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lines become theme-free data (`app.Line`) that the TUI paints, instead of ANSI strings baked at arrival.

**Architecture:** `rules` splits into `Judge` (attention/quiet from tag names, core) and `Highlighter` (tag styles from the theme, paint time). A new package `internal/app` starts with just `Line` and `Rules`, which turn a `logstore.Entry` into a `Line`. `internal/ui` keeps an `app.Line` on every scrollback and log-mode line and paints it; a theme change repaints, a rules change remakes lines from their entries and repaints.

**Tech Stack:** Go, Bubble Tea v2 (only in `ui`), existing `classify`, `rules`, `style`, `ansi`, `logstore`, `theme`.

**Spec:** `docs/superpowers/specs/2026-10-07-headless-core-design.md` (sections "Lines" and "Order of work", step 1)

## Global Constraints

- Nothing visible changes: the golden screens in `internal/ui/testdata/golden` are NOT updated (never run `-update`).
- Every existing test keeps passing. Tests may change how they set up state (e.g. constructing a line), never what they expect.
- `internal/app` doesn't depend on `charm.land/bubbletea` or lipgloss and never calls `theme.Paint` or `theme.Fill`.
- User-facing strings live in `internal/str/locales/en.toml` (see CLAUDE.md); this step adds none. `go test ./internal/str` must pass.
- Colors come from theme roles, never escape codes, in `internal/ui` (`TestNoHardCodedStyles`).
- Run the whole suite with `go test ./...` from the repo root before each commit.

## Review Focus

1. **Theme changes while older history is being read** — a page of scrollback or a log-mode day read under the old theme must arrive painted in the new one (existing tests in `theme_test.go` cover both; they must still pass unchanged).
2. **Editing a character's rules** (a classify pattern, attention list) — lines already on screen pick up the new tags' looks, as they do today. Task 3 adds a test.
3. **Day dividers** — still repaint on a theme change, and `Scrollback.LastTime` still skips them (it only looks at lines with an entry). Task 3 adds a test.
4. **Quiet lines** — still never count as unseen and never notify; sent lines with `local_echo` off still don't show. Covered by existing `ui` tests.
5. **Lines you sent and Kiln's sys lines** — never classified (no tags, no attention), same as today. Task 2 adds a test.

---

### Task 1: Split `rules` into `Judge` and `Highlighter`

**Files:**
- Modify: `internal/rules/rules.go`
- Modify: `internal/rules/rules_test.go`
- Modify: `internal/ui/model.go` (`compile`, `renderLine`, `handleEvent`)
- Modify: `internal/ui/notify.go` (`notifyCmd`)
- Modify: `internal/ui/browse.go` (nothing but compiling: `makeLine` ignores the verdict)
- Modify: `internal/ui/model_test.go` (`TestRenderLineMatchScope` setup)

**Interfaces:**
- Produces:
  - `type rules.Judge struct{ Attention, Quiet []string }`
  - `type rules.Verdict struct{ Attention, Quiet bool }`
  - `func (j rules.Judge) Of(tags []classify.Tag) rules.Verdict`
  - `func rules.New(th *theme.Theme) *rules.Highlighter`
  - `func (h *rules.Highlighter) Runs(plain string, tags []classify.Tag) []style.Run`
  - `func (h *rules.Highlighter) Styled(tag string) (string, bool)` (unchanged)
  - `rules.Result` and `Highlighter.Apply` are removed.

- [ ] **Step 1: Rewrite the rules tests against the new API**

In `internal/rules/rules_test.go`, keep `mustTheme`, `tag` and `sgr`. Replace the five tests with:

```go
func TestWholeLineTags(t *testing.T) {
	th := mustTheme(t, `"page/in" = { fg = "#ff9f43", bold = true }
self = { italic = true }`)
	h := New(th)
	plain := "Mira pages: hi Kit"
	if got := h.Runs("Rook waves.", nil); got != nil {
		t.Errorf("untagged line styled: %+v", got)
	}
	got := h.Runs(plain, []classify.Tag{tag("page"), tag("page/in"), tag("self", classify.Span{Start: 15, End: 18})})
	want := []style.Run{{Start: 0, End: len(plain), SGR: sgr(t, th, "page/in", "self")}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("Runs = %+v, want %+v", got, want)
	}
}

func TestMatchScopeOnTopOfLine(t *testing.T) {
	th := mustTheme(t, `highlight = { fg = "#ffd166", scope = "match" }
page = { italic = true }`)
	h := New(th)
	// The match-scope tag comes first on the line but still draws on top.
	got := h.Runs("Mira pages: the lighthouse", []classify.Tag{tag("highlight", classify.Span{Start: 16, End: 26}), tag("page", classify.Span{Start: 0, End: 11})})
	want := []style.Run{
		{Start: 0, End: 16, SGR: sgr(t, th, "page")},
		{Start: 16, End: 26, SGR: sgr(t, th, "page", "highlight")},
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Runs = %+v, want %+v", got, want)
	}
}

func TestMatchScopeWithoutSpans(t *testing.T) {
	th := mustTheme(t, `highlight = { fg = "#ffd166", scope = "match" }`)
	if got := New(th).Runs("x", []classify.Tag{tag("highlight")}); got != nil {
		t.Errorf("a match-scope tag with no spans styled the line: %+v", got)
	}
}

func TestListsMatchTagAndChildren(t *testing.T) {
	j := Judge{Attention: []string{"page"}}
	for _, c := range []struct {
		tag  string
		want bool
	}{{"page", true}, {"page/in", true}, {"page/in/x", true}, {"pages", false}, {"whisper", false}} {
		if got := j.Of([]classify.Tag{tag(c.tag)}).Attention; got != c.want {
			t.Errorf("attention for %q = %v, want %v", c.tag, got, c.want)
		}
	}
}

func TestQuietWinsOverAttention(t *testing.T) {
	j := Judge{Attention: []string{"self", "spam"}, Quiet: []string{"spam"}}
	for _, tags := range [][]classify.Tag{{tag("spam")}, {tag("self"), tag("spam")}} {
		v := j.Of(tags)
		if !v.Quiet || v.Attention {
			t.Errorf("%v: quiet %v attention %v, want quiet only", tags, v.Quiet, v.Attention)
		}
	}
}

// The verdict doesn't depend on the theme: a tag with no style still
// asks for attention.
func TestJudgeIgnoresStyles(t *testing.T) {
	if !(Judge{Attention: []string{"page"}}).Of([]classify.Tag{tag("page")}).Attention {
		t.Error("an unstyled tag in the attention list didn't ask for attention")
	}
}
```

- [ ] **Step 2: Run the rules tests to see them fail**

Run: `go test ./internal/rules`
Expected: build failure — `undefined: Judge`, `h.Runs undefined`, `too many arguments in call to New` style errors.

- [ ] **Step 3: Implement the split in `internal/rules/rules.go`**

Replace the package doc, `Highlighter`, `Result`, `New` and `Apply` (keep `in`, `hit`, `b2i`, `covers`, `Styled`):

```go
// Package rules works out what a classified line asks for (Judge:
// attention and quiet, from tag lists) and how it looks (Highlighter:
// its tags' styles from the theme).
package rules

// Judge decides what a line asks for from its tags. It doesn't depend
// on the theme.
type Judge struct {
	Attention, Quiet []string
}

// Verdict is what a line asks for.
type Verdict struct {
	Attention bool // never with Quiet
	Quiet     bool // not unread, no attention, no notification
}

// Of works out what a line with tags asks for. A line asks for attention
// (or is quiet) when one of its tags is in the list, or is under one:
// "page" covers "page/in". Quiet wins over attention.
func (j Judge) Of(tags []classify.Tag) Verdict {
	var v Verdict
	for _, t := range tags {
		v.Attention = v.Attention || in(j.Attention, t.Name)
		v.Quiet = v.Quiet || in(j.Quiet, t.Name)
	}
	if v.Quiet {
		v.Attention = false
	}
	return v
}

// Highlighter draws a character's tags in a theme's styles.
type Highlighter struct {
	th *theme.Theme
}

// New makes a Highlighter drawing tags in th's styles.
func New(th *theme.Theme) *Highlighter {
	return &Highlighter{th: th}
}
```

and change `Apply` into `Runs` (same algorithm, attention/quiet removed):

```go
// Runs works out plain's styled runs from its tags; nil means the line
// as the server sent it. Each tag takes the style of the most specific
// styled name up its slashes; tags that land on the same name count
// once. Whole-line styles fold first, in tag order, then match-scope
// ones on top of the text they matched. Later colors win and attributes
// add up.
func (h *Highlighter) Runs(plain string, tags []classify.Tag) []style.Run {
	var hits []hit
	at := map[string]int{}
	for _, t := range tags {
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
	hits = slices.DeleteFunc(hits, func(h hit) bool { return h.ts.Match && len(h.spans) == 0 })
	if len(hits) == 0 {
		return nil
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
	var runs []style.Run
	for i := 0; i+1 < len(cuts); i++ {
		var st theme.Style
		for _, h := range hits {
			if !h.ts.Match || covers(h.spans, cuts[i], cuts[i+1]) {
				st = st.Over(h.ts.Style)
			}
		}
		run := style.Run{Start: cuts[i], End: cuts[i+1], SGR: st.SGR()}
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

- [ ] **Step 4: Run the rules tests**

Run: `go test ./internal/rules`
Expected: PASS.

- [ ] **Step 5: Update `ui` to the new API**

`internal/ui/model.go`:
- In `charState`, after `hl *rules.Highlighter` add `judge rules.Judge`.
- In `compile`, replace the last assignment with:

```go
	cs.cls, cs.hl = cls, rules.New(th)
	cs.judge = rules.Judge{Attention: cs.ch.Rules.Attention, Quiet: cs.ch.Rules.Quiet}
```

- Change `render` and `renderLine` to return a `rules.Verdict` and take the judge:

```go
func (cs *charState) render(e logstore.Entry) (string, rules.Verdict) {
	return renderLine(cs.cls, cs.judge, cs.hl, e)
}

// renderLine styles e with the given rules. It only reads its arguments
// (compiled once, never mutated), so it is safe off the UI goroutine.
func renderLine(cls *classify.Classifier, judge rules.Judge, hl *rules.Highlighter, e logstore.Entry) (string, rules.Verdict) {
	text := ansi.Sanitize(e.Text)
	switch e.Dir {
	case logstore.Out:
		return theme.Paint(theme.ScrollbackEcho, gutterMark+text), rules.Verdict{}
	case logstore.Sys:
		return theme.Paint(theme.ScrollbackSys, "* "+text), rules.Verdict{}
	}
	plain := ansi.Strip(text)
	tags := cls.Tags(plain)
	return style.Highlight(text, hl.Runs(plain, tags)), judge.Of(tags)
}
```

- Thread `judge` through `renderDays` (both the method and the function: add `judge rules.Judge` after `cls`) and `pageOlder` (capture `judge := cs.judge` alongside `cls, hl`), and through `makeLine`/`readOlder` in `browse.go` the same way (`makeLine(cls, judge, hl, e)`, `readOlder(h, cls, judge, hl)`; `b.cs.judge` at the call sites in `newLine`, `loadOlder`, `requestOlder`).

`internal/ui/notify.go`: change `notifyCmd`'s last parameter to `res rules.Verdict`. Its body is unchanged.

`internal/ui/model_test.go`, `TestRenderLineMatchScope`: replace the `hl :=` and `got, res :=` lines with

```go
	hl := rules.New(th)
	got, res := renderLine(cls, rules.Judge{Attention: []string{"page"}}, hl, logstore.Entry{Dir: logstore.In, Text: "PAGE: Mira says hi"})
```

- [ ] **Step 6: Run everything**

Run: `go vet ./... && go test ./...`
Expected: PASS, including `internal/ui` goldens.

- [ ] **Step 7: Commit**

```bash
git add internal/rules internal/ui
git commit -m "rules: Judge decides attention and quiet, Highlighter only styles

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01PC4WGuqjdnGDVGdiaL3Z7n"
```

---

### Task 2: `internal/app` with `Line` and `Rules`

**Files:**
- Create: `internal/app/line.go`
- Create: `internal/app/line_test.go`
- Create: `internal/app/deps_test.go`

**Interfaces:**
- Consumes: `rules.Judge`, `rules.Verdict` (Task 1); `classify.Classifier.Tags`, `ansi.Sanitize`, `ansi.Strip`, `logstore.Entry`.
- Produces:
  - `type app.Kind int` with `app.Server`, `app.Echo`, `app.Sys`, `app.Day`
  - `type app.Line struct { Kind Kind; Entry logstore.Entry; Day string; Text, Plain string; Tags []classify.Tag; rules.Verdict }`
  - `func (l app.Line) TagNames() []string`
  - `type app.Rules struct { Classifier *classify.Classifier; Judge rules.Judge }`
  - `func (r app.Rules) Line(e logstore.Entry) app.Line`
  - `func (r app.Rules) Days(entries []logstore.Entry, echo, startsDay bool) []app.Line`
  - `func (r app.Rules) Reline(l app.Line) app.Line`
  - `func app.DayOf(t time.Time) string`

- [ ] **Step 1: Write the tests**

`internal/app/line_test.go`:

```go
package app

import (
	"slices"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/rules"
)

func testRules(t *testing.T) Rules {
	t.Helper()
	cls, err := classify.New([]config.ClassifyRule{{Tag: "page", Pattern: `^PAGE:`}}, "Kit", nil)
	if err != nil {
		t.Fatal(err)
	}
	return Rules{Classifier: cls, Judge: rules.Judge{Attention: []string{"page"}}}
}

func TestServerLineIsClassified(t *testing.T) {
	r := testRules(t)
	l := r.Line(logstore.Entry{Dir: logstore.In, Text: "PAGE: \x1b[31mhi\x1b[0m\x07"})
	if l.Kind != Server {
		t.Errorf("Kind = %v, want Server", l.Kind)
	}
	if l.Plain != "PAGE: hi" {
		t.Errorf("Plain = %q", l.Plain)
	}
	if l.Text != "PAGE: \x1b[31mhi\x1b[0m" {
		t.Errorf("Text = %q, want the server's SGR kept and the bell dropped", l.Text)
	}
	if !slices.Contains(l.TagNames(), "page") || !l.Attention {
		t.Errorf("tags %v attention %v, want page and attention", l.TagNames(), l.Attention)
	}
}

// Lines you sent and Kiln's own lines are never classified, even when
// they'd match.
func TestEchoAndSysAreNotClassified(t *testing.T) {
	r := testRules(t)
	for _, c := range []struct {
		dir  logstore.Dir
		kind Kind
	}{{logstore.Out, Echo}, {logstore.Sys, Sys}} {
		l := r.Line(logstore.Entry{Dir: c.dir, Text: "PAGE: hi"})
		if l.Kind != c.kind || l.Tags != nil || l.Attention || l.Quiet {
			t.Errorf("%c: %+v, want kind %v and no tags or verdict", c.dir, l, c.kind)
		}
	}
}

func TestDaysDividesAndSkipsEcho(t *testing.T) {
	r := testRules(t)
	d1 := time.Date(2026, 9, 24, 23, 0, 0, 0, time.Local)
	d2 := d1.Add(2 * time.Hour)
	es := []logstore.Entry{
		{Dir: logstore.In, Time: d1, Text: "a"},
		{Dir: logstore.Out, Time: d1, Text: "sent"},
		{Dir: logstore.In, Time: d2, Text: "b"},
	}
	kinds := func(ls []Line) []Kind {
		var ks []Kind
		for _, l := range ls {
			ks = append(ks, l.Kind)
		}
		return ks
	}
	if got, want := kinds(r.Days(es, false, false)), []Kind{Server, Day, Server}; !slices.Equal(got, want) {
		t.Errorf("Days(echo off, mid-day) = %v, want %v", got, want)
	}
	if got, want := kinds(r.Days(es, true, true)), []Kind{Day, Server, Echo, Day, Server}; !slices.Equal(got, want) {
		t.Errorf("Days(echo on, starts day) = %v, want %v", got, want)
	}
	if ls := r.Days(es, false, true); ls[0].Day != "2026-09-24" || ls[2].Day != DayOf(d2) {
		t.Errorf("day dividers say %q and %q", ls[0].Day, ls[2].Day)
	}
}

func TestRelineUsesNewRules(t *testing.T) {
	old := testRules(t)
	l := old.Line(logstore.Entry{Dir: logstore.In, Text: "PAGE: hi"})
	cls, err := classify.New(nil, "Kit", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := (Rules{Classifier: cls}).Reline(l); got.Attention || slices.Contains(got.TagNames(), "page") {
		t.Errorf("Reline kept the old rules' verdict: %+v", got)
	}
	day := Line{Kind: Day, Day: "2026-09-24"}
	if got := old.Reline(day); got.Kind != Day || got.Day != day.Day {
		t.Errorf("Reline(day) = %+v, want it unchanged", got)
	}
}
```

`internal/app/deps_test.go`:

```go
package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The core never touches the terminal: no Bubble Tea or lipgloss in its
// dependencies, and no painting.
func TestNoTerminal(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range strings.Fields(string(out)) {
		if strings.Contains(p, "bubbletea") || strings.Contains(p, "lipgloss") {
			t.Errorf("internal/app depends on %s", p)
		}
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range []string{"theme.Paint(", "theme.Fill(", ".Paint(", ".Fill("} {
			if strings.Contains(string(src), call) {
				t.Errorf("%s paints (%s)", f, call)
			}
		}
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/app`
Expected: build failure — `undefined: Rules`, `undefined: Server`, etc.

- [ ] **Step 3: Write `internal/app/line.go`**

```go
// Package app is what Kiln decides, apart from how any front end draws
// it. See docs/superpowers/specs/2026-10-07-headless-core-design.md.
package app

import (
	"time"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/rules"
)

// Kind is what a line is.
type Kind int

const (
	Server Kind = iota // a line from the server
	Echo               // a line you sent
	Sys                // Kiln's own note in the log (connected, closed, …)
	Day                // a divider before the first line of a day
)

// Line is a log entry and what Kiln made of it. Nothing in it depends on
// the theme: each front end paints it its own way.
type Line struct {
	Kind  Kind
	Entry logstore.Entry // zero for a Day
	Day   string         // the local day, "2006-01-02"
	Text  string         // Entry.Text sanitized, the server's SGR kept
	Plain string         // Text without SGR
	Tags  []classify.Tag // a Server line's tags, with where they matched
	rules.Verdict
}

// TagNames is the line's tag names, in order.
func (l Line) TagNames() []string {
	var names []string
	for _, t := range l.Tags {
		names = append(names, t.Name)
	}
	return names
}

// DayOf is the local day t falls on, as Line.Day has it.
func DayOf(t time.Time) string { return t.Local().Format("2006-01-02") }

// Rules are one character's compiled rules: how its lines are tagged and
// what they ask for. They're never changed after they're made, so they're
// safe to use off the front end's loop.
type Rules struct {
	Classifier *classify.Classifier
	Judge      rules.Judge
}

// Line makes e into a line. Only lines from the server are classified.
func (r Rules) Line(e logstore.Entry) Line {
	text := ansi.Sanitize(e.Text)
	l := Line{Entry: e, Day: DayOf(e.Time), Text: text, Plain: ansi.Strip(text)}
	switch e.Dir {
	case logstore.Out:
		l.Kind = Echo
	case logstore.Sys:
		l.Kind = Sys
	default:
		l.Kind = Server
		l.Tags = r.Classifier.Tags(l.Plain)
		l.Verdict = r.Judge.Of(l.Tags)
	}
	return l
}

// Days makes entries into lines with a Day divider before the first line
// of each day. The very first entry gets one only if startsDay, i.e. it
// really is the first line of its day. Sent lines are left out unless
// echo (the character's local_echo) is on.
func (r Rules) Days(entries []logstore.Entry, echo, startsDay bool) []Line {
	out := make([]Line, 0, len(entries)+2)
	prev := ""
	for i, e := range entries {
		if e.Dir == logstore.Out && !echo {
			continue
		}
		day := DayOf(e.Time)
		if day != prev && (i > 0 || startsDay) {
			out = append(out, Line{Kind: Day, Day: day})
		}
		prev = day
		out = append(out, r.Line(e))
	}
	return out
}

// Reline is l made again from its entry under r, for when a character's
// rules change. A Day divider has no entry and comes back as it was.
func (r Rules) Reline(l Line) Line {
	if l.Kind == Day {
		return l
	}
	return r.Line(l.Entry)
}
```

Note: `renderDays` today skips sent lines *before* the day check, so a day whose only line is a sent one gets no divider — `Days` keeps that exactly (the `continue` comes first).

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/app && go test ./internal/str`
Expected: PASS. (If `TestNoStrayStrings` flags anything in `line.go`, it's a bug in the code above, not a reason to add `//str:ok`: none of its literals are prose.)

- [ ] **Step 5: Commit**

```bash
git add internal/app
git commit -m "app: a Line is a log entry and what Kiln made of it, without the theme

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01PC4WGuqjdnGDVGdiaL3Z7n"
```

---

### Task 3: The TUI paints `app.Line`s

**Files:**
- Create: `internal/ui/paint.go`
- Create: `internal/ui/paint_test.go`
- Modify: `internal/ui/model.go` (`charState`, `compile`, `loadTheme`, `applyConfig`, `preload`, `pageOlder`, `renderDays`, `render`/`renderLine`, the `sbOlderMsg` case in `update`, `handleEvent`)
- Modify: `internal/ui/scrollback.go` (`sbLine`, `entryLine`, `Rerender`, `restyled`, `LastTime`)
- Modify: `internal/ui/browse.go` (`bline`, `newLine`, `makeLine`, `readOlder`, `receive`, `restyle`, and `l.e`/`l.day` uses)
- Modify: `internal/ui/notify.go` (`notifyCmd`)
- Modify tests that build lines: `internal/ui/scrollback_test.go` (`TestRerenderRepaintsChromeLines`), `internal/ui/sidebar_test.go` (three `entryLine` calls), `internal/ui/model_test.go` (`TestRenderLineMatchScope`)

**Interfaces:**
- Consumes: `app.Line`, `app.Rules` (`Line`, `Days`, `Reline`), `app.DayOf`, kinds (Task 2); `rules.New`, `Highlighter.Runs`, `rules.Judge` (Task 1).
- Produces (inside `ui`, used by later steps):
  - `func paint(hl *rules.Highlighter, l app.Line) string`
  - `charState.rules app.Rules` (replaces `cls` and `judge` fields), `charState.hl` unchanged
  - `sbLine.line *app.Line` (replaces `sbLine.entry`)
  - `func lineOf(text string, l app.Line) sbLine` (replaces `entryLine`)
  - `func (s *Scrollback) Rerender(reline func(app.Line) app.Line, paint func(app.Line) string)` — `reline` may be nil (repaint only)
  - `bline` embeds `app.Line`; its `e` and `day` fields are gone (`l.Entry`, `l.Day`)

- [ ] **Step 1: Write `paint`'s test**

`internal/ui/paint_test.go`:

```go
package ui

import (
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

// paint draws each kind of line the way renderLine and renderDays did.
func TestPaintKinds(t *testing.T) {
	cls, err := classify.New([]config.ClassifyRule{{Tag: "page", Pattern: `^PAGE:`}}, "Kit", nil)
	if err != nil {
		t.Fatal(err)
	}
	th, err := theme.FromTOML("[tags]\npage = { fg = \"#2053ff\", bold = true, scope = \"match\" }\n")
	if err != nil {
		t.Fatal(err)
	}
	r, hl := app.Rules{Classifier: cls}, rules.New(th)
	server := r.Line(logstore.Entry{Dir: logstore.In, Text: "PAGE: hi"})
	if got, want := paint(hl, server), style.Highlight(server.Text, hl.Runs(server.Plain, server.Tags)); got != want {
		t.Errorf("server line = %q, want %q", got, want)
	}
	echo := r.Line(logstore.Entry{Dir: logstore.Out, Text: "hi"})
	if got, want := paint(hl, echo), theme.Paint(theme.ScrollbackEcho, gutterMark+"hi"); got != want {
		t.Errorf("echo line = %q, want %q", got, want)
	}
	sys := r.Line(logstore.Entry{Dir: logstore.Sys, Text: "connected"})
	if got, want := paint(hl, sys), theme.Paint(theme.ScrollbackSys, "* connected"); got != want {
		t.Errorf("sys line = %q, want %q", got, want)
	}
	day := app.Line{Kind: app.Day, Day: app.DayOf(time.Date(2026, 9, 24, 12, 0, 0, 0, time.Local))}
	if got, want := paint(hl, day), theme.Paint(theme.ScrollbackDay, "── "+dayLabel(day.Day)+" ──"); got != want {
		t.Errorf("day divider = %q, want %q", got, want)
	}
}
```

Run: `go test ./internal/ui -run TestPaintKinds`
Expected: build failure — `undefined: paint`.

- [ ] **Step 2: Write `internal/ui/paint.go`**

```go
package ui

import (
	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

// paint draws l for the terminal: a server line in its tags' styles
// from hl, over the server's own colors; Kiln's lines in their roles.
// It only reads hl, so it's safe off the UI goroutine.
func paint(hl *rules.Highlighter, l app.Line) string {
	switch l.Kind {
	case app.Echo:
		return theme.Paint(theme.ScrollbackEcho, gutterMark+l.Text)
	case app.Sys:
		return theme.Paint(theme.ScrollbackSys, "* "+l.Text)
	case app.Day:
		return theme.Paint(theme.ScrollbackDay, "── "+dayLabel(l.Day)+" ──")
	}
	return style.Highlight(l.Text, hl.Runs(l.Plain, l.Tags))
}
```

Run: `go test ./internal/ui -run TestPaintKinds`
Expected: PASS.

- [ ] **Step 3: Write the Review Focus tests (they pin today's behavior, so they pass before and after the refactor)**

Add to `internal/ui/theme_test.go` (it already imports everything these use):

```go
// Adding a classify rule tags lines already on screen, not just new ones.
func TestClassifyEditRetagsLinesOnScreen(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.show("[Wiki] Tapestries edited")
	world := fmWorld + "\n[[classify]]\ntag = \"wiki\"\npattern = '^\\[Wiki\\]'\n\n[tags]\nwiki = { fg = \"#0a0b0c\" }\n"
	if err := os.WriteFile(filepath.Join(h.dir, "worlds", "fm.toml"), []byte(world), 0o600); err != nil {
		t.Fatal(err)
	}
	h.m.Update(reloadMsg{})
	if s := h.drawn(); !strings.Contains(s, "38;2;10;11;12m[Wiki] Tapestries") {
		t.Errorf("the line on screen wasn't tagged by the new rule:\n%q", s)
	}
}

// Day dividers take a theme change, and the newest line from the log is
// a real line, never a divider.
func TestDayDividersRepaintAndAreNotTheNewestLine(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24.AddDate(0, 0, -1), "yesterday")
	h.writeLog(day24, "today")
	cs := h.m.chars["fm/kit"]
	cs.sb = Scrollback{}
	h.m.preload(cs)
	writeUserTheme(t, h, "extends = \"kiln\"\n[ui]\n\"scrollback.day\" = { fg = \"#0a0b0c\" }\n")
	h.m.Update(reloadMsg{})
	day := theme.Active().SGR(theme.ScrollbackDay)
	n := 0
	for _, l := range cs.sb.lines {
		if strings.Contains(l.text, "── ") {
			n++
			if !strings.HasPrefix(l.text, day) {
				t.Errorf("divider %q kept the old style", l.text)
			}
		}
	}
	if n != 2 {
		t.Errorf("dividers = %d, want one per day", n)
	}
	if got, ok := cs.sb.LastTime(); !ok || !got.Equal(day24) {
		t.Errorf("LastTime = %v, %v; want %v (today's line)", got, ok, day24)
	}
}
```

Run: `go test ./internal/ui -run 'TestClassifyEditRetagsLinesOnScreen|TestDayDividersRepaintAndAreNotTheNewestLine'`
Expected: PASS on the current code. If either fails here, stop: the test is wrong about today's behavior, and must be fixed before the refactor relies on it.

- [ ] **Step 4: Move `ui` onto `app.Line`**

`internal/ui/model.go`:
- `charState`: replace `cls *classify.Classifier` and `judge rules.Judge` with `rules app.Rules`. Keep `hl *rules.Highlighter`.
- `compile`:

```go
	cs.rules = app.Rules{Classifier: cls, Judge: rules.Judge{Attention: cs.ch.Rules.Attention, Quiet: cs.ch.Rules.Quiet}}
	cs.hl = rules.New(th)
	return true, lookErr
```

- Delete `renderLine` and the `renderDays` function; replace the methods with:

```go
// render makes e into a line and paints it.
func (cs *charState) render(e logstore.Entry) (string, app.Line) {
	l := cs.rules.Line(e)
	return paint(cs.hl, l), l
}

// renderDays is app.Rules.Days, painted. Like paint it is safe off the
// UI goroutine.
func renderDays(r app.Rules, hl *rules.Highlighter, echo bool, entries []logstore.Entry, startsDay bool) []sbLine {
	ls := r.Days(entries, echo, startsDay)
	out := make([]sbLine, len(ls))
	for i, l := range ls {
		out[i] = lineOf(paint(hl, l), l)
	}
	return out
}
```

- `preload`: `for _, l := range renderDays(cs.rules, cs.hl, cs.ch.LocalEcho, entries, startsDay) {`
- `pageOlder`: capture `r, hl := cs.rules, cs.hl` and call `renderDays(r, hl, echo, …)` in both places.
- The `sbOlderMsg` case in `update`: replace the restyle loop with

```go
			if msg.theme != theme.Active() { // painted in a theme since replaced
				for i := range msg.lines {
					msg.lines[i] = msg.lines[i].repainted(func(l app.Line) string { return paint(cs.hl, l) })
				}
			}
```

- `loadTheme`'s per-character loop:

```go
		cs.sb.Rerender(nil, func(l app.Line) string { return paint(cs.hl, l) })
		for _, b := range cs.browses() {
			b.restyle(func(l app.Line) string { return paint(cs.hl, l) })
		}
```

- `applyConfig`, where it restyles:

```go
		if installed && restyle {
			cs.sb.Rerender(cs.rules.Reline, func(l app.Line) string { return paint(cs.hl, l) })
		}
```

- `handleEvent`'s `EventLine` case: `text, l := cs.render(ev.Entry)`; use `lineOf(text, l)` in place of `entryLine(text, ev.Entry)`, `l.Quiet` for `res.Quiet`, `l.Attention` for `res.Attention`, and `m.notifyCmd(cs, l)`.
- Remove now-unused imports (`classify`, `style`) if the compiler says so.

`internal/ui/notify.go`: `func (m *Model) notifyCmd(cs *charState, l app.Line) tea.Cmd` — inside, `e := l.Entry` at the top and `l.Quiet`/`l.Attention` for `res.Quiet`/`res.Attention`.

`internal/ui/scrollback.go`:

```go
type sbLine struct {
	text string
	line *app.Line // what text was painted from; nil for Kiln's chrome and plain text
	role theme.Role // for a line Kiln wrote: its style, and raw its text
	raw  string
	// … wrap and mouse fields unchanged
}

// lineOf is a line painted from l.
func lineOf(text string, l app.Line) sbLine { return sbLine{text: text, line: &l} }

// Rerender redraws every line: lines made from the log are remade with
// reline first, if it's not nil (the character's rules changed), then
// painted with paint; Kiln's own lines are painted in their roles. The
// view keeps its offset; a selection is dropped, since its byte
// positions may no longer fit.
func (s *Scrollback) Rerender(reline func(app.Line) app.Line, paint func(app.Line) string) {
	for i, l := range s.lines {
		if reline != nil && l.line != nil {
			nl := reline(*l.line)
			l.line = &nl
		}
		s.lines[i] = l.repainted(paint)
	}
	s.sel, s.hover = nil, nil
}

// repainted is l drawn afresh: from its line with paint, or in its role.
// A line with neither is returned as is.
func (l sbLine) repainted(paint func(app.Line) string) sbLine {
	switch {
	case l.line != nil:
		return lineOf(paint(*l.line), *l.line)
	case l.role != "":
		return chromeLine(l.role, l.raw)
	}
	return l
}
```

Delete `entryLine` and `restyled`. In `LastTime`, replace the loop body with

```go
		if l := s.lines[i].line; l != nil && l.Kind != app.Day {
			return l.Entry.Time, true
		}
```

`internal/ui/browse.go`:

```go
type bline struct {
	app.Line
	tags  []string // TagNames, kept for the filters
	text  string   // painted for display
	lower string   // plain text, lowercased, for text filters
}

func (b *browse) newLine(e logstore.Entry) *bline { return makeLine(b.cs.rules, b.cs.hl, e) }

// makeLine makes and paints e. Like paint it only reads its arguments,
// so older days can be prepared off the UI goroutine.
func makeLine(r app.Rules, hl *rules.Highlighter, e logstore.Entry) *bline {
	l := r.Line(e)
	return &bline{Line: l, tags: l.TagNames(), text: paint(hl, l), lower: strings.ToLower(l.Plain)}
}

// restyle repaints every line with paint, after a theme change.
func (b *browse) restyle(paint func(app.Line) string) {
	for _, l := range b.lines {
		l.text = paint(l.Line)
	}
}
```

- `readOlder(h, r app.Rules, hl *rules.Highlighter)`, and the callers in `loadOlder` and `requestOlder` (`b.cs.rules`).
- `receive`: `l.text = paint(b.cs.hl, l.Line)` in the stale-theme loop.
- Replace every `l.e` / `b.start.e` with `l.Entry` / `b.start.Entry`, and every `l.day` / `b.lines[0].day` with `.Day` (the compiler lists them: lines 111, 217, 335, 512, 593-594, 604, 968, 974 today).

Note on `bline.tags`: `makeLine` used `cls.Classify(plain)` only for incoming lines; `app.Rules.Line` only classifies `Server` (incoming) lines, so `TagNames()` is the same list in the same order.

Test setup updates (setup only, expectations unchanged):
- `scrollback_test.go` `TestRerenderRepaintsChromeLines`: `s.Rerender(nil, func(app.Line) string { return "" })`.
- `sidebar_test.go`: each `entryLine(text, logstore.Entry{…})` becomes `lineOf(text, app.Line{Entry: logstore.Entry{…}})`.
- `theme_test.go` `TestOlderHistoryArrivingAfterThemeChange` finds dividers with `l.role == theme.ScrollbackDay`; day dividers now carry a line instead of a role, so the locator becomes `l.line != nil && l.line.Kind == app.Day` (what it asserts doesn't change).
- `model_test.go` `TestRenderLineMatchScope`: replace the `renderLine` call with

```go
	l := app.Rules{Classifier: cls, Judge: rules.Judge{Attention: []string{"page"}}}.Line(logstore.Entry{Dir: logstore.In, Text: "PAGE: Mira says hi"})
	got, res := paint(hl, l), l.Verdict
```

- [ ] **Step 5: Run everything**

Run: `go vet ./... && go test ./...`
Expected: PASS — every `internal/ui` test including the goldens and the two Review Focus tests from Step 3, `internal/app`, `internal/str`, and `TestNoHardCodedStyles`.
If a golden fails, the refactor changed a look: find the line kind whose `paint` differs from the old `renderLine`/`renderDays` and fix the code. Do not run `-update`.

- [ ] **Step 6: Check the web build still compiles**

Run: `cd web && GOOS=js GOARCH=wasm go build ./cmd/kiln-web`
Expected: builds (it doesn't use these internals directly, but `ui` must still compile for wasm).

- [ ] **Step 7: Commit**

```bash
git add internal/ui
git commit -m "ui: scrollback and log mode paint app.Lines; a theme change repaints, a rules change relines

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01PC4WGuqjdnGDVGdiaL3Z7n"
```
