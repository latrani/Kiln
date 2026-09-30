# Log Filter Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace log mode's numbered tri-state tag chips with a Filter panel in the sidebar: every tag (nested, collapsible) and every added text term gets a row with Hide and Only buttons.

**Architecture:** A new `scene.Filter` holds the state (Hide flags, the one Only, text terms) and decides visibility; it lives on `charState` so it outlasts a log-mode session. `browse` asks it which lines show. A new `filterPanel` on `browse` draws the panel into the sidebar column and handles its keys and clicks, the way the picker does for `Ctrl+O`.

**Tech Stack:** Go, Bubble Tea v2 (`charm.land/bubbletea/v2`), `github.com/charmbracelet/x/ansi`, Kiln's `internal/str` catalog and `internal/theme` roles.

**Spec:** `docs/superpowers/specs/2026-09-29-log-filter-panel-design.md`

## Global Constraints

- Every string a person reads goes in `internal/str/locales/en.toml`; run `go generate ./internal/str` and call the generated function. `{n}` is an int, `{err}` an error, everything else `any`. `go test ./internal/str` fails on stray literals; mark a literal `//str:ok` only when nobody reads it.
- A commit that adds or changes catalog entries ends with a `Strings:` trailer listing the keys, e.g. `Strings: filter.hide, filter.only (new)`.
- Tests build expected text from the catalog (`str.FilterHide()`), never a copy of it. `TestTestsReadTheCatalog` flags copied phrases.
- Every color comes from a theme role: `theme.Paint(role, s)`; painted areas use `theme.Fill(role, row, w)`. A new role goes in `internal/theme/roles.go` (the const block and `Roles`, each after its parent), `internal/theme/default.toml`, and the README's role list. `TestNoHardCodedStyles` fails on escape codes in `internal/ui`.
- Glyphs are one cell (East Asian Width not W/F) and preferably WGL4: `▼` (U+25BC), `►` (U+25BA), `•` (U+2022), `▲` (U+25B2), `…` (U+2026).
- Golden screens (`internal/ui/testdata/golden`) are updated with `go test ./internal/ui -run TestGolden -update` only where the look is meant to change; say so in the commit.
- Commits end with:
  ```
  Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_012rcJ1xqoQxzWAqkGanid5J
  ```
- Test fixtures use the characters Kit, Rook, Ash, Mira, Sable and world `fm`. No real people's names.
- Run the whole suite with `go test ./...` and `go vet ./...` before each commit.

## Review Focus

1. **Only on a text row with tags hidden.** "Only `lighthouse`" must show every lighthouse line whatever its tags, except tags the user hid themself. It must not hide every tag (Task 1 test `only text keeps tag hides separate`).
2. **Tags arriving later.** Live lines and older history paging in bring new tags. Under a hidden parent, or while a tag Only is set, the new tag's Hide button must light and its lines must hide. With nothing set, it just shows (Task 1 `Sync` tests; Task 2 `TestFilterNewTagUnderOnlyArrivesHidden`).
3. **The cursor line gets hidden.** Hiding the tag of the line under the cursor must move the cursor to the nearest visible line, not leave it on an invisible one (Task 2 `TestBrowseHidingCursorLineKeepsPlace`).
4. **Clicks while a prompt is open** (export format, find, the new text prompt) must not change filters. Today's chips guard this, and the panel must too (Task 4 `TestPanelClicksIgnoredDuringPrompt`).
5. **Many tags on a short screen.** The panel scrolls with `▲ N more` / `▼ N more`, keeps the selected item in view, and the wheel scrolls it (Task 4 `TestPanelScrolls`).

---

## File Structure

- **Create `internal/scene/filter.go`:** `Item`, `Filter`, `TagItems`, `Parent`. Pure logic, no UI.
- **Create `internal/scene/filter_test.go`:** table tests for it.
- **Modify `internal/scene/scene.go`:** delete `Chip`, `Neutral`/`Only`/`Hide`, `Next`, `Visible`.
- **Modify `internal/scene/scene_test.go`:** delete `TestVisible`.
- **Modify `internal/ui/model.go`:** `charState` gains `filter scene.Filter` and `collapsed map[string]bool`; `browseBodyH` becomes `height-5`; sidebar clicks and wheel route to the panel.
- **Modify `internal/ui/browse.go`:** the chips go; `bline.lower`; `items`, `shows`, `refilter`; one-row header with the Filter chip; `f` opens the panel; the new text prompt.
- **Create `internal/ui/filter.go`:** `filterPanel`, its rows, drawing, keys and clicks.
- **Create `internal/ui/filter_test.go`:** panel tests.
- **Modify `internal/ui/view.go`:** draw the panel in the sidebar column.
- **Modify `internal/ui/keys.go`:** drop `glyphChipOnly`/`glyphChipHide`; add `openFilterKey` and panel glyphs.
- **Modify `internal/theme/roles.go`, `internal/theme/default.toml`, `README.md`:** new roles and docs.
- **Modify `internal/str/locales/en.toml`:** `[filter]` section; `browse.hints`; delete `browse.tags`.

---

### Task 1: The filter model

**Files:**
- Create: `internal/scene/filter.go`
- Test: `internal/scene/filter_test.go`

**Interfaces:**
- Produces:
  - `type Item struct { Name string; Text bool }`: a tag (`Text` false) or a text term.
  - `func Parent(tag string) string`: `"page/in"` gives `"page"`, `"page"` gives `""`.
  - `func TagItems(tags []string) []Item`: every tag plus all their parents, deduplicated, in tree order.
  - `func (f *Filter) Hidden(it Item) bool`
  - `func (f *Filter) Only() (Item, bool)`
  - `func (f *Filter) Texts() []Item`, in the order added.
  - `func (f *Filter) Active() bool`
  - `func (f *Filter) Sync(items []Item)`
  - `func (f *Filter) ToggleHide(it Item, items []Item)`
  - `func (f *Filter) PressOnly(it Item, items []Item)`
  - `func (f *Filter) AddText(term string, items []Item) bool`
  - `func (f *Filter) RemoveText(term string)`
  - `func (f *Filter) Visible(tags []string, lower string) bool`, where `lower` is the line's plain text, lowercased.
  - The zero `Filter` is ready to use.

Semantics, from the spec as amended:
- Hiding a tag hides its descendants too. Unhiding a tag unhides its descendants and every ancestor. Unhiding anything clears the Only.
- `PressOnly` sets Hide on every other item **of the same kind** (tags or texts) that isn't an ancestor or descendant. It clears Hide on the pressed item and on the same-kind items it doesn't hide. The other kind's Hide flags stay.
- `PressOnly` on the item that already has Only clears the Only and every Hide of its kind.
- `ToggleHide` on the item that has Only clears the Only and every Hide of its kind, then hides just that item (and its descendants).
- `Sync` hides items it hasn't seen before when an ancestor is hidden, or when a same-kind Only is set that they aren't related to.

- [ ] **Step 1: Write the failing tests**

`internal/scene/filter_test.go`:

```go
package scene

import (
	"slices"
	"testing"
)

func tag(n string) Item  { return Item{Name: n} }
func text(n string) Item { return Item{Name: n, Text: true} }

func TestTagItemsAddsParentsInTreeOrder(t *testing.T) {
	got := TagItems([]string{"self", "page/out", "page-x", "page/in", "page/in"})
	want := []Item{tag("page"), tag("page/in"), tag("page/out"), tag("page-x"), tag("self")}
	if !slices.Equal(got, want) {
		t.Errorf("TagItems = %v, want %v", got, want)
	}
}

func TestParent(t *testing.T) {
	for in, want := range map[string]string{"page/in": "page", "a/b/c": "a/b", "page": ""} {
		if got := Parent(in); got != want {
			t.Errorf("Parent(%q) = %q, want %q", in, got, want)
		}
	}
}

// items is the panel's list for these tests: page, page/in, page/out,
// self, and the text row "lighthouse".
func items() []Item {
	return append(TagItems([]string{"page/in", "page/out", "self"}), text("lighthouse"))
}

func hiddenSet(f *Filter, its []Item) []string {
	var out []string
	for _, it := range its {
		if f.Hidden(it) {
			out = append(out, it.Name)
		}
	}
	return out
}

func TestHideParentHidesChildren(t *testing.T) {
	var f Filter
	its := items()
	f.ToggleHide(tag("page"), its)
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"page", "page/in", "page/out"}) {
		t.Errorf("hidden = %v", got)
	}
	f.ToggleHide(tag("page/in"), its) // unhide a child: parent clears, sibling stays
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"page/out"}) {
		t.Errorf("after unhiding page/in, hidden = %v", got)
	}
	f.ToggleHide(tag("page"), its) // hide the parent again, then unhide it
	f.ToggleHide(tag("page"), its)
	if got := hiddenSet(&f, its); len(got) != 0 {
		t.Errorf("unhiding the parent should unhide its children, hidden = %v", got)
	}
}

func TestOnlyHidesOthersOfItsKind(t *testing.T) {
	var f Filter
	its := items()
	f.ToggleHide(text("lighthouse"), its)
	f.PressOnly(tag("page/in"), its)
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"page/out", "self", "lighthouse"}) {
		t.Errorf("hidden = %v (page is page/in's parent; lighthouse is the other kind)", got)
	}
	if o, ok := f.Only(); !ok || o != tag("page/in") {
		t.Errorf("Only = %v, %v", o, ok)
	}
}

func TestOnlyAgainClearsItsKind(t *testing.T) {
	var f Filter
	its := items()
	f.ToggleHide(text("lighthouse"), its)
	f.PressOnly(tag("self"), its)
	f.PressOnly(tag("self"), its)
	if _, ok := f.Only(); ok {
		t.Error("Only still set")
	}
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"lighthouse"}) {
		t.Errorf("hidden = %v, want the text hide kept", got)
	}
}

func TestOnlyMovesToASecondItem(t *testing.T) {
	var f Filter
	its := items()
	f.PressOnly(tag("self"), its)
	f.PressOnly(tag("page"), its)
	if o, _ := f.Only(); o != tag("page") {
		t.Errorf("Only = %v", o)
	}
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"self"}) {
		t.Errorf("hidden = %v", got)
	}
}

func TestHideOnTheOnlyItemFlipsIt(t *testing.T) {
	var f Filter
	its := items()
	f.PressOnly(tag("page"), its)
	f.ToggleHide(tag("page"), its)
	if _, ok := f.Only(); ok {
		t.Error("Only still set")
	}
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"page", "page/in", "page/out"}) {
		t.Errorf("hidden = %v", got)
	}
}

func TestUnhidingClearsOnly(t *testing.T) {
	var f Filter
	its := items()
	f.PressOnly(tag("self"), its)
	f.ToggleHide(tag("page/in"), its)
	if _, ok := f.Only(); ok {
		t.Error("Only still set after unhiding page/in")
	}
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"page/out"}) {
		t.Errorf("hidden = %v", got)
	}
}

func TestSyncHidesNewcomers(t *testing.T) {
	var f Filter
	its := items()
	f.Sync(its)
	f.ToggleHide(tag("page"), its)
	more := append(TagItems([]string{"page/in", "page/out", "page/x", "say", "self"}), text("lighthouse"))
	f.Sync(more)
	if !f.Hidden(tag("page/x")) || f.Hidden(tag("say")) {
		t.Errorf("under a hidden parent: page/x %v, say %v", f.Hidden(tag("page/x")), f.Hidden(tag("say")))
	}

	var g Filter
	g.Sync(its)
	g.PressOnly(tag("self"), its)
	g.Sync(more)
	if !g.Hidden(tag("say")) || !g.Hidden(tag("page/x")) {
		t.Error("tags arriving under an Only should arrive hidden")
	}
}

func TestTexts(t *testing.T) {
	var f Filter
	its := items()[:4] // tags only
	if !f.AddText("lighthouse", its) || f.AddText("lighthouse", its) || f.AddText("  ", its) {
		t.Fatal("AddText: want true, then false for a duplicate and a blank")
	}
	f.AddText("Rook", its)
	if got := f.Texts(); !slices.Equal(got, []Item{text("lighthouse"), text("Rook")}) {
		t.Errorf("Texts = %v", got)
	}
	if o, _ := f.Only(); o != text("Rook") || !f.Hidden(text("lighthouse")) {
		t.Errorf("a new text row takes Only: Only %v, lighthouse hidden %v", o, f.Hidden(text("lighthouse")))
	}
	f.RemoveText("Rook")
	if _, ok := f.Only(); ok || len(f.Texts()) != 1 || !f.Hidden(text("lighthouse")) {
		t.Errorf("RemoveText: Only cleared, the Hide it set kept")
	}
}

func TestVisible(t *testing.T) {
	its := items()
	cases := []struct {
		name  string
		set   func(f *Filter)
		tags  []string
		lower string
		want  bool
	}{
		{"nothing set", func(*Filter) {}, []string{"page/in"}, "", true},
		{"hidden tag", func(f *Filter) { f.ToggleHide(tag("self"), its) }, []string{"self"}, "", false},
		{"untagged survives hide", func(f *Filter) { f.ToggleHide(tag("self"), its) }, nil, "", true},
		{"hidden ancestor, tag not synced", func(f *Filter) { f.ToggleHide(tag("page"), its) }, []string{"page/new"}, "", false},
		{"only: has it", func(f *Filter) { f.PressOnly(tag("page/in"), its) }, []string{"page", "page/in"}, "", true},
		{"only: parent covers child", func(f *Filter) { f.PressOnly(tag("page"), its) }, []string{"page/out"}, "", true},
		{"only: untagged hidden", func(f *Filter) { f.PressOnly(tag("self"), its) }, nil, "", false},
		{"hide beats only", func(f *Filter) {
			f.PressOnly(tag("page"), its)
			f.ToggleHide(text("lighthouse"), its)
		}, []string{"page"}, "the lighthouse", false},
		{"hidden text", func(f *Filter) { f.ToggleHide(text("lighthouse"), its) }, nil, "the lighthouse is dark", false},
		{"text match ignores case", func(f *Filter) { f.AddText("LightHouse", its) }, []string{"self"}, "the lighthouse is dark", true},
		{"only text keeps tag hides separate", func(f *Filter) {
			f.ToggleHide(tag("self"), its)
			f.PressOnly(text("lighthouse"), its)
		}, []string{"page"}, "the lighthouse", true},
		{"only text, tag hidden", func(f *Filter) {
			f.ToggleHide(tag("self"), its)
			f.PressOnly(text("lighthouse"), its)
		}, []string{"self"}, "the lighthouse", false},
		{"only text: no match", func(f *Filter) { f.PressOnly(text("lighthouse"), its) }, nil, "evening", false},
	}
	for _, c := range cases {
		var f Filter
		c.set(&f)
		if got := f.Visible(c.tags, c.lower); got != c.want {
			t.Errorf("%s: Visible = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestActive(t *testing.T) {
	var f Filter
	if f.Active() {
		t.Error("zero filter active")
	}
	f.ToggleHide(tag("self"), items())
	if !f.Active() {
		t.Error("a hide should make it active")
	}
}
```

- [ ] **Step 2: Run the tests and watch them fail**

Run: `go test ./internal/scene -run 'TestTagItems|TestParent|TestHide|TestOnly|TestUnhiding|TestSync|TestTexts|TestVisible|TestActive'`
Expected: build failure: `undefined: TagItems`, `undefined: Filter` (and `TestVisible redeclared`, since the old one is still in `scene_test.go`). Delete the old `TestVisible` from `internal/scene/scene_test.go` now (lines 12–35, the `chips map[string]Chip` table) so only the undefined names remain.

- [ ] **Step 3: Write the implementation**

`internal/scene/filter.go`:

