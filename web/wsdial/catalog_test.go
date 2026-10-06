package wsdial

import "strings"

// Tests build expected text from the catalog; mark fills a placeholder.
const mark = "\x00"

// upTo is s up to the first mark.
func upTo(s string) string { h, _, _ := strings.Cut(s, mark); return h }
