package main

import (
	"fmt"
	"strconv"
	"strings"

	"kiln/internal/config"
	"kiln/internal/logstore"
	"kiln/internal/rules"
)

// render formats one entry for plain terminal output. Highlighted lines
// are wrapped in the rule's style; attention lines get a "» " marker.
// Sent lines are dimmed and sys lines are prefixed with "* ".
func render(e logstore.Entry, res rules.Result) string {
	switch e.Dir {
	case logstore.Out:
		return "\x1b[2m> " + e.Text + "\x1b[0m"
	case logstore.Sys:
		return "\x1b[2m* " + e.Text + "\x1b[0m"
	}
	marker := ""
	if res.Attention {
		marker = "» "
	}
	if !res.Styled {
		return marker + e.Text + "\x1b[0m"
	}
	sgr := sgrFor(res.Style)
	// Re-apply our style after any reset inside the server's own ANSI.
	text := strings.ReplaceAll(e.Text, "\x1b[0m", "\x1b[0m"+sgr)
	text = strings.ReplaceAll(text, "\x1b[m", "\x1b[m"+sgr)
	return sgr + marker + text + "\x1b[0m"
}

func sgrFor(s config.Style) string {
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
