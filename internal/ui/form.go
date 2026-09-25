package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/style"
)

// editKey applies a line-editing key to in: cursor movement, deletion
// and typed text. It reports false for any other key, including Enter,
// Up and Down, which mean different things in the chat input and in a
// form.
func editKey(in *Input, k tea.KeyPressMsg) bool {
	switch k.String() {
	case "left":
		in.Left()
	case "right":
		in.Right()
	case "ctrl+left", "alt+left", "alt+b":
		in.WordLeft()
	case "ctrl+right", "alt+right", "alt+f":
		in.WordRight()
	case "home", "ctrl+a":
		in.Home()
	case "end", "ctrl+e":
		in.End()
	case "backspace":
		in.Backspace()
	case "delete", "ctrl+d":
		in.Delete()
	case "ctrl+w", "alt+backspace", "ctrl+backspace":
		in.DeleteWordBack()
	case "alt+delete", "ctrl+delete", "alt+d":
		in.DeleteWordForward()
	case "ctrl+u":
		in.KillToStart()
	case "ctrl+k":
		in.KillToEnd()
	default:
		if k.Text == "" || k.Mod&(tea.ModCtrl|tea.ModAlt) != 0 {
			return false
		}
		in.InsertText(k.Text)
	}
	return true
}

// field is one row of a form: a labeled single-line input, toggle or
// choice, a button, or the header that shows and hides the extra fields.
type field struct {
	label   string
	in      *Input // nil for everything but a text field
	on      bool   // a toggle's state
	button  bool
	section bool               // the header over the extra fields
	extra   bool               // shown only while the form is expanded
	choices []string           // a choice's options; see choiceField
	choice  int                // the chosen option
	shown   []string           // how each choice is drawn
	hint    string             // drawn dim in an empty text field
	accept  func(string) error // nil accepts anything; else edits it rejects don't happen
}

func textField(label string) field   { return field{label: label, in: NewInput()} }
func toggleField(label string) field { return field{label: label} }
func buttonField(label string) field { return field{label: label, button: true} }

// sectionField heads the extra fields, which it shows and hides.
func sectionField(label string) field { return field{label: label, section: true} }

// choiceField picks one of choices, drawn as shown; Space and ←/→ cycle.
func choiceField(label string, choices, shown []string) field {
	return field{label: label, choices: choices, shown: shown}
}

// asExtra marks fl as one of the fields the section header hides.
func asExtra(fl field) field {
	fl.extra = true
	return fl
}

// withHint gives a text field dim text to show while it's empty.
func withHint(fl field, hint string) field {
	fl.hint = hint
	return fl
}

// withValue fills a text field.
func withValue(fl field, v string) field {
	fl.in.SetValue(v)
	return fl
}

// chosen is choice field i's option.
func (f *form) chosen(i int) string { return f.fields[i].choices[f.fields[i].choice] }

// choose sets choice field i to the option v, if it has one.
func (f *form) choose(i int, v string) {
	for j, c := range f.fields[i].choices {
		if c == v {
			f.fields[i].choice = j
		}
	}
}

// field finds a field by label; -1 if there's none.
func (f *form) field(label string) int {
	for i, fl := range f.fields {
		if fl.label == label {
			return i
		}
	}
	return -1
}

// form is a set of fields shown in the input area as a prompt, owning
// the keys while it's open. The picker's filter is a one-field form, drawn
// on one row, where Enter is the owner's; a form with more fields stacks
// them, one per row, and Enter moves to the next until it's on a button.
type form struct {
	fields   []field
	focus    int    // the field being edited
	hint     string // what the keys do, shown dim after the fields
	reject   string // why the last edit was refused; shown instead of the hint
	expanded bool   // the extra fields are shown
	title    string // drawn above the fields, if any
}

// visible reports whether field i is shown.
func (f *form) visible(i int) bool { return !f.fields[i].extra || f.expanded }

// step moves the focus to the next shown field in direction dir,
// wrapping around if wrap, else stopping at the ends.
func (f *form) step(dir int, wrap bool) {
	n := len(f.fields)
	for i, j := 1, f.focus; i < n; i++ {
		j += dir
		if !wrap && (j < 0 || j >= n) {
			return
		}
		j = (j + n) % n
		if f.visible(j) {
			f.focus = j
			return
		}
	}
}

func newForm(hint string, fields ...field) *form {
	return &form{fields: fields, hint: hint}
}

// value is field i's text.
func (f *form) value(i int) string { return f.fields[i].in.Value() }

// on is toggle i's state.
func (f *form) on(i int) bool { return f.fields[i].on }

// pressed reports whether Enter now presses the focused button.
func (f *form) pressed() bool { return f.fields[f.focus].button }

// pressedLabel is the focused button's label, or "".
func (f *form) pressedLabel() string {
	if f.pressed() {
		return f.fields[f.focus].label
	}
	return ""
}

