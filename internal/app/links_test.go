package app

import (
	"slices"
	"testing"
)

func TestLinks(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"See https://kiln.test/map for the way", []string{"https://kiln.test/map"}},
		{"(https://x.test/a_(b)) and http://y.test.", []string{"https://x.test/a_(b)", "http://y.test"}},
		{"café https://x.test/☕?q=1!", []string{"https://x.test/☕?q=1"}},
		{"no links here", nil},
	} {
		var got []string
		for _, r := range Links(c.in) {
			got = append(got, c.in[r[0]:r[1]])
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("Links(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTrimURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://x.test/a.":           "https://x.test/a",
		"https://x.test/a),":          "https://x.test/a",
		"https://en.wiki/Foo_(bar)":   "https://en.wiki/Foo_(bar)",
		"https://en.wiki/Foo_(bar)).": "https://en.wiki/Foo_(bar)",
		"http://x.test/?q=1&r=2!":     "http://x.test/?q=1&r=2",
		"https://x.test/[1]]":         "https://x.test/[1]",
	} {
		if got := trimURL(in); got != want {
			t.Errorf("trimURL(%q) = %q, want %q", in, got, want)
		}
	}
}
