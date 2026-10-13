package webapi

import (
	"slices"
	"strings"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/rules"
)

// Span is a run of a line's text with one look: the server's style, the
// styled tags over it in the order their styles fold (see
// rules.TagRuns), and the link it's part of. The page draws it by laying
// each tag's style over the server's: a set color replaces, flags add up.
type Span struct {
	Text string
	ansi.SpanStyle
	Tags []string `json:",omitempty"`
	Link string   `json:",omitempty"`
}

// spansOf splits text (sanitized, the server's escape codes in it) into
// spans: cut wherever the server's style, the styled tags (from hl; nil
// for none) or a link change, and merged where none does.
func spansOf(text string, hl *rules.Highlighter, tags []classify.Tag) []Span {
	type seg struct {
		start, end int
		st         ansi.SpanStyle
	}
	var segs []seg
	var b strings.Builder
	for _, s := range ansi.Spans(text) {
		segs = append(segs, seg{b.Len(), b.Len() + len(s.Text), s.Style})
		b.WriteString(s.Text)
	}
	if len(segs) == 0 {
		return nil
	}
	plain := b.String()
	var runs []rules.TagRun
	if hl != nil {
		runs = hl.TagRuns(plain, tags)
	}
	links := app.Links(plain)
	cuts := []int{0, len(plain)}
	for _, s := range segs {
		cuts = append(cuts, s.start)
	}
	for _, r := range runs {
		cuts = append(cuts, r.Start, r.End)
	}
	for _, l := range links {
		cuts = append(cuts, l[0], l[1])
	}
	slices.Sort(cuts)
	cuts = slices.Compact(cuts)
	var out []Span
	si, ri, li := 0, 0, 0
	for i := 0; i+1 < len(cuts); i++ {
		a, z := cuts[i], cuts[i+1]
		for si+1 < len(segs) && segs[si].end <= a {
			si++
		}
		for ri < len(runs) && runs[ri].End <= a {
			ri++
		}
		for li < len(links) && links[li][1] <= a {
			li++
		}
		sp := Span{Text: plain[a:z], SpanStyle: segs[si].st}
		if ri < len(runs) && runs[ri].Start <= a {
			sp.Tags = runs[ri].Names
		}
		if li < len(links) && links[li][0] <= a {
			sp.Link = plain[links[li][0]:links[li][1]]
		}
		if n := len(out); n > 0 && out[n-1].SpanStyle == sp.SpanStyle && out[n-1].Link == sp.Link && slices.Equal(out[n-1].Tags, sp.Tags) {
			out[n-1].Text += sp.Text
			continue
		}
		out = append(out, sp)
	}
	return out
}
