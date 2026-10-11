# Web API, step 1a: the filter panel and log mode's cursor go back to the TUI — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The core's filter-panel and log-mode calls name the tag, term or line they act on. The panel being open, its highlight, and log mode's cursor become the TUI's own state. Closes #135.

**Architecture:**
- **Panel (#135).** `app.Log` loses `Panel`, `TogglePanel`, `MovePanel`, `Select`, `FilterSel` and `PanelEntry.Sel`/`Add`. It keeps `Entries`/`EntriesOf` (now listing *every* item, with children of a folded parent marked `FoldedAway`), `FilteredUnder`, `ToggleHide`, `PressOnly`, and `Fold(tag, shut)` (was `SetCollapsed`), plus `AddText(term) bool` and `RemoveText(term) bool`, which just report whether the filter changed. The TUI's `browse.Panel *filterPanel` holds the highlight (`Sel filterSel`), scroll (`top`) and `follow`, replacing `browse.pv`. The `+ Text` row and "keep a highlight when a text row goes" are TUI behavior.
- **Cursor.** `app.Log` loses `Cursor`, `SetCursor`, `MoveCursor`, `ToTop`, `ToBottom`. Calls that read the cursor take a line instead: `Mark(l)`, `SetFind(term, from, found)`, `FindOlder(from, includeFrom, found)`, `FindNewer(from) *LogLine`, `FindStatus(at)`. `found` is called with the match when it turns up, maybe after older days are read (as `GotoDate`'s `show` already is). `Refilter()` only syncs the filter; `NearestVisible(l)` is exported for the front end to keep its place.
- **The TUI keeps its call sites.** `browse` embeds `*app.Log`; it gains a `Cursor` field and methods with the old names (`MoveCursor`, `ToTop`, `ToBottom`, `SetCursor`, `Mark`, `Refilter`, `SetFind`, `FindOlder`, `FindNewer`, `FindStatus`, `GotoDate`) that shadow the embedded ones and pass `b.Cursor`. So `b.Cursor`, `b.MoveCursor(-1)`, `b.Panel.Sel` and the rest read as before, in code and tests.
- **Following live lines** moves from `app.Log.appendLive` to `Model.handleEvent`: a log mode (shown or hidden) whose cursor was on the newest shown line, or had none, moves to the new newest line when a line is added.

**Tech Stack:** Go; `internal/app`, `internal/ui`, `internal/scene`.

**Spec:** `docs/superpowers/specs/2026-10-11-web-api-design.md`, sections "The filter panel (#135)" and "Log mode's cursor". Issue #135 has the reasoning.

## Global Constraints

- Nothing visible changes: golden screens are NOT updated (never `-update`).
- Every existing `ui` test passes. Tests may change how they reach state (`pv.top` → `Panel.top`, `SetCollapsed(app.FilterSel{…}, true)` → `Fold(name, true)`), never what they expect.
- `internal/app` tests change with the API (the calls take lines and terms); what they check stays, except the panel-highlight tests, which move to `ui`.
- `internal/app` doesn't depend on Bubble Tea or lipgloss and never paints.
- Strings come from `internal/str`; this step adds none.
- Run `go vet ./... && go test ./...` and the web module's tests (`cd web && go test ./... && GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...`) before each commit.
- `TestBrowseFindIsFastOnLargeHistories` fails under `-race` on `main` too (a timing threshold); run the suite without `-race`.

## What the old code did as a side effect (keep all of it)

Panel:
- `TogglePanel`: opening highlights the first entry (Untagged or the first tag; `+ Text` if nothing else) and scrolls to it (`pv.follow`); closing drops the highlight and the scroll.
- `MovePanel`: moves by *listed* entries (folded-away children skipped), stopping at both ends; if the highlight isn't listed any more it counts from the first entry.
- `RemoveText(sel)`: does nothing unless `sel` is a text row; otherwise moves the highlight down one first (so one stays, landing on `+ Text` after the last text row), then drops the filter. Callers set `follow` when it removed something.
- `AddText(term)`: only when the filter was added *and* the panel is open does the new row become the highlight; the prompt's caller then sets `follow`. `Refilter` runs either way.
- `SetCollapsed(sel, shut)`: does nothing on `+ Text`, a text row, or a tag without loaded children.
- `Entries` → `foldNew`: a parent is folded the first time the panel sees it (`Char.FoldSeen`, `Char.Collapsed` outlast log mode).

Cursor:
- `OpenLog` put the cursor on the newest shown line. A log mode brought back by Ctrl+L keeps its own.
- `appendLive`: when a live line is *added* (not dropped as a duplicate of a loaded one), a cursor that was nil or on the newest shown line moves to the newest shown line. This applies to a hidden log mode too: there's one `app.Log` per character.
- `Refilter`: syncs the filter's items, then a cursor on a now-hidden line moves to the nearest shown line after it, else before it.
- `MoveCursor`: stops a running search first; moves by shown lines, clamped; past the oldest it pages in an older day and finishes the move when it arrives; with nothing shown and moving up, it pages older and tries again.
- `ToTop`: moves to the oldest shown line, then pages older and repeats until history runs out.
- `FindOlder`/`SetFind`: start at the cursor (or the newest line with no cursor), newest first; a match moves the cursor and clears log mode's status; with no match loaded it pages older days (`Searching`) and goes on below the oldest line it had checked; at the end it says no older matches (or no matches at all).
- `FindNewer`: from just after the cursor; a match moves it and clears the status; else it says no newer matches.
- `GotoDate`: moves the cursor to the day's first shown line *and* calls `show` (the TUI sets `top`).
- `Mark` with no cursor does nothing.

## Review Focus

1. **Live lines while log mode is hidden.** Ctrl+L away with the cursor at the bottom; a line arrives; Ctrl+L back: the cursor is on the new line. With the cursor higher up it stays put (existing `TestLogModeKeptAcrossToggle`). Task 2's `TestHiddenLogModeFollowsLiveLinesAtTheBottom` pins the first half.
2. **Removing the last text row** leaves the highlight on `+ Text`, and Up at the top of the panel stays on the first entry. Task 1's `TestPanelHighlightStaysPut` pins both (they were core tests).
3. **Folded-away entries in the TUI.** They must never be drawn, highlighted or stepped onto, and `panelClick`'s row → entry index must use the same list `panelRows` built. Task 1's `TestEntriesListFoldedAwayChildren` pins the core side; existing `TestPanelOpensAndCloses` and `TestPanelKeys` pin the drawing and stepping.
4. **A find that pages older days lands on the right browse.** `found` closes over the `browse`, so a match arriving after Ctrl+L hid log mode moves the hidden one's cursor, and one arriving after Esc closed it is dropped by `HandleLogOlder` (stale `Log`). Existing `TestLogDropsAStaleOlderRead` and the find tests cover it; the reviewer should trace the closure.
5. **Refilter keeps the place.** Hiding the cursor's line moves it to the nearest shown one. Existing `TestBrowseHidingCursorLineKeepsPlace` pins it; every `b.Refilter()` call site must reach the TUI's wrapper, not `b.Log.Refilter()`.

---

### Task 0: branch and docs

- [ ] **Step 1: Branch**

```bash
git switch -c web-api-1a
```

- [ ] **Step 2: Commit the spec and this plan**

```bash
git add docs/superpowers/specs/2026-10-11-web-api-design.md docs/superpowers/plans/2026-10-11-web-api-1a-panel-and-cursor.md
git commit -m "docs: spec for web-native Kiln part 2, the web API (#125); plan for step 1a"
```

(End every commit message with the session's attribution lines.)

---

### Task 1: the filter panel's core calls name items; the highlight is the TUI's

**Files:**
- Modify: `internal/app/panel.go`, `internal/app/panel_test.go`
- Modify: `internal/ui/filter.go`, `internal/ui/browse.go`, `internal/ui/filter_test.go`

**Interfaces:**
- Produces (in `app`): `type PanelEntry struct{ Item scene.Item; Depth int; Parent bool; FoldedAway bool }`; `Log.Entries() []PanelEntry`; `Log.EntriesOf(items []scene.Item) []PanelEntry`; `Log.Fold(tag string, shut bool)`; `Log.AddText(term string) bool` (added); `Log.RemoveText(term string) bool` (removed); unchanged `ToggleHide`, `PressOnly`, `FilteredUnder`.
- Removed from `app`: `FilterSel`, `Panel`, `Log.Panel`, `TogglePanel`, `MovePanel`, `Select`, `SetCollapsed`, `PanelEntry.Add`, `PanelEntry.Sel`.
- Produces (in `ui`): `type filterSel struct{ Item scene.Item; Add bool }`; `type filterPanel struct{ Sel filterSel; top int; follow bool }`; `browse.Panel *filterPanel` (replaces `browse.pv`); `type panelEntry struct{ app.PanelEntry; Add bool }` with `sel() filterSel`; `browse.entriesOf(items) []panelEntry`, `browse.entries() []panelEntry`, `browse.togglePanel()`, `browse.movePanel(delta int)`, `browse.removeText(sel filterSel) bool`, `browse.fold(sel filterSel, shut bool)`.

- [ ] **Step 1: Rewrite the core panel tests**

Replace `internal/app/panel_test.go` from `func entryNames` to the end with:

```go
// entryNames lists entries as names, a text row as "text:…", and one
// folded away in brackets.
func entryNames(es []PanelEntry) []string {
	var out []string
	for _, e := range es {
		n := e.Item.Name
		if e.Item.Text {
			n = "text:" + n
		}
		if e.FoldedAway {
			n = "(" + n + ")"
		}
		out = append(out, n)
	}
	return out
}

func TestEntriesListFoldedAwayChildren(t *testing.T) {
	_, g := panelLog(t)
	es := g.Entries()
	if got := entryNames(es); !slices.Equal(got, []string{"p", "(p/a)", "(p/b)", "q"}) || !es[0].Parent || es[1].Depth != 1 {
		t.Fatalf("entries %q, %+v; want p folded, its children listed folded away", got, es[:2])
	}
	g.Fold("p", false)
	if got := entryNames(g.Entries()); !slices.Equal(got, []string{"p", "p/a", "p/b", "q"}) {
		t.Errorf("unfolded: %q", got)
	}
	g.Fold("q", true) // no children: nothing to fold
	if g.c.Collapsed["q"] {
		t.Error("folded a tag with no children")
	}
}

func TestFoldsOutliveLogMode(t *testing.T) {
	a, g := panelLog(t)
	g.Entries()
	g.Fold("p", false)
	a.CloseLog("fm/kit")
	g = a.OpenLog("fm/kit")
	if got := entryNames(g.Entries()); !slices.Equal(got, []string{"p", "p/a", "p/b", "q"}) {
		t.Errorf("after reopening: %q; p should stay unfolded", got)
	}
}

func TestTextRowsAddAndRemove(t *testing.T) {
	_, g := panelLog(t)
	if !g.AddText("one") || g.AddText("one") || !g.AddText("three") {
		t.Fatal("AddText should report only a new term")
	}
	if g.RemoveText("q") {
		t.Error("removed a tag")
	}
	if !g.RemoveText("one") || g.RemoveText("one") {
		t.Fatal("RemoveText should report only a term it removed")
	}
	if got := entryNames(g.Entries()); !slices.Equal(got, []string{"p", "(p/a)", "(p/b)", "q", "text:three"}) {
		t.Errorf("entries after removing one: %q", got)
	}
}

func TestHideAndOnlyThroughTheLog(t *testing.T) {
	_, g := panelLog(t)
	q := scene.Item{Name: "q"}
	g.ToggleHide(q)
	if !g.c.Filter.Hidden(q) {
		t.Error("q not hidden")
	}
	g.Fold("p", true)
	g.PressOnly(scene.Item{Name: "p/a"})
	if !g.FilteredUnder("p", g.Items()) {
		t.Error("p's folded children don't show as filtered")
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/app -run 'Entries|Folds|TextRows|HideAndOnly'`
Expected: build failure (`g.Fold undefined`, `e.FoldedAway undefined`, `RemoveText` argument type).

- [ ] **Step 3: Rewrite the core panel**

In `internal/app/panel.go`, add `"slices"` to the imports, and replace everything from `// FilterSel is` through the end of `TogglePanel` with:

```go
// PanelEntry is one item in the filter panel's list.
type PanelEntry struct {
	Item       scene.Item
	Depth      int  // slashes in a tag; 0 for text
	Parent     bool // a tag with children
	FoldedAway bool // under a folded parent: a list shows it only once that opens
}
```

Replace `Entries` and `EntriesOf` with:

```go
// Entries is what the panel lists: tags (each child of a folded parent
// marked FoldedAway), then text rows.
func (g *Log) Entries() []PanelEntry { return g.EntriesOf(g.Items()) }

// EntriesOf is Entries for items already in hand.
func (g *Log) EntriesOf(items []scene.Item) []PanelEntry {
	g.foldNew(items)
	out := make([]PanelEntry, 0, len(items))
	for i, it := range items {
		e := PanelEntry{Item: it}
		if !it.Text {
			e.Depth = strings.Count(it.Name, "/")
			e.Parent = i+1 < len(items) && !items[i+1].Text && strings.HasPrefix(items[i+1].Name, it.Name+"/")
			e.FoldedAway = g.foldedAway(it.Name)
		}
		out = append(out, e)
	}
	return out
}
```

Delete `MovePanel` and `Select`. Replace `SetCollapsed` with:

```go
// Fold folds a parent tag shut or open. A tag without loaded children is
// left be.
func (g *Log) Fold(tag string, shut bool) {
	if !g.hasChildren(tag) {
		return
	}
	if g.c.Collapsed == nil {
		g.c.Collapsed = map[string]bool{}
	}
	if shut {
		g.c.Collapsed[tag] = true
	} else {
		delete(g.c.Collapsed, tag)
	}
}
```

Replace `AddText` and `RemoveText` with:

```go
// AddText adds a text filter for term, with Only, and reports whether it
// was new.
func (g *Log) AddText(term string) bool { return g.c.Filter.AddText(term, g.Items()) }

// RemoveText drops the text filter for term and reports whether there
// was one.
func (g *Log) RemoveText(term string) bool {
	if !slices.Contains(g.c.Filter.Texts(), scene.Item{Name: term, Text: true}) {
		return false
	}
	g.c.Filter.RemoveText(term)
	return true
}
```

In `internal/app/log.go`, delete the `Panel *Panel` field from `Log`.

- [ ] **Step 4: Run the core tests**

Run: `go test ./internal/app`
Expected: PASS. (`internal/ui` doesn't build yet.)

- [ ] **Step 5: Write the TUI test that takes over the core's highlight tests**

Add to `internal/ui/filter_test.go`:

```go
// The highlight stops at the top, and removing the last text row leaves
// it on + Text.
func TestPanelHighlightStaysPut(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	h.key("up")
	if h.br().Panel.Sel.Item != (scene.Item{Untagged: true}) {
		t.Errorf("up at the top moved to %v", h.br().Panel.Sel)
	}
	for range 10 {
		h.key("down") // to + Text
	}
	h.key("enter")
	h.typeText("lighthouse")
	h.key("enter") // the new row is highlighted
	h.key("x")
	if !h.br().Panel.Sel.Add {
		t.Errorf("after removing the last text row: %v; want + Text", h.br().Panel.Sel)
	}
}
```

In the same file, change the access paths: every `h.br().pv.top` becomes `h.br().Panel.top`, and in `TestPanelIsFastOnLargeHistories`

```go
		h.br().SetCollapsed(app.FilterSel{Item: scene.Item{Name: fmt.Sprintf("p%d", i)}}, true)
```

becomes

```go
		h.br().Fold(fmt.Sprintf("p%d", i), true)
```

(If `app` is then unused in the file's imports, drop it.)

- [ ] **Step 6: Move the highlight into the TUI**

In `internal/ui/browse.go`, replace the field

```go
	pv        filterPanel // the filter panel's scroll, while it's open
```

with

```go
	Panel     *filterPanel // the filter panel, while it's open
```

and in `promptKey`'s `promptFilterText` case replace

```go
			if b.AddText(v) {
				b.pv.follow = true
			}
```

with

```go
			if b.AddText(v) && b.Panel != nil { // a new row becomes the highlight
				b.Panel.Sel = filterSel{Item: scene.Item{Name: v, Text: true}}
				b.Panel.follow = true
			}
```

adding `"github.com/latrani/Kiln/internal/scene"` to `browse.go`'s imports.

In `internal/ui/filter.go`, add `"github.com/latrani/Kiln/internal/scene"` to the imports and replace everything from `// filterPanel is` through the end of `togglePanel` with:

```go
// filterSel is the panel's highlighted entry: an item, or the + Text
// row.
type filterSel struct {
	Item scene.Item
	Add  bool
}

// filterPanel is the filter panel while it's open: its highlight and
// scroll. The core holds the filter and the folds.
type filterPanel struct {
	Sel    filterSel // the highlighted entry
	top    int       // first row drawn
	follow bool      // scroll the highlight into view on the next draw
}

// panelEntry is a row the panel lists: one of the core's entries, or
// + Text.
type panelEntry struct {
	app.PanelEntry
	Add bool
}

func (e panelEntry) sel() filterSel { return filterSel{Item: e.Item, Add: e.Add} }

type panelRowKind int

const (
	prName panelRowKind = iota
	prButtons
	prBlank
	prAdd
)

type panelRow struct {
	text  string    // drawn, not yet fitted
	entry int       // index into entries()
	sel   filterSel // the entry's, on its name or + Text row
	kind  panelRowKind
}

// entriesOf is what the panel lists for items: the core's entries less
// those folded away, then + Text.
func (b *browse) entriesOf(items []scene.Item) []panelEntry {
	var out []panelEntry
	for _, e := range b.EntriesOf(items) {
		if !e.FoldedAway {
			out = append(out, panelEntry{PanelEntry: e})
		}
	}
	return append(out, panelEntry{Add: true})
}

// entries is entriesOf the items as they are now.
func (b *browse) entries() []panelEntry { return b.entriesOf(b.Items()) }

// togglePanel opens the filter panel on its first entry, scrolled to it,
// or closes it.
func (b *browse) togglePanel() {
	if b.Panel != nil {
		b.Panel = nil
		return
	}
	b.Panel = &filterPanel{Sel: b.entries()[0].sel(), follow: true}
}

// movePanel moves the highlight by delta entries, stopping at the ends.
func (b *browse) movePanel(delta int) {
	es := b.entries()
	i := 0
	for j, e := range es {
		if e.sel() == b.Panel.Sel {
			i = j
		}
	}
	b.Panel.Sel = es[min(max(0, i+delta), len(es)-1)].sel()
}

// removeText drops sel if it's a text row, moving the highlight down one
// first so one stays, and reports whether it did.
func (b *browse) removeText(sel filterSel) bool {
	if !sel.Item.Text {
		return false
	}
	b.movePanel(1)
	b.RemoveText(sel.Item.Name)
	return true
}

// fold folds sel shut or open if it's a tag; + Text and text rows don't
// fold.
func (b *browse) fold(sel filterSel, shut bool) {
	if !sel.Add && !sel.Item.Text {
		b.Fold(sel.Item.Name, shut)
	}
}
```

Then, in the rest of `filter.go`:
- `panelRows`: `es := b.EntriesOf(items)` → `es := b.entriesOf(items)`; each `e.Sel()` → `e.sel()`.
- `panelView`: `p := &b.pv` → `p := b.Panel`.
- `panelWindow`: `p := &b.pv` → `p := b.Panel`.
- `panelKey`: the close case's `b.TogglePanel()` → `b.togglePanel()`; `b.MovePanel(-1)` / `b.MovePanel(1)` → `b.movePanel(-1)` / `b.movePanel(1)`; each `b.pv.follow = true` → `b.Panel.follow = true`; `b.SetCollapsed(sel, true)` / `(sel, false)` → `b.fold(sel, true)` / `b.fold(sel, false)`; `if b.RemoveText(sel) {` → `if b.removeText(sel) {`.
- `panelClick`: `es := b.Entries()` → `es := b.entries()`; each `b.Select(e.Sel())` → `b.Panel.Sel = e.sel()`; `if b.RemoveText(e.Sel()) {` → `if b.removeText(e.sel()) {`; `b.pv.follow = true` → `b.Panel.follow = true`; `b.SetCollapsed(e.Sel(), !b.cs.Collapsed[e.Item.Name])` → `b.Fold(e.Item.Name, !b.cs.Collapsed[e.Item.Name])`.
- `panelScroll`: `b.pv.top` (both) → `b.Panel.top`.

Find any other user:

```bash
grep -rn "\.pv\b\|\.pv\.\|TogglePanel\|MovePanel\|SetCollapsed\|FilterSel\|\.Select(" internal/ web/ --include=*.go
```

Expected: no hits outside comments. (`model.go` and `view.go` read `cs.browse.Panel != nil`, which now reads the TUI's field and needs no change; `view.go:291`'s `cs.browse.togglePanel()` is already the TUI's.)

- [ ] **Step 7: Run everything**

Run: `go vet ./... && go test ./...`, then the web module's tests (Global Constraints).
Expected: PASS, golden screens untouched.

- [ ] **Step 8: Commit**

```bash
git add internal/app/panel.go internal/app/panel_test.go internal/app/log.go internal/ui/filter.go internal/ui/browse.go internal/ui/filter_test.go
git commit -m "app: the filter panel's calls name a tag or term; its highlight is the TUI's

Entries lists folded-away children, marked, so a list can open a fold
without asking again. Closes #135."
```

---

### Task 2: log mode's cursor is the TUI's

**Files:**
- Modify: `internal/app/log.go`, `internal/app/log_test.go`
- Create: `internal/ui/logcursor.go`
- Modify: `internal/ui/browse.go`, `internal/ui/model.go`, `internal/ui/browse_test.go`

**Interfaces:**
- Consumes: Task 1's `browse.Panel`.
- Produces (in `app`): `Log.Mark(l *LogLine)`; `Log.Refilter()` (syncs only); `Log.NearestVisible(l *LogLine) *LogLine`; `Log.SetFind(term string, from *LogLine, found func(*LogLine)) []Effect`; `Log.FindOlder(from *LogLine, includeFrom bool, found func(*LogLine)) []Effect`; `Log.FindNewer(from *LogLine) *LogLine`; `Log.FindStatus(at *LogLine) string`; unchanged `Log.GotoDate(day string, show func(*LogLine)) []Effect` (no longer touches a cursor).
- Removed from `app`: `Log.Cursor`, `SetCursor`, `MoveCursor`, `ToTop`, `ToBottom`.
- Produces (in `ui`): `browse.Cursor *app.LogLine`; `browse` methods `SetCursor(l)`, `MoveCursor(delta int) []app.Effect`, `ToTop() []app.Effect`, `ToBottom()`, `Mark()`, `Refilter()`, `SetFind(term string) []app.Effect`, `FindOlder(includeCursor bool) []app.Effect`, `FindNewer()`, `FindStatus() string`, `GotoDate(day string) []app.Effect`, `found(l *app.LogLine)`.

- [ ] **Step 1: Update the core log tests**

In `internal/app/log_test.go`:

`TestOpenLogLoadsWholeDaysUpFront`'s second check becomes

```go
	if a.Char("fm/kit").Log != g || a.OpenLog("fm/kit") != g {
		t.Error("the log isn't the character's")
	}
```

`TestFindReadsOlderDaysUntilAMatch` becomes

```go
func TestFindReadsOlderDaysUntilAMatch(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	var found *LogLine
	effs := g.SetFind("09-22 7", nil, func(l *LogLine) { found = l })
	if !g.Searching || g.FindStatus(nil) != str.BrowseSearching("09-22 7", DayLabel("2026-09-23")) {
		t.Fatalf("searching %v, status %q", g.Searching, g.FindStatus(nil))
	}
	drain(a, effs)
	if g.Searching || found == nil || found.Entry.Text != "09-22 79" {
		t.Errorf("searching %v, found %v", g.Searching, found)
	}
}
```

`TestStopSearchLeavesTheCursor` becomes

```go
func TestStopSearchFindsNothing(t *testing.T) {
	a := logApp(t, 150, 150, 150)
	g := a.OpenLog("fm/kit")
	called := false
	effs := g.SetFind("nowhere", nil, func(*LogLine) { called = true })
	if !g.StopSearch() || g.Searching || g.Status != str.BrowseSearchCancelled() {
		t.Fatalf("stop: searching %v, status %q", g.Searching, g.Status)
	}
	drain(a, effs) // the day still arrives, to nothing
	if called {
		t.Error("the cancelled search found something")
	}
}
```

In `TestGotoDateShowsTheDaysFirstLine`, the check becomes

```go
	if shown == nil || shown.Entry.Text != "09-22 0" {
		t.Errorf("shown %v", shown)
	}
```

In `TestSelectionIsReceivedShownUnexcludedLines` and `TestCopyAndExportAreEffects`, each `g.SetCursor(x)` + `g.Mark()` pair becomes `g.Mark(x)`.

Add:

```go
func TestFindStepsFromTheLineItsGiven(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	l, _ := a.LogLayout(a.Char("fm/kit").Ch)
	kilntest.WriteLog(t, l, logDay, "x 1", "y", "x 2", "x 3")
	g := a.OpenLog("fm/kit")
	var found *LogLine
	at := func(l *LogLine) { found = l }
	g.SetFind("x", nil, at) // nowhere yet: from the newest
	if found != g.Lines[3] {
		t.Fatalf("found %v; want the newest match", found)
	}
	g.FindOlder(g.Lines[3], false, at)
	if found != g.Lines[2] {
		t.Errorf("older than x 3: %v", found)
	}
	if got := g.FindNewer(g.Lines[0]); got != g.Lines[2] {
		t.Errorf("newer than x 1: %v", got)
	}
	if got := g.FindNewer(g.Lines[3]); got != nil || g.Status != str.BrowseNoNewerMatches("x") {
		t.Errorf("newer than the newest: %v, %q", got, g.Status)
	}
	if s := g.FindStatus(g.Lines[2]); s != str.BrowseFindStatus("x", 2, 3) {
		t.Errorf("status at x 2: %q", s)
	}
}

func TestMarkWithNoLineDoesNothing(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	g := a.OpenLog("fm/kit")
	g.Mark(nil)
	if g.Start != nil || g.Status != "" {
		t.Errorf("start %v, status %q", g.Start, g.Status)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/app -run 'OpenLog|Find|StopSearch|GotoDate|Selection|CopyAndExport|Mark'`
Expected: build failure (wrong argument counts to `SetFind`, `Mark`, `FindStatus`, `FindNewer`).

- [ ] **Step 3: Take the cursor out of the core**

In `internal/app/log.go`:

- Delete the `Cursor *LogLine` field.
- In `OpenLog`, delete `g.Cursor = g.LastVisible()`.
- `appendLive`: delete the `atEnd` line and the `if atEnd { … }` block, leaving `g.Lines = append(g.Lines, makeLogLine(g.c.Rules, e))`. Update its doc comment's first sentence to: `appendLive adds a line that arrived while the log is open; a front end following the newest line moves itself.`
- Replace `Refilter` and `nearestVisible` with:

```go
// Refilter syncs the filter with the items after it changes, so a tag
// seen for the first time obeys it. Keeping the reader's place is the
// front end's: see NearestVisible.
func (g *Log) Refilter() { g.Items() }

// NearestVisible returns the first shown line after l, or failing that
// the last shown line before it, so hiding l keeps the reader's place.
func (g *Log) NearestVisible(l *LogLine) *LogLine {
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
```

- Delete `SetCursor`, `MoveCursor`, `ToTop` and `ToBottom`.
- Replace `Mark` with:

```go
// Mark marks l as the start of a range, or, with a start marked, as its
// other end. A nil l does nothing.
func (g *Log) Mark(l *LogLine) {
	if l == nil {
		return
	}
	if g.Start == nil || g.End != nil {
		g.NewRange(l, nil)
		g.SetStatus(false, str.BrowseRangeStart())
		return
	}
	g.End = l
	if g.Index(g.End) < g.Index(g.Start) {
		g.Start, g.End = g.End, g.Start
	}
	g.RangeStatus()
}
```

- Replace `SetFind`, `FindOlder` and `findOlderFrom` with:

```go
// SetFind sets the find term and finds its nearest match at from or
// older; see FindOlder.
func (g *Log) SetFind(term string, from *LogLine, found func(*LogLine)) []Effect {
	g.Find = term
	return g.FindOlder(from, true, found)
}

// FindOlder finds the nearest match older than from (or at it, with
// includeFrom; from the newest line if from is nil), the way find goes:
// newest first, and hands it to found. It reads in older days until one
// turns up or history runs out; it doesn't wrap.
func (g *Log) FindOlder(from *LogLine, includeFrom bool, found func(*LogLine)) []Effect {
	if g.Find == "" {
		return nil
	}
	i := len(g.Lines) - 1
	if fi := g.Index(from); fi >= 0 {
		i = fi
		if !includeFrom {
			i--
		}
	}
	return g.findOlderFrom(i, found)
}

// findOlderFrom looks for a match at line i or older; see FindOlder.
func (g *Log) findOlderFrom(i int, found func(*LogLine)) []Effect {
	re := FindRE(g.Find)
	g.Searching = false
	for ; i >= 0; i-- {
		if l := g.Lines[i]; g.matchAt(re, l) {
			g.Status = ""
			if found != nil {
				found(l)
			}
			return nil
		}
	}
	if !g.HistDone {
		var oldest *LogLine // where this pass stopped; the next goes on below it
		if len(g.Lines) > 0 {
			oldest = g.Lines[0]
		}
		effs := g.Older(func() []Effect { return g.findOlderFrom(g.Index(oldest)-1, found) })
		g.Searching = true
		return effs
	}
	g.findFailed(str.BrowseNoOlderMatches(g.Find))
	return nil
}
```

- Replace `FindNewer` with:

```go
// FindNewer returns the nearest match newer than from (from the oldest
// line if from is nil). Everything newer is always loaded; it doesn't
// wrap. nil, saying so, if there's none.
func (g *Log) FindNewer(from *LogLine) *LogLine {
	if g.Find == "" {
		return nil
	}
	re := FindRE(g.Find)
	for _, l := range g.Lines[g.Index(from)+1:] {
		if g.matchAt(re, l) {
			g.Status = ""
			return l
		}
	}
	g.findFailed(str.BrowseNoNewerMatches(g.Find))
	return nil
}
```

- `matchPos` and `FindStatus` take the line:

```go
// matchPos is at's match counted from the newest, of n loaded; more says
// older history not yet loaded may hold others.
func (g *Log) matchPos(at *LogLine) (i, n int, more bool) {
	ms := g.Matches()
	if k := slices.Index(ms, at); k >= 0 {
		i = len(ms) - k
	}
	return i, len(ms), !g.HistDone
}

// FindStatus is the find term and at's place among the matches, for the
// top bar; "" without a find.
func (g *Log) FindStatus(at *LogLine) string {
	if g.Find == "" {
		return ""
	}
	if g.Searching {
		return str.BrowseSearching(g.Find, g.searchedTo())
	}
	i, n, more := g.matchPos(at)
	if more {
		return str.BrowseFindStatusMore(g.Find, i, n)
	}
	return str.BrowseFindStatus(g.Find, i, n)
}
```

- In `GotoDate`, delete `g.Cursor = l`, and change its doc comment's last sentence to: `then hands its first visible line to show, for the front end to go to.`

- [ ] **Step 4: Run the core tests**

Run: `go test ./internal/app`
Expected: PASS.

- [ ] **Step 5: Write the TUI test for following live lines while hidden**

Add to `internal/ui/browse_test.go`, after `TestBrowseLiveLinesArrive`:

```go
func TestHiddenLogModeFollowsLiveLinesAtTheBottom(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.key("ctrl+l")
	b := h.br()
	h.key("ctrl+l") // hidden, the cursor on the newest line
	h.conn("fm/kit").Feed("Brand new line")
	h.settle("fm/kit", func() bool { return b.Last().Entry.Text == "Brand new line" })
	if b.Cursor != b.LastVisible() {
		t.Errorf("a hidden log mode's cursor should follow live lines: on %q", b.Cursor.Entry.Text)
	}
}
```

- [ ] **Step 6: Give the TUI its cursor**

Create `internal/ui/logcursor.go`:

```go
package ui

import (
	"slices"

	"github.com/latrani/Kiln/internal/app"
)

// Log mode's cursor is the terminal's: the line keys move, find steps
// from, m marks and t describes. These methods shadow app.Log's, passing
// the cursor where the core takes a line.

// SetCursor puts the cursor on l: a click, or the view dragging it along.
func (b *browse) SetCursor(l *app.LogLine) { b.Cursor = l }

// MoveCursor moves by delta shown lines. Moving up past the oldest
// loaded line stops there and pages in older history; the rest of the
// move happens when it arrives.
func (b *browse) MoveCursor(delta int) []app.Effect {
	b.StopSearch() // the wheel, say: you've moved on
	v := b.Visible()
	if len(v) == 0 {
		if delta < 0 { // everything loaded is hidden; look further back
			return b.Older(func() []app.Effect { return b.MoveCursor(delta) })
		}
		return nil
	}
	i := slices.Index(v, b.Cursor)
	if i < 0 {
		i = len(v) - 1
	}
	b.Cursor = v[min(max(0, i+delta), len(v)-1)]
	if rest := i + delta; rest < 0 {
		return b.Older(func() []app.Effect { return b.MoveCursor(rest) })
	}
	return nil
}

// ToTop pages in all history, then moves to the oldest shown line.
func (b *browse) ToTop() []app.Effect {
	if v := b.Visible(); len(v) > 0 {
		b.Cursor = v[0]
	}
	return b.Older(b.ToTop)
}

// ToBottom moves to the newest shown line.
func (b *browse) ToBottom() { b.Cursor = b.LastVisible() }

// Mark marks the cursor's line; see app.Log.Mark.
func (b *browse) Mark() { b.Log.Mark(b.Cursor) }

// Refilter syncs the filter and keeps the reader's place: a cursor on a
// line now hidden moves to the nearest shown one.
func (b *browse) Refilter() {
	b.Log.Refilter()
	if b.Cursor != nil && !b.Shows(b.Cursor) {
		if n := b.NearestVisible(b.Cursor); n != nil {
			b.Cursor = n
		}
	}
}

// found moves the cursor to a find's match.
func (b *browse) found(l *app.LogLine) { b.Cursor = l }

// SetFind sets the find term and goes to its nearest match at the cursor
// or older.
func (b *browse) SetFind(term string) []app.Effect { return b.Log.SetFind(term, b.Cursor, b.found) }

// FindOlder goes to the nearest match older than the cursor (or at it,
// with includeCursor).
func (b *browse) FindOlder(includeCursor bool) []app.Effect {
	return b.Log.FindOlder(b.Cursor, includeCursor, b.found)
}

// FindNewer goes to the nearest match newer than the cursor.
func (b *browse) FindNewer() {
	if l := b.Log.FindNewer(b.Cursor); l != nil {
		b.Cursor = l
	}
}

// FindStatus is the find term and the cursor's match, for the top bar.
func (b *browse) FindStatus() string { return b.Log.FindStatus(b.Cursor) }

// GotoDate goes to day's first shown line and puts it at the top of the
// body.
func (b *browse) GotoDate(day string) []app.Effect {
	return b.Log.GotoDate(day, func(l *app.LogLine) { b.Cursor, b.top = l, l })
}
```

In `internal/ui/browse.go`, add the field after `cs *charState`:

```go
	Cursor    *app.LogLine            // the highlighted line; see logcursor.go
```

and in `promptKey` change

```go
			return b.run(b.GotoDate(v, func(l *app.LogLine) { b.top = l }))
```

to

```go
			return b.run(b.GotoDate(v))
```

In `internal/ui/model.go`, `openBrowse`, after the line that builds `cs.browse`, add:

```go
	cs.browse.Cursor = g.LastVisible()
```

In `internal/ui/model.go`, `handleEvent`, replace

```go
	prev := m.a.Active()
	ev, ok, effs := m.a.Handle(msg)
	if !ok {
		return nil // stale session, or it has shut down
	}
	if ev.Closed {
		m.closed(ev.Key, prev)
		m.activated(prev)
		return nil
	}
	cs := m.chars[ev.Key]
```

with

```go
	prev := m.a.Active()
	type following struct {
		b    *browse
		last *app.LogLine
	}
	var follow []following // log modes whose cursor is on the newest line: a new line takes it along
	if cs := m.chars[msg.Key]; cs != nil {
		for _, b := range cs.browses() {
			if b.Cursor == nil || b.Cursor == b.LastVisible() {
				follow = append(follow, following{b, b.Last()})
			}
		}
	}
	ev, ok, effs := m.a.Handle(msg)
	if !ok {
		return nil // stale session, or it has shut down
	}
	if ev.Closed {
		m.closed(ev.Key, prev)
		m.activated(prev)
		return nil
	}
	for _, f := range follow {
		if f.b.Last() != f.last { // a line was added, not dropped as one already loaded
			f.b.Cursor = f.b.LastVisible()
		}
	}
	cs := m.chars[ev.Key]
```

Find any other user of the removed core calls:

```bash
grep -rn "\.Log\.Cursor\|g\.Cursor\|\.MoveCursor\|\.ToTop()\|\.ToBottom()\|FindOlder(\|FindNewer(\|FindStatus(\|SetFind(\|GotoDate(\|\.Mark(" internal/ web/ --include=*.go | grep -v "_test.go"
```

Expected: only `internal/app/log.go` (definitions), `internal/ui/logcursor.go`, and `browse`/`view.go` calls through the TUI's methods (`b.…` with the TUI's arguments). `cs.sb.ToBottom()` in `model.go` is the scrollback's own and is unrelated.

- [ ] **Step 7: Run everything**

Run: `go vet ./... && go test ./...`, then the web module's tests (Global Constraints).
Expected: PASS, golden screens untouched. `ui` tests that set `b.Cursor = …` directly (e.g. `TestBrowseFindIsFastOnLargeHistories`) now set the TUI's field, which is what the methods read.

- [ ] **Step 8: Commit**

```bash
git add internal/app/log.go internal/app/log_test.go internal/ui/logcursor.go internal/ui/browse.go internal/ui/model.go internal/ui/browse_test.go
git commit -m "app: log mode's cursor is the TUI's; find, mark and status take a line"
```

---

### Task 3: review and PR

- [ ] **Step 1: Fresh review.** A fresh opus subagent reviews the whole branch against `main` with this plan's Review Focus and side-effect list, looking hardest for a side effect lost in the move. Fix Critical and Important findings with a failing test first; log minors in the PR.
- [ ] **Step 2: Open the PR** titled "Web API step 1a: the filter panel and log mode's cursor go back to the TUI". The body says it closes #135, links the spec, says golden screens are unchanged and no strings changed, and lists the core API changes. Then stop: Indi reviews and merges.
