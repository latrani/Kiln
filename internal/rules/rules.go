// Package rules works out what a classified line asks for (Judge:
// priority and quiet, from tag lists) and how it looks (Highlighter:
// its tags' styles from the theme).
package rules

import (
	"cmp"
	"slices"
	"strings"

	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

// Judge decides what a line asks for from its tags. It doesn't depend
// on the theme.
type Judge struct {
	Priority, Quiet []string
}

// Verdict is what a line asks for.
type Verdict struct {
	Priority bool // never with Quiet
	Quiet    bool // not unread, not priority, no notification
}

// Of works out what a line with tags asks for. A line is a priority
// (or quiet) when one of its tags is in the list, or is under one:
// "page" covers "page/in". Quiet wins over priority.
func (j Judge) Of(tags []classify.Tag) Verdict {
	var v Verdict
	for _, t := range tags {
		v.Priority = v.Priority || in(j.Priority, t.Name)
		v.Quiet = v.Quiet || in(j.Quiet, t.Name)
	}
	if v.Quiet {
		v.Priority = false
	}
	return v
}

// Highlighter draws a character's tags in a theme's styles.
type Highlighter struct {
	th *theme.Theme
}

// New makes a Highlighter drawing tags in th's styles.
func New(th *theme.Theme) *Highlighter {
	return &Highlighter{th: th}
}

// in reports whether tag is in list or under one of its entries.
func in(list []string, tag string) bool {
	return slices.ContainsFunc(list, func(x string) bool { return tag == x || strings.HasPrefix(tag, x+"/") })
}

// hit is one styled tag name on a line, with the spans of the line's
// tags that fall back to it.
type hit struct {
	name  string
	ts    theme.TagStyle
	spans []classify.Span
}

// Styled is the tag whose style tag takes: tag itself, or the nearest
// styled one up its slashes. ok is false when none has a style.
func (h *Highlighter) Styled(tag string) (styled string, ok bool) {
	styled, _, ok = h.th.Tag(tag)
	return styled, ok
}

// TagRun is a stretch [Start, End) of a line's plain text and the styled
// tag names drawn over it, in the order their styles fold.
type TagRun struct {
	Start, End int
	Names      []string
}

// TagRuns works out which styled tags draw over each stretch of plain;
// nil means none does. Each tag takes the style of the most specific
// styled name up its slashes; tags that land on the same name count
// once. The names come in the order their styles fold: whole-line ones
// first, in tag order, then match-scope ones over the text they matched,
// in tag order. The runs cover all of plain, and neighbors differ.
func (h *Highlighter) TagRuns(plain string, tags []classify.Tag) []TagRun {
	var hits []hit
	at := map[string]int{}
	for _, t := range tags {
		name, ts, ok := h.th.Tag(t.Name)
		if !ok {
			continue
		}
		if i, ok := at[name]; ok {
			hits[i].spans = append(hits[i].spans, t.Spans...)
			continue
		}
		at[name] = len(hits)
		hits = append(hits, hit{name: name, ts: ts, spans: slices.Clone(t.Spans)})
	}
	hits = slices.DeleteFunc(hits, func(h hit) bool { return h.ts.Match && len(h.spans) == 0 })
	if len(hits) == 0 {
		return nil
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
	var runs []TagRun
	for i := 0; i+1 < len(cuts); i++ {
		var names []string
		for _, h := range hits {
			if !h.ts.Match || covers(h.spans, cuts[i], cuts[i+1]) {
				names = append(names, h.name)
			}
		}
		if n := len(runs); n > 0 && slices.Equal(runs[n-1].Names, names) {
			runs[n-1].End = cuts[i+1]
			continue
		}
		runs = append(runs, TagRun{Start: cuts[i], End: cuts[i+1], Names: names})
	}
	return runs
}

// Runs works out plain's styled runs from its tags; nil means the line
// as the server sent it. Each run folds its TagRuns names' styles: later
// colors win and attributes add up.
func (h *Highlighter) Runs(plain string, tags []classify.Tag) []style.Run {
	trs := h.TagRuns(plain, tags)
	if trs == nil {
		return nil
	}
	var runs []style.Run
	for _, tr := range trs {
		var st theme.Style
		for _, n := range tr.Names {
			_, ts, _ := h.th.Tag(n)
			st = st.Over(ts.Style)
		}
		run := style.Run{Start: tr.Start, End: tr.End, SGR: st.SGR()}
		if n := len(runs); n > 0 && runs[n-1].SGR == run.SGR {
			runs[n-1].End = run.End
			continue
		}
		runs = append(runs, run)
	}
	if len(runs) == 1 && runs[0].SGR == "" {
		return nil // styles that set nothing
	}
	return runs
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
