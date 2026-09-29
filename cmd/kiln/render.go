package main

import (
	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

// render formats one entry for plain terminal output. Styled lines are
// drawn in their tags' styles; attention lines get a "» " marker, which
// is never styled. Sent lines are dimmed and sys lines are prefixed with
// "* ". Text is sanitized first, so res must be computed on
// ansi.Strip(ansi.Sanitize(…)).
func render(e logstore.Entry, res rules.Result) string {
	e.Text = ansi.Sanitize(e.Text)
	switch e.Dir {
	case logstore.Out:
		return theme.Paint(theme.ScrollbackEcho, "> "+e.Text)
	case logstore.Sys:
		return theme.Paint(theme.ScrollbackSys, "* "+e.Text)
	}
	marker := ""
	if res.Attention {
		marker = "» "
	}
	return marker + style.Highlight(e.Text, res.Runs) // runs index e.Text, not the marker
}
