package rules

import (
	"reflect"
	"testing"

	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/config"
)

// line is a whole-line Result.
func line(s config.Style, styled, attention bool) Result {
	return Result{Style: s, Styled: styled, Attention: attention}
}

// tagged makes span-less tags from names.
func tagged(names ...string) []classify.Tag {
	var tags []classify.Tag
	for _, n := range names {
		tags = append(tags, classify.Tag{Name: n})
	}
	return tags
}

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
		tags  []classify.Tag
		want  Result
	}{
		{"no match", "Rook waves.", nil, Result{}},
		{"tag match", "Mira pages: hi", tagged("page"), line(config.Style{FG: "#ff9f43", Bold: true}, true, true)},
		{"pattern match", "the lighthouse glows", nil, line(config.Style{FG: "#00ffff", Underline: true}, true, false)},
		{"any-of tags", "Rook waves to Kit.", tagged("self"), line(config.Style{Italic: true}, true, false)},
		{"later color overrides, flags accumulate", "Mira pages: the lighthouse", tagged("page"),
			line(config.Style{FG: "#00ffff", Bold: true, Underline: true}, true, true)},
		{"tags AND pattern: pattern missing", "Mira pages: hi", tagged("page"), line(config.Style{FG: "#ff9f43", Bold: true}, true, true)},
		{"tags AND pattern: both", "Mira pages: urgent", tagged("page"), line(config.Style{FG: "#ff9f43", BG: "#330000", Bold: true}, true, true)},
		{"tags AND pattern: tag missing", "urgent news", nil, Result{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := h.Apply(c.plain, c.tags); !reflect.DeepEqual(got, c.want) {
				t.Errorf("Apply = %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestApplyMatchScope(t *testing.T) {
	blue := config.Style{FG: "#2053ff", Bold: true}
	pageSpans := []classify.Tag{{Name: "page", Spans: []classify.Span{{Start: 0, End: 5}}}}
	const plain = "PAGE: hi Kit" // PAGE: 0-5, Kit 9-12

	t.Run("match and line rules fold per character", func(t *testing.T) {
		h, _ := New([]config.HighlightRule{
			{Match: config.Match{Tags: []string{"page"}}, Style: blue, Scope: "match", Attention: true},
			{Match: config.Match{Pattern: `Kit`}, Style: config.Style{Underline: true}, Scope: "match"},
			{Match: config.Match{Tags: []string{"page"}}, Style: config.Style{Italic: true}}, // whole line
		})
		want := Result{Styled: true, Attention: true, Runs: []Run{
			{Start: 0, End: 5, Style: config.Style{FG: "#2053ff", Bold: true, Italic: true}, Styled: true},
			{Start: 5, End: 9, Style: config.Style{Italic: true}, Styled: true},
			{Start: 9, End: 12, Style: config.Style{Italic: true, Underline: true}, Styled: true},
		}}
		if got := h.Apply(plain, pageSpans); !reflect.DeepEqual(got, want) {
			t.Errorf("Apply = %+v\nwant    %+v", got, want)
		}
	})

	t.Run("unstyled gaps", func(t *testing.T) {
		h, _ := New([]config.HighlightRule{{Match: config.Match{Tags: []string{"page"}}, Style: blue, Scope: "match"}})
		want := Result{Styled: true, Runs: []Run{
			{Start: 0, End: 5, Style: blue, Styled: true},
			{Start: 5, End: 12},
		}}
		if got := h.Apply(plain, pageSpans); !reflect.DeepEqual(got, want) {
			t.Errorf("Apply = %+v", got)
		}
	})

	t.Run("a pattern's matches beat its tags' spans", func(t *testing.T) {
		h, _ := New([]config.HighlightRule{{Match: config.Match{Tags: []string{"page"}, Pattern: `hi`}, Style: blue, Scope: "match"}})
		want := Result{Styled: true, Runs: []Run{{Start: 0, End: 6}, {Start: 6, End: 8, Style: blue, Styled: true}, {Start: 8, End: 12}}}
		if got := h.Apply(plain, pageSpans); !reflect.DeepEqual(got, want) {
			t.Errorf("Apply = %+v", got)
		}
	})

	t.Run("overlapping spans merge", func(t *testing.T) {
		h, _ := New([]config.HighlightRule{{Match: config.Match{Tags: []string{"page"}}, Style: blue, Scope: "match"}})
		tags := []classify.Tag{{Name: "page", Spans: []classify.Span{{Start: 0, End: 4}, {Start: 0, End: 5}}}}
		want := Result{Styled: true, Runs: []Run{{Start: 0, End: 5, Style: blue, Styled: true}, {Start: 5, End: 12}}}
		if got := h.Apply(plain, tags); !reflect.DeepEqual(got, want) {
			t.Errorf("Apply = %+v", got)
		}
	})

	t.Run("no spans: no style, attention still counts", func(t *testing.T) {
		h, _ := New([]config.HighlightRule{{Match: config.Match{Tags: []string{"start"}}, Style: blue, Scope: "match", Attention: true}})
		if got := h.Apply(plain, tagged("start")); !reflect.DeepEqual(got, Result{Attention: true}) {
			t.Errorf("Apply = %+v", got)
		}
	})
}
