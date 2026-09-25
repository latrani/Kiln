package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/style"
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
	cs := &charState{key: k, ch: ch, in: NewInput()}
	if err := cs.compile(); err != nil {
		m.setStatus(true, "%s: %v", k, err)
	}
	m.chars[k] = cs
	m.order = append(m.order, k)
	m.sortOrder()
	m.preload(cs)
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
	if m.active != k {
		return
	}
	m.active = ""
	if len(m.order) > 0 {
		m.switchTo(m.order[min(i, len(m.order)-1)])
	}
}

// rowKind says what a sidebar row is. #41 adds rows for adding a
// character or a world to the picker.
type rowKind int

const (
	rowWorld rowKind = iota // a world header
	rowChar                 // a character
	rowAdd                  // "+ Add connection"
)

type sidebarRow struct {
	kind  rowKind
	world string
	char  string // character key, for rowChar
}

// addLabel is the sidebar's last row.
const addLabel = "+ Add connection"

// badgeX is the column of a character row's connection badge; clicking a
// × there closes the character.
const badgeX = 2

// attentionMark prefixes the unread count when a line needed attention.
var attentionMark = style.SGR(config.HighlightStyle) + "●" + style.Reset

// sidebarRows lists the open characters under their worlds, then the
// add-connection row.
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
	if m.picker != nil {
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
		if a := slices.IndexFunc(sv.rows, func(r sidebarRow) bool { return r.kind == rowChar && r.char == focus }); a >= 0 {
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
		return bold + fitName(r.world, w) + style.Reset
	case rowAdd:
		return style.Dim(fitName(addLabel, w))
	}
	cs := m.chars[r.char]
	badge := " "
	switch {
	case cs.state == session.Connecting:
		badge = "…"
	case closable(cs):
		badge = "×"
	}
	activity, shown := "", ""
	if cs.unread > 0 {
		activity = fmt.Sprintf(" %d", cs.unread)
		shown = activity
		if cs.attention {
			activity = " ●" + activity
			shown = " " + attentionMark + shown
		}
	}
	line := fitName("  "+badge+" "+cs.ch.Name, w-xansi.StringWidth(activity)) + shown
	if r.char == m.active {
		return reverse + line + style.Reset
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
