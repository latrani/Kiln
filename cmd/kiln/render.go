package main

import (
	"fmt"
	"io"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

// render formats one entry for plain terminal output. Styled lines are
// drawn in their tags' styles; priority lines get a "» " marker, which
// is never styled. Sent lines are prefixed with "> " (their runs cover
// it; see sentRuns) and sys lines with "* ". Text is sanitized first, so
// runs must be computed on ansi.Strip(ansi.Sanitize(…)).
func render(e logstore.Entry, runs []style.Run, priority bool) string {
	e.Text = ansi.Sanitize(e.Text)
	switch e.Dir {
	case logstore.Out:
		return style.Highlight("> "+e.Text, runs)
	case logstore.Sys:
		return theme.Paint(theme.ScrollbackSys, "* "+e.Text)
	}
	marker := ""
	if priority {
		marker = "» "
	}
	return marker + style.Highlight(e.Text, runs) // runs index e.Text, not the marker
}

// sentRuns are sent entry e's runs as render draws it: its one tag,
// classify.SentTag, over the "> " and the text.
func sentRuns(hl *rules.Highlighter, e logstore.Entry) []style.Run {
	return hl.Runs(ansi.Strip("> "+ansi.Sanitize(e.Text)), []classify.Tag{{Name: classify.SentTag}})
}

// lookTheme is the active theme with ch's packs' looks under it and its
// own on top. Looks that don't resolve are reported to w, and the most
// that does stands in.
func lookTheme(ch config.Character, w io.Writer) *theme.Theme {
	th, err := theme.Active().Best(ch.PackLooks, ch.Looks)
	if err != nil {
		fmt.Fprintln(w, theme.Paint(theme.StatusError, "* "+str.StatusCharError(ch.World+"/"+ch.ID, err)))
	}
	return th
}
