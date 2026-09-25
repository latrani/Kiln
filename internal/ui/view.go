package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/style"
)

// emptyHint is the input area's prompt while nothing is open.
const emptyHint = "Nothing open · Enter or Ctrl+O to add a connection"

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
	sw, rw int // sidebar width; right pane width
	sbH    int // scrollback rows
	inRows []string
	inTop  int  // input row shown first, when it's too tall to fit
	prompt bool // inRows is a prompt, not the editable text
	curRow int  // cursor row within inRows
	curCol int
	pillW  int
}

func (m *Model) layout() layout {
	l := layout{sw: min(22, max(12, m.width/5))}
	l.rw = max(1, m.width-l.sw-1)
	cs := m.cur()
	if text, col, ok := m.prompt(cs); ok {
		l.inRows, l.curCol, l.prompt = []string{fit(text, l.rw)}, min(col, l.rw-1), true
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
	l.sbH = max(1, m.height-len(l.inRows)-3) // two rules + statusline
	if cs != nil && cs.sb.Scrolled() {
		l.pillW = xansi.StringWidth(pillText(cs))
	}
	return l
}

// prompt is what the input area shows instead of the editable text while
// Kiln is asking something, or hinting at what Enter will do: understated
// text with no "›" (starting where the "›" would), so it never looks
// like a line bound for the server.
// It returns the row and the cursor's column.
func (m *Model) prompt(cs *charState) (text string, col int, ok bool) {
	hint := func(s string) (string, int, bool) { return style.Dim(s), 0, true }
	switch {
	case m.mode == modeSavePassword:
		name := m.pendingCh[1]
		if pc := m.chars[key(m.pendingCh[0], m.pendingCh[1])]; pc != nil {
			name = pc.ch.Name
		}
		return hint(fmt.Sprintf("Save password for %s in %s? [Y/n]", name, storeName(m.passwordStore())))
	case m.picker != nil:
		text, col := m.picker.form.row()
		return text, col, true
	case cs == nil && m.idle.Empty():
		return hint(emptyHint)
	case cs == nil:
		return "", 0, false
	case cs.needPW:
		// Bullets for what's typed, between a label and the keys to press.
		rows, _, c := cs.in.Render(1<<20, 0, false, true)
		label := fmt.Sprintf("Password for %s: ", cs.ch.Name)
		text = style.Dim(label) + strings.TrimPrefix(rows[0], gutterMark) + style.Dim("   Enter to log in · Esc to skip")
		return text, c - gutterWidth + xansi.StringWidth(label), true
	case !cs.in.Empty() || cs.state == session.Connected:
		return "", 0, false
	case cs.pin != nil:
		return hint("Certificate changed · /trust to accept it")
	case cs.state == session.Connecting:
		return hint("Connecting…")
	case cs.state == session.Failed:
		return hint("Connection failed · Enter to retry")
	}
	return hint("Disconnected · Enter to connect")
}

// storeName describes a password_store setting for the save prompt.
func storeName(store string) string {
	if store == "file" {
		return "the password file"
	}
	return "the keychain"
}

// pillText is the "jump to live" marker shown while scrolled up.
func pillText(cs *charState) string {
	if n := cs.sb.Unseen(); n > 0 {
		return fmt.Sprintf(" ▼ %d new ", n)
	}
	return " ▼ more "
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
		if len(m.allChars()) == 0 {
			right[0] = style.Dim("No characters yet: add one in " + m.d.ConfigDir + "/worlds/")
		}
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
			b.WriteString(style.Dim(fit(fmt.Sprintf("▲ %d more", sv.top), l.sw)))
		case hint > 0:
			b.WriteString(style.Dim(fit(fmt.Sprintf("▼ %d more", len(sv.rows)-sv.top-sv.avail), l.sw)))
		case r != nil && m.picker != nil:
			b.WriteString(m.pickerLine(*r, l.sw))
		case r != nil:
			b.WriteString(m.sidebarLine(*r, l.sw))
		case y == 0 && m.picker != nil:
			b.WriteString(style.Dim(fitName(noMatches, l.sw)))
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
