// Package style draws styled runs over server text, keeping the server's
// own SGR.
package style

import (
	"strings"

	"github.com/latrani/Kiln/internal/ansi"
)

// Reset clears all SGR attributes.
const Reset = "\x1b[0m"

// Run is a stretch [Start, End) of a line's plain text, drawn in SGR, or
// as the server sent it when SGR is "".
type Run struct {
	Start, End int
	SGR        string
}

// Apply wraps text (which may contain the server's own SGR sequences) in
// sgr, re-applying it after every server SGR inside text, and ends with
// Reset.
func Apply(text, sgr string) string {
	if sgr == "" {
		return text + Reset
	}
	return Highlight(text, []Run{{Start: 0, End: len(text), SGR: sgr}})
}

// Highlight draws runs over server text. Where a run ends mid-line, the
// server's own SGR state is restored; any server SGR inside a run (a
// reset, or a color of its own) is followed by the run's style again, so
// ours wins. text's ANSI-stripped form must be the plain text the runs
// index. The result ends with Reset.
func Highlight(text string, runs []Run) string {
	if len(runs) == 0 {
		return text + Reset
	}
	var b strings.Builder
	server := "" // the server's SGR since its last reset
	ri, pos := 0, 0
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
		if ours == "" && ri < len(runs) && runs[ri].SGR != "" {
			ours = runs[ri].SGR
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

// Reassert writes base again after every SGR sequence in s that resets,
// so a painted area keeps its colors across the resets of what's drawn
// inside it.
func Reassert(s, base string) string {
	if base == "" || !strings.Contains(s, "\x1b[") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			i++
			continue
		}
		j := ansi.EscapeEnd(s, i)
		b.WriteString(s[i:j])
		if params, ok := sgrParams(s[i:j]); ok {
			if _, reset := afterReset(params); reset {
				b.WriteString(base)
			}
		}
		i = j
	}
	return b.String()
}
