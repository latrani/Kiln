package classify

import (
	"reflect"
	"testing"

	"github.com/latrani/Kiln/internal/config"
)

func TestClassify(t *testing.T) {
	c, err := New([]config.ClassifyRule{
		{Tag: "page", Pattern: `^\S+ pages: `},
		{Tag: "page", Pattern: `^In a page-pose`},
		{Tag: "ooc", Pattern: `OOC`},
	}, "Kit", []string{"Kitty"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		in   string
		want []string
	}{
		{"Rook says, \"Evening!\"", nil},
		{"Mira pages: you around?", []string{"page"}},
		{"In a page-pose to you, Mira grins.", []string{"page"}},
		{"Mira pages: OOC brb", []string{"page", "ooc"}},
		{"Rook waves to Kit.", []string{SelfTag}},
		{"Rook waves to kitty!", []string{SelfTag}},
		{"Mira pages: hi Kit", []string{"page", SelfTag}},
		{"Kit", []string{SelfTag}},
		{"Kitten wanders by.", nil},       // not a whole word
		{"The skit was funny.", nil},      // not a whole word
		{"Kitë is a different name", nil}, // non-ASCII letter continues the word
	}
	for _, tc := range cases {
		if got := c.Classify(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Classify(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSelfNamesAreLiteralAndUnicode(t *testing.T) {
	c, err := New(nil, "K.i.t", []string{"Zoë", "(Ash)"})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{
		"hi K.i.t!":        true,
		"hi Kxixt":         false, // dots are literal, not wildcards
		"Zoë waves.":       true,
		"ZOË waves.":       true,
		"hello (Ash) here": true,
	}
	for in, want := range cases {
		got := len(c.Classify(in)) == 1
		if got != want {
			t.Errorf("self match %q = %v, want %v", in, got, want)
		}
	}
}

func TestNoNamesNoSelf(t *testing.T) {
	c, _ := New(nil, "", nil)
	if got := c.Classify("anything"); got != nil {
		t.Errorf("got %v", got)
	}
}

func TestTagsSpans(t *testing.T) {
	c, err := New([]config.ClassifyRule{
		{Tag: "page", Pattern: `^PAGE:`},
		{Tag: "page", Pattern: `pages: `},
		{Tag: "ooc", Pattern: `OOC`},
		{Tag: "start", Pattern: `^`},
	}, "Kit", []string{"Kitty"})
	if err != nil {
		t.Fatal(err)
	}
	// PAGE: 0-5, "pages: " 11-18, OOC 18-21, Kit 22-25, Kit 27-30, kitty 32-37
	got := c.Tags("PAGE: Mira pages: OOC Kit, Kit! kitty")
	want := []Tag{
		{Name: "page", Spans: []Span{{0, 5}, {11, 18}}},
		{Name: "ooc", Spans: []Span{{18, 21}}},
		{Name: "start"}, // zero-width: tagged, but no spans
		{Name: SelfTag, Spans: []Span{{22, 25}, {27, 30}, {32, 37}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Tags = %+v\nwant   %+v", got, want)
	}
}

func TestSelfSpansCoverOnlyTheName(t *testing.T) {
	cases := []struct {
		name    string
		aliases []string
		in      string
		want    []Span
	}{
		{"Kit", nil, "KitKit Kit", []Span{{7, 10}}},                     // not a whole word, then a whole word
		{"Kit", nil, "Kitë Kit", []Span{{6, 9}}},                        // ë continues the word
		{"Zoë", nil, "hi ZOË!", []Span{{3, 7}}},                         // multi-byte, any case
		{"Kit", []string{"(Ash)"}, "hello (Ash) here", []Span{{6, 11}}}, // literal regex specials
		{"Kit", nil, "Rook waves.", nil},
	}
	for _, tc := range cases {
		c, err := New(nil, tc.name, tc.aliases)
		if err != nil {
			t.Fatal(err)
		}
		var got []Span
		for _, tag := range c.Tags(tc.in) {
			if tag.Name == SelfTag {
				got = tag.Spans
			}
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: self spans = %v, want %v", tc.in, got, tc.want)
		}
	}
}
