package ui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// overLimit is the style for bytes past a line's max_line_bytes.
const overLimit = "\x1b[97;41m"

// Input gutter, that we show at the beginning of the line we send to the server
// (and the aligner that goes below it for continuation rows)
const (
	gutterMark  = "›"
	gutterBlank = " "
	gutterWidth = 1
)

// Input is a multi-line editor with per-character history. The cursor
// moves, and Backspace/Delete remove, whole grapheme clusters, so emoji
// ZWJ sequences, flags and skin-tone modifiers act as one character.
type Input struct {
	lines   [][]rune // never empty
	row     int
	col     int // rune index into lines[row], always on a cluster boundary
	history []string
	hist    int    // position while browsing history; len(history) = not browsing
	draft   string // text being typed before history browsing started
	goal    int    // screen column Up/Down aim for; -1 = the cursor's own
	width   int    // text width of the last Render; 0 = no wrapping
	masked  bool   // the last Render showed bullets
	sel     *inSel // a mouse selection; see StartSelect
}

// NewInput returns an empty editor.
func NewInput() *Input { return &Input{lines: [][]rune{nil}, goal: -1} }

// Value is the full text, lines joined with "\n".
func (in *Input) Value() string {
	parts := make([]string, len(in.lines))
	for i, l := range in.lines {
		parts[i] = string(l)
	}
	return strings.Join(parts, "\n")
}

// Empty reports whether there is no text.
func (in *Input) Empty() bool { return len(in.lines) == 1 && len(in.lines[0]) == 0 }

// SetValue replaces the text and puts the cursor at the end.
func (in *Input) SetValue(s string) {
	in.lines = nil
	for _, l := range strings.Split(s, "\n") {
		in.lines = append(in.lines, []rune(l))
	}
	in.row = len(in.lines) - 1
	in.col = len(in.lines[in.row])
	in.goal = -1
}

// Reset clears the text and leaves history browsing.
func (in *Input) Reset() {
	in.SetValue("")
	in.hist = len(in.history)
}

// Commit returns the text, records it in history, and clears the editor.
func (in *Input) Commit() string {
	v := in.Value()
	if v != "" && (len(in.history) == 0 || in.history[len(in.history)-1] != v) {
		in.history = append(in.history, v)
	}
	in.Reset()
	return v
}

// CommitSecret returns the text and clears the editor without recording
// it in history (for passwords).
func (in *Input) CommitSecret() string {
	v := in.Value()
	in.Reset()
	return v
}

// InsertText inserts s at the cursor; "\n" (or "\r\n") starts a new line.
func (in *Input) InsertText(s string) {
	in.goal = -1
	s = strings.ReplaceAll(s, "\r\n", "\n")
	for i, part := range strings.Split(s, "\n") {
		if i > 0 {
			in.Newline()
		}
		line := in.lines[in.row]
		ins := []rune(part)
		out := make([]rune, 0, len(line)+len(ins))
		out = append(out, line[:in.col]...)
		out = append(out, ins...)
		out = append(out, line[in.col:]...)
		in.lines[in.row] = out
		in.col = snapForward(out, in.col+len(ins))
	}
}

// clusters returns the rune index of every grapheme cluster boundary in
// line, from 0 through len(line).
func clusters(line []rune) []int {
	bounds := []int{0}
	g := uniseg.NewGraphemes(string(line))
	n := 0
	for g.Next() {
		n += utf8.RuneCountInString(g.Str())
		bounds = append(bounds, n)
	}
	return bounds
}

// prevBoundary is the cluster boundary before col (col > 0).
func prevBoundary(line []rune, col int) int {
	bounds := clusters(line)
	for i := len(bounds) - 1; i >= 0; i-- {
		if bounds[i] < col {
			return bounds[i]
		}
	}
	return 0
}

// nextBoundary is the cluster boundary after col (col < len(line)).
func nextBoundary(line []rune, col int) int {
	for _, b := range clusters(line) {
		if b > col {
			return b
		}
	}
	return len(line)
}

// snapBack moves col back to the start of the cluster it falls inside.
func snapBack(line []rune, col int) int {
	if col >= len(line) {
		return len(line)
	}
	return prevBoundary(line, col+1)
}

// snapForward moves col forward to the end of the cluster it falls inside
// (typed text can join the cluster after it, e.g. a skin-tone modifier).
func snapForward(line []rune, col int) int {
	if col <= 0 {
		return 0
	}
	return nextBoundary(line, col-1)
}

// Newline splits the current line at the cursor.
func (in *Input) Newline() {
	in.goal = -1
	line := in.lines[in.row]
	head := append([]rune(nil), line[:in.col]...)
	tail := append([]rune(nil), line[in.col:]...)
	in.lines[in.row] = head
	in.lines = append(in.lines[:in.row+1], append([][]rune{tail}, in.lines[in.row+1:]...)...)
	in.row++
	in.col = 0
}

