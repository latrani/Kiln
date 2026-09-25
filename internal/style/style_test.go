package style

import (
	"testing"

	"github.com/latrani/Kiln/internal/config"
)

func TestSGR(t *testing.T) {
	cases := []struct {
		in   config.Style
		want string
	}{
		{config.Style{}, ""},
		{config.Style{Bold: true}, "\x1b[1m"},
		{config.Style{FG: "#ff9f43", Bold: true}, "\x1b[1;38;2;255;159;67m"},
		{config.Style{BG: "#330000", Italic: true, Underline: true}, "\x1b[3;4;48;2;51;0;0m"},
		{config.Style{FG: "orange"}, ""},
		{config.Style{FG: "#zzzzzz"}, ""},
	}
	for _, c := range cases {
		if got := SGR(c.in); got != c.want {
			t.Errorf("SGR(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestApply(t *testing.T) {
	if got := Apply("x", config.Style{}); got != "x\x1b[0m" {
		t.Errorf("unstyled = %q", got)
	}
	got := Apply("\x1b[1mMira\x1b[0m pages", config.Style{Italic: true})
	want := "\x1b[3m\x1b[1mMira\x1b[0m\x1b[3m pages\x1b[0m"
	if got != want {
		t.Errorf("Apply = %q, want %q", got, want)
	}
}
