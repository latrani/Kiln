package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

// Minimum usable terminal size.
const (
	MinWidth  = 40
	MinHeight = 10
)

type layout struct {
	sw, rw int // sidebar width; right pane width
	top    int // rows above the body: the top bar and its rule
	sbH    int // scrollback rows
	inRows []string
	inTop  int  // input row shown first, when it's too tall to fit
	prompt bool // inRows is a prompt, not the editable text
	curRow int  // cursor row within inRows
	curCol int
	pillW  int
}

// SidebarWidth is the sidebar's width on a screen w columns wide: a
// fifth of it, at least wide enough for the add chips, at most 22.
func SidebarWidth(w int) int {
	chips := 2 + max(xansi.StringWidth(addLabel), 1+xansi.StringWidth(addCharLabel), xansi.StringWidth(addWorldLabel))
	return min(22, max(chips, w/5))
}

func (m *Model) layout() layout {
	l := layout{sw: SidebarWidth(m.width), top: topH}
	l.rw = max(1, m.width-l.sw-1)
	cs := m.cur()
	if rows, row, col, ok := m.prompt(cs); ok {
		// A tall form gets up to half the screen, scrolled to its focus.
		maxP, top := max(3, m.height/2), 0
		if len(rows) > maxP {
			top = min(max(0, row-maxP+1), len(rows)-maxP)
			rows = rows[top : top+maxP]
		}
		for _, r := range rows {
			l.inRows = append(l.inRows, fit(r, l.rw))
		}
		l.curRow, l.curCol, l.prompt = row-top, min(col, l.rw-1), true
	} else {
		limit, flatten := 0, false
		if cs != nil {
			limit, flatten = cs.ch.MaxLineBytes, cs.ch.NewlineMode == "flatten"
		}
		rows, r, c := m.input().Render(l.rw, limit, flatten, false)
		maxIn := max(1, m.height/3)
		top := 0
		if len(rows) > maxIn {
			top = min(max(0, r-maxIn+1), len(rows)-maxIn)
		}
		l.inRows = rows[top:min(len(rows), top+maxIn)]
		l.inTop, l.curRow, l.curCol = top, r-top, c
	}
	l.sbH = max(1, m.height-l.top-len(l.inRows)-3) // top bar and rule; two rules + statusline
	if cs != nil && cs.sb.Scrolled() {
		l.pillW = xansi.StringWidth(pillText(cs))
	}
	return l
}

// prompt is what the input area shows instead of the editable text while
// Kiln is asking something, or hinting at what Enter will do: understated
// text with no "›" (starting where the "›" would), so it never looks
// like a line bound for the server.
// It returns the rows (one, except for a form with several fields) and
// the cursor's row and column.
func (m *Model) prompt(cs *charState) (rows []string, row, col int, ok bool) {
	hint := func(s string) ([]string, int, int, bool) {
		return []string{theme.Paint(theme.InputHint, s)}, 0, 0, true
	}
	switch {
	case m.mode == modeSavePassword:
		name := m.pendingCh[1]
		if pc := m.chars[key(m.pendingCh[0], m.pendingCh[1])]; pc != nil {
			name = pc.ch.Name
		}
		return hint(str.ViewSavePassword(name, storeName(m.passwordStore())))
	case m.picker != nil:
		f := m.picker.form
		if m.picker.edit != nil {
			f = m.picker.edit.form
		}
		rows, row, col := f.rows()
		return rows, row, col, true
	case cs == nil && m.idle.Empty():
		return hint(str.ViewNothingOpen())
	case cs == nil:
		return nil, 0, 0, false
	case cs.needPW:
		// Bullets for what's typed, between a label and the keys to press.
		rows, _, c := cs.in.Render(1<<20, 0, false, true)
		label := str.ViewPasswordLabel(cs.ch.Name)
		text := theme.Paint(theme.InputHint, label) + strings.TrimPrefix(rows[0], gutterMark) + theme.Paint(theme.InputHint, str.ViewPasswordKeys())
		return []string{text}, 0, c - gutterWidth + xansi.StringWidth(label), true
	case !cs.in.Empty() || cs.state == session.Connected:
		return nil, 0, 0, false
	case cs.pin != nil:
		return hint(str.ViewCertChanged())
	case cs.state == session.Connecting:
		return hint(str.ViewConnecting())
	case cs.state == session.Failed:
		return hint(str.ViewConnectFailed())
	}
	return hint(str.ViewDisconnected())
}

