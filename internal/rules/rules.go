// Package rules applies highlight rules to classified lines.
package rules

import (
	"regexp"
	"slices"

	"kiln/internal/config"
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
}

// Result is the combined effect of every matching rule.
type Result struct {
	Style     config.Style
	Styled    bool // at least one rule matched
	Attention bool
}

// New compiles rules.
func New(rules []config.HighlightRule) (*Highlighter, error) {
	h := &Highlighter{}
	for _, r := range rules {
		c := compiled{tags: r.Match.Tags, style: r.Style, attention: r.Attention}
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

// Apply evaluates every rule against a line's plain text and tags. All
// matching rules apply in order: later non-empty colors override earlier
// ones, and bold/italic/underline/attention accumulate.
func (h *Highlighter) Apply(plain string, tags []string) Result {
	var res Result
	for _, r := range h.rules {
		if len(r.tags) > 0 && !slices.ContainsFunc(r.tags, func(t string) bool { return slices.Contains(tags, t) }) {
			continue
		}
		if r.re != nil && !r.re.MatchString(plain) {
			continue
		}
		res.Styled = true
		res.Attention = res.Attention || r.attention
		if r.style.FG != "" {
			res.Style.FG = r.style.FG
		}
		if r.style.BG != "" {
			res.Style.BG = r.style.BG
		}
		res.Style.Bold = res.Style.Bold || r.style.Bold
		res.Style.Italic = res.Style.Italic || r.style.Italic
		res.Style.Underline = res.Style.Underline || r.style.Underline
	}
	return res
}