```go
package scene

import (
	"slices"
	"strings"
)

// Item is a row in log mode's filter panel: a tag, or a term a line's
// text must contain.
type Item struct {
	Name string // the tag, or the term as typed
	Text bool
}

// Parent is tag's parent up its slashes, or "" at the top.
func Parent(tag string) string {
	i := strings.LastIndex(tag, "/")
	if i < 0 {
		return ""
	}
	return tag[:i]
}

// TagItems is every tag in tags and every parent up their slashes, once
// each, parents before children and siblings alphabetically.
func TagItems(tags []string) []Item {
	seen := map[string]bool{}
	var names []string
	for _, t := range tags {
		for n := t; n != "" && !seen[n]; n = Parent(n) {
			seen[n] = true
			names = append(names, n)
		}
	}
	slices.SortFunc(names, func(a, b string) int {
		return slices.Compare(strings.Split(a, "/"), strings.Split(b, "/"))
	})
	out := make([]Item, len(names))
	for i, n := range names {
		out[i] = Item{Name: n}
	}
	return out
}

// under reports whether b is a descendant of a (tags only).
func under(a, b Item) bool {
	return !a.Text && !b.Text && strings.HasPrefix(b.Name, a.Name+"/")
}

// related reports whether a and b are the same item, or one is under
// the other.
func related(a, b Item) bool { return a == b || under(a, b) || under(b, a) }

// Filter is log mode's filter: a Hide flag per item, at most one item
// with Only, and the text terms. Its zero value filters nothing. Hiding
// a tag hides its descendants; flags are always the item's own, so the
// panel shows exactly what is hidden.
type Filter struct {
	hidden  map[Item]bool // only true entries
	only    Item
	hasOnly bool
	texts   []string
	known   map[Item]bool // items Sync has seen
}

func (f *Filter) init() {
	if f.hidden == nil {
		f.hidden, f.known = map[Item]bool{}, map[Item]bool{}
	}
}

// Hidden reports it's Hide flag.
func (f *Filter) Hidden(it Item) bool { return f.hidden[it] }

// Only is the item with Only, if any.
func (f *Filter) Only() (Item, bool) { return f.only, f.hasOnly }

// Texts is the text rows, in the order added.
func (f *Filter) Texts() []Item {
	out := make([]Item, len(f.texts))
	for i, t := range f.texts {
		out[i] = Item{Name: t, Text: true}
	}
	return out
}

// Active reports whether anything is hidden or has Only.
func (f *Filter) Active() bool { return len(f.hidden) > 0 || f.hasOnly }

// Sync takes the current items. One it hasn't seen before is hidden when
// an ancestor is, or when an Only of its kind is set that it isn't
// related to, so a tag arriving late obeys the filter already set.
func (f *Filter) Sync(items []Item) {
	f.init()
	for _, it := range items {
		if f.known[it] {
			continue
		}
		f.known[it] = true
		if !it.Text && f.hidden[Item{Name: Parent(it.Name)}] {
			f.hidden[it] = true
		} else if f.hasOnly && f.only.Text == it.Text && !related(f.only, it) {
			f.hidden[it] = true
		}
	}
}

// hide sets Hide on it and its descendants.
func (f *Filter) hide(it Item, items []Item) {
	for _, o := range items {
		if o == it || under(it, o) {
			f.hidden[o] = true
		}
	}
	f.hidden[it] = true
}

// clearKind drops the Only and every Hide of text's kind.
func (f *Filter) clearKind(text bool) {
	f.hasOnly = false
	for it := range f.hidden {
		if it.Text == text {
			delete(f.hidden, it)
		}
	}
}

// ToggleHide is the Hide button. On the item with Only it clears that
// kind's filter and hides just it. Unhiding clears the Only, the item's
// descendants and its ancestors.
func (f *Filter) ToggleHide(it Item, items []Item) {
	f.init()
	switch {
	case f.hasOnly && f.only == it:
		f.clearKind(it.Text)
		f.hide(it, items)
	case f.hidden[it]:
		f.hasOnly = false
		for _, o := range items {
			if under(it, o) {
				delete(f.hidden, o)
			}
		}
		delete(f.hidden, it)
		if !it.Text {
			for p := Parent(it.Name); p != ""; p = Parent(p) {
				delete(f.hidden, Item{Name: p})
			}
		}
	default:
		f.hide(it, items)
	}
}

// PressOnly is the Only button: it gives it Only and hides every other
// item of its kind except its ancestors and descendants. Pressed on the
// item that has Only, it clears that kind's filter.
func (f *Filter) PressOnly(it Item, items []Item) {
	f.init()
	if f.hasOnly && f.only == it {
		f.clearKind(it.Text)
		return
	}
	f.clearKind(it.Text)
	f.only, f.hasOnly = it, true
	for _, o := range items {
		if o.Text == it.Text && !related(it, o) {
			f.hidden[o] = true
		}
	}
}

// AddText adds a text row with Only. A blank term or one already listed
// adds nothing and reports false.
func (f *Filter) AddText(term string, items []Item) bool {
	f.init()
	term = strings.TrimSpace(term)
	if term == "" || slices.Contains(f.texts, term) {
		return false
	}
	f.texts = append(f.texts, term)
	it := Item{Name: term, Text: true}
	f.known[it] = true
	f.PressOnly(it, append(slices.Clone(items), f.Texts()...))
	return true
}

// RemoveText drops a text row. If it had Only, the Only goes; the Hide
// flags it set stay.
func (f *Filter) RemoveText(term string) {
	f.init()
	it := Item{Name: term, Text: true}
	f.texts = slices.DeleteFunc(f.texts, func(t string) bool { return t == term })
	delete(f.hidden, it)
	delete(f.known, it)
	if f.hasOnly && f.only == it {
		f.hasOnly = false
	}
}

// Visible reports whether a line shows: none of its tags or their
// ancestors is hidden, it matches no hidden text row, and, with an Only
// set, it carries that tag (or one under it) or matches that text. lower
// is the line's plain text, lowercased.
func (f *Filter) Visible(tags []string, lower string) bool {
	for _, t := range tags {
		for n := t; n != ""; n = Parent(n) {
			if f.hidden[Item{Name: n}] {
				return false
			}
		}
	}
	for _, term := range f.texts {
		if f.hidden[Item{Name: term, Text: true}] && strings.Contains(lower, strings.ToLower(term)) {
			return false
		}
	}
	if !f.hasOnly {
		return true
	}
	if f.only.Text {
		return strings.Contains(lower, strings.ToLower(f.only.Name))
	}
	for _, t := range tags {
		if t == f.only.Name || strings.HasPrefix(t, f.only.Name+"/") {
			return true
		}
	}
	return false
}
```

Note on `AddText`: the `items` a caller passes are the tags and the texts *before* the new term. `PressOnly` needs the other texts to hide them, so it gets `items` plus `f.Texts()`. That can list a text twice, which is harmless because `hidden` is a set.

- [ ] **Step 4: Run the tests and watch them pass**

Run: `go test ./internal/scene`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/scene/filter.go internal/scene/filter_test.go internal/scene/scene_test.go
git commit -m "feat(scene): a Filter with Hide flags, one Only and text rows

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_012rcJ1xqoQxzWAqkGanid5J"
```

(`scene.Visible` and `Chip` stay until Task 2 so the build keeps passing. Deleting `TestVisible` only removes the old test.)

---

### Task 2: Log mode uses the Filter; one-row header with the Filter chip

**Files:**
- Modify: `internal/ui/model.go` (`charState`, `browseBodyH`)
- Modify: `internal/ui/browse.go`
- Modify: `internal/ui/keys.go:69-70` (delete the chip glyphs)
- Modify: `internal/scene/scene.go:20-51` (delete `Chip`, `Next`, `Visible`)
- Modify: `internal/str/locales/en.toml` (`[browse]`: delete `tags`, change `hints`; new `[filter]` with `title`)
- Modify: `README.md` (log-mode key table)
- Test: `internal/ui/browse_test.go`, `internal/ui/theme_test.go`, `internal/ui/golden_test.go`

**Interfaces:**
- Consumes: `scene.Filter`, `scene.TagItems`, `scene.Item` (Task 1).
- Produces:
  - `charState.filter scene.Filter`
  - `(b *browse) items() []scene.Item`: tag items from the loaded lines, then text rows. It calls `Sync`.
  - `(b *browse) shows(l *bline) bool`
  - `(b *browse) refilter()`: after any filter change, moves the cursor off a hidden line.
  - `browse.filterChip [2]int`: the chip's columns `[from, to)` on header row 0.
  - `bline.lower string`
  - `str.FilterTitle()`

- [ ] **Step 1: Catalog changes**

In `internal/str/locales/en.toml`, under `[browse]`, delete `tags = "tags:"` and change `hints`:

```toml
hints = "m mark · space exclude · / find · n/N · g date · f filter · t tags · e export · c copy · esc back"
```

After the `[browse]` block (after `tag_unstyled`), add:

```toml
# Log mode's filter panel, in the sidebar.
[filter]
title = "Filter"
```

Run: `go generate ./internal/str`

- [ ] **Step 2: Write the failing tests**

In `internal/ui/browse_test.go`, replace `TestBrowseChips`, `TestBrowseHidingCursorLineKeepsPlace`, `TestBrowseChipNumbersStayStable` and `TestChipClickDuringFormatPromptDoesNotCrash` with these. `kitFilter` is a helper:

```go
// kitFilter is fm/kit's log filter.
func (h *harness) kitFilter() *scene.Filter { return &h.m.chars["fm/kit"].filter }

func TestBrowseFilterHidesAndOnly(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	b := h.br()
	f := h.kitFilter()
	f.PressOnly(scene.Item{Name: "page"}, b.items())
	b.refilter()
	s := h.screen()
	if strings.Contains(s, "Sable") || !strings.Contains(s, "Mira pages") {
		t.Errorf("only page:\n%s", s)
	}
	f.PressOnly(scene.Item{Name: "page"}, b.items()) // clean slate
	f.ToggleHide(scene.Item{Name: "page"}, b.items())
	b.refilter()
	s = h.screen()
	if strings.Contains(s, "Mira pages") || !strings.Contains(s, "Sable") {
		t.Errorf("hide page:\n%s", s)
	}
}

