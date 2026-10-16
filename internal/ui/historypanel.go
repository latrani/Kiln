package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/theme"
)

// historyShows reports whether the history panel is up in the input
// area: open, and nothing else (the picker, a question) has the area.
func (m *Model) historyShows() bool {
	_, _, _, prompt := m.prompt(m.cur())
	return m.a.History().IsOpen() && !prompt && !m.overviewing()
}

// historyRows draws the history panel's list, h rows of width w, the
// newest line at the bottom. A line is "›" (selected) or a space, its
// first line cut to fit, and when it was sent at the right. The selected
// line stays in view: while it's among the newest h the list is at the
// bottom; above them it's the top row, and a "more" badge says there's
// more below. idx is the Shown index each row draws, -1 for none.
func (m *Model) historyRows(w, h int) (rows []string, idx []int) {
	hist := m.a.History()
	shown, sel := hist.Shown(), hist.Sel()
	if len(shown) == 0 {
		hint := str.ViewHistoryNone()
		if hist.Query() != "" {
			hint = str.ViewHistoryNoMatch()
		}
		rows, idx = make([]string, h), make([]int, h)
		for i := range idx {
			idx[i] = -1
		}
		rows[h-1] = strings.Repeat(" ", gutterWidth) + theme.Paint(theme.InputHint, fitName(hint, w-gutterWidth))
		return rows, idx
	}
	start := max(0, min(sel, len(shown)-h))
	end := min(len(shown), start+h)
	for range h - (end - start) { // few lines: they sit at the bottom
		rows, idx = append(rows, ""), append(idx, -1)
	}
	now := m.d.Now()
	for i := start; i < end; i++ {
		rows, idx = append(rows, historyRow(shown[i], i == sel, w, now)), append(idx, i)
	}
	if end < len(shown) {
		more := " " + str.ViewPillMore() + " "
		last := len(rows) - 1
		rows[last] = fit(rows[last], w-xansi.StringWidth(more)) + theme.Reset + theme.Paint(theme.InputHistoryMore, more) + theme.Reset
	}
	return rows, idx
}

// historyRow draws one line of the history panel.
func historyRow(s app.Sent, selected bool, w int, now time.Time) string {
	text, _, multi := strings.Cut(s.Text, "\n")
	text = ansi.Strip(ansi.Sanitize(text))
	if multi {
		text += "…"
	}
	when := app.Clock(s.At, now)
	textW := max(0, w-gutterWidth-1-xansi.StringWidth(when))
	text = fit(xansi.Truncate(text, textW, "…"), textW)
	mark, textRole, timeRole := gutterBlank, theme.InputHistory, theme.InputHistoryTime
	if selected {
		mark, textRole, timeRole = gutterMark, theme.InputHistorySelected, theme.InputHistoryCursor
	}
	return theme.Paint(textRole, mark+text) + " " + theme.Paint(timeRole, when)
}

// historyKey handles k while the history panel is up: ↑ and ↓ (PgUp
// and PgDn a page) move the selection, ↓ past the newest and Esc close
// it, the draft back. Enter takes the selected line to edit, through
// submit. ok is false for keys the input gets: typing changes the
// query.
func (m *Model) historyKey(k tea.KeyPressMsg) (cmd tea.Cmd, ok bool) {
	page := max(1, len(m.layout().hist)-1)
	switch k.String() {
	case "up":
		cmd = m.run(m.a.HistoryMove(-1))
	case "down":
		cmd = m.run(m.a.HistoryMove(1))
	case "pgup":
		cmd = m.run(m.a.HistoryMove(-page))
	case "pgdown":
		if m.a.History().Sel() < len(m.a.History().Shown())-1 { // a page never closes it
			cmd = m.run(m.a.HistoryMove(min(page, len(m.a.History().Shown())-1-m.a.History().Sel())))
		}
	case "esc":
		m.a.HistoryClose()
	default:
		return nil, false
	}
	m.pullInput(m.cur())
	m.pullIdle()
	return cmd, true
}

// historyClick handles a click on the history panel's line i (Shown's
// index; -1 for none): it's taken to edit.
func (m *Model) historyClick(i int) tea.Cmd {
	if i < 0 {
		return nil
	}
	cmd := m.run(m.a.HistorySelect(i))
	m.a.HistoryPick()
	m.pullInput(m.cur())
	m.pullIdle()
	return cmd
}

// historyToggle is a click on the History chip.
func (m *Model) historyToggle() tea.Cmd {
	cmd := m.run(m.a.HistoryToggle())
	m.pullInput(m.cur())
	m.pullIdle()
	return cmd
}

// historyScope runs effs, from setting the panel's scope (a click on a
// half of the switch, or Tab), and brings the input up to date.
func (m *Model) historyScope(effs []app.Effect) tea.Cmd {
	cmd := m.run(effs)
	m.pullInput(m.cur())
	m.pullIdle()
	return cmd
}
