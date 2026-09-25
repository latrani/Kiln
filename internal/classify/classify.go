// Package classify tags lines with semantic kinds (page, whisper, …).
package classify

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/latrani/Kiln/internal/config"
)

// SelfTag is added to any line that mentions the character's own name
// or one of its aliases.
const SelfTag = "self"

// Classifier holds one character's compiled classify rules.
type Classifier struct {
	rules []rule
	names []*regexp.Regexp // one per name and alias
}

type rule struct {
	tag string
	re  *regexp.Regexp
}

// Span is a byte range [Start, End) of a line's plain text.
type Span struct{ Start, End int }

// Tag is a tag on a line and where in the line it matched.
type Tag struct {
	Name  string
	Spans []Span // nil if every match was empty (e.g. a bare `^`)
}

// New compiles rules plus the built-in self rule for name and aliases.
func New(rules []config.ClassifyRule, name string, aliases []string) (*Classifier, error) {
	c := &Classifier{}
	for _, r := range rules {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, err
		}
		c.rules = append(c.rules, rule{r.Tag, re})
	}
	for _, n := range append([]string{name}, aliases...) {
		if n = strings.TrimSpace(n); n != "" {
			c.names = append(c.names, regexp.MustCompile(`(?i)`+regexp.QuoteMeta(n)))
		}
	}
	return c, nil
}

// Classify returns the tags for plain (ANSI-stripped) text, in rule order,
// without duplicates, with SelfTag last.
func (c *Classifier) Classify(plain string) []string {
	var names []string
	for _, t := range c.Tags(plain) {
		names = append(names, t.Name)
	}
	return names
}

// Tags is Classify with where each tag matched: every non-empty match of
// each of its rules, in rule order, then by position.
func (c *Classifier) Tags(plain string) []Tag {
	var tags []Tag
	at := map[string]int{} // tag name → index in tags
	for _, r := range c.rules {
		if !r.re.MatchString(plain) {
			continue
		}
		i, ok := at[r.tag]
		if !ok {
			i = len(tags)
			at[r.tag] = i
			tags = append(tags, Tag{Name: r.tag})
		}
		for _, m := range r.re.FindAllStringIndex(plain, -1) {
			if m[0] < m[1] {
				tags[i].Spans = append(tags[i].Spans, Span{m[0], m[1]})
			}
		}
	}
	if _, ok := at[SelfTag]; !ok {
		if spans := c.selfSpans(plain); spans != nil {
			tags = append(tags, Tag{Name: SelfTag, Spans: spans})
		}
	}
	return tags
}

// selfSpans finds each whole-word mention of the character's names, in
// order. Word boundaries are Unicode-aware (Go's \b is ASCII-only, which
// would miss names like "Zoë") and checked by hand, so a span covers just
// the name and back-to-back mentions are each found. Each name is matched
// on its own, so a longer alias that isn't a whole word ("Ash Grey" in
// "Ash Greyson") can't hide a shorter name ("Ash"); a mention inside a
// longer one is dropped.
func (c *Classifier) selfSpans(plain string) []Span {
	var spans []Span
	for _, re := range c.names {
		for _, m := range re.FindAllStringIndex(plain, -1) {
			before, _ := utf8.DecodeLastRuneInString(plain[:m[0]])
			after, _ := utf8.DecodeRuneInString(plain[m[1]:])
			if m[0] > 0 && isWordRune(before) || m[1] < len(plain) && isWordRune(after) {
				continue
			}
			spans = append(spans, Span{m[0], m[1]})
		}
	}
	// By start, longest first, then drop spans inside an earlier one.
	slices.SortFunc(spans, func(a, b Span) int { return cmp.Or(a.Start-b.Start, b.End-a.End) })
	var out []Span
	for _, s := range spans {
		if n := len(out); n > 0 && s.End <= out[n-1].End {
			continue
		}
		out = append(out, s)
	}
	return out
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}