func TestBrowseHidingCursorLineKeepsPlace(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.keys("home", "down", "down") // Mira pages
	b := h.br()
	f := h.kitFilter()
	if b.cursor.e.Text != "Mira pages: you around?" {
		t.Fatalf("cursor on %q", b.cursor.e.Text)
	}
	f.ToggleHide(scene.Item{Name: "page"}, b.items())
	b.refilter()
	if got := b.cursor.e.Text; got != "> :grins." && got != ":grins." {
		t.Errorf("cursor moved to %q, want the next line", got)
	}
	f.ToggleHide(scene.Item{Name: "page"}, b.items()) // shown again
	h.keys("end")                                     // Rook yawns, the last line
	f.PressOnly(scene.Item{Name: "page"}, b.items())
	b.refilter()
	if got := b.cursor.e.Text; got != "Mira pages: you around?" {
		t.Errorf("cursor moved to %q, want the previous visible line", got)
	}
}

func TestFilterNewTagUnderOnlyArrivesHidden(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, "Rook whispers, \"Hi, Kit.\"")
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.key("ctrl+l")
	b := h.br()
	f := h.kitFilter()
	f.PressOnly(scene.Item{Name: "whisper"}, b.items())
	h.conn("fm/kit").lines <- "Mira pages: you around?"
	h.settle("fm/kit", func() bool { return slices.Contains(b.items(), scene.Item{Name: "page"}) })
	if !f.Hidden(scene.Item{Name: "page"}) || strings.Contains(h.screen(), "Mira pages") {
		t.Errorf("a tag arriving under Only whisper should arrive hidden:\n%s", h.screen())
	}
}

func TestFilterOutlastsLogMode(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.kitFilter().ToggleHide(scene.Item{Name: "page"}, h.br().items())
	h.key("esc")
	h.key("ctrl+l")
	if strings.Contains(h.screen(), "Mira pages") {
		t.Errorf("filter lost on leaving log mode:\n%s", h.screen())
	}
}

func TestFilterChipOnHeaderRow(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	rows := strings.Split(h.screen(), "\n")
	head := strings.SplitN(rows[0], "│", 2)[1]
	if !strings.HasSuffix(strings.TrimRight(head, " "), " "+str.FilterTitle()) {
		t.Errorf("header row = %q, want the Filter chip at its end", head)
	}
	if strings.Contains(h.screen(), "1[") {
		t.Errorf("numbered chips still drawn:\n%s", h.screen())
	}
}
```

Add `"slices"` and `"github.com/latrani/Kiln/internal/scene"` to `browse_test.go`'s imports if they're missing.

In `TestBrowseMouse`, change `return i + 3` to `return i + 2` and delete the last block (the `chipSpans` click and its check).

In `internal/ui/theme_test.go`, in `TestLogModeUsesTheme`, replace `h.key("1")` with:

```go
	h.kitFilter().PressOnly(scene.Item{Name: "page"}, h.br().items()) // lights the Filter chip
```

and add the `scene` import.

In `internal/ui/golden_test.go`, in `TestGoldenLog`, replace `h.key("1")             // a chip on` with:

```go
	h.kitFilter().PressOnly(scene.Item{Name: "page"}, h.br().items()) // the Filter chip on
```

and add the `scene` import.

- [ ] **Step 3: Run the tests and watch them fail**

Run: `go test ./internal/ui 2>&1 | head -20`
Expected: build failure: `h.m.chars["fm/kit"].filter undefined`, `b.items undefined`, `b.refilter undefined`, `str.FilterTitle` exists (from Step 1).

- [ ] **Step 4: Implement**

`internal/ui/model.go`, in `charState` after `browse`:

```go
	filter      scene.Filter     // log mode's filter; outlasts a log-mode session
```

