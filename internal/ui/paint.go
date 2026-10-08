package ui

import (
	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

// paint draws l for the terminal: a server line in its tags' styles
// from hl, over the server's own colors; Kiln's lines in their roles.
// It only reads hl, so it's safe off the UI goroutine.
func paint(hl *rules.Highlighter, l app.Line) string {
	switch l.Kind {
	case app.Echo:
		return theme.Paint(theme.ScrollbackEcho, gutterMark+l.Text)
	case app.Sys:
		return theme.Paint(theme.ScrollbackSys, "* "+l.Text)
	case app.Day:
		return theme.Paint(theme.ScrollbackDay, "── "+dayLabel(l.Day)+" ──")
	}
	return style.Highlight(l.Text, hl.Runs(l.Plain, l.Tags))
}
