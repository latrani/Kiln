# Live range marking Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `m` starts a range whose far end follows the log-mode cursor (keys, clicks, wheel) until `m`, `c` or `e` locks it or `Esc` drops it, in the TUI and the web.

**Architecture:** The core (`app.Log`) holds the anchor and a `Marking` flag and offers `Mark`, `MarkTo`, `EndMark`. The cursor stays each front end's; each has one choke point where the cursor changes (TUI `browse.SetCursor`, web `Store.setLogView`) that calls `MarkTo` while marking.

**Tech Stack:** Go (bubbletea v2), TypeScript/Preact web page, the string catalog (`internal/str`).

**Spec:** `docs/superpowers/specs/2026-10-15-live-range-design.md`

## Global Constraints

- Every user-facing string lives in `internal/str/locales/en.toml`; run `go generate ./internal/str` after editing it (regenerates Go and `web/ui/src/str.gen.ts`).
- Tests build expected text from the catalog (`str.BrowseMarking()`), never a copy.
- Commits that touch `en.toml` end with a `Strings:` trailer listing the keys.
- Names in tests: Kit, Rook, Ash; world `fm`.
- Try Kiln only against a scratch `XDG_CONFIG_HOME`/`XDG_DATA_HOME`.
- Below 1.0: no compatibility shims for the old pending-start behavior.
- Web copies the TUI; differences only where the browser forces them.

## Review Focus

1. **Async cursor moves:** `PgUp` past the oldest loaded line moves the cursor again when the older day arrives (`Older` continuation). The range must follow that second move too. Pinned in Task 2.
2. **Live lines arriving while marking at the bottom:** the cursor follows new lines (`model.go` `follow`), so the range grows with them. That's acceptable but must not panic or desync; pinned in Task 2.
3. **Filter hides the anchor or the cursor's line while marking:** `Refilter` moves the cursor; the range follows it, and the anchor (hidden) stays an end. `Selection` already drops hidden lines. Pinned in Task 1 (`MarkTo` with a hidden anchor) and Task 2.
4. **Closing log mode with Ctrl+L while marking, then reopening:** marking survives (log mode is kept as left), and the next cursor move keeps extending. Pinned in Task 2.
5. **Web drag-select while marking:** a drag sends `range`, which must lock (turn `Marking` off) rather than leave a live range with a stale anchor. Pinned in Task 3.

---

### Task 1: Core live range and strings

**Files:**
- Modify: `internal/app/log.go` (struct `Log` ~line 38; `InRange` ~256; `Mark`/`NewRange`/`RangeStatus`/`ExtendTo`/`ToggleExclude`/`Selection` ~323-392; `Copy` ~407; `ExportReady` ~419)
- Modify: `internal/str/locales/en.toml` (`[browse]` ~line 259)
- Test: `internal/app/log_test.go`

**Interfaces:**
- Produces:
  - `Log.Marking bool` (exported field; front ends read it)
  - `func (g *Log) Mark(l *LogLine)`: no live range → anchor/Start/End = `l`, exclusions cleared, `Marking` on, status `str.BrowseRangeStart()`. Live range → `MarkTo(l)` (if `l` non-nil), then `Marking` off, `RangeStatus()`. Nil `l` with no live range: nothing.
  - `func (g *Log) MarkTo(l *LogLine)`: while `Marking` and `l` non-nil: Start/End = anchor and `l` in order; exclusions outside them deleted. Otherwise nothing.
  - `func (g *Log) EndMark()`: Start, End nil, exclusions cleared, `Marking` off, anchor nil.
  - `str.BrowseMarking() string`

Note: the spec said `Mark` with a live range ignores `l`; this plan has it extend to `l` first, then lock. Front ends pass the cursor, so it's the same thing, and old two-call `Mark(a); Mark(b)` callers (tests, the web `mark` action) keep meaning "range a..b". Update the spec's Core bullet to match in this task.

- [ ] **Step 1: Strings.** In `en.toml` `[browse]`, change and add:

