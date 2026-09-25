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
// s, re-applying s after every server SGR inside text, and ends with Reset.
func Apply(text string, s config.Style) string {
	return Highlight(text, rules.Result{Styled: true, Runs: []rules.Run{
		{Start: 0, End: len(text), Style: s, Styled: true}}})
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
// own SGR state is restored; any server SGR inside a run (a reset, or a
// color of its own) is followed by the run's style again, so ours wins. text's ANSI-stripped form must be the plain text res was
// computed on. The result ends with Reset.
func Highlight(text string, res rules.Result) string {
	if !res.Styled {
		return text + Reset
	}
	if res.Runs == nil {
		return Apply(text, res.Style)
	}
	var b strings.Builder
	server := "" // the server's SGR since its last reset
	runs, ri, pos := res.Runs, 0, 0
	ours := "" // our SGR, while inside a styled run
	for i := 0; i < len(text); {
		if text[i] == 0x1b {
			j := ansi.EscapeEnd(text, i)
			seq := text[i:j]
			b.WriteString(seq)
			if params, ok := sgrParams(seq); ok {
				if tail, reset := afterReset(params); !reset {
					server += seq
				} else if server = ""; tail != "" {
					server = "\x1b[" + tail + "m" // e.g. ESC[0;31m: reset, then red
				}
				b.WriteString(ours)
			}
			i = j
			continue
		}
		for ri < len(runs) && pos >= runs[ri].End {
			if ours != "" {
				b.WriteString(Reset + server)
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

// afterReset reports whether SGR params contain a reset (a parameter that
// is empty or all zeros, like "", "0" or "00", anywhere in the list), and
// returns the parameters after the last one. The arguments of extended
// colors (38;5;n, 38;2;r;g;b and the 48/58 forms) are skipped, so the 0
// in 38;5;0 is black, not a reset.
func afterReset(params string) (tail string, reset bool) {
	ps := strings.Split(params, ";")
	last := -1
	for i := 0; i < len(ps); i++ {
		switch ps[i] {
		case "38", "48", "58":
			if i+1 < len(ps) && ps[i+1] == "5" {
				i += 2
			} else if i+1 < len(ps) && ps[i+1] == "2" {
				i += 4
			}
		default:
			if strings.Trim(ps[i], "0") == "" {
				last = i
			}
		}
	}
	if last < 0 {
		return "", false
	}
	return strings.Join(ps[last+1:], ";"), true
}
