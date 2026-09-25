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

// addCharHint is the hint for the editor that adds a character.
const addCharHint = "Enter to connect · Esc to cancel"

// The picker's add rows, and their selection keys. Neither key can be a
// character's, which always has a "/".
const (
	addCharLabel  = "+ Character"
	addWorldLabel = "+ World"
	addWorldSel   = "+"
)

func addCharSel(world string) string { return "+" + world }

// worldSel is the selection key of world's header row, which opens the
// world's editor.
func worldSel(world string) string { return "=" + world }

// browseBlocksPicker is the status when the picker can't open because
// browse mode has the pane. Short, so it fits after browse mode's
// status-line prefix on an 80-column screen.
const browseBlocksPicker = "Esc out of browse mode first"

// questionBlocksPicker is the status when the picker can't open because
// the save-password question owns the input area.
const questionBlocksPicker = "Answer the question first"

// picker is the add-connection list shown in the sidebar: every
// configured character that isn't open, narrowed by a one-field form,
// with rows for adding characters and worlds.
type picker struct {
	form *form
	sel  string  // selection key of the highlighted row (see selKey)
	edit *editor // non-nil while adding or editing a world or character
}

// selKey is what picker.sel holds when r is highlighted; "" for a row
// that can't be.
func selKey(r sidebarRow) string {
	switch r.kind {
	case rowChar:
		return r.char
	case rowWorld:
		return worldSel(r.world)
	case rowAddChar:
		return addCharSel(r.world)
	case rowAddWorld:
		return addWorldSel
	}
	return ""
}

