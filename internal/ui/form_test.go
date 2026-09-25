package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/ansi"
)

func TestFormEditsFocusedField(t *testing.T) {
	f := newForm("Enter to go", textField("Filter"))
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
	f := newForm("Enter to connect · Esc to close", textField("Filter"))
	f.key(tea.KeyPressMsg{Code: 'm', Text: "m"})
	f.key(tea.KeyPressMsg{Code: 'a', Text: "a"})
	rows, row, col := f.rows()
	if len(rows) != 1 || row != 0 {
		t.Fatalf("a one-field form is one row: %q, cursor row %d", rows, row)
	}
	if got := ansi.Strip(rows[0]); got != "Filter: ma  · Enter to connect · Esc to close" {
		t.Errorf("row = %q", got)
	}
	if col != len("Filter: ma") {
		t.Errorf("cursor col = %d", col)
	}
}

func typeText(f *form, s string) {
	for _, r := range s {
		f.key(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func worldForm() *form {
	return newForm("Esc to cancel",
		textField("World"), textField("Host"), textField("Port"), toggleField("TLS"), buttonField("Save"))
}

func TestFormMovesBetweenFields(t *testing.T) {
	f := worldForm()
	typeText(f, "fm")
	for _, k := range []tea.KeyPressMsg{{Code: tea.KeyTab}, {Code: tea.KeyDown}} {
		if !f.key(k) {
			t.Fatalf("%s not handled by a multi-field form", k.String())
		}
	}
	typeText(f, "23")
	f.key(tea.KeyPressMsg{Code: tea.KeyUp})
	typeText(f, "h")
	if f.value(0) != "fm" || f.value(1) != "h" || f.value(2) != "23" {
		t.Errorf("values = %q %q %q", f.value(0), f.value(1), f.value(2))
	}
	f.key(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	f.key(tea.KeyPressMsg{Code: tea.KeyUp}) // stops at the top
	if f.focus != 0 {
		t.Errorf("focus = %d, want 0", f.focus)
	}
	f.key(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}) // Shift+Tab wraps
	if f.focus != 4 {
		t.Errorf("focus = %d, want 4", f.focus)
	}
}

func TestFormEnterAdvancesToButton(t *testing.T) {
	f := worldForm()
	for want := 1; want <= 4; want++ {
		if !f.key(tea.KeyPressMsg{Code: tea.KeyEnter}) || f.focus != want {
			t.Fatalf("Enter: focus = %d, want %d", f.focus, want)
		}
	}
	if f.key(tea.KeyPressMsg{Code: tea.KeyEnter}) {
		t.Error("Enter on the button should be left to the owner")
	}
	if !f.pressed() {
		t.Error("the button isn't focused")
	}
}

func TestFormToggle(t *testing.T) {
	f := worldForm()
	f.focus = 3
	if f.on(3) {
		t.Fatal("a toggle starts off")
	}
	f.key(tea.KeyPressMsg{Code: ' ', Text: " "})
	if !f.on(3) {
		t.Error("Space didn't turn it on")
	}
	if !f.key(tea.KeyPressMsg{Code: 'q', Text: "q"}) || !f.on(3) {
		t.Error("typing on a toggle should do nothing, and be swallowed")
	}
	f.paste("x") // mustn't panic
}

func TestFormRejectsKeystrokes(t *testing.T) {
	name := textField("Name")
	name.accept = func(s string) error {
		if strings.Contains(s, " ") {
			return errors.New("no spaces")
		}
		return nil
	}
	f := newForm("Enter to connect", name)
	typeText(f, "Kit ")
	rows, _, _ := f.rows()
	if !strings.Contains(ansi.Strip(rows[0]), "no spaces") {
		t.Errorf("the reason isn't shown: %q", ansi.Strip(rows[0]))
	}
	typeText(f, "Fox")
	if f.value(0) != "KitFox" {
		t.Errorf("value = %q", f.value(0))
	}
	if rows, _, _ = f.rows(); strings.Contains(ansi.Strip(rows[0]), "no spaces") {
		t.Errorf("the reason outlives the next good keystroke: %q", ansi.Strip(rows[0]))
	}
	f.paste("a b")
	if f.value(0) != "KitFox" {
		t.Errorf("paste let a space in: %q", f.value(0))
	}
}

func TestFormStackedRows(t *testing.T) {
	f := worldForm()
	typeText(f, "fm")
	f.key(tea.KeyPressMsg{Code: tea.KeyTab})
	typeText(f, "muck.example.org")
	rows, row, col := f.rows()
	var got []string
	for _, r := range rows {
		got = append(got, ansi.Strip(r))
	}
	want := []string{
		"World: fm",
		"Host:  muck.example.org",
		"Port:  ",
		"TLS:   [ ]",
		"[ Save ]  · Enter next field · Esc to cancel",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("rows =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if row != 1 || col != len("Host:  muck.example.org") {
		t.Errorf("cursor = %d,%d", row, col)
	}
	f.key(tea.KeyPressMsg{Code: tea.KeyDown})
	f.key(tea.KeyPressMsg{Code: tea.KeyDown})
	if _, row, col = f.rows(); row != 3 || col != len("TLS:   [") {
		t.Errorf("toggle cursor = %d,%d", row, col)
	}
	f.key(tea.KeyPressMsg{Code: tea.KeyDown})
	if _, row, col = f.rows(); row != 4 || col != len("[ ") {
		t.Errorf("button cursor = %d,%d", row, col)
	}
}
