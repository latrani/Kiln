package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

// compareChars orders characters alphabetically, ignoring case: by world
// id, then by name. The sidebar and the picker both use it.
func compareChars(a, b config.Character) int {
	return cmp.Or(
		cmp.Compare(strings.ToLower(a.World), strings.ToLower(b.World)),
		cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
		cmp.Compare(key(a.World, a.ID), key(b.World, b.ID)),
	)
}

// allChars is every configured character, in sidebar order.
func (m *Model) allChars() []config.Character {
	var all []config.Character
	if m.cfg != nil {
		for _, w := range m.cfg.Worlds {
			all = append(all, w.Characters...)
		}
	}
	slices.SortFunc(all, compareChars)
	return all
}

// find looks up a configured character by key.
func (m *Model) find(k string) (config.Character, bool) {
	if m.cfg != nil {
		for _, w := range m.cfg.Worlds {
			for _, ch := range w.Characters {
				if key(ch.World, ch.ID) == k {
					return ch, true
				}
			}
		}
	}
	return config.Character{}, false
}

// sortOrder puts the open characters in sidebar order.
func (m *Model) sortOrder() {
	slices.SortFunc(m.order, func(a, b string) int { return compareChars(m.chars[a].ch, m.chars[b].ch) })
}

// open adds configured character k to the sidebar, preloading its
// history, and returns it; an open character is returned as is. It
// returns nil if k isn't configured. It doesn't connect. With nothing
// active, k becomes active.
func (m *Model) open(k string) *charState {
	if cs := m.chars[k]; cs != nil {
		return cs
	}
	ch, ok := m.find(k)
	if !ok {
		return nil
	}
	cs := &charState{key: k, ch: ch, in: NewInput(), sentGen: -1}
	if _, err := cs.compile(); err != nil {
		m.setStatus(true, str.StatusCharError(k, err))
	}
	m.chars[k] = cs
	m.order = append(m.order, k)
	m.sortOrder()
	m.preload(cs)
	cs.sb.MarkSeen() // history isn't news
	if m.active == "" {
		m.active = k
	}
	return cs
}

// close stops an open character's session and removes it from the
// sidebar. If it was active, the next one down becomes active, or the one
// above if it was last.
func (m *Model) close(k string) {
	i := slices.Index(m.order, k)
	if i < 0 {
		return
	}
	if cs := m.chars[k]; cs.cancel != nil {
		cs.cancel()
	}
	delete(m.chars, k)
	m.order = slices.Delete(m.order, i, i+1)
	m.recent = slices.DeleteFunc(m.recent, func(r string) bool { return r == k })
	if w, ok := m.activeWorld(); ok && !m.worldOpen(w) {
		k = m.active // its last character closed: the world's row goes too
	}
	if m.active != k {
		return
	}
	m.active = ""
	if len(m.order) > 0 {
		m.switchTo(m.order[min(i, len(m.order)-1)])
	}
}

// activeWorld is the world whose overview is showing, when its sidebar
// row is the active one instead of a character.
func (m *Model) activeWorld() (string, bool) { return strings.CutPrefix(m.active, worldSel("")) }

// worldOpen reports whether any of world's characters are open.
func (m *Model) worldOpen(world string) bool {
	return slices.ContainsFunc(m.order, func(k string) bool { return m.chars[k].ch.World == world })
}

// stops are what Ctrl+↑/↓ steps through: each world's row and each
// character's, in sidebar order, as values m.active takes.
func (m *Model) stops() []string {
	var s []string
	for _, r := range m.sidebarRows() {
		switch r.kind {
		case rowWorld:
			s = append(s, worldSel(r.world))
		case rowChar:
			s = append(s, r.char)
		}
	}
	return s
}

// rowKind says what a sidebar row is.
type rowKind int

const (
	rowWorld    rowKind = iota // a world header
	rowChar                    // a character
	rowGap                     // the blank row above rowAdd
	rowAdd                     // "+ Connection"
	rowAddChar                 // the picker's "+ Character", ending a world
	rowAddWorld                // the picker's "+ World", ending the list
)

type sidebarRow struct {
	kind  rowKind
	world string
	char  string // character key, for rowChar
}

// addLabel is the sidebar's last row.
var addLabel = str.SidebarOpenConnection()

// badgeX is the column of a character row's connection badge; clicking a
// × there closes the character.
const badgeX = 1

// attentionMark prefixes the unread count when a line needed attention.
func attentionMark() string { return theme.Paint(theme.SidebarAttention, "●") }

// sidebarRows lists the open characters under their worlds, then a blank
// row and the open-connection row.
func (m *Model) sidebarRows() []sidebarRow {
	var rows []sidebarRow
	lastWorld := ""
	for _, k := range m.order {
		w := m.chars[k].ch.World
		if w != lastWorld {
			lastWorld = w
			rows = append(rows, sidebarRow{kind: rowWorld, world: w})
		}
		rows = append(rows, sidebarRow{kind: rowChar, world: w, char: k})
	}
	if len(rows) > 0 {
		rows = append(rows, sidebarRow{kind: rowGap})
	}
	return append(rows, sidebarRow{kind: rowAdd})
}

