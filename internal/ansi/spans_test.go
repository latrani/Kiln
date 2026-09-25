package ansi

import (
	"reflect"
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
		{"16 color", "\x1b[31mred\x1b[39m plain", []Span{{"red", SpanStyle{FG: "#cd0000"}}, {" plain", SpanStyle{}}}},
		{"bright bg", "\x1b[101mx", []Span{{"x", SpanStyle{BG: "#ff0000"}}}},
		{"256 color", "\x1b[38;5;208mx", []Span{{"x", SpanStyle{FG: "#ff8700"}}}},
		{"256 gray", "\x1b[48;5;244mx", []Span{{"x", SpanStyle{BG: "#808080"}}}},
		{"truecolor", "\x1b[38;2;255;159;67mx", []Span{{"x", SpanStyle{FG: "#ff9f43"}}}},
		{"combined params", "\x1b[1;3;4;32mx\x1b[22;23;24mY", []Span{{"x", SpanStyle{FG: "#00cd00", Bold: true, Italic: true, Underline: true}}, {"Y", SpanStyle{FG: "#00cd00"}}}},
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
