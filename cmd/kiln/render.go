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
// is never styled. Sent lines are prefixed with "> " and sys lines with
// "* ", their runs covering it (see kilnRuns). Text is sanitized first, so
// runs must be computed on ansi.Strip(ansi.Sanitize(…)).
func render(e logstore.Entry, runs []style.Run, priority bool) string {
	e.Text = ansi.Sanitize(e.Text)
	switch e.Dir {
	case logstore.Out:
		return style.Highlight("> "+e.Text, runs)
	case logstore.Sys:
		return style.Highlight("* "+e.Text, runs)
	}
	marker := ""
	if priority {
		marker = "» "
	}
	return marker + style.Highlight(e.Text, runs) // runs index e.Text, not the marker
}

// kilnRuns are the runs render draws a sent or sys entry e with: its one
// tag, classify.SentTag or classify.KilnTag, over the "> " or "* " and
// the text.
func kilnRuns(hl *rules.Highlighter, e logstore.Entry) []style.Run {
	prefix, tag := "> ", classify.SentTag
	if e.Dir == logstore.Sys {
		prefix, tag = "* ", classify.KilnTag
	}
	return hl.Runs(ansi.Strip(prefix+ansi.Sanitize(e.Text)), []classify.Tag{{Name: tag}})
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
