package ui

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
)

// overLimit is the style for bytes past a line's max_line_bytes.
const overLimit = "\x1b[97;41m"

// Input is a multi-line editor with per-character history.
type Input struct {
	lines   [][]rune // never empty
	row     int
	col     int
	history []string
	hist    int    // position while browsing history; len(history) = not browsing
	draft   string // text being typed before history browsing started
}

// NewInput returns an empty editor.
func NewInput() *Input { return &Input{lines: [][]rune{nil}} }

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
		in.col += len(ins)
	}
}

// Newline splits the current line at the cursor.
func (in *Input) Newline() {
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
	if in.col > 0 {
		line := in.lines[in.row]
		in.lines[in.row] = append(line[:in.col-1], line[in.col:]...)
		in.col--
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
	line := in.lines[in.row]
	if in.col < len(line) {
		in.lines[in.row] = append(line[:in.col], line[in.col+1:]...)
		return
	}
	if in.row == len(in.lines)-1 {
		return
	}
	in.lines[in.row] = append(line, in.lines[in.row+1]...)
	in.lines = append(in.lines[:in.row+1], in.lines[in.row+2:]...)
}

// Left moves the cursor back one rune, wrapping to the previous line.
func (in *Input) Left() {
	if in.col > 0 {
		in.col--
	} else if in.row > 0 {
		in.row--
		in.col = len(in.lines[in.row])
	}
}

// Right moves the cursor forward one rune, wrapping to the next line.
func (in *Input) Right() {
	if in.col < len(in.lines[in.row]) {
		in.col++
	} else if in.row < len(in.lines)-1 {
		in.row++
		in.col = 0
	}
}

// Home moves to the start of the line; End to its end.
func (in *Input) Home() { in.col = 0 }
func (in *Input) End()  { in.col = len(in.lines[in.row]) }

// Up moves up a line, or on the first line recalls older history.
func (in *Input) Up() {
	if in.row > 0 {
		in.row--
		in.col = min(in.col, len(in.lines[in.row]))
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

// Down moves down a line, or on the last line recalls newer history and
// finally the draft that was being typed.
func (in *Input) Down() {
	if in.row < len(in.lines)-1 {
		in.row++
		in.col = min(in.col, len(in.lines[in.row]))
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

// Render lays the text out for width w (including a 2-cell "> " gutter),
// highlighting bytes past limit in red, and returns the rows plus the
// cursor's row and column. The limit applies to each line, or with joined
// to the lines joined by spaces (newline_mode = "flatten"). Masked text
// shows as bullets.
func (in *Input) Render(w, limit int, joined, masked bool) (rows []string, curRow, curCol int) {
	aw := max(1, w-2)
	total := 0 // bytes before this line when joined
	for li, line := range in.lines {
		var b strings.Builder
		col, bytes, red := 0, 0, false
		if joined {
			if li > 0 {
				total++ // the joining space
			}
			bytes = total
			if limit > 0 && bytes > limit {
				red = true
				b.WriteString(overLimit)
			}
		}
		flush := func() {
			if red {
				b.WriteString("\x1b[0m")
			}
			prefix := "  "
			if len(rows) == 0 {
				prefix = "> "
			}
			rows = append(rows, prefix+b.String())
			b.Reset()
			if red {
				b.WriteString(overLimit)
			}
			col = 0
		}
		for ri, r := range line {
			shown := string(r)
			if masked {
				shown = "•"
			}
			rw := xansi.StringWidth(shown)
			if col+rw > aw {
				flush()
			}
			if li == in.row && ri == in.col {
				curRow, curCol = len(rows), col
			}
			bytes += len(string(r))
			if !red && limit > 0 && bytes > limit {
				red = true
				b.WriteString(overLimit)
			}
			b.WriteString(shown)
			col += rw
		}
		if li == in.row && in.col == len(line) {
			if col >= aw && len(line) > 0 {
				flush()
			}
			curRow, curCol = len(rows), col
		}
		flush()
		total = bytes
	}
	return rows, curRow, curCol + 2
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