(import `github.com/latrani/Kiln/internal/scene` if it isn't already). Change `browseBodyH`:

```go
// browseBodyH is the number of line rows in browse mode: the pane less
// the header row, two rules, the action bar and the statusline.
func (m *Model) browseBodyH() int { return max(1, m.height-5) }
```

`internal/ui/browse.go`:

1. In `bline`, add after `text`:
   ```go
   	lower string // plain text, lowercased, for text filters
   ```
   and in `makeLine` set it: `return &bline{e: e, tags: tags, text: text, lower: strings.ToLower(plain), day: ...}`.
2. In `browse`, delete `chips`, `tagOrder`, `chipSpans`, and add:
   ```go
   	filterChip   [2]int // columns [from, to) of the Filter chip on header row 0
   ```
   Delete the `chipSpan` type. In `newBrowse`, drop `chips: map[string]scene.Chip{}` and the `b.tagList()` call.
3. Replace `tagList` with:
   ```go
   // items is the filter panel's list: every tag on a loaded line with its
   // parents, then the text rows. It syncs the filter, so a tag seen for
   // the first time obeys the filter already set.
   func (b *browse) items() []scene.Item {
   	var tags []string
   	for _, l := range b.lines {
   		tags = append(tags, l.tags...)
   	}
   	f := &b.cs.filter
   	items := append(scene.TagItems(tags), f.Texts()...)
   	f.Sync(items)
   	return items
   }

   // shows reports whether the filter lets l through.
   func (b *browse) shows(l *bline) bool { return b.cs.filter.Visible(l.tags, l.lower) }
   ```
4. In `visible()` and `nearestVisible`, replace `scene.Visible(x.tags, b.chips)` with `b.shows(x)`. In `selection()` (line ~469), replace `scene.Visible(l.tags, b.chips)` with `b.shows(l)`.
5. Replace `cycleChip` with:
   ```go
   // refilter keeps the reader's place after the filter changes: a cursor
   // on a line now hidden moves to the nearest visible one.
   func (b *browse) refilter() {
   	b.items() // sync
   	if b.cursor != nil && !b.shows(b.cursor) {
   		if n := b.nearestVisible(b.cursor); n != nil {
   			b.cursor = n
   		}
   	}
   }
   ```
6. In `key`, delete the `1`–`9` block.
7. Live lines and paged history must sync, so a newly seen tag gets its flag before it's drawn. In `view`, call `b.items()` first (the line after `func (b *browse) view(...) {`), then `v := b.visible()`:
   ```go
   	b.items() // new tags take the filter already set
   	v := b.visible()
   	bodyH := max(1, h-4)
   ```
8. Replace the header block (from `// Header row 1.` down to and including the `rows = []string{...}` line) with:
   ```go
   	// Header: title and span, the Filter chip at the right end.
   	span := str.BrowseNoLogs()
   	if len(b.lines) > 0 {
   		span = str.BrowseToToday(dayLabel(b.lines[0].day))
   		if !b.histDone {
   			span = "…" + span
   		}
   	}
   	head := theme.Paint(theme.LogTitle, str.BrowseLog()+" "+b.cs.ch.Name) + str.Separator() + span
   	if b.find != "" {
   		i, n := b.matchPos()
   		head += "   " + str.BrowseFindStatus(b.find, i, n)
   	}
   	role := theme.LogChip
   	if b.cs.filter.Active() {
   		role = theme.LogChipOn
   	}
   	chipText := chip(role, str.FilterTitle())
   	cw := xansi.StringWidth(chipText)
   	b.filterChip = [2]int{w - cw, w}
   	if w-cw-1 < 1 {
   		b.filterChip = [2]int{}
   		cw = 0
   		chipText = ""
   	}
   	head = fitName(head, w-cw-1) + " " + chipText
   	rows = []string{theme.Fill(theme.LogHeader, head, w), theme.Paint(theme.Rule, strings.Repeat("─", w))}
   ```
   (`fitName` pads to width and cuts with `…`. Its `w-cw-1` plus one space plus the chip fills the row exactly.)
9. In `click`, delete the `if y == 1 { ... chipSpans ... }` block and change `row := y - 3` to `row := y - 2`. (Task 3 wires the chip click.)

`internal/ui/keys.go`: delete `glyphChipOnly` and `glyphChipHide` (and the `const (` group if it becomes empty).

`internal/scene/scene.go`: delete lines 20–51 (`Chip` through `Visible`). Delete `"slices"` from its imports if nothing else uses it.

`README.md`, in the log-mode key table, replace the `1`–`9` row with:

```markdown
| `f`, or click ` Filter ` | Open the filter panel (see below) |
```

(Task 5 adds the panel's section.)

- [ ] **Step 5: Run the tests**

Run: `go test ./... 2>&1 | grep -v '^ok\|no test files'`
Expected: only the golden screens fail (`TestGoldenLog` and friends: the header is one row now). Update them, and check that only log mode changed:

Run: `go test ./internal/ui -run TestGolden -update && git diff --stat internal/ui/testdata`
Expected: only `log.txt` changes. `git diff internal/ui/testdata/golden/log.txt` shows the chip row gone, ` Filter ` at the end of row 0, and one more body line.

Run: `go test ./... && go vet ./...`
Expected: all `ok`.

- [ ] **Step 6: Commit**

```bash
git add -A internal README.md
git commit -m "feat(log): filter through scene.Filter; one header row with a Filter chip

The numbered tri-state chips and their 1-9 keys go. Filters live on the
character, so they outlast a log-mode session. The log golden changes on
purpose: one header row, the Filter chip, one more body line.

Strings: browse.hints, browse.tags (removed), filter.title (new)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_012rcJ1xqoQxzWAqkGanid5J"
```

---

### Task 3: The panel: roles, drawing, opening and closing

**Files:**
- Create: `internal/ui/filter.go`
- Create: `internal/ui/filter_test.go`
- Modify: `internal/theme/roles.go`, `internal/theme/default.toml`, `README.md` (role list)
- Modify: `internal/str/locales/en.toml` (`[filter]`)
- Modify: `internal/ui/keys.go` (`openFilterKey`, glyphs)
- Modify: `internal/ui/browse.go` (`panel` field, `f` key, chip click, `Esc`)
- Modify: `internal/ui/view.go` (draw the panel in the sidebar column)
- Modify: `internal/ui/model.go` (`collapsed` on `charState`)

**Interfaces:**
- Consumes: `browse.items()`, `charState.filter`, `browse.filterChip` (Task 2).
- Produces:
  - `type filterPanel struct { sel filterSel; top int }` and `type filterSel struct { item scene.Item; add bool }`
  - `browse.panel *filterPanel`, non-nil while the panel is open
  - `(b *browse) togglePanel()`
  - `type panelEntry struct { item scene.Item; add bool; depth int; parent bool }`
  - `(b *browse) entries() []panelEntry`: the rows the panel lists, with collapsed children left out
  - `type panelRow struct { text string; entry int; kind panelRowKind }`, where `panelRowKind` is `prTitle`, `prName`, `prButtons`, `prBlank` or `prAdd`
  - `(b *browse) panelRows(w int) []panelRow`
  - `(b *browse) panelView(w, h int) []string`
  - `charState.collapsed map[string]bool`
  - Theme roles `theme.Filter`, `FilterItem`, `FilterSelected`, `FilterButton`, `FilterButtonOn`, `FilterAdd`, `FilterMore`
  - Strings `str.FilterHide()`, `str.FilterOnly()`, `str.FilterAddText()`, `str.FilterTextItem(term)`

- [ ] **Step 1: Roles, theme and strings**

`internal/theme/roles.go`, after the `Log...` block and before `Export`:

```go
	Filter         Role = "filter" // area, in place of sidebar while log mode's filter panel is open
	FilterItem     Role = "filter.item"
	FilterSelected Role = "filter.selected"
	FilterButton   Role = "filter.button"
	FilterButtonOn Role = "filter.button.on"
	FilterAdd      Role = "filter.add"
	FilterMore     Role = "filter.more"
```

and in `Roles`, before `Export,`:

```go
	Filter, FilterItem, FilterSelected, FilterButton, FilterButtonOn, FilterAdd, FilterMore,
```

`internal/theme/default.toml`, after the `log.*` block:

```toml
"filter"           = { fg = "cobalt-ink" }
"filter.item"      = { bold = true }
"filter.selected"  = { reverse = true }
"filter.button"    = { fg = "cobalt-mid" }
"filter.button.on" = { fg = "cobalt-bright", bg = "cobalt-deep", bold = true }
"filter.add"       = { fg = "cobalt-bright", bg = "cobalt-deep", bold = true }
"filter.more"      = { fg = "cobalt-mid" }
```

`README.md` role list (the `The roles are:` paragraph): after the `log (...)` entry add ``, `filter` (`.item`, `.selected`, `.button`, `.button.on`, `.add`, `.more`)``.

`internal/str/locales/en.toml`, `[filter]` becomes:

```toml
# Log mode's filter panel, in the sidebar.
[filter]
title = "Filter"
hide = "Hide"
only = "Only"
add_text = "+ Text"
# A text row's name in the panel.
text_item = "\"{term}\""
```

Run: `go generate ./internal/str`

- [ ] **Step 2: Write the failing tests**

`internal/ui/filter_test.go`:

```go
package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/str"
)

// panelSide is the sidebar column's rows, trimmed.
func panelSide(h *harness) []string {
	var out []string
	for _, row := range strings.Split(h.screen(), "\n") {
		out = append(out, strings.TrimSpace(strings.SplitN(row, "│", 2)[0]))
	}
	return out
}

func buttons() string { return str.FilterHide() + " " + str.FilterOnly() }

func TestPanelOpensAndCloses(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	side := panelSide(h)
	want := []string{str.FilterTitle(), "", "▼ page", buttons(), "in", buttons(), "", "self", buttons(), "", str.FilterAddText()}
	for i, w := range want {
		if side[i] != w {
			t.Fatalf("panel row %d = %q, want %q:\n%s", i, side[i], w, h.screen())
		}
	}
	h.key("f")
	if h.br().panel != nil || panelSide(h)[0] != "fm" {
		t.Errorf("f should close the panel:\n%s", h.screen())
	}
	h.key("f")
	h.key("esc")
	if h.br() == nil || h.br().panel != nil {
		t.Error("Esc should close the panel and stay in log mode")
	}
	// The chip toggles it too.
	l := h.m.layout()
	h.screen()
	h.m.Update(tea.MouseClickMsg{X: l.sw + 1 + h.br().filterChip[0] + 1, Y: 0, Button: tea.MouseLeft})
	if h.br().panel == nil {
		t.Fatal("clicking the Filter chip should open the panel")
	}
	h.m.Update(tea.MouseClickMsg{X: l.sw + 1 + h.br().filterChip[0] + 1, Y: 0, Button: tea.MouseLeft})
	if h.br().panel != nil {
		t.Error("clicking it again should close the panel")
	}
}

func TestPanelLightsTheItemsOwnState(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	h.kitFilter().PressOnly(scene.Item{Name: "self"}, h.br().items())
	s := h.drawn()
	on := func(label string) string { return theme.Paint(theme.FilterButtonOn, label) }
	if strings.Count(s, on(str.FilterHide())) != 2 || strings.Count(s, on(str.FilterOnly())) != 1 {
		t.Errorf("want Hide lit on page and page/in, Only on self:\n%q", s)
	}
}
```

`scene1` gives the tags `page`, `page/in` and `self` (see `TestGoldenLog`), so `page` is a parent with one child, `in`. Add the `scene` and `theme` imports.

- [ ] **Step 3: Run the tests and watch them fail**

Run: `go test ./internal/ui -run 'TestPanel' 2>&1 | head`
Expected: build failure: `h.br().panel undefined`.

- [ ] **Step 4: Implement**

`internal/ui/keys.go`, add:

```go
// openFilterKey opens and closes log mode's filter panel.
const openFilterKey = "f"

// Filter panel glyphs: a parent's disclosure, and the mark on a
// collapsed parent with a filter set under it.
const (
	glyphOpen      = "▼"
	glyphCollapsed = "►"
	glyphFiltered  = "•"
)
```

`internal/ui/model.go`, in `charState` after `filter`:

```go
	collapsed   map[string]bool  // filter panel parents folded shut, by tag
```

`internal/ui/filter.go`:

```go
package ui

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/scene"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/theme"
)

// filterSel is the highlighted panel entry: an item, or the + Text row.
type filterSel struct {
	item scene.Item
	add  bool
}

// filterPanel is log mode's filter panel, open in the sidebar column.
type filterPanel struct {
	sel filterSel
	top int // first row drawn
}

// panelEntry is one item in the panel, or the + Text row.
type panelEntry struct {
	item   scene.Item
	add    bool
	depth  int  // slashes in a tag; 0 for text
	parent bool // a tag with children
}

func (e panelEntry) sel() filterSel { return filterSel{item: e.item, add: e.add} }

type panelRowKind int

const (
	prTitle panelRowKind = iota
	prName
	prButtons
	prBlank
	prAdd
)

type panelRow struct {
	text  string // drawn, not yet fitted
	entry int    // index into entries(); -1 for the title and its blank
	kind  panelRowKind
}

// togglePanel opens or closes the filter panel.
func (b *browse) togglePanel() {
	if b.panel != nil {
		b.panel = nil
		return
	}
	b.panel = &filterPanel{}
	if es := b.entries(); len(es) > 0 {
		b.panel.sel = es[0].sel()
	}
}

// entries is what the panel lists: tags (children of a collapsed parent
// left out), text rows, then + Text.
func (b *browse) entries() []panelEntry {
	items := b.items()
	var out []panelEntry
	for i, it := range items {
		if !it.Text && b.foldedAway(it.Name) {
			continue
		}
		e := panelEntry{item: it}
		if !it.Text {
			e.depth = strings.Count(it.Name, "/")
			e.parent = i+1 < len(items) && !items[i+1].Text && strings.HasPrefix(items[i+1].Name, it.Name+"/")
		}
		out = append(out, e)
	}
	return append(out, panelEntry{add: true})
}

// foldedAway reports whether tag sits under a collapsed parent.
func (b *browse) foldedAway(tag string) bool {
	for p := scene.Parent(tag); p != ""; p = scene.Parent(p) {
		if b.cs.collapsed[p] {
			return true
		}
	}
	return false
}

// filteredUnder reports whether any tag under parent is hidden or has Only.
func (b *browse) filteredUnder(parent string) bool {
	f := &b.cs.filter
	only, hasOnly := f.Only()
	for _, it := range b.items() {
		if !it.Text && strings.HasPrefix(it.Name, parent+"/") && (f.Hidden(it) || hasOnly && only == it) {
			return true
		}
	}
	return false
}

// panelRows lays the panel out: a title, then per entry its name, its
// Hide and Only buttons, and a blank row; + Text last.
func (b *browse) panelRows(w int) []panelRow {
	rows := []panelRow{{text: theme.Paint(theme.FilterItem, " "+str.FilterTitle()), entry: -1, kind: prTitle}, {entry: -1, kind: prBlank}}
	f := &b.cs.filter
	only, hasOnly := f.Only()
	for i, e := range b.entries() {
		if e.add {
			rows = append(rows, panelRow{text: " " + theme.Paint(theme.FilterAdd, str.FilterAddText()), entry: i, kind: prAdd})
			continue
		}
		indent := strings.Repeat(" ", 1+2*e.depth)
		name := e.item.Name
		if e.item.Text {
			name = str.FilterTextItem(name)
		} else if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		switch {
		case e.parent && b.cs.collapsed[e.item.Name]:
			name = glyphCollapsed + " " + name
			if b.filteredUnder(e.item.Name) {
				name += " " + glyphFiltered
			}
		case e.parent:
			name = glyphOpen + " " + name
		}
		role := theme.FilterItem
		if b.panel != nil && b.panel.sel == e.sel() {
			role = theme.FilterSelected
		}
		hide, onlyRole := theme.FilterButton, theme.FilterButton
		if f.Hidden(e.item) {
			hide = theme.FilterButtonOn
		}
		if hasOnly && only == e.item {
			onlyRole = theme.FilterButtonOn
		}
		rows = append(rows,
			panelRow{text: indent + theme.Paint(role, name), entry: i, kind: prName},
			panelRow{text: indent + " " + theme.Paint(hide, str.FilterHide()) + " " + theme.Paint(onlyRole, str.FilterOnly()), entry: i, kind: prButtons},
			panelRow{entry: i, kind: prBlank})
	}
	return rows
}
```

Rows aren't fitted here; `panelView` fits each one to the column, cutting with `…`. The `w` parameter is unused for now and kept so a later search field can size itself; drop it if the linter objects.

The buttons row has one more space of indent than the name, to match the spec's mock (`▼ page` / `   Hide Only`). Keep that alignment, because Task 4's clicks compute the button columns from it.

```go
// panelView draws the panel into a column w wide and h tall, scrolled so
// the selected entry's rows show; "▲ N more" / "▼ N more" replace the
// edge rows when it overflows, as in the sidebar.
func (b *browse) panelView(w, h int) []string {
	rows := b.panelRows(w)
	p := b.panel
	selRow := 0
	for i, r := range rows {
		if (r.kind == prName || r.kind == prAdd) && r.entry >= 0 && b.entries()[r.entry].sel() == p.sel {
			selRow = i
			break
		}
	}
	top, above, below, avail := b.panelWindow(len(rows), h, selRow)
	out := make([]string, 0, h)
	if above {
		out = append(out, theme.Paint(theme.FilterMore, fit(str.ViewMoreAbove(top), w)))
	}
	for i := top; i < top+avail && i < len(rows); i++ {
		out = append(out, fitName(rows[i].text, w))
	}
	if below {
		out = append(out, theme.Paint(theme.FilterMore, fit(str.ViewMoreBelow(len(rows)-top-avail), w)))
	}
	for len(out) < h {
		out = append(out, "")
	}
	return out
}

// panelWindow works out the panel's scroll for total rows on h screen
// rows, keeping row sel (and the button row under it) in view, and
// stores the top it settles on.
func (b *browse) panelWindow(total, h, sel int) (top int, above, below bool, avail int) {
	p := b.panel
	if total <= h {
		p.top = 0
		return 0, false, false, total
	}
	fitAt := func(t int) (int, bool, bool, int) {
		t = min(max(0, t), total-h+1)
		a := t > 0
		n := h
		if a {
			n--
		}
		bl := t+n < total
		if bl {
			n--
		}
		return t, a, bl, n
	}
	top, above, below, avail = fitAt(p.top)
	if sel >= 0 { // -1: keep the scroll as it is (clicks)
		if sel < top {
			top, above, below, avail = fitAt(sel - 1)
		}
		for sel+1 >= top+avail && top < total-avail {
			top, above, below, avail = fitAt(top + 1)
		}
	}
	p.top = top
	return top, above, below, avail
}
```

`internal/ui/browse.go`:

- In `browse`, add `panel *filterPanel // non-nil while the filter panel is open`.
- In `key`, before `b.status = ""`:
  ```go
  	if b.panel != nil {
  		return b.panelKey(k), false
  	}
  	if s == openFilterKey {
  		b.togglePanel()
  		return nil, false
  	}
  ```
- In `click`, after the prompt guard:
  ```go
  	if y == 0 {
  		if x >= b.filterChip[0] && x < b.filterChip[1] {
  			b.togglePanel()
  		}
  		return
  	}
  ```

In `internal/ui/filter.go`, a minimal `panelKey` that Task 4 extends:

```go
// panelKey handles a key while the filter panel is open.
func (b *browse) panelKey(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc", openFilterKey:
		b.panel = nil
	}
	return nil
}
```

(add the `tea "charm.land/bubbletea/v2"` import).

`internal/ui/view.go`, in the sidebar loop: before `sv := m.sidebarView()`, compute:

```go
	var panel []string
	if cs != nil && cs.browse != nil && cs.browse.panel != nil {
		side = theme.Filter
		panel = cs.browse.panelView(l.sw, m.height)
	}
```

and as the first case of the `switch r, hint := sv.at(y); {`:

```go
		case panel != nil:
			b.WriteString(theme.Fill(side, panel[y], l.sw))
```

(`sidebarView` still runs, but its result goes unused while the panel is open. That's fine, and it keeps the sidebar's scroll state intact.)

Make sure `cs` is in scope there. `View` already has `cs := m.cur()` near the top.

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/ui -run 'TestPanel' -v 2>&1 | tail -20`
Expected: PASS. If a panel row doesn't match, print `panelSide(h)` and compare it with the spec's layout. The indentation is `1+2*depth` for names and one more for buttons.

Run: `go test ./... && go vet ./...`
Expected: all `ok`. `TestNoHardCodedStyles` passes because every color goes through `theme`.

- [ ] **Step 6: Commit**

```bash
git add -A internal README.md
git commit -m "feat(log): a filter panel in the sidebar, opened by f or the Filter chip

Strings: filter.hide, filter.only, filter.add_text, filter.text_item (new)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_012rcJ1xqoQxzWAqkGanid5J"
```

---

### Task 4: Working the panel: keys, clicks, collapsing, scrolling

**Files:**
- Modify: `internal/ui/filter.go`
- Modify: `internal/ui/model.go` (sidebar clicks and wheel route to the panel)
- Modify: `internal/str/locales/en.toml` (`filter.hints`)
- Modify: `internal/ui/browse.go` (action bar shows the panel's hints while it's open)
- Test: `internal/ui/filter_test.go`, `internal/ui/browse_test.go` (harness `key` learns `left`, `right`, `delete`)

**Interfaces:**
- Consumes: everything from Task 3.
- Produces:
  - `(b *browse) panelKey(k tea.KeyPressMsg) tea.Cmd` (full version)
  - `(b *browse) panelClick(x, y, h int)`: `x`, `y` within the sidebar column, `h` the screen height
  - `(b *browse) panelScroll(delta, h int)`
  - `(b *browse) movePanel(delta int)`
  - `str.FilterHints()`

- [ ] **Step 1: Catalog**

Add to `[filter]`:

```toml
hints = "↑↓ move · h hide · o only · ←→ fold · x remove · f close"
```

Run `go generate ./internal/str`.

- [ ] **Step 2: Teach the harness three keys**

In `internal/ui/browse_test.go`'s `(h *harness) key`, add cases:

```go
	case "left":
		k = tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		k = tea.KeyPressMsg{Code: tea.KeyRight}
	case "delete":
		k = tea.KeyPressMsg{Code: tea.KeyDelete}
```

- [ ] **Step 3: Write the failing tests**

Append to `internal/ui/filter_test.go`:

```go
func TestPanelKeys(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	b, f := h.br(), h.kitFilter()
	page, in, self := scene.Item{Name: "page"}, scene.Item{Name: "page/in"}, scene.Item{Name: "self"}
	if b.panel.sel.item != page {
		t.Fatalf("starts on %v", b.panel.sel)
	}
	h.key("h")
	if !f.Hidden(page) || !f.Hidden(in) || strings.Contains(h.screen(), "Mira pages") {
		t.Errorf("h on page should hide it and page/in:\n%s", h.screen())
	}
	h.keys("down", "h") // page/in: unhide clears page, keeps nothing else hidden
	if f.Hidden(page) || f.Hidden(in) {
		t.Errorf("unhiding page/in: page %v, page/in %v", f.Hidden(page), f.Hidden(in))
	}
	h.keys("down", "o") // self: Only
	if o, _ := f.Only(); o != self || !f.Hidden(page) {
		t.Errorf("o on self: Only %v, page hidden %v", o, f.Hidden(page))
	}
	h.key("h") // Hide on the Only item: just self hidden
	if _, ok := f.Only(); ok || !f.Hidden(self) || f.Hidden(page) {
		t.Errorf("h on the Only item should hide just it")
	}
	h.keys("up", "up") // back to page
	h.key("left")      // collapse
	if !h.m.chars["fm/kit"].collapsed["page"] || slices.Contains(panelSide(h), "in") {
		t.Errorf("← should collapse page:\n%s", h.screen())
	}
	h.key("right")
	if h.m.chars["fm/kit"].collapsed["page"] {
		t.Error("→ should expand page")
	}
}

func TestCollapsedParentMarksFilterBelow(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	h.keys("down", "h", "up", "left") // hide page/in, collapse page
	if got := panelSide(h)[2]; got != glyphCollapsed+" page "+glyphFiltered {
		t.Errorf("collapsed page row = %q", got)
	}
}

func TestPanelClicks(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	f := h.kitFilter()
	click := func(x, y int) { h.m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}) }
	// Rows: 0 Filter, 1 blank, 2 ▼ page, 3 buttons, 4 in, 5 buttons, 6 blank, 7 self, 8 buttons.
	// Buttons for depth 0 start at column 2: "  Hide Only".
	hideX, onlyX := 2, 2+len(str.FilterHide())+1
	click(hideX, 8)
	if !f.Hidden(scene.Item{Name: "self"}) {
		t.Error("clicking self's Hide")
	}
	click(onlyX, 3)
	if o, _ := f.Only(); o != (scene.Item{Name: "page"}) {
		t.Error("clicking page's Only")
	}
	click(1, 2) // the ▼
	if !h.m.chars["fm/kit"].collapsed["page"] {
		t.Error("clicking ▼ should collapse page")
	}
	click(4, 7-2) // rows moved up two: self's name is now row 5
	if h.br().panel.sel.item != (scene.Item{Name: "self"}) {
		t.Errorf("clicking a name selects it: %v", h.br().panel.sel)
	}
}

