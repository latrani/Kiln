package ui

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/glyph"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

// pickerHint is the picker's keys, in the input area while its filter
// is typed in the sidebar.
var pickerHint = str.PickerHint()

// addCharHint is the hint for the editor that adds a character.
var addCharHint = str.PickerAddCharHint()

// The picker's add rows, and their selection keys. Neither key can be a
// character's, which always has a "/".
var (
	addCharLabel  = str.PickerAddCharacter()
	addWorldLabel = str.PickerAddWorld()
)

const addWorldSel = "+"

func addCharSel(world string) string { return "+" + world }

// worldSel is the selection key of world's header row, which opens the
// world's editor.
func worldSel(world string) string { return app.WorldSel(world) }

// browseBlocksPicker is the status when the picker can't open because
// browse mode has the pane. Short, so it fits after browse mode's
// status-line prefix on an 80-column screen.
var browseBlocksPicker = str.PickerBlockedByLog()

// questionBlocksPicker is the status when the picker can't open because
// the save-password question owns the input area.
var questionBlocksPicker = str.PickerBlockedByQuestion()

// picker is the jump list shown in the sidebar: every configured world
// and character, open or not, narrowed by a one-field form, with rows
// for adding characters and worlds.
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
	if _, _, asking := m.a.PendingSave(); asking {
		m.setStatus(true, questionBlocksPicker)
		return
	}
	if cs := m.cur(); cs != nil && cs.browse != nil {
		m.setStatus(true, browseBlocksPicker)
		return
	}
	m.picker = &picker{form: newForm(pickerHint, textField(str.PickerFilter())), sel: m.a.Active()}
	m.sideTop, m.sideShown = 0, ""
	m.fixPick()
}

// jump is /open with a filter: the picker, filtered, and if anything
// matches, Enter on the first match. With nothing matching it stays
// open, showing so.
func (m *Model) jump(filter string) tea.Cmd {
	if m.openPicker(); m.picker == nil {
		return nil
	}
	m.picker.form.fields[0].in.SetValue(filter)
	sel, ok := m.firstMatch()
	if !ok {
		return nil
	}
	m.picker.sel = sel
	return m.choose(sel)
}

// closePicker goes back to the open characters.
func (m *Model) closePicker() {
	m.picker = nil
	m.sideShown = ""     // scroll the active character back into view
	m.narrowOpen = false // picked or escaped: the narrow sidebar closes
}

// none is below any score.
const none = math.MinInt / 2

// The filter's scoring, after fzf's (its src/algo/algo.go): each letter
// matched scores, more on a word's first letter (the filter's first
// letter most), more in a run, less after a gap.
const (
	scoreMatch       = 16
	bonusWordStart   = 8  // the start, or after anything but a letter or digit
	bonusCamel       = 7  // an uppercase letter after a lowercase one
	bonusConsecutive = 4  // straight after the last letter matched
	scoreGapStart    = -3 // the first letter skipped
	scoreGapMore     = -1 // each after it
)

// startBonus is the bonus for a match at rune j of rs: on a word's
// first letter, or none.
func startBonus(rs []rune, j int) int {
	if j == 0 {
		return bonusWordStart
	}
	switch prev := rs[j-1]; {
	case !unicode.IsLetter(prev) && !unicode.IsDigit(prev):
		return bonusWordStart
	case unicode.IsLower(prev) && unicode.IsUpper(rs[j]):
		return bonusCamel
	}
	return 0
}

// match reports whether f's letters are in s in order, ignoring case;
// if so, which of s's runes they are, placed where they score best, and
// that score. Ties go to the placement earliest in s.
func match(s, f string) (at []int, score int, ok bool) {
	rs, want := []rune(s), []rune(f)
	if len(want) == 0 {
		return nil, 0, true
	}
	n, m := len(rs), len(want)
	// best[i][j] is the best score with the filter's letter i at rune j,
	// from[i][j] where letter i-1 was then.
	best, from := make([][]int, m), make([][]int, m)
	for i := range m {
		best[i], from[i] = make([]int, n), make([]int, n)
		for j := range n {
			best[i][j] = none
			if unicode.ToLower(rs[j]) != unicode.ToLower(want[i]) {
				continue
			}
			if i == 0 {
				best[0][j] = scoreMatch + 2*startBonus(rs, j)
				continue
			}
			for k := i - 1; k < j; k++ {
				if best[i-1][k] == none {
					continue
				}
				sc := best[i-1][k] + scoreMatch + startBonus(rs, j)
				if k == j-1 {
					sc += bonusConsecutive
				} else {
					sc += scoreGapStart + scoreGapMore*(j-k-2)
				}
				if sc > best[i][j] {
					best[i][j], from[i][j] = sc, k
				}
			}
		}
	}
	end := -1
	for j := range n {
		if best[m-1][j] != none && (end < 0 || best[m-1][j] > best[m-1][end]) {
			end = j
		}
	}
	if end < 0 {
		return nil, 0, false
	}
	at = make([]int, m)
	for i, j := m-1, end; i >= 0; i-- {
		at[i] = j
		j = from[i][j]
	}
	return at, best[m-1][end], true
}

