package ui

import (
	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/glyph"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

// dayRule is drawn either side of a day's label.
const dayRule = glyph.Rule + glyph.Rule

// paint draws l for the terminal: a server, sent or Kiln's own line in
// its tags' styles from hl, over the server's own colors; dividers in
// their roles.
// It only reads hl, so it's safe off the UI goroutine.
func paint(hl *rules.Highlighter, l app.Line) string {
	switch l.Kind {
	case app.Echo: // its tag styles the gutter mark too
		text := gutterMark + l.Text()
		return style.Highlight(text, hl.Runs(ansi.Strip(text), l.Tags))
	case app.Sys: // its tag styles the glyph too
		text := glyph.Sys + " " + l.Text()
		return style.Highlight(text, hl.Runs(ansi.Strip(text), l.Tags))
	case app.Day:
		return theme.Paint(theme.ScrollbackDay, dayRule+" "+app.DayLabel(l.Day)+" "+dayRule)
	case app.HistoryEnd:
		return theme.Paint(theme.ScrollbackHistoryEnd, str.ScrollbackHistoryEnds(l.Entry.Time.Format(str.DateDayTime())))
	}
	text := l.Text()
	return style.Highlight(text, hl.Runs(ansi.Strip(text), l.Tags))
}

// plainShown is l as paint draws it, without the styling: what find
// matches in log mode.
func plainShown(l app.Line) string {
	switch l.Kind {
	case app.Echo:
		return gutterMark + l.Plain()
	case app.Sys:
		return glyph.Sys + " " + l.Plain()
	}
	return l.Plain()
}
