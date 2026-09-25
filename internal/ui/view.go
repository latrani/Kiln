package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/style"
)

// Minimum usable terminal size.
const (
	MinWidth  = 40
	MinHeight = 10
)

const (
	reverse = "\x1b[7m"
	red     = "\x1b[31m"
	bold    = "\x1b[1m"
)

type layout struct {
	sw, rw     int // sidebar width; right pane width
	sbH        int // scrollback rows
	inRows     []string
	inTop      int // input row shown first, when it's too tall to fit
	curRow     int // cursor row within inRows
	curCol     int
	pillW      int
	masked     bool
	inputLimit int
}

func (m *Model) layout() layout {
	l := layout{sw: min(22, max(12, m.width/5))}
	l.rw = max(1, m.width-l.sw-1)
	cs := m.cur()
	if cs == nil {
		l.inRows = []string{"> "}
		l.curCol = 2
	} else {
		l.masked = cs.needPW
		l.inputLimit = cs.ch.MaxLineBytes
		rows, r, c := cs.in.Render(l.rw, l.inputLimit, cs.ch.NewlineMode == "flatten", l.masked)
		maxIn := max(1, m.height/3)
		top := 0
		if len(rows) > maxIn {
			top = min(max(0, r-maxIn+1), len(rows)-maxIn)
		}
		l.inRows = rows[top:min(len(rows), top+maxIn)]
		l.inTop, l.curRow, l.curCol = top, r-top, c
	}
	l.sbH = max(1, m.height-len(l.inRows)-3) // two rules + statusline
	if cs != nil && cs.sb.Scrolled() {
		l.pillW = xansi.StringWidth(pillText(cs))
	}
	return l
}

// pillText is the "jump to live" marker shown while scrolled up.
func pillText(cs *charState) string {
	if n := cs.sb.Unseen(); n > 0 {
		return fmt.Sprintf(" ▼ %d new ", n)
	}
	return " ▼ more "
}

type sidebarRow struct {
	world string
	char  string // "" for a world header
}

func (m *Model) sidebarRows() []sidebarRow {
	var rows []sidebarRow
	lastWorld := ""
	for _, k := range m.order {
		cs := m.chars[k]
		if cs.ch.World != lastWorld {
			lastWorld = cs.ch.World
			rows = append(rows, sidebarRow{world: lastWorld})
		}
		if !m.collapsed[lastWorld] {
			rows = append(rows, sidebarRow{world: lastWorld, char: k})
		}
	}
	return rows
}

// sideView is the part of the sidebar that fits on screen. When the rows
// overflow, a "▴ N more" row replaces the top row and a "▾ N more" row
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
	sv := sideView{rows: m.sidebarRows()}
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
	if m.active != m.sideShown {
		m.sideShown = m.active
		if a := slices.IndexFunc(sv.rows, func(r sidebarRow) bool { return r.char == m.active }); a >= 0 {
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

func (m *Model) sidebarLine(r sidebarRow, w int) string {
	if r.char == "" {
		arrow := "▾ "
		if m.collapsed[r.world] {
			arrow = "▸ "
		}
		return bold + fit(arrow+r.world, w) + style.Reset
	}
	cs := m.chars[r.char]
	badge := "✕"
	switch {
	case cs.attention:
		badge = "●"
	case cs.state == session.Connected:
		badge = "○"
	case cs.state == session.Connecting:
		badge = "…"
	}
	count := ""
	if cs.unread > 0 {
		count = fmt.Sprintf(" %d", cs.unread)
	}
	name := fit("  "+badge+" "+cs.ch.Name, w-xansi.StringWidth(count))
	line := name + count
	if r.char == m.active {
		return reverse + line + style.Reset
	}
	return line
}

// fit truncates or pads s to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = xansi.Truncate(s, w, "")
	if pad := w - xansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// statusLine shows the active character, its connection and the clock,
// or, while there is a status message, just the character and the message.
// In browse mode it starts with "BROWSE · N selected".
func (m *Model) statusLine(w int) string {
	cs := m.cur()
	name := ""
	if cs != nil {
		name = cs.ch.World + "/" + cs.ch.Name
		if cs.ch.TLS {
			name += " 🔒"
		}
		if cs.browse != nil {
			name = fmt.Sprintf("%sBROWSE%s · %d selected · %s", bold, style.Reset, len(cs.browse.selection()), name)
		}
	}
	if m.status != "" {
		msg := m.status
		if m.statusErr {
			msg = red + msg + style.Reset
		}
		if name == "" {
			return fit(msg, w)
		}
		return fit(name+" · "+msg, w)
	}
	parts := []string{}
	if cs != nil {
		parts = append(parts, name, cs.state.String())
	}
	parts = append(parts, m.d.Now().Format("15:04"))
	return fit(strings.Join(parts, " · "), w)
}

// View draws the whole screen.
func (m *Model) View() tea.View {
	v := tea.View{AltScreen: true, MouseMode: tea.MouseModeCellMotion}
	if m.width < MinWidth || m.height < MinHeight {
		v.Content = fmt.Sprintf("Kiln needs at least %dx%d (now %dx%d)", MinWidth, MinHeight, m.width, m.height)
		return v
	}
	l := m.layout()
	right := make([]string, 0, m.height)
	cs := m.cur()
	var cursor *tea.Cursor
	if cs != nil && cs.browse != nil {
		rows, x, y, show := cs.browse.view(l.rw, m.height-1)
		right = append(rows, m.statusLine(l.rw))
		if show {
			cursor = tea.NewCursor(l.sw+1+x, y)
		}
	} else if cs == nil {
		right = append(right, make([]string, l.sbH)...)
		right[0] = style.Dim("No characters yet: add one in " + m.d.ConfigDir + "/worlds/")
	} else {
		cs.sb.SetWidth(l.rw)
		rows := cs.sb.View(l.sbH)
		if cs.sb.Scrolled() {
			pill := pillText(cs)
			last := len(rows) - 1
			rows[last] = fit(rows[last], l.rw-xansi.StringWidth(pill)) + style.Reset + reverse + pill + style.Reset
		}
		right = append(right, rows...)
	}
	if cs == nil || cs.browse == nil {
		rule := style.Dim(strings.Repeat("─", l.rw))
		right = append(right, rule)
		right = append(right, l.inRows...)
		right = append(right, rule, m.statusLine(l.rw))
		cursor = tea.NewCursor(l.sw+1+l.curCol, l.sbH+1+l.curRow)
	}

	sv := m.sidebarView()
	var b strings.Builder
	for y := 0; y < m.height; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		switch r, hint := sv.at(y); {
		case hint < 0:
			b.WriteString(style.Dim(fit(fmt.Sprintf("▴ %d more", sv.top), l.sw)))
		case hint > 0:
			b.WriteString(style.Dim(fit(fmt.Sprintf("▾ %d more", len(sv.rows)-sv.top-sv.avail), l.sw)))
		case r != nil:
			b.WriteString(m.sidebarLine(*r, l.sw))
		default:
			b.WriteString(strings.Repeat(" ", l.sw))
		}
		b.WriteString(style.Dim("│"))
		if y < len(right) {
			b.WriteString(fit(right[y], l.rw) + style.Reset)
		}
	}
	v.Content = b.String()
	v.Cursor = cursor
	return v
}
