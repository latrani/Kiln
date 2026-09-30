package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

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

// panelRows lays the panel out: a title, then per entry its name and its
// Hide and Only buttons, a blank row after each top-level group (a tag
// and its children, or a text row); + Text last.
func (b *browse) panelRows(w int) []panelRow {
	rows := []panelRow{{text: theme.Paint(theme.FilterItem, " "+str.FilterTitle()), entry: -1, kind: prTitle}, {entry: -1, kind: prBlank}}
	f := &b.cs.filter
	only, hasOnly := f.Only()
	es := b.entries()
	for i, e := range es {
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
			panelRow{text: indent + " " + theme.Paint(hide, str.FilterHide()) + " " + theme.Paint(onlyRole, str.FilterOnly()), entry: i, kind: prButtons})
		if next := es[i+1]; next.depth == 0 { // a blank row ends a top-level group
			rows = append(rows, panelRow{entry: i, kind: prBlank})
		}
	}
	return rows
}

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

// panelKey handles a key while the filter panel is open.
func (b *browse) panelKey(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc", openFilterKey:
		b.panel = nil
	}
	return nil
}
