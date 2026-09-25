// Package ansi handles ANSI escape sequences in server output.
package ansi

import "strings"

// Strip removes ANSI escape sequences, leaving the visible text. It
// handles CSI (ESC [ … final), OSC (ESC ] … BEL or ESC \), and short ESC
// sequences with optional intermediates (ESC ( B). An unterminated
// sequence at the end is dropped.
func Strip(s string) string {
	if strings.IndexByte(s, 0x1b) < 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			i = skipEscape(s, i)
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// skipEscape returns the index just past the one escape sequence that
// starts at s[i] (which must be ESC): CSI (ESC [ … final), OSC (ESC ] …
// BEL or ESC \), or ESC with optional intermediates and a final byte.
// An unterminated sequence runs to the end of s.
func skipEscape(s string, i int) int {
	if i+1 >= len(s) {
		return len(s)
	}
	switch s[i+1] {
	case '[':
		j := i + 2
		for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
			j++
		}
		return min(j+1, len(s))
	case ']':
		for j := i + 2; j < len(s); j++ {
			if s[j] == 0x07 {
				return j + 1
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				return j + 2
			}
		}
		return len(s)
	default:
		j := i + 1
		for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
			j++
		}
		return min(j+1, len(s))
	}
}

// EscapeEnd returns the index just past the escape sequence that starts
// at s[i] (which must be ESC), exactly as Strip skips it.
func EscapeEnd(s string, i int) int { return skipEscape(s, i) }
