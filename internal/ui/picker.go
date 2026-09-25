package ui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/style"
)

// pickerHint follows the filter in the input area.
const pickerHint = "Enter to connect · Esc to close"

// browseBlocksPicker is the status when the picker can't open because
// browse mode has the pane. Short, so it fits after browse mode's
// status-line prefix on an 80-column screen.
const browseBlocksPicker = "Esc out of browse mode first"

// noMatches fills the picker when it has nothing to offer.
const noMatches = "No matches"

// picker is the add-connection list shown in the sidebar: every
// configured character that isn't open, narrowed by a one-field form.
type picker struct {
	form *form
	sel  string // key of the highlighted character; "" when nothing matches
}

// openPicker shows the picker, unless the active character is in browse
// mode, which has the pane (and the input area) to itself.
func (m *Model) openPicker() {
	if cs := m.cur(); cs != nil && cs.browse != nil {
		m.setStatus(true, browseBlocksPicker)
		return
	}
	m.picker = &picker{form: newForm(pickerHint, "Filter")}
	m.sideTop, m.sideShown = 0, ""
	m.fixPick()
}

// closePicker goes back to the open characters.
func (m *Model) closePicker() {
	m.picker = nil
	m.sideShown = "" // scroll the active character back into view
}

// matches reports whether ch fits the lowercased filter f: its name, an
// alias or its world contains it.
func matches(ch config.Character, f string) bool {
	if f == "" {
		return true
	}
	for _, s := range append([]string{ch.Name, ch.World}, ch.Aliases...) {
		if strings.Contains(strings.ToLower(s), f) {
			return true
		}
	}
	return false
}

// pickerRows lists the characters the picker offers, under their worlds.
func (m *Model) pickerRows() []sidebarRow {
	f := strings.ToLower(strings.TrimSpace(m.picker.form.value(0)))
	var rows []sidebarRow
	for _, ch := range m.allChars() {
		k := key(ch.World, ch.ID)
		if m.chars[k] != nil || !matches(ch, f) {
			continue
		}
		if len(rows) == 0 || rows[len(rows)-1].world != ch.World {
			rows = append(rows, sidebarRow{kind: rowWorld, world: ch.World})
		}
		rows = append(rows, sidebarRow{kind: rowChar, world: ch.World, char: k})
	}
	return rows
}

// pickable is the keys of the characters the picker offers, in order.
func (m *Model) pickable() []string {
	var keys []string
	for _, r := range m.pickerRows() {
		if r.kind == rowChar {
			keys = append(keys, r.char)
		}
	}
	return keys
}

// fixPick keeps the highlight on its character if it's still offered,
// and otherwise moves it to the first one (or none).
func (m *Model) fixPick() {
	keys := m.pickable()
	if slices.Contains(keys, m.picker.sel) {
		return
	}
	m.picker.sel = ""
	if len(keys) > 0 {
		m.picker.sel = keys[0]
	}
}

// movePick moves the highlight by delta characters, stopping at the ends.
func (m *Model) movePick(delta int) {
	keys := m.pickable()
	i := slices.Index(keys, m.picker.sel)
	if i < 0 {
		return
	}
	m.picker.sel = keys[min(max(0, i+delta), len(keys)-1)]
}

// pick opens, connects and switches to k, closing the picker.
func (m *Model) pick(k string) tea.Cmd {
	cs := m.open(k)
	if cs == nil {
		return nil
	}
	m.closePicker()
	m.switchTo(k)
	return m.connect(cs)
}

// pickerKey handles a key while the picker is open.
func (m *Model) pickerKey(k tea.KeyPressMsg) tea.Cmd {
	page := max(1, m.sidebarView().avail-1)
	switch k.String() {
	case "esc", "ctrl+c", openPickerKey:
		m.closePicker()
	case "enter":
		return m.pick(m.picker.sel)
	case "up":
		m.movePick(-1)
	case "down":
		m.movePick(1)
	case "pgup":
		m.movePick(-page)
	case "pgdown":
		m.movePick(page)
	default:
		if m.picker.form.key(k) {
			m.fixPick()
		}
	}
	return nil
}

// pickerLine draws one picker row: a world, or a character indented
// under it, highlighted when selected.
func (m *Model) pickerLine(r sidebarRow, w int) string {
	if r.kind == rowWorld {
		return bold + fitName(r.world, w) + style.Reset
	}
	ch, _ := m.find(r.char)
	line := fitName("  "+ch.Name, w)
	if r.char == m.picker.sel {
		return reverse + line + style.Reset
	}
	return line
}
