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
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			i++
			continue
		}
		if i+1 >= len(s) {
			break
		}
		switch s[i+1] {
		case '[': // CSI: parameters/intermediates, then a final byte 0x40–0x7E
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j + 1
		case ']': // OSC: terminated by BEL or ESC \
			j := i + 2
			for j < len(s) {
				if s[j] == 0x07 {
					j++
					break
				}
				if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
					j += 2
					break
				}
				j++
			}
			i = j
		default: // ESC, optional intermediates 0x20–0x2F (e.g. "(" in ESC ( B), final byte
			j := i + 1
			for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
				j++
			}
			i = j + 1
		}
	}
	return b.String()
}
