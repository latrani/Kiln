package ui

import (
	"strconv"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/glyph"
	"github.com/latrani/Kiln/internal/theme"
)

// toggleSidebarKey collapses the sidebar, or brings it back.
const toggleSidebarKey = "ctrl+b"

// sidebarShown reports whether the sidebar has its column: not collapsed,
// or shown for now (see sideForced).
func (m *Model) sidebarShown() bool { return !m.a.SidebarCollapsed() || m.sideForced() }

// sideForced reports whether the sidebar shows only for now, though
// collapsed: the picker's list is up, or log mode's filter panel has
// focus, so you never type into a list you can't see. Collapsing is what
// was chosen, so it's collapsed again after.
func (m *Model) sideForced() bool {
	if m.narrow() || !m.a.SidebarCollapsed() { // narrow: ‹ always closes it
		return false
	}
	cs := m.cur()
	return m.listing() || cs != nil && cs.browse != nil && cs.browse.panelFocus
}

// canToggle reports whether the sidebar can collapse or come back now:
// not while it lists the picker, or an editor opened from there. An
// editor opened from the main view leaves the sidebar as it was, so it
// can. ‹ and ≡ show only when it can.
func (m *Model) canToggle() bool { return !m.listing() }

// toggleSidebar collapses the sidebar or brings it back, as Ctrl+B, ‹ and
// ≡ do; not while it lists the picker (see canToggle). Collapsing while log
// mode's panel has focus gives the log focus, or the panel would stay.
func (m *Model) toggleSidebar() {
	if !m.canToggle() {
		return
	}
	if m.narrow() { // a toggle, not saved; in log mode, the panel with the keys, as f
		if cs := m.cur(); cs != nil && cs.browse != nil {
			cs.browse.focusPanel(!cs.browse.panelFocus)
		} else {
			m.narrowOpen = !m.narrowOpen
		}
		return
	}
	shut := !m.a.SidebarCollapsed()
	if cs := m.cur(); shut && cs != nil && cs.browse != nil {
		cs.browse.focusPanel(false)
	}
	m.a.SetSidebarCollapsed(shut)
}

// menuPrefix is the top bar's start while the sidebar is collapsed: ≡,
// then for the connections (not log mode's filters) the rows' activity
// summed, as each row shows its own: ● if any has an unread priority
// line, then their unread counts' total; then a space before the name.
// menu is the columns ≡ and its badge take, the button they make; zero
// when it isn't drawn (shown, or it can't toggle now).
func (m *Model) menuPrefix() (s string, menu [2]int) {
	shown := m.sidebarShown()
	if m.narrow() {
		shown = m.narrowShown()
	}
	if shown || !m.canToggle() {
		return "", menu
	}
	s = theme.Paint(theme.StatusMenu, glyph.Menu)
	if cs := m.cur(); cs == nil || cs.browse == nil {
		if n, priority := m.a.Activity(); n > 0 {
			s += " "
			if priority {
				s += theme.Paint(theme.SidebarPriority, glyph.Priority)
			}
			s += theme.Paint(theme.SidebarUnread, strconv.Itoa(n))
		}
	}
	return s + " ", [2]int{0, xansi.StringWidth(s)}
}
