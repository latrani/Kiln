package ui

import (
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

// paint draws each kind of line the way renderLine and renderDays did.
func TestPaintKinds(t *testing.T) {
	cls, err := classify.New([]config.ClassifyRule{{Tag: "page", Pattern: `^PAGE:`}}, "Kit", nil)
	if err != nil {
		t.Fatal(err)
	}
	th, err := theme.FromTOML("[tags]\npage = { fg = \"#2053ff\", bold = true, scope = \"match\" }\n")
	if err != nil {
		t.Fatal(err)
	}
	r, hl := app.Rules{Classifier: cls}, rules.New(th)
	server := r.Line(logstore.Entry{Dir: logstore.In, Text: "PAGE: hi"})
	if got, want := paint(hl, server), style.Highlight(server.Text(), hl.Runs(server.Plain(), server.Tags)); got != want {
		t.Errorf("server line = %q, want %q", got, want)
	}
	echo := r.Line(logstore.Entry{Dir: logstore.Out, Text: "hi"})
	if got, want := paint(hl, echo), theme.Paint(theme.ScrollbackEcho, gutterMark+"hi"); got != want {
		t.Errorf("echo line = %q, want %q", got, want)
	}
	sys := r.Line(logstore.Entry{Dir: logstore.Sys, Text: "connected"})
	if got, want := paint(hl, sys), theme.Paint(theme.ScrollbackSys, "* connected"); got != want {
		t.Errorf("sys line = %q, want %q", got, want)
	}
	day := app.Line{Kind: app.Day, Day: app.DayOf(time.Date(2026, 9, 24, 12, 0, 0, 0, time.Local))}
	if got, want := paint(hl, day), theme.Paint(theme.ScrollbackDay, "── "+dayLabel(day.Day)+" ──"); got != want {
		t.Errorf("day divider = %q, want %q", got, want)
	}
	at := time.Date(2026, 9, 24, 21, 0, 0, 0, time.Local)
	end := app.Line{Kind: app.HistoryEnd, Entry: logstore.Entry{Time: at}}
	if got, want := paint(hl, end), theme.Paint(theme.ScrollbackHistoryEnd, str.ScrollbackHistoryEnds(at.Format(str.DateDayTime()))); got != want {
		t.Errorf("history end = %q, want %q", got, want)
	}
}
