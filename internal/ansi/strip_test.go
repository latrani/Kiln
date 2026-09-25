package ansi

import "testing"

func TestStrip(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"\x1b[1mRook\x1b[0m says, \"hi\"", "Rook says, \"hi\""},
		{"\x1b[38;2;255;159;67morange\x1b[m", "orange"},
		{"a\x1b]8;;http://x\x07link\x1b]8;;\x07b", "alinkb"},
		{"a\x1b]0;title\x1b\\b", "ab"},
		{"a\x1b(Bb", "ab"},
		{"Zoë 🦊 \x1b[31m日本\x1b[0m", "Zoë 🦊 日本"},
		{"cut\x1b[3", "cut"},
		{"cut\x1b", "cut"},
	}
	for _, c := range cases {
		if got := Strip(c.in); got != c.want {
			t.Errorf("Strip(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
