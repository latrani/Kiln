package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/ansi"
)

func TestFormEditsFocusedField(t *testing.T) {
	f := newForm("Enter to go", "Filter")
	for _, r := range "rok" {
		if !f.key(tea.KeyPressMsg{Code: r, Text: string(r)}) {
			t.Fatalf("typing %q not handled", r)
		}
	}
	f.key(tea.KeyPressMsg{Code: tea.KeyLeft})
	f.key(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if f.value(0) != "rook" {
		t.Errorf("value = %q", f.value(0))
	}
	for _, k := range []tea.KeyPressMsg{{Code: tea.KeyUp}, {Code: tea.KeyDown}, {Code: tea.KeyEnter}, {Code: tea.KeyEscape}} {
		if f.key(k) {
			t.Errorf("%s should be left to the form's owner", k.String())
		}
	}
	f.paste("a\nb")
	if f.value(0) != "rooa bk" {
		t.Errorf("paste = %q, want one line", f.value(0))
	}
}

func TestFormRow(t *testing.T) {
	f := newForm("Enter to connect · Esc to close", "Filter")
	f.key(tea.KeyPressMsg{Code: 'm', Text: "m"})
	f.key(tea.KeyPressMsg{Code: 'a', Text: "a"})
	text, col := f.row()
	if got := ansi.Strip(text); got != "Filter: ma  · Enter to connect · Esc to close" {
		t.Errorf("row = %q", got)
	}
	if col != len("Filter: ma") {
		t.Errorf("cursor col = %d", col)
	}
}
