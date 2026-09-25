package ansi

import (
	"fmt"
	"strconv"
	"strings"
)

// SpanStyle is the SGR state for a run of text. Colors are "#rrggbb", or
// "" for the terminal default.
type SpanStyle struct {
	FG, BG    string
	Bold      bool
	Italic    bool
	Underline bool
}

// Span is a run of text drawn in one style.
type Span struct {
	Text  string
	Style SpanStyle
}

// Spans splits s into styled runs by interpreting its SGR sequences
// (16-color, 256-color and truecolor; bold, italic, underline). Other
// escape sequences are dropped. Adjacent runs never share a style.
func Spans(s string) []Span {
	var out []Span
	var st SpanStyle
	var text strings.Builder
	flush := func() {
		if text.Len() == 0 {
			return
		}
		if n := len(out); n > 0 && out[n-1].Style == st {
			out[n-1].Text += text.String()
		} else {
			out = append(out, Span{Text: text.String(), Style: st})
		}
		text.Reset()
	}
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			text.WriteByte(s[i])
			i++
			continue
		}
		j := skipEscape(s, i)
		if i+1 < len(s) && s[i+1] == '[' && s[j-1] == 'm' {
			flush()
			st = applySGR(st, s[i+2:j-1])
		}
		i = j
	}
	flush()
	return out
}

func applySGR(st SpanStyle, params string) SpanStyle {
	if params == "" {
		return SpanStyle{}
	}
	ps := strings.Split(strings.ReplaceAll(params, ":", ";"), ";")
	num := func(k int) int {
		if k >= len(ps) {
			return -1
		}
		n, err := strconv.Atoi(ps[k])
		if err != nil {
			return -1
		}
		return n
	}
	for k := 0; k < len(ps); k++ {
		switch n := num(k); {
		case n == 0 || ps[k] == "":
			st = SpanStyle{}
		case n == 1:
			st.Bold = true
		case n == 22:
			st.Bold = false
		case n == 3:
			st.Italic = true
		case n == 23:
			st.Italic = false
		case n == 4:
			st.Underline = true
		case n == 24:
			st.Underline = false
		case n >= 30 && n <= 37:
			st.FG = palette16[n-30]
		case n >= 90 && n <= 97:
			st.FG = palette16[n-90+8]
		case n == 39:
			st.FG = ""
		case n >= 40 && n <= 47:
			st.BG = palette16[n-40]
		case n >= 100 && n <= 107:
			st.BG = palette16[n-100+8]
		case n == 49:
			st.BG = ""
		case n == 38 || n == 48:
			var c string
			switch num(k + 1) {
			case 5:
				c = color256(num(k + 2))
				k += 2
			case 2:
				r, g, b := num(k+2), num(k+3), num(k+4)
				if r >= 0 && g >= 0 && b >= 0 {
					c = fmt.Sprintf("#%02x%02x%02x", r&255, g&255, b&255)
				}
				k += 4
			}
			if n == 38 {
				st.FG = c
			} else {
				st.BG = c
			}
		}
	}
	return st
}

// palette16 is the xterm default 16-color palette.
var palette16 = [16]string{
	"#000000", "#cd0000", "#00cd00", "#cdcd00", "#0000ee", "#cd00cd", "#00cdcd", "#e5e5e5",
	"#7f7f7f", "#ff0000", "#00ff00", "#ffff00", "#5c5cff", "#ff00ff", "#00ffff", "#ffffff",
}

// color256 maps an xterm 256-color index to "#rrggbb" ("" if invalid).
func color256(n int) string {
	switch {
	case n < 0 || n > 255:
		return ""
	case n < 16:
		return palette16[n]
	case n < 232:
		n -= 16
		level := func(v int) int {
			if v == 0 {
				return 0
			}
			return 55 + v*40
		}
		return fmt.Sprintf("#%02x%02x%02x", level(n/36), level(n/6%6), level(n%6))
	default:
		v := 8 + (n-232)*10
		return fmt.Sprintf("#%02x%02x%02x", v, v, v)
	}
}
