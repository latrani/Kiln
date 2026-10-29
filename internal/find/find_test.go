package find

import (
	"slices"
	"testing"
)

func TestMatch(t *testing.T) {
	for _, c := range []struct {
		term, text string
		want       bool
	}{
		{"lighthouse", `Rook says, "The lighthouse is dark."`, true},
		{"LIGHT", "the lighthouse", true},                           // a word's start, any case
		{"ight", "the night", false},                                // not inside a word
		{"rook dark", `Rook says, "The lighthouse is dark."`, true}, // every word, any order
		{"rook bright", `Rook says, "The lighthouse is dark."`, false},
		{"don't", "I don’t know", true}, // curly quotes fold
		{`"hi"`, "Ash says, “hi”", true},
		{"a-b", "a—b", true},          // dashes fold
		{"cafe", "Mira's café", true}, // accents fold
		{"café", "Mira's cafe", true},
		{"s", "Mira's cafe", true}, // after the ', a word starts
		{"*", "a*b", true},         // punctuation first: anywhere
		{":)", "hi:)", true},
		{"coyote", "two coyotes", true},
		{"coyote", "a coyotter", false},
		{"", "anything", false},
		{"   ", "anything", false},
	} {
		if got := New(c.term).Match(c.text); got != c.want {
			t.Errorf("New(%q).Match(%q) = %v, want %v", c.term, c.text, got, c.want)
		}
	}
}

func TestSpans(t *testing.T) {
	for _, c := range []struct {
		term, text string
		want       [][2]int
	}{
		{"rook dark", "Rook: dark, Rook", [][2]int{{0, 4}, {6, 10}, {12, 16}}},
		{"li lig", "light", [][2]int{{0, 3}}}, // overlapping hits join
		{"cafe", "a café", [][2]int{{2, 7}}},  // é is two bytes: all of it
		{"don't", "don’t", [][2]int{{0, 7}}},  // ’ is three
		{"rook bright", "Rook", nil},
	} {
		if got := New(c.term).Spans(c.text); !slices.Equal(got, c.want) {
			t.Errorf("New(%q).Spans(%q) = %v, want %v", c.term, c.text, got, c.want)
		}
	}
}