func TestPanelClicksIgnoredDuringPrompt(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.keys("up", "m", "up", "up", "m", "f") // a range, then the panel
	h.key("esc")                            // close the panel
	h.key("e")                              // export's format prompt
	h.br().togglePanel()                    // panel open under the prompt
	h.screen()
	h.m.Update(tea.MouseClickMsg{X: 2, Y: 3, Button: tea.MouseLeft}) // page's Hide
	h.key("p")                                                       // must not panic
	if h.kitFilter().Active() {
		t.Error("clicks should be ignored while a prompt is open")
	}
}

func TestPanelScrolls(t *testing.T) {
	var lines []string
	for i := range 12 {
		lines = append(lines, fmt.Sprintf("[t%02d] note", i))
	}
	h := newHarness(t, map[string]string{"fm": fmWorld + manyTags(12)})
	h.writeLog(day24, lines...)
	h.key("ctrl+l")
	h.key("f")
	side := panelSide(h)
	// 12 tags × 3 rows + title and blank + "+ Text" = 39 rows on 24.
	if want := str.ViewMoreBelow(39 - 23 + 1); side[len(side)-1] != want {
		t.Fatalf("last row = %q, want %q:\n%s", side[len(side)-1], want, h.screen())
	}
	for range 40 {
		h.key("down")
	}
	if !h.br().panel.sel.add {
		t.Fatalf("down should reach + Text: %v", h.br().panel.sel)
	}
	if !slices.Contains(panelSide(h), str.FilterAddText()) {
		t.Errorf("+ Text not scrolled into view:\n%s", h.screen())
	}
	before := h.br().panel.top
	h.m.Update(tea.MouseWheelMsg{X: 1, Y: 5, Button: tea.MouseWheelUp})
	if h.br().panel.top >= before {
		t.Errorf("wheel up: top %d, was %d", h.br().panel.top, before)
	}
}
```

`manyTags(n)` goes in `filter_test.go`. It's a world snippet with n classify rules, each tagging `[tNN] note` as `tNN`. Rules are appended after the characters, as `TestQuietLinesDontCountAsUnread` does:

```go
// manyTags adds n classify rules to a world: "[tNN] …" gets tag tNN.
func manyTags(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "\n[[classify]]\ntag = \"t%02d\"\npattern = '^\\[t%02d\\]'\n", i, i)
	}
	return b.String()
}
```

The row count in `TestPanelScrolls` assumes these lines get no other tags. If the starter pack tags them too (check with `t` in log mode), count those tags' rows in. The expected `▼ N more` is total rows minus the rows shown: 23 shown (24 less the hint row), so `39-23+1`, since the top-hint row isn't there yet. Run once, confirm the exact number, and fix the constant if the arithmetic is off by the hint row.

Add imports: `fmt`, `slices`.

- [ ] **Step 4: Run the tests and watch them fail**

Run: `go test ./internal/ui -run 'TestPanel|TestCollapsed' 2>&1 | grep -v '^ \{8\}' | head`
Expected: FAIL. The keys do nothing yet, and clicks go to the sidebar.

- [ ] **Step 5: Implement**

In `internal/ui/filter.go`, replace `panelKey` and add the rest:

```go
// panelKey handles a key while the filter panel is open.
func (b *browse) panelKey(k tea.KeyPressMsg) tea.Cmd {
	f := &b.cs.filter
	sel := b.panel.sel
	switch k.String() {
	case "esc", openFilterKey:
		b.panel = nil
		return nil
	case "up":
		b.movePanel(-1)
	case "down":
		b.movePanel(1)
	case "h":
		if !sel.add {
			f.ToggleHide(sel.item, b.items())
		}
	case "o":
		if !sel.add {
			f.PressOnly(sel.item, b.items())
		}
	case "left":
		b.setCollapsed(sel, true)
	case "right":
		b.setCollapsed(sel, false)
	}
	b.refilter()
	return nil
}

