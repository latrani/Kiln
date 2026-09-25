// Package classify tags lines with semantic kinds (page, whisper, …).
package classify

import (
	"regexp"
	"strings"

	"github.com/latrani/Kiln/internal/config"
)

// SelfTag is added to any line that mentions the character's own name
// or one of its aliases.
const SelfTag = "self"

// Classifier holds one character's compiled classify rules.
type Classifier struct {
	rules []rule
	self  *regexp.Regexp // nil if no names
}

type rule struct {
	tag string
	re  *regexp.Regexp
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
			names = append(names, regexp.QuoteMeta(n))
		}
	}
	if len(names) > 0 {
		// Unicode-aware word boundaries: Go's \b is ASCII-only, which
		// would miss names like "Zoë".
		c.self = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}_])(?:` + strings.Join(names, "|") + `)(?:$|[^\p{L}\p{N}_])`)
	}
	return c, nil
}

// Classify returns the tags for plain (ANSI-stripped) text, in rule order,
// without duplicates, with SelfTag last.
func (c *Classifier) Classify(plain string) []string {
	var tags []string
	seen := map[string]bool{}
	for _, r := range c.rules {
		if !seen[r.tag] && r.re.MatchString(plain) {
			seen[r.tag] = true
			tags = append(tags, r.tag)
		}
	}
	if c.self != nil && !seen[SelfTag] && c.self.MatchString(plain) {
		tags = append(tags, SelfTag)
	}
	return tags
}
