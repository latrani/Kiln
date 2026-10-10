# Headless core, step 4b: the core owns the filter panel — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The filter panel's behavior lives in `app.Log`: whether it's open, its entries, which parents are folded, the highlighted entry, and the actions on it (hide, only, fold, add and remove a text filter). The TUI keeps drawing the panel: its rows and glyphs, its scroll window and when to scroll the highlight into view, click hit-testing, and the key map. With this, part 1's "Done means" holds.

**Architecture:**
- **`app.Log.Panel`** (`*app.Panel`, nil while closed) holds the highlighted entry, `Sel app.FilterSel`. `TogglePanel` opens it on the first entry or closes it.
- **Entries** (`app.PanelEntry`: item, depth, parent, or the `+ Text` row) come from `Log.Entries()`, which folds each parent the first time it sees it.
- **Folds outlast log mode,** as they do now: `collapsed` and `foldSeen` move from the TUI's `charState` to `app.Char.Collapsed` and `app.Char.FoldSeen`.
- **Actions** are `Log` methods: `MovePanel`, `Select`, `SetCollapsed`, `ToggleHide`, `PressOnly`, `AddText`, `RemoveText`. The TUI still calls `Refilter` after a panel key or click, as now.
- **Scrolling the highlight into view is the TUI's.** The TUI's `filterPanel` keeps `top` and `follow`, lives at `browse.pv`, and sets `follow` at exactly the places `main`'s code path did: opening the panel, moving with the keys, removing a text row (key or click), and adding a text filter that gets selected.

**Tech Stack:** Go; `internal/app`, `internal/ui`, `internal/scene`.