// storeName describes a password_store setting for the save prompt.
func storeName(store string) string {
	if store == "file" {
		return str.ViewStoreFile()
	}
	return str.ViewStoreKeychain()
}

// pillText is the "jump to live" marker shown while scrolled up.
func pillText(cs *charState) string {
	if n := cs.sb.Unseen(); n > 0 {
		return " " + str.ViewPillNew(n) + " "
	}
	return " " + str.ViewPillMore() + " "
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

// statusLine is the bottom bar: a status message while there is one
// (log mode's included; see takeLogStatus), else in log mode how many
// lines are selected, else the connection. Nothing in it changes by the
// minute.
func (m *Model) statusLine(w int) string {
	cs := m.cur()
	msg, isErr := m.status, m.statusErr
	switch {
	case msg != "" && isErr:
		msg = theme.Paint(theme.StatusError, msg)
	case msg != "":
	case cs == nil:
	case cs.browse != nil:
		msg = str.ViewSelected(len(cs.browse.selection()))
	case cs.state == session.Connected:
		msg = m.connectedSince(cs)
	default:
		msg = stateName(cs.state)
	}
	return fitName(msg, w)
}

// connectedSince says when the connection came up: the time, with the day
// first if that wasn't today.
func (m *Model) connectedSince(cs *charState) string {
	at, now := cs.connectedAt.Local(), m.d.Now().Local()
	t := at.Format("15:04")                                  //str:ok
	if at.Format("2006-01-02") != now.Format("2006-01-02") { //str:ok
		return str.ViewConnectedSinceDay(at.Format(str.DateDay()), t)
	}
	return str.ViewConnectedSince(t)
}

// topH is the rows above the body: the top bar and its rule.
const topH = 2

// topBar draws the top status bar: the character and, in log mode, its
// find status on the left; the Filter chip (log mode) and the Log chip
// pinned right, Log at the far right. It says which columns each chip
// takes, [from, to); zero when it isn't drawn whole.
func (m *Model) topBar(w int) (line string, logChip, filterChip [2]int) {
	cs := m.cur()
	if cs == nil {
		return "", logChip, filterChip
	}
	left := cs.ch.World + "/" + cs.ch.Name
	logRole, filt := theme.StatusLog, ""
	if b := cs.browse; b != nil {
		logRole = theme.StatusLogOn
		if f := b.findStatus(); f != "" {
			left += "   " + f
		}
		role := theme.StatusFilter
		if b.panel != nil {
			role = theme.StatusFilterOn
		}
		filt = chip(role, str.ViewFilterButton())
	}
	logc := chip(logRole, str.ViewLogButton())
	fw, lw := xansi.StringWidth(filt), xansi.StringWidth(logc)
	start := w - fw - lw
	if start < 2 { // no room for the chips beside a name
		return fitName(left, w), logChip, filterChip
	}
	if fw > 0 {
		filterChip = [2]int{start, start + fw}
	}
	logChip = [2]int{start + fw, w}
	return fitName(left, start-1) + " " + filt + logc, logChip, filterChip
}

// topClick handles a click at column x of the top bar.
func (m *Model) topClick(x int) {
	cs := m.cur()
	if cs == nil {
		return
	}
	_, logc, filt := m.topBar(m.layout().rw)
	switch {
	case x >= logc[0] && x < logc[1]:
		if cs.browse != nil {
			cs.browse = nil // as Esc does
		} else {
			m.openBrowse(cs)
		}
	case x >= filt[0] && x < filt[1] && cs.browse != nil && cs.browse.prompt == promptNone:
		cs.browse.togglePanel()
	}
}

// chip draws something you can click: its label with a space either
// side, in r.
func chip(r theme.Role, label string) string { return theme.Paint(r, " "+label+" ") }

// modal reports whether the picker or an editor owns the input area; the
// scrollback is then a backdrop.
func (m *Model) modal() bool { return m.picker != nil }

// listing reports whether the sidebar shows the picker's list. An editor
// opened by /edit leaves the open characters there instead.
func (m *Model) listing() bool {
	return m.picker != nil && (m.picker.edit == nil || !m.picker.edit.closeAll)
}

// View draws the whole screen.
func (m *Model) View() tea.View {
	v := tea.View{AltScreen: true, MouseMode: tea.MouseModeAllMotion, ReportFocus: true} // all motion: links light up on hover; focus: notifications
	if m.width < MinWidth || m.height < MinHeight {
		v.Content = str.ViewTooSmall(MinWidth, MinHeight, m.width, m.height)
		return v
	}
	l := m.layout()
	right := make([]string, 0, m.height)
	cs := m.cur()
	var cursor *tea.Cursor
	top, _, _ := m.topBar(l.rw)
	rule := strings.Repeat("─", l.rw)
	right = append(right, theme.Fill(theme.Status, top, l.rw), theme.Paint(theme.RuleStatus, rule))
	if cs != nil && cs.browse != nil {
		rows, x, y, show := cs.browse.view(l.rw, m.height-l.top-1)
		right = append(right, rows...)
		right = append(right, theme.Fill(theme.Status, m.statusLine(l.rw), l.rw))
		if show {
			cursor = tea.NewCursor(l.sw+1+x, l.top+y)
		}
	} else if cs == nil {
		right = append(right, make([]string, l.sbH)...)
		if len(m.allChars()) == 0 {
			right[l.top] = theme.Paint(theme.ScrollbackEmpty, str.ViewNoCharacters())
		}
	} else {
		cs.sb.SetWidth(l.rw)
		rows := cs.sb.View(l.sbH)
		if m.modal() {
			for i, r := range rows {
				rows[i] = theme.Paint(theme.ScrollbackInactive, ansi.Strip(r))
			}
		}
		if cs.sb.Scrolled() {
			pill := pillText(cs)
			last := len(rows) - 1
			rows[last] = fit(rows[last], l.rw-xansi.StringWidth(pill)) + theme.Reset + theme.Paint(theme.ScrollbackPill, pill) + theme.Reset
		}
		right = append(right, rows...)
	}
	if cs == nil || cs.browse == nil {
		// Each rule takes the color of the area below it.
		area, topRule := theme.Input, theme.RuleInput
		if m.modal() {
			area, topRule = theme.Form, theme.RuleForm
		}
		right = append(right, theme.Paint(topRule, rule))
		for _, r := range l.inRows {
			right = append(right, theme.Fill(area, r, l.rw))
		}
		right = append(right, theme.Paint(theme.RuleStatus, rule), theme.Fill(theme.Status, m.statusLine(l.rw), l.rw))
		cursor = tea.NewCursor(l.sw+1+l.curCol, l.top+l.sbH+1+l.curRow)
	}

	side := theme.Sidebar
	if m.listing() {
		side = theme.Picker
	}
	var panel []string
	if cs != nil && cs.browse != nil && cs.browse.panel != nil {
		side = theme.Filter
		panel = cs.browse.panelView(l.sw, m.height)
	}
	sv := m.sidebarView()
	var b strings.Builder
	for y := 0; y < m.height; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		switch r, hint := sv.at(y); {
		case panel != nil:
			b.WriteString(theme.Fill(side, panel[y], l.sw))
		case hint < 0:
			b.WriteString(theme.Fill(side, theme.Paint(theme.SidebarMore, fit(str.ViewMoreAbove(sv.top), l.sw)), l.sw))
		case hint > 0:
			b.WriteString(theme.Fill(side, theme.Paint(theme.SidebarMore, fit(str.ViewMoreBelow(len(sv.rows)-sv.top-sv.avail), l.sw)), l.sw))
		case r != nil && m.listing():
			b.WriteString(theme.Fill(side, m.pickerLine(*r, l.sw), l.sw))
		case r != nil:
			b.WriteString(theme.Fill(side, m.sidebarLine(*r, l.sw), l.sw))
		default:
			b.WriteString(theme.Fill(side, "", l.sw))
		}
		b.WriteString(theme.Paint(theme.Divider, "│"))
		if y < len(right) {
			b.WriteString(fit(right[y], l.rw) + style.Reset)
		}
	}
	v.Content = b.String()
	v.Cursor = cursor
	return v
}

// stateName is a connection state as the statusline says it.
func stateName(s session.State) string {
	switch s {
	case session.Connecting:
		return str.StateConnecting()
	case session.Connected:
		return str.StateConnected()
	case session.Failed:
		return str.StateFailed()
	}
	return str.StateDisconnected()
}
