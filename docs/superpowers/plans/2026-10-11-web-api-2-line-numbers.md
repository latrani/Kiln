# Web API, step 2: line numbers — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every scrollback line and every log-mode line has a number that follows its place: new lines count up from 0, older ones down from -1, and no number is used twice. A rules change keeps each line's number.

**Architecture:**
- `app.Line` gains `Seq int`. `LogLine` embeds `Line`, so log-mode lines have it too.
- A small `numbering` (`lo`, `hi`) hands out numbers: `newer()` returns `hi` and bumps it; `older()` lowers `lo` and returns it. `Char` and `Log` each hold one. Numbers are given on the loop, when a line enters `Lines`, never when it's made (pages are made off the loop).
- **Scrollback** (`Char.Lines`): `add` (live lines and echoes) takes `newer()`. `Preload` and `HandleOlder` number their lines with `older()`, newest first, so a page sits below everything already numbered. A second `Preload` (the TUI's tests, and any future reload) numbers below the first; numbers stay unique per character for as long as it's open.
- **Log mode** (`Log.Lines`): `prepend` (the initial load and every older day) numbers with `older()`; `appendLive` with `newer()`. Each `OpenLog` starts a fresh `Log`, so a fresh numbering.
- **Rules changes:** `Rules.Reline` keeps `Seq`, so `ApplyConfig`'s re-tagging, and a page read under old rules, keep their numbers.

Nothing reads `Seq` yet: the change feed (step 3) and the web (step 4) will. The TUI doesn't change.

**Tech Stack:** Go; `internal/app`.

**Spec:** `docs/superpowers/specs/2026-10-11-web-api-design.md`, "Line numbers".

## Global Constraints

- Nothing visible changes; golden screens are NOT updated. No strings change.
- Run `go vet ./... && go test ./...` and the web module's tests (`cd web && go test ./... && GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...`) before committing. No `-race`.
- End the commit message with the session's attribution lines.

## Where lines enter `Lines` (each must number them)

- `Char.add` (`scrollback.go`): `Handle`'s shown session lines and `Echo`.
- `Preload` (`scrollback.go`): replaces `Lines` with the history tail plus a `HistoryEnd` marker. Day dividers and the marker are lines and get numbers.
- `HandleOlder` (`scrollback.go`): prepends a page (relined first if the rules changed).
- `Log.prepend` (`log.go`): `OpenLog`'s synchronous `loadOlder` and `HandleLogOlder`.
- `Log.appendLive` (`log.go`): live lines while log mode is open; a dropped duplicate gets no number.
- Not entering: an unshown session line (`Handle`'s `ev.Line` when `!ev.Shown`) keeps `Seq` 0 and is never in `Lines`.

## Review Focus

1. **No reuse after a second `Preload`.** The new lines must number below the old lowest, and a live line after it must number above everything. `TestScrollbackNumbersFollowTheLines` pins it.
2. **Order equals number order.** In `Char.Lines` and `Log.Lines`, `Seq` strictly increases from first to last after every kind of change, with no gaps inside one load. Tests check it after preload, live lines, echoes, older pages, and log mode's initial load, older day and live line.
3. **Re-tagging keeps numbers.** `ApplyConfig` with changed rules, and an older page read under old rules, keep `Seq`. `TestOlderPageAfterARulesChange` and `TestLogRelinesAPageReadUnderOldRules` gain checks.
4. **Numbers are given on the loop.** No `numbering` is touched inside a `Run` func (`RequestOlder`, `Log.Older`); the reviewer should check the closures.
5. **Day dividers and the history-end marker** are numbered like any line and keep their numbers through `Reline` (which returns them unchanged).

---

### Task 1: lines are numbered

**Files:**
- Modify: `internal/app/line.go`, `internal/app/scrollback.go`, `internal/app/log.go`, `internal/app/app.go` (the `Char.seq` field)
- Test: `internal/app/scrollback_test.go`, `internal/app/log_test.go`, `internal/app/line_test.go`

**Interfaces:**
- Produces: `Line.Seq int`; `type numbering struct{ lo, hi int }` with `newer() int`, `older() int`; `Char.seq numbering`; `Log.seq numbering`.

- [ ] **Step 1: Write the tests**

In `internal/app/scrollback_test.go`, add:

```go
// seqs is ls's numbers, in order.
func seqs(ls []*Line) []int {
	var out []int
	for _, l := range ls {
		out = append(out, l.Seq)
	}
	return out
}

// runOf reports whether ns counts up by one.
func runOf(ns []int) bool {
	for i := 1; i < len(ns); i++ {
		if ns[i] != ns[i-1]+1 {
			return false
		}
	}
	return true
}

func TestScrollbackNumbersFollowTheLines(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	writeLog(t, a, day23, "one", "two")
	openAll(t, a, "fm/kit")
	kit := a.Char("fm/kit")
	if got := seqs(kit.Lines); !slices.Equal(got, []int{-4, -3, -2, -1}) { // day, one, two, end of history
		t.Fatalf("preloaded %v", got)
	}
	s := attach(a, "fm/kit")
	a.Handle(lineMsg("fm/kit", s, "Welcome!"))
	a.Echo("fm/kit", logstore.Entry{Dir: logstore.Out, Text: "hi"}) // local_echo off: not a line
	kit.Ch.LocalEcho = true
	a.Echo("fm/kit", logstore.Entry{Dir: logstore.Out, Text: "hi"})
	if got := seqs(kit.Lines); !slices.Equal(got, []int{-4, -3, -2, -1, 0, 1}) {
		t.Errorf("after a line and an echo: %v", got)
	}
	a.Preload("fm/kit") // again: below everything so far
	got := seqs(kit.Lines)
	if !runOf(got) || got[len(got)-1] != -5 {
		t.Errorf("preloaded again: %v; want a run ending at -5", got)
	}
	a.Handle(lineMsg("fm/kit", s, "Again"))
	if l := kit.Lines[len(kit.Lines)-1]; l.Seq != 2 {
		t.Errorf("a live line after: %d; want 2", l.Seq)
	}
}
```

In `TestPagingOlderHistory`, after the `HandleOlder` check, add:

```go
	if got := seqs(kit.Lines); !runOf(got) || got[len(got)-1] != -1 {
		t.Errorf("after an older page: %v; want one run ending at -1", got)
	}
```

In `TestOlderPageAfterARulesChange`, record the numbers before the reload and check them after — right after `openAll(t, a, "fm/kit")` add `before := seqs(a.Char("fm/kit").Lines)`, and at the end add:

```go
	if got := seqs(a.Char("fm/kit").Lines); !slices.Equal(got[len(got)-len(before):], before) || !runOf(got) {
		t.Errorf("numbers after the reload and the page: %v; the preloaded %v should keep theirs", got, before)
	}
```

In `internal/app/log_test.go`, add:

```go
// logSeqs is g's line numbers, in order.
func logSeqs(g *Log) []int {
	var out []int
	for _, l := range g.Lines {
		out = append(out, l.Seq)
	}
	return out
}

func TestLogNumbersFollowTheLines(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	if got := logSeqs(g); !runOf(got) || got[len(got)-1] != -1 {
		t.Fatalf("loaded %d lines numbered %v…; want one run ending at -1", len(got), got[:3])
	}
	lowest := g.Lines[0].Seq
	drain(a, g.Older(nil))
	if got := logSeqs(g); !runOf(got) || g.Lines[0].Seq >= lowest {
		t.Errorf("after an older day: lowest %d (was %d), run %v", g.Lines[0].Seq, lowest, runOf(got))
	}
	g.appendLive(logstore.Entry{Time: time.Now(), Dir: logstore.In, Text: "live"})
	if l := g.Last(); l.Seq != 0 {
		t.Errorf("a live line: %d; want 0", l.Seq)
	}
}
```

In `TestLogRelinesAPageReadUnderOldRules`, add a check that the relined lines keep their numbers: read it first (`sed -n 95,110p internal/app/log_test.go`), record `logSeqs(g)` after the page arrives under the new rules… the page is numbered on arrival, so instead check `runOf(logSeqs(g))` at the end.

In `internal/app/line_test.go`, in the Reline test (the one that builds `old` and a `day` line), add:

```go
	l.Seq = 7
	if got := old.Reline(l); got.Seq != 7 {
		t.Errorf("Reline lost the line's number: %d", got.Seq)
	}
```

placed where `l` is a server line in scope (adapt the variable name to the test).

Add `"github.com/latrani/Kiln/internal/logstore"` and `"time"` to `log_test.go`'s imports if they're missing.

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/app -run 'Numbers|Paging|RulesChange|Reline'`
Expected: build failure (`Seq` undefined).

- [ ] **Step 3: Implement**

In `internal/app/line.go`, add to `Line` (first field after `Kind`):

```go
	Seq   int            // its number in its scrollback or log mode; see numbering
```

and, after the `Line` methods:

```go
// numbering hands out line numbers: new lines count up from 0, older
// ones down from -1, so numbers follow the lines' order and none is used
// twice. A front end names a line by its number ("everything after 41",
// "lines 3 to 9").
type numbering struct{ lo, hi int } // lo: the lowest given; hi: the next new one

// newer is the number for a line added after every other.
func (n *numbering) newer() int { s := n.hi; n.hi++; return s }

// older is the number for a line added before every other.
func (n *numbering) older() int { n.lo--; return n.lo }

// numberOlder numbers ls, oldest first, to go before every line n has
// numbered.
func (n *numbering) numberOlder(ls []*Line) {
	for i := len(ls) - 1; i >= 0; i-- {
		ls[i].Seq = n.older()
	}
}
```

`Reline` keeps the number:

```go
func (r Rules) Reline(l Line) Line {
	if l.Kind == Day || l.Kind == HistoryEnd {
		return l
	}
	out := r.Line(l.Entry)
	out.Seq = l.Seq
	return out
}
```

(and its doc comment gains: `The line keeps its number.`)

In `internal/app/app.go`, add to `Char` (after `Lines`):

```go
	seq         numbering       // Lines' numbers
```

In `internal/app/scrollback.go`:
- `Preload`: after the two lines that build `c.Lines`, add `c.seq.numberOlder(c.Lines)`.
- `HandleOlder`: before `c.Lines = append(msg.lines, c.Lines...)`, add `c.seq.numberOlder(msg.lines)`.
- `add`: `l.Seq = c.seq.newer()` before appending.

In `internal/app/log.go`:
- `Log` gains `seq numbering // Lines' numbers` among its unexported fields.
- `prepend`: before `g.Lines = append(msg.Lines, g.Lines...)`, number the page:

```go
	for i := len(msg.Lines) - 1; i >= 0; i-- {
		msg.Lines[i].Seq = g.seq.older()
	}
```

- `appendLive`: make the line, number it, append:

```go
	l := makeLogLine(g.c.Rules, e)
	l.Seq = g.seq.newer()
	g.Lines = append(g.Lines, l)
```

In `HandleLogOlder`, the reline loop (`l.Line = c.Rules.Reline(l.Line)`) runs before `prepend` numbers the page, so it needs nothing.

- [ ] **Step 4: Run everything** (Global Constraints). Expected: PASS, golden screens untouched.

- [ ] **Step 5: Commit**

```bash
git add internal/app docs/superpowers/plans/2026-10-11-web-api-2-line-numbers.md
git commit -m "app: scrollback and log-mode lines are numbered

New lines count up from 0, older pages down from -1; a rules change
keeps each line's number. For the web's change feed."
```

---

### Task 2: review and PR

- [ ] **Step 1: Fresh review** of the branch against `main` with this plan's Review Focus. Fix Critical and Important findings with a failing test first; log minors.
- [ ] **Step 2: Open the PR** "Web API step 2: lines are numbered", linking the spec, saying nothing visible changes and no strings change, and noting for step 3 that `Preload` replaces all of a character's lines (the feed must send a replacement, not an append). Then stop.
