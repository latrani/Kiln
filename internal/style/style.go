// Package style turns config.Style values into ANSI SGR sequences.
package style

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/rules"
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

// Highlight draws res over server text: the whole line in res.Style, or
// each styled run in its style. Where a run ends mid-line, the server's
// own SGR state is restored; a server reset inside a run re-applies the
// run's style. text's ANSI-stripped form must be the plain text res was
// computed on. The result ends with Reset.
func Highlight(text string, res rules.Result) string {
	if !res.Styled {
		return text + Reset
	}
	if res.Runs == nil {
		return Apply(text, res.Style)
	}
	var b, server strings.Builder // server: its SGR since its last reset
	runs, ri, pos := res.Runs, 0, 0
	ours := "" // our SGR, while inside a styled run
	for i := 0; i < len(text); {
		if text[i] == 0x1b {
			j := ansi.EscapeEnd(text, i)
			seq := text[i:j]
			b.WriteString(seq)
			if params, ok := sgrParams(seq); ok {
				if first, _, _ := strings.Cut(params, ";"); first == "" || first == "0" {
					server.Reset()
					if params != "" && params != "0" {
						server.WriteString(seq) // e.g. ESC[0;31m: reset, then red
					}
					b.WriteString(ours)
				} else {
					server.WriteString(seq)
				}
			}
			i = j
			continue
		}
		for ri < len(runs) && pos >= runs[ri].End {
			if ours != "" {
				b.WriteString(Reset + server.String())
				ours = ""
			}
			ri++
		}
		if ours == "" && ri < len(runs) && runs[ri].Styled {
			ours = SGR(runs[ri].Style)
			b.WriteString(ours)
		}
		b.WriteByte(text[i])
		i++
		pos++
	}
	return b.String() + Reset
}

// sgrParams returns the parameters of an SGR sequence (ESC [ … m).
func sgrParams(seq string) (string, bool) {
	if len(seq) < 3 || seq[1] != '[' || seq[len(seq)-1] != 'm' {
		return "", false
	}
	return seq[2 : len(seq)-1], true
}
