package app

import (
	"time"

	"github.com/latrani/Kiln/internal/history"
	"github.com/latrani/Kiln/internal/logstore"
)

// HistoryLines is how many logged lines are preloaded per character.
const HistoryLines = 200

// OlderMsg is a page of older history, as a Run from RequestOlder returns
// it; HandleOlder takes it.
type OlderMsg struct {
	Key   string
	hist  *history.Reader // the reader that was asked; stale if it has changed
	gen   int             // the rules generation the page was made under
	lines []*Line
	more  bool
}

func pointers(ls []Line) []*Line {
	out := make([]*Line, len(ls))
	for i := range ls {
		out[i] = &ls[i]
	}
	return out
}

// Preload replaces k's lines with the tail of its most recent log days,
// and keeps everything older to page in on demand, so scrollback is
// unlimited. Open does this.
func (a *App) Preload(k string) {
	c := a.chars[k]
	if c == nil {
		return
	}
	c.Lines, c.More, c.Loading, c.hist, c.leftover = nil, false, false, nil, nil
	l, ok := a.LogLayout(c.Ch)
	if !ok {
		return
	}
	hist, err := history.NewReader(l)
	if err != nil {
		return
	}
	var entries []logstore.Entry
	for len(entries) < HistoryLines {
		es, _, ok, err := hist.LoadOlder()
		if !ok || err != nil {
			break
		}
		entries = append(es, entries...)
	}
	if len(entries) == 0 {
		return
	}
	// entries holds whole days. Keep the newest HistoryLines; the rest
	// (the start of the oldest day, plus any fuller days before it) is
	// the first page RequestOlder hands over.
	var leftover []logstore.Entry
	if len(entries) > HistoryLines {
		leftover = entries[:len(entries)-HistoryLines]
		entries = entries[len(entries)-HistoryLines:]
	}
	// The preload starts a day only if nothing of that day was left over.
	startsDay := len(leftover) == 0 || DayOf(leftover[len(leftover)-1].Time) != DayOf(entries[0].Time)
	c.Lines = pointers(c.Rules.Days(entries, c.Ch.LocalEcho, startsDay))
	c.Lines = append(c.Lines, &Line{Kind: HistoryEnd, Entry: logstore.Entry{Time: entries[len(entries)-1].Time}})
	c.hist, c.leftover = hist, leftover
	c.More = leftover != nil || !hist.Exhausted()
}

// RequestOlder starts reading k's next older page of history off the
// loop: first the preload's leftover, then one log day per read. It
// returns nothing when there's nothing more to read or a read is already
// in flight. HandleOlder takes what the Run returns.
func (a *App) RequestOlder(k string) []Effect {
	c := a.chars[k]
	if c == nil || c.hist == nil || !c.More || c.Loading {
		return nil
	}
	c.Loading = true
	h, r, echo, leftover, gen := c.hist, c.Rules, c.Ch.LocalEcho, c.leftover, c.rulesGen
	c.leftover = nil
	return []Effect{Run{Func: func() any {
		msg := OlderMsg{Key: k, hist: h, gen: gen}
		if leftover != nil {
			msg.lines, msg.more = pointers(r.Days(leftover, echo, true)), !h.Exhausted()
			return msg
		}
		if es, _, ok, err := h.LoadOlder(); ok && err == nil {
			msg.lines, msg.more = pointers(r.Days(es, echo, true)), !h.Exhausted()
		}
		return msg
	}}}
}

// HandleOlder puts a page RequestOlder read above k's lines and returns
// it, for the front end to show. ok is false for a page from a reader
// that has since been replaced. A page read under rules that have since
// changed is made again under the current ones.
func (a *App) HandleOlder(msg OlderMsg) (lines []*Line, ok bool) {
	c := a.chars[msg.Key]
	if c == nil || c.hist != msg.hist {
		return nil, false
	}
	if msg.gen != c.rulesGen {
		for _, l := range msg.lines {
			*l = c.Rules.Reline(*l)
		}
	}
	c.Lines = append(msg.lines, c.Lines...)
	c.More, c.Loading = msg.more, false
	return msg.lines, true
}

// Echo adds a line you sent to k's lines, if its local_echo shows sent
// lines, and returns it; nil if it doesn't show.
func (a *App) Echo(k string, e logstore.Entry) *Line {
	c := a.chars[k]
	if c == nil || !c.shows(e) {
		return nil
	}
	return c.add(c.Rules.Line(e))
}

// shows reports whether e belongs in the scrollback: everything but sent
// lines, which only show with local_echo on. They're logged either way.
func (c *Char) shows(e logstore.Entry) bool { return e.Dir != logstore.Out || c.Ch.LocalEcho }

// add appends l to the scrollback. The server has moved on from any
// prompt.
func (c *Char) add(l Line) *Line {
	c.Lines = append(c.Lines, &l)
	c.Prompt = ""
	return &l
}

// LastTime is when the newest line that arrived for k was logged: from
// the server, or Kiln's own notes (connected, closed); what you sent
// doesn't count. false when there's none.
func (a *App) LastTime(k string) (time.Time, bool) {
	if c := a.chars[k]; c != nil {
		for i := len(c.Lines) - 1; i >= 0; i-- {
			if l := c.Lines[i]; l.Kind == Server || l.Kind == Sys {
				return l.Entry.Time, true
			}
		}
	}
	return time.Time{}, false
}
