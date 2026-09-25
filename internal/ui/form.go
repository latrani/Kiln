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

// field is one row of a form: a labeled single-line input or toggle, or
// a button.
type field struct {
	label  string
	in     *Input // nil for a toggle or a button
	on     bool   // a toggle's state
	button bool
	accept func(string) error // nil accepts anything; else edits it rejects don't happen
}

func textField(label string) field   { return field{label: label, in: NewInput()} }
func toggleField(label string) field { return field{label: label} }
func buttonField(label string) field { return field{label: label, button: true} }

// form is a set of fields shown in the input area as a prompt, owning
// the keys while it's open. The picker's filter is a one-field form, drawn
// on one row, where Enter is the owner's; a form with more fields stacks
// them, one per row, and Enter moves to the next until it's on a button.
type form struct {
	fields []field
	focus  int    // the field being edited
	hint   string // what the keys do, shown dim after the fields
	reject string // why the last edit was refused; shown instead of the hint
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
			f.focus++
			return true
		case "tab":
			f.focus = (f.focus + 1) % len(f.fields)
			return true
		case "shift+tab":
			f.focus = (f.focus + len(f.fields) - 1) % len(f.fields)
			return true
		case "up":
			f.focus = max(0, f.focus-1)
			return true
		case "down":
			f.focus = min(len(f.fields)-1, f.focus+1)
			return true
		}
	}
	fl := &f.fields[f.focus]
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
	w := 0
	for _, fl := range f.fields {
		w = max(w, xansi.StringWidth(fl.label))
	}
	for i := range f.fields {
		text, col := f.fieldRow(i, w)
		rows = append(rows, text)
		if i == f.focus {
			curRow, curCol = i, col
		}
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
	if fl.button {
		b := "[ " + fl.label + " ]"
		if i == f.focus {
			b = reverse + b + style.Reset
		}
		return b, 2
	}
	label := fl.label + ": " + strings.Repeat(" ", w-xansi.StringWidth(fl.label))
	if fl.in == nil {
		box := "[ ]"
		if fl.on {
			box = "[x]"
		}
		return style.Dim(label) + box, xansi.StringWidth(label) + 1
	}
	rows, _, c := fl.in.Render(1<<20, 0, false, false)
	return style.Dim(label) + strings.TrimPrefix(rows[0], gutterMark), c - gutterWidth + xansi.StringWidth(label)
}