```toml
range_start = "marking: move to the other end, m to finish, Esc to drop"
# The statusline's part while a range is live (m, then moving).
marking = "marking"
```

Run: `go generate ./internal/str`

- [ ] **Step 2: Failing tests** in `internal/app/log_test.go`:

```go
func TestMarkFollowsAndLocks(t *testing.T) {
	a := logApp(t, 6)
	g := a.OpenLog("fm/kit")
	g.Mark(g.Lines[3])
	if !g.Marking || g.Start != g.Lines[3] || g.End != g.Lines[3] || g.Status != str.BrowseRangeStart() {
		t.Fatalf("m: marking %v, %v–%v, status %q", g.Marking, g.Start, g.End, g.Status)
	}
	g.MarkTo(g.Lines[5])
	if g.Index(g.Start) != 3 || g.Index(g.End) != 5 {
		t.Errorf("below the anchor: %d–%d", g.Index(g.Start), g.Index(g.End))
	}
	g.MarkTo(g.Lines[1])
	if g.Index(g.Start) != 1 || g.Index(g.End) != 3 {
		t.Errorf("above the anchor: %d–%d; the anchor stays an end", g.Index(g.Start), g.Index(g.End))
	}
	g.Mark(g.Lines[0])
	if g.Marking || g.Index(g.Start) != 0 || g.Index(g.End) != 3 || g.Status != str.BrowseLinesInRange(4) {
		t.Errorf("m again: marking %v, %d–%d, status %q", g.Marking, g.Index(g.Start), g.Index(g.End), g.Status)
	}
	g.MarkTo(g.Lines[5])
	if g.Index(g.End) != 3 {
		t.Error("a locked range moved")
	}
}

func TestMarkToForgetsExclusionsItLeaves(t *testing.T) {
	a := logApp(t, 6)
	g := a.OpenLog("fm/kit")
	g.Mark(g.Lines[0])
	g.MarkTo(g.Lines[4])
	g.ToggleExclude(g.Lines[3])
	g.ToggleExclude(g.Lines[1])
	g.MarkTo(g.Lines[2])
	g.MarkTo(g.Lines[4])
	if g.Excluded[g.Lines[3]] || !g.Excluded[g.Lines[1]] {
		t.Errorf("excluded 3 %v (left behind: want false), 1 %v (still inside: want true)", g.Excluded[g.Lines[3]], g.Excluded[g.Lines[1]])
	}
}

func TestEndMarkDropsTheRange(t *testing.T) {
	a := logApp(t, 3)
	g := a.OpenLog("fm/kit")
	g.Mark(g.Lines[0])
	g.MarkTo(g.Lines[2])
	g.EndMark()
	if g.Marking || g.Start != nil || g.End != nil || len(g.Selection()) != 0 {
		t.Errorf("marking %v, %v–%v", g.Marking, g.Start, g.End)
	}
}

func TestCopyExportAndOtherRangesLock(t *testing.T) {
	a := logApp(t, 4)
	g := a.OpenLog("fm/kit")
	for name, lock := range map[string]func(){
		"copy":     func() { g.Copy() },
		"export":   func() { g.ExportReady() },
		"newrange": func() { g.NewRange(g.Lines[0], g.Lines[1]) },
		"extend":   func() { g.ExtendTo(g.Lines[3]) },
	} {
		g.Mark(g.Lines[1])
		g.MarkTo(g.Lines[2])
		lock()
		if g.Marking {
			t.Errorf("%s left the range live", name)
		}
		g.EndMark()
	}
}

func TestMarkToKeepsAHiddenAnchor(t *testing.T) {
	a := logApp(t, 4)
	g := a.OpenLog("fm/kit")
	g.Mark(g.Lines[0])
	g.c.Filter.SetText(g.Lines[0].Lower, true) // hide the anchor's line; see Step 3 note
	g.Refilter()
	g.MarkTo(g.Lines[3])
	if g.Start != g.Lines[0] || g.End != g.Lines[3] || len(g.Selection()) != 3 {
		t.Errorf("%d–%d, %d selected", g.Index(g.Start), g.Index(g.End), len(g.Selection()))
	}
}
```