// filter is the picker's filter, trimmed.
func (m *Model) filter() string { return strings.TrimSpace(m.picker.form.value(0)) }

// pickerRows lists every world and character, open or not, those the
// filter matches: a world when its name does or one of its characters'
// does, a character when "world/name" does. Each world ends with a row
// to add a character to it, and the list with a row to add a world. A
// row that matched carries which of its letters did, and how well; a
// world listed only for its characters carries the letters of its name
// the best of them matched.
func (m *Model) pickerRows() []sidebarRow {
	f := m.filter()
	chars := m.a.AllChars()
	var rows []sidebarRow
	for _, w := range m.worldIDs() {
		wat, wscore, wok := match(w, f)
		head := sidebarRow{kind: rowWorld, world: w, match: wok, marks: wat, score: wscore}
		wr := []sidebarRow{head}
		wn := len([]rune(w)) + 1 // past the "/"
		bestChar := none
		for _, ch := range chars {
			if ch.World != w {
				continue
			}
			at, score, ok := match(w+"/"+ch.Name, f)
			if !ok {
				continue
			}
			r := sidebarRow{kind: rowChar, world: w, char: key(ch.World, ch.ID), match: true, score: score}
			var inWorld []int
			for _, i := range at {
				if i >= wn {
					r.marks = append(r.marks, i-wn)
				} else {
					inWorld = append(inWorld, i)
				}
			}
			if !wok && score > bestChar {
				wr[0].marks, bestChar = inWorld, score
			}
			wr = append(wr, r)
		}
		if len(wr) > 1 || wok {
			rows = append(append(rows, wr...), sidebarRow{kind: rowAddChar, world: w})
			if e := m.addingChar(); e != nil && e.world == w && e.form.reject != "" {
				// One space in, as the field; wrapped, since the sidebar
				// is narrow.
				bw, _ := m.sideBox()
				if bw == 0 {
					bw = SidebarWidth(m.width)
				}
				wrapped := xansi.Wrap(e.form.reject, max(1, bw-1), "")
				for _, l := range strings.Split(wrapped, "\n") {
					rows = append(rows, sidebarRow{kind: rowAddCharError, world: w, text: l})
				}
			}
		}
	}
	return branch(append(rows, sidebarRow{kind: rowAddWorld}))
}

// firstMatch is the selection key of the row the filter matched best, a
// world or a character, the first of those that tie; false when none
// did.
func (m *Model) firstMatch() (string, bool) {
	sel, top := "", none
	for _, r := range m.pickerRows() {
		if r.match && (r.kind == rowWorld || r.kind == rowChar) && r.score > top {
			sel, top = selKey(r), r.score
		}
	}
	return sel, sel != ""
}

// addingChar is the editor naming a new character, which the picker
// draws in place of its world's "+ Character"; nil when there's none.
func (m *Model) addingChar() *editor {
	if m.picker == nil || m.picker.edit == nil || m.picker.edit.kind != addChar {
		return nil
	}
	return m.picker.edit
}

