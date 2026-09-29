package style

import (
	"testing"
)

func TestAfterReset(t *testing.T) {
	cases := []struct {
		params, tail string
		reset        bool
	}{
		{"", "", true}, {"0", "", true}, {"00", "", true}, {"1;0", "", true},
		{"0;31", "31", true}, {"1;;4", "4", true}, {"1;0;31;0;4", "4", true},
		{"31", "", false}, {"38;5;0", "", false}, {"48;2;0;0;0;1", "", false},
		{"38;5;0;0", "", true},
	}
	for _, c := range cases {
		if tail, reset := afterReset(c.params); tail != c.tail || reset != c.reset {
			t.Errorf("afterReset(%q) = %q, %v; want %q, %v", c.params, tail, reset, c.tail, c.reset)
		}
	}
}

func TestApply(t *testing.T) {
	if got := Apply("x", ""); got != "x\x1b[0m" {
		t.Errorf("unstyled = %q", got)
	}
	got := Apply("\x1b[1mMira\x1b[0m pages", "\x1b[3m")
	want := "\x1b[1m\x1b[3mMira\x1b[0m\x1b[3m pages\x1b[0m"
	if got != want {
		t.Errorf("Apply = %q, want %q", got, want)
	}
}

func TestHighlight(t *testing.T) {
	b := "\x1b[38;2;32;83;255m"
	under := "\x1b[4m"
	styled := func(start, end int, sgr string) Run { return Run{Start: start, End: end, SGR: sgr} }
	plain := func(start, end int) Run { return Run{Start: start, End: end} }
	runs := func(rs ...Run) []Run { return rs }
	cases := []struct {
		name, text string
		runs       []Run
		want       string
	}{
		{"prefix, then the server's color comes back", "\x1b[32mPAGE: hi",
			runs(styled(0, 5, b), plain(5, 9)),
			"\x1b[32m" + b + "PAGE:" + Reset + "\x1b[32m hi" + Reset},
		{"a server reset inside a span doesn't end it", "\x1b[1mPA\x1b[0mGE: hi",
			runs(styled(0, 5, b), plain(5, 9)),
			"\x1b[1m" + b + "PA\x1b[0m" + b + "GE:" + Reset + " hi" + Reset},
		{"a span in the middle", "hi Kit!",
			runs(plain(0, 3), styled(3, 6, under), plain(6, 7)),
			"hi \x1b[4mKit" + Reset + "!" + Reset},
		{"multi-byte", "hi Zoë!",
			runs(plain(0, 3), styled(3, 7, under), plain(7, 8)),
			"hi \x1b[4mZoë" + Reset + "!" + Reset},
		{"non-SGR escapes don't shift spans", "\x1b]0;title\x07PAGE: hi",
			runs(styled(0, 5, b), plain(5, 9)),
			"\x1b]0;title\x07" + b + "PAGE:" + Reset + " hi" + Reset},
		{"span to the end, then a trailing reset", "PAGE:\x1b[0m",
			runs(styled(0, 5, b)),
			b + "PAGE:\x1b[0m" + b + Reset},
		{"uncommon resets are resets", "\x1b[1mPA\x1b[00mGE\x1b[1;0m: hi",
			runs(styled(0, 5, b), plain(5, 9)),
			"\x1b[1m" + b + "PA\x1b[00m" + b + "GE\x1b[1;0m" + b + ":" + Reset + " hi" + Reset},
		{"server state resumes from after the last reset", "PA\x1b[1;0;31mGE: hi",
			runs(styled(0, 5, b), plain(5, 9)),
			b + "PA\x1b[1;0;31m" + b + "GE:" + Reset + "\x1b[31m hi" + Reset},
		{"a server color inside a span doesn't override it", "PA\x1b[31mGE: hi",
			runs(styled(0, 5, b), plain(5, 9)),
			b + "PA\x1b[31m" + b + "GE:" + Reset + "\x1b[31m hi" + Reset},
		{"a 0 inside a color isn't a reset", "\x1b[1mPA\x1b[38;5;0;48;2;0;0;0mGE: hi",
			runs(styled(0, 5, b), plain(5, 9)),
			"\x1b[1m" + b + "PA\x1b[38;5;0;48;2;0;0;0m" + b + "GE:" + Reset + "\x1b[1m\x1b[38;5;0;48;2;0;0;0m hi" + Reset},
		{"unstyled", "\x1b[31mhi", nil, "\x1b[31mhi" + Reset},
		{"whole line", "hi", runs(styled(0, 2, "\x1b[3m")), "\x1b[3mhi" + Reset},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Highlight(c.text, c.runs); got != c.want {
				t.Errorf("Highlight = %q\nwant        %q", got, c.want)
			}
		})
	}
}

func TestReassert(t *testing.T) {
	base := "\x1b[48;2;1;2;3m"
	for _, c := range []struct{ in, want string }{
		{"a\x1b[0mb", "a\x1b[0m" + base + "b"},
		{"a\x1b[mb", "a\x1b[m" + base + "b"},
		{"a\x1b[1;31mb", "a\x1b[1;31mb"},             // no reset: left alone
		{"a\x1b[0;31mb", "a\x1b[0;31m" + base + "b"}, // a reset, then red
		{"a\x1b[38;5;0mb", "a\x1b[38;5;0mb"},         // black, not a reset
		{"plain", "plain"},
	} {
		if got := Reassert(c.in, base); got != c.want {
			t.Errorf("Reassert(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
