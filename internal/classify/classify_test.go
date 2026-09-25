package classify

import (
	"reflect"
	"testing"

	"kiln/internal/config"
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
