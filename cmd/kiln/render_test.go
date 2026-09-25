package main

import (
	"testing"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/rules"
)

func TestRender(t *testing.T) {
	in := func(s string) logstore.Entry { return logstore.Entry{Dir: logstore.In, Text: s} }
	cases := []struct {
		name string
		e    logstore.Entry
		res  rules.Result
		want string
	}{
		{"plain", in("hi"), rules.Result{}, "hi\x1b[0m"},
		{"sent", logstore.Entry{Dir: logstore.Out, Text: ":grins."}, rules.Result{}, "\x1b[2m> :grins.\x1b[0m"},
		{"sys", logstore.Entry{Dir: logstore.Sys, Text: "connected"}, rules.Result{}, "\x1b[2m* connected\x1b[0m"},
		{"styled attention", in("Mira pages: hi"),
			rules.Result{Style: config.Style{FG: "#ff9f43", Bold: true}, Styled: true, Attention: true},
			"\x1b[1;38;2;255;159;67m» Mira pages: hi\x1b[0m"},
		{"style survives server reset", in("\x1b[1mMira\x1b[0m pages"),
			rules.Result{Style: config.Style{Italic: true}, Styled: true},
			"\x1b[3m\x1b[1mMira\x1b[0m\x1b[3m pages\x1b[0m"},
		{"partial", in("PAGE: hi"),
			rules.Result{Styled: true, Attention: true, Runs: []rules.Run{
				{Start: 0, End: 5, Style: config.Style{Bold: true}, Styled: true}, {Start: 5, End: 8}}},
			"» \x1b[1mPAGE:\x1b[0m hi\x1b[0m"},
		{"sanitized", in("\x1b]52;c;cHduZWQ=\x07\x1b]0;title\x07hi\x1b[2J"), rules.Result{}, "hi\x1b[0m"},
		{"spans align after sanitizing", in("\x1b]0;t\x07\tPAGE: hi"),
			rules.Result{Styled: true, Runs: []rules.Run{
				{Start: 0, End: 4}, {Start: 4, End: 9, Style: config.Style{Bold: true}, Styled: true}, {Start: 9, End: 12}}},
			"    \x1b[1mPAGE:\x1b[0m hi\x1b[0m"},
		{"bad color ignored", in("x"), rules.Result{Style: config.Style{FG: "orange"}, Styled: true}, "x\x1b[0m"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := render(c.e, c.res); got != c.want {
				t.Errorf("render = %q, want %q", got, c.want)
			}
		})
	}
}
