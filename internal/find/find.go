// Package find matches log mode's find terms (and the filter panel's
// text rows) against a line: each space-separated word of the term must
// be in the line, in any order, at the start of one of its words, with
// case, accents, curly quotes and dashes folded away. A word that starts
// with anything but a letter or digit (*, :), --) matches anywhere.
package find

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Term is a find term, ready to match.
type Term struct {
	words [][]rune // folded
}

// New makes a Term of term's space-separated words.
func New(term string) Term {
	var t Term
	for _, w := range strings.Fields(term) {
		if f, _ := fold(w); len(f) > 0 {
			t.words = append(t.words, f)
		}
	}
	return t
}

// Empty reports whether t has no words, so matches nothing.
func (t Term) Empty() bool { return len(t.words) == 0 }

// Match reports whether text holds every one of t's words.
func (t Term) Match(text string) bool { return t.In(Fold(text)) }

// Folded is a text folded for matching, to match many terms against.
type Folded []rune

// Fold folds text for In.
func Fold(text string) Folded {
	f, _ := fold(text)
	return f
}

// In reports whether the folded text holds every one of t's words.
func (t Term) In(f Folded) bool {
	if t.Empty() {
		return false
	}
	for _, w := range t.words {
		if len(hits(f, w)) == 0 {
			return false
		}
	}
	return true
}

// Spans is where t's words are in text, as byte ranges of text, in
// order and not overlapping, for highlighting; nil when text doesn't
// match.
func (t Term) Spans(text string) [][2]int {
	if t.Empty() {
		return nil
	}
	f, from := fold(text)
	var runs [][2]int // in f's runes
	for _, w := range t.words {
		hs := hits(f, w)
		if len(hs) == 0 {
			return nil
		}
		for _, h := range hs {
			runs = append(runs, [2]int{h, h + len(w)})
		}
	}
	slices.SortFunc(runs, func(a, b [2]int) int { return a[0] - b[0] })
	var out [][2]int
	for _, r := range runs {
		s, e := from[r[0]][0], from[r[1]-1][1]
		if n := len(out); n > 0 && s <= out[n-1][1] {
			out[n-1][1] = max(out[n-1][1], e)
			continue
		}
		out = append(out, [2]int{s, e})
	}
	return out
}

// hits is where w starts in f, by rune: at a word's start, unless w
// starts with neither a letter nor a digit.
func hits(f, w []rune) []int {
	anywhere := !wordy(w[0])
	var out []int
	for i := 0; i+len(w) <= len(f); i++ {
		if (anywhere || i == 0 || !wordy(f[i-1])) && slices.Equal(f[i:i+len(w)], w) {
			out = append(out, i)
		}
	}
	return out
}

func wordy(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// fold is s with case, accents, curly quotes and dashes folded away,
// and where each of its runes came from in s: from[i] is the byte range
// in s of the rune folded rune i came from.
func fold(s string) (f []rune, from [][2]int) {
	for i, r := range s {
		for _, d := range norm.NFD.String(string(r)) {
			if unicode.Is(unicode.Mn, d) {
				continue // an accent
			}
			f, from = append(f, foldRune(d)), append(from, [2]int{i, i + utf8.RuneLen(r)})
		}
	}
	return f, from
}

// foldRune is r lowercase, a curly quote straight, a dash a hyphen.
func foldRune(r rune) rune {
	switch r {
	case '‘', '’', '‚', '‛', '′', 'ʼ':
		return '\''
	case '“', '”', '„', '‟', '″':
		return '"'
	case '‐', '‑', '‒', '–', '—', '―', '−':
		return '-'
	}
	return unicode.ToLower(r)
}
