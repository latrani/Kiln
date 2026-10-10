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
