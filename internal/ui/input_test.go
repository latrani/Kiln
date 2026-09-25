package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/latrani/Kiln/internal/ansi"
)

func typed(s string) *Input {
	in := NewInput()
	in.InsertText(s)
	return in
}

func TestInputEditing(t *testing.T) {
	in := typed("helo")
	in.Left()
	in.InsertText("l")
	if in.Value() != "hello" {
		t.Errorf("Value = %q", in.Value())
	}
	in.End()
	in.Backspace()
	in.Home()
	in.Delete()
	if in.Value() != "ell" {
		t.Errorf("Value = %q", in.Value())
	}
}

func TestInputMultiline(t *testing.T) {
	in := typed("one")
	in.Newline()
	in.InsertText("two")
	if in.Value() != "one\ntwo" {
		t.Errorf("Value = %q", in.Value())
	}
	in.Home()
	in.Backspace() // join
	if in.Value() != "onetwo" {
		t.Errorf("after join = %q", in.Value())
	}
	in.SetValue("")
	in.InsertText("a\r\nb\nc") // paste
	if in.Value() != "a\nb\nc" {
		t.Errorf("paste = %q", in.Value())
	}
	in.Up()
	in.Up()
	in.End()
	in.Delete() // joins a and b
	if in.Value() != "ab\nc" {
		t.Errorf("delete-join = %q", in.Value())
	}
}

func TestInputHistory(t *testing.T) {
	in := NewInput()
	in.InsertText("first")
	in.Commit()
	in.InsertText("second")
	in.Commit()
	in.InsertText("draft")
	in.Up()
	if in.Value() != "second" {
		t.Errorf("Up = %q", in.Value())
	}
	in.Up()
	in.Up() // clamps at oldest
	if in.Value() != "first" {
		t.Errorf("Up Up = %q", in.Value())
	}
	in.Down()
	in.Down()
	if in.Value() != "draft" {
		t.Errorf("back to draft = %q", in.Value())
	}
}

func TestInputHistorySkipsSecretsAndRepeats(t *testing.T) {
	in := NewInput()
	in.InsertText("hunter2")
	in.CommitSecret()
	in.InsertText("look")
	in.Commit()
	in.InsertText("look")
	in.Commit()
	if !reflect.DeepEqual(in.history, []string{"look"}) {
		t.Errorf("history = %q", in.history)
	}
}

func plainRows(rows []string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = ansi.Strip(r)
	}
	return out
}

func TestRenderSoftWraps(t *testing.T) {
	rows, r, c := typed("abcdefgh").Render(6, 0, false) // 4 cells of text per row
	if got := plainRows(rows); !reflect.DeepEqual(got, []string{"> abcd", "  efgh", "  "}) {
		t.Errorf("rows = %q", got)
	}
	if r != 2 || c != 2 {
		t.Errorf("cursor = %d,%d; want 2,2 (wrapped past a full row)", r, c)
	}
}

func TestRenderCursorMidLine(t *testing.T) {
	in := typed("hello")
	in.Home()
	in.Right()
	_, r, c := in.Render(20, 0, false)
	if r != 0 || c != 3 {
		t.Errorf("cursor = %d,%d; want 0,3", r, c)
	}
}

func TestRenderWideRunes(t *testing.T) {
	rows, _, _ := typed("日本語").Render(6, 0, false) // 4 cells: 2 wide runes per row
	if got := plainRows(rows); !reflect.DeepEqual(got, []string{"> 日本", "  語"}) {
		t.Errorf("rows = %q", got)
	}
}

func TestRenderOverLimitStartsAtCutByte(t *testing.T) {
	in := typed("abcdé") // é is 2 bytes: bytes 5-6
	rows, _, _ := in.Render(40, 5, false)
	if !strings.Contains(rows[0], "abcd"+overLimit+"é") {
		t.Errorf("row = %q, want red from é", rows[0])
	}
	if !in.OverLimit(5) || in.OverLimit(6) {
		t.Error("OverLimit wrong")
	}
	rows, _, _ = typed("abc").Render(40, 5, false)
	if strings.Contains(rows[0], overLimit) {
		t.Errorf("under-limit row highlighted: %q", rows[0])
	}
}

func TestRenderOverLimitIsPerLine(t *testing.T) {
	in := typed("abcdef\nxy")
	rows, _, _ := in.Render(40, 4, false)
	if !strings.Contains(rows[0], overLimit) || strings.Contains(rows[1], overLimit) {
		t.Errorf("rows = %q", rows)
	}
}

func TestRenderMasked(t *testing.T) {
	rows, _, _ := typed("pw!").Render(20, 0, true)
	if got := ansi.Strip(rows[0]); got != "> •••" {
		t.Errorf("masked = %q", got)
	}
}
