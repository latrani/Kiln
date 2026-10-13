package app

import (
	"regexp"
	"strings"
)

// urlRE finds web links. Trailing punctuation is trimmed by trimURL.
var urlRE = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `]+`) //str:ok

// trimURL drops sentence punctuation after a link, and a closing bracket
// that doesn't close one inside it.
func trimURL(u string) string {
	for u != "" {
		switch c := u[len(u)-1]; {
		case strings.IndexByte(".,;:!?'\"", c) >= 0:
		case c == ')' && strings.Count(u, "(") < strings.Count(u, ")"):
		case c == ']' && strings.Count(u, "[") < strings.Count(u, "]"):
		default:
			return u
		}
		u = u[:len(u)-1]
	}
	return u
}

// Links is where plain's web links are, as byte ranges, each without
// trailing sentence punctuation or a closing bracket it doesn't open.
func Links(plain string) [][2]int {
	var out [][2]int
	for _, m := range urlRE.FindAllStringIndex(plain, -1) {
		out = append(out, [2]int{m[0], m[0] + len(trimURL(plain[m[0]:m[1]]))})
	}
	return out
}