// openPicker shows the picker, unless the active character is in browse
// mode, which has the pane (and the input area) to itself, or the
// save-password question owns the input area.
func (m *Model) openPicker() {
	if m.mode == modeSavePassword {
		m.setStatus(true, questionBlocksPicker)
		return
	}
	if cs := m.cur(); cs != nil && cs.browse != nil {
		m.setStatus(true, browseBlocksPicker)
		return
	}
	m.picker = &picker{form: newForm(pickerHint, textField("Filter"))}
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

// pickerRows lists the characters the picker offers under their worlds,
// each world ending with a row to add a character to it, then a row to
// add a world. A world is listed when the filter matches it or one of
// its characters, even with no characters to offer.
func (m *Model) pickerRows() []sidebarRow {
	f := strings.ToLower(strings.TrimSpace(m.picker.form.value(0)))
	chars := m.allChars()
	var rows []sidebarRow
	for _, w := range m.worldIDs() {
		wr := []sidebarRow{{kind: rowWorld, world: w}}
		for _, ch := range chars {
			k := key(ch.World, ch.ID)
			if ch.World == w && m.chars[k] == nil && matches(ch, f) {
				wr = append(wr, sidebarRow{kind: rowChar, world: w, char: k})
			}
		}
		if len(wr) > 1 || strings.Contains(strings.ToLower(w), f) {
			rows = append(append(rows, wr...), sidebarRow{kind: rowAddChar, world: w})
		}
	}
	return append(rows, sidebarRow{kind: rowAddWorld})
}

// worldIDs is every configured world, in sidebar order.
func (m *Model) worldIDs() []string {
	var ids []string
	if m.cfg != nil {
		for _, w := range m.cfg.Worlds {
			ids = append(ids, w.ID)
		}
	}
	slices.SortFunc(ids, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return ids
}

// pickable is the selection keys of the rows the picker can highlight,
// in order.
func (m *Model) pickable() []string {
	var keys []string
	for _, r := range m.pickerRows() {
		if k := selKey(r); k != "" {
			keys = append(keys, k)
		}
	}
	return keys
}

// fixPick keeps the highlight on its row if it's still offered, and
// otherwise moves it to the first character, or the first row there is.
func (m *Model) fixPick() {
	keys := m.pickable()
	if slices.Contains(keys, m.picker.sel) {
		return
	}
	m.picker.sel = keys[0] // there's always + World
	if i := slices.IndexFunc(keys, func(k string) bool { return strings.Contains(k, "/") }); i >= 0 {
		m.picker.sel = keys[i]
	}
}

// movePick moves the highlight by delta rows, stopping at the ends.
func (m *Model) movePick(delta int) {
	keys := m.pickable()
	i := slices.Index(keys, m.picker.sel)
	if i < 0 {
		return
	}
	m.picker.sel = keys[min(max(0, i+delta), len(keys)-1)]
}

// pagePick moves the highlight about one page of rows (world headers
// included) in direction dir, landing on the last highlightable row
// within that page, or on the next one when the page holds none.
func (m *Model) pagePick(dir, page int) {
	rows := m.pickerRows()
	cur := slices.IndexFunc(rows, func(r sidebarRow) bool { return selKey(r) == m.picker.sel })
	if cur < 0 {
		return
	}
	for i := min(max(0, cur+dir*page), len(rows)-1); i != cur; i -= dir {
		if k := selKey(rows[i]); k != "" {
			m.picker.sel = k
			return
		}
	}
	m.movePick(dir)
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

// choose acts on the picker row with selection key sel: it connects a
// character, or opens an editor for a world or an add row.
func (m *Model) choose(sel string) tea.Cmd {
	switch {
	case strings.HasPrefix(sel, "="):
		m.openWorldEditor(strings.TrimPrefix(sel, "="))
	case sel == addWorldSel:
		m.openWorldEditor("")
	case strings.HasPrefix(sel, "+"):
		name := textField("Name")
		name.accept = config.NameChars
		m.picker.edit = &editor{form: newForm(addCharHint, name), kind: addChar, world: strings.TrimPrefix(sel, "+")}
	default:
		return m.pick(sel)
	}
	return nil
}

// saveCharacter writes the character editor's character, then opens and
// connects it.
func (m *Model) saveCharacter() tea.Cmd {
	e := m.picker.edit
	id, err := config.AddCharacter(m.d.ConfigDir, e.world, e.form.value(0))
	if err != nil {
		e.form.reject = err.Error()
		return nil
	}
	m.picker.edit = nil
	if !m.reloadNow() {
		return nil
	}
	return m.pick(key(e.world, id))
}

// pickerKey handles a key while the picker is open.
func (m *Model) pickerKey(k tea.KeyPressMsg) tea.Cmd {
	if m.picker.edit != nil {
		return m.editorKey(k)
	}
	page := max(1, m.sidebarView().avail-1)
	switch k.String() {
	case "esc", "ctrl+c", openPickerKey:
		m.closePicker()
	case "enter":
		return m.choose(m.picker.sel)
	case openEditorKey:
		switch sel := m.picker.sel; {
		case strings.HasPrefix(sel, "="):
			m.openWorldEditor(strings.TrimPrefix(sel, "="))
		case strings.Contains(sel, "/"):
			m.openCharEditor(sel)
		}
	case "up":
		m.movePick(-1)
	case "down":
		m.movePick(1)
	case "pgup":
		m.pagePick(-1, page)
	case "pgdown":
		m.pagePick(1, page)
	default:
		if m.picker.form.key(k) {
			m.fixPick()
		}
	}
	return nil
}

// pickerLine draws one picker row: a world, a character or an add row
// indented under it, or the add-world row; highlighted when selected.
func (m *Model) pickerLine(r sidebarRow, w int) string {
	var line string
	switch r.kind {
	case rowWorld:
		if selKey(r) == m.picker.sel {
			return reverse + bold + fitName(r.world, w) + style.Reset
		}
		return bold + fitName(r.world, w) + style.Reset
	case rowAddChar:
		line = fitName("  "+addCharLabel, w)
	case rowAddWorld:
		line = fitName(addWorldLabel, w)
	default:
		ch, _ := m.find(r.char)
		line = fitName("  "+ch.Name, w)
	}
	switch {
	case selKey(r) == m.picker.sel:
		return reverse + line + style.Reset
	case r.kind != rowChar:
		return style.Dim(line)
	}
	return line
}
