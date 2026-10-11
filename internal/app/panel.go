package app

import (
	"slices"
	"strings"

	"github.com/latrani/Kiln/internal/scene"
)

// PanelEntry is one item in the filter panel's list.
type PanelEntry struct {
	Item       scene.Item
	Depth      int  // slashes in a tag; 0 for text
	Parent     bool // a tag with children
	FoldedAway bool // under a folded parent: a list shows it only once that opens
}

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
