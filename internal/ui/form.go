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

// field is one labeled, single-line input in a form.
type field struct {
	label string
	in    *Input
}

// form is a set of fields shown in the input area as a prompt, owning
// the keys while it's open. The picker's filter is a one-field form;
// #41's world and character editors add fields.
type form struct {
	fields []field
	focus  int    // the field being edited
	hint   string // what the keys do, shown dim after the field
}

func newForm(hint string, labels ...string) *form {
	f := &form{hint: hint}
	for _, l := range labels {
		f.fields = append(f.fields, field{label: l, in: NewInput()})
	}
	return f
}

// value is field i's text.
func (f *form) value(i int) string { return f.fields[i].in.Value() }

// key edits the focused field; it reports false for keys that aren't
// editing keys, which the form's owner handles.
func (f *form) key(k tea.KeyPressMsg) bool { return editKey(f.fields[f.focus].in, k) }

// paste inserts s into the focused field, as one line.
func (f *form) paste(s string) {
	f.fields[f.focus].in.InsertText(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", " "), "\n", " "))
}

// row draws the focused field as the input area's prompt row, and
// returns the cursor's column.
func (f *form) row() (text string, col int) {
	fl := f.fields[f.focus]
	rows, _, c := fl.in.Render(1<<20, 0, false, false)
	label := fl.label + ": "
	text = style.Dim(label) + strings.TrimPrefix(rows[0], gutterMark) + style.Dim("  · "+f.hint)
	return text, c - gutterWidth + xansi.StringWidth(label)
}
