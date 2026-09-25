package ui

import "github.com/latrani/Kiln/internal/ansi"

// Scrollback holds one character's rendered lines. Lines are stored
// ready to draw (sanitized and styled) and wrapped lazily, only when they
// come into view, with the result cached per width.
type Scrollback struct {
	lines  []sbLine
	width  int
	offset int // visual rows scrolled up from the bottom; 0 = live
	unseen int // lines appended while scrolled up
	prompt string
}

type sbLine struct {
	text  string
	rows  []string
	wrapW int
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
	if s.prompt != "" && s.offset == 0 {
		rows := ansi.Wrap(s.prompt, w)
		for i := len(rows) - 1; i >= 0; i-- {
			tail = append(tail, rows[i])
		}
	}
	need := s.offset + h
	i := len(s.lines) - 1
	for ; i >= 0 && len(tail) < need; i-- {
		rows := s.lines[i].wrap(w)
		for j := len(rows) - 1; j >= 0; j-- {
			tail = append(tail, rows[j])
		}
	}
	if i < 0 && len(tail) < need { // hit the top: clamp the offset
		s.offset = max(0, len(tail)-h)
		if s.offset == 0 {
			s.unseen = 0
		}
	}
	start := min(s.offset, len(tail))
	end := min(start+h, len(tail))
	out := make([]string, h)
	for k := start; k < end; k++ {
		out[h-1-(k-start)] = tail[k]
	}
	return out
}
