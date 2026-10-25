package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/app"
)

// A mouse drag over the log picks the lines it crosses, as the page's
// does: s where it's pressed, and the cursor moved to where it's let go,
// the range still live for keys and clicks to finish. A press
// becomes a drag once the pointer reaches another line (or leaves the
// log); a press let go on its own line is a click, so a click acts on
// release. Terminals that keep drags for their own selection (Blink)
// still send clicks, and click, s, click works as it always has.

// edgeEvery is how often a drag held above or below the log scrolls it.
// Terminals report motion only when the pointer moves, so a tick keeps
// it going.
const edgeEvery = 60 * time.Millisecond

// logEdgeMsg is a tick of drag d's scrolling.
type logEdgeMsg struct{ d *logDrag }

// logDrag is a press in the log and, once it moves on, the drag it
// became.
type logDrag struct {
	from    *app.LogLine // the line pressed (nil: a divider, or none)
	shift   bool
	started bool         // it's a drag, not a click
	none    bool         // it can't be one (under a prompt, a shift-drag off the range): its release is a click
	paint   *logPaint    // a shift-drag's
	at      *app.LogLine // the line it last reached
	y       int          // the pointer's body row, outside the body too
	edge    int          // rows the pointer is above (<0) or below (>0) the body
	ticking bool         // a logEdgeMsg is on its way
}

// logPaint is a shift-drag over the range: the line pressed says which way
// (in the range: out; already out: back in), and each line of the range
// it crosses goes that way, once, however often it's crossed.
type logPaint struct {
	out  bool
	done map[*app.LogLine]bool
}

// lineAt is the line drawn on body row y, or nil (a divider, or off the
// body).
func (b *browse) lineAt(y int) *app.LogLine {
	if y < 0 || y >= len(b.rowLines) {
		return nil
	}
	return b.rowLines[y]
}

// lineNear is the line nearest body row y, clamped into the body: the
// one drawn there, else the nearest above, else below.
func (b *browse) lineNear(y int) *app.LogLine {
	y = min(max(0, y), len(b.rowLines)-1)
	for i := y; i >= 0; i-- {
		if l := b.rowLines[i]; l != nil {
			return l
		}
	}
	for i := y + 1; i < len(b.rowLines); i++ {
		if l := b.rowLines[i]; l != nil {
			return l
		}
	}
	return nil
}

// press is a left press on body row y. It does nothing yet: it's a
// click or a drag, and only moving or letting go says which.
func (b *browse) press(y int, shift bool) {
	b.drag = &logDrag{from: b.lineAt(y), shift: shift, y: y}
}

// dragMove is the pointer at body row y (of bodyH) with the button
// down. The first time it reaches another line, or leaves the body, the
// press becomes a drag. It says whether to start the edge's tick.
func (b *browse) dragMove(y, bodyH int) bool {
	d := b.drag
	if d == nil || d.none || d.from == nil {
		return false
	}
	d.y = y
	l := b.lineNear(y)
	if !d.started {
		if l == d.from && y >= 0 && y < bodyH {
			return false
		}
		if !b.dragStart() {
			d.none = true
			return false
		}
	}
	b.follow(l)
	d.edge = 0
	if y < 0 {
		d.edge = y
	} else if y >= bodyH {
		d.edge = y - bodyH + 1
	}
	if d.edge != 0 && !d.ticking {
		d.ticking = true
		return true
	}
	return false
}

// dragStart turns the press into a drag. A plain one starts a range at
// the pressed line, dropping a live one first as Esc would; a shift one
// paints, if it was pressed on the range. False under a prompt, which
// the range can't change under.
func (b *browse) dragStart() bool {
	d := b.drag
	if b.prompt != promptNone {
		return false
	}
	if d.shift {
		if !b.InRange(d.from) {
			return false
		}
		d.paint = &logPaint{out: !b.Excluded[d.from], done: map[*app.LogLine]bool{}}
	} else {
		b.StopSearch()
		b.focusPanel(false)
		if b.Marking {
			b.EndMark()
		}
		b.SetCursor(d.from)
		b.Mark()
	}
	d.started, d.at = true, d.from
	if d.paint != nil {
		b.paintLine(d.from)
	}
	return true
}

// follow takes the drag to l: the cursor goes there and the range's end
// with it, or a paint does l.
func (b *browse) follow(l *app.LogLine) {
	d := b.drag
	if l == nil || l == d.at {
		return
	}
	d.at = l
	if d.paint != nil {
		b.paintLine(l)
		return
	}
	b.SetCursor(l)
}

// paintLine takes l out of the range or puts it back, as the paint
// goes, the first time it's reached. A line already that way is left
// alone.
func (b *browse) paintLine(l *app.LogLine) {
	p := b.drag.paint
	if p.done[l] {
		return
	}
	p.done[l] = true
	if b.InRange(l) && b.Excluded[l] != p.out {
		b.ToggleExclude(l)
	}
}

// edgeStep is a tick of a drag held above or below the log: it scrolls
// a line for each row out. A plain drag's cursor goes with it, and the
// range's end; a paint takes the line the pointer's over now. It says
// whether to tick again.
func (b *browse) edgeStep() ([]app.Effect, bool) {
	d := b.drag
	if d.edge == 0 {
		d.ticking = false
		return nil, false
	}
	if d.paint != nil {
		b.follow(b.lineNear(d.y))
		return b.scrollBy(d.edge), true
	}
	eff := b.MoveCursor(d.edge)
	d.at = b.Cursor
	return eff, true
}

// release is letting go. A press that never became a drag is a click on
// the line it was pressed on; a drag's range stays as it is, still live.
func (b *browse) release() {
	d := b.drag
	b.drag = nil
	if d != nil && !d.started {
		b.click(d.from, d.shift)
	}
}

// edgeTick is the next tick of d's scrolling.
func edgeTick(d *logDrag) tea.Cmd {
	return tea.Tick(edgeEvery, func(time.Time) tea.Msg { return logEdgeMsg{d} })
}
