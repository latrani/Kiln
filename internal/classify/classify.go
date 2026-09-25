// Package classify tags lines with semantic kinds (page, whisper, …).
package classify

import (
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
	names *regexp.Regexp // the character's names, longest first; nil if none
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
	var names []string
	for _, n := range append([]string{name}, aliases...) {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}
	if len(names) > 0 {
		// Longest first, so "Kitty" is tried before "Kit".
		slices.SortStableFunc(names, func(a, b string) int { return len(b) - len(a) })
		for i, n := range names {
			names[i] = regexp.QuoteMeta(n)
		}
		c.names = regexp.MustCompile(`(?i)(?:` + strings.Join(names, "|") + `)`)
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

// selfSpans finds each whole-word mention of the character's names.
// Word boundaries are Unicode-aware (Go's \b is ASCII-only, which would
// miss names like "Zoë") and checked by hand, so a span covers just the
// name and back-to-back mentions are each found.
func (c *Classifier) selfSpans(plain string) []Span {
	if c.names == nil {
		return nil
	}
	var spans []Span
	for _, m := range c.names.FindAllStringIndex(plain, -1) {
		before, _ := utf8.DecodeLastRuneInString(plain[:m[0]])
		after, _ := utf8.DecodeRuneInString(plain[m[1]:])
		if m[0] > 0 && isWordRune(before) || m[1] < len(plain) && isWordRune(after) {
			continue
		}
		spans = append(spans, Span{m[0], m[1]})
	}
	return spans
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}
