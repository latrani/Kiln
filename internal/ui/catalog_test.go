package ui

import (
	"strings"

	"github.com/latrani/Kiln/internal/str"
)

// Tests build the text they expect from the catalog (package str), so
// rewording a string doesn't break them. These cut a message around a
// placeholder filled with mark: upTo(str.SessionConnected(mark, mark)) is
// "connected to ".
const mark = "\x00"

// upTo is s up to the first mark.
func upTo(s string) string { h, _, _ := strings.Cut(s, mark); return h }

// after is s after the last mark.
func after(s string) string { return s[strings.LastIndex(s, mark)+1:] }

// firstPart is s up to its first separator: the first of a list of hints.
func firstPart(s string) string { h, _, _ := strings.Cut(s, str.Separator()); return h }
