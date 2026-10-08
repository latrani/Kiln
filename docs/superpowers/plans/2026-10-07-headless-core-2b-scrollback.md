# Headless core, step 2b: the core owns scrollback data — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Each character's scrollback lines, prompt and history paging live in `app.Char`; the TUI's `Scrollback` becomes a view (painted text, wraps, scroll offset, selection) over the core's lines; and an `app.Line` stops carrying copies of its text.

**Architecture:** `app.Char.Lines` is `[]*Line`, oldest first. The core appends to it (session lines, echoes), prepends to it (older history it reads in a `Run`), and relines it in place when a character's rules change. `ui.Scrollback` keeps one `sbLine` per core line, holding the same `*app.Line` plus its paint and wrap caches. The ui mirrors every core change right after the call that made it. `Scrollback` keeps its own paging throttle (`more`, `loading`, `RequestOlder`) because that's about what the view needs; the core is the source of truth for whether more history exists.

**Tech Stack:** Go; `internal/app`, `internal/ui`, `history`, `logstore`, `ansi`.

**Spec:** `docs/superpowers/specs/2026-10-07-headless-core-design.md`, sections "Lines" (the scrollback split) and "Order of work" step 2. The 2a plan's "Scope ruling" splits step 2 into 2a/2b/2c.

## Global Constraints

- Nothing visible changes: golden screens are NOT updated (never `-update`).
- Every existing test passes. Tests may change how they reach state (`app.OlderMsg` instead of `sbOlderMsg`, `l.Text()` instead of `l.Text`), never what they expect.
- `internal/app` doesn't depend on Bubble Tea or lipgloss and never paints.
- Only `App` changes `app.Char` fields and the `Line`s it holds; front ends keep pointers and read.
- User-facing strings come from `internal/str`; this step adds none.
- `ui.Deps` doesn't change.
- Run `go vet ./... && go test ./...` before each commit.

## Review Focus

