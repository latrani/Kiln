// Package app is what Kiln decides, apart from how any front end draws
// it. See docs/superpowers/specs/2026-10-07-headless-core-design.md.
package app

import (
	"time"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/rules"
)

// Kind is what a line is.
type Kind int

const (
	Server     Kind = iota // a line from the server
	Echo                   // a line you sent
	Sys                    // Kiln's own note in the log (connected, closed, …)
	Day                    // a divider before the first line of a day
	HistoryEnd             // where preloaded history ends; Entry.Time is its newest line's
)

// Line is a log entry and what Kiln made of it. Nothing in it depends on
// the theme: each front end paints it its own way.
type Line struct {
	Kind  Kind
	Entry logstore.Entry // zero for a Day
	Day   string         // the local day, "2006-01-02"
	Tags  []classify.Tag // a Server line's tags, with where they matched
	rules.Verdict
}

// TagNames is the line's tag names, in order.
func (l Line) TagNames() []string {
	var names []string
	for _, t := range l.Tags {
		names = append(names, t.Name)
	}
	return names
}

// Text is the entry's text sanitized, the server's SGR kept.
func (l Line) Text() string { return ansi.Sanitize(l.Entry.Text) }

// Plain is Text without SGR: what the tags' spans index.
func (l Line) Plain() string { return ansi.Strip(l.Text()) }

// DayOf is the local day t falls on, as Line.Day has it.
func DayOf(t time.Time) string { return t.Local().Format("2006-01-02") }

// Rules are one character's compiled rules: how its lines are tagged and
// what they ask for. They're never changed after they're made, so they're
// safe to use off the front end's loop.
type Rules struct {
	Classifier *classify.Classifier
	Judge      rules.Judge
}

// Line makes e into a line. Only lines from the server are classified.
func (r Rules) Line(e logstore.Entry) Line {
	l := Line{Entry: e, Day: DayOf(e.Time)}
	switch e.Dir {
	case logstore.Out:
		l.Kind = Echo
	case logstore.Sys:
		l.Kind = Sys
	default:
		l.Kind = Server
		l.Tags = r.Classifier.Tags(l.Plain())
		l.Verdict = r.Judge.Of(l.Tags)
	}
	return l
}

// Days makes entries into lines with a Day divider before the first line
// of each day. The very first entry gets one only if startsDay, i.e. it
// really is the first line of its day. Sent lines are left out unless
// echo (the character's local_echo) is on.
func (r Rules) Days(entries []logstore.Entry, echo, startsDay bool) []Line {
	out := make([]Line, 0, len(entries)+2)
	prev := ""
	for i, e := range entries {
		if e.Dir == logstore.Out && !echo {
			continue
		}
		day := DayOf(e.Time)
		if day != prev && (i > 0 || startsDay) {
			out = append(out, Line{Kind: Day, Day: day})
		}
		prev = day
		out = append(out, r.Line(e))
	}
	return out
}

// Reline is l made again from its entry under r, for when a character's
// rules change. Kiln's own lines (dividers, the end of history) have no
// entry to remake and come back as they were.
func (r Rules) Reline(l Line) Line {
	if l.Kind == Day || l.Kind == HistoryEnd {
		return l
	}
	return r.Line(l.Entry)
}
