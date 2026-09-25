package ui

import (
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/style"
)

// pickerHint follows the filter in the input area.
const pickerHint = "Enter to connect · Esc to close"

// Hints for the editors that add a world or a character.
const (
	addWorldHint = "Esc to cancel"
	addCharHint  = "Enter to connect · Esc to cancel"
)

// The picker's add rows, and their selection keys. Neither key can be a
// character's, which always has a "/".
const (
	addCharLabel  = "+ Character"
	addWorldLabel = "+ World"
	addWorldSel   = "+"
)

func addCharSel(world string) string { return "+" + world }

// browseBlocksPicker is the status when the picker can't open because
// browse mode has the pane. Short, so it fits after browse mode's
// status-line prefix on an 80-column screen.
const browseBlocksPicker = "Esc out of browse mode first"

// picker is the add-connection list shown in the sidebar: every
// configured character that isn't open, narrowed by a one-field form,
// with rows for adding characters and worlds.
type picker struct {
	form *form
	sel  string  // selection key of the highlighted row (see selKey)
	edit *editor // non-nil while adding a world or character
}

// editor is the form for a new world, or a new character in world.
type editor struct {
	form  *form
	world string // "" for a new world
}

// selKey is what picker.sel holds when r is highlighted; "" for a row
// that can't be.
func selKey(r sidebarRow) string {
	switch r.kind {
	case rowChar:
		return r.char
	case rowAddChar:
		return addCharSel(r.world)
	case rowAddWorld:
		return addWorldSel
	}
	return ""
}

// openPicker shows the picker, unless the active character is in browse
// mode, which has the pane (and the input area) to itself.
func (m *Model) openPicker() {
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
// character, or opens an editor for an add row.
func (m *Model) choose(sel string) tea.Cmd {
	switch {
	case sel == addWorldSel:
		world, host, port := textField("World"), textField("Host"), textField("Port")
		world.accept = onlyMatching(worldIDChar, "a world id can only use letters, digits, _ and -")
		host.accept = func(s string) error {
			if strings.ContainsFunc(s, func(r rune) bool { return r <= ' ' || r > '~' }) {
				return errors.New("a host can't have spaces")
			}
			return nil
		}
		port.accept = onlyMatching(portChar, "a port is a number")
		m.picker.edit = &editor{form: newForm(addWorldHint, world, host, port, toggleField("TLS"), buttonField("Save"))}
	case strings.HasPrefix(sel, "+"):
		name := textField("Name")
		name.accept = config.NameChars
		m.picker.edit = &editor{form: newForm(addCharHint, name), world: strings.TrimPrefix(sel, "+")}
	default:
		return m.pick(sel)
	}
	return nil
}

var (
	worldIDChar = regexp.MustCompile(`^[A-Za-z0-9_-]*$`)
	portChar    = regexp.MustCompile(`^[0-9]{0,5}$`)
)

// onlyMatching accepts what re matches, and otherwise says why.
func onlyMatching(re *regexp.Regexp, why string) func(string) error {
	return func(s string) error {
		if !re.MatchString(s) {
			return errors.New(why)
		}
		return nil
	}
}

// editorKey handles a key while an editor is open over the picker.
func (m *Model) editorKey(k tea.KeyPressMsg) tea.Cmd {
	e := m.picker.edit
	switch k.String() {
	case "esc", "ctrl+c":
		m.picker.edit = nil
	case "enter":
		switch {
		case e.form.key(k): // moved to the next field
		case e.world == "":
			m.saveWorld()
		default:
			return m.saveCharacter()
		}
	default:
		e.form.key(k)
	}
	return nil
}

// saveWorld writes the world editor's world, then highlights its row
// for adding a character. A problem is shown in the editor, which stays.
func (m *Model) saveWorld() {
	f := m.picker.edit.form
	port, _ := strconv.Atoi(f.value(2))
	id := f.value(0)
	if err := config.AddWorld(m.d.ConfigDir, id, f.value(1), port, f.on(3)); err != nil {
		f.reject = err.Error()
		return
	}
	m.picker.edit = nil
	m.picker.form.fields[0].in.SetValue("") // so the new world is listed
	m.reloadNow()
	m.picker.sel = addCharSel(id)
	m.fixPick()
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

// pickerLine draws one picker row: a world, a character or an add row
// indented under it, or the add-world row; highlighted when selected.
func (m *Model) pickerLine(r sidebarRow, w int) string {
	var line string
	switch r.kind {
	case rowWorld:
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
