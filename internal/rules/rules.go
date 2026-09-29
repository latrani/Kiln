// Package rules works out how a classified line looks and behaves: its
// tags' styles from the theme, and attention and quiet from tag lists.
package rules

import (
	"cmp"
	"slices"
	"strings"

	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

// Highlighter is one character's look and behavior for its lines.
type Highlighter struct {
	th               *theme.Theme
	attention, quiet []string
}

// Result is how a line is drawn and what it asks for.
type Result struct {
	Runs      []style.Run // nil: the line as the server sent it
	Attention bool        // never with Quiet
	Quiet     bool        // not unread, no attention, no notification
}

// New makes a Highlighter drawing tags in th's styles. A line asks for
// attention (or is quiet) when one of its tags is in the list, or is
// under one: "page" covers "page/in".
func New(th *theme.Theme, attention, quiet []string) *Highlighter {
	return &Highlighter{th: th, attention: attention, quiet: quiet}
}

// in reports whether tag is in list or under one of its entries.
func in(list []string, tag string) bool {
	return slices.ContainsFunc(list, func(x string) bool { return tag == x || strings.HasPrefix(tag, x+"/") })
}

// hit is one styled tag name on a line, with the spans of the line's
// tags that fall back to it.
type hit struct {
	ts    theme.TagStyle
	spans []classify.Span
}

// Styled is the tag whose style tag takes: tag itself, or the nearest
// styled one up its slashes. ok is false when none has a style.
func (h *Highlighter) Styled(tag string) (styled string, ok bool) {
	styled, _, ok = h.th.Tag(tag)
	return styled, ok
}

// Apply works out plain's runs and behavior from its tags. Each tag
// takes the style of the most specific styled name up its slashes; tags
// that land on the same name count once. Whole-line styles fold first,
// in tag order, then match-scope ones on top of the text they matched.
// Later colors win and attributes add up. Quiet wins over attention.
func (h *Highlighter) Apply(plain string, tags []classify.Tag) Result {
	var res Result
	var hits []hit
	at := map[string]int{}
	for _, t := range tags {
		res.Attention = res.Attention || in(h.attention, t.Name)
		res.Quiet = res.Quiet || in(h.quiet, t.Name)
		name, ts, ok := h.th.Tag(t.Name)
		if !ok {
			continue
		}
		if i, ok := at[name]; ok {
			hits[i].spans = append(hits[i].spans, t.Spans...)
			continue
		}
		at[name] = len(hits)
		hits = append(hits, hit{ts: ts, spans: slices.Clone(t.Spans)})
	}
	if res.Quiet {
		res.Attention = false
	}
	hits = slices.DeleteFunc(hits, func(h hit) bool { return h.ts.Match && len(h.spans) == 0 })
	if len(hits) == 0 {
		return res
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return cmp.Compare(b2i(a.ts.Match), b2i(b.ts.Match)) })
	cuts := []int{0, len(plain)}
	for _, h := range hits {
		if h.ts.Match {
			for _, s := range h.spans {
				cuts = append(cuts, s.Start, s.End)
			}
		}
	}
	slices.Sort(cuts)
	cuts = slices.Compact(cuts)
	for i := 0; i+1 < len(cuts); i++ {
		var st theme.Style
		for _, h := range hits {
			if !h.ts.Match || covers(h.spans, cuts[i], cuts[i+1]) {
				st = st.Over(h.ts.Style)
			}
		}
		run := style.Run{Start: cuts[i], End: cuts[i+1], SGR: st.SGR()}
		if n := len(res.Runs); n > 0 && res.Runs[n-1].SGR == run.SGR {
			res.Runs[n-1].End = run.End
			continue
		}
		res.Runs = append(res.Runs, run)
	}
	if len(res.Runs) == 1 && res.Runs[0].SGR == "" {
		res.Runs = nil // styles that set nothing
	}
	return res
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// covers reports whether [start, end) lies inside one of spans. Runs are
// cut at every span edge, so a run is wholly inside a span or outside it.
func covers(spans []classify.Span, start, end int) bool {
	return slices.ContainsFunc(spans, func(s classify.Span) bool { return s.Start <= start && end <= s.End })
}
