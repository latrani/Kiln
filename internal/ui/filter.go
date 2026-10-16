package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/scene"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/theme"
)

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

// newPanel puts the filter panel on its first entry, scrolled to it,
// with the log focused: log mode as it starts.
func (b *browse) newPanel() {
	b.Panel, b.panelFocus = &filterPanel{Sel: b.entries()[0].sel(), follow: true}, false
}

// focusPanel gives the panel the keys, or (false) the log; the
// highlight comes into view.
func (b *browse) focusPanel(on bool) {
	b.panelFocus = on
	if on {
		b.Panel.follow = true
	}
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

// toggleFold folds sel shut if it's open, or open if it's shut.
func (b *browse) toggleFold(sel filterSel) { b.fold(sel, !b.cs.Collapsed[sel.Item.Name]) }

// panelRows lays the panel out: per entry its name and its
// Hide and Only buttons, a blank row after each top-level group (a tag
// and its children, or a text row); + Text last.
func (b *browse) panelRows(w int) []panelRow {
	var rows []panelRow
	f := &b.cs.Filter
	only, hasOnly := f.Only()
	items := b.Items()
	es := b.entriesOf(items)
	for i, e := range es {
		if e.Add {
			rows = append(rows, panelRow{text: " " + theme.Paint(theme.FilterAdd, str.FilterAddText()), entry: i, sel: e.sel(), kind: prAdd})
			continue
		}
		indent := strings.Repeat(" ", 1+2*e.Depth)
		name := e.Item.Name
		if e.Item.Untagged {
			name = str.FilterUntagged()
		} else if e.Item.Text {
			name = glyphRemove + " " + str.FilterTextItem(name)
		} else if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		switch {
		case e.Parent && b.cs.Collapsed[e.Item.Name]:
			name = glyphCollapsed + " " + name
			if b.FilteredUnder(e.Item.Name, items) {
				name += " " + glyphFiltered
			}
		case e.Parent:
			name = glyphOpen + " " + name
		}
		role := theme.FilterItem
		if b.Panel != nil && b.Panel.Sel == e.sel() {
			role = theme.FilterSelected
		}
		hide, onlyRole := theme.FilterButton, theme.FilterButton
		if f.Hidden(e.Item) {
			hide = theme.FilterButtonOn
		}
		if hasOnly && only == e.Item {
			onlyRole = theme.FilterButtonOn
		}
		rows = append(rows,
			panelRow{text: indent + theme.Paint(role, name), entry: i, sel: e.sel(), kind: prName},
			panelRow{text: indent + " " + theme.Paint(hide, str.FilterHide()) + str.Separator() + theme.Paint(onlyRole, str.FilterOnly()), entry: i, kind: prButtons})
		if next := es[i+1]; next.Depth == 0 { // a blank row ends a top-level group
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
	p := b.Panel
	selRow := -1 // keep the scroll where the wheel left it
	if p.follow {
		p.follow = false
		for i, r := range rows {
			if (r.kind == prName || r.kind == prAdd) && r.sel == b.Panel.Sel {
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
	p := b.Panel
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

// panelKey handles a key for the filter panel: one only it uses, from
// either side, or a shared one while it has focus.
func (b *browse) panelKey(k tea.KeyPressMsg) tea.Cmd {
	sel := b.Panel.Sel
	switch k.String() {
	case "esc", "ctrl+c": // one step back: to the log
		b.focusPanel(false)
		return nil
	case "up", "k":
		b.movePanel(-1)
		b.Panel.follow = true
	case "down", "j":
		b.movePanel(1)
		b.Panel.follow = true
	case "h":
		if !sel.Add {
			b.ToggleHide(sel.Item)
		}
	case "o":
		if !sel.Add {
			b.PressOnly(sel.Item)
		}
	case "left":
		b.fold(sel, true)
	case "right":
		b.fold(sel, false)
	case "enter":
		if sel.Add {
			b.prompt = promptFilterText
			b.pin.SetValue("")
		} else {
			b.toggleFold(sel)
		}
	case "space", " ":
		b.toggleFold(sel)
	case "x", "delete":
		if b.removeText(sel) {
			b.Panel.follow = true
		}
	}
	b.Refilter()
	return nil
}

// panelClick handles a click at (x, y) in the sidebar column, h rows
// tall: a button presses it, a disclosure glyph folds, a text row's ×
// removes it, a name selects.
func (b *browse) panelClick(x, y, h int) {
	if b.prompt != promptNone {
		return
	}
	b.panelFocus = true // not focusPanel: the panel stays where the click found it
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
	indent := 1 + 2*e.Depth
	switch rows[i].kind {
	case prAdd:
		b.Panel.Sel = e.sel()
		b.prompt = promptFilterText
		b.pin.SetValue("")
	case prName:
		onGlyph := x >= indent && x < indent+1
		switch {
		case e.Item.Text && onGlyph:
			b.Panel.Sel = e.sel()
			if b.removeText(e.sel()) {
				b.Panel.follow = true
			}
			b.Refilter()
			return
		case e.Parent && onGlyph:
			b.toggleFold(e.sel())
		}
		b.Panel.Sel = e.sel()
	case prButtons:
		hx := indent + 1
		ox := hx + xansi.StringWidth(str.FilterHide()) + xansi.StringWidth(str.Separator())
		switch {
		case x >= hx && x < hx+xansi.StringWidth(str.FilterHide()):
			b.ToggleHide(e.Item)
		case x >= ox && x < ox+xansi.StringWidth(str.FilterOnly()):
			b.PressOnly(e.Item)
		}
		b.Panel.Sel = e.sel()
	}
	b.Refilter()
}

// panelScroll moves the panel's view by delta rows.
func (b *browse) panelScroll(delta, h int) {
	rows := b.panelRows(0)
	b.Panel.top = min(max(0, b.Panel.top+delta), max(0, len(rows)-h+1))
}