1. **Paging older history while the theme or rules change in between.** A page arrives painted in the current theme, and with the current rules' tags. (Existing tests `TestOlderHistoryArrivingAfterThemeChange` and `TestOlderHistoryArrivingAfterRulesAndThemeChange` must pass unchanged in what they assert.)
2. **A prompt from the server** shows below the last line until the next line arrives, and only a line that shows clears it (a sent line with `local_echo` off doesn't).
3. **The overview's "last seen" time and its tail** ignore day dividers and the "history ends" line, as before. Task 2 tests the core; existing view tests cover the overview.
4. **Paging stops at the oldest day.** `more` goes false, and the "loading" row goes away. Existing test `TestScrollbackPagesHistoryAcrossPartialDay` covers it.
5. **Memory:** a line's sanitized and plain text aren't stored. Task 1 tests that `Line` has no text fields besides its entry.

---

### Task 1: A slimmer `app.Line`, and a "history ends" kind

**Files:**
- Modify: `internal/app/line.go`, `internal/app/line_test.go`
- Modify: `internal/ui/paint.go`, `internal/ui/paint_test.go`, `internal/ui/browse.go` (`makeLine`)

**Interfaces:**
- Produces: `func (l app.Line) Text() string`, `func (l app.Line) Plain() string`; `app.HistoryEnd` kind (a line whose `Entry.Time` is the newest preloaded line's); `Line.Text`/`Line.Plain` fields removed; `Reline` passes `HistoryEnd` through like `Day`.

- [ ] **Step 1: Update the line tests, and add the memory test and the HistoryEnd case**

In `internal/app/line_test.go`, `TestServerLineIsClassified`: `l.Plain` → `l.Plain()`, `l.Text` → `l.Text()` (the expectations stay). Add:

```go
// A line keeps its entry, not copies of its text: Text and Plain are
// worked out from the entry when asked.
func TestLineStoresNoTextCopies(t *testing.T) {
	typ := reflect.TypeOf(Line{})
	for i := range typ.NumField() {
		f := typ.Field(i)
		if f.Type.Kind() == reflect.String && f.Name != "Day" {
			t.Errorf("Line stores a string field %s", f.Name)
		}
	}
}

func TestRelineKeepsChrome(t *testing.T) {
	r := testRules(t)
	end := Line{Kind: HistoryEnd, Entry: logstore.Entry{Time: time.Date(2026, 9, 24, 21, 0, 0, 0, time.Local)}}
	if got := r.Reline(end); got.Kind != HistoryEnd || !got.Entry.Time.Equal(end.Entry.Time) {
		t.Errorf("Reline(history end) = %+v", got)
	}
}
```

(Add `"reflect"` to the imports.)

In `internal/ui/paint_test.go`: `server.Text` → `server.Text()`, `server.Plain` → `server.Plain()`, and add to `TestPaintKinds`:

```go
	at := time.Date(2026, 9, 24, 21, 0, 0, 0, time.Local)
	end := app.Line{Kind: app.HistoryEnd, Entry: logstore.Entry{Time: at}}
	if got, want := paint(hl, end), theme.Paint(theme.ScrollbackHistoryEnd, str.ScrollbackHistoryEnds(at.Format(str.DateDayTime()))); got != want {
		t.Errorf("history end = %q, want %q", got, want)
	}
```

(Add `"github.com/latrani/Kiln/internal/str"` to its imports.)

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/app ./internal/ui -run 'TestServerLineIsClassified|TestLineStoresNoTextCopies|TestRelineKeepsChrome|TestPaintKinds'`
Expected: build failure — `l.Plain()` called on a field / `undefined: HistoryEnd`.

- [ ] **Step 3: Implement**

`internal/app/line.go`:
- Add `HistoryEnd // where preloaded history ends; Entry.Time is its newest line's` after `Day` in the `Kind` consts.
- Remove the `Text` and `Plain` fields from `Line` and add:

```go
// Text is the entry's text sanitized, the server's SGR kept.
func (l Line) Text() string { return ansi.Sanitize(l.Entry.Text) }

// Plain is Text without SGR: what the tags' spans index.
func (l Line) Plain() string { return ansi.Strip(l.Text()) }
```

- `Rules.Line`:

```go
func (r Rules) Line(e logstore.Entry) Line {
	l := Line{Entry: e, Day: DayOf(e.Time)}
	switch e.Dir {
	case logstore.Out:
		l.Kind = Echo
	case logstore.Sys:
		l.Kind = Sys
	default:
		l.Kind = Server
		l.Tags = r.Classifier.Tags(l.Plain())
		l.Verdict = r.Judge.Of(l.Tags)
	}
	return l
}
```

- `Reline`: `if l.Kind == Day || l.Kind == HistoryEnd { return l }`, and its comment says "Kiln's own lines (dividers, the end of history) have no entry to remake and come back as they were."

`internal/ui/paint.go` (imports `ansi` and `str` as well):

```go
func paint(hl *rules.Highlighter, l app.Line) string {
	switch l.Kind {
	case app.Echo:
		return theme.Paint(theme.ScrollbackEcho, gutterMark+l.Text())
	case app.Sys:
		return theme.Paint(theme.ScrollbackSys, "* "+l.Text())
	case app.Day:
		return theme.Paint(theme.ScrollbackDay, "── "+dayLabel(l.Day)+" ──")
	case app.HistoryEnd:
		return theme.Paint(theme.ScrollbackHistoryEnd, str.ScrollbackHistoryEnds(l.Entry.Time.Format(str.DateDayTime())))
	}
	text := l.Text()
	return style.Highlight(text, hl.Runs(ansi.Strip(text), l.Tags))
}
```

`internal/ui/browse.go` `makeLine`: `lower: strings.ToLower(l.Plain())`.

- [ ] **Step 4: Run everything**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app internal/ui
git commit -m "app: a Line keeps its entry, not copies of its text; history end is a kind of line

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01PC4WGuqjdnGDVGdiaL3Z7n"
```

---

### Task 2: Scrollback data in the core

**Files:**
- Create: `internal/app/scrollback.go`, `internal/app/scrollback_test.go`
- Modify: `internal/app/app.go` (`Char` fields, `compile` bumps a generation, `Open` preloads, `ApplyConfig` relines), `internal/app/session.go` (`Handle` appends lines and keeps the prompt; `Event.Line` becomes `*Line` plus `Shown`)
- Modify: `internal/app/session_test.go` (`ev.Line.Entry` still works through the pointer; nothing else)

**Interfaces:**
- Consumes: Task 1's `Line`, `HistoryEnd`, `Reline`.
- Produces:
  - `const app.HistoryLines = 200`
  - `app.Char` gains `Lines []*Line`, `Prompt string`, `More bool`, `Loading bool` (and unexported `hist *history.Reader`, `leftover []logstore.Entry`, `rulesGen int`)
  - `(*App).Preload(k string)`; `(*App).Echo(k string, e logstore.Entry) *Line`; `(*App).RequestOlder(k string) []Effect`; `(*App).HandleOlder(msg OlderMsg) ([]*Line, bool)`; `(*App).LastTime(k string) (time.Time, bool)`
  - `type app.OlderMsg struct { Key string; hist *history.Reader; gen int; lines []*Line; more bool }`
  - `app.Event.Line` is `*Line`; `app.Event.Shown bool` (the line went into `Lines`)

- [ ] **Step 1: Write the tests**

`internal/app/scrollback_test.go`:

```go
package app

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
)

// writeLog puts lines in fm/kit's log under a's log root, one a minute
// from start; "> " starts a sent line.
func writeLog(t *testing.T, a *App, start time.Time, lines ...string) {
	t.Helper()
	w := logstore.NewWriter(logstore.Layout{Root: a.d.LogRoot, World: "fm", Char: "kit", CharName: "Kit"})
	defer w.Close()
	for i, l := range lines {
		dir := logstore.In
		if s, ok := strings.CutPrefix(l, "> "); ok {
			dir, l = logstore.Out, s
		}
		if err := w.Append(logstore.Entry{Time: start.Add(time.Duration(i) * time.Minute), Dir: dir, Text: l}); err != nil {
			t.Fatal(err)
		}
	}
}

func kinds(ls []*Line) []Kind {
	var ks []Kind
	for _, l := range ls {
		ks = append(ks, l.Kind)
	}
	return ks
}

var day23 = time.Date(2026, 9, 23, 20, 0, 0, 0, time.Local)

func TestOpenPreloadsTheTailOfHistory(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	writeLog(t, a, day23, "one", "> sent", "two")
	openAll(t, a, "fm/kit")
	kit := a.Char("fm/kit")
	if got, want := kinds(kit.Lines), []Kind{Day, Server, Server, HistoryEnd}; !slices.Equal(got, want) {
		t.Errorf("preloaded %v, want %v (sent lines hidden with local_echo off)", got, want)
	}
	if end := kit.Lines[len(kit.Lines)-1]; !end.Entry.Time.Equal(day23.Add(2 * time.Minute)) {
		t.Errorf("history ends at %v", end.Entry.Time)
	}
	if kit.More {
		t.Error("More with everything preloaded")
	}
	if at, ok := a.LastTime("fm/kit"); !ok || !at.Equal(day23.Add(2*time.Minute)) {
		t.Errorf("LastTime = %v, %v; want the last line's, not the end marker's or a divider's", at, ok)
	}
}

func TestPagingOlderHistory(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	lines := make([]string, HistoryLines+50)
	for i := range lines {
		lines[i] = "line"
	}
	writeLog(t, a, day23, lines...)
	openAll(t, a, "fm/kit")
	kit := a.Char("fm/kit")
	if !kit.More {
		t.Fatal("not More with history past the preload")
	}
	effs := a.RequestOlder("fm/kit")
	if len(effs) != 1 || !kit.Loading {
		t.Fatalf("RequestOlder = %v, loading %v", effs, kit.Loading)
	}
	if again := a.RequestOlder("fm/kit"); again != nil {
		t.Error("a second read started while one was in flight")
	}
	before := len(kit.Lines)
	msg := effs[0].(Run).Func().(OlderMsg)
	got, ok := a.HandleOlder(msg)
	if !ok || len(got) == 0 || len(kit.Lines) != before+len(got) || kit.Lines[0] != got[0] {
		t.Fatalf("HandleOlder = %d lines, %v; lines %d → %d", len(got), ok, before, len(kit.Lines))
	}
	if kit.Loading || kit.More {
		t.Errorf("after the last page: loading %v more %v", kit.Loading, kit.More)
	}
	a.Preload("fm/kit") // a new reader: a page from the old one is stale
	if _, ok := a.HandleOlder(msg); ok {
		t.Error("took a page from a replaced reader")
	}
}

// A page read under rules that have since changed is made again under
// the current ones when it arrives.
func TestOlderPageAfterARulesChange(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	lines := make([]string, HistoryLines+50)
	for i := range lines {
		lines[i] = "[Wiki] edited"
	}
	writeLog(t, a, day23, lines...)
	openAll(t, a, "fm/kit")
	msg := a.RequestOlder("fm/kit")[0].(Run).Func().(OlderMsg)
	dir := filepath.Dir(a.d.LogRoot) // sessionApp's log root is <config dir>/logs
	writeWorlds(t, dir, map[string]string{"fm": fmWorld + "\n[[classify]]\ntag = \"wiki\"\npattern = '^\\[Wiki\\]'\n"})
	a.ApplyConfig(load(t, dir))
	got, _ := a.HandleOlder(msg)
	for _, l := range got {
		if l.Kind == Server && !slices.Contains(l.TagNames(), "wiki") {
			t.Fatalf("%q arrived with the old rules' tags", l.Entry.Text)
		}
	}
	for _, l := range a.Char("fm/kit").Lines {
		if l.Kind == Server && !slices.Contains(l.TagNames(), "wiki") {
			t.Fatalf("a preloaded line kept the old rules' tags after the reload")
		}
	}
}

func TestSessionLinesAndPrompt(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	kit := a.Char("fm/kit")
	a.Handle(SessionMsg{Key: "fm/kit", Sess: s, OK: true, Ev: session.Event{Kind: session.EventPrompt, Entry: logstore.Entry{Text: "Name?\x07 "}}})
	if kit.Prompt != "Name? " {
		t.Errorf("Prompt = %q, want it sanitized", kit.Prompt)
	}
	ev, _, _ := a.Handle(SessionMsg{Key: "fm/kit", Sess: s, OK: true, Ev: session.Event{Kind: session.EventLine, Entry: logstore.Entry{Dir: logstore.Out, Text: "hi"}}})
	if ev.Shown || kit.Prompt == "" || len(kit.Lines) != 0 {
		t.Errorf("a sent line with local_echo off: shown %v prompt %q lines %d", ev.Shown, kit.Prompt, len(kit.Lines))
	}
	ev, _, _ = a.Handle(lineMsg("fm/kit", s, "Welcome!"))
	if !ev.Shown || kit.Prompt != "" || len(kit.Lines) != 1 || kit.Lines[0] != ev.Line {
		t.Errorf("a server line: shown %v prompt %q lines %d", ev.Shown, kit.Prompt, len(kit.Lines))
	}
	if l := a.Echo("fm/kit", logstore.Entry{Dir: logstore.Out, Text: "connect Kit ******"}); l != nil {
		t.Error("echoed a sent line with local_echo off")
	}
}

func TestEchoWithLocalEcho(t *testing.T) {
	echo := strings.Replace(fmWorld, "tls = true\n", "tls = true\nlocal_echo = true\n", 1)
	a := sessionApp(t, map[string]string{"fm": echo})
	openAll(t, a, "fm/kit")
	l := a.Echo("fm/kit", logstore.Entry{Dir: logstore.Out, Text: ":waves."})
	if kit := a.Char("fm/kit"); l == nil || l.Kind != Echo || kit.Lines[len(kit.Lines)-1] != l {
		t.Errorf("Echo = %+v", l)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/app`
Expected: build failure — `undefined: HistoryLines`, `kit.Lines undefined`, `a.RequestOlder undefined`, `undefined: OlderMsg`.

- [ ] **Step 3: Implement**

`internal/app/app.go`, `Char` gains (after `ConnectedAt`):

```go
	Lines       []*Line   // the scrollback, oldest first; front ends may keep the pointers
	Prompt      string    // an unterminated prompt from the server, sanitized; "" when there's none
	More        bool      // older history exists that isn't in Lines yet; see RequestOlder
	Loading     bool      // a page of older history is being read
	hist        *history.Reader  // pages older log days in; only an in-flight RequestOlder read touches it
	leftover    []logstore.Entry // the preload's unshown start of its oldest day
	rulesGen    int              // bumped whenever Rules changes, so a page read under older ones is made again
```

`compile` bumps `c.rulesGen++` after installing new rules.

`Open`: after `a.sortOrder()` and before setting active, call `a.Preload(k)` (the ui no longer preloads; see Task 3).

`ApplyConfig`, for a character still configured, before `c.Ch, c.Orphan = ch, false`:

```go
		retag := !reflect.DeepEqual(tagInputs(c.Ch), tagInputs(ch))
```

and after a successful `compile`:

```go
		if err := c.compile(); err != nil {
			res.Errs[k] = err
		} else if retag {
			for _, l := range c.Lines {
				*l = c.Rules.Reline(*l)
			}
		}
```

with

```go
// tagInputs is what a character's lines' tags and verdicts depend on.
func tagInputs(ch config.Character) any {
	return struct {
		rules   config.Rules
		name    string
		aliases []string
	}{ch.Rules, ch.Name, ch.Aliases}
}
```

`internal/app/scrollback.go`:

```go
package app

import (
	"time"

	"github.com/latrani/Kiln/internal/history"
	"github.com/latrani/Kiln/internal/logstore"
)

// HistoryLines is how many logged lines are preloaded per character.
const HistoryLines = 200

// OlderMsg is a page of older history, as a Run from RequestOlder returns
// it; HandleOlder takes it.
type OlderMsg struct {
	Key   string
	hist  *history.Reader // the reader that was asked; stale if it has changed
	gen   int             // the rules generation the page was made under
	lines []*Line
	more  bool
}

func pointers(ls []Line) []*Line {
	out := make([]*Line, len(ls))
	for i := range ls {
		out[i] = &ls[i]
	}
	return out
}

// Preload replaces k's lines with the tail of its most recent log days,
// and keeps everything older to page in on demand, so scrollback is
// unlimited. Open does this.
func (a *App) Preload(k string) {
	c := a.chars[k]
	if c == nil {
		return
	}
	c.Lines, c.More, c.Loading, c.hist, c.leftover = nil, false, false, nil, nil
	l, ok := a.LogLayout(c.Ch)
	if !ok {
		return
	}
	hist, err := history.NewReader(l)
	if err != nil {
		return
	}
	var entries []logstore.Entry
	for len(entries) < HistoryLines {
		es, _, ok, err := hist.LoadOlder()
		if !ok || err != nil {
			break
		}
		entries = append(es, entries...)
	}
	if len(entries) == 0 {
		return
	}
	// entries holds whole days. Keep the newest HistoryLines; the rest
	// (the start of the oldest day, plus any fuller days before it) is
	// the first page RequestOlder hands over.
	var leftover []logstore.Entry
	if len(entries) > HistoryLines {
		leftover = entries[:len(entries)-HistoryLines]
		entries = entries[len(entries)-HistoryLines:]
	}
	// The preload starts a day only if nothing of that day was left over.
	startsDay := len(leftover) == 0 || DayOf(leftover[len(leftover)-1].Time) != DayOf(entries[0].Time)
	c.Lines = pointers(c.Rules.Days(entries, c.Ch.LocalEcho, startsDay))
	c.Lines = append(c.Lines, &Line{Kind: HistoryEnd, Entry: logstore.Entry{Time: entries[len(entries)-1].Time}})
	c.hist, c.leftover = hist, leftover
	c.More = leftover != nil || !hist.Exhausted()
}

// RequestOlder starts reading k's next older page of history off the
// loop: first the preload's leftover, then one log day per read. It
// returns nothing when there's nothing more to read or a read is already
// in flight. HandleOlder takes what the Run returns.
func (a *App) RequestOlder(k string) []Effect {
	c := a.chars[k]
	if c == nil || c.hist == nil || !c.More || c.Loading {
		return nil
	}
	c.Loading = true
	h, r, echo, leftover, gen := c.hist, c.Rules, c.Ch.LocalEcho, c.leftover, c.rulesGen
	c.leftover = nil
	return []Effect{Run{Func: func() any {
		msg := OlderMsg{Key: k, hist: h, gen: gen}
		if leftover != nil {
			msg.lines, msg.more = pointers(r.Days(leftover, echo, true)), !h.Exhausted()
			return msg
		}
		if es, _, ok, err := h.LoadOlder(); ok && err == nil {
			msg.lines, msg.more = pointers(r.Days(es, echo, true)), !h.Exhausted()
		}
		return msg
	}}}
}

// HandleOlder puts a page RequestOlder read above k's lines and returns
// it, for the front end to show. ok is false for a page from a reader
// that has since been replaced. A page read under rules that have since
// changed is made again under the current ones.
func (a *App) HandleOlder(msg OlderMsg) (lines []*Line, ok bool) {
	c := a.chars[msg.Key]
	if c == nil || c.hist != msg.hist {
		return nil, false
	}
	if msg.gen != c.rulesGen {
		for _, l := range msg.lines {
			*l = c.Rules.Reline(*l)
		}
	}
	c.Lines = append(msg.lines, c.Lines...)
	c.More, c.Loading = msg.more, false
	return msg.lines, true
}

// Echo adds a line you sent to k's lines, if its local_echo shows sent
// lines, and returns it; nil if it doesn't show.
func (a *App) Echo(k string, e logstore.Entry) *Line {
	c := a.chars[k]
	if c == nil || !c.shows(e) {
		return nil
	}
	return c.add(c.Rules.Line(e))
}

// shows reports whether e belongs in the scrollback: everything but sent
// lines, which only show with local_echo on. They're logged either way.
func (c *Char) shows(e logstore.Entry) bool { return e.Dir != logstore.Out || c.Ch.LocalEcho }

// add appends l to the scrollback. The server has moved on from any
// prompt.
func (c *Char) add(l Line) *Line {
	c.Lines = append(c.Lines, &l)
	c.Prompt = ""
	return &l
}

// LastTime is when k's newest line from the log was logged; false when
// there's none.
func (a *App) LastTime(k string) (time.Time, bool) {
	if c := a.chars[k]; c != nil {
		for i := len(c.Lines) - 1; i >= 0; i-- {
			if l := c.Lines[i]; l.Kind != Day && l.Kind != HistoryEnd {
				return l.Entry.Time, true
			}
		}
	}
	return time.Time{}, false
}
```

`internal/app/session.go`: `Event.Line` becomes `Line *Line // for an EventLine, what Kiln made of its entry` and add `Shown bool // the line went into the scrollback (Lines)`. In `Handle`:

```go
	case session.EventLine:
		l := c.Rules.Line(msg.Ev.Entry)
		ev.Line = &l
		if c.shows(msg.Ev.Entry) {
			ev.Line, ev.Shown = c.add(l), true
		}
		if msg.Key != a.active && msg.Ev.Entry.Dir == logstore.In && !ev.Line.Quiet {
			c.Unread++
			c.Attention = c.Attention || ev.Line.Attention
		}
	case session.EventPrompt:
		c.Prompt = ansi.Sanitize(msg.Ev.Entry.Text)
```

(import `ansi`; `app.go` imports `reflect` for `tagInputs`). `TestHandleCountsUnreadOnlyOffScreen` reads `ev.Line.Entry.Text`, which works through the pointer.

Keep `internal/ui` compiling and green in this commit: in `handleEvent`, pass `*ev.Line` where it passes `ev.Line` today (`paint`, `lineOf`, `notifyCmd`). The ui still preloads its own view from the log in this commit; Task 3 switches it to the core's lines.

- [ ] **Step 4: Run the tests**

Run: `go test -race ./internal/app && go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app
git commit -m "app: the core holds each character's scrollback, prompt and paging

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01PC4WGuqjdnGDVGdiaL3Z7n"
```

---

### Task 3: The TUI's scrollback is a view over the core's lines

**Files:**
- Modify: `internal/ui/scrollback.go`, `internal/ui/model.go`, `internal/ui/sidebar.go` (`open`), `internal/ui/view.go` (overview `LastTime`)
- Modify tests (access paths/setup only): `internal/ui/scrollback_test.go`, `internal/ui/model_test.go`, `internal/ui/theme_test.go`, `internal/ui/sidebar_test.go`

**Interfaces:**
- Consumes: Task 2's `Char.Lines/Prompt/More`, `Preload`, `Echo`, `RequestOlder`, `HandleOlder`, `OlderMsg`, `Event.Shown`, `LastTime`.
- Produces: `func lineOf(text string, l *app.Line) sbLine`; `func (s *Scrollback) Repaint(paint func(app.Line) string)`; `(*Model).preload(cs)` reloads the core's preload and rebuilds the view (kept as a seam for tests); `charState` loses `hist` and `leftover`; `sbOlderMsg` is gone.

- [ ] **Step 1: Scrollback holds pointers and repaints**

`internal/ui/scrollback.go`:
- `sbLine.line` stays `*app.Line` (now the core's own pointer); remove `role` and `raw`.
- `func lineOf(text string, l *app.Line) sbLine { return sbLine{text: text, line: l} }`.
- Delete `chromeLine` and `Rerender`/`repainted`; add:

```go
// Repaint redraws every line from its app.Line with paint, after a theme
// change, or after the core made a character's lines again under new
// rules. The view keeps its offset; a selection is dropped, since its
// byte positions may no longer fit.
func (s *Scrollback) Repaint(paint func(app.Line) string) {
	for i, l := range s.lines {
		if l.line != nil {
			s.lines[i] = lineOf(paint(*l.line), l.line)
		}
	}
	s.sel, s.hover = nil, nil
}
```

- Delete `LastTime` (the overview asks the core: `m.a.LastTime(k)` in `view.go`).
- `RequestOlder`, `SetMore`, `Prepend`, `PrependLines`, `more`, `loading` stay: they're the view's throttle and its "loading" row, told by the ui what the core says.

- [ ] **Step 2: The model mirrors the core**

`internal/ui/model.go`:
- `charState` loses `hist` and `leftover`. Delete `sbOlderMsg`, `renderDays`, and `HistoryLines` (keep `echoes`: log mode uses it) (it's `app.HistoryLines`; if `cmd/kiln` or `web` reference `ui.HistoryLines`, keep `const HistoryLines = app.HistoryLines`).
- `preload`:

```go
// preload has the core read cs's history again and shows it: what Open
// does, kept for tests that write logs after the harness opens Kit.
func (m *Model) preload(cs *charState) {
	m.a.Preload(cs.Key)
	m.showLines(cs)
}

// showLines builds cs's view from the core's lines, painted.
func (m *Model) showLines(cs *charState) {
	for _, l := range cs.Lines {
		cs.sb.AppendLine(lineOf(paint(cs.hl, *l), l))
	}
	cs.sb.SetMore(cs.More)
}
```

- `pageOlder`:

```go
func (m *Model) pageOlder() tea.Cmd {
	cs := m.cur()
	if cs == nil || cs.browse != nil || !cs.sb.RequestOlder(m.layout().sbH) {
		return nil
	}
	effs := m.a.RequestOlder(cs.Key)
	if effs == nil { // the core has nothing more after all
		cs.sb.PrependLines(nil, cs.More)
	}
	return m.run(effs)
}
```

- `update`: replace the `sbOlderMsg` case with

```go
	case app.OlderMsg:
		if lines, ok := m.a.HandleOlder(msg); ok {
			cs := m.chars[msg.Key]
			batch := make([]sbLine, len(lines))
			for i, l := range lines {
				batch[i] = lineOf(paint(cs.hl, *l), l)
			}
			cs.sb.PrependLines(batch, cs.More)
		}
```

- `loadTheme`: `cs.sb.Repaint(func(l app.Line) string { return paint(cs.hl, l) })`.
- `applyConfig`: the restyle becomes `cs.sb.Repaint(func(l app.Line) string { return paint(cs.hl, l) })` (the core has already made the lines again if the rules changed).
- `handleEvent`, `EventLine`:

```go
	case session.EventLine:
		if ev.Shown {
			text := paint(cs.hl, *ev.Line)
			if ev.Line.Quiet {
				cs.sb.append(lineOf(text, ev.Line))
			} else {
				cs.sb.AppendLine(lineOf(text, ev.Line))
			}
		}
		…(Pause, browses, as now)…
		if n := m.notifyCmd(cs, *ev.Line); n != nil {
```

  and `EventPrompt`: `cs.sb.SetPrompt(cs.Prompt)`.
- `submit`, the login echo and each sent line: `if l := m.a.Echo(cs.Key, e); l != nil { cs.sb.AppendLine(lineOf(paint(cs.hl, *l), l)) }`; for the sent lines, `echoed = l != nil`.

`internal/ui/sidebar.go` `open`: replace `m.preload(cs)` with `m.showLines(cs)` (the core preloaded in `Open`). Keep `cs.sb.MarkSeen()` after it.

`internal/ui/view.go`: the overview's `cs.sb.LastTime()` → `m.a.LastTime(k)`.

Run: `go build ./... && go vet ./internal/ui`
Expected: the build passes; vet names only test files.

- [ ] **Step 3: Tests' access paths**

| Old (tests) | New |
|---|---|
| `sbOlderMsg` (type assertions and vars) | `app.OlderMsg` |
| `entryLine`/`lineOf(text, app.Line{…})` | `lineOf(text, &app.Line{…})` |
| `chromeLine(theme.ScrollbackDay, "── Thu Sep 24 ──")` in `TestRerenderRepaintsChromeLines` | `lineOf("── Thu Sep 24 ──", &app.Line{Kind: app.Day, Day: "2026-09-24"})`, and `s.Rerender(nil, …)` → `s.Repaint(func(l app.Line) string { return paint(nil, l) })` (a Day line never touches the highlighter); the assertion stays |
| `l.line.Kind`, `l.line.Entry`, `l.line.TagNames()` | unchanged (pointer) |
| `cs.render(e)` in theme tests | unchanged |
| `cs.sb.LastTime()` | `h.m.a.LastTime("fm/kit")` |
| the tests that reset `cs.sb = Scrollback{}` and call `h.m.preload(cs)` | unchanged: `preload` reloads the core and rebuilds the view |

Run: `go vet ./... && go test ./...`
Expected: PASS everywhere, goldens untouched. If a test fails, compare with `git show main:internal/ui/model.go`; don't change an expectation.

- [ ] **Step 4: Other builds**

Run: `cd web && GOOS=js GOARCH=wasm go build -o /dev/null ./cmd/kiln-web && go test ./... && cd .. && go build -o /dev/null ./cmd/kiln`
Expected: all succeed.

- [ ] **Step 5: Commit**

```bash
git add internal/ui
git commit -m "ui: the scrollback is a view over the core's lines

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01PC4WGuqjdnGDVGdiaL3Z7n"
```
