package ui

import (
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/app"
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
	if m.overviewing() { // no input box: the overview has the pane down to the statusline
		l.sbH = max(1, m.height-l.top-2) // top bar and rule; rule + statusline
		return l
	}
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
			limit, flatten = cs.Ch.MaxLineBytes, cs.Ch.NewlineMode == "flatten"
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
	case m.asking():
		world, char, _ := m.a.PendingSave()
		name := char
		if pc := m.chars[key(world, char)]; pc != nil {
			name = pc.Ch.Name
		}
		return hint(str.ViewSavePassword(name, storeName(m.a.PasswordStore())))
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
	case cs.NeedPW:
		// Bullets for what's typed, between a label and the keys to press.
		rows, _, c := cs.in.Render(1<<20, 0, false, true)
		label := str.ViewPasswordLabel(cs.Ch.Name)
		text := theme.Paint(theme.InputHint, label) + strings.TrimPrefix(rows[0], gutterMark) + theme.Paint(theme.InputHint, str.ViewPasswordKeys())
		return []string{text}, 0, c - gutterWidth + xansi.StringWidth(label), true
	case !cs.in.Empty() || cs.State == session.Connected:
		return nil, 0, 0, false
	case cs.Pin != nil:
		return hint(str.ViewCertChanged())
	case cs.State == session.Connecting:
		return hint(str.ViewConnecting())
	case cs.State == session.Failed:
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
	msg, isErr := m.a.Status().Text, m.a.Status().Err
	switch {
	case msg != "" && isErr:
		msg = theme.Paint(theme.StatusError, msg)
	case msg != "":
	case cs == nil:
	case cs.browse != nil:
		msg = str.ViewSelected(len(cs.browse.selection()))
	case cs.State == session.Connected:
		msg = m.connectedSince(cs)
	default:
		msg = stateName(cs.State)
	}
	return fitName(msg, w)
}

// connectedSince says when the connection came up: the time, with the day
// first if that wasn't today.
func (m *Model) connectedSince(cs *charState) string {
	at, now := cs.ConnectedAt.Local(), m.d.Now().Local()
	t := at.Format("15:04")                                  //str:ok
	if at.Format("2006-01-02") != now.Format("2006-01-02") { //str:ok
		return str.ViewConnectedSinceDay(at.Format(str.DateDay()), t)
	}
	return str.ViewConnectedSince(t)
}

// topH is the rows above the body: the top bar and its rule.
const topH = 2

// topBar draws the top status bar: the character and, in log mode, its
// find status on the left; on the right, pinned, the presence chip, the
// Filter chip (log mode) and the Log chip at the far right, a spaced dot
// between each. It says which columns each chip takes, [from, to); zero
// when it isn't drawn whole.
func (m *Model) topBar(w int) (line string, logChip, filterChip, presenceChip [2]int) {
	return m.topBarFor(w, m.shownPresence)
}

// topBarFor is topBar with the presence chip as it is for p, which
// decides the chip's width; a click lands on the layout it was made on.
func (m *Model) topBarFor(w int, p app.Presence) (line string, logChip, filterChip, presenceChip [2]int) {
	cs := m.cur()
	if cs == nil {
		world, _ := m.a.ActiveWorld() // "" with nothing open
		return fitName(world, w), logChip, filterChip, presenceChip
	}
	left := cs.Ch.World + str.Separator() + cs.Ch.Name
	logRole := theme.StatusLog
	type part struct {
		text string
		span *[2]int
	}
	presLabel, presRole := str.ViewPresenceUnknown(), theme.StatusPresenceUnknown
	switch p {
	case app.PresenceHere:
		presLabel, presRole = str.ViewPresenceHere(), theme.StatusPresenceHere
	case app.PresenceAway:
		presLabel, presRole = str.ViewPresenceAway(), theme.StatusPresenceAway
	}
	parts := []part{{chip(presRole, presLabel), &presenceChip}}
	if b := cs.browse; b != nil {
		logRole = theme.StatusLogOn
		if f := b.findStatus(); f != "" {
			left += "   " + f
		}
		role := theme.StatusFilter
		if b.panel != nil {
			role = theme.StatusFilterOn
		}
		parts = append(parts, part{chip(role, str.ViewFilterButton()), &filterChip})
	}
	parts = append(parts, part{chip(logRole, str.ViewLogButton()), &logChip})

	sep := str.Separator()
	width := func() int {
		n := xansi.StringWidth(sep) * (len(parts) - 1)
		for _, pt := range parts {
			n += xansi.StringWidth(pt.text)
		}
		return n
	}
	// Too tight beside a name: Filter goes first, then presence; Log stays.
	for len(parts) > 1 && w-width() < 2 {
		drop := 0 // presence
		if len(parts) == 3 {
			drop = 1 // Filter
		}
		parts = slices.Delete(parts, drop, drop+1)
	}
	start := w - width()
	if start < 2 { // no room for a chip at all
		return fitName(left, w), [2]int{}, [2]int{}, [2]int{}
	}
	x := start
	texts := make([]string, len(parts))
	for i, pt := range parts {
		texts[i] = pt.text
		*pt.span = [2]int{x, x + xansi.StringWidth(pt.text)}
		x += xansi.StringWidth(pt.text) + xansi.StringWidth(sep)
	}
	return fitName(left, start-1) + " " + strings.Join(texts, sep), logChip, filterChip, presenceChip
}

