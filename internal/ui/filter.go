package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/theme"
)

// filterPanel is the filter panel's scroll, the terminal's half of it:
// the core's Log.Panel holds the rest.
type filterPanel struct {
	top    int  // first row drawn
	follow bool // scroll the selection into view on the next draw
}

type panelRowKind int

const (
	prName panelRowKind = iota
	prButtons
	prBlank
	prAdd
)

type panelRow struct {
	text  string        // drawn, not yet fitted
	entry int           // index into entries()
	sel   app.FilterSel // the entry's, on its name or + Text row
	kind  panelRowKind
}

// togglePanel opens or closes the filter panel, scrolled to its
// highlight when it opens.
func (b *browse) togglePanel() {
	b.TogglePanel()
	b.pv = filterPanel{follow: true}
}

// panelRows lays the panel out: per entry its name and its
// Hide and Only buttons, a blank row after each top-level group (a tag
// and its children, or a text row); + Text last.
func (b *browse) panelRows(w int) []panelRow {
	var rows []panelRow
	f := &b.cs.Filter
	only, hasOnly := f.Only()
	items := b.Items()
	es := b.EntriesOf(items)
	for i, e := range es {
		if e.Add {
			rows = append(rows, panelRow{text: " " + theme.Paint(theme.FilterAdd, str.FilterAddText()), entry: i, sel: e.Sel(), kind: prAdd})
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
		if b.Panel != nil && b.Panel.Sel == e.Sel() {
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
			panelRow{text: indent + theme.Paint(role, name), entry: i, sel: e.Sel(), kind: prName},
			panelRow{text: indent + " " + theme.Paint(hide, str.FilterHide()) + " " + theme.Paint(onlyRole, str.FilterOnly()), entry: i, kind: prButtons})
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
	p := &b.pv
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
	p := &b.pv
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
	sel := b.Panel.Sel
	switch k.String() {
	case "esc", "ctrl+c", openFilterKey: // as they back out of log mode
		b.TogglePanel() // it's open: this closes it
		return nil
	case "up", "k":
		b.MovePanel(-1)
		b.pv.follow = true
	case "down", "j":
		b.MovePanel(1)
		b.pv.follow = true
	case "h":
		if !sel.Add {
			b.ToggleHide(sel.Item)
		}
	case "o":
		if !sel.Add {
			b.PressOnly(sel.Item)
		}
	case "left":
		b.SetCollapsed(sel, true)
	case "right":
		b.SetCollapsed(sel, false)
	case "enter":
		if sel.Add {
			b.prompt = promptFilterText
			b.pin.SetValue("")
		}
	case "x", "delete":
		if b.RemoveText(sel) {
			b.pv.follow = true
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
	es := b.Entries()
	e := es[rows[i].entry]
	indent := 1 + 2*e.Depth
	switch rows[i].kind {
	case prAdd:
		b.Select(e.Sel())
		b.prompt = promptFilterText
		b.pin.SetValue("")
	case prName:
		onGlyph := x >= indent && x < indent+1
		switch {
		case e.Item.Text && onGlyph:
			b.Select(e.Sel())
			if b.RemoveText(e.Sel()) {
				b.pv.follow = true
			}
			b.Refilter()
			return
		case e.Parent && onGlyph:
			b.SetCollapsed(e.Sel(), !b.cs.Collapsed[e.Item.Name])
		}
		b.Select(e.Sel())
	case prButtons:
		hx := indent + 1
		ox := hx + xansi.StringWidth(str.FilterHide()) + 1
		switch {
		case x >= hx && x < ox-1:
			b.ToggleHide(e.Item)
		case x >= ox && x < ox+xansi.StringWidth(str.FilterOnly()):
			b.PressOnly(e.Item)
		}
		b.Select(e.Sel())
	}
	b.Refilter()
}

// panelScroll moves the panel's view by delta rows.
func (b *browse) panelScroll(delta, h int) {
	rows := b.panelRows(0)
	b.pv.top = min(max(0, b.pv.top+delta), max(0, len(rows)-h+1))
}