For `TestMarkToKeepsAHiddenAnchor`, use whatever hide call the filter tests in `internal/app` use (grep `Hide\|AddText` in `internal/app/*_test.go`); the point is: the anchor's line is hidden, the range still runs from it, and `Selection` skips it.

Run: `go test ./internal/app -run 'Mark|Copy' -v`
Expected: FAIL (no `Marking`, `MarkTo`, `EndMark`).

- [ ] **Step 3: Implement** in `internal/app/log.go`:

```go
// in Log:
	Start, End *LogLine   // the marked range, both set or both nil
	Marking    bool       // the range is live: its far end follows the cursor; see Mark
	Excluded   map[*LogLine]bool
// unexported, beside the others:
	anchor *LogLine // the line m was pressed on, while Marking
```

```go
// InRange reports whether l is inside the marked range.
func (g *Log) InRange(l *LogLine) bool {
	if g.Start == nil {
		return false
	}
	i := g.Index(l)
	return i >= g.Index(g.Start) && i <= g.Index(g.End)
}

// Mark is m on l, the cursor's line. With no live range it starts one
// there: l is its anchor, and MarkTo moves its other end. With one, it
// takes the range to l and locks it. A nil l starts nothing.
func (g *Log) Mark(l *LogLine) {
	if g.Marking {
		g.MarkTo(l)
		g.Marking, g.anchor = false, nil
		g.RangeStatus()
		return
	}
	if l == nil {
		return
	}
	g.NewRange(l, l)
	g.Marking, g.anchor = true, l
	g.SetStatus(false, str.BrowseRangeStart())
}

// MarkTo moves a live range's far end to l, the cursor's line: the range
// runs from the anchor to l, either way round. Exclusions it no longer
// covers are forgotten, so growing back over them doesn't bring them
// back. Nothing when the range isn't live.
func (g *Log) MarkTo(l *LogLine) {
	if !g.Marking || l == nil {
		return
	}
	g.Start, g.End = g.anchor, l
	if g.Index(l) < g.Index(g.anchor) {
		g.Start, g.End = l, g.anchor
	}
	for x := range g.Excluded {
		if !g.InRange(x) {
			delete(g.Excluded, x)
		}
	}
}

// EndMark drops a live range: Esc while marking.
func (g *Log) EndMark() {
	g.Start, g.End, g.Marking, g.anchor = nil, nil, false, nil
	clear(g.Excluded)
}

// NewRange starts a range from start to end, either way round, dropping
// exclusions left from an earlier one so they can't resurface if it
// grows over them. A live range locks: this is a range of its own.
func (g *Log) NewRange(start, end *LogLine) {
	if g.Index(end) < g.Index(start) {
		start, end = end, start
	}
	g.Start, g.End, g.Marking, g.anchor = start, end, false, nil
	clear(g.Excluded)
}
```

`Mark` sets `Marking` after `NewRange` (which clears it). `ExtendTo`: drop the `if g.End == nil` block, add `g.Marking, g.anchor = false, nil` first. `ToggleExclude`: condition becomes `l == nil || !g.InRange(l)`. `Selection`: condition becomes `g.Start == nil`. `Copy` and `ExportReady`: first line `g.Marking, g.anchor = false, nil` (a helper `func (g *Log) lock()` is fine; use it in `ExtendTo` too). Callers of `NewRange(l, nil)` (`internal/ui/browse.go:451`): change to `NewRange(b.Cursor, b.Cursor)`.

Fix the doc comment on `Start, End` and any other comment mentioning "only a start marked".

- [ ] **Step 4: Run** `go test ./internal/app ./internal/feed ./internal/webapi ./internal/ui ./internal/str`
Expected: PASS. Existing `Mark(a); Mark(b)` tests still pass (second `Mark` extends to `b`, then locks).

- [ ] **Step 5: Update the spec's Core bullet** for `Mark` with a live range ("takes the range to `l`, then locks"). Commit:

```bash
git add internal/app internal/str internal/ui/browse.go web/ui/src/str.gen.ts docs/superpowers/specs/2026-10-15-live-range-design.md
git commit -m "Log mode: m starts a live range; the core's Mark, MarkTo and EndMark

Strings: browse.range_start, browse.marking (new)"
```

---

### Task 2: TUI

**Files:**
- Modify: `internal/ui/logcursor.go` (every `b.Cursor =` becomes `b.SetCursor(...)`; `SetCursor` calls `MarkTo`)
- Modify: `internal/ui/model.go:594` and `:935` (`Cursor =` → `SetCursor`)
- Modify: `internal/ui/browse.go` (`key`'s `actBack` ~line 101; `statusParts` ~154; `click` ~434)
- Modify: `docs/log-mode.md`
- Test: `internal/ui/browse_test.go`

**Interfaces:**
- Consumes: `Log.Marking`, `Log.Mark`, `Log.MarkTo`, `Log.EndMark`, `str.BrowseMarking()` from Task 1.

- [ ] **Step 1: Failing tests** in `internal/ui/browse_test.go`. Use `scene1`/`echoWorld` and the `click` helper shape from `TestBrowseMouseSelectsLikeFinder` (copy its `click` closure into each test that clicks).

```go
// m starts a range that follows the cursor; m again locks it.
func TestBrowseLiveRangeKeys(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": echoWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	b := h.br()
	last := len(b.Lines) - 1
	h.key("m")
	h.keys("up", "up")
	if !b.Marking || b.Index(b.Start) != last-2 || b.Index(b.End) != last {
		t.Fatalf("live: marking %v, %d–%d", b.Marking, b.Index(b.Start), b.Index(b.End))
	}
	if bar := ansi.Strip(h.m.statusLine(80)); !strings.Contains(bar, str.BrowseMarking()) || !strings.Contains(bar, str.ViewSelected(len(b.Selection()))) {
		t.Errorf("statusline %q: want marking and the live count", bar)
	}
	h.key("m")
	h.key("up")
	if b.Marking || b.Index(b.Start) != last-2 {
		t.Errorf("locked range moved: marking %v, start %d", b.Marking, b.Index(b.Start))
	}
}

// Paging stretches it, including a page that waits for an older day.
func TestBrowseLiveRangePagesOlder(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	for d := 21; d <= 24; d++ { // four days; log mode opens with two
		lines := make([]string, 150)
		for i := range lines {
			lines[i] = fmt.Sprintf("day %d line %d", d, i)
		}
		h.writeLog(time.Date(2026, 9, d, 8, 0, 0, 0, time.Local), lines...)
	}
	h.key("ctrl+l")
	b := h.br()
	end := b.Cursor
	h.key("m")
	h.key("home") // ToTop: moves again as each older day arrives
	if !b.HistDone || b.Start != b.Lines[0] || b.End != end {
		t.Errorf("range %q–%q; want the oldest line to the anchor", b.Start.Entry.Text, b.End.Entry.Text)
	}
}

// Click, m, click: the range runs between; shift and gutter clicks
// only move the end while marking.
func TestBrowseLiveRangeClicks(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": echoWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.screen()
	b := h.br()
	x := h.m.layout().sw + 1 + 10
	click := func(i int, shift bool) {
		t.Helper()
		var mod tea.KeyMod
		if shift {
			mod = tea.ModShift
		}
		for row, bl := range b.rowLines {
			if bl == b.Lines[i] {
				h.m.Update(tea.MouseClickMsg{X: x, Y: row + 2, Button: tea.MouseLeft, Mod: mod})
				h.screen()
				return
			}
		}
		t.Fatalf("line %d not on screen", i)
	}
	click(1, false)
	h.key("m")
	click(4, false)
	if !b.Marking || b.Index(b.Start) != 1 || b.Index(b.End) != 4 {
		t.Fatalf("click while marking: marking %v, %d–%d", b.Marking, b.Index(b.Start), b.Index(b.End))
	}
	click(3, true)
	if b.Excluded[b.Lines[3]] || b.Index(b.End) != 3 {
		t.Errorf("shift-click while marking: excluded %v, end %d; want just the end moved", b.Excluded[b.Lines[3]], b.Index(b.End))
	}
	h.key("m")
	click(0, false)
	if b.Index(b.Start) != 0 || b.Index(b.End) != 0 {
		t.Error("a click after locking should start a new one-line range")
	}
}

// Esc ends marking first, then find, then log mode.
func TestBrowseEscEndsMarkingFirst(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": echoWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	b := h.br()
	h.keys("/", "R", "enter") // find on (use typeText if keys doesn't type)
	h.keys("m", "up")
	h.key("esc")
	if b.Marking || b.Start != nil || b.Find == "" || h.m.cur().browse == nil {
		t.Fatalf("esc: marking %v, start %v, find %q", b.Marking, b.Start, b.Find)
	}
	h.key("esc")
	if b.Find != "" || h.m.cur().browse == nil {
		t.Error("second esc should end find and stay in log mode")
	}
}

// Ctrl+L away and back keeps a live range live.
func TestBrowseLiveRangeSurvivesToggle(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": echoWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.keys("m", "up")
	h.key("ctrl+l")
	h.key("ctrl+l")
	b := h.br()
	h.key("up")
	if !b.Marking || b.Index(b.End)-b.Index(b.Start) != 2 {
		t.Errorf("after toggle: marking %v, %d–%d", b.Marking, b.Index(b.Start), b.Index(b.End))
	}
}

// A live line arriving while the cursor follows the bottom grows the range.
func TestBrowseLiveRangeFollowsLiveLines(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.key("ctrl+l")
	b := h.br()
	h.key("m")
	anchor := b.Start
	h.conn("fm/kit").Feed("Brand new line")
	h.settle("fm/kit", func() bool { return strings.Contains(h.screen(), "Brand new line") })
	if b.Start != anchor || b.End.Entry.Text != "Brand new line" {
		t.Errorf("range %v–%q; want the anchor to the new line", b.Start, b.End.Entry.Text)
	}
}
```

For find in `TestBrowseEscEndsMarkingFirst`, type the term the way `TestBrowseFind` does (`h.key("/")`, `h.typeText("Rook")`, `h.key("enter")`) if `h.keys` doesn't type text.

Run: `go test ./internal/ui -run 'LiveRange|EscEndsMarking' -v`
Expected: FAIL.

- [ ] **Step 2: One cursor choke point.** In `logcursor.go`:

```go
// SetCursor puts the cursor on l: a key, a click, find, or the view
// dragging it along. Every cursor move comes through here, so a live
// range's end follows it.
func (b *browse) SetCursor(l *app.LogLine) {
	b.Cursor = l
	b.MarkTo(l)
}
```

Replace every `b.Cursor = x` in `logcursor.go` with `b.SetCursor(x)` (lines 32, 42, 48, 59, 65, 80; `GotoTime`'s closure becomes `b.SetCursor(l); b.top = l`). In `model.go:594` use `f.b.SetCursor(f.b.LastVisible())`; `model.go:935` may stay a plain assignment (a new log can't be marking) but use `SetCursor` for uniformity. Then confirm nothing else assigns it:

Run: `grep -n "\.Cursor = " internal/ui/*.go | grep -v _test`
Expected: only the line inside `SetCursor`.

- [ ] **Step 3: Click, Esc, statusline** in `browse.go`:

```go
// click: first case of the switch
	case b.Marking: // the live range's end follows; nothing else a click does
		b.SetCursor(l)
```

and in the `shift` case, `b.NewRange(b.Cursor, nil)` becomes `b.NewRange(b.Cursor, b.Cursor)`. Update the Finder comment to say a click while marking moves the end.

```go
	case actBack: // marking, then find, as Esc backs out of one thing at a time
		if b.Marking {
			b.EndMark()
			return nil, false
		}
		if b.Find != "" {
```

```go
// statusParts: marking first
	var parts []string
	if b.Marking {
		parts = append(parts, str.BrowseMarking())
	}
	if f := b.FindStatus(); f != "" {
```

Update `statusParts`'s doc comment to name marking.

- [ ] **Step 4: Run** `go test ./internal/ui` (goldens included)
Expected: PASS, goldens unchanged.

- [ ] **Step 5: Docs.** In `docs/log-mode.md`, replace the `m` and click rows and amend the Esc wording:

```markdown
| `m` | Start a range on the current line: its other end follows the cursor as you move (keys, paging, clicks) until `m` again locks it. `c` and `e` lock it too; `Esc` drops it |
| `Space` | Leave the current line out of the range |
| Click, shift-click | Select a line; shift-click outside the range to extend it, or inside it to leave a line out (or bring it back). While marking, any click just moves the range's end |
```

In the statusline paragraph, add "`marking` while a range is live," before find's count. In the `/` row's Esc sentence nothing changes; add to the intro paragraph's Esc sentence: "`Esc` while marking drops the range first."

- [ ] **Step 6: Try it.** Build and run against a scratch config:

```bash
go build -o /tmp/kiln-191 ./cmd/kiln
```

Hand Indi the build (or `~/Downloads` per the remote setup memory) and stop for feedback before Task 3.

- [ ] **Step 7: Commit**

```bash
git add internal/ui docs/log-mode.md
git commit -m "Log mode: a live range follows the cursor, clicks included (#191)"
```

---

### Task 3: Web

**Files:**
- Modify: `internal/feed/events.go:133` (`Log` gains `Marking bool`), `internal/feed/feed.go:338` (`logState` sets it)
- Modify: `internal/webapi/actions.go:301-330` (`mark_to`, `grow`, `end_mark`)
- Modify: `web/ui/src/log.ts` (`LogState.Marking`, `LogKey` `"mark"`, `letters.m`, `logAct` `mark` and `back`, `logClick`)
- Modify: `web/ui/src/store.ts:482` (`setLogView` sends `mark_to`)
- Modify: `web/ui/src/ui/StatusLine.tsx:20` (`logStatus` marking part)
- Modify: `internal/str/locales/en.toml` `[web] log_hints`
- Test: `internal/webapi/loop_test.go`, `web/ui/src/log.test.ts`, `web/ui/src/ui/LogView.test.tsx`, `web/smoke/log_test.go`

**Interfaces:**
- Consumes: Task 1's core API.
- Produces: actions `{"Do":"mark_to","Key":k,"Seq":n}`, `{"Do":"grow","Key":k,"Seq":n}` and `{"Do":"end_mark","Key":k}`; feed `Log.Marking`.

**Renamed after Task 2:** the mode is *selecting* on `s`, not marking on `m`. Wherever this task says `m`, read `s`: `letters` gets `s: "mark"` (the `LogKey` and actions keep the core's names), the letters test maps `"s"` → `"mark"` and checks `k("m") === undefined`, the statusline part is `str.browseSelecting()`, and `web.log_hints` gains `s select`. The smoke test presses `s`.

**Changed after Task 2 (Indi tried it):** a click while marking is a sticky shift-click. Inside the range: `exclude`, cursor unchanged. Outside: `grow` (core `GrowTo`), then the cursor moves to the line (which sends `mark_to`, a no-op after `grow`). This replaces the "a click while marking only moves the end" steps below.

**Also changed after Task 2:** while marking, the wheel no longer drags the cursor (TUI: `scrollToCursor` keeps the view and `scrolled` until `SetCursor`). On the web, `LogView.onScroll` skips `dragCursor` while `g.Marking`; test it in `LogView.test.tsx` or `log.test.ts` (a scroll while marking sends no `mark_to`). The web `log_hints` and docs say paging doesn't stretch the range there. The web draws the ▲/▼ too (glyph.CursorAbove/Below, role `log.cursor.away`), by the time of the first/last line in view while the cursor is out of it.

- [ ] **Step 1: Go side, failing test** in `internal/webapi/loop_test.go`, after the existing `mark`/`mark` pair (~line 397), make it a live range and check it:

```go
	h.act(fmt.Sprintf(`{"Do":"mark","Key":"fm/kit","Seq":%d}`, seqs[0]))
	b = h.act(fmt.Sprintf(`{"Do":"mark_to","Key":"fm/kit","Seq":%d}`, seqs[2]))
	if lg := event(b, "Log"); lg == nil || lg["Marking"] != true || lg["End"] != float64(seqs[2]) {
		t.Errorf("mark_to: %v", lg)
	}
	b = h.act(`{"Do":"end_mark","Key":"fm/kit"}`)
	if lg := event(b, "Log"); lg == nil || lg["Marking"] != false || lg["Start"] != nil {
		t.Errorf("end_mark: %v", lg)
	}
```

Place it before the existing range checks so they still start from a known state (re-mark after). Run `go test ./internal/webapi -run TestLog -v`; expect FAIL.

- [ ] **Step 2: Implement Go side.** `feed.Log`: add `Marking bool // m's range is live: its far end follows the cursor`. `logState`: `Marking: g.Marking`. `actions.go`: add `"mark_to", "end_mark"` to the first case list (with `//str:ok`), and:

```go
	case "mark_to": //str:ok
		g.MarkTo(at(act.Seq))
	case "grow": //str:ok
		g.GrowTo(at(act.Seq))
	case "end_mark": //str:ok
		g.EndMark()
```

Run `go test ./internal/webapi ./internal/feed`; expect PASS.

- [ ] **Step 3: Web, failing tests.** In `log.test.ts`, change the letters test: add `"m"` → `"mark"` to the mapped list, drop the `k("m") === undefined` assertion. In `LogView.test.tsx` add:

```tsx
test("m marks at the cursor; while marking, moving the cursor sends mark_to; Esc ends marking before find", () => {
  const sent: any[] = [];
  const s = new Store();
  s.set({ phase: "running", act: (o: object) => sent.push(o) });
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" },
    { type: "Log", Key: "fm/kit", Open: true, Find: "x" },
    { type: "LogLines", Key: "fm/kit", Epoch: 1, Op: "replace", Lines: [/* two lines, Seq 1 and 2, shaped as this file's other LogLines fixtures */] }]);
  s.setLogView("fm/kit", { cursor: 2 });
  logAct(s, "mark");
  assert.deepEqual(sent.at(-1), { Do: "mark", Key: "fm/kit", Seq: 2 });
  s.apply([{ type: "Log", Key: "fm/kit", Open: true, Find: "x", Marking: true, Start: 2, End: 2 }]);
  logAct(s, "up");
  assert.deepEqual(sent.at(-1), { Do: "mark_to", Key: "fm/kit", Seq: 1 });
  logClick(s, 2, {});
  assert.deepEqual(sent.at(-1), { Do: "mark_to", Key: "fm/kit", Seq: 2 }, "a click while marking only moves the end");
  logClick(s, 1, { shift: true, inRange: true });
  assert.deepEqual(sent.at(-1), { Do: "mark_to", Key: "fm/kit", Seq: 1 }, "shift-click too");
  logAct(s, "back");
  assert.deepEqual(sent.at(-1), { Do: "end_mark", Key: "fm/kit" });
});

test("the status line says marking first while a range is live", () => {
  // pageWith(...) as the other statusline tests in this file, with
  // Marking: true; assert the status text starts with str.browseMarking().
});
```

Copy the `LogLines` fixture shape and `pageWith` usage from existing tests in the file. Run `cd web/ui && npm test`; expect FAIL.

- [ ] **Step 4: Implement web side.**

`log.ts`:
```ts
// LogState: add
  Marking?: boolean;
// LogKey: add "mark"
// letters: add m: "mark"
// logAct:
    case "mark": act({ Do: "mark", Key: k, Seq: here }); break;
    case "back": // one thing at a time: a search, the panel, marking, find, then log mode
      if (g.Searching) act({ Do: "stop_search", Key: k });
      else if (v.panel) closePanel(store);
      else if (g.Marking) act({ Do: "end_mark", Key: k });
      else if (g.Find) act({ Do: "end_find", Key: k });
      else act({ Do: "log_close", Key: k });
      break;
```

Update `logKey`'s comment: `m` is the TUI's; Space stays the mouse's (gutter or shift-click). Update `logClick`:

```ts
  if (s.logView[k]?.prompt) return;
  if (g.Marking) { // a sticky shift-click, as the TUI's
    if (how.inRange) act({ Do: "exclude", Key: k, Seq: seq });
    else { act({ Do: "grow", Key: k, Seq: seq }); store.setLogView(k, { cursor: seq }); }
    return;
  }
```

`store.ts` `setLogView`:

```ts
  // setLogView changes k's log view. A cursor move while a range is live
  // moves its end (the TUI's SetCursor).
  setLogView(k: string, patch: Partial<LogView>): void {
    const was = this.state.logView[k]?.cursor;
    this.set({ logView: { ...this.state.logView, [k]: { ...(this.state.logView[k] ?? { goN: 0 }), ...patch } } });
    if (patch.cursor !== undefined && patch.cursor !== was && (this.state.logs[k] as { Marking?: boolean } | undefined)?.Marking)
      this.state.act?.({ Do: "mark_to", Key: k, Seq: patch.cursor });
  }
```

(Use the `LogState` type if `store.ts` can import it without a cycle; `log.ts` already imports from `store.ts`, so the inline cast avoids one.)

`StatusLine.tsx` `logStatus`: `[g.Marking ? str.browseMarking() : "", findStatus(...), tags, str.viewSelected(g.Selected)]`; update its comment.

`en.toml` `[web] log_hints`: `"click to select · shift-click extends · m mark · / find · g date · f filter · e export · c copy · esc back"`, and fix the comment above it (m is the TUI's now). `go generate ./internal/str`.

Run `cd web/ui && npm test && npm run check`; expect PASS.

- [ ] **Step 5: Smoke.** In `web/smoke/log_test.go` `TestLog`, after the shift-click block, add click, `m`, click, `m`:

```go
	// m on a clicked line, then a click elsewhere: the range runs between
	// (no Shift needed), and m locks it.
	click(t, ws, ev, rows+`[0]`)
	devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "m", "text": "m"})
	devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "m"})
	waitFor(t, ev, `document.querySelector(".status").textContent.includes(`+jsString(str.BrowseMarking())+`)`, "marking")
	click(t, ws, ev, rows+`.at(-1)`)
	waitFor(t, ev, rows+`.every(r => r.querySelector(".gutter").textContent === `+jsString(glyph.Selected)+`)`, "the live range runs to the clicked line")
	devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "m", "text": "m"})
	devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "m"})
	waitFor(t, ev, `!document.querySelector(".status").textContent.includes(`+jsString(str.BrowseMarking())+`)`, "locked")
```

Make sure the log element has focus before the keys (`ev(logEl + ".focus()")` as the copy step does). Then the drag case from Review Focus 5: in `web/ui` unit tests is enough — assert that `range` during marking is what `onMouseUp` sends (it already is) and rely on Task 1's `NewRange` lock test.

Run: `cd web/ui && npm run build && cd .. && KILN_SMOKE=1 go test ./smoke`
Expected: PASS; goldens unchanged.

- [ ] **Step 6: Commit**

```bash
git add internal/feed internal/webapi internal/str web/ui/src web/smoke
git commit -m "Web log mode: m and the live range, as the TUI's (#191)

Strings: web.log_hints"
```

---

### Task 4: Review and PR

- [ ] Fresh whole-branch review (most capable model), checking especially for cursor moves that bypass `SetCursor`/`setLogView`.
- [ ] `go test ./... && (cd web/ui && npm test && npm run check)`.
- [ ] Push the branch and open a PR closing #191. The description lists the strings: `browse.range_start` (reworded: says how to finish), `browse.marking` (new: statusline part while live), `web.log_hints` (adds `m mark`). Stop; Indi merges.
