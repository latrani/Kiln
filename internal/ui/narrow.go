package ui

// NarrowWidth is the narrowest Kiln keeps the sidebar in its own column.
// Under it, narrow mode: no column, and the sidebar is a toggle that
// takes the top bar and scrollback while it's open.
const NarrowWidth = 64

// narrow reports whether the screen is under NarrowWidth.
func (m *Model) narrow() bool { return m.width < NarrowWidth }

// narrowShown reports whether the narrow sidebar is open: opened (≡,
// Ctrl+B), the picker's list up, or log mode's panel with focus (in
// narrow log mode the panel is open exactly while it has the keys).
func (m *Model) narrowShown() bool {
	if !m.narrow() {
		return false
	}
	cs := m.cur()
	if cs != nil && cs.browse != nil {
		return cs.browse.panelFocus
	}
	return m.narrowOpen || m.listing()
}

// narrowH is how many rows the open narrow sidebar takes: the top bar
// and the scrollback (or the log), down to the input area (or the log's
// bar) and the statusline, which stay.
func (m *Model) narrowH() int {
	if cs := m.cur(); cs != nil && cs.browse != nil {
		return max(sideTitleH+1, m.height-4) // the log's rule, bar and rule, and the statusline
	}
	l := m.layout()
	return l.top + l.sbH
}
