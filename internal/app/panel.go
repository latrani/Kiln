package app

import (
	"slices"
	"strings"

	"github.com/latrani/Kiln/internal/scene"
)

// PanelEntry is one item in the filter panel's list.
type PanelEntry struct {
	Item       scene.Item
	Depth      int  // 0 for a group or a text; a tag's slashes, plus one
	Parent     bool // a group or tag with tags under it
	FoldedAway bool // under a folded parent: a list shows it only once that opens
}

// Entries is what the panel lists: Categories and its tags, Flags and
// its tags (each child of a folded parent marked FoldedAway), then text
// rows.
func (g *Log) Entries() []PanelEntry { return g.EntriesOf(g.Items()) }

// EntriesOf is Entries for items already in hand.
func (g *Log) EntriesOf(items []scene.Item) []PanelEntry {
	g.foldNew(items)
	out := make([]PanelEntry, 0, len(items))
	for i, it := range items {
		e := PanelEntry{Item: it, Parent: parentAt(items, i)}
		if !it.Text && it.Group == "" {
			e.Depth = strings.Count(it.Name, "/") + 1
			e.FoldedAway = g.foldedAway(it)
		}
		out = append(out, e)
	}
	return out
}

// parentAt reports whether items[i] has tags under it: Items lists a
// group's or tag's children right after it.
func parentAt(items []scene.Item, i int) bool {
	it := items[i]
	if it.Text || i+1 >= len(items) {
		return false
	}
	next := items[i+1]
	if next.Text || next.Group != "" {
		return false
	}
	return it.Group != "" || strings.HasPrefix(next.Name, it.Name+"/")
}

// foldNew folds each parent tag, at any depth, the first time the panel
// sees it; after that it stays as the reader left it. Groups start open.
func (g *Log) foldNew(items []scene.Item) {
	c := g.c
	for i, it := range items {
		if it.Group != "" || !parentAt(items, i) || c.FoldSeen[it] {
			continue
		}
		if c.FoldSeen == nil {
			c.FoldSeen, c.Collapsed = map[scene.Item]bool{}, nilToEmpty(c.Collapsed)
		}
		c.FoldSeen[it] = true
		c.Collapsed[it] = true
	}
}

func nilToEmpty(m map[scene.Item]bool) map[scene.Item]bool {
	if m == nil {
		return map[scene.Item]bool{}
	}
	return m
}

// hasChildren reports whether any loaded tag sits under it.
func (g *Log) hasChildren(it scene.Item) bool {
	items := g.Items()
	i := slices.Index(items, it)
	return i >= 0 && parentAt(items, i)
}

// foldedAway reports whether it sits under a collapsed parent.
func (g *Log) foldedAway(it scene.Item) bool {
	f := &g.c.Filter
	for p, ok := f.ParentOf(it); ok; p, ok = f.ParentOf(p) {
		if g.c.Collapsed[p] {
			return true
		}
	}
	return false
}

// FilteredUnder reports whether any tag under parent is hidden or has
// Only, for the mark on a folded parent.
func (g *Log) FilteredUnder(parent scene.Item, items []scene.Item) bool {
	f := &g.c.Filter
	only, hasOnly := f.Only()
	for _, it := range items {
		if f.Under(parent, it) && (f.Hidden(it) || hasOnly && only == it) {
			return true
		}
	}
	return false
}

// Fold folds a parent shut or open. One without loaded children is left
// be.
func (g *Log) Fold(it scene.Item, shut bool) {
	if !g.hasChildren(it) {
		return
	}
	if g.c.Collapsed == nil {
		g.c.Collapsed = map[scene.Item]bool{}
	}
	if shut {
		g.c.Collapsed[it] = true
	} else {
		delete(g.c.Collapsed, it)
	}
}

// ToggleHide hides it, or shows it again.
func (g *Log) ToggleHide(it scene.Item) { g.c.Filter.ToggleHide(it, g.Items()) }

// PressOnly shows only it, or everything again.
func (g *Log) PressOnly(it scene.Item) { g.c.Filter.PressOnly(it, g.Items()) }

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