**Spec:** `docs/superpowers/specs/2026-10-07-headless-core-design.md`, section "Log mode" (the panel's entries, folds and selection in `app.Log`; its scroll window and hit-testing in `ui`). Step 4a's plan (`2026-10-10-headless-core-4a-log.md`) has the rulings this builds on.

## Global Constraints

- Nothing visible changes: golden screens are NOT updated (never `-update`).
- Every existing test passes. Tests may change how they reach state (`b.Panel.Sel`, `b.pv.top`, `cs.Collapsed`, `app.FilterSel{Item: …}`), never what they expect.
- `internal/app` doesn't depend on Bubble Tea or lipgloss and never paints.
- Strings come from `internal/str`; this step adds none.
- Only `App` (and its `Log` methods) change `app.Char` and `app.Log` fields; the TUI reads them.
- Run `go vet ./... && go test ./...` and the web module's tests (`cd web && go test ./... && GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...`) before committing.

## What the old code did as a side effect (keep all of it)

- `togglePanel`: opening selects the first entry and scrolls it into view (`follow`); closing drops the panel (its scroll too).
- `entriesOf` → `foldNew`: a parent tag, at any depth, is folded the first time the panel sees it; after that it stays as the reader left it, across closing and reopening log mode (`collapsed`/`foldSeen` live on the character). Children of a folded parent are left out; `+ Text` is always last.
- `panelKey`: Esc, Ctrl+C and the panel key close the panel without refiltering; every other key refilters after acting, even one that did nothing. `h`/`o` do nothing on `+ Text`; Enter on `+ Text` opens the text prompt; Left/Right fold only a tag that has children.
- `movePanel`: moves by entries, stopping at the ends, and scrolls into view.
- `removeText`: only a text row; it moves the highlight down one first (so one stays), which scrolls into view, then drops the filter.
- Adding a text filter from the prompt: the filter is added (it may already exist: then nothing new), and only if it was added *and* the panel is open does the new row become the highlight, scrolled into view; refilter either way.
- `panelClick`: ignored while a prompt is open; `▲`/`▼` rows page the scroll; `+ Text` selects it and opens the prompt; on a name, the glyph column removes a text row (and returns after refiltering) or folds/unfolds a parent, then the name is selected; on the button row, Hide or Only is pressed, then the entry is selected; refilter at the end. A click selects without scrolling into view (except the × removal, through `removeText`).
- `filteredUnder`: a folded parent shows the filtered mark when any tag under it is hidden or is the Only.

## Review Focus

1. **Folds across log-mode sessions.** Fold a parent open, Esc out of log mode, reopen: it stays open; a parent seen for the first time starts folded. Task 1's `TestFoldsOutliveLogMode` pins it in `app`.
2. **When the panel scrolls to the highlight.** Opening, arrow keys, removing a text row and adding one scroll it into view; clicks and the wheel don't. Existing `filter_test.go` scroll tests cover most of it; the reviewer should trace each `follow` site against `main`.
3. **Removing the last text row** leaves the highlight on `+ Text`. Task 1's `TestTextRowsAddAndRemove` pins it.
4. **Hide and Only on a tag that just appeared.** Items come from the loaded lines plus the filter's own tags, synced each time; a tag first seen while the panel is open obeys the filter already set. Existing filter tests cover it.
5. **Keys on `+ Text`.** `h`, `o`, Left, Right and `x` do nothing there; Enter opens the prompt. Existing tests plus the side-effect list.

---

### Task 1: the filter panel in `app.Log`, drawn by the TUI

**Files:**
- Create: `internal/app/panel.go`, `internal/app/panel_test.go`
- Modify: `internal/app/app.go` (`Char.Collapsed`, `Char.FoldSeen`), `internal/app/log.go` (`Log.Panel`), `internal/ui/filter.go`, `internal/ui/browse.go`, `internal/ui/model.go`, `internal/ui/view.go`
- Modify (access paths only): `internal/ui/filter_test.go` and any other hit of Step 6's grep

**Interfaces:**
- Produces (in `app`): `type FilterSel struct{ Item scene.Item; Add bool }`; `type PanelEntry struct{ Item scene.Item; Add bool; Depth int; Parent bool }` with `Sel() FilterSel`; `type Panel struct{ Sel FilterSel }`; `Log.Panel *Panel`; `Char.Collapsed, Char.FoldSeen map[string]bool`; `Log` methods `TogglePanel()`, `Entries() []PanelEntry`, `EntriesOf(items []scene.Item) []PanelEntry`, `FilteredUnder(parent string, items []scene.Item) bool`, `MovePanel(delta int)`, `Select(sel FilterSel)`, `SetCollapsed(sel FilterSel, shut bool)`, `ToggleHide(it scene.Item)`, `PressOnly(it scene.Item)`, `AddText(term string) (selected bool)`, `RemoveText(sel FilterSel) (removed bool)`.
- The TUI's `browse` loses `panel` and gains `pv filterPanel` (`top int`, `follow bool`); `filterSel` and `panelEntry` go; `panelRow.sel` is an `app.FilterSel`.

- [ ] **Step 1: Write the core tests** — `internal/app/panel_test.go`:

```go
package app

import (
	"slices"
	"testing"

	"github.com/latrani/Kiln/internal/kilntest"
	"github.com/latrani/Kiln/internal/scene"
)

// tagWorld classifies "A …" as p/a, "B …" as p/b and "Q …" as q.
const tagWorld = fmWorld + `
[[classify]]
tag = "p/a"
pattern = '^A '

[[classify]]
tag = "p/b"
pattern = '^B '

[[classify]]
tag = "q"
pattern = '^Q '
`

func panelLog(t *testing.T) (*App, *Log) {
	t.Helper()
	a := sessionApp(t, map[string]string{"fm": tagWorld})
	openAll(t, a, "fm/kit")
	l, _ := a.LogLayout(a.Char("fm/kit").Ch)
	kilntest.WriteLog(t, l, logDay, "A one", "B two", "Q three")
	return a, a.OpenLog("fm/kit")
}

func entryNames(es []PanelEntry) []string {
	var out []string
	for _, e := range es {
		switch {
		case e.Add:
			out = append(out, "+")
		case e.Item.Text:
			out = append(out, "text:"+e.Item.Name)
		default:
			out = append(out, e.Item.Name)
		}
	}
	return out
}

func TestEntriesFoldNewParents(t *testing.T) {
	_, g := panelLog(t)
	if got := entryNames(g.Entries()); !slices.Equal(got, []string{"p", "q", "+"}) {
		t.Fatalf("entries %q; want p folded", got)
	}
	g.SetCollapsed(FilterSel{Item: scene.Item{Name: "p"}}, false)
	es := g.Entries()
	if got := entryNames(es); !slices.Equal(got, []string{"p", "p/a", "p/b", "q", "+"}) || !es[0].Parent || es[1].Depth != 1 {
		t.Errorf("unfolded: %q, %+v", got, es[:2])
	}
	g.SetCollapsed(FilterSel{Item: scene.Item{Name: "q"}}, true) // no children: nothing to fold
	if g.c.Collapsed["q"] {
		t.Error("folded a tag with no children")
	}
}

func TestFoldsOutliveLogMode(t *testing.T) {
	a, g := panelLog(t)
	g.Entries()
	g.SetCollapsed(FilterSel{Item: scene.Item{Name: "p"}}, false)
	a.CloseLog("fm/kit")
	g = a.OpenLog("fm/kit")
	if got := entryNames(g.Entries()); !slices.Equal(got, []string{"p", "p/a", "p/b", "q", "+"}) {
		t.Errorf("after reopening: %q; p should stay unfolded", got)
	}
}

func TestPanelOpensOnTheFirstEntryAndMovesWithinIt(t *testing.T) {
	_, g := panelLog(t)
	g.TogglePanel()
	if g.Panel == nil || g.Panel.Sel != (FilterSel{Item: scene.Item{Name: "p"}}) {
		t.Fatalf("opened on %+v", g.Panel)
	}
	g.MovePanel(-1)
	if g.Panel.Sel.Item.Name != "p" {
		t.Error("moved past the top")
	}
	g.MovePanel(100)
	if !g.Panel.Sel.Add {
		t.Errorf("moved to %+v; want + Text", g.Panel.Sel)
	}
	g.TogglePanel()
	if g.Panel != nil {
		t.Error("didn't close")
	}
}

func TestTextRowsAddAndRemove(t *testing.T) {
	_, g := panelLog(t)
	if g.AddText("three") {
		t.Error("selected a row with the panel closed")
	}
	g.TogglePanel()
	if !g.AddText("one") || g.Panel.Sel != (FilterSel{Item: scene.Item{Name: "one", Text: true}}) {
		t.Fatalf("add: sel %+v", g.Panel.Sel)
	}
	if g.RemoveText(FilterSel{Item: scene.Item{Name: "q"}}) {
		t.Error("removed a tag row")
	}
	sel := g.Panel.Sel
	if !g.RemoveText(sel) {
		t.Fatal("didn't remove the text row")
	}
	if got := entryNames(g.Entries()); !slices.Equal(got, []string{"p", "q", "text:three", "+"}) {
		t.Errorf("entries after removing one: %q", got)
	}
	g.Select(FilterSel{Item: scene.Item{Name: "three", Text: true}})
	g.RemoveText(g.Panel.Sel) // the last text row
	if !g.Panel.Sel.Add {
		t.Errorf("highlight after removing the last text row: %+v; want + Text", g.Panel.Sel)
	}
}

func TestHideAndOnlyThroughTheLog(t *testing.T) {
	_, g := panelLog(t)
	q := scene.Item{Name: "q"}
	g.ToggleHide(q)
	if !g.c.Filter.Hidden(q) {
		t.Error("q not hidden")
	}
	g.SetCollapsed(FilterSel{Item: scene.Item{Name: "p"}}, true)
	g.PressOnly(scene.Item{Name: "p/a"})
	if !g.FilteredUnder("p", g.Items()) {
		t.Error("p's folded children don't show as filtered")
	}
}
```

Notes: `tagWorld` appends `[[classify]]` after `[[characters]]`; check `fmWorld` in `app_test.go` ends with a character table and that TOML assigns these to the world (the 4a quiet test put world keys *above* `[[characters]]`, but array-of-tables headers after it are fine). Whether `Untagged` appears depends on every line being tagged; all three are, so it doesn't. If an expectation is off because of how `scene.TagItems` orders items, fix the expectation (from what `main`'s panel would list) and ledger it.

