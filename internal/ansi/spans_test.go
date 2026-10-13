package ansi

import (
	"reflect"
	"strings"
	"testing"
)

func TestSpans(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []Span
	}{
		{"plain", "hi", []Span{{"hi", SpanStyle{}}}},
		{"empty", "", nil},
		{"bold then reset", "\x1b[1mRook\x1b[0m says", []Span{{"Rook", SpanStyle{Bold: true}}, {" says", SpanStyle{}}}},
		{"16 color", "\x1b[31mred\x1b[39m plain", []Span{{"red", SpanStyle{FG: "ansi:1"}}, {" plain", SpanStyle{}}}},
		{"bright bg", "\x1b[101mx", []Span{{"x", SpanStyle{BG: "ansi:9"}}}},
		{"256 below 16 is the sixteen", "\x1b[38;5;3mx", []Span{{"x", SpanStyle{FG: "ansi:3"}}}},
		{"256 color", "\x1b[38;5;208mx", []Span{{"x", SpanStyle{FG: "#ff8700"}}}},
		{"256 gray", "\x1b[48;5;244mx", []Span{{"x", SpanStyle{BG: "#808080"}}}},
		{"truecolor", "\x1b[38;2;255;159;67mx", []Span{{"x", SpanStyle{FG: "#ff9f43"}}}},
		{"combined params", "\x1b[1;3;4;32mx\x1b[22;23;24mY", []Span{{"x", SpanStyle{FG: "ansi:2", Bold: true, Italic: true, Underline: true}}, {"Y", SpanStyle{FG: "ansi:2"}}}},
		{"faint and reverse", "\x1b[2;7mx\x1b[27my\x1b[22mz", []Span{{"x", SpanStyle{Faint: true, Reverse: true}}, {"y", SpanStyle{Faint: true}}, {"z", SpanStyle{}}}},
		{"22 ends bold and faint", "\x1b[1;2mx\x1b[22my", []Span{{"x", SpanStyle{Bold: true, Faint: true}}, {"y", SpanStyle{}}}},
		{"empty sgr resets", "\x1b[1mA\x1b[mB", []Span{{"A", SpanStyle{Bold: true}}, {"B", SpanStyle{}}}},
		{"merges equal styles", "a\x1b[0mb", []Span{{"ab", SpanStyle{}}}},
		{"drops non-sgr", "a\x1b]0;t\x07b\x1b[2Jc", []Span{{"abc", SpanStyle{}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Spans(c.in); !reflect.DeepEqual(got, c.want) {
				t.Errorf("Spans(%q) = %+v, want %+v", c.in, got, c.want)
			}
		})
	}
}

func TestHex(t *testing.T) {
	for in, want := range map[string]string{"ansi:1": "#cd0000", "ansi:15": "#ffffff", "#ff9f43": "#ff9f43", "": ""} {
		if got := Hex(in); got != want {
			t.Errorf("Hex(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSpansJoinToStrip: tags index Strip's text, so Spans must cover
// exactly the same bytes, whatever the escapes.
func TestSpansJoinToStrip(t *testing.T) {
	for _, s := range []string{"a\x1b[31mb\x1b[0mc", "a\x1b]0;t\x07b\x1b[2Jc", "x\x1b[", "\x1b[38;5mx", "caf\u00e9 \x1b[1m\u2615\x1b[0m Kit", "a\x1bPdcs\x1b\\b"} {
		var b strings.Builder
		for _, sp := range Spans(s) {
			b.WriteString(sp.Text)
		}
		if b.String() != Strip(s) {
			t.Errorf("Spans(%q) joins to %q, Strip gives %q", s, b.String(), Strip(s))
		}
	}
}