// Backspace deletes before the cursor, joining lines at column 0.
func (in *Input) Backspace() {
	in.goal = -1
	if in.col > 0 {
		line := in.lines[in.row]
		from := prevBoundary(line, in.col)
		in.lines[in.row] = append(line[:from], line[in.col:]...)
		in.col = from
		return
	}
	if in.row == 0 {
		return
	}
	prev := in.lines[in.row-1]
	in.col = len(prev)
	in.lines[in.row-1] = append(prev, in.lines[in.row]...)
	in.lines = append(in.lines[:in.row], in.lines[in.row+1:]...)
	in.row--
}

// Delete deletes at the cursor, joining with the next line at line end.
func (in *Input) Delete() {
	in.goal = -1
	line := in.lines[in.row]
	if in.col < len(line) {
		in.lines[in.row] = append(line[:in.col], line[nextBoundary(line, in.col):]...)
		return
	}
	if in.row == len(in.lines)-1 {
		return
	}
	in.lines[in.row] = append(line, in.lines[in.row+1]...)
	in.lines = append(in.lines[:in.row+1], in.lines[in.row+2:]...)
}

// Left moves the cursor back one character, wrapping to the previous line.
func (in *Input) Left() {
	in.goal = -1
	if in.col > 0 {
		in.col = prevBoundary(in.lines[in.row], in.col)
	} else if in.row > 0 {
		in.row--
		in.col = len(in.lines[in.row])
	}
}

// Right moves the cursor forward one character, wrapping to the next line.
func (in *Input) Right() {
	in.goal = -1
	if in.col < len(in.lines[in.row]) {
		in.col = nextBoundary(in.lines[in.row], in.col)
	} else if in.row < len(in.lines)-1 {
		in.row++
		in.col = 0
	}
}

// Home moves to the start of the line; End to its end.
func (in *Input) Home() { in.goal, in.col = -1, 0 }
func (in *Input) End()  { in.goal, in.col = -1, len(in.lines[in.row]) }

// isWord reports whether r is part of a word for word-wise movement.
func isWord(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.M, r)
}

// wordStart is where a word-left from col lands: back over any non-word
// characters, then over the word before them.
func wordStart(line []rune, col int) int {
	for col > 0 && !isWord(line[col-1]) {
		col--
	}
	for col > 0 && isWord(line[col-1]) {
		col--
	}
	return snapBack(line, col)
}

// wordEnd is where a word-right from col lands: forward over any non-word
// characters, then to the end of the word after them.
func wordEnd(line []rune, col int) int {
	for col < len(line) && !isWord(line[col]) {
		col++
	}
	for col < len(line) && isWord(line[col]) {
		col++
	}
	return snapForward(line, col)
}

// WordLeft moves to the start of the previous word; at a line's start it
// wraps like Left.
func (in *Input) WordLeft() {
	if in.col == 0 {
		in.Left()
		return
	}
	in.goal, in.col = -1, wordStart(in.lines[in.row], in.col)
}

// WordRight moves to the end of the next word; at a line's end it wraps
// like Right.
func (in *Input) WordRight() {
	if in.col == len(in.lines[in.row]) {
		in.Right()
		return
	}
	in.goal, in.col = -1, wordEnd(in.lines[in.row], in.col)
}

// cut removes line[from:to] from the current line and leaves the cursor at from.
func (in *Input) cut(from, to int) {
	line := in.lines[in.row]
	in.lines[in.row] = append(line[:from:from], line[to:]...)
	in.goal, in.col = -1, from
}

// DeleteWordBack deletes back to the start of the word before the cursor;
// at a line's start it joins lines like Backspace.
func (in *Input) DeleteWordBack() {
	if in.col == 0 {
		in.Backspace()
		return
	}
	in.cut(wordStart(in.lines[in.row], in.col), in.col)
}

// DeleteWordForward deletes to the end of the word after the cursor; at a
// line's end it joins lines like Delete.
func (in *Input) DeleteWordForward() {
	if in.col == len(in.lines[in.row]) {
		in.Delete()
		return
	}
	in.cut(in.col, wordEnd(in.lines[in.row], in.col))
}

// KillToStart deletes from the start of the line to the cursor.
func (in *Input) KillToStart() { in.cut(0, in.col) }

// KillToEnd deletes from the cursor to the end of the line, or at the end
// joins the next line.
func (in *Input) KillToEnd() {
	if in.col == len(in.lines[in.row]) {
		in.Delete()
		return
	}
	line := in.lines[in.row]
	in.lines[in.row] = line[:in.col:in.col]
	in.goal = -1
}

