package ui

import (
	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/style"
)

// Scrollback holds one character's rendered lines. Lines are stored
// ready to draw (sanitized and styled) and wrapped lazily, only when they
// come into view, with the result cached per width.
type Scrollback struct {
	lines   []sbLine
	width   int
	offset  int // visual rows scrolled up from the bottom; 0 = live
	unseen  int // lines appended while scrolled up
	prompt  string
	more    bool       // older lines exist that have not been paged in yet
	loading bool       // a page of older lines is being read; see RequestOlder
	shown   []sbRef    // what each row of the last View shows
	sel     *selection // a mouse selection; see sbmouse.go
}

type sbLine struct {
	text  string
	rows  []string
	wrapW int
	// Plain text and its rows, for the mouse; see plainRows.
	plain   string
	pRows   []string
	pStarts []int
	plainW  int
}

func (l *sbLine) wrap(w int) []string {
	if l.wrapW != w || l.rows == nil {
		l.rows, l.wrapW = ansi.Wrap(l.text, w), w
	}
	return l.rows
}

// SetWidth sets the wrap width; cached wraps at other widths are redone
// lazily. While scrolled up, the offset is rebased so the same logical
// line stays at the bottom of the view instead of whatever row now sits
// the old number of rows up.
func (s *Scrollback) SetWidth(w int) {
	if w < 1 {
		w = 1
	}
	if w != s.width && s.offset > 0 {
		s.offset = s.rebase(s.w(), w)
	}
	s.width = w
}

// rebase maps the offset at width from to the offset at width to. The
// bottom row of the view is anchored to (logical line, position within
// the line); the position scales with the line's new row count.
func (s *Scrollback) rebase(from, to int) int {
	acc := 0
	for i := len(s.lines) - 1; i >= 0; i-- {
		n := len(s.lines[i].wrap(from))
		if s.offset < acc+n {
			fromTop := n - 1 - (s.offset - acc)
			nn := len(s.lines[i].wrap(to))
			newTop := min(fromTop*nn/n, nn-1)
			off := nn - 1 - newTop
			for _, l := range s.lines[i+1:] {
				off += len(l.wrap(to))
			}
			return off
		}
		acc += n
	}
	return s.offset // past the top; View clamps it
}

// Append adds a line. A pending prompt is cleared: the server has moved on.
// While scrolled up, the view stays put and the line counts as unseen.
func (s *Scrollback) Append(text string) {
	s.lines = append(s.lines, sbLine{text: text})
	s.prompt = ""
	if s.offset > 0 {
		s.offset += len(s.lines[len(s.lines)-1].wrap(s.w()))
		s.unseen++
	}
}

// loadingRow sits above the oldest line while more history may exist.
var loadingRow = style.Dim("─── loading older history… ───")

// SetMore records whether lines older than the first one exist. The
// scrollback never reads them itself: RequestOlder says when the view
// needs them, and the caller reads them off the UI goroutine and hands
// them to Prepend.
func (s *Scrollback) SetMore(more bool) { s.more = more }

// RequestOlder reports whether a view h rows tall at the current offset
// runs past the oldest line while more may exist and no read is already
// in flight. If so it marks a read as in flight; the caller must answer
// with Prepend.
func (s *Scrollback) RequestOlder(h int) bool {
	if !s.more || s.loading || s.hasRows(s.offset+h) {
		return false
	}
	s.loading = true
	return true
}

// hasRows reports whether the lines (and prompt) wrap to at least n rows.
func (s *Scrollback) hasRows(n int) bool {
	w := s.w()
	rows := 0
	if s.prompt != "" && s.offset == 0 {
		rows = len(ansi.Wrap(s.prompt, w))
	}
	for i := len(s.lines) - 1; i >= 0 && rows < n; i-- {
		rows += len(s.lines[i].wrap(w))
	}
	return rows >= n
}

// Prepend adds an older batch (oldest first; possibly empty) above the
// first line, ending the read RequestOlder started; more says whether
// still older lines exist. The view is anchored to the bottom, so
// prepending never moves what is on screen.
func (s *Scrollback) Prepend(lines []string, more bool) {
	s.loading, s.more = false, more
	batch := make([]sbLine, len(lines))
	for i, l := range lines {
		batch[i] = sbLine{text: l}
	}
	s.lines = append(batch, s.lines...)
	if s.sel != nil {
		s.sel.anchor.line += len(batch)
		s.sel.head.line += len(batch)
	}
}

// SetPrompt shows an unterminated prompt below the last line.
func (s *Scrollback) SetPrompt(p string) { s.prompt = p }

// Len is the number of logical lines.
func (s *Scrollback) Len() int { return len(s.lines) }

// Scrolled reports whether the view is above the live bottom.
func (s *Scrollback) Scrolled() bool { return s.offset > 0 }

// Unseen is the number of lines appended since scrolling up.
func (s *Scrollback) Unseen() int { return s.unseen }

// ScrollUp moves the view n rows toward older lines (clamped in View).
func (s *Scrollback) ScrollUp(n int) { s.offset += n }

// ScrollDown moves the view n rows toward newer lines.
func (s *Scrollback) ScrollDown(n int) {
	s.offset -= n
	if s.offset <= 0 {
		s.ToBottom()
	}
}

// ToBottom returns to the live view.
func (s *Scrollback) ToBottom() { s.offset, s.unseen = 0, 0 }

func (s *Scrollback) w() int {
	if s.width < 1 {
		return 1
	}
	return s.width
}

// View returns exactly h rows (blank-padded at the top) for the current
// scroll position, wrapping only the lines it needs.
func (s *Scrollback) View(h int) []string {
	if h < 1 {
		return nil
	}
	w := s.w()
	var tail []string // rows in reverse order, newest first
	var refs []sbRef  // what each tail row shows
	if s.prompt != "" && s.offset == 0 {
		rows := ansi.Wrap(s.prompt, w)
		for i := len(rows) - 1; i >= 0; i-- {
			tail = append(tail, rows[i])
			refs = append(refs, sbRef{line: -1})
		}
	}
	need := s.offset + h
	i := len(s.lines) - 1
	for len(tail) < need && i >= 0 {
		rows := s.lines[i].wrap(w)
		for j := len(rows) - 1; j >= 0; j-- {
			tail = append(tail, rows[j])
			refs = append(refs, sbRef{line: i, row: j})
		}
		i--
	}
	start := s.offset
	if i < 0 && len(tail) < need {
		if s.more {
			// Older lines are on their way: show the top with a loading
			// row, but keep the offset so the scroll lands once they arrive.
			tail = append(tail, loadingRow)
			refs = append(refs, sbRef{line: -1})
			start = max(0, len(tail)-h)
		} else { // hit the very top: clamp the offset
			s.offset = max(0, len(tail)-h)
			start = s.offset
			if s.offset == 0 {
				s.unseen = 0
			}
		}
	}
	start = min(start, len(tail))
	end := min(start+h, len(tail))
	out := make([]string, h)
	s.shown = make([]sbRef, h)
	for k := range s.shown {
		s.shown[k].line = -1
	}
	for k := start; k < end; k++ {
		y := h - 1 - (k - start)
		out[y], s.shown[y] = tail[k], refs[k]
		if refs[k].line >= 0 {
			out[y] = s.highlightRow(out[y], refs[k])
		}
	}
	return out
}
