package ui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// dragHarness is log mode over scene1, drawn, with mouse helpers on its
// lines: press, move and let go on the row line i is drawn on.
type dragHarness struct {
	*harness
	b *browse
	x int
}

func newDragHarness(t *testing.T) *dragHarness {
	h := newHarness(t, map[string]string{"fm": echoWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.screen()
	return &dragHarness{harness: h, b: h.br(), x: h.m.layout().sw + 1 + 10}
}

func (d *dragHarness) row(i int) int {
	d.t.Helper()
	for row, bl := range d.b.rowLines {
		if bl == d.b.Lines[i] {
			return row + d.m.layout().top
		}
	}
	d.t.Fatalf("line %d not on screen", i)
	return 0
}

func (d *dragHarness) press(i int, mod tea.KeyMod) {
	d.m.Update(tea.MouseClickMsg{X: d.x, Y: d.row(i), Button: tea.MouseLeft, Mod: mod})
	d.screen()
}

func (d *dragHarness) moveTo(y int, mod tea.KeyMod) tea.Cmd {
	_, cmd := d.m.Update(tea.MouseMotionMsg{X: d.x, Y: y, Button: tea.MouseLeft, Mod: mod})
	d.screen()
	return cmd
}

func (d *dragHarness) release() {
	d.m.Update(tea.MouseReleaseMsg{X: d.x, Button: tea.MouseLeft})
	d.screen()
}

// drag presses on line from, moves over each line to line to, and lets go.
func (d *dragHarness) drag(from, to int, mod tea.KeyMod) {
	d.press(from, mod)
	step := 1
	if to < from {
		step = -1
	}
	for i := from + step; ; i += step {
		d.moveTo(d.row(i), mod)
		if i == to {
			break
		}
	}
	d.release()
}

func (d *dragHarness) span() (int, int) { return d.b.Index(d.b.Start), d.b.Index(d.b.End) }

// A drag selects the lines it crosses, as s where it's pressed and the
// cursor moved to where it's let go: the range is still live there, for
// keys and clicks to finish.
func TestLogDragSelects(t *testing.T) {
	d := newDragHarness(t)
	d.drag(1, 4, 0)
	if s, e := d.span(); s != 1 || e != 4 || !d.b.Marking || d.b.Cursor != d.b.Lines[4] {
		t.Errorf("drag 1→4: %d–%d, marking %v, cursor %d; want a live 1–4, cursor on 4", s, e, d.b.Marking, d.b.Index(d.b.Cursor))
	}
	d.key("down")
	if s, e := d.span(); s != 1 || e != 5 {
		t.Errorf("down after the drag: %d–%d; want 1–5", s, e)
	}
	d.drag(3, 0, 0) // a new range, upward
	if s, e := d.span(); s != 0 || e != 3 || !d.b.Marking {
		t.Errorf("drag 3→0: %d–%d, marking %v; want a live 0–3", s, e, d.b.Marking)
	}
}

// While the drag is on, the range follows the pointer, as it follows
// the cursor while selecting.
func TestLogDragRangeFollows(t *testing.T) {
	d := newDragHarness(t)
	d.press(1, 0)
	d.moveTo(d.row(3), 0)
	if s, e := d.span(); !d.b.Marking || s != 1 || e != 3 {
		t.Fatalf("mid-drag: marking %v, %d–%d; want a live 1–3", d.b.Marking, s, e)
	}
	d.moveTo(d.row(2), 0)
	if s, e := d.span(); s != 1 || e != 2 {
		t.Errorf("back up a line: %d–%d; want 1–2", s, e)
	}
	d.release()
}

// A drag drops a live range and starts its own, as Esc then s would.
func TestLogDragReplacesLiveRange(t *testing.T) {
	d := newDragHarness(t)
	d.press(5, 0)
	d.release()
	d.keys("s", "up")
	d.drag(0, 2, 0)
	if s, e := d.span(); s != 0 || e != 2 {
		t.Errorf("drag over a live range: %d–%d; want 0–2", s, e)
	}
}

// A press and release on one line is a click, even with the pointer
// moving within the line: while selecting it grows the range, as
// click, s, click always has.
func TestLogClickWithinLineStaysAClick(t *testing.T) {
	d := newDragHarness(t)
	d.press(1, 0)
	d.release()
	d.key("s")
	d.press(4, 0)
	d.m.Update(tea.MouseMotionMsg{X: d.x + 3, Y: d.row(4), Button: tea.MouseLeft})
	d.release()
	if s, e := d.span(); !d.b.Marking || s != 1 || e != 4 {
		t.Errorf("click while selecting: marking %v, %d–%d; want a live 1–4", d.b.Marking, s, e)
	}
}

// A click acts when it's let go: a press alone, which might become a
// drag, changes nothing yet.
func TestLogClickActsOnRelease(t *testing.T) {
	d := newDragHarness(t)
	was := d.b.Cursor
	d.press(1, 0)
	if d.b.Cursor != was {
		t.Errorf("a press moved the cursor to %d", d.b.Index(d.b.Cursor))
	}
	d.release()
	if d.b.Cursor != d.b.Lines[1] {
		t.Errorf("after release: cursor %d; want 1", d.b.Index(d.b.Cursor))
	}
}

// A shift-click inside a locked range takes its line out, or puts it
// back; outside one it only moves the cursor.
func TestLogShiftClickLockedRange(t *testing.T) {
	d := newDragHarness(t)
	d.drag(1, 4, 0)
	d.key("s")
	d.press(2, tea.ModShift)
	d.release()
	if !d.b.Excluded[d.b.Lines[2]] || d.b.Cursor != d.b.Lines[4] {
		t.Fatalf("shift-click inside: 2 out %v, cursor %d; want 2 out, cursor kept on 4", d.b.Excluded[d.b.Lines[2]], d.b.Index(d.b.Cursor))
	}
	d.press(2, tea.ModShift)
	d.release()
	if d.b.Excluded[d.b.Lines[2]] {
		t.Error("a second shift-click should bring 2 back")
	}
	d.press(0, tea.ModShift)
	d.release()
	if s, e := d.span(); s != 1 || e != 4 || d.b.Cursor != d.b.Lines[0] {
		t.Errorf("shift-click outside: %d–%d, cursor %d; want the range kept, cursor on 0", s, e, d.b.Index(d.b.Cursor))
	}
}

// A shift-drag paints the range the way the pressed line went, each
// line once however often it's crossed, and leaves the cursor be.
func TestLogShiftDragPaints(t *testing.T) {
	d := newDragHarness(t)
	d.drag(0, 5, 0)
	d.key("s")
	d.press(3, 0)
	d.release() // the cursor on 3; the range stays
	d.key("space")
	if !d.b.Excluded[d.b.Lines[3]] {
		t.Fatal("space should leave 3 out")
	}
	d.drag(1, 4, tea.ModShift) // 1 is in: paints out, 3 stays out
	for i := 1; i <= 4; i++ {
		if !d.b.Excluded[d.b.Lines[i]] {
			t.Errorf("after shift-drag 1→4, line %d is in", i)
		}
	}
	if d.b.Excluded[d.b.Lines[0]] || d.b.Excluded[d.b.Lines[5]] || d.b.Cursor != d.b.Lines[3] {
		t.Errorf("0 out %v, 5 out %v, cursor %d; want 0 and 5 in, cursor kept on 3",
			d.b.Excluded[d.b.Lines[0]], d.b.Excluded[d.b.Lines[5]], d.b.Index(d.b.Cursor))
	}
	d.press(2, tea.ModShift) // 2 is out: paints in, back and forth
	d.moveTo(d.row(3), tea.ModShift)
	d.moveTo(d.row(2), tea.ModShift)
	d.moveTo(d.row(3), tea.ModShift)
	d.release()
	if d.b.Excluded[d.b.Lines[2]] || d.b.Excluded[d.b.Lines[3]] {
		t.Errorf("shift-drag 2↔3 from an out line: 2 out %v, 3 out %v; want both in", d.b.Excluded[d.b.Lines[2]], d.b.Excluded[d.b.Lines[3]])
	}
	if s, e := d.span(); s != 0 || e != 5 {
		t.Errorf("painting changed the range to %d–%d", s, e)
	}
}

// Under a prompt a drag changes nothing, as a click doesn't.
func TestLogDragUnderPrompt(t *testing.T) {
	d := newDragHarness(t)
	d.key("/")
	d.drag(1, 3, 0)
	if d.b.Start != nil {
		t.Errorf("drag under a prompt selected %d–%d", d.b.Index(d.b.Start), d.b.Index(d.b.End))
	}
}

// Held above the log, a drag keeps scrolling up, the range's end
// riding along, until the pointer comes back or lets go.
func TestLogDragScrollsAtEdge(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": echoWorld})
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	h.writeLog(day24, lines...)
	h.key("ctrl+l")
	h.screen()
	d := &dragHarness{harness: h, b: h.br(), x: h.m.layout().sw + 1 + 10}
	first := d.b.Index(d.b.lineNear(0)) // row 0 may be a divider
	d.press(first+2, 0)
	d.moveTo(d.row(first), 0)
	cmd := d.moveTo(d.m.layout().top-1, 0) // the top bar
	if cmd == nil {
		t.Fatal("dragging above the log should start scrolling")
	}
	for range 5 {
		msg := cmd()
		if _, ok := msg.(logEdgeMsg); !ok {
			t.Fatalf("scrolling ticks with %T", msg)
		}
		_, cmd = d.m.Update(msg)
		d.screen()
		if cmd == nil {
			t.Fatal("scrolling stopped while held above the log")
		}
	}
	if s, e := d.span(); e != first+2 || s >= first || d.b.Cursor != d.b.Start {
		t.Errorf("after scrolling: %d–%d, cursor %d; want the start above %d, the end on %d", s, e, d.b.Index(d.b.Cursor), first, first+2)
	}
	if d.b.Index(d.b.lineNear(0)) >= first {
		t.Errorf("the view didn't scroll: first row %d, was %d", d.b.Index(d.b.lineNear(0)), first)
	}
	d.moveTo(d.row(d.b.Index(d.b.lineNear(1))), 0) // back in
	if _, cmd = d.m.Update(cmd()); cmd != nil {
		t.Error("scrolling went on with the pointer back in the log")
	}
	d.release()
	if !d.b.Marking {
		t.Error("letting go locked the range; it should stay live")
	}
}