// Up moves up a screen row, or on the top row recalls older history.
func (in *Input) Up() {
	rows := in.visualRows()
	if r, x := in.cursorAt(rows); r > 0 {
		in.moveTo(rows[r-1], x)
		return
	}
	if in.hist == 0 || len(in.history) == 0 {
		return
	}
	if in.hist == len(in.history) {
		in.draft = in.Value()
	}
	in.hist--
	in.SetValue(in.history[in.hist])
}

// Down moves down a screen row, or on the bottom row recalls newer history
// and finally the draft that was being typed.
func (in *Input) Down() {
	rows := in.visualRows()
	if r, x := in.cursorAt(rows); r < len(rows)-1 {
		in.moveTo(rows[r+1], x)
		return
	}
	if in.hist >= len(in.history) {
		return
	}
	in.hist++
	if in.hist == len(in.history) {
		in.SetValue(in.draft)
	} else {
		in.SetValue(in.history[in.hist])
	}
}

// Click puts the cursor at text cell (x, y) of the last Render, clamped to
// the text. x and y exclude the gutter.
func (in *Input) Click(x, y int) {
	rows := in.visualRows()
	in.goal = -1
	in.moveTo(rows[min(max(0, y), len(rows)-1)], max(0, x))
	in.goal = -1
}

// moveTo puts the cursor on screen row vr, at the character under cell x
// (or the goal column kept from the last vertical move).
func (in *Input) moveTo(vr vrow, x int) {
	if in.goal < 0 {
		in.goal = x
	}
	in.row, in.col = vr.line, vr.start
	for _, c := range vr.cells {
		if c.x > in.goal {
			return
		}
		in.col = c.col
	}
	if vr.last && vr.endX <= in.goal {
		in.col = vr.end
	}
}

// cell is one grapheme cluster on screen.
type cell struct {
	col  int    // rune index of the cluster in its line
	x    int    // screen column, excluding the gutter
	text string // the cluster
}

// vrow is one screen row of the input: part or all of one logical line.
type vrow struct {
	line       int
	start, end int // rune range of the line shown on this row
	endX       int // screen column after the last cell
	cells      []cell
	last       bool // the line's final row; the cursor may sit at end
}

// visualRows wraps the text at the width of the last Render. A cursor at
// the end of a line that exactly fills its last row gets a row of its own.
func (in *Input) visualRows() []vrow {
	var rows []vrow
	for li, line := range in.lines {
		cur := vrow{line: li}
		g := uniseg.NewGraphemes(string(line))
		for ri := 0; g.Next(); {
			cluster := g.Str()
			rw := xansi.StringWidth(cluster)
			if in.masked {
				rw = 1
			}
			if in.width > 0 && cur.endX+rw > in.width && len(cur.cells) > 0 {
				cur.end = ri
				rows = append(rows, cur)
				cur = vrow{line: li, start: ri}
			}
			cur.cells = append(cur.cells, cell{col: ri, x: cur.endX, text: cluster})
			cur.endX += rw
			ri += utf8.RuneCountInString(cluster)
		}
		cur.end = len(line)
		if li == in.row && in.col == len(line) && in.width > 0 && cur.endX >= in.width && len(line) > 0 {
			rows = append(rows, cur)
			cur = vrow{line: li, start: len(line), end: len(line)}
		}
		cur.last = true
		rows = append(rows, cur)
	}
	return rows
}

// cursorAt finds the cursor's screen row and column in rows.
func (in *Input) cursorAt(rows []vrow) (r, x int) {
	for i, vr := range rows {
		if vr.line != in.row {
			continue
		}
		for _, c := range vr.cells {
			if c.col == in.col {
				return i, c.x
			}
		}
		if vr.last && in.col == vr.end {
			return i, vr.endX
		}
	}
	return 0, 0
}

// Render lays the text out for width w (including a 2-cell gutter),
// highlighting bytes past limit in red, and returns the rows plus the
// cursor's row and column. Each line that will be sent on its own starts
// with "›": every line, or with joined (newline_mode = "flatten") only
// the first, and the limit applies to the lines joined by spaces. Masked
// text shows as bullets. Render remembers w and masked for Up, Down and
// Click, which move by screen row.
func (in *Input) Render(w, limit int, joined, masked bool) (rows []string, curRow, curCol int) {
	in.width, in.masked = max(1, w-gutterWidth), masked
	vrows := in.visualRows()
	curRow, curCol = in.cursorAt(vrows)
	total, bytes, red := 0, 0, false // total: bytes before this line when joined
	for i, vr := range vrows {
		if i == 0 || vrows[i-1].line != vr.line { // a new line
			if i > 0 {
				total = bytes
			}
			bytes, red = 0, false
			if joined {
				if vr.line > 0 {
					total++ // the joining space
				}
				bytes = total
				red = limit > 0 && bytes > limit
			}
		}
		var b strings.Builder
		if vr.start == 0 && (!joined || vr.line == 0) {
			b.WriteString(gutterMark)
		} else {
			b.WriteString(gutterBlank)
		}
		if red {
			b.WriteString(overLimit)
		}
		for _, c := range vr.cells {
			bytes += len(c.text)
			if !red && limit > 0 && bytes > limit {
				red = true
				b.WriteString(overLimit)
			}
			switch {
			case masked:
				b.WriteString("•")
			case in.selected(vr.line, c.col):
				b.WriteString(reverse + c.text + "\x1b[27m")
			default:
				b.WriteString(c.text)
			}
		}
		if red {
			b.WriteString("\x1b[0m")
		}
		rows = append(rows, b.String())
	}
	return rows, curRow, curCol + gutterWidth
}