// sideView is the part of the sidebar that fits on screen. When the rows
// overflow, a "▲ N more" row replaces the top row and a "▼ N more" row
// the bottom one, counting the rows hidden past each edge.
type sideView struct {
	rows         []sidebarRow
	top, avail   int // first row shown; rows shown between the hints
	above, below bool
}

// at maps a screen row to a sidebar row, or to a hint: -1 above, 1 below.
func (sv sideView) at(y int) (*sidebarRow, int) {
	if sv.above {
		if y == 0 {
			return nil, -1
		}
		y--
	}
	if y < 0 || y >= sv.avail {
		if sv.below && y == sv.avail {
			return nil, 1
		}
		return nil, 0
	}
	if i := sv.top + y; i < len(sv.rows) {
		return &sv.rows[i], 0
	}
	return nil, 0
}

// sidebarView lays the sidebar out for the screen height. It scrolls the
// active character into view when it changes, and otherwise keeps the
// position the mouse wheel left.
func (m *Model) sidebarView() sideView {
	rows, focus := m.sidebarRows(), m.active
	if m.listing() {
		rows, focus = m.pickerRows(), m.picker.sel
	}
	sv := sideView{rows: rows}
	h := max(1, m.height)
	total := len(sv.rows)
	if total <= h {
		m.sideTop = 0
		sv.avail = total
		return sv
	}
	fit := func(top int) sideView {
		v := sv
		v.top = min(max(0, top), total-h+1) // at the end only the top hint is shown
		v.above = v.top > 0
		v.avail = h
		if v.above {
			v.avail--
		}
		if v.top+v.avail < total {
			v.below = true
			v.avail--
		}
		return v
	}
	sv = fit(m.sideTop)
	if focus != m.sideShown {
		m.sideShown = focus
		if a := slices.IndexFunc(sv.rows, func(r sidebarRow) bool {
			return r.kind == rowChar && r.char == focus || (m.listing() || r.kind == rowWorld) && selKey(r) == focus
		}); a >= 0 {
			if a < sv.top {
				sv = fit(a - 1) // show the row above too (often its world header)
			}
			for a >= sv.top+sv.avail {
				sv = fit(sv.top + 1)
			}
		}
	}
	m.sideTop = sv.top
	return sv
}

// scrollSidebar moves the sidebar by delta rows.
func (m *Model) scrollSidebar(delta int) {
	m.sideTop += delta
	m.sidebarView() // clamp
}

// closable reports whether cs shows a × that closes it.
func closable(cs *charState) bool {
	return cs.state == session.Disconnected || cs.state == session.Failed
}

// sidebarLine draws one row: the connection badge on the left (blank
// when connected), and activity (unread count, "●" for attention) on the
// right, which the name gives way to.
func (m *Model) sidebarLine(r sidebarRow, w int) string {
	switch r.kind {
	case rowWorld:
		if worldSel(r.world) == m.active {
			return theme.Paint(theme.SidebarActive, fitName(r.world, w))
		}
		return theme.Paint(theme.SidebarWorld, fitName(r.world, w))
	case rowGap:
		return fit("", w)
	case rowAdd:
		return fit(chip(theme.SidebarAdd, addLabel), w)
	}
	cs := m.chars[r.char]
	// A connected character's name sits one space in, under its world; a
	// badge (connecting, or disconnected with a × to close) goes there
	// instead and pushes the name over.
	lead, leadRole := " ", theme.SidebarChar
	switch {
	case cs.state == session.Connecting:
		lead, leadRole = " … ", theme.SidebarConnecting
	case closable(cs):
		lead, leadRole = " × ", theme.SidebarDisconnected
	}
	active := r.char == m.active
	count, activity := "", ""
	if cs.unread > 0 {
		count = fmt.Sprintf(" %d", cs.unread)
		activity = count
		if cs.attention {
			activity = " ●" + activity
		}
	}
	name := fitName(lead+cs.ch.Name, w-xansi.StringWidth(activity))
	if !active {
		name, count = theme.Paint(leadRole, name), theme.Paint(theme.SidebarUnread, count)
	}
	line := name + count
	if cs.attention && cs.unread > 0 {
		line = name + " " + attentionMark() + count
	}
	if active {
		// The active style covers the whole row: its parts draw plain
		// inside it, and it comes back after the dot's reset.
		return theme.Paint(theme.SidebarActive, style.Reassert(line, theme.SGR(theme.SidebarActive)))
	}
	return line
}

// fitName is fit for names: too long, they're cut with "…".
func fitName(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return fit(xansi.Truncate(s, w, "…"), w)
}
