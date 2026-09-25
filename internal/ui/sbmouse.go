package ui

import (
	"regexp"
	"strings"
	"unicode/utf8"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/style"
)

// Mouse support for the scrollback: positions under the pointer, links,
// and a drag selection to copy. Positions are logical (line, byte offset
// in the line's plain text), so they survive scrolling and rewrapping.

// sbPos is a place in the scrollback's text.
type sbPos struct{ line, off int }

func (p sbPos) before(q sbPos) bool {
	return p.line < q.line || p.line == q.line && p.off < q.off
}

// sbRef says what a drawn row shows: row row of line line, or nothing
// (line -1) for padding, the prompt and the loading row.
type sbRef struct{ line, row int }

// selection is a drag in progress or done. Both ends are inclusive.
type selection struct {
	anchor, head sbPos
	dragging     bool // the button is still down
	moved        bool // the pointer has left the anchor; a click otherwise
}

// plainRows is the line's wrapped rows without escapes, and the byte
// offset in its plain text where each starts. Wrapping only drops
// whitespace where it breaks, so each row is found in order.
func (l *sbLine) plainRows(w int) (plain string, rows []string, starts []int) {
	styled := l.wrap(w)
	if l.plainW != w || l.pRows == nil {
		l.plain = ansi.Strip(l.text)
		l.pRows, l.pStarts = make([]string, len(styled)), make([]int, len(styled))
		pos := 0
		for i, r := range styled {
			r = ansi.Strip(r)
			if j := strings.Index(l.plain[pos:], r); j >= 0 {
				pos += j
			}
			l.pRows[i], l.pStarts[i] = r, pos
			pos += len(r)
		}
		l.plainW = w
		l.links = nil
		for _, m := range urlRE.FindAllStringIndex(l.plain, -1) {
			l.links = append(l.links, [2]int{m[0], m[0] + len(trimURL(l.plain[m[0]:m[1]]))})
		}
	}
	return l.plain, l.pRows, l.pStarts
}

// colToByte is the byte index in s of the cell at column col, or len(s)
// when col is past the end.
func colToByte(s string, col int) int {
	w := 0
	for i, r := range s {
		w += xansi.StringWidth(string(r))
		if w > col {
			return i
		}
	}
	return len(s)
}

// At maps a cell in the last drawn view (row y from the top, column x) to
// a text position. ok is false over rows that aren't text.
func (s *Scrollback) At(y, x int) (p sbPos, ok bool) {
	if y < 0 || y >= len(s.shown) || s.shown[y].line < 0 {
		return sbPos{}, false
	}
	ref := s.shown[y]
	_, rows, starts := s.lines[ref.line].plainRows(s.w())
	return sbPos{ref.line, starts[ref.row] + colToByte(rows[ref.row], max(0, x))}, true
}

// urlRE finds web links. Trailing punctuation is trimmed by trimURL.
var urlRE = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `]+`)

// trimURL drops sentence punctuation after a link, and a closing bracket
// that doesn't close one inside it.
func trimURL(u string) string {
	for u != "" {
		switch c := u[len(u)-1]; {
		case strings.IndexByte(".,;:!?'\"", c) >= 0:
		case c == ')' && strings.Count(u, "(") < strings.Count(u, ")"):
		case c == ']' && strings.Count(u, "[") < strings.Count(u, "]"):
		default:
			return u
		}
		u = u[:len(u)-1]
	}
	return u
}

// linkAt is the byte range of the link at p, if there is one.
func (s *Scrollback) linkAt(p sbPos) ([2]int, bool) {
	if p.line < 0 || p.line >= len(s.lines) {
		return [2]int{}, false
	}
	l := &s.lines[p.line]
	l.plainRows(s.w())
	for _, lk := range l.links {
		if p.off >= lk[0] && p.off < lk[1] {
			return lk, true
		}
	}
	return [2]int{}, false
}

// URLAt is the link at p, or "".
func (s *Scrollback) URLAt(p sbPos) string {
	lk, ok := s.linkAt(p)
	if !ok {
		return ""
	}
	return s.lines[p.line].plain[lk[0]:lk[1]]
}

// Hover records the text under the pointer (nil when it's elsewhere), so
// the link there is drawn lit.
func (s *Scrollback) Hover(p *sbPos) { s.hover = p }

// Link styles. They replace the line's own styling across the link.
const (
	linkSGR  = "\x1b[4m"    // underlined
	hoverSGR = "\x1b[4;94m" // underlined, bright blue: under the pointer
)