// movePanel moves the highlight by delta entries, stopping at the ends.
func (b *browse) movePanel(delta int) {
	es := b.entries()
	i := 0
	for j, e := range es {
		if e.sel() == b.panel.sel {
			i = j
		}
	}
	b.panel.sel = es[min(max(0, i+delta), len(es)-1)].sel()
}

// setCollapsed folds a parent tag shut or open.
func (b *browse) setCollapsed(sel filterSel, shut bool) {
	if sel.add || sel.item.Text {
		return
	}
	if b.cs.collapsed == nil {
		b.cs.collapsed = map[string]bool{}
	}
	if shut {
		b.cs.collapsed[sel.item.Name] = true
	} else {
		delete(b.cs.collapsed, sel.item.Name)
	}
}

// panelClick handles a click at (x, y) in the sidebar column, h rows
// tall: a button presses it, a disclosure glyph folds, a name selects.
func (b *browse) panelClick(x, y, h int) {
	if b.prompt != promptNone {
		return
	}
	rows := b.panelRows(0)
	top, above, _, _ := b.panelWindow(len(rows), h, -1)
	if above {
		if y == 0 {
			b.panelScroll(-max(1, h-2), h)
			return
		}
		y--
	}
	i := top + y
	if i < 0 || i >= len(rows) || rows[i].entry < 0 {
		return
	}
	es := b.entries()
	e := es[rows[i].entry]
	f := &b.cs.filter
	indent := 1 + 2*e.depth
	switch rows[i].kind {
	case prAdd:
		b.panel.sel = e.sel()
		// Task 5: + Text asks for a term.
	case prName:
		if e.parent && x >= indent && x < indent+1 {
			b.setCollapsed(e.sel(), !b.cs.collapsed[e.item.Name])
		}
		b.panel.sel = e.sel()
	case prButtons:
		hx := indent + 1
		ox := hx + xansi.StringWidth(str.FilterHide()) + 1
		switch {
		case x >= hx && x < ox-1:
			f.ToggleHide(e.item, b.items())
		case x >= ox && x < ox+xansi.StringWidth(str.FilterOnly()):
			f.PressOnly(e.item, b.items())
		}
		b.panel.sel = e.sel()
	}
	b.refilter()
}