Run: `go test ./internal/app/` — Expected: FAIL to compile (`undefined: PanelEntry`, …).

- [ ] **Step 2: Folds move to `Char`; `Log` gets the panel** — `internal/app/app.go`, in `Char` after `Filter`:

```go
	Collapsed   map[string]bool // filter panel parents folded shut, by tag
	FoldSeen    map[string]bool // parents the panel has already met; a new one starts folded
```

`internal/app/log.go`, in `Log` after `Searching`:

```go
	Panel      *Panel // the filter panel, while it's open
```

- [ ] **Step 3: Write `internal/app/panel.go`** — moved from `ui/filter.go`, `b` → `g`, `b.cs.collapsed` → `g.c.Collapsed`, `b.cs.foldSeen` → `g.c.FoldSeen`, `b.cs.filter` → `g.c.Filter`, `b.items()` → `g.Items()`, `b.panel` → `g.Panel`:

```go
package app

import (
	"strings"

	"github.com/latrani/Kiln/internal/scene"
)

// FilterSel is the filter panel's highlighted entry: an item, or the
// + Text row.
type FilterSel struct {
	Item scene.Item
	Add  bool
}

// PanelEntry is one item in the filter panel, or the + Text row.
type PanelEntry struct {
	Item   scene.Item
	Add    bool
	Depth  int  // slashes in a tag; 0 for text
	Parent bool // a tag with children
}

// Sel is how the panel's highlight names e.
func (e PanelEntry) Sel() FilterSel { return FilterSel{Item: e.Item, Add: e.Add} }

// Panel is log mode's filter panel, while it's open.
type Panel struct {
	Sel FilterSel // the highlighted entry
}

// TogglePanel opens the filter panel on its first entry, or closes it.
func (g *Log) TogglePanel() {
	if g.Panel != nil {
		g.Panel = nil
		return
	}
	g.Panel = &Panel{}
	if es := g.Entries(); len(es) > 0 {
		g.Panel.Sel = es[0].Sel()
	}
}

// Entries is what the panel lists: tags (children of a collapsed parent
// left out), text rows, then + Text.
func (g *Log) Entries() []PanelEntry { return g.EntriesOf(g.Items()) }

// EntriesOf is Entries for items already in hand.
func (g *Log) EntriesOf(items []scene.Item) []PanelEntry {
	g.foldNew(items)
	var out []PanelEntry
	for i, it := range items {
		if !it.Text && g.foldedAway(it.Name) {
			continue
		}
		e := PanelEntry{Item: it}
		if !it.Text {
			e.Depth = strings.Count(it.Name, "/")
			e.Parent = i+1 < len(items) && !items[i+1].Text && strings.HasPrefix(items[i+1].Name, it.Name+"/")
		}
		out = append(out, e)
	}
	return append(out, PanelEntry{Add: true})
}

// foldNew folds each parent tag, at any depth, the first time the panel
// sees it; after that it stays as the reader left it.
func (g *Log) foldNew(items []scene.Item) {
	c := g.c
	for i, it := range items {
		if it.Text || i+1 >= len(items) || items[i+1].Text || !strings.HasPrefix(items[i+1].Name, it.Name+"/") || c.FoldSeen[it.Name] {
			continue
		}
		if c.FoldSeen == nil {
			c.FoldSeen, c.Collapsed = map[string]bool{}, nilToEmpty(c.Collapsed)
		}
		c.FoldSeen[it.Name] = true
		c.Collapsed[it.Name] = true
	}
}

func nilToEmpty(m map[string]bool) map[string]bool {
	if m == nil {
		return map[string]bool{}
	}
	return m
}

// hasChildren reports whether any loaded tag sits under tag.
func (g *Log) hasChildren(tag string) bool {
	for _, it := range g.Items() {
		if !it.Text && strings.HasPrefix(it.Name, tag+"/") {
			return true
		}
	}
	return false
}

// foldedAway reports whether tag sits under a collapsed parent.
func (g *Log) foldedAway(tag string) bool {
	for p := scene.Parent(tag); p != ""; p = scene.Parent(p) {
		if g.c.Collapsed[p] {
			return true
		}
	}
	return false
}

// FilteredUnder reports whether any tag under parent is hidden or has
// Only, for the mark on a folded parent.
func (g *Log) FilteredUnder(parent string, items []scene.Item) bool {
	f := &g.c.Filter
	only, hasOnly := f.Only()
	for _, it := range items {
		if !it.Text && strings.HasPrefix(it.Name, parent+"/") && (f.Hidden(it) || hasOnly && only == it) {
			return true
		}
	}
	return false
}

// MovePanel moves the highlight by delta entries, stopping at the ends.
func (g *Log) MovePanel(delta int) {
	es := g.Entries()
	i := 0
	for j, e := range es {
		if e.Sel() == g.Panel.Sel {
			i = j
		}
	}
	g.Panel.Sel = es[min(max(0, i+delta), len(es)-1)].Sel()
}

// Select highlights sel, as a click does.
func (g *Log) Select(sel FilterSel) { g.Panel.Sel = sel }

// SetCollapsed folds a parent tag shut or open. Anything else is left be.
func (g *Log) SetCollapsed(sel FilterSel, shut bool) {
	if sel.Add || sel.Item.Text || !g.hasChildren(sel.Item.Name) {
		return
	}
	if g.c.Collapsed == nil {
		g.c.Collapsed = map[string]bool{}
	}
	if shut {
		g.c.Collapsed[sel.Item.Name] = true
	} else {
		delete(g.c.Collapsed, sel.Item.Name)
	}
}

// ToggleHide hides it, or shows it again.
func (g *Log) ToggleHide(it scene.Item) { g.c.Filter.ToggleHide(it, g.Items()) }

// PressOnly shows only it, or everything again.
func (g *Log) PressOnly(it scene.Item) { g.c.Filter.PressOnly(it, g.Items()) }

// AddText adds a text filter for term. If it's new and the panel is
// open, its row becomes the highlight, and AddText reports true.
func (g *Log) AddText(term string) bool {
	if !g.c.Filter.AddText(term, g.Items()) || g.Panel == nil {
		return false
	}
	g.Panel.Sel = FilterSel{Item: scene.Item{Name: term, Text: true}}
	return true
}

// RemoveText drops sel if it's a text row, moving the highlight down one
// first so one stays, and reports whether it did.
func (g *Log) RemoveText(sel FilterSel) bool {
	if !sel.Item.Text {
		return false
	}
	g.MovePanel(1) // keep a highlight when the row goes
	g.c.Filter.RemoveText(sel.Item.Name)
	return true
}
```

