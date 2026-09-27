package ansi

import (
	"reflect"
	"strings"
	"testing"
)

func TestSanitize(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain", "hello", "hello"},
		{"keeps sgr", "\x1b[1;31mred\x1b[0m", "\x1b[1;31mred\x1b[0m"},
		{"drops osc title", "a\x1b]0;pwned\x07b", "ab"},
		{"drops osc52 clipboard", "a\x1b]52;c;ZXZpbA==\x1b\\b", "ab"},
		{"drops cursor movement", "a\x1b[2Jb\x1b[10;10Hc", "abc"},
		{"drops bell and backspace", "a\x07b\x08c\rd", "abcd"},
		{"expands tab", "a\tb", "a    b"},
		{"keeps unicode", "Zoë 🦊 日本", "Zoë 🦊 日本"},
		{"unterminated csi", "a\x1b[31", "a"},
		{"lone esc", "a\x1b", "a"},
		{"osc then sgr then text", "\x1b]0;t\x07hi \x1b[31mred", "hi \x1b[31mred"},
		{"repeated charset resets", "\x1b(Bx\x1b(By\x1b(Bz", "xyz"},
		{"two-byte esc mid-line", "a\x1b7b\x1b8c", "abc"},
		{"drops csi m with nested esc", "a\x1b[\x1b#8mb", "ab"},
		{"drops csi m with bel in params", "a\x1b[1;\x07mb", "ab"},
		{"drops csi m with intermediate", "a\x1b[1 mb", "ab"},
		{"keeps colon sgr", "\x1b[38:5:196mx", "\x1b[38:5:196mx"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Sanitize(c.in); got != c.want {
				t.Errorf("Sanitize(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestWrapPlain(t *testing.T) {
	got := Wrap("the quick brown fox", 10)
	want := []string{"the quick", "brown fox"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Wrap = %q, want %q", got, want)
	}
}

func TestWrapCarriesStyleToNextRow(t *testing.T) {
	got := Wrap("\x1b[31mred red red\x1b[0m done", 8)
	if len(got) < 2 {
		t.Fatalf("Wrap = %q, want 2+ rows", got)
	}
	if got[1][:5] != "\x1b[31m" {
		t.Errorf("row 1 = %q, want it to reopen red", got[1])
	}
	last := got[len(got)-1]
	if Width(last) == 0 || last == "" {
		t.Errorf("last row empty: %q", got)
	}
}

func TestWrapResetStopsCarry(t *testing.T) {
	got := Wrap("\x1b[31mred\x1b[0m plain plain plain", 10)
	for i, row := range got[1:] {
		if len(row) >= 2 && row[:2] == "\x1b[" {
			t.Errorf("row %d = %q carries style after reset", i+1, row)
		}
	}
}

func TestWrapWideChars(t *testing.T) {
	for _, row := range Wrap("日本語日本語日本語", 7) {
		if w := Width(row); w > 7 {
			t.Errorf("row %q is %d cells wide, limit 7", row, w)
		}
	}
}

func TestWrapLongWordHardBreaks(t *testing.T) {
	for _, row := range Wrap("aaaaaaaaaaaaaaaaaaaa", 6) {
		if w := Width(row); w > 6 {
			t.Errorf("row %q is %d wide", row, w)
		}
	}
}

func TestWrapEmpty(t *testing.T) {
	if got := Wrap("", 10); len(got) != 1 || got[0] != "" {
		t.Errorf("Wrap(\"\") = %q", got)
	}
}

func TestWrapIndent(t *testing.T) {
	cases := []struct {
		in   string
		w    int
		want []string
	}{
		{"hello world", 20, []string{"hello world"}},
		{"0123456789", 10, []string{"0123456789"}}, // exactly fits: no early wrap
		{"one two three four", 9, []string{"one two", " three", " four"}},
		{"abcdefghijklmn", 6, []string{"abcdef", " ghijk", " lmn"}},
		{"\x1b[31mred text here\x1b[0m ok", 9,
			[]string{"\x1b[31mred text", " \x1b[31mhere\x1b[0m ok"}},
	}
	for _, c := range cases {
		got := WrapIndent(c.in, c.w, 1)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("WrapIndent(%q, %d) = %q, want %q", c.in, c.w, got, c.want)
		}
		for i, r := range got {
			if Width(r) > c.w {
				t.Errorf("%q row %d is %d wide", c.in, i, Width(r))
			}
		}
	}
}