// key handles a key for the focused field, and in a multi-field form Tab,
// Shift+Tab, Up, Down and (off a button) Enter to move between fields.
// It reports false for the keys it leaves to the form's owner.
func (f *form) key(k tea.KeyPressMsg) bool {
	if len(f.fields) > 1 {
		switch k.String() {
		case "enter":
			if f.pressed() {
				return false
			}
			if f.fields[f.focus].section {
				f.expanded = !f.expanded
				return true
			}
			f.step(1, false)
			return true
		case "tab":
			f.step(1, true)
			return true
		case "shift+tab":
			f.step(-1, true)
			return true
		case "up":
			f.step(-1, false)
			return true
		case "down":
			f.step(1, false)
			return true
		}
	}
	fl := &f.fields[f.focus]
	if fl.section && k.String() == "space" {
		f.expanded = !f.expanded
		return true
	}
	if fl.choices != nil {
		switch k.String() {
		case "space", "right":
			fl.choice = (fl.choice + 1) % len(fl.choices)
			return true
		case "left":
			fl.choice = (fl.choice + len(fl.choices) - 1) % len(fl.choices)
			return true
		}
	}
	if fl.in == nil {
		switch {
		case k.String() == "space" && !fl.button:
			fl.on = !fl.on
			return true
		case k.Text != "" && k.Mod&(tea.ModCtrl|tea.ModAlt) == 0:
			return true // typing doesn't reach the owner either
		}
		return false
	}
	return f.edit(func(in *Input) bool { return editKey(in, k) })
}

// paste inserts s into the focused field, as one line.
func (f *form) paste(s string) {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", " "), "\n", " ")
	f.edit(func(in *Input) bool { in.InsertText(s); return true })
}

// edit applies do to the focused input, undoing it if the field rejects
// the result.
func (f *form) edit(do func(*Input) bool) bool {
	fl := f.fields[f.focus]
	if fl.in == nil {
		return false
	}
	old, col := fl.in.Value(), fl.in.col
	if !do(fl.in) {
		return false
	}
	f.reject = ""
	if fl.accept != nil {
		if err := fl.accept(fl.in.Value()); err != nil {
			fl.in.SetValue(old)
			fl.in.col = col
			f.reject = err.Error()
		}
	}
	return true
}

// rows draws the form for the input area and returns the cursor's row
// and column. One field goes on one row, followed by the hint; more are
// stacked with their labels lined up, and the hint follows the last.
func (f *form) rows() (rows []string, curRow, curCol int) {
	note := style.Dim(f.hint)
	if f.reject != "" {
		note = red + f.reject + style.Reset
	}
	if len(f.fields) == 1 {
		text, col := f.fieldRow(0, len(f.fields[0].label))
		return []string{text + style.Dim("  · ") + note}, 0, col
	}
	if f.title != "" {
		rows = append(rows, bold+f.title+style.Reset)
	}
	w := 0
	for i, fl := range f.fields {
		if f.visible(i) && !fl.button && !fl.section {
			w = max(w, xansi.StringWidth(fl.label))
		}
	}
	for i := range f.fields {
		if !f.visible(i) {
			continue
		}
		text, col := f.fieldRow(i, w)
		if i == f.focus {
			curRow, curCol = len(rows), col
		}
		rows = append(rows, text)
	}
	if f.reject == "" {
		note = style.Dim("Enter next field · " + f.hint)
	}
	rows[len(rows)-1] += style.Dim("  · ") + note
	return rows, curRow, curCol
}

// fieldRow draws field i with its label padded to w columns, and returns
// where the cursor goes when it's focused.
func (f *form) fieldRow(i, w int) (text string, col int) {
	fl := f.fields[i]
	if fl.section {
		mark := "► "
		if f.expanded {
			mark = "▼ "
		}
		t := mark + fl.label
		if i == f.focus {
			t = reverse + t + style.Reset
		}
		return t, 0
	}
	if fl.button {
		b := "[ " + fl.label + " ]"
		if i == f.focus {
			b = reverse + b + style.Reset
		}
		return b, 2
	}
	label := fl.label + ": " + strings.Repeat(" ", w-xansi.StringWidth(fl.label))
	if fl.choices != nil {
		v := fl.shown[fl.choice]
		if i == f.focus {
			v = "◄ " + v + " ►"
		} else {
			v = "  " + v
		}
		return style.Dim(label) + v, xansi.StringWidth(label)
	}
	if fl.in == nil {
		box := "[ ]"
		if fl.on {
			box = "[x]"
		}
		return style.Dim(label) + box, xansi.StringWidth(label) + 1
	}
	rows, _, c := fl.in.Render(1<<20, 0, false, false)
	text = strings.TrimPrefix(rows[0], gutterMark)
	if fl.in.Empty() && fl.hint != "" {
		text = style.Dim(fl.hint)
	}
	return style.Dim(label) + text, c - gutterWidth + xansi.StringWidth(label)
}
