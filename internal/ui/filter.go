package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/glyph"
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
	Sel    filterSel   // the highlighted entry
	top    int         // first row drawn
	follow bool        // scroll the highlight into view on the next draw
	listH  int         // the list's rows, above the footer, at the last draw
	field  *tea.Cursor // where the text being added was drawn, in the list; nil with none
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
	x     int       // where the name's glyph or Hide starts, past the tree
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

// fold folds sel shut or open if it's a group or tag; + Text and text
// rows don't fold.
func (b *browse) fold(sel filterSel, shut bool) {
	if !sel.Add && !sel.Item.Text {
		b.Fold(sel.Item, shut)
	}
}

// toggleFold folds sel shut if it's open, or open if it's shut.
func (b *browse) toggleFold(sel filterSel) { b.fold(sel, !b.cs.Collapsed[sel.Item]) }

// panelRows lays the panel out: per entry its name and its Hide and
// Only buttons, as a tree; + Text last. A line runs down from under each
// open parent's ▼, one column a level, branching to each child. Hide
// and Only sit under the name, one column in. A blank row parts the tree
// from the text rows and + Text, which is the field while text is
// added.
func (b *browse) panelRows(w int) []panelRow {
	var rows []panelRow
	f := &b.cs.Filter
	items := b.Items()
	es := b.entriesOf(items)
	var more []bool // per level from 1, whether the entry at it has a sibling still to come
	for i, e := range es {
		if (e.Add || e.Item.Text) && i > 0 && !es[i-1].Item.Text {
			rows = append(rows, panelRow{entry: -1, kind: prBlank})
		}
		if e.Add {
			text := theme.Paint(theme.FilterAdd, str.FilterAddText())
			if b.prompt == promptFilterText {
				text, _ = addField(b.pin, str.FilterField(), theme.FilterAdd, theme.FilterAddHint, w)
			}
			rows = append(rows, panelRow{text: text, entry: i, sel: e.sel(), kind: prAdd})
			continue
		}
		d := e.Depth
		var lead, nameTree, btnTree string
		if d > 0 {
			more = append(more[:min(len(more), d-1)], hasSibling(es, i))
			for _, m := range more[:d-1] {
				lead += treeLine(m)
			}
			nameTree, btnTree = lead+glyphTreeLast, lead+" "
			if more[d-1] {
				nameTree, btnTree = lead+glyphTreeBranch, lead+glyphTreeLine
			}
		}
		name := e.Item.Name
		if e.Item.Group == scene.CategoryGroup {
			name = str.FilterCategories()
		} else if e.Item.Group == scene.FlagGroup {
			name = str.FilterFlags()
		} else if e.Item.Text {
			name = glyphRemove + " " + str.FilterTextItem(name)
		} else if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		open := e.Parent && !b.cs.Collapsed[e.Item] && i+1 < len(es) && es[i+1].Depth > d
		switch {
		case e.Parent && b.cs.Collapsed[e.Item]:
			name = glyphCollapsed + name
			if b.FilteredUnder(e.Item, items) {
				name += " " + glyphFiltered
			}
		case e.Parent:
			name = glyphOpen + name
		}
		if open {
			btnTree += glyphTreeLine
		}
		role := theme.FilterItem
		if b.Panel != nil && b.Panel.Sel == e.sel() {
			role = theme.FilterSelected
		}
		hide, onlyRole := theme.FilterButton, theme.FilterButton
		if f.Hidden(e.Item) {
			hide = theme.FilterButtonOn
		}
		if f.Only(e.Item) {
			onlyRole = theme.FilterButtonOn
		}
		btns := theme.Paint(hide, str.FilterHide()) + str.Separator() + theme.Paint(onlyRole, str.FilterOnly())
		btnX := xansi.StringWidth(nameTree) + 1
		switch {
		case e.Parent: // past the glyph
			btnX++
		case e.Item.Text: // past the × and its space
			btnX += 2
		}
		rows = append(rows,
			panelRow{text: theme.Paint(theme.FilterTree, nameTree) + theme.Paint(role, name), x: xansi.StringWidth(nameTree), entry: i, sel: e.sel(), kind: prName},
			panelRow{text: theme.Paint(theme.FilterTree, btnTree) + strings.Repeat(" ", btnX-xansi.StringWidth(btnTree)) + btns, x: btnX, entry: i, kind: prButtons})
	}
	return rows
}

// hasSibling reports whether es[i] has a sibling after it.
func hasSibling(es []panelEntry, i int) bool {
	for _, e := range es[i+1:] {
		switch {
		case e.Add || e.Depth < es[i].Depth:
			return false
		case e.Depth == es[i].Depth:
			return true
		}
	}
	return false
}

// treeLine is an ancestor's column in the tree: its line while it has a
// sibling still to come, blank after its last.
func treeLine(more bool) string {
	if more {
		return glyphTreeLine
	}
	return " "
}

