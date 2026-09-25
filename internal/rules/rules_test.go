package rules

import (
	"testing"

	"github.com/latrani/Kiln/internal/config"
)

func TestApply(t *testing.T) {
	h, err := New([]config.HighlightRule{
		{Match: config.Match{Tags: []string{"page"}}, Style: config.Style{FG: "#ff9f43", Bold: true}, Attention: true},
		{Match: config.Match{Pattern: `lighthouse`}, Style: config.Style{FG: "#00ffff", Underline: true}},
		{Match: config.Match{Tags: []string{"whisper", "self"}}, Style: config.Style{Italic: true}},
		{Match: config.Match{Tags: []string{"page"}, Pattern: `urgent`}, Style: config.Style{BG: "#330000"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		plain string
		tags  []string
		want  Result
	}{
		{"no match", "Rook waves.", nil, Result{}},
		{"tag match", "Mira pages: hi", []string{"page"}, Result{config.Style{FG: "#ff9f43", Bold: true}, true, true}},
		{"pattern match", "the lighthouse glows", nil, Result{config.Style{FG: "#00ffff", Underline: true}, true, false}},
		{"any-of tags", "Rook waves to Kit.", []string{"self"}, Result{config.Style{Italic: true}, true, false}},
		{"later color overrides, flags accumulate", "Mira pages: the lighthouse", []string{"page"},
			Result{config.Style{FG: "#00ffff", Bold: true, Underline: true}, true, true}},
		{"tags AND pattern: pattern missing", "Mira pages: hi", []string{"page"}, Result{config.Style{FG: "#ff9f43", Bold: true}, true, true}},
		{"tags AND pattern: both", "Mira pages: urgent", []string{"page"}, Result{config.Style{FG: "#ff9f43", BG: "#330000", Bold: true}, true, true}},
		{"tags AND pattern: tag missing", "urgent news", nil, Result{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := h.Apply(c.plain, c.tags); got != c.want {
				t.Errorf("Apply = %+v, want %+v", got, c.want)
			}
		})
	}
}