// linkRow draws the links in one row underlined, and the one under the
// pointer blue as well, whatever the line's own colors.
func (s *Scrollback) linkRow(row string, ref sbRef) string {
	l := &s.lines[ref.line]
	_, rows, starts := l.plainRows(s.w())
	pr, start := rows[ref.row], starts[ref.row]
	var hovered [2]int
	hoverOK := false
	if s.hover != nil && s.hover.line == ref.line {
		hovered, hoverOK = s.linkAt(*s.hover)
	}
	for i := len(l.links) - 1; i >= 0; i-- { // right to left, so earlier columns hold
		lk := l.links[i]
		a, z := max(0, lk[0]-start), min(len(pr), lk[1]-start)
		if a >= z {
			continue
		}
		sgr := linkSGR
		if hoverOK && lk == hovered {
			sgr = hoverSGR
		}
		c1, c2 := xansi.StringWidth(pr[:a]), xansi.StringWidth(pr[:z])
		row = xansi.Cut(row, 0, c1) + style.Reset + sgr + pr[a:z] + style.Reset + xansi.Cut(row, c2, 1<<30)
	}
	return row
}

// StartSelect starts a drag at p, dropping any earlier selection.
func (s *Scrollback) StartSelect(p sbPos) {
	s.sel = &selection{anchor: p, head: p, dragging: true}
}

// Dragging reports whether a drag is in progress.
func (s *Scrollback) Dragging() bool { return s.sel != nil && s.sel.dragging }

// DragTo moves the drag's free end to p.
func (s *Scrollback) DragTo(p sbPos) {
	if s.sel == nil || !s.sel.dragging {
		return
	}
	s.sel.head = p
	s.sel.moved = s.sel.moved || p != s.sel.anchor
}

// EndSelect finishes the drag. It returns the selected text, or, when
// the pointer never moved (a click), "" and where it was clicked, and
// then there is no selection.
func (s *Scrollback) EndSelect() (text string, click sbPos, clicked bool) {
	if s.sel == nil || !s.sel.dragging {
		return "", sbPos{}, false
	}
	s.sel.dragging = false
	if !s.sel.moved {
		click = s.sel.anchor
		s.sel = nil
		return "", click, true
	}
	return s.selectedText(), sbPos{}, false
}

// ClearSelection drops the selection.
func (s *Scrollback) ClearSelection() { s.sel = nil }

// selRange is the selection as a half-open byte range [from, to) running
// from line from.line to line to.line.
func (s *Scrollback) selRange() (from, to sbPos, ok bool) {
	if s.sel == nil || !s.sel.moved {
		return sbPos{}, sbPos{}, false
	}
	from, to = s.sel.anchor, s.sel.head
	if to.before(from) {
		from, to = to, from
	}
	plain, _, _ := s.lines[to.line].plainRows(s.w())
	if to.off < len(plain) { // take in the last character
		_, n := utf8.DecodeRuneInString(plain[to.off:])
		to.off += n
	}
	return from, to, true
}

// selectedText is the selection's plain text, lines joined with "\n".
func (s *Scrollback) selectedText() string {
	from, to, ok := s.selRange()
	if !ok {
		return ""
	}
	var b strings.Builder
	for i := from.line; i <= to.line; i++ {
		plain, _, _ := s.lines[i].plainRows(s.w())
		a, z := 0, len(plain)
		if i == from.line {
			a = from.off
		}
		if i == to.line {
			z = to.off
		}
		if i > from.line {
			b.WriteByte('\n')
		}
		b.WriteString(plain[min(a, z):z])
	}
	return b.String()
}

// highlightRow draws the selected part of one row in reverse video.
func (s *Scrollback) highlightRow(row string, ref sbRef) string {
	from, to, ok := s.selRange()
	if !ok || ref.line < from.line || ref.line > to.line {
		return row
	}
	_, rows, starts := s.lines[ref.line].plainRows(s.w())
	pr, start := rows[ref.row], starts[ref.row]
	a, z := 0, len(pr)
	if ref.line == from.line {
		a = min(max(0, from.off-start), len(pr))
	}
	if ref.line == to.line {
		z = min(max(0, to.off-start), len(pr))
	}
	if a >= z {
		return row
	}
	c1, c2 := xansi.StringWidth(pr[:a]), xansi.StringWidth(pr[:z])
	return xansi.Cut(row, 0, c1) + style.Reset + reverse + pr[a:z] + style.Reset + xansi.Cut(row, c2, 1<<30)
}