// panelView draws the panel into a column w wide and h tall: its list,
// scrolled so the selected entry's rows show ("▲ N more" / "▼ N more"
// replace the edge rows when it overflows, as in the sidebar), then a
// rule and its footer.
func (b *browse) panelView(w, h int) []string {
	foot := b.panelFooter(w, h)
	listH := h
	if len(foot) > 0 {
		listH -= len(foot) + 1
	}
	out := b.panelList(w, listH)
	if len(foot) > 0 {
		out = append(out, theme.Paint(theme.RuleFilter, strings.Repeat(glyph.Rule, w)))
		role := theme.FilterHints
		if !b.panelFocus { // the keys aren't the panel's: faded
			role = theme.FilterHintsAway
		}
		for _, r := range foot {
			out = append(out, theme.Paint(role, fit(r, w)))
		}
	}
	return out
}

// panelList draws the panel's list, h rows.
func (b *browse) panelList(w, h int) []string {
	rows := b.panelRows(w)
	p := b.Panel
	p.listH = h
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
	p.field = nil
	for i := top; i < top+avail && i < len(rows); i++ {
		if rows[i].kind == prAdd && b.prompt == promptFilterText {
			_, x := addField(b.pin, str.FilterField(), theme.FilterAdd, theme.FilterAddHint, w)
			p.field = &tea.Cursor{Position: tea.Position{X: x, Y: len(out)}}
		}
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
	if b.Panel.listH > 0 {
		h = min(h, b.Panel.listH) // the footer's rows below do nothing
	}
	if y >= h {
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
	e, at := es[rows[i].entry], rows[i].x
	switch rows[i].kind {
	case prAdd:
		b.Panel.Sel = e.sel()
		b.prompt = promptFilterText
		b.pin.SetValue("")
	case prName:
		onGlyph := x == at
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
		hx := at
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
	if b.Panel.listH > 0 {
		h = min(h, b.Panel.listH)
	}
	rows := b.panelRows(0)
	b.Panel.top = min(max(0, b.Panel.top+delta), max(0, len(rows)-h+1))
}

// footerRows lays hints out in rows w wide, breaking only between the
// keys the separator parts; a key too wide for a row is cut.
func footerRows(hints string, w int) []string {
	sep := str.Separator()
	var rows []string
	for _, k := range strings.Split(hints, sep) {
		k = xansi.Truncate(k, w, "…")
		if n := len(rows); n > 0 && xansi.StringWidth(rows[n-1]+sep+k) <= w {
			rows[n-1] += sep + k
			continue
		}
		rows = append(rows, k)
	}
	return rows
}

// footKind is what a panel row is, for the keys that work on it.
type footKind int

const (
	footLeaf   footKind = iota // a tag with none under it
	footParent                 // a group or tag with tags under it: it folds
	footText                   // a text: it's removed
	footAdd                    // + Text: it adds
)

// panelKeys is the footer's keys for a row of kind k: those that work on
// it from the side that has focus (focus: the panel), then f while the
// panel has it.
func panelKeys(k footKind, focus bool) []string {
	var keys []string
	if k != footAdd {
		keys = append(keys, str.FilterKeyHide(), str.FilterKeyOnly())
	}
	switch {
	case k == footParent && focus:
		keys = append(keys, str.FilterKeyFold())
	case k == footParent:
		keys = append(keys, str.FilterKeyFoldAway())
	case k == footText:
		keys = append(keys, str.FilterKeyRemove())
	case k == footAdd && focus:
		keys = append(keys, str.FilterKeyAdd())
	}
	if focus {
		keys = append(keys, str.FilterKeyLog())
	}
	return keys
}

// selKind is footKind for the highlighted row.
func (b *browse) selKind() footKind {
	sel := b.Panel.Sel
	switch {
	case sel.Add:
		return footAdd
	case sel.Item.Text:
		return footText
	}
	for _, e := range b.entries() {
		if e.sel() == sel && e.Parent {
			return footParent
		}
	}
	return footLeaf
}

// panelFooter is the footer's rows for a column w wide and h tall: the
// keys for the highlighted row, in as many rows as the most any row
// needs, so the list doesn't change height as the highlight moves; at
// most a third of the column, and never so many the list gets fewer
// than minPanelList rows (the last keys go first).
func (b *browse) panelFooter(w, h int) []string {
	sep := str.Separator()
	need := 0
	for k := footLeaf; k <= footAdd; k++ {
		need = max(need, len(footerRows(strings.Join(panelKeys(k, b.panelFocus), sep), w)))
	}
	rows := footerRows(strings.Join(panelKeys(b.selKind(), b.panelFocus), sep), w)
	n := min(need, h/3, h-1-minPanelList) // the rule takes a row
	for len(rows) < n {
		rows = append(rows, "")
	}
	return rows[:max(0, n)]
}

// minPanelList is the fewest rows the panel's list keeps for itself.
const minPanelList = 3