// OverLimit reports whether any line is longer than limit bytes, or with
// joined whether the lines joined by spaces are.
func (in *Input) OverLimit(limit int, joined bool) bool {
	if limit <= 0 {
		return false
	}
	if joined {
		return len(strings.Join(strings.Split(in.Value(), "\n"), " ")) > limit
	}
	for _, l := range in.lines {
		if len(string(l)) > limit {
			return true
		}
	}
	return false
}

// inPos is a place in the input: a line and a rune index in it.
type inPos struct{ line, col int }

func (p inPos) before(q inPos) bool {
	return p.line < q.line || p.line == q.line && p.col < q.col
}

// inSel is a mouse selection in the input. Both ends are the start of a
// character, and both characters are selected.
type inSel struct {
	anchor, head inPos
	dragging     bool
	moved        bool
}

// posAt is the character under cell x of screen row y (both from the
// last Render, x past the gutter), clamped to the text.
func (in *Input) posAt(x, y int) inPos {
	rows := in.visualRows()
	vr := rows[min(max(0, y), len(rows)-1)]
	p := inPos{vr.line, vr.start}
	for _, c := range vr.cells {
		if c.x > x {
			break
		}
		p.col = c.col
	}
	return p
}

// StartSelect puts the cursor at the clicked cell and starts a drag
// there. A masked (password) input never selects.
func (in *Input) StartSelect(x, y int) {
	in.Click(x, y)
	in.sel = nil
	if !in.masked {
		p := in.posAt(x, y)
		in.sel = &inSel{anchor: p, head: p, dragging: true}
	}
}

// Dragging reports whether a drag is in progress.
func (in *Input) Dragging() bool { return in.sel != nil && in.sel.dragging }

// DragTo moves the drag's free end, and the cursor, to the cell.
func (in *Input) DragTo(x, y int) {
	if !in.Dragging() {
		return
	}
	in.Click(x, y)
	p := in.posAt(x, y)
	in.sel.head = p
	in.sel.moved = in.sel.moved || p != in.sel.anchor
}

// EndSelect finishes the drag and returns the selected text: logical
// lines joined with "\n", with no breaks where rows merely wrap. A click
// that never moved selects nothing and returns "".
func (in *Input) EndSelect() string {
	if !in.Dragging() {
		return ""
	}
	in.sel.dragging = false
	if !in.sel.moved {
		in.sel = nil
		return ""
	}
	from, to, _ := in.selRange()
	var b strings.Builder
	for li := from.line; li <= to.line; li++ {
		line := in.lines[li]
		a, z := 0, len(line)
		if li == from.line {
			a = from.col
		}
		if li == to.line {
			z = to.col
		}
		if li > from.line {
			b.WriteByte('\n')
		}
		b.WriteString(string(line[min(a, z):z]))
	}
	return b.String()
}

// ClearSelection drops the selection.
func (in *Input) ClearSelection() { in.sel = nil }

// selRange is the selection as rune ranges [from, to) across lines,
// clamped to the current text.
func (in *Input) selRange() (from, to inPos, ok bool) {
	if in.sel == nil || !in.sel.moved {
		return inPos{}, inPos{}, false
	}
	from, to = in.sel.anchor, in.sel.head
	if to.before(from) {
		from, to = to, from
	}
	clamp := func(p inPos) inPos {
		p.line = min(max(0, p.line), len(in.lines)-1)
		p.col = min(max(0, p.col), len(in.lines[p.line]))
		return p
	}
	from, to = clamp(from), clamp(to)
	if rest := string(in.lines[to.line][to.col:]); rest != "" { // take in the last character
		c, _, _, _ := uniseg.FirstGraphemeClusterInString(rest, -1)
		to.col += utf8.RuneCountInString(c)
	}
	return from, to, true
}

// selected reports whether the character at line li, rune col, is in the
// selection.
func (in *Input) selected(li, col int) bool {
	from, to, ok := in.selRange()
	p := inPos{li, col}
	return ok && !p.before(from) && p.before(to)
}
