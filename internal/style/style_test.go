package style

import (
	"testing"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/rules"
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

func TestHighlight(t *testing.T) {
	blue := config.Style{FG: "#2053ff"}
	b := SGR(blue)
	styled := func(start, end int, s config.Style) rules.Run {
		return rules.Run{Start: start, End: end, Style: s, Styled: true}
	}
	plain := func(start, end int) rules.Run { return rules.Run{Start: start, End: end} }
	runs := func(rs ...rules.Run) rules.Result { return rules.Result{Styled: true, Runs: rs} }
	cases := []struct {
		name, text string
		res        rules.Result
		want       string
	}{
		{"prefix, then the server's color comes back", "\x1b[32mPAGE: hi",
			runs(styled(0, 5, blue), plain(5, 9)),
			"\x1b[32m" + b + "PAGE:" + Reset + "\x1b[32m hi" + Reset},
		{"a server reset inside a span doesn't end it", "\x1b[1mPA\x1b[0mGE: hi",
			runs(styled(0, 5, blue), plain(5, 9)),
			"\x1b[1m" + b + "PA\x1b[0m" + b + "GE:" + Reset + " hi" + Reset},
		{"a span in the middle", "hi Kit!",
			runs(plain(0, 3), styled(3, 6, config.Style{Underline: true}), plain(6, 7)),
			"hi \x1b[4mKit" + Reset + "!" + Reset},
		{"multi-byte", "hi Zoë!",
			runs(plain(0, 3), styled(3, 7, config.Style{Underline: true}), plain(7, 8)),
			"hi \x1b[4mZoë" + Reset + "!" + Reset},
		{"non-SGR escapes don't shift spans", "\x1b]0;title\x07PAGE: hi",
			runs(styled(0, 5, blue), plain(5, 9)),
			"\x1b]0;title\x07" + b + "PAGE:" + Reset + " hi" + Reset},
		{"span to the end, then a trailing reset", "PAGE:\x1b[0m",
			runs(styled(0, 5, blue)),
			b + "PAGE:\x1b[0m" + b + Reset},
		{"unstyled", "\x1b[31mhi", rules.Result{}, "\x1b[31mhi" + Reset},
		{"whole line", "hi", rules.Result{Styled: true, Style: config.Style{Italic: true}}, "\x1b[3mhi" + Reset},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Highlight(c.text, c.res); got != c.want {
				t.Errorf("Highlight = %q\nwant        %q", got, c.want)
			}
		})
	}
}
