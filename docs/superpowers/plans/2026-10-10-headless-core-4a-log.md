# Headless core, step 4a: shared test fakes, and the core owns log mode — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Log mode's behavior lives in `app.Log`: its lines, paging older days in, live lines, which lines show, the cursor, the range and its exclusions, find and its progress through older days, going to a date, the selection, copy and export (as `Copy` and `SaveFile` effects), and its status. The TUI keeps drawing: the top line of the body, `rowLines`, painting, the prompts' text, click hit-testing and the key map. The fake connections the tests use move to one shared package first.

**Not in 4a (that's 4b):** the filter panel's entries, folds and selection (`filter.go`, `collapsed`, `foldSeen`), and the `+ Text` prompt. 4a moves the filter itself (`scene.Filter`) onto `app.Char` and the panel's item list (`Items`) into `app.Log`, because which lines show depends on them; the panel keeps working from the TUI over those.

**Architecture:**
- **`app.Log`** is one character's log mode, at `Char.Log` while it's open (shown or hidden by Ctrl+L: hiding is the TUI's). `App.OpenLog(k)` makes it, loading at least 200 lines of whole days synchronously as now; `App.CloseLog(k)` drops it. Its lines are `*app.LogLine` (an `app.Line` plus the tag names and lowercased plain text the filters read), referenced by pointer so paging never disturbs marks or exclusions.
- **Paging older days** is a `Run` effect whose result, `app.LogOlderMsg`, goes back through `App.HandleLogOlder`. `Log.Older(then)` asks for one; `then func() []Effect` runs when it arrives. Core moves (cursor, find, go to date) pass core continuations; the TUI's scroll passes its own. A page read under rules since replaced is made again (`rulesGen`), as the scrollback's pages are.
- **Live lines** reach the log in `App.Handle`, not the TUI.
- **Painting** is the TUI's, lazily, cached per line (`browse.text(l)`), cleared when the theme changes. Find matches the line's text *as shown*, which only the front end knows, so `Log.Shown` is a front-end function (`plainShown` in the TUI: the echo gutter mark and the sys `* ` included).
- **Copy and export** become effects: `Copy{Text}` and `SaveFile{Key, Name, Data}`. The TUI performs `SaveFile` as `browse.save` does now: a download through `Deps.SaveFile` in the web build, else a file under `export_dir`, never overwriting, with its status on the log.
- The TUI's `browse` embeds `*app.Log`, so `b.Cursor`, `b.Lines`, `b.Start`… read the core. Every browse operation that can page returns `[]app.Effect`, performed by `b.run` (the model's `run`).

**Tech Stack:** Go; `internal/app`, `internal/ui`, `internal/history`, `internal/scene`, `internal/logstore`, new `internal/kilntest`.

**Spec:** `docs/superpowers/specs/2026-10-07-headless-core-design.md`, sections "Log mode" and "Testing".

## Global Constraints

- Nothing visible changes: golden screens are NOT updated (never `-update`).
- Every existing test passes. Tests may change how they reach state (`b.Cursor`, `b.text(l)`, `cs.Filter`, `kilntest.Conn.Feed`) or set it up, never what they expect. A test of purely core behavior that reached into browse internals may move to `app` (see Rulings).
- `internal/app` doesn't depend on Bubble Tea or lipgloss and never paints.
- Strings come from `internal/str`; this step adds none (the `Browse*` messages, `StatusDownloaded`, `DateDay`, `DateDayYear` move with their code).
- Run `go vet ./... && go test ./...` and the web module's tests (`cd web && go test ./... && GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...`) before each commit.

## Rulings carried from the spec

- **The prompts' text stays in the TUI's widget.** The spec says the find and date prompts "use `app` input buffers like the main one". Nothing about them persists or is shared (the main input's buffer exists for history, drafts and the password prompt). The core takes the value when it's entered (`SetFind`, `GotoDate`, `Export`), and the TUI fills the find prompt from `Log.Find`. A web page will have its own text field.
- **Painting is lazy and cached; find matches `Shown`.** `main` paints a page of older lines off the loop as it's read, so lines carry a painted copy. The core can't paint, so the TUI paints a line the first time it needs it. One edge differs: a line *first drawn* after a character's looks changed shows the new look, where `main` showed the look from when it was read. The theme-change repaint (`restyle`) is unchanged.
- **A page read under rules since replaced is classified again** by comparing `rulesGen`, as the scrollback does. `main` compared the theme instead (a stand-in for "theme and maybe rules changed"). Rules changing mid-read without a theme change now reclassifies; the theme changing no longer needs to, since painting is lazy.
- **`SaveFile` checks for an empty selection before the export directory.** `main`'s desktop path said "no export directory" first. The selection can't empty while the file-name prompt is open (no live line removes one, and the filter can't change mid-prompt), so the order is unobservable.
- **Two tests move to `app`:** `TestBrowseLiveDedupeAtMillisecondPrecision` called `appendLive` directly; it becomes `TestLogLiveDedupesAtMillisecondPrecision` in `app`, same assertions. Any other `ui` test that only exercises a core method directly gets the same treatment, noted in the ledger.

## What the old code did as a side effect (keep all of it)

- `newBrowse`: reads whole days synchronously until ≥200 lines or none left; cursor on the newest visible line; `loadedTo` = newest loaded time; `filter.ResetSeen()` (this session's items light by the filter as it stands).
- `openBrowse`: a hidden log (Ctrl+L) comes back as it was, without `ResetSeen`; either way clears the app status.
- `hideBrowse`: stops a find reading older days and clears the log's status ("not news by the time it's back").
- `requestOlder`: a newer request replaces the waiting continuation; any request stops a find waiting on older days (`searching = false`); only one read in flight.
- `receive`: drops a read for a log that has since closed (Esc then reopen is a new log); prepends; a read error says `BrowseReadingLogs` on the log's status; runs the continuation.
- `appendLive`: skips a line no newer than `loadedTo` matching one of the newest 256 loaded (text, direction, millisecond time); the cursor follows only if it was at the last visible line (or nil). Runs for a hidden log too.
- `moveCursor`, `scrollBy`, `click`: each stops a find reading older days first (the status says it was cancelled). Esc while searching stops only the search, not log mode.
- `key`: clears the log's status before acting (not before the filter-panel or prompt keys).
- `findOlderFrom` / `findNewer` / `gotoDate`: a hit clears the log's status; `gotoDate` also puts the line at the top of the body.
- `findFailed`: "no matches at all" when nothing loaded matches and history is exhausted, else the direction's message.
- `mark`: a new range drops exclusions left from an earlier one; marking swaps start and end if the end is above.
- `toggleExclude`: needs a range with both ends and the line inside it, else says so.
- `selection`: received lines (`scene.Exportable`) inside the range, not excluded, shown.
- Export: with no selection, `x` and `c` say "mark a range"; choosing a format with nothing left says so and closes the prompt; the file name defaults from `export_name` with `export_dir` (or no directory for a download); saving trims the name, refuses an empty one, expands `~/`, puts a relative name in `export_dir` (or says there isn't one), creates directories, never overwrites (`O_EXCL`), and says "saved <path>"; a download says "downloaded <name>" using the last path element.
- `restyle` (theme change): repaints every loaded line of both the shown and hidden log.
- `applyConfig` → `setExport`: export settings follow the config for open logs (reading the config at use is the same thing).

## Review Focus

1. **Find across older days.** A find with no loaded match reads older days one at a time until a match or the start of history, showing "searching … back to <day>". Esc stops just the search; any other key, the wheel or a click stops it and then does its own thing; the day being read arrives to nothing. Covered by existing `browse_test.go` find tests through the key map.
2. **What find matches.** Echo lines match with their `›` gutter mark and sys lines with `* `, as drawn. Task 2's `TestPlainShownIsPaintUnstyled` pins `plainShown` == `ansi.Strip(paint(l))` for each kind, highlighted lines included.
3. **A read in flight when the log closes or the character closes.** It arrives to nothing, with no panic and no lines in a new log. Task 2's `TestLogDropsAStaleOlderRead` pins it.
4. **Live lines while the log is hidden by Ctrl+L.** They still arrive in it, deduplicated, and show when it comes back. Existing tests plus the moved dedupe test.
5. **Export status ownership.** "saved …", "file exists", "downloaded …" land on the log's status (so the next log-mode key clears them, as now), not as a plain app status. Existing `browse_test.go` and `web_test.go` save tests check the screen; Task 3's `TestSaveFileStatusIsTheLogs` pins where it lands.

---

### Task 1: `internal/kilntest`, one set of fakes

**Files:**
- Create: `internal/kilntest/kilntest.go`, `internal/kilntest/kilntest_test.go`
- Modify: `internal/ui/model_test.go` (drop `testConn`, `memLog`), `internal/ui/browse_test.go` (`writeLog`), `internal/app/session_test.go` (drop `lineConn`, `nopLog`), `internal/app/submit_test.go` (drop `sentConn`), and every test using them (access paths only)

**Interfaces:**
- Produces: `kilntest.Conn` (`NewConn() *Conn`; `Lines`, `Prompts`, `Err`, `Send`, `Close`, `Resize` for `session.LineConn` and its optional interfaces; `Feed(line string)`, `FeedPrompt(p string)`, `Sent() []string`, `Sizes() [][2]int`), `kilntest.NopLog` (a `session.Appender` that keeps nothing), `kilntest.WriteLog(t testing.TB, l logstore.Layout, start time.Time, lines ...string)` (one entry a minute from start; a line starting `"> "` is sent, the rest received).

- [ ] **Step 1: Write the package's test** — `internal/kilntest/kilntest_test.go`:

```go
package kilntest

import (
	"slices"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/history"
	"github.com/latrani/Kiln/internal/logstore"
)

func TestConnRecordsAndFeeds(t *testing.T) {
	c := NewConn()
	c.Feed("hello")
	if got := <-c.Lines(); got != "hello" {
		t.Errorf("Lines gave %q", got)
	}
	c.Send("look")
	c.Resize(80, 24)
	if !slices.Equal(c.Sent(), []string{"look"}) || !slices.Equal(c.Sizes(), [][2]int{{80, 24}}) {
		t.Errorf("sent %q, sizes %v", c.Sent(), c.Sizes())
	}
	c.Close()
	c.Close() // twice is fine
	if _, ok := <-c.Lines(); ok {
		t.Error("Lines still open after Close")
	}
}

func TestWriteLogWritesADay(t *testing.T) {
	l := logstore.Layout{Root: t.TempDir(), World: "fm", Char: "kit", CharName: "Kit"}
	start := time.Date(2026, 9, 24, 21, 0, 0, 0, time.Local)
	WriteLog(t, l, start, "hi", "> wave")
	h, err := history.NewReader(l)
	if err != nil {
		t.Fatal(err)
	}
	es, _, _, err := h.LoadOlder()
	if err != nil || len(es) != 2 || es[1].Dir != logstore.Out || es[1].Text != "wave" || !es[1].Time.Equal(start.Add(time.Minute)) {
		t.Errorf("read %+v, %v", es, err)
	}
}
```

Run: `go test ./internal/kilntest/` — Expected: FAIL (no package files besides the test).

- [ ] **Step 2: Write `internal/kilntest/kilntest.go`** — the union of `ui`'s `testConn` and `app`'s `lineConn`/`sentConn`:

```go
// Package kilntest holds fakes the app and ui tests share: a scripted
// server connection, a log that keeps nothing, and a way to write logs.
// Only tests import it.
package kilntest

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/logstore"
)

// Conn is a scripted server connection: Feed puts a line on it, Sent is
// what was sent, Sizes the window sizes reported.
type Conn struct {
	lines     chan string
	prompts   chan string
	mu        sync.Mutex
	sent      []string
	sizes     [][2]int
	closeOnce sync.Once
}

func NewConn() *Conn {
	return &Conn{lines: make(chan string, 100), prompts: make(chan string, 1)}
}

func (c *Conn) Lines() <-chan string   { return c.lines }
func (c *Conn) Prompts() <-chan string { return c.prompts }
func (c *Conn) Err() error             { return nil }
func (c *Conn) Close() error           { c.closeOnce.Do(func() { close(c.lines) }); return nil }

// Feed has the server send line.
func (c *Conn) Feed(line string) { c.lines <- line }

// FeedPrompt has the server send an unterminated prompt.
func (c *Conn) FeedPrompt(p string) { c.prompts <- p }

func (c *Conn) Send(l string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, l)
	return nil
}

func (c *Conn) Resize(w, h int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sizes = append(c.sizes, [2]int{w, h})
	return nil
}

func (c *Conn) Sent() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.sent)
}

func (c *Conn) Sizes() [][2]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.sizes)
}

// NopLog is a session log that keeps nothing.
type NopLog struct{}

func (NopLog) Append(logstore.Entry) error { return nil }

// WriteLog logs lines one a minute from start where l says: a line
// starting "> " as sent, the rest as received.
func WriteLog(t testing.TB, l logstore.Layout, start time.Time, lines ...string) {
	t.Helper()
	w := logstore.NewWriter(l)
	defer w.Close()
	for i, s := range lines {
		dir := logstore.In
		if strings.HasPrefix(s, "> ") {
			dir, s = logstore.Out, strings.TrimPrefix(s, "> ")
		}
		if err := w.Append(logstore.Entry{Time: start.Add(time.Duration(i) * time.Minute), Dir: dir, Text: s}); err != nil {
			t.Fatal(err)
		}
	}
}
```

Before relying on it, read `ui`'s `testConn` (`model_test.go:31-69`) and `app`'s `lineConn`/`sentConn`: if either has a method not above, add it here. `kilntest` must not import `app` or `ui`.

Run: `go test ./internal/kilntest/` — Expected: PASS.

- [ ] **Step 3: Use it everywhere.**
  - `ui`: delete `testConn`, `newTestConn`, `memLog`; the harness's conns map holds `*kilntest.Conn` (`kilntest.NewConn()`), `NewLog` returns `kilntest.NopLog{}`. Replace `.lines <- s` with `.Feed(s)` and `.prompts <- p` with `.FeedPrompt(p)` (`grep -n '\.lines <-\|\.prompts <-' internal/ui/*_test.go`). `h.writeLog(start, lines...)` becomes a one-line wrapper: `kilntest.WriteLog(h.t, logstore.Layout{Root: h.m.d.LogRoot, World: "fm", Char: "kit", CharName: "Kit"}, start, lines...)`.
  - `app`: delete `lineConn`, `nopLog`, `sentConn`; `sessionApp`'s `Dial` returns `kilntest.NewConn()`, `NewLog` returns `kilntest.NopLog{}`, `attach` uses `kilntest.NopLog{}`; `submitApp` keeps the `*kilntest.Conn` it dials and returns it where it returned the `*sentConn`.
  - Fix imports.

- [ ] **Step 4: Run everything** (see Global Constraints). Expected: PASS, golden screens untouched.

- [ ] **Step 5: Commit**

```bash
git add internal/kilntest internal/app internal/ui
git commit -m "kilntest: one set of fakes for the app and ui tests"
```

---

### Task 2: `app.Log`, and the TUI's log mode draws it

**Files:**
- Create: `internal/app/log.go`, `internal/app/log_test.go`
- Modify: `internal/app/app.go` (`Char.Log`, `Char.Filter`, `Char.Echoes`), `internal/app/session.go` (`Handle`), `internal/ui/browse.go`, `internal/ui/filter.go`, `internal/ui/model.go`, `internal/ui/view.go`, `internal/ui/paint.go`, `internal/ui/scrollback.go` (if it calls `echoes`)
- Modify (access paths only): `internal/ui/browse_test.go`, `internal/ui/filter_test.go`, `internal/ui/theme_test.go`, `internal/ui/golden_test.go`, `internal/ui/web_test.go`, and any other hit of Step 7's grep
- Test: `internal/ui/paint_test.go` (create if absent) for `plainShown`

**Interfaces:**
- Consumes: Task 1's `kilntest.WriteLog`, `kilntest.NopLog`.
- Produces (all in `app`):
  - `const LogInitialLines = 200`
  - `type LogLine struct { Line; Tags []string; Lower string }`
  - `type Log struct { Lines []*LogLine; Cursor, Start, End *LogLine; Excluded map[*LogLine]bool; Find string; Status string; StatusErr bool; HistDone, Loading, Searching bool; Shown func(*LogLine) string; … }`
  - `type LogOlderMsg struct { Key string; Log *Log; Lines []*LogLine; Done bool; Err error; … }`
  - `Char.Log *Log`, `Char.Filter scene.Filter`, `func (c *Char) Echoes(e logstore.Entry) bool`
  - `func (a *App) OpenLog(k string) *Log`, `CloseLog(k string)`, `HandleLogOlder(msg LogOlderMsg) []Effect`
  - `Log` methods: `Older(then func() []Effect) []Effect`, `Last() *LogLine`, `Visible() []*LogLine`, `LastVisible() *LogLine`, `Index(l *LogLine) int`, `Shows(l *LogLine) bool`, `InRange(l *LogLine) bool`, `Items() []scene.Item`, `Refilter()`, `SetStatus(isErr bool, msg string)`, `ClearStatus()`, `SetCursor(l *LogLine)`, `MoveCursor(delta int) []Effect`, `ToTop() []Effect`, `ToBottom()`, `Mark()`, `NewRange(start, end *LogLine)`, `RangeStatus()`, `ExtendTo(l *LogLine)`, `ToggleExclude(l *LogLine)`, `Selection() []logstore.Entry`, `Title() string`, `SetFind(term string) []Effect`, `FindOlder(includeCursor bool) []Effect`, `FindNewer()`, `Matches() []*LogLine`, `FindStatus() string`, `StopSearch() bool`, `GotoDate(day string, show func(*LogLine)) []Effect`
  - `func FindRE(term string) *regexp.Regexp`, `func DayLabel(day string) string`
- The TUI's `browse` embeds `*app.Log` and keeps: `cs`, `top`, `scrolled`, `rowLines []*app.LogLine`, `prompt`, `pin`, `panel`, `format`, `run func([]app.Effect) tea.Cmd`, `painted map[*app.LogLine]string`, and (until Task 3) `copy`, `saveFile`, `exportDir`, `exportName`, `exportFormat`.

- [ ] **Step 1: Write the core tests** — `internal/app/log_test.go`:

```go
package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/kilntest"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/str"
)

var logDay = time.Date(2026, 9, 24, 9, 0, 0, 0, time.Local)

// logApp is sessionApp with Kit open and its logs written: each day of
// days is that many minutes of "<day> <n>" lines, the last day today.
func logApp(t *testing.T, days ...int) *App {
	t.Helper()
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	l, _ := a.LogLayout(a.Char("fm/kit").Ch)
	for i, n := range days {
		start := logDay.AddDate(0, 0, i-len(days)+1)
		var lines []string
		for j := range n {
			lines = append(lines, fmt.Sprintf("%s %d", start.Format("01-02"), j))
		}
		kilntest.WriteLog(t, l, start, lines...)
	}
	return a
}

// drain performs Run effects as a front end would until none are left.
func drain(a *App, effs []Effect) {
	for len(effs) > 0 {
		var next []Effect
		for _, e := range effs {
			if r, ok := e.(Run); ok {
				if msg, ok := r.Func().(LogOlderMsg); ok {
					next = append(next, a.HandleLogOlder(msg)...)
				}
			}
		}
		effs = next
	}
}

func TestOpenLogLoadsWholeDaysUpFront(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	if len(g.Lines) != 300 || g.HistDone {
		t.Fatalf("loaded %d lines, done %v; want two whole days", len(g.Lines), g.HistDone)
	}
	if g.Cursor != g.Lines[len(g.Lines)-1] || a.Char("fm/kit").Log != g || a.OpenLog("fm/kit") != g {
		t.Error("cursor not on the newest line, or the log isn't the character's")
	}
}

func TestOlderRunsWhatWaitedWhenTheDayArrives(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	ran := 0
	effs := g.Older(func() []Effect { ran++; return nil })
	if len(effs) != 1 || !g.Loading {
		t.Fatalf("Older gave %v, loading %v", effs, g.Loading)
	}
	if again := g.Older(func() []Effect { ran += 10; return nil }); again != nil {
		t.Error("a second read started while one is in flight")
	}
	drain(a, effs)
	if len(g.Lines) != 450 || !g.HistDone || g.Loading || ran != 10 {
		t.Errorf("lines %d, done %v, loading %v, ran %d (the newer request replaces the older)", len(g.Lines), g.HistDone, g.Loading, ran)
	}
}

func TestLogDropsAStaleOlderRead(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	effs := g.Older(nil)
	a.CloseLog("fm/kit")
	fresh := a.OpenLog("fm/kit")
	n := len(fresh.Lines)
	drain(a, effs)
	if len(fresh.Lines) != n {
		t.Errorf("a read for the closed log landed in the new one: %d → %d", n, len(fresh.Lines))
	}
	effs = fresh.Older(nil)
	a.Close("fm/kit")
	drain(a, effs) // the character is gone: nothing to do, and no panic
}

func TestLogRelinesAPageReadUnderOldRules(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	effs := g.Older(nil)
	c := a.Char("fm/kit")
	c.Ch.Rules.Classify = append(c.Ch.Rules.Classify, config.ClassifyRule{Tag: "early", Pattern: `^09-22 `})
	if err := c.compile(); err != nil {
		t.Fatal(err)
	}
	drain(a, effs)
	if l := g.Lines[0]; len(l.Tags) != 1 || l.Tags[0] != "early" {
		t.Errorf("the oldest day wasn't classified again: %q tagged %q", l.Entry.Text, l.Tags)
	}
}

func TestLogLiveDedupesAtMillisecondPrecision(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	l, _ := a.LogLayout(a.Char("fm/kit").Ch)
	base := logDay.Add(123456789 * time.Nanosecond) // not a whole millisecond
	w := logstore.NewWriter(l)
	var sent []logstore.Entry
	for i, text := range []string{"one", "two", "three"} {
		e := logstore.Entry{Time: base.Add(time.Duration(i) * time.Second), Dir: logstore.In, Text: text}
		w.Append(e)
		sent = append(sent, e)
	}
	w.Close()
	g := a.OpenLog("fm/kit")
	for _, e := range append(sent, logstore.Entry{Time: base.Add(5 * time.Second), Dir: logstore.In, Text: "four"}) {
		msg := lineMsg("fm/kit", s, e.Text)
		msg.Ev.Entry = e
		a.Handle(msg) // the same burst arrives as events after the log opened
	}
	var got []string
	for _, l := range g.Lines {
		got = append(got, l.Entry.Text)
	}
	if strings.Join(got, ",") != "one,two,three,four" {
		t.Errorf("lines = %q", got)
	}
}

func TestFindReadsOlderDaysUntilAMatch(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	effs := g.SetFind("09-22 7")
	if !g.Searching || g.FindStatus() != str.BrowseSearching("09-22 7", DayLabel("2026-09-23")) {
		t.Fatalf("searching %v, status %q", g.Searching, g.FindStatus())
	}
	drain(a, effs)
	if g.Searching || g.Cursor == nil || g.Cursor.Entry.Text != "09-22 79" {
		t.Errorf("searching %v, cursor %v", g.Searching, g.Cursor)
	}
}

func TestStopSearchLeavesTheCursor(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	cursor := g.Cursor
	effs := g.SetFind("nowhere")
	if !g.StopSearch() || g.Searching || g.Cursor != cursor || g.Status != str.BrowseSearchCancelled() {
		t.Fatalf("stop: searching %v, status %q", g.Searching, g.Status)
	}
	drain(a, effs) // the day still arrives, to nothing
	if g.Cursor != cursor {
		t.Error("the cancelled search moved the cursor")
	}
}

func TestGotoDateShowsTheDaysFirstLine(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	var shown *LogLine
	drain(a, g.GotoDate("2026-09-22", func(l *LogLine) { shown = l }))
	if shown == nil || shown != g.Cursor || shown.Entry.Text != "09-22 0" {
		t.Errorf("shown %v, cursor %v", shown, g.Cursor)
	}
	g.GotoDate("2026/09/22", nil)
	if g.Status != str.BrowseDateFormat() {
		t.Errorf("bad date: %q", g.Status)
	}
}

func TestSelectionIsReceivedShownUnexcludedLines(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	l, _ := a.LogLayout(a.Char("fm/kit").Ch)
	kilntest.WriteLog(t, l, logDay, "a", "> b", "c", "d")
	g := a.OpenLog("fm/kit")
	g.SetCursor(g.Lines[0])
	g.Mark()
	g.SetCursor(g.Lines[3])
	g.Mark()
	g.ToggleExclude(g.Lines[2])
	var got []string
	for _, e := range g.Selection() {
		got = append(got, e.Text)
	}
	if strings.Join(got, ",") != "a,d" || g.Status != str.BrowseLinesInRange(4) {
		t.Errorf("selection %q, status %q", got, g.Status)
	}
	g.ToggleExclude(nil)
	if !g.StatusErr {
		t.Error("excluding nothing didn't say why not")
	}
}

var _ = filepath.Join // keep the import if a later test drops its use
```

Notes for this step:
- `config.ClassifyRule` and its field names: check `internal/config` (`grep -n "type ClassifyRule\|Classify " internal/config/*.go`) and adjust the literal to match; add the `config` import.
- "09-22 7" first matches, reading newest-first, at "09-22 79" (the newest line on 09-22 containing it). Confirm against `fmt.Sprintf("%s %d")` output once the test runs; if the expected text is off, fix the expectation, not the code, and ledger it.
- Drop the trailing `var _ = filepath.Join` and the import if unused.

Run: `go test ./internal/app/` — Expected: FAIL to compile (`undefined: LogLine`, `a.OpenLog undefined`, …).

- [ ] **Step 2: `Char` gains the log, the filter and `Echoes`** — in `internal/app/app.go`, add to `Char` (exported, after `NeedPW`):

```go
	Log         *Log         // log mode, while it's open (shown or hidden)
	Filter      scene.Filter // log mode's filter; outlasts a log-mode session
```

and after `compile`:

```go
// Echoes reports whether e belongs in the scrollback and log mode:
// everything but sent lines, which only show with local_echo on. They're
// logged either way.
func (c *Char) Echoes(e logstore.Entry) bool {
	return e.Dir != logstore.Out || c.Ch.LocalEcho
}
```

- [ ] **Step 3: Write `internal/app/log.go`** — moved from `ui/browse.go`, with `b` → `g`, `*bline` → `*LogLine`, `b.cs.filter` → `g.c.Filter`, `b.cs.echoes` → `g.c.Echoes`, and `tea.Cmd` → `[]Effect`:

```go
package app

import (
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/latrani/Kiln/internal/history"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/scene"
	"github.com/latrani/Kiln/internal/str"
)

// LogInitialLines is how much history log mode loads up front (whole
// days, newest first, until at least this many lines).
const LogInitialLines = 200

// dedupeTail is how many of the newest loaded lines appendLive checks for
// a copy of an incoming line.
const dedupeTail = 256

// LogLine is a logged line in log mode, with what its filters read.
type LogLine struct {
	Line
	Tags  []string // Line.TagNames, kept for the filters
	Lower string   // Line.Plain, lowercased, for text filters
}

func makeLogLine(r Rules, e logstore.Entry) *LogLine {
	l := r.Line(e)
	return &LogLine{Line: l, Tags: l.TagNames(), Lower: strings.ToLower(l.Plain())}
}

// Log is one character's log mode. Lines are referenced by pointer so
// paging in older history never disturbs marks or exclusions. Front ends
// read its fields; only its methods change them.
type Log struct {
	Lines      []*LogLine // oldest first
	Cursor     *LogLine
	Start, End *LogLine // the marked range; End nil while only Start is marked
	Excluded   map[*LogLine]bool
	Find       string
	Status     string // log mode's message; the front end shows it and clears it
	StatusErr  bool
	HistDone   bool // every log day is loaded (or there is no history)
	Loading    bool // an older day is being read
	Searching  bool // a find is reading older days for a match
	// Shown is a line's text as the front end shows it, unstyled: what
	// find matches. nil: Line.Plain.
	Shown func(*LogLine) string

	a           *App
	c           *Char
	hist        *history.Reader
	loadedTo    time.Time      // newest logged time at open, at log (millisecond) precision
	pending     func() []Effect // what to do when the day being read arrives; see Older
	tagNames    []string        // the distinct tags on Lines[:tagNamesN]; see Items
	tagNamesN   int
	anyUntagged bool // some line in Lines[:tagNamesN] has no tags
}

// LogOlderMsg is one older day for log mode, read off the loop. It goes
// to HandleLogOlder.
type LogOlderMsg struct {
	Key   string
	Log   *Log // the log that asked; stale if it has since closed
	Lines []*LogLine
	Done  bool // history is now exhausted
	Err   error
	gen   int // the character's rulesGen when the read started
}

// OpenLog opens log mode for open character k over its logs, loading
// whole days until at least LogInitialLines, and returns it; an open log
// is returned as it is. nil if k isn't open.
func (a *App) OpenLog(k string) *Log {
	c := a.chars[k]
	if c == nil {
		return nil
	}
	if c.Log != nil {
		return c.Log
	}
	g := &Log{a: a, c: c, Excluded: map[*LogLine]bool{}}
	if l, ok := a.LogLayout(c.Ch); ok {
		if h, err := history.NewReader(l); err == nil {
			g.hist = h
		}
	}
	g.HistDone = g.hist == nil || g.hist.Exhausted()
	for len(g.Lines) < LogInitialLines && g.loadOlder() {
	}
	g.Cursor = g.LastVisible()
	if l := g.Last(); l != nil {
		g.loadedTo = l.Entry.Time
	}
	c.Filter.ResetSeen() // this session's items light by the filter as it stands
	c.Log = g
	return g
}

// CloseLog closes k's log mode. A day it was reading arrives to nothing.
func (a *App) CloseLog(k string) {
	if c := a.chars[k]; c != nil {
		c.Log = nil
	}
}

// readLogOlder reads and makes the next older day. Only one call may run
// at a time, and nothing else may touch h meanwhile.
func readLogOlder(h *history.Reader, r Rules) LogOlderMsg {
	es, _, _, err := h.LoadOlder()
	msg := LogOlderMsg{Done: h.Exhausted(), Err: err}
	for _, e := range es {
		msg.Lines = append(msg.Lines, makeLogLine(r, e))
	}
	return msg
}

// loadOlder synchronously prepends the next older day; only OpenLog uses
// it, for the bounded initial load. It reports false when there is
// nothing more to load.
func (g *Log) loadOlder() bool {
	if g.HistDone {
		return false
	}
	g.prepend(readLogOlder(g.hist, g.c.Rules))
	return true
}

func (g *Log) prepend(msg LogOlderMsg) {
	if msg.Err != nil {
		g.SetStatus(true, str.BrowseReadingLogs(msg.Err))
	}
	g.HistDone = msg.Done
	g.Lines = append(msg.Lines, g.Lines...)
}

// Older starts reading the next older day off the loop, so a big log
// never stalls it. then runs when it arrives (it may ask again to keep
// paging). While a read is in flight, a newer request just replaces then.
// Any request stops a find waiting on older days. It returns nothing if
// history is exhausted.
func (g *Log) Older(then func() []Effect) []Effect {
	if g.HistDone {
		return nil
	}
	g.pending = then
	g.Searching = false // a find waiting on older days gives way to whatever asked now
	if g.Loading {
		return nil
	}
	g.Loading = true
	h, r, gen, k := g.hist, g.c.Rules, g.c.rulesGen, g.c.Key
	return []Effect{Run{Func: func() any {
		msg := readLogOlder(h, r)
		msg.Key, msg.Log, msg.gen = k, g, gen
		return msg
	}}}
}

// HandleLogOlder prepends a day read by Older and runs what was waiting.
// A day for a log that has since closed arrives to nothing.
func (a *App) HandleLogOlder(msg LogOlderMsg) []Effect {
	c := a.chars[msg.Key]
	if c == nil || c.Log == nil || c.Log != msg.Log {
		return nil
	}
	g := c.Log
	g.Loading = false
	if msg.gen != c.rulesGen { // read under rules since replaced
		for _, l := range msg.Lines {
			l.Line = c.Rules.Reline(l.Line)
			l.Tags = l.TagNames()
		}
	}
	g.prepend(msg)
	then := g.pending
	g.pending = nil
	if then != nil {
		return then()
	}
	return nil
}

// appendLive adds a line that arrived while the log is open. Lines
// logged before it opened can still arrive as events afterwards; those
// are already loaded from the log (at millisecond precision), so a line
// no newer than the load watermark that matches a recently loaded one is
// skipped.
func (g *Log) appendLive(e logstore.Entry) {
	t := e.Time.Truncate(time.Millisecond)
	if !g.loadedTo.IsZero() && !t.After(g.loadedTo) {
		for _, l := range g.Lines[max(0, len(g.Lines)-dedupeTail):] {
			if l.Entry.Text == e.Text && l.Entry.Dir == e.Dir && l.Entry.Time.Truncate(time.Millisecond).Equal(t) {
				return
			}
		}
	}
	atEnd := g.Cursor == nil || g.Cursor == g.LastVisible()
	g.Lines = append(g.Lines, makeLogLine(g.c.Rules, e))
	if atEnd {
		g.Cursor = g.LastVisible()
	}
}

// Last is the newest loaded line; nil with none.
func (g *Log) Last() *LogLine {
	if len(g.Lines) == 0 {
		return nil
	}
	return g.Lines[len(g.Lines)-1]
}

// SetStatus puts msg on log mode's status, as an error if isErr.
func (g *Log) SetStatus(isErr bool, msg string) { g.Status, g.StatusErr = msg, isErr }

// ClearStatus takes log mode's message down.
func (g *Log) ClearStatus() { g.Status = "" }

// Shows reports whether l is drawn: sent lines only with local_echo on
// (they're logged either way), and whatever the filter lets through. The
// lines stay loaded, so turning local_echo on brings them back.
func (g *Log) Shows(l *LogLine) bool {
	return g.c.Echoes(l.Entry) && g.c.Filter.Visible(l.Tags, l.Lower)
}

// Visible is the shown lines, oldest first.
func (g *Log) Visible() []*LogLine {
	out := make([]*LogLine, 0, len(g.Lines))
	for _, l := range g.Lines {
		if g.Shows(l) {
			out = append(out, l)
		}
	}
	return out
}

// LastVisible is the newest shown line; nil with none.
func (g *Log) LastVisible() *LogLine {
	v := g.Visible()
	if len(v) == 0 {
		return nil
	}
	return v[len(v)-1]
}

// Index is l's position in Lines; -1 if it isn't there.
func (g *Log) Index(l *LogLine) int { return slices.Index(g.Lines, l) }

// InRange reports whether l is inside the marked range. With only a
// start marked, just the start counts.
func (g *Log) InRange(l *LogLine) bool {
	if g.Start == nil {
		return false
	}
	if g.End == nil {
		return l == g.Start
	}
	i := g.Index(l)
	return i >= g.Index(g.Start) && i <= g.Index(g.End)
}

// Items is the filter panel's list: Untagged, every tag on a loaded line
// or with a filter set (with their parents), then the text rows. It
// syncs the filter, so a tag seen for the first time obeys the filter
// already set.
func (g *Log) Items() []scene.Item {
	if g.tagNamesN != len(g.Lines) { // lines are only ever added
		seen := map[string]bool{}
		g.tagNames, g.anyUntagged = nil, false
		for _, l := range g.Lines {
			if len(l.Tags) == 0 {
				g.anyUntagged = true
			}
			for _, t := range l.Tags {
				if !seen[t] {
					seen[t] = true
					g.tagNames = append(g.tagNames, t)
				}
			}
		}
		g.tagNamesN = len(g.Lines)
	}
	f := &g.c.Filter
	untagged := scene.Item{Untagged: true}
	only, hasOnly := f.Only()
	var items []scene.Item
	if g.anyUntagged || f.Hidden(untagged) || hasOnly && only == untagged {
		items = append(items, untagged)
	}
	items = append(items, scene.TagItems(append(slices.Clone(g.tagNames), f.Tags()...))...)
	items = append(items, f.Texts()...)
	f.Sync(items)
	return items
}

// Refilter keeps the reader's place after the filter changes: a cursor
// on a line now hidden moves to the nearest visible one.
func (g *Log) Refilter() {
	g.Items() // sync
	if g.Cursor != nil && !g.Shows(g.Cursor) {
		if n := g.nearestVisible(g.Cursor); n != nil {
			g.Cursor = n
		}
	}
}

// nearestVisible returns the first visible line after l, or failing that
// the last visible line before it, so hiding l keeps the reader's place.
func (g *Log) nearestVisible(l *LogLine) *LogLine {
	i := g.Index(l)
	for j := i + 1; j < len(g.Lines); j++ {
		if g.Shows(g.Lines[j]) {
			return g.Lines[j]
		}
	}
	for j := i - 1; j >= 0; j-- {
		if g.Shows(g.Lines[j]) {
			return g.Lines[j]
		}
	}
	return nil
}

// SetCursor puts the cursor on l: a click, or the view dragging it along.
func (g *Log) SetCursor(l *LogLine) { g.Cursor = l }

// MoveCursor moves by delta visible lines. Moving up past the oldest
// loaded line stops there and pages in older history; the rest of the
// move happens when it arrives.
func (g *Log) MoveCursor(delta int) []Effect {
	g.StopSearch() // the wheel, say: you've moved on
	v := g.Visible()
	if len(v) == 0 {
		if delta < 0 { // everything loaded is hidden; look further back
			return g.Older(func() []Effect { return g.MoveCursor(delta) })
		}
		return nil
	}
	i := slices.Index(v, g.Cursor)
	if i < 0 {
		i = len(v) - 1
	}
	g.Cursor = v[min(max(0, i+delta), len(v)-1)]
	if rest := i + delta; rest < 0 {
		return g.Older(func() []Effect { return g.MoveCursor(rest) })
	}
	return nil
}

// ToTop pages in all history, then moves to the oldest visible line.
func (g *Log) ToTop() []Effect {
	if v := g.Visible(); len(v) > 0 {
		g.Cursor = v[0]
	}
	return g.Older(g.ToTop)
}

// ToBottom moves to the newest visible line.
func (g *Log) ToBottom() { g.Cursor = g.LastVisible() }

// Mark marks the cursor's line as the start of a range, or, with a start
// marked, as its other end.
func (g *Log) Mark() {
	if g.Cursor == nil {
		return
	}
	if g.Start == nil || g.End != nil {
		g.NewRange(g.Cursor, nil)
		g.SetStatus(false, str.BrowseRangeStart())
		return
	}
	g.End = g.Cursor
	if g.Index(g.End) < g.Index(g.Start) {
		g.Start, g.End = g.End, g.Start
	}
	g.RangeStatus()
}

// NewRange starts a range, dropping exclusions left from an earlier one
// so they can't resurface if it grows over them.
func (g *Log) NewRange(start, end *LogLine) {
	g.Start, g.End = start, end
	clear(g.Excluded)
}

// RangeStatus says how many lines the range holds.
func (g *Log) RangeStatus() {
	g.SetStatus(false, str.BrowseLinesInRange(g.Index(g.End)-g.Index(g.Start)+1))
}

// ExtendTo grows the range to take in l, from whichever end is nearer.
// With only a start marked, l becomes the other end.
func (g *Log) ExtendTo(l *LogLine) {
	if g.End == nil {
		g.End = g.Start
	}
	if g.Index(l) < g.Index(g.Start) {
		g.Start = l
	} else if g.Index(l) > g.Index(g.End) {
		g.End = l
	}
	g.RangeStatus()
}

// ToggleExclude takes l out of the range, or puts it back.
func (g *Log) ToggleExclude(l *LogLine) {
	if l == nil || g.End == nil || !g.InRange(l) {
		g.SetStatus(true, str.BrowseExcludeNeedsRange())
		return
	}
	g.Excluded[l] = !g.Excluded[l]
}

// Selection is what an export contains: received lines inside the range,
// not excluded and not hidden by the filter.
func (g *Log) Selection() []logstore.Entry {
	if g.Start == nil || g.End == nil {
		return nil
	}
	var out []logstore.Entry
	for _, l := range g.Lines[g.Index(g.Start) : g.Index(g.End)+1] {
		if !g.Excluded[l] && g.Shows(l) && scene.Exportable(l.Entry) {
			out = append(out, l.Entry)
		}
	}
	return out
}

// Title is an export's title: world, character, and the range's day.
func (g *Log) Title() string {
	ch := g.c.Ch
	when := ""
	if g.Start != nil {
		when = " — " + g.Start.Entry.Time.Local().Format(str.DateDayYear())
	}
	return ch.World + " " + ch.Name + when
}

// FindRE matches a find term literally and case-insensitively. Match
// offsets always index the original text: lowercasing a copy and reusing
// its offsets breaks on letters whose case forms differ in byte length.
func FindRE(term string) *regexp.Regexp {
	return regexp.MustCompile("(?i)" + regexp.QuoteMeta(term))
}

func (g *Log) shown(l *LogLine) string {
	if g.Shown != nil {
		return g.Shown(l)
	}
	return l.Plain()
}

// Matches is the shown lines the find term matches, oldest first.
func (g *Log) Matches() []*LogLine {
	if g.Find == "" {
		return nil
	}
	re := FindRE(g.Find)
	var out []*LogLine
	for _, l := range g.Visible() {
		if re.MatchString(g.shown(l)) {
			out = append(out, l)
		}
	}
	return out
}

// matchAt reports whether l is a find match that's shown.
func (g *Log) matchAt(re *regexp.Regexp, l *LogLine) bool {
	return g.Shows(l) && re.MatchString(g.shown(l))
}

// SetFind sets the find term and goes to its nearest match at the cursor
// or older.
func (g *Log) SetFind(term string) []Effect {
	g.Find = term
	return g.FindOlder(true)
}

// FindOlder moves to the nearest match older than the cursor (or at it,
// with includeCursor), the way find goes: newest first. It reads in
// older days until one turns up or history runs out; it doesn't wrap.
func (g *Log) FindOlder(includeCursor bool) []Effect {
	if g.Find == "" {
		return nil
	}
	i := len(g.Lines) - 1
	if ci := g.Index(g.Cursor); ci >= 0 {
		i = ci
		if !includeCursor {
			i--
		}
	}
	return g.findOlderFrom(i)
}

// findOlderFrom looks for a match at line i or older; see FindOlder.
func (g *Log) findOlderFrom(i int) []Effect {
	re := FindRE(g.Find)
	g.Searching = false
	for ; i >= 0; i-- {
		if l := g.Lines[i]; g.matchAt(re, l) {
			g.Cursor = l
			g.Status = ""
			return nil
		}
	}
	if !g.HistDone {
		var oldest *LogLine // where this pass stopped; the next goes on below it
		if len(g.Lines) > 0 {
			oldest = g.Lines[0]
		}
		effs := g.Older(func() []Effect { return g.findOlderFrom(g.Index(oldest) - 1) })
		g.Searching = true
		return effs
	}
	g.findFailed(str.BrowseNoOlderMatches(g.Find))
	return nil
}

// searchedTo is the oldest day loaded, which a search has read through.
func (g *Log) searchedTo() string {
	if len(g.Lines) == 0 {
		return ""
	}
	return DayLabel(g.Lines[0].Day)
}

// StopSearch ends a find that's reading older days, leaving the cursor
// where it was, and reports whether there was one. The day being read
// still arrives, to nothing.
func (g *Log) StopSearch() bool {
	if !g.Searching {
		return false
	}
	g.Searching, g.pending = false, nil
	g.SetStatus(false, str.BrowseSearchCancelled())
	return true
}

// FindNewer moves to the nearest match newer than the cursor. Everything
// newer is always loaded; it doesn't wrap.
func (g *Log) FindNewer() {
	if g.Find == "" {
		return
	}
	re := FindRE(g.Find)
	for _, l := range g.Lines[g.Index(g.Cursor)+1:] {
		if g.matchAt(re, l) {
			g.Cursor = l
			g.Status = ""
			return
		}
	}
	g.findFailed(str.BrowseNoNewerMatches(g.Find))
}

// findFailed says there's nothing further: that the term matches nothing
// at all, if so, else msg.
func (g *Log) findFailed(msg string) {
	if len(g.Matches()) == 0 && g.HistDone {
		msg = str.BrowseNoMatches(g.Find)
	}
	g.SetStatus(true, msg)
}

// matchPos is the cursor's match counted from the newest, of n loaded;
// more says older history not yet loaded may hold others.
func (g *Log) matchPos() (i, n int, more bool) {
	ms := g.Matches()
	if k := slices.Index(ms, g.Cursor); k >= 0 {
		i = len(ms) - k
	}
	return i, len(ms), !g.HistDone
}

// FindStatus is the find term and match position, for the top bar; ""
// without a find.
func (g *Log) FindStatus() string {
	if g.Find == "" {
		return ""
	}
	if g.Searching {
		return str.BrowseSearching(g.Find, g.searchedTo())
	}
	i, n, more := g.matchPos()
	if more {
		return str.BrowseFindStatusMore(g.Find, i, n)
	}
	return str.BrowseFindStatus(g.Find, i, n)
}

// GotoDate pages in history back to day (YYYY-MM-DD), then moves to its
// first visible line and hands it to show, for the front end to put at
// the top of what it draws.
func (g *Log) GotoDate(day string, show func(*LogLine)) []Effect {
	if _, err := time.Parse("2006-01-02", day); err != nil {
		g.SetStatus(true, str.BrowseDateFormat())
		return nil
	}
	if (len(g.Lines) == 0 || g.Lines[0].Day > day) && !g.HistDone {
		return g.Older(func() []Effect { return g.GotoDate(day, show) })
	}
	for _, l := range g.Visible() {
		if l.Day >= day {
			g.Cursor = l
			if show != nil {
				show(l)
			}
			g.Status = ""
			return nil
		}
	}
	g.SetStatus(true, str.BrowseNoLogsAfter(day))
	return nil
}

// DayLabel formats "2026-09-24" as "Thu Sep 24".
func DayLabel(day string) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	return t.Format(str.DateDay())
}
```

- [ ] **Step 4: `Handle` feeds the log** — in `internal/app/session.go`'s `session.EventLine` case, after the unread count and before `note = …`:

```go
		if c.Log != nil {
			c.Log.appendLive(msg.Ev.Entry)
		}
```

Run: `go test ./internal/app/` — Expected: PASS (fix test expectations only per Step 1's notes, with a ledger line).

- [ ] **Step 5: The TUI draws the core's log** — `internal/ui/paint.go`, add:

```go
// plainShown is l as paint draws it, without the styling: what find
// matches in log mode.
func plainShown(l app.Line) string {
	switch l.Kind {
	case app.Echo:
		return gutterMark + l.Plain()
	case app.Sys:
		return "* " + l.Plain()
	}
	return l.Plain()
}
```

and its test, `internal/ui/paint_test.go` (append if the file exists):

```go
// Find matches what's drawn: plainShown must be paint with the styling
// stripped, for every kind of line log mode shows, highlighted ones too.
func TestPlainShownIsPaintUnstyled(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	cs := h.m.chars["fm/kit"]
	for _, e := range []logstore.Entry{
		{Dir: logstore.In, Text: "Rook pages, \"Kit, you there?\""},
		{Dir: logstore.In, Text: "\x1b[1mbold\x1b[0m and plain"},
		{Dir: logstore.Out, Text: "look"},
		{Dir: logstore.Sys, Text: "connected"},
	} {
		l := cs.Rules.Line(e)
		if got, want := plainShown(l), ansi.Strip(paint(cs.hl, l)); got != want {
			t.Errorf("%q: plainShown %q, paint shows %q", e.Text, got, want)
		}
	}
}
```

(check `logstore.Sys` is the sys direction's name; `grep -n "Sys\b\|Out\b\|In\b" internal/logstore/*.go | head`.)

Then rework `internal/ui/browse.go`:
  - Delete `bline`, `olderMsg`, `newBrowse`, `newLine`, `makeLine`, `readOlder`, `loadOlder`, `prepend`, `requestOlder`, `receive`, `dedupeTail`, `appendLive`, `last`, `setStatus`, `visible`, `lastVisible`, `index`, `inRange`, `items`, `findStatus`, `shows`, `moveCursor`, `toTop`, `mark`, `newRange`, `rangeStatus`, `extendTo`, `toggleExclude`, `findRE`, `matches`, `matchAt`, `findOlder`, `findOlderFrom`, `searchedTo`, `stopSearch`, `findNewer`, `findFailed`, `matchPos`, `gotoDate`, `selection`, `title`, `refilter`, `nearestVisible`, `dayLabel`, `browseInitialLines`. Keep `browsePrefixW`, `promptKind`, `scrollBy`, `copyCmd`, `save`, `key`, `lineTags`, `promptKey`, `promptLabel`, `rowsFor`, `scrollToCursor`, `bottomTop`, `view`, `restyle`, `highlightFind`, `click`, `promptWindow`, `setExport`.
  - The struct becomes:

    ```go
    // browse is one character's log mode as the terminal draws it, over
    // the core's log.
    type browse struct {
    	*app.Log
    	cs           *charState
    	top          *app.LogLine // first line drawn at the top of the body
    	scrolled     bool         // top was set by scrollBy: the cursor follows the view, not the other way round
    	rowLines     []*app.LogLine // body row → line (nil for dividers), from the last draw
    	painted      map[*app.LogLine]string
    	prompt       promptKind
    	pin          *Input
    	panel        *filterPanel // non-nil while the filter panel is open
    	format       string
    	run          func([]app.Effect) tea.Cmd           // the model's run
    	copy         func(text string) tea.Cmd            // the model's clipboard write; nil: plain OSC 52
    	saveFile     func(name string, data []byte) error // Deps.SaveFile; set, saving downloads instead
    	exportDir    string
    	exportName   string // file name template; see config.ExportNameVars
    	exportFormat string // preselected format; "" asks
    }

    // text is l painted, painting it the first time it's needed.
    func (b *browse) text(l *app.LogLine) string {
    	s, ok := b.painted[l]
    	if !ok {
    		s = paint(b.cs.hl, l.Line)
    		b.painted[l] = s
    	}
    	return s
    }
    ```
  - Everywhere in the kept code: `*bline` → `*app.LogLine`; `l.text` → `b.text(l)`; `l.tags` → `l.Tags`; `b.cursor` → `b.Cursor` (reads) and `b.SetCursor(x)` (writes); `b.lines` → `b.Lines`; `b.start`/`b.end`/`b.excluded`/`b.find`/`b.histDone`/`b.loading`/`b.searching` → their capitalized fields; `b.setStatus` → `b.SetStatus`; `b.status = ""` → `b.ClearStatus()`; `b.visible()` → `b.Visible()`; `b.lastVisible()` → `b.LastVisible()`; `b.index` → `b.Index`; `b.inRange` → `b.InRange`; `b.items()` → `b.Items()`; `b.matches()` → `b.Matches()`; `b.selection()` → `b.Selection()`; `b.title()` → `b.Title()`; `b.refilter()` → `b.Refilter()`; `b.stopSearch()` → `b.StopSearch()`; `dayLabel` → `app.DayLabel`; `findRE` → `app.FindRE`; `b.cs.filter` → `b.cs.Filter`.
  - `scrollBy` returns `[]app.Effect`; its paging line becomes `return b.Older(func() []app.Effect { return b.scrollBy(rest) })`.
  - `key`: each case that returned a `tea.Cmd` from a moved method returns `b.run(…)` of it: `b.run(b.MoveCursor(-1))`, `b.run(b.MoveCursor(max(1, pageH-1)))`, `b.run(b.ToTop())`, `b.run(b.FindOlder(false))`. `actBottom` → `b.ToBottom()`; `actMark` → `b.Mark()`; `actExclude` → `b.ToggleExclude(b.Cursor)`; `actFind` → `b.pin.SetValue(b.Find)`; `actPrevMatch` → `b.FindNewer()`.
  - `promptKey`: `promptFind` → `return b.run(b.SetFind(v))`; `promptDate` → `return b.run(b.GotoDate(v, func(l *app.LogLine) { b.top = l }))`; `promptFilterText` → `b.cs.Filter.AddText(v, b.Items())`, then `b.Refilter()`.
  - `view`: the "no logs yet" row tests `len(b.Lines) == 0 && b.HistDone`; the loading row `b.Loading`; `ms := b.Matches()`; `highlightFind(ansi.Strip(b.text(l)), b.Find)`; `text := b.text(l)`; `b.items()` at the top → `b.Items()`.
  - `restyle()` takes no argument and does `clear(b.painted)`.
  - `click`: `b.StopSearch()`; `b.NewRange(b.Cursor, nil)`; `b.SetCursor(l)`; `b.ExtendTo(l)`; `b.ToggleExclude(l)`; `b.NewRange(l, l)`; `b.RangeStatus()`.
  - `scrollToCursor` writes the cursor with `b.SetCursor(v[…])`.
  - `copyCmd`, `save`, the export branches of `key` and `promptKey`, and `setExport` stay as they are this task, reading `b.Selection()`, `b.Title()`, `b.cs.Ch`, and setting `b.SetStatus`.

  `internal/ui/filter.go`: `b.cs.filter` → `b.cs.Filter`; `b.items()` → `b.Items()`; `b.refilter()` → `b.Refilter()`. `collapsed`/`foldSeen` stay on `charState` (4b).

- [ ] **Step 6: The model** — `internal/ui/model.go`:
  - `charState`: delete `filter` (it's `Char.Filter` now). Delete `charState.echoes`; callers use `cs.Echoes`.
  - `hideBrowse`: `if cs.browse.StopSearch() { cs.browse.ClearStatus() }`.
  - `openBrowse`, the new-log branch:

    ```go
    	g := m.a.OpenLog(cs.Key)
    	cs.browse = &browse{Log: g, cs: cs, pin: NewInput(), run: m.run, painted: map[*app.LogLine]string{}}
    	g.Shown = func(l *app.LogLine) string { return plainShown(l.Line) }
    	cs.browse.copy = m.copyCmd
    	cs.browse.saveFile = m.d.SaveFile
    	cs.browse.setExport(m.a.Config())
    ```
  - Where `key` reports the log closed (`cmd, closed := cs.browse.key(…)`), also `m.a.CloseLog(cs.Key)`.
  - `update`: replace the `olderMsg` case with `case app.LogOlderMsg: return m, m.run(m.a.HandleLogOlder(msg))`.
  - `handleEvent`: delete the `for _, b := range cs.browses() { b.appendLive(…) }` loop (the core's `Handle` does it).
  - `loadTheme`: `b.restyle()`.
  - `takeLogStatus`: `cs.browse.Status`, `cs.browse.StatusErr`, then `cs.browse.ClearStatus()`.
  - `handleWheel`: `return m.run(cs.browse.scrollBy(…))`.
  - `internal/ui/view.go`: `cs.browse.selection()` → `cs.browse.Selection()`, `b.findStatus()` → `b.FindStatus()`, and the same renames wherever `grep -n 'browse\.' internal/ui/view.go` shows a moved one.
  - Fix every remaining compile error the same way (`go build ./...`), and delete `ui`'s `dayLabel` if nothing but the moved code used it, else make it `return app.DayLabel(day)`.

- [ ] **Step 7: Tests reach the core** —

```bash
grep -rnE '\bb\.(cursor|lines|start|end|excluded|find|histDone|loading|searching|status|statusErr)\b|\bb\.(items|refilter|index|findStatus|selection|last|findOlder|visible|lastVisible|requestOlder|appendLive)\(|cs\.filter\b|\.text\b|\.tags\b|olderMsg|dayLabel' internal/ui/*_test.go
```

Change access paths only: fields capitalized, `b.text(l)` for a log line's painted text, `l.Tags`, `cs.Filter`; methods that now return `[]app.Effect` are run through `b.run(…)` where the test needs the `tea.Cmd` (`cmd := b.run(b.FindOlder(true))`; `b.Older(nil)` likewise). `TestBrowseLiveDedupeAtMillisecondPrecision` is deleted from `browse_test.go`: Step 1's `TestLogLiveDedupesAtMillisecondPrecision` replaces it (ledger it). Scrollback lines keep their own `.text`; only log-mode lines change.

- [ ] **Step 8: Run everything** (see Global Constraints). Expected: PASS, golden screens untouched (`git status internal/ui/testdata` clean).

- [ ] **Step 9: Commit**

```bash
git add internal/app internal/ui
git commit -m "app: log mode's lines, paging, cursor, range and find live in the core"
```

---

### Task 3: copy and export as effects

**Files:**
- Modify: `internal/app/effect.go` (`Copy`, `SaveFile`), `internal/app/log.go` (`Copy`, `ExportReady`, `ExportFileName`, `Export`), `internal/app/log_test.go`, `internal/ui/browse.go`, `internal/ui/model.go` (`run`, `saveFile`, `openBrowse`, `applyWith`)
- Test: `internal/ui/browse_test.go` (one new test)

**Interfaces:**
- Consumes: Task 2's `Log`.
- Produces: `type app.Copy struct{ Text string }`, `type app.SaveFile struct{ Key, Name string; Data []byte }` (Key: whose log asked; Name: the file name as typed, trimmed); `Log.Copy() []Effect`, `Log.ExportReady() bool`, `Log.ExportFileName(format, dir string) (string, bool)`, `Log.Export(format, name string) []Effect`. The TUI's `browse` loses `copy`, `saveFile`, `exportDir`, `exportName`, `exportFormat`, `copyCmd`, `setExport`; `Model` gains `saveFile(e app.SaveFile)`. `browse` gains `cfg func() *config.Config` and `downloads bool`.

- [ ] **Step 1: Write the core tests** — append to `internal/app/log_test.go`:

```go
func TestCopyAndExportAreEffects(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	l, _ := a.LogLayout(a.Char("fm/kit").Ch)
	kilntest.WriteLog(t, l, logDay, "a", "b")
	g := a.OpenLog("fm/kit")
	if effs := g.Copy(); effs != nil || g.Status != str.BrowseMarkRange() {
		t.Errorf("copy with no range: %v, %q", effs, g.Status)
	}
	if g.ExportReady() || g.Status != str.BrowseMarkRange() {
		t.Error("export ready with no range")
	}
	g.SetCursor(g.Lines[0])
	g.Mark()
	g.SetCursor(g.Lines[1])
	g.Mark()
	if effs := g.Copy(); len(effs) != 1 || effs[0] != (Copy{Text: scene.Plain(g.Selection())}) || g.Status != str.BrowseCopied(2) {
		t.Errorf("copy: %v, %q", effs, g.Status)
	}
	if name, ok := g.ExportFileName("txt", ""); !ok || name == "" || strings.Contains(name, "/") {
		t.Errorf("default download name %q, %v", name, ok)
	}
	if effs := g.Export("txt", "  "); effs != nil || g.Status != str.BrowseNoFileName() {
		t.Errorf("empty name: %v, %q", effs, g.Status)
	}
	effs := g.Export("txt", " scene.txt ")
	if len(effs) != 1 {
		t.Fatalf("export: %v", effs)
	}
	if s, ok := effs[0].(SaveFile); !ok || s.Key != "fm/kit" || s.Name != "scene.txt" || string(s.Data) != scene.Render("txt", g.Selection(), g.Title()) {
		t.Errorf("SaveFile = %+v", effs[0])
	}
}
```

(add the `scene` import.) Run: `go test ./internal/app/` — Expected: FAIL to compile (`g.Copy undefined`, `undefined: SaveFile`).

- [ ] **Step 2: Effects and methods** — `internal/app/effect.go`:

```go
// Copy puts Text on the clipboard.
type Copy struct{ Text string }

// SaveFile offers Data as a file: in a browser a download named after
// Name's last element; in the terminal a file at Name (relative to
// export_dir, "~/" expanded), never overwriting one. Key is the
// character whose log mode asked, for where to say how it went.
type SaveFile struct {
	Key, Name string
	Data      []byte
}
```

with `func (Copy) effect() {}` and `func (SaveFile) effect() {}`.

`internal/app/log.go`:

```go
// Copy puts the selection on the clipboard as plain text, or says to
// mark a range first.
func (g *Log) Copy() []Effect {
	sel := g.Selection()
	if len(sel) == 0 {
		g.SetStatus(true, str.BrowseMarkRange())
		return nil
	}
	g.SetStatus(false, str.BrowseCopied(len(sel)))
	return []Effect{Copy{Text: scene.Plain(sel)}}
}

// ExportReady reports whether there's a selection to export, and if not
// says to mark a range first.
func (g *Log) ExportReady() bool {
	if len(g.Selection()) == 0 {
		g.SetStatus(true, str.BrowseMarkRange())
		return false
	}
	return true
}

// ExportFileName is the file name an export in format starts with, from
// export_name, in dir ("" for a download: just a name). false, saying
// so, if nothing's left to export.
func (g *Log) ExportFileName(format, dir string) (string, bool) {
	sel := g.Selection()
	if len(sel) == 0 {
		g.SetStatus(true, str.BrowseNothingLeft())
		return "", false
	}
	return scene.FileName(dir, g.a.cfg.ExportName, sel[0].Time.Local(), g.c.Ch.World, g.c.Ch.Name, format), true
}

// Export renders the selection in format and offers it as a file called
// name.
func (g *Log) Export(format, name string) []Effect {
	name = strings.TrimSpace(name)
	if name == "" {
		g.SetStatus(true, str.BrowseNoFileName())
		return nil
	}
	sel := g.Selection()
	if len(sel) == 0 {
		g.SetStatus(true, str.BrowseNothingToExport())
		return nil
	}
	return []Effect{SaveFile{Key: g.c.Key, Name: name, Data: []byte(scene.Render(format, sel, g.Title()))}}
}
```

Run: `go test ./internal/app/` — Expected: PASS.

- [ ] **Step 3: The TUI performs them** — `internal/ui/model.go`:
  - `run`: add

    ```go
		case app.Copy:
			cmds = append(cmds, m.copyCmd(e.Text))
		case app.SaveFile:
			m.saveFile(e)
    ```
  - Add `saveFile`, the body of `browse.save` after the selection, reporting on the asking log:

    ```go
    // saveFile offers a file: through Deps.SaveFile (the web build) a
    // download named after the name's last element, else a file at the
    // name, "~/" expanded and a relative one placed in export_dir, never
    // overwriting. How it went goes on the asking log's status.
    func (m *Model) saveFile(e app.SaveFile) {
    	say := m.setStatus
    	if c := m.a.Char(e.Key); c != nil && c.Log != nil {
    		say = c.Log.SetStatus
    	}
    	if m.d.SaveFile != nil {
    		name := filepath.Base(e.Name)
    		if err := m.d.SaveFile(name, e.Data); err != nil {
    			say(true, err.Error())
    			return
    		}
    		say(false, str.StatusDownloaded(name))
    		return
    	}
    	path, err := config.ExpandHome(e.Name)
    	if err != nil {
    		say(true, err.Error())
    		return
    	}
    	if !filepath.IsAbs(path) {
    		dir := m.a.Config().ExportDir
    		if dir == "" {
    			say(true, str.BrowseNoExportDir())
    			return
    		}
    		path = filepath.Join(dir, path)
    	}
    	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
    		say(true, err.Error())
    		return
    	}
    	// O_EXCL makes "never overwrite" atomic: no window between checking
    	// for the file and creating it.
    	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
    	if errors.Is(err, os.ErrExist) {
    		say(true, str.BrowseFileExists(filepath.Base(path)))
    		return
    	} else if err != nil {
    		say(true, err.Error())
    		return
    	}
    	_, err = f.Write(e.Data)
    	if cerr := f.Close(); err == nil {
    		err = cerr
    	}
    	if err != nil {
    		say(true, err.Error())
    		return
    	}
    	say(false, str.BrowseSaved(path))
    }
    ```
  - `openBrowse`: drop the `copy`, `saveFile` and `setExport` lines. `applyWith`: drop the `setExport` loop over `cs.browses()`.
- `internal/ui/browse.go`: delete `copy`, `saveFile`, `exportDir`, `exportName`, `exportFormat` from the struct, and `copyCmd` and `setExport`. Then:
  - `save(path string)` becomes `func (b *browse) save(path string) { b.run(b.Export(b.format, path)) }` (`run` performs `SaveFile` on the spot, so tests that check the file right after still see it).
  - `key`'s `actExport`: `if !b.ExportReady() { break }; b.prompt = promptFormat`. `actCopy`: `return b.run(b.Copy()), false`.
  - Export settings are read from the config when they're used, instead of copied in by `setExport`. Give `browse` two fields set in `openBrowse`: `cfg func() *config.Config` (`m.a.Config`) and `downloads bool` (`m.d.SaveFile != nil`). Read `b.cfg().ExportFormat` wherever `b.exportFormat` was read.
  - `promptKey`'s format branch, after choosing `f` (the `exportFormatKeys` lookup and the Enter-takes-the-preselected-format rule stay as they are):

    ```go
    		dir := b.cfg().ExportDir
    		if b.downloads { // a download: just a name
    			dir = ""
    		}
    		name, ok := b.ExportFileName(f, dir)
    		if !ok {
    			b.prompt = promptNone
    			return nil
    		}
    		b.format, b.prompt = f, promptFilename
    		b.pin.SetValue(name)
    ```

  - `promptLabel`: `b.cfg().ExportFormat` where it read `b.exportFormat`.

- [ ] **Step 4: Pin where the status lands** — append to `internal/ui/browse_test.go`:

```go
// Saving says how it went on log mode's status, so the next log-mode key
// clears it like any log message.
func TestSaveFileStatusIsTheLogs(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.a.Config().ExportDir = t.TempDir()
	h.writeLog(day24, "a", "b")
	h.key("ctrl+l")
	b := h.br()
	b.SetCursor(b.Lines[0])
	b.Mark()
	b.SetCursor(b.Lines[1])
	b.Mark()
	b.format = "txt"
	b.ClearStatus()
	b.save("scene.txt")
	if want := str.BrowseSaved(filepath.Join(h.m.a.Config().ExportDir, "scene.txt")); b.Status != want {
		t.Errorf("log status %q, want %q", b.Status, want)
	}
}
```

Run: `go test ./internal/ui -run SaveFileStatus` — Expected: PASS. Then check it bites: temporarily make `saveFile`'s `say` always `m.setStatus`, run it, see it FAIL, and restore.

- [ ] **Step 5: Tests reach the core** — `grep -rnE '\bb\.(copy|saveFile|exportDir|exportName|exportFormat|copyCmd|setExport)\b' internal/ui/*_test.go`; tests that set export settings on the browse set them on the config instead (`h.m.a.Config().ExportDir = …`), and a web test that set `b.saveFile` sets the harness's `Deps.SaveFile` (setup only).

- [ ] **Step 6: Run everything** (see Global Constraints). Expected: PASS, golden screens untouched.

- [ ] **Step 7: Commit**

```bash
git add internal/app internal/ui
git commit -m "app: log mode's copy and export are effects the front end performs"
```

Then: fresh-reviewer pass over the branch against `main` with this plan's Review Focus and side-effect list, fix Critical/Important with a failing test first, open the PR (no `Strings:` trailer: no catalog changes) and stop. 4b (the filter panel) gets its own plan.
