// Package style turns config.Style values into ANSI SGR sequences.
package style

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/latrani/Kiln/internal/config"
)

// Reset clears all SGR attributes.
const Reset = "\x1b[0m"

// SGR returns the escape sequence for s, or "" if s sets nothing.
// Colors must be "#rrggbb"; anything else is ignored.
func SGR(s config.Style) string {
	var codes []string
	if s.Bold {
		codes = append(codes, "1")
	}
	if s.Italic {
		codes = append(codes, "3")
	}
	if s.Underline {
		codes = append(codes, "4")
	}
	if r, g, b, ok := hexRGB(s.FG); ok {
		codes = append(codes, fmt.Sprintf("38;2;%d;%d;%d", r, g, b))
	}
	if r, g, b, ok := hexRGB(s.BG); ok {
		codes = append(codes, fmt.Sprintf("48;2;%d;%d;%d", r, g, b))
	}
	if len(codes) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

// Apply wraps text (which may contain the server's own SGR sequences) in
// s, re-applying s after every reset inside text, and ends with Reset.
func Apply(text string, s config.Style) string {
	sgr := SGR(s)
	if sgr == "" {
		return text + Reset
	}
	text = strings.ReplaceAll(text, "\x1b[0m", "\x1b[0m"+sgr)
	text = strings.ReplaceAll(text, "\x1b[m", "\x1b[m"+sgr)
	return sgr + text + Reset
}

// Dim renders text faint, for client-generated and sent lines.
func Dim(text string) string { return "\x1b[2m" + text + Reset }

// hexRGB parses "#rrggbb".
func hexRGB(h string) (r, g, b uint8, ok bool) {
	if len(h) != 7 || h[0] != '#' {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(h[1:], 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return uint8(v >> 16), uint8(v >> 8), uint8(v), true
}
