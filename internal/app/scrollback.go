package app

import (
	"strings"
	"time"

	"github.com/latrani/Kiln/internal/history"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
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
	c.History.SetLogged(nil)
	c.sentHist, c.sentLoading = nil, false
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
	// What was sent on the days read goes in the input's history; older
	// days are read for it as its panel needs them (moreSent).
	c.History.SetLogged(sentOf(entries, session.IsLogin(c.Ch)))
	if !hist.Exhausted() {
		c.sentHist = hist.Clone()
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
	c.seq.numberOlder(c.Lines)
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
	c.seq.numberOlder(msg.lines)
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
	l.Seq = c.seq.newer()
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

// sentOf is what es records as sent, oldest first, but login lines, each
// as it was typed: a line sent starting with "/" was typed with "//".
func sentOf(es []logstore.Entry, isLogin func(string) bool) []Sent {
	var out []Sent
	for _, e := range es {
		if e.Dir == logstore.Out && !isLogin(e.Text) {
			text := e.Text
			if strings.HasPrefix(text, "/") {
				text = "/" + text
			}
			out = append(out, Sent{Text: text, At: e.Time})
		}
	}
	return out
}

// sentAhead is how near the oldest line the history panel's selection
// gets before an older log day is read for it.
const sentAhead = 50

// SentOlderMsg is an older log day's sent lines, for the history panel,
// as a Run from moreSent returns it; HandleSentOlder takes it.
type SentOlderMsg struct {
	Key  string
	hist *history.Reader // the reader that was asked; stale if it has changed
	sent []Sent
}

// moreSent starts reading an older log day for the active character's
// history panel, off the loop, if the panel is up, its selection is near
// the oldest line, and older days are still to read.
func (a *App) moreSent() []Effect {
	if c := a.chars[a.active]; c == nil || c.History.Sel() >= sentAhead {
		return nil
	}
	return a.HistoryMore()
}

// HistoryMore starts reading an older log day for the active character's
// history panel, as moreSent but wherever its selection is: the page's
// panel scrolled to its top. In session scope the panel lists no logged
// lines, so there's nothing to read.
func (a *App) HistoryMore() []Effect {
	c := a.chars[a.active]
	if c == nil || !c.History.IsOpen() || c.sentHist == nil || c.sentLoading || a.sent.scope == ScopeSession {
		return nil
	}
	c.sentLoading = true
	h, k, isLogin := c.sentHist, c.Key, session.IsLogin(c.Ch)
	return []Effect{Run{Func: func() any {
		msg := SentOlderMsg{Key: k, hist: h}
		if es, _, ok, err := h.LoadOlder(); ok && err == nil {
			msg.sent = sentOf(es, isLogin)
		}
		return msg
	}}}
}

// HandleSentOlder puts a day moreSent read before the rest of its
// character's history, and reads on while the panel still wants more.
func (a *App) HandleSentOlder(msg SentOlderMsg) []Effect {
	c := a.chars[msg.Key]
	if c == nil || c.sentHist != msg.hist {
		return nil
	}
	c.History.AddOlder(msg.sent)
	c.sentLoading = false
	if msg.hist.Exhausted() {
		c.sentHist = nil
	}
	if msg.Key != a.active {
		return nil
	}
	return a.moreSent()
}
