package app

// History is one input's sent lines, oldest first, and where browsing
// them has got to. The zero value is empty and not browsing.
type History struct {
	lines []string
	pos   int    // the line shown while browsing; len(lines) = not browsing
	draft string // what was being typed when browsing started
}

// Add records v, unless it's empty or the same as the last line, and
// stops browsing.
func (h *History) Add(v string) {
	if v != "" && (len(h.lines) == 0 || h.lines[len(h.lines)-1] != v) {
		h.lines = append(h.lines, v)
	}
	h.Stop()
}

// Older is the line before the one shown, for Up at the top of the input.
// cur is the input's text: starting to browse keeps it as the draft that
// Newer brings back. ok is false when there's nothing older.
func (h *History) Older(cur string) (string, bool) {
	if h.pos == 0 || len(h.lines) == 0 {
		return "", false
	}
	if h.pos == len(h.lines) {
		h.draft = cur
	}
	h.pos--
	return h.lines[h.pos], true
}

// Newer is the line after the one shown, and past the newest, the draft.
// ok is false when not browsing.
func (h *History) Newer() (string, bool) {
	if h.pos >= len(h.lines) {
		return "", false
	}
	h.pos++
	if h.pos == len(h.lines) {
		return h.draft, true
	}
	return h.lines[h.pos], true
}

// Stop stops browsing: the input has been cleared or sent.
func (h *History) Stop() { h.pos = len(h.lines) }

// Pos is the line being shown while browsing; len(Lines()) when not.
func (h *History) Pos() int { return h.pos }

// Lines is the history, oldest first. Don't change it.
func (h *History) Lines() []string { return h.lines }
