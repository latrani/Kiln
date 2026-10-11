package ui

import (
	"slices"

	"github.com/latrani/Kiln/internal/app"
)

// Log mode's cursor is the terminal's: the line keys move, find steps
// from, m marks and t describes. These methods shadow app.Log's, passing
// the cursor where the core takes a line.

// SetCursor puts the cursor on l: a click, or the view dragging it along.
func (b *browse) SetCursor(l *app.LogLine) { b.Cursor = l }

// MoveCursor moves by delta shown lines. Moving up past the oldest
// loaded line stops there and pages in older history; the rest of the
// move happens when it arrives.
func (b *browse) MoveCursor(delta int) []app.Effect {
	b.StopSearch() // the wheel, say: you've moved on
	v := b.Visible()
	if len(v) == 0 {
		if delta < 0 { // everything loaded is hidden; look further back
			return b.Older(func() []app.Effect { return b.MoveCursor(delta) })
		}
		return nil
	}
	i := slices.Index(v, b.Cursor)
	if i < 0 {
		i = len(v) - 1
	}
	b.Cursor = v[min(max(0, i+delta), len(v)-1)]
	if rest := i + delta; rest < 0 {
		return b.Older(func() []app.Effect { return b.MoveCursor(rest) })
	}
	return nil
}

// ToTop pages in all history, then moves to the oldest shown line.
func (b *browse) ToTop() []app.Effect {
	if v := b.Visible(); len(v) > 0 {
		b.Cursor = v[0]
	}
	return b.Older(b.ToTop)
}

// ToBottom moves to the newest shown line.
func (b *browse) ToBottom() { b.Cursor = b.LastVisible() }

// Mark marks the cursor's line; see app.Log.Mark.
func (b *browse) Mark() { b.Log.Mark(b.Cursor) }

// Refilter syncs the filter and keeps the reader's place: a cursor on a
// line now hidden moves to the nearest shown one.
func (b *browse) Refilter() {
	b.Log.Refilter()
	if b.Cursor != nil && !b.Shows(b.Cursor) {
		if n := b.NearestVisible(b.Cursor); n != nil {
			b.Cursor = n
		}
	}
}

// found moves the cursor to a find's match.
func (b *browse) found(l *app.LogLine) { b.Cursor = l }

// SetFind sets the find term and goes to its nearest match at the cursor
// or older.
func (b *browse) SetFind(term string) []app.Effect { return b.Log.SetFind(term, b.Cursor, b.found) }

// FindOlder goes to the nearest match older than the cursor (or at it,
// with includeCursor).
func (b *browse) FindOlder(includeCursor bool) []app.Effect {
	return b.Log.FindOlder(b.Cursor, includeCursor, b.found)
}

// FindNewer goes to the nearest match newer than the cursor.
func (b *browse) FindNewer() {
	if l := b.Log.FindNewer(b.Cursor); l != nil {
		b.Cursor = l
	}
}

// FindStatus is the find term and the cursor's match, for the top bar.
func (b *browse) FindStatus() string { return b.Log.FindStatus(b.Cursor) }

// GotoDate goes to day's first shown line and puts it at the top of the
// body.
func (b *browse) GotoDate(day string) []app.Effect {
	return b.Log.GotoDate(day, func(l *app.LogLine) { b.Cursor, b.top = l, l })
}