// panelScroll moves the panel's view by delta rows.
func (b *browse) panelScroll(delta, h int) {
	rows := b.panelRows(0)
	b.panel.top = min(max(0, b.panel.top+delta), max(0, len(rows)-h+1))
}
```

`TestPanelClicks` relies on the depth-0 buttons row being `"  Hide Only"`, meaning indent 1 plus one more space, so Hide starts at column 2. `hx := indent + 1` matches that.

`internal/ui/model.go`, sidebar clicks: at the top of `if msg.X < l.sw {` (in the click handler, above `if m.picker != nil && !m.listing()`):

```go
		if cs := m.cur(); cs != nil && cs.browse != nil && cs.browse.panel != nil {
			cs.browse.panelClick(msg.X, msg.Y, m.height)
			return nil
		}
```

`handleWheel`, at the top of `if msg.X < l.sw {`:

```go
		if cs != nil && cs.browse != nil && cs.browse.panel != nil {
			switch msg.Button {
			case tea.MouseWheelUp:
				cs.browse.panelScroll(-3, m.height)
			case tea.MouseWheelDown:
				cs.browse.panelScroll(3, m.height)
			}
			return nil
		}
```

That branch has to come **before** the existing `if cs != nil && cs.browse != nil && msg.X > l.sw` log-body wheel branch can't catch it. It can't anyway, since `msg.X < l.sw`, so placing it inside `if msg.X < l.sw` is enough.

`internal/ui/browse.go`, the action bar's `default:` case:

```go
	default:
		hints := str.BrowseHints()
		if b.panel != nil {
			hints = str.FilterHints()
		}
		rows = append(rows, theme.Fill(theme.LogBar, theme.Paint(theme.LogHints, hints), w))
```

- [ ] **Step 6: Run the tests**

Run: `go test ./internal/ui -run 'TestPanel|TestCollapsed' -v 2>&1 | grep -E '^(=== RUN|--- |\s+\w+_test)' | head -40`
Expected: PASS. If `TestPanelClicks` misses, print `h.screen()` and check the column math against the drawn row.

Run: `go test ./... && go vet ./...`
Expected: all `ok`.

- [ ] **Step 7: Commit**

```bash
git add -A internal
git commit -m "feat(log): Hide, Only, folding and scrolling in the filter panel

Strings: filter.hints (new)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_012rcJ1xqoQxzWAqkGanid5J"
```

---

### Task 5: Text rows, the panel golden and the README

**Files:**
- Modify: `internal/ui/browse.go` (`promptFilterText`)
- Modify: `internal/ui/filter.go` (`Enter` on `+ Text`, `x`/`Delete`, the `+ Text` click)
- Modify: `internal/str/locales/en.toml` (`filter.prompt`)
- Modify: `README.md` (a Filter section under Log mode)
- Test: `internal/ui/filter_test.go`, `internal/ui/golden_test.go`

**Interfaces:**
- Consumes: Tasks 1–4.
- Produces: `promptFilterText` (a `promptKind`), `str.FilterPrompt()`.

- [ ] **Step 1: Catalog**

Add to `[filter]`:

```toml
prompt = "filter text: "
```

Run `go generate ./internal/str`.

- [ ] **Step 2: Write the failing tests**

Append to `internal/ui/filter_test.go`:

```go
func TestPanelTextRows(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	for range 10 {
		h.key("down") // to + Text
	}
	h.key("enter")
	if h.br().prompt != promptFilterText {
		t.Fatalf("Enter on + Text should ask for a term:\n%s", h.screen())
	}
	h.typeText("lighthouse")
	h.key("enter")
	s := h.screen()
	if !strings.Contains(s, "The lighthouse is dark") || strings.Contains(s, "Sable") {
		t.Errorf("a new text row takes Only:\n%s", s)
	}
	if !slices.Contains(panelSide(h), str.FilterTextItem("lighthouse")) {
		t.Errorf("no text row:\n%s", s)
	}
	// An exact repeat adds nothing; a different term is a second row.
	h.keys("down", "enter") // + Text
	h.typeText("lighthouse")
	h.key("enter")
	h.keys("down", "enter")
	h.typeText("Rook")
	h.key("enter")
	if got := h.kitFilter().Texts(); len(got) != 2 {
		t.Errorf("texts = %v, want lighthouse and Rook", got)
	}
	h.key("up") // back onto a text row
	h.key("x")
	h.key("up")
	h.key("delete")
	if n := len(h.kitFilter().Texts()); n != 0 {
		t.Errorf("x and Delete should remove text rows, %d left", n)
	}
	if !strings.Contains(h.screen(), "Sable") {
		t.Errorf("removing the Only text row should show lines again:\n%s", h.screen())
	}
}
```

In `internal/ui/golden_test.go`, add:

```go
func TestGoldenLogFilter(t *testing.T) {
	h := goldenHarness(t)
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	h.keys("down", "h", "down", "down", "o") // hide page/in, then Only on self
	assertGolden(t, "log-filter", h.drawn())
}
```

- [ ] **Step 3: Run the tests and watch them fail**

Run: `go test ./internal/ui -run 'TestPanelTextRows|TestGoldenLogFilter' 2>&1 | grep -v '^ \{8\}' | head`
Expected: build failure on `promptFilterText`. Once it builds, `TestGoldenLogFilter` fails for want of a golden file.

- [ ] **Step 4: Implement**

`internal/ui/browse.go`:
- Add `promptFilterText` to the `promptKind` consts after `promptFilename`.
- In `promptKey`'s `enter` case, add:
  ```go
  		case promptFilterText:
  			if b.cs.filter.AddText(v, b.items()) {
  				b.panel.sel = filterSel{item: scene.Item{Name: v, Text: true}}
  			}
  			b.refilter()
  ```
- In `promptLabel`, add `case promptFilterText: return str.FilterPrompt()`.
- In `key`, the prompt branch must run before the panel branch. It already does (`if b.prompt != promptNone` is first), so typing into the prompt while the panel is open works.

`internal/ui/filter.go`, in `panelKey`'s switch:

```go
	case "enter":
		if sel.add {
			b.prompt = promptFilterText
			b.pin.SetValue("")
		}
	case "x", "delete":
		if sel.item.Text {
			b.movePanel(1) // keep a highlight when the row goes
			f.RemoveText(sel.item.Name)
		}
```

In `panelClick`'s `prAdd` case, replace the Task 5 comment with:

```go
		b.prompt = promptFilterText
		b.pin.SetValue("")
```

Note that `AddText` trims, so the term stored is `v` trimmed, and `promptKey` already trims `v`.

`README.md`: after the log-mode key table's paragraph about exports, add:

```markdown
#### Filter

`f`, or the ` Filter ` chip at the top right, opens the filter panel in the sidebar. It lists every tag in the loaded logs, children indented under their parents (`page/in` under `page`), then any text you've added, each with **Hide** and **Only**:

- **Hide** hides lines with that tag (or text). Hiding a parent hides its children too; unhiding one child brings its parent back and leaves the other children hidden.
- **Only** shows just lines with that tag (or text), and lights Hide on every other tag (or text) so you can see what's hidden. Unhide any of them and Only lets go; press Only again to clear everything; press Hide on it to flip it, showing everything but that.
- **+ Text** adds text to filter on, matched like `/` find. It starts on Only. `x` or `Delete` removes it.

`↑`/`↓` move, `h`/`o` press Hide/Only, `←`/`→` fold a parent (a folded parent with a filter inside shows `•`), and `f` or `Esc` closes the panel. The log updates as you go. Filters stay set until you quit Kiln.
```

Also update line 23's feature summary, "You can filter by tag, search, …", to "You can filter by tag or text, search, …".

- [ ] **Step 5: Run the tests and create the golden**

Run: `go test ./internal/ui -run TestGoldenLogFilter -update && go test ./internal/ui -run 'TestPanelTextRows|TestGoldenLogFilter'`
Expected: PASS. Read `internal/ui/testdata/golden/log-filter.txt` and check it by eye: the panel in the sidebar column, `in`'s Hide lit, `self`'s Only lit, `page`'s Hide lit (from the Only), and the log showing only `Kit grins.`.

Run: `go test ./... && go vet ./...`
Expected: all `ok`.

- [ ] **Step 6: Commit**

```bash
git add -A internal README.md
git commit -m "feat(log): text rows in the filter panel; its golden and README

The new log-filter golden pins the panel's look.

Strings: filter.prompt (new)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_012rcJ1xqoQxzWAqkGanid5J"
```

---

## After the last task

- Run a smoke test with a scratch config (`XDG_CONFIG_HOME` and `XDG_DATA_HOME` pointed at temp dirs; never the real setup): open log mode, press `f`, hide and only a few tags, add a text row, collapse a parent, leave log mode and come back.
- If the implementation had to depart from the spec anywhere, update the spec in the same commit and say so.