// topClick handles a click at column x of the top bar. was is the
// presence before the click counted as you being here: clicking the
// presence chip sets Away unless you already were, and then it's the
// click itself that ended it.
func (m *Model) topClick(x int, was app.Presence) {
	cs := m.cur()
	if cs == nil {
		return
	}
	_, logc, filt, pres := m.topBarFor(m.layout().rw, was)
	switch {
	case x >= pres[0] && x < pres[1]:
		if was != app.PresenceAway {
			m.a.SetAway()
		}
	case x >= logc[0] && x < logc[1]:
		if cs.browse != nil {
			cs.hideBrowse() // as Ctrl+L does
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
	top, _, _, _ := m.topBar(l.rw)
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
		if w, ok := m.a.ActiveWorld(); ok {
			copy(right[l.top:], m.overview(w, l.rw, l.sbH))
		} else if len(m.a.AllChars()) == 0 {
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
	if m.overviewing() {
		right = append(right, theme.Paint(theme.RuleStatus, rule), theme.Fill(theme.Status, m.statusLine(l.rw), l.rw))
	} else if cs == nil || cs.browse == nil {
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
	foot := m.footerH()
	var b strings.Builder
	for y := 0; y < m.height; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		switch r, hint := sv.at(y); {
		case panel != nil:
			b.WriteString(theme.Fill(side, panel[y], l.sw))
		case y >= m.height-foot:
			b.WriteString(theme.Fill(side, fit(chip(theme.SidebarAction, footerLabels[y-(m.height-foot)]), l.sw), l.sw))
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

// overviewLines is how many of each character's last lines a world's
// overview shows.
const overviewLines = 5

// overview draws world's overview, h rows of width w from row m.ovTop
// on: each open character's name and when its last line came, then those
// last lines, with a rule between characters. It notes which character
// each row shows in m.ovKeys, for clicks.
func (m *Model) overview(world string, w, h int) []string {
	var rows, keys []string
	for _, k := range m.a.Order() {
		cs := m.chars[k]
		if cs.Ch.World != world {
			continue
		}
		if len(rows) > 0 {
			rows, keys = append(rows, theme.Paint(theme.ScrollbackRule, strings.Repeat("─", w))), append(keys, "")
		}
		when := str.ViewOverviewQuiet()
		if t, ok := m.a.LastTime(k); ok {
			when = m.clock(t)
		}
		rows, keys = append(rows, theme.Paint(theme.ScrollbackOverview, cs.Ch.Name)+theme.Paint(theme.ScrollbackSys, str.Separator()+when)), append(keys, k)
		for _, l := range cs.sb.Tail(overviewLines) {
			rows, keys = append(rows, xansi.Truncate(l, w, "…")+style.Reset), append(keys, k)
		}
	}
	if m.modal() {
		for i, r := range rows {
			rows[i] = theme.Paint(theme.ScrollbackInactive, ansi.Strip(r))
		}
	}
	m.ovTop = min(max(0, m.ovTop), max(0, len(rows)-h))
	end := min(len(rows), m.ovTop+h)
	m.ovKeys = keys[m.ovTop:end]
	return rows[m.ovTop:end]
}

// overviewing reports whether a world's overview has the pane, with
// nothing asking in the input area: then there's no input box.
func (m *Model) overviewing() bool {
	_, ok := m.a.ActiveWorld()
	return ok && m.picker == nil && !m.asking()
}

// asking reports whether the save-password question has the input area.
func (m *Model) asking() bool {
	_, _, ok := m.a.PendingSave()
	return ok
}

// scrollOverview moves the overview by delta rows; the next draw clamps it.
func (m *Model) scrollOverview(delta int) { m.ovTop = max(0, m.ovTop+delta) }

// clock is t as a time of day, with the day first if that wasn't today.
func (m *Model) clock(t time.Time) string {
	t = t.Local()
	if t.Format("2006-01-02") != m.d.Now().Local().Format("2006-01-02") { //str:ok
		return t.Format(str.DateDayTime())
	}
	return t.Format("15:04") //str:ok
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
