package relay

import "strings"

// Tests build the text they expect from the catalog (package str), so
// rewording a string doesn't break them. These cut a message around a
// placeholder filled with mark.
const mark = "\x00"

// upTo is s up to the first mark.
func upTo(s string) string { h, _, _ := strings.Cut(s, mark); return h }

// errMark is an error that prints as mark, for cutting messages with {err}.
type errMark struct{}

func (errMark) Error() string { return mark }
