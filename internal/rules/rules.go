// Package rules applies highlight rules to classified lines.
package rules

import (
	"regexp"
	"slices"

	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/config"
)

// Highlighter holds one character's compiled highlight rules.
type Highlighter struct {
	rules []compiled
}

type compiled struct {
	tags      []string
	re        *regexp.Regexp // nil = no pattern constraint
	style     config.Style
	attention bool
	match     bool // scope = "match": style only the rule's spans
}

// Run is a stretch of a line's plain text and the style it gets.
type Run struct {
	Start, End int
	Style      config.Style
	Styled     bool // false: left as the server sent it
}

// Result is the combined effect of every rule that applies to a line.
type Result struct {
	Style     config.Style // the whole line's style, when Runs is nil
	Styled    bool         // some rule styled some of the line
	Attention bool
	Runs      []Run // with match-scope rules: the line, cut into styled runs
}

// New compiles rules.
func New(rules []config.HighlightRule) (*Highlighter, error) {
	h := &Highlighter{}
	for _, r := range rules {
		c := compiled{tags: r.Match.Tags, style: r.Style, attention: r.Attention, match: r.Scope == "match"}
		if r.Match.Pattern != "" {
			re, err := regexp.Compile(r.Match.Pattern)
			if err != nil {
				return nil, err
			}
			c.re = re
		}
		h.rules = append(h.rules, c)
	}
	return h, nil
}

// hit is a rule that applies to a line, with its spans (nil: the whole line).
type hit struct {
	style config.Style
	spans []classify.Span
}

// Apply evaluates every rule against a line's plain text and tags. A rule
// applies when the line has one of its tags (if any) and matches its
// pattern (if any). Each character's style folds the rules that cover it,
// in order: later non-empty colors override earlier ones, and
// bold/italic/underline/attention accumulate. A whole-line rule covers
// every character; a match-scope rule covers its pattern's matches, or
// else its tags' spans.
func (h *Highlighter) Apply(plain string, tags []classify.Tag) Result {
	var res Result
	var hits []hit
	partial := false
	for _, r := range h.rules {
		if len(r.tags) > 0 && !slices.ContainsFunc(tags, func(t classify.Tag) bool { return slices.Contains(r.tags, t.Name) }) {
			continue
		}
		if r.re != nil && !r.re.MatchString(plain) {
			continue
		}
		res.Attention = res.Attention || r.attention
		if !r.match {
			hits = append(hits, hit{style: r.style})
			continue
		}
		if spans := r.spans(plain, tags); len(spans) > 0 {
			hits = append(hits, hit{style: r.style, spans: spans})
			partial = true
		}
	}
	if len(hits) == 0 {
		return res
	}
	res.Styled = true
	if !partial {
		for _, h := range hits {
			res.Style = fold(res.Style, h.style)
		}
		return res
	}
	cuts := []int{0, len(plain)}
	for _, h := range hits {
		for _, s := range h.spans {
			cuts = append(cuts, s.Start, s.End)
		}
	}
	slices.Sort(cuts)
	cuts = slices.Compact(cuts)
	for i := 0; i+1 < len(cuts); i++ {
		run := Run{Start: cuts[i], End: cuts[i+1]}
		for _, h := range hits {
			if h.spans == nil || covers(h.spans, run.Start, run.End) {
				run.Style, run.Styled = fold(run.Style, h.style), true
			}
		}
		if n := len(res.Runs); n > 0 && res.Runs[n-1].Style == run.Style && res.Runs[n-1].Styled == run.Styled {
			res.Runs[n-1].End = run.End
			continue
		}
		res.Runs = append(res.Runs, run)
	}
	return res
}

// spans is where a match-scope rule styles: its pattern's non-empty
// matches, or else the spans of its tags on the line.
func (r compiled) spans(plain string, tags []classify.Tag) []classify.Span {
	var out []classify.Span
	if r.re != nil {
		for _, m := range r.re.FindAllStringIndex(plain, -1) {
			if m[0] < m[1] {
				out = append(out, classify.Span{Start: m[0], End: m[1]})
			}
		}
		return out
	}
	for _, t := range tags {
		if slices.Contains(r.tags, t.Name) {
			out = append(out, t.Spans...)
		}
	}
	return out
}

// covers reports whether [start, end) lies inside one of spans. Runs are
// cut at every span edge, so a run is wholly inside a span or outside it.
func covers(spans []classify.Span, start, end int) bool {
	return slices.ContainsFunc(spans, func(s classify.Span) bool { return s.Start <= start && end <= s.End })
}

// fold layers b over a: b's non-empty colors win, attributes accumulate.
func fold(a, b config.Style) config.Style {
	if b.FG != "" {
		a.FG = b.FG
	}
	if b.BG != "" {
		a.BG = b.BG
	}
	a.Bold = a.Bold || b.Bold
	a.Italic = a.Italic || b.Italic
	a.Underline = a.Underline || b.Underline
	return a
}
