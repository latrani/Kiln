package ui

import (
	"fmt"
	"slices"
	"strings"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/glyph"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

// compareChars orders characters for the sidebar and the picker; see
// app.CompareChars.
func compareChars(a, b config.Character) int { return app.CompareChars(a, b) }

// open opens configured character k with its view, preloading its
// history, and returns it; an open character is returned as is. It
// returns nil if k isn't configured. It doesn't connect. With nothing
// active, k becomes active.
func (m *Model) open(k string) *charState {
	if cs := m.chars[k]; cs != nil {
		return cs
	}
	c, err := m.a.Open(k)
	if c == nil {
		return nil
	}
	cs := &charState{Char: c, in: NewInput()}
	if err == nil {
		err = cs.compileLook()
	}
	if err != nil {
		m.setStatus(true, str.StatusCharError(k, err))
	}
	m.chars[k] = cs
	m.showLines(cs)
	cs.sb.MarkSeen() // history isn't news
	return cs
}

// close stops an open character's session and removes it from the
// sidebar. If it was active, the next one down becomes active, or the one
// above if it was last.
func (m *Model) close(k string) {
	if m.chars[k] == nil {
		return
	}
	prev := m.a.Active()
	m.a.Close(k)
	m.closed(k, prev)
	m.activated(prev)
}

// closed does the screen's part of closing k, after the core has, when
// prev was active: an editor open on it keeps its edits as a draft, and
// its view goes.
func (m *Model) closed(k, prev string) {
	if k == prev {
		m.leaveEditor()
	}
	if e := m.parked[k]; e != nil {
		delete(m.parked, k)
		m.stashDraft(e)
	}
	delete(m.chars, k)
}

// rowKind says what a sidebar row is.
type rowKind int

const (
	rowWorld        rowKind = iota // a world header
	rowChar                        // a character
	rowGap                         // the blank row above rowAdd
	rowAdd                         // "+ Connection"
	rowAddChar                     // the picker's "+ Character", ending a world; the name field while adding one
	rowAddCharError                // a row of why that name won't do, under the field
	rowAddWorld                    // the picker's "+ World", ending the list
)

type sidebarRow struct {
	kind  rowKind
	world string
	char  string // character key, for rowChar
	text  string // the row's text, for rowAddCharError
}

// addLabel is the sidebar's last row.
var addLabel = str.SidebarOpenConnection()

// badgeX is the column of a character row's connection badge; clicking a
// × there closes the character.
const badgeX = 1

// priorityMark prefixes the unread count when a line is a priority.
func priorityMark() string { return theme.Paint(theme.SidebarPriority, glyph.Priority) }

// sidebarRows lists the open characters under their worlds, then a blank
// row and the open-connection row.
func (m *Model) sidebarRows() []sidebarRow {
	var rows []sidebarRow
	lastWorld := ""
	for _, k := range m.a.Order() {
		w := m.chars[k].Ch.World
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
	rows, focus := m.sidebarRows(), m.a.Active()
	shown := focus // scrolled into view when it changes
	if m.listing() {
		rows, focus = m.pickerRows(), m.picker.sel
		shown = focus
		if e := m.addingChar(); e != nil {
			shown += "\x00" + e.form.reject // a new error under the name comes into view too
		}
	}
	sv := sideView{rows: rows}
	_, bh := m.sideBox()
	if bh == 0 { // not shown: as its column would be (the picker pages by it)
		bh = m.height
	}
	h := max(1, bh-sideTitleH)
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
	if shown != m.sideShown {
		m.sideShown = shown
		if a := slices.IndexFunc(sv.rows, func(r sidebarRow) bool {
			return r.kind == rowChar && r.char == focus || (m.listing() || r.kind == rowWorld) && selKey(r) == focus
		}); a >= 0 {
			if a < sv.top {
				sv = fit(a - 1) // show the row above too (often its world header)
			}
			end := a // and the error rows under a name being added
			for end+1 < total && sv.rows[end+1].kind == rowAddCharError && end+1-a < sv.avail {
				end++
			}
			for end >= sv.top+sv.avail {
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
	return cs.State == session.Disconnected || cs.State == session.Failed
}

// sidebarLine draws one row: the connection badge on the left (blank
// when connected), and activity (unread count, "●" for priority) on the
// right, which the name gives way to.
func (m *Model) sidebarLine(r sidebarRow, w int) string {
	switch r.kind {
	case rowWorld:
		if worldSel(r.world) == m.a.Active() {
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
	case cs.State == session.Connecting:
		lead, leadRole = " "+glyph.Connecting+" ", theme.SidebarConnecting
	case closable(cs):
		lead, leadRole = " "+glyph.Remove+" ", theme.SidebarDisconnected
	}
	active := r.char == m.a.Active()
	count, activity := "", ""
	if cs.Unread > 0 {
		count = fmt.Sprintf(" %d", cs.Unread)
		activity = count
		if cs.Priority {
			activity = " ●" + activity
		}
	}
	name := fitName(lead+cs.Ch.Name, w-xansi.StringWidth(activity))
	if !active {
		name, count = theme.Paint(leadRole, name), theme.Paint(theme.SidebarUnread, count)
	}
	line := name + count
	if cs.Priority && cs.Unread > 0 {
		line = name + " " + priorityMark() + count
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

// sideTitleH is the rows above the sidebar's list: its title, which
// names the list (open connections, the picker's, or log mode's
// filters).
const sideTitleH = 2

// sideTitle draws the sidebar's title row y (0 or 1), w wide: label in
// r, then a rule across the column in rule, as the top bar's beside it.
func sideTitle(y int, r, rule theme.Role, label string, w int) string {
	if y == 0 {
		return theme.Paint(r, fit(label, w))
	}
	return theme.Paint(rule, strings.Repeat(glyph.Rule, w))
}
