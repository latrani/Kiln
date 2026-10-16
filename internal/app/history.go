package app

import (
	"strings"
	"time"
)

// Sent is a line sent from an input, and when.
type Sent struct {
	Text string
	At   time.Time
}

// Scope is which lines the history panels list: an input's own
// (ScopeCharacter), or every input's since Kiln started (ScopeSession).
type Scope int

const (
	ScopeCharacter Scope = iota
	ScopeSession
)

// sentLog is every line sent from any input since Kiln started, oldest
// first, and the panels' scope. gen goes up with each change, so a panel
// knows to work out what it shows again.
type sentLog struct {
	lines []Sent
	scope Scope
	gen   int
}

// History is one input's sent lines, and its history panel: a list of
// them, filtered by what's typed (the query), with one selected. The
// zero value is empty and closed.
type History struct {
	logged []Sent   // read from the logs, oldest first
	run    []Sent   // sent since Kiln started, oldest first
	all    *sentLog // every input's, shared; nil for none
	allGen int      // all.gen when shown was worked out
	open   bool     // the panel is up
	query  string   // the input's text while the panel is up
	draft  string   // the input's text when the panel opened
	back   int      // the selection, counted back from the newest shown
	shown  []Sent   // Shown's answer
	stale  bool     // shown needs working out again
}

// Add records v, sent at at, unless it's empty, in this input's lines
// and every input's, in each unless it's the same as that one's last.
func (h *History) Add(v string, at time.Time) {
	if v == "" {
		return
	}
	if len(h.run) == 0 || h.run[len(h.run)-1].Text != v {
		h.run = append(h.run, Sent{Text: v, At: at})
		h.stale = true
	}
	if l := h.all; l != nil && (len(l.lines) == 0 || l.lines[len(l.lines)-1].Text != v) {
		l.lines = append(l.lines, Sent{Text: v, At: at})
		l.gen++
	}
}

// SetLogged replaces what was read from the logs with ss, oldest first.
func (h *History) SetLogged(ss []Sent) {
	h.logged = ss
	h.stale = true
}

// AddOlder puts ss, read from older logs, before the rest.
func (h *History) AddOlder(ss []Sent) {
	if len(ss) == 0 {
		return
	}
	h.logged = append(append([]Sent(nil), ss...), h.logged...)
	h.stale = true
}

// Lines is every line, oldest first, repeats and all.
func (h *History) Lines() []string {
	out := make([]string, 0, len(h.logged)+len(h.run))
	for _, s := range h.logged {
		out = append(out, s.Text)
	}
	for _, s := range h.run {
		out = append(out, s.Text)
	}
	return out
}

// Open puts the panel up over draft, the input's text, which starts as
// the query. The newest line that matches is selected.
func (h *History) Open(draft string) {
	h.open, h.draft, h.query, h.back = true, draft, draft, 0
	h.stale = true
}

// IsOpen reports whether the panel is up.
func (h *History) IsOpen() bool { return h.open }

// Close takes the panel down and returns the draft it opened over.
func (h *History) Close() string {
	d := h.draft
	h.open, h.query, h.draft, h.back, h.shown = false, "", "", 0, nil
	return d
}

// SetQuery filters the panel by q, selecting the newest match.
func (h *History) SetQuery(q string) {
	if q != h.query {
		h.query, h.back, h.stale = q, 0, true
	}
}

// Query is what the panel is filtered by.
func (h *History) Query() string { return h.query }

// Shown is what the panel lists, oldest first: the lines that contain
// the query (ignoring case), each only at its newest. Don't change it.
func (h *History) Shown() []Sent {
	if !h.open {
		return nil
	}
	if h.shown != nil && !h.stale && (h.all == nil || h.all.gen == h.allGen) {
		return h.shown
	}
	parts := [][]Sent{h.run, h.logged}
	if h.all != nil {
		h.allGen = h.all.gen
		if h.all.scope == ScopeSession {
			parts = [][]Sent{h.all.lines}
		}
	}
	q := strings.ToLower(h.query)
	seen := map[string]bool{}
	h.shown = []Sent{}
	for _, part := range parts {
		for i := len(part) - 1; i >= 0; i-- {
			s := part[i]
			if seen[s.Text] || !strings.Contains(strings.ToLower(s.Text), q) {
				continue
			}
			seen[s.Text] = true
			h.shown = append(h.shown, s)
		}
	}
	for i, j := 0, len(h.shown)-1; i < j; i, j = i+1, j-1 {
		h.shown[i], h.shown[j] = h.shown[j], h.shown[i]
	}
	h.back = min(h.back, max(0, len(h.shown)-1))
	h.stale = false
	return h.shown
}

// Sel is the index in Shown of the selected line; -1 when nothing's
// shown.
func (h *History) Sel() int { return len(h.Shown()) - 1 - h.back }

// Select selects Shown()[i].
func (h *History) Select(i int) {
	if n := len(h.Shown()); i >= 0 && i < n {
		h.back = n - 1 - i
	}
}

// Move moves the selection delta lines newer (negative: older), stopping
// at the oldest shown. Past the newest it closes the panel: closed is
// true and draft is what it opened over.
func (h *History) Move(delta int) (draft string, closed bool) {
	n := len(h.Shown())
	if h.back-delta < 0 {
		return h.Close(), true
	}
	h.back = min(h.back-delta, max(0, n-1))
	return "", false
}

// Pick closes the panel and returns the selected line; ok is false, and
// the panel stays up, when nothing's shown.
func (h *History) Pick() (string, bool) {
	ss := h.Shown()
	if len(ss) == 0 {
		return "", false
	}
	v := ss[len(ss)-1-h.back].Text
	h.Close()
	return v, true
}
