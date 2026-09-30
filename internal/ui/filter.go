package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
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
	sel    filterSel
	top    int  // first row drawn
	follow bool // scroll the selection into view on the next draw
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
	text  string    // drawn, not yet fitted
	entry int       // index into entries(); -1 for the title and its blank
	sel   filterSel // the entry's, on its name or + Text row
	kind  panelRowKind
}

// togglePanel opens or closes the filter panel.
func (b *browse) togglePanel() {
	if b.panel != nil {
		b.panel = nil
		return
	}
	b.panel = &filterPanel{follow: true}
	if es := b.entries(); len(es) > 0 {
		b.panel.sel = es[0].sel()
	}
}

// entries is what the panel lists: tags (children of a collapsed parent
// left out), text rows, then + Text.
func (b *browse) entries() []panelEntry { return b.entriesOf(b.items()) }

// entriesOf is entries for items already in hand.
func (b *browse) entriesOf(items []scene.Item) []panelEntry {
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
func (b *browse) filteredUnder(parent string, items []scene.Item) bool {
	f := &b.cs.filter
	only, hasOnly := f.Only()
	for _, it := range items {
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
	items := b.items()
	es := b.entriesOf(items)
	for i, e := range es {
		if e.add {
			rows = append(rows, panelRow{text: " " + theme.Paint(theme.FilterAdd, str.FilterAddText()), entry: i, sel: e.sel(), kind: prAdd})
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
			if b.filteredUnder(e.item.Name, items) {
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
			panelRow{text: indent + theme.Paint(role, name), entry: i, sel: e.sel(), kind: prName},
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
	selRow := -1 // keep the scroll where the wheel left it
	if p.follow {
		p.follow = false
		for i, r := range rows {
			if (r.kind == prName || r.kind == prAdd) && r.sel == p.sel {
				selRow = i
				break
			}
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
	}
	b.refilter()
	return nil
}

// movePanel moves the highlight by delta entries, stopping at the ends.
func (b *browse) movePanel(delta int) {
	b.panel.follow = true
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
	top, above, below, avail := b.panelWindow(len(rows), h, -1)
	if above {
		if y == 0 {
			b.panelScroll(-max(1, h-2), h)
			return
		}
		y--
	}
	if below && y == avail {
		b.panelScroll(max(1, h-2), h)
		return
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
		b.prompt = promptFilterText
		b.pin.SetValue("")
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
