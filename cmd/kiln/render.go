package main

import (
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/style"
)

// render formats one entry for plain terminal output. Highlighted lines
// are wrapped in the rule's style; attention lines get a "» " marker.
// Sent lines are dimmed and sys lines are prefixed with "* ".
func render(e logstore.Entry, res rules.Result) string {
	switch e.Dir {
	case logstore.Out:
		return style.Dim("> " + e.Text)
	case logstore.Sys:
		return style.Dim("* " + e.Text)
	}
	marker := ""
	if res.Attention {
		marker = "» "
	}
	if res.Runs != nil {
		return marker + style.Highlight(e.Text, res) // spans index e.Text, not the marker
	}
	if !res.Styled {
		return marker + e.Text + style.Reset
	}
	return style.Apply(marker+e.Text, res.Style)
}
