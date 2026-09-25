package ansi

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
)

// Sanitize keeps only SGR sequences (colors and attributes: ESC [ … m)
// and printable text. Every other escape sequence (window titles, OSC 52
// clipboard writes, cursor movement, hyperlinks) and every C0 control
// except TAB is dropped, so server output can never drive the terminal.
// TABs become four spaces so width math stays simple.
func Sanitize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == 0x1b:
			if i+1 < len(s) && s[i+1] == '[' {
				j := i + 2
				for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
					j++
				}
				if j < len(s) && s[j] == 'm' {
					b.WriteString(s[i : j+1])
				}
				i = j + 1
				continue
			}
			// Not CSI: reuse Strip's handling by skipping one sequence.
			n := len(Strip(s[i:]))
			i = len(s) - n
		case c == '\t':
			b.WriteString("    ")
			i++
		case c < 0x20 || c == 0x7f:
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// Wrap soft-wraps one logical line (which may contain SGR sequences) to
// width cells and returns the rows. Each row after the first is prefixed
// with the SGR state still active from earlier rows, so rows can be drawn
// independently without losing color.
func Wrap(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	rows := strings.Split(xansi.Wrap(s, width, ""), "\n")
	var active strings.Builder
	for i, row := range rows {
		if i > 0 && active.Len() > 0 {
			rows[i] = active.String() + row
		}
		for _, seq := range sgrSequences(row) {
			if seq == "\x1b[0m" || seq == "\x1b[m" {
				active.Reset()
			} else {
				active.WriteString(seq)
			}
		}
	}
	return rows
}

// Width is the number of terminal cells s occupies, ignoring escapes.
func Width(s string) int { return xansi.StringWidth(s) }

func sgrSequences(s string) []string {
	var out []string
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b || i+1 >= len(s) || s[i+1] != '[' {
			continue
		}
		j := i + 2
		for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
			j++
		}
		if j < len(s) && s[j] == 'm' {
			out = append(out, s[i:j+1])
		}
		i = j
	}
	return out
}