// worldIDs is every configured world, in sidebar order.
func (m *Model) worldIDs() []string {
	var ids []string
	if m.a.Config() != nil {
		for _, w := range m.a.Config().Worlds {
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

// filtered moves the highlight to the first row the filter matches as
// it changes, so the same letters always land on the same row; with the
// filter emptied, to what's showing. With nothing matching it's kept
// where it can be (see fixPick).
func (m *Model) filtered() {
	switch sel, ok := m.firstMatch(); {
	case ok && m.filter() != "":
		m.picker.sel = sel
	case m.filter() == "":
		m.picker.sel = m.a.Active()
	}
	m.fixPick()
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

// pick opens, connects and switches to k, closing the picker; one
// already open is only switched to.
func (m *Model) pick(k string) tea.Cmd {
	if m.chars[k] != nil {
		m.closePicker()
		m.switchTo(k)
		return nil
	}
	cs := m.open(k)
	if cs == nil {
		return nil
	}
	m.closePicker()
	m.switchTo(k)
	return m.connect(cs)
}

// choose acts on the picker row with selection key sel: it shows a
// world's overview or a character, opening and connecting one that
// isn't open, or opens an editor for an add row. A world with nothing
// open has no overview: its editor opens instead.
func (m *Model) choose(sel string) tea.Cmd {
	switch {
	case strings.HasPrefix(sel, "=") && m.a.CanSwitch(sel):
		m.closePicker()
		m.switchTo(sel)
	case strings.HasPrefix(sel, "="):
		m.openWorldEditor(strings.TrimPrefix(sel, "="))
	case sel == addWorldSel:
		m.openWorldEditor("")
	case strings.HasPrefix(sel, "+"):
		name := withHint(textField(str.PickerName()), str.PickerName())
		name.accept = config.NameChars
		m.setEditor(&editor{form: newForm(addCharHint, name), kind: addChar, world: strings.TrimPrefix(sel, "+")})
	default:
		return m.pick(sel)
	}
	return nil
}

// saveCharacter writes the character editor's character, then opens and
// connects it.
func (m *Model) saveCharacter() tea.Cmd {
	e := m.picker.edit
	id, err := m.a.AddCharacter(e.world, e.form.value(0))
	if err != nil {
		e.form.reject = firstProblem(err)
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
		was := m.filter()
		if m.picker.form.key(k) {
			if m.filter() != was {
				m.filtered()
			} else {
				m.fixPick()
			}
		}
	}
	return nil
}

// filterField draws the picker's filter, under the sidebar's title, the
// width across; see addField. It returns the cursor's column too.
func filterField(f *form, w int) (line string, col int) {
	return addField(f.fields[0].in, f.fields[0].label, theme.PickerFilter, w)
}

// nameField draws the name being added, in place of "+ Character" and
// in its colors, past the end of its world's tree line; see addField.
// It returns the cursor's column too.
func nameField(f *form, w int) (line string, col int) {
	line, col = addField(f.fields[0].in, f.fields[0].hint, theme.PickerAdd, w-1)
	return theme.Paint(theme.PickerTree, glyphTreeLast) + line, col + 1
}

// addField draws in as the add row it replaces, in role: filling w,
// scrolled to keep the cursor in view; empty, its hint, dim. It returns
// the cursor's column too.
func addField(in *Input, hint string, role theme.Role, w int) (line string, col int) {
	if in.Empty() && hint != "" {
		return fit(theme.Fill(role, " "+theme.Paint(theme.FormHint, hint), w), w), 1
	}
	rows, _, c := in.Render(1<<20, 0, false, false)
	text, c := strings.TrimPrefix(rows[0], gutterMark), c-gutterWidth
	tw := max(1, w-1) // the text's columns, past the chip's leading space
	off := max(0, c-tw+1)
	return theme.Paint(role, fit(" "+xansi.Cut(text, off, off+tw), w)), 1 + c - off
}

// pickerLine draws one picker row: a world, a character or an add row
// on its world's tree, or the add-world row; highlighted when selected,
// the letters the filter matched underlined. An open character is bold,
// with its badge and unread count as in the sidebar.
func (m *Model) pickerLine(r sidebarRow, w int) string {
	sel := selKey(r) == m.picker.sel
	switch r.kind {
	case rowWorld:
		role := theme.PickerWorld
		if sel {
			role = theme.PickerWorldSelected
		}
		return theme.Paint(role, style.Reassert(marked(r.world, r.marks, w), theme.SGR(role)))
	case rowAddCharError:
		return fit(" "+theme.Paint(theme.PickerError, r.text), w)
	case rowAddChar, rowAddWorld:
		if e := m.addingChar(); e != nil && r.kind == rowAddChar && e.world == r.world {
			line, _ := nameField(e.form, w)
			return line
		}
		label, role, indent := addWorldLabel, theme.PickerAdd, ""
		if r.kind == rowAddChar {
			label, indent = addCharLabel, theme.Paint(theme.PickerTree, r.tree())
		}
		if sel {
			role = theme.PickerSelected
		}
		return fit(indent+chip(role, label), w)
	}
	tree := theme.Paint(theme.PickerTree, r.tree())
	w-- // past the tree
	ch, _ := m.a.Find(r.char)
	cs := m.chars[r.char]
	if cs == nil {
		line := marked(ch.Name, r.marks, w)
		if sel {
			return tree + theme.Paint(theme.PickerSelected, style.Reassert(line, theme.SGR(theme.PickerSelected)))
		}
		return tree + line
	}
	lead := ""
	switch {
	case cs.State == session.Connecting:
		lead = glyph.Connecting + " "
	case closable(cs):
		lead = glyph.Remove + " "
	}
	activity := ""
	if cs.Unread > 0 {
		activity = fmt.Sprintf(" %d", cs.Unread)
		if cs.Priority {
			activity = " " + priorityMark() + activity
		}
	}
	name := marked(lead+ch.Name, shift(r.marks, len([]rune(lead))), w-xansi.StringWidth(activity))
	line := theme.Paint(theme.PickerOpen, style.Reassert(name, theme.SGR(theme.PickerOpen))) + activity
	if sel {
		return tree + theme.Paint(theme.PickerSelected, style.Reassert(line, theme.SGR(theme.PickerSelected)))
	}
	return tree + line
}

// marked is s fitted to w cells (cut with "…" when too long, as fitName),
// the runes at at underlined as the filter's matches.
func marked(s string, at []int, w int) string {
	if w <= 0 {
		return ""
	}
	t := xansi.Truncate(s, w, "…")
	rs := []rune(t)
	kept := len(rs)
	if t != s {
		kept-- // the …
	}
	var b strings.Builder
	for i, r := range rs {
		if i < kept && slices.Contains(at, i) {
			b.WriteString(theme.Paint(theme.PickerMatch, string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return fit(b.String(), w)
}

// shift moves rune positions n to the right.
func shift(at []int, n int) []int {
	out := make([]int, len(at))
	for i, a := range at {
		out[i] = a + n
	}
	return out
}