Run: `go test ./internal/app/` — Expected: PASS (fix only test expectations per Step 1's notes, ledgered).

- [ ] **Step 4: The TUI draws the core's panel** — `internal/ui/filter.go`:
  - Delete `filterSel`, `panelEntry` (and its `sel`), `entries`, `entriesOf`, `foldNew`, `nilToEmpty`, `hasChildren`, `foldedAway`, `filteredUnder`, `removeText`, `movePanel`, `setCollapsed`.
  - `filterPanel` keeps only `top` and `follow`; its doc says it's the panel's scroll, the TUI's half.
  - `togglePanel` becomes:

    ```go
    // togglePanel opens or closes the filter panel, scrolled to its
    // highlight when it opens.
    func (b *browse) togglePanel() {
    	b.TogglePanel()
    	b.pv = filterPanel{follow: true}
    }
    ```
  - `panelRow.sel` is `app.FilterSel`. In `panelRows`: `es := b.EntriesOf(items)`; entry fields capitalized (`e.Add`, `e.Item`, `e.Depth`, `e.Parent`, `e.Sel()`); `b.cs.collapsed[…]` → `b.cs.Collapsed[…]`; `b.filteredUnder(…)` → `b.FilteredUnder(…)`; `b.panel != nil && b.panel.sel == e.sel()` → `b.Panel != nil && b.Panel.Sel == e.Sel()`.
  - `panelView` and `panelWindow`: `p := &b.pv`; `r.sel == p.sel` → `r.sel == b.Panel.Sel`.
  - `panelKey`: `sel := b.Panel.Sel`; Esc/Ctrl+C/panel key → `b.TogglePanel()` (it's open, so this closes it) and `return nil`; up/down → `b.MovePanel(∓1); b.pv.follow = true`; `h` → `if !sel.Add { b.ToggleHide(sel.Item) }`; `o` → `if !sel.Add { b.PressOnly(sel.Item) }`; left/right → `b.SetCollapsed(sel, true/false)`; enter → as now; `x`/delete → `if b.RemoveText(sel) { b.pv.follow = true }`. It still ends `b.Refilter()`.
  - `panelClick`: `es := b.Entries()`; `prAdd` → `b.Select(e.Sel())`; text glyph → `b.Select(e.Sel()); if b.RemoveText(e.Sel()) { b.pv.follow = true }; b.Refilter(); return`; parent glyph → `b.SetCollapsed(e.Sel(), !b.cs.Collapsed[e.Item.Name])`; name → `b.Select(e.Sel())`; buttons → `b.ToggleHide(e.Item)` / `b.PressOnly(e.Item)`, then `b.Select(e.Sel())`.
  - `panelScroll`: `b.pv.top`.

  `internal/ui/browse.go`: the `panel` field goes; add `pv filterPanel // the filter panel's scroll, while it's open`. `b.panel != nil` → `b.Panel != nil` (in `key`, `view`). The text prompt's Enter:

  ```go
		case promptFilterText:
			if b.AddText(v) {
				b.pv.follow = true
			}
			b.Refilter()
  ```

  `internal/ui/model.go`: delete `collapsed` and `foldSeen` from `charState`; `cs.browse.panel != nil` → `cs.browse.Panel != nil`. `internal/ui/view.go`: `b.panel != nil` / `cs.browse.panel != nil` → `.Panel != nil`.

- [ ] **Step 5: Build** — `go build ./...`; fix what's left the same way.

- [ ] **Step 6: Tests reach the core** —

```bash
grep -nE '\.panel\b|\.panel\.(sel|top|follow)|cs\.(collapsed|foldSeen)|filterSel\{|\.entriesOf\(|\.entries\(\)|\.setCollapsed\(|\.sel\.(item|add)|\be\.(item|add|depth|parent)\b' internal/ui/*_test.go
```

Access paths only: `.panel.sel` → `.Panel.Sel`, `.panel.top` → `.pv.top`, `.panel` (nil checks) → `.Panel`, `cs.collapsed` → `cs.Collapsed`, `filterSel{item: x}` → `app.FilterSel{Item: x}`, `.entriesOf(` → `.EntriesOf(`, `.entries()` → `.Entries()`, `.setCollapsed(` → `.SetCollapsed(`, `.sel.item`/`.sel.add` → `.Sel.Item`/`.Sel.Add` (and `r.sel.item` on a `panelRow` → `r.sel.Item`), entry fields capitalized.

- [ ] **Step 7: Run everything** (see Global Constraints). Expected: PASS, golden screens untouched (`git status internal/ui/testdata` clean).

- [ ] **Step 8: Commit**

```bash
git add internal/app internal/ui
git commit -m "app: the filter panel's entries, folds and selection live in the core"
```

Then: fresh-reviewer pass over the branch against `main` with this plan's Review Focus and side-effect list, fix Critical/Important with a failing test first, open the PR (no `Strings:` trailer: no catalog changes) and stop.
