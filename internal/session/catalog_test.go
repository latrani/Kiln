package session

import "strings"

// Tests build the text they expect from the catalog (package str), so
// rewording a string doesn't break them. These cut a message around a
// placeholder filled with mark: upTo(str.SessionConnected(mark, mark)) is
// "connected to ".
const mark = "\x00"

// upTo is s up to the first mark.
func upTo(s string) string { h, _, _ := strings.Cut(s, mark); return h }

// after is s after the last mark.
func after(s string) string { return s[strings.LastIndex(s, mark)+1:] }
