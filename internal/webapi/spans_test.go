package webapi

import (
	"reflect"
	"strings"
	"testing"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

// fold is how the page draws a span: its server style, each tag's style
// on top in order, a set color replacing, flags adding up.
func fold(sp Span, tags map[string]theme.TagLook) ansi.SpanStyle {
	st := sp.SpanStyle
	for _, n := range sp.Tags {
		l := tags[n].Look
		if l.FG != "" {
			st.FG = l.FG
		}
		if l.BG != "" {
			st.BG = l.BG
		}
		st.Bold, st.Faint, st.Italic = st.Bold || l.Bold, st.Faint || l.Faint, st.Italic || l.Italic
		st.Underline, st.Reverse = st.Underline || l.Underline, st.Reverse || l.Reverse
	}
	return st
}

// perByte is each byte's style, from spans with text.
func perByte[T any](spans []T, text func(T) string, style func(T) ansi.SpanStyle) []ansi.SpanStyle {
	var out []ansi.SpanStyle
	for _, s := range spans {
		for range len(text(s)) {
			out = append(out, style(s))
		}
	}
	return out
}

// TestSpansDrawWhatTheTUIPaints: the page's fold over the spans gives,
// byte for byte, what the TUI paints.
func TestSpansDrawWhatTheTUIPaints(t *testing.T) {
	th, err := theme.FromTOML(`[tags]
page = { fg = "#ff9f43", bold = true }
"page/in" = { italic = true }
self = { scope = "match", bg = "blue", reverse = true }
quiet = { faint = true, fg = "default" }
hot = { scope = "match", fg = "green" }`)
	if err != nil {
		t.Fatal(err)
	}
	hl := rules.New(th)
	_, looks := th.Looks()
	match := func(name string, start, end int) classify.Tag {
		return classify.Tag{Name: name, Spans: []classify.Span{{Start: start, End: end}}}
	}
	for _, c := range []struct {
		name, text string
		tags       []classify.Tag
	}{
		{"plain", "Rook waves.", nil},
		{"server colors only", "\x1b[31mred\x1b[0m plain", nil},
		{"whole line over server color", "\x1b[32mMira\x1b[0m pages: hi", []classify.Tag{{Name: "page/in"}}},
		{"match over part", "Rook says, \"Evening, Kit.\"", []classify.Tag{match("self", 20, 23)}},
		{"reset inside a match", "Kit \x1b[0;31mKit\x1b[0m Kit", []classify.Tag{match("self", 4, 7)}},
		{"reset with color inside a line tag", "a \x1b[0;31mb\x1b[0m c", []classify.Tag{{Name: "page"}}},
		{"line and match overlap", "\x1b[1mKit\x1b[22m pages", []classify.Tag{{Name: "page"}, match("self", 0, 3)}},
		{"a match's color wins over the line's", "Mira pages Kit", []classify.Tag{match("hot", 11, 14), {Name: "page"}}},
		{"default clears nothing", "\x1b[33mquiet\x1b[0m", []classify.Tag{{Name: "quiet"}}},
		{"multibyte before a match", "café ☕ Kit", []classify.Tag{match("self", len("café ☕ "), len("café ☕ Kit"))}},
	} {
		t.Run(c.name, func(t *testing.T) {
			plain := ansi.Strip(c.text)
			painted := style.Highlight(c.text, hl.Runs(plain, c.tags))
			want := perByte(ansi.Spans(painted), func(s ansi.Span) string { return s.Text }, func(s ansi.Span) ansi.SpanStyle { return s.Style })
			got := perByte(spansOf(c.text, hl, c.tags), func(s Span) string { return s.Text }, func(s Span) ansi.SpanStyle { return fold(s, looks) })
			if !reflect.DeepEqual(got, want) {
				t.Errorf("web %v\nTUI %v", got, want)
			}
		})
	}
}

func TestSpansJoinToPlainAndMarkLinks(t *testing.T) {
	text := "See \x1b[4mhttps://kiln.test/map\x1b[0m, café"
	sp := spansOf(text, nil, nil)
	var b strings.Builder
	var link string
	for _, s := range sp {
		b.WriteString(s.Text)
		if s.Link != "" {
			link += s.Text
			if s.Link != "https://kiln.test/map" {
				t.Errorf("span %q links to %q", s.Text, s.Link)
			}
		}
	}
	if b.String() != ansi.Strip(text) {
		t.Errorf("joined %q", b.String())
	}
	if link != "https://kiln.test/map" {
		t.Errorf("link text %q", link)
	}
}
