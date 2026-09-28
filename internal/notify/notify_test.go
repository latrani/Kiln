package notify

import (
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	for _, s := range []string{"all", "first", "attention", "none"} {
		if l, err := ParseLevel(s); err != nil || string(l) != s {
			t.Errorf("ParseLevel(%q) = %q, %v", s, l, err)
		}
	}
	if _, err := ParseLevel("loud"); err == nil || !strings.Contains(err.Error(), `"all", "first", "attention" or "none"`) {
		t.Errorf("bad level error = %v", err)
	}
}

func TestParseMethod(t *testing.T) {
	for _, s := range []string{"osc", "bell", "both"} {
		if m, err := ParseMethod(s); err != nil || string(m) != s {
			t.Errorf("ParseMethod(%q) = %q, %v", s, m, err)
		}
	}
	if _, err := ParseMethod("smoke"); err == nil {
		t.Error("ParseMethod accepted smoke")
	}
}

func TestMessageSanitizes(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{"Rook pages: hi", "Kit: Rook pages: hi"},
		{"\x1b[1;31mred\x1b[0m text", "Kit: red text"},
		{"a\x1b]0;pwned\x07b", "Kit: ab"},
		{"bell\x07 tab\t cr\r", "Kit: bell tab cr"},
		{"c1\u009b31m csi", "Kit: c131m csi"},
		{"del\x7f", "Kit: del"},
		{"bad\xffutf8", "Kit: badutf8"},
		{"  spaced  ", "Kit: spaced"},
	} {
		if got := Message("Kit", c.line); got != c.want {
			t.Errorf("Message(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

func TestMessageTruncates(t *testing.T) {
	got := Message("Kit", strings.Repeat("é", 300))
	if r := []rune(got); len(r) != MaxLen || r[len(r)-1] != '…' {
		t.Errorf("len %d, ends %q", len(r), string(r[len(r)-1]))
	}
	short := Message("Kit", strings.Repeat("x", MaxLen-5))
	if len([]rune(short)) != MaxLen || strings.HasSuffix(short, "…") {
		t.Errorf("exactly MaxLen should not be cut: %q", short)
	}
}

func TestEncode(t *testing.T) {
	for _, c := range []struct {
		m    Method
		tmux bool
		want string
	}{
		{OSC, false, "\x1b]9;hi\x07"},
		{Bell, false, "\x07"},
		{Both, false, "\x1b]9;hi\x07\x07"},
		{OSC, true, "\x1bPtmux;\x1b\x1b]9;hi\x07\x1b\\"},
		{Bell, true, "\x07"},
		{Both, true, "\x1bPtmux;\x1b\x1b]9;hi\x07\x1b\\\x07"},
	} {
		if got := Encode("hi", c.m, c.tmux); got != c.want {
			t.Errorf("Encode(%s, tmux=%v) = %q, want %q", c.m, c.tmux, got, c.want)
		}
	}
}
