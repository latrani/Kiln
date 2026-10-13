// Package feed tells a page what changed. After each turn of a front
// end's loop it compares what the page shows with what it last sent and
// returns events for the differences, so no core code has to remember to
// announce a change. The TUI redraws from the core instead and doesn't
// use it.
package feed

import (
	"reflect"
	"slices"
	"strings"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/session"
)

// Identities of per-character small things in Feed.small: never shown.
const (
	idChar   = "char "   //str:ok: a map key
	idLog    = "log "    //str:ok: a map key
	idFilter = "filter " //str:ok: a map key
	idInput  = "input "  //str:ok: a map key
)

// Feed is one page's view of the core. Like the core, it isn't safe for
// concurrent use: it runs on the front end's loop.
type Feed struct {
	a     *app.App
	small map[string]Event // the small things as last sent, by identity
	lines map[string]*sent // each character's scrollback as sent
	logs  map[string]*sent // each character's log-mode lines as sent
	epoch int              // the last numbering handed out; see Lines.Epoch
	acks  map[string]int   // each input's Ack; see Told
}

// sent is how far a list of lines has been sent.
type sent struct {
	owner  any // the *app.Char or *app.Log they belong to
	gen    int // its rules generation
	lo, hi int // the lowest and highest numbers sent
	any    bool
	epoch  int // the numbering they were sent in
}

// New starts a feed over a; its first Turn sends everything.
func New(a *app.App) *Feed {
	f := &Feed{a: a, acks: map[string]int{}}
	f.forget()
	return f
}

func (f *Feed) forget() {
	f.small, f.lines, f.logs = map[string]Event{}, map[string]*sent{}, map[string]*sent{}
}

// All forgets what was sent and returns everything, starting with
// Start: the page drops what it holds.
func (f *Feed) All() []Event {
	f.forget()
	return append([]Event{Start{}}, f.Turn(nil)...)
}

func (f *Feed) input(k string) Input {
	if c := f.a.Char(k); c != nil {
		return Input{Key: k, Text: f.a.Draft(k), Ack: f.acks[k], Prompt: c.Prompt, NeedPW: c.NeedPW}
	}
	return Input{Text: f.a.IdleText(), Ack: f.acks[""]}
}

// Told records that the page set key's input text itself ("" for the
// idle input) in its action numbered ack, so the next turn doesn't send
// it back.
func (f *Feed) Told(key string, ack int) {
	f.acks[key] = ack
	f.small[idInput+key] = f.input(key)
}

// Restyle sends character key's scrollback and log-mode lines again on
// the next turn, as a Replace: the front end's styling of them changed
// (which tags have styles), though the lines didn't.
func (f *Feed) Restyle(key string) {
	for _, m := range []map[string]*sent{f.lines, f.logs} {
		if s := m[key]; s != nil {
			s.gen = -1 // no generation the core hands out
		}
	}
}

// Resend sends key's input on the next turn even if it hasn't changed:
// the page is waiting to hear its action was taken.
func (f *Feed) Resend(key string) { delete(f.small, idInput+key) }

// Since is open character k's scrollback after line seq of numbering
// epoch, as last sent. ok is false if k's numbering has since restarted,
// seq is older than any line the core holds, or k isn't open: the page
// has to take everything again (All).
func (f *Feed) Since(k string, epoch, seq int) (lines []app.Line, ok bool) {
	c, s := f.a.Char(k), f.lines[k]
	if c == nil || s == nil || s.epoch != epoch {
		return nil, false
	}
	if len(c.Lines) > 0 && c.Lines[0].Seq > seq+1 { // the page would miss what's between
		return nil, false
	}
	var out []app.Line
	for i := len(c.Lines) - 1; i >= 0 && c.Lines[i].Seq > seq; i-- {
		out = append(out, *c.Lines[i])
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, true
}

// Turn returns what changed since the last turn, then effs (all but
// app.Run, which is the loop's own).
func (f *Feed) Turn(effs []app.Effect) []Event {
	var evs []Event
	put := func(id string, e Event) {
		if old, ok := f.small[id]; !ok || !reflect.DeepEqual(old, e) {
			f.small[id] = e
			evs = append(evs, e)
		}
	}
	a := f.a
	put("worlds", worlds(a))
	put("order", Order{Keys: append([]string{}, a.Order()...)})
	open := map[string]bool{}
	for _, k := range a.Order() {
		open[k] = true
		put(idChar+k, char(a, a.Char(k)))
		put(idInput+k, f.input(k))
	}
	var gone []string
	for _, e := range f.small {
		if c, ok := e.(Char); ok && !open[c.Key] {
			gone = append(gone, c.Key)
		}
	}
	slices.Sort(gone)
	for _, k := range gone {
		delete(f.small, idChar+k)
		delete(f.small, idInput+k)
		delete(f.acks, k)
		delete(f.small, idLog+k)
		delete(f.small, idFilter+k)
		delete(f.lines, k)
		delete(f.logs, k)
		evs = append(evs, Closed{Key: k})
	}
	put(idInput, f.input(""))
	put("presence", Presence{State: a.Presence()})
	put("active", Active{Key: a.Active()})
	st := a.Status()
	put("status", Status{Text: st.Text, Err: st.Err, Gen: st.Gen})
	put("confirm", Confirm{On: a.Confirming()})
	w, ch, asking := a.PendingSave()
	put("save", SaveQuestion{World: w, Char: ch, Store: a.PasswordStore(), Asking: asking})
	for _, k := range a.Order() {
		evs = append(evs, f.scrollback(a.Char(k))...)
	}
	for _, k := range a.Order() {
		more := f.logMode(a.Char(k), put) // put appends the Log event first
		evs = append(evs, more...)
	}
	for _, e := range effs {
		if _, ok := e.(app.Run); !ok {
			evs = append(evs, Effect{e})
		}
	}
	return evs
}

// worlds is every configured world and its characters, in the picker's
// order: worlds by id, characters by name, ignoring case.
func worlds(a *app.App) Worlds {
	var out Worlds
	if cfg := a.Config(); cfg != nil {
		for _, w := range cfg.Worlds {
			ws := World{ID: w.ID}
			chars := slices.Clone(w.Characters)
			slices.SortFunc(chars, app.CompareChars)
			for _, c := range chars {
				ws.Characters = append(ws.Characters, Character{ID: c.ID, Name: c.Name, Aliases: c.Aliases})
			}
			out.Worlds = append(out.Worlds, ws)
		}
	}
	slices.SortFunc(out.Worlds, func(x, y World) int { return strings.Compare(strings.ToLower(x.ID), strings.ToLower(y.ID)) })
	return out
}

func char(a *app.App, c *app.Char) Char {
	ch := Char{
		Key: c.Key, World: c.Ch.World, ID: c.Ch.ID, Name: c.Ch.Name,
		State: c.State, Unread: c.Unread, Priority: c.Priority,
		CertChanged: c.Pin != nil, Orphan: c.Orphan, ConnectedAt: c.ConnectedAt,
		MaxLineBytes: c.Ch.MaxLineBytes, NewlineMode: c.Ch.NewlineMode, More: c.More, Loading: c.Loading,
	}
	now := a.Now()
	if c.State == session.Connected && !c.ConnectedAt.IsZero() {
		ch.ConnectedSince = app.ConnectedSince(c.ConnectedAt, now)
	}
	if t, ok := a.LastTime(c.Key); ok {
		ch.LastSeen = app.Clock(t, now)
	}
	return ch
}

// scrollback is c's Lines event, if its lines changed.
func (f *Feed) scrollback(c *app.Char) []Event {
	op, ls, ok := f.diff(f.lines, c.Key, c, c.Relined(), len(c.Lines), func(i int) *app.Line { return c.Lines[i] })
	if !ok {
		return nil
	}
	return []Event{Lines{Key: c.Key, Epoch: f.lines[c.Key].epoch, Op: op, Lines: ls}}
}

// diff works out how key's lines (n of them, at(i) the i'th, numbered in
// order) changed since they were last sent, and records them as sent.
// A new owner, or lines gone from either end, is a Reset; a new rules
// generation a Replace; otherwise new lines at the start or end are a
// Prepend or an Append. Only the ends are looked at. ok is false when
// nothing changed.
func (f *Feed) diff(m map[string]*sent, key string, owner any, gen, n int, at func(int) *app.Line) (Op, []app.Line, bool) {
	s := m[key]
	all := func() []app.Line {
		out := make([]app.Line, n)
		for i := range n {
			out[i] = *at(i)
		}
		return out
	}
	epoch := 0
	if s != nil {
		epoch = s.epoch
	}
	record := func(reset bool) {
		if reset {
			f.epoch++
			epoch = f.epoch
		}
		ns := sent{owner: owner, gen: gen, any: n > 0, epoch: epoch}
		if n > 0 {
			ns.lo, ns.hi = at(0).Seq, at(n-1).Seq
		}
		m[key] = &ns
	}
	if s == nil || s.owner != owner {
		record(true)
		return Reset, all(), true
	}
	if n == 0 {
		if !s.any {
			return 0, nil, false
		}
		record(true)
		return Reset, nil, true
	}
	lo, hi := at(0).Seq, at(n-1).Seq
	switch {
	case !s.any: // the first lines: added to nothing
		record(false)
		return Append, all(), true
	case lo > s.lo || hi < s.hi:
		record(true)
		return Reset, all(), true
	case gen != s.gen && lo == s.lo && hi == s.hi:
		record(false)
		return Replace, all(), true
	case gen != s.gen: // remade, and an end moved too: start over
		record(true)
		return Reset, all(), true
	case lo < s.lo && hi > s.hi:
		record(true) // both ends grew in one turn: rare; one Reset says it simply
		return Reset, all(), true
	case lo < s.lo:
		var out []app.Line
		for i := 0; i < n && at(i).Seq < s.lo; i++ {
			out = append(out, *at(i))
		}
		record(false)
		return Prepend, out, true
	case hi > s.hi:
		i := n
		for i > 0 && at(i-1).Seq > s.hi {
			i--
		}
		out := make([]app.Line, 0, n-i)
		for ; i < n; i++ {
			out = append(out, *at(i))
		}
		record(false)
		return Append, out, true
	}
	return 0, nil, false
}

// logMode is c's log-mode events: Log (through put), then LogLines and
// Filter while it's open. Closing it sends Log with Open false once.
func (f *Feed) logMode(c *app.Char, put func(string, Event)) []Event {
	g := c.Log
	if g == nil {
		delete(f.logs, c.Key)
		delete(f.small, idFilter+c.Key)
		if _, ok := f.small[idLog+c.Key]; ok {
			put(idLog+c.Key, Log{Key: c.Key})
		}
		return nil
	}
	items := g.Items() // sync the filter first, as the TUI does
	put(idLog+c.Key, logState(c.Key, g))
	var evs []Event
	if op, ls, ok := f.diff(f.logs, c.Key, g, 0, len(g.Lines), func(i int) *app.Line { return &g.Lines[i].Line }); ok {
		evs = append(evs, LogLines{Key: c.Key, Epoch: f.logs[c.Key].epoch, Op: op, Lines: ls})
	}
	only, hasOnly := c.Filter.Only()
	fl := Filter{Key: c.Key}
	for _, e := range g.EntriesOf(items) {
		fl.Entries = append(fl.Entries, FilterEntry{
			PanelEntry: e, Hidden: c.Filter.Hidden(e.Item), Only: hasOnly && only == e.Item,
			Folded:        e.Parent && c.Collapsed[e.Item.Name],
			FilteredUnder: e.Parent && c.Collapsed[e.Item.Name] && g.FilteredUnder(e.Item.Name, items),
		})
	}
	if old, ok := f.small[idFilter+c.Key]; !ok || !reflect.DeepEqual(old, fl) {
		f.small[idFilter+c.Key] = fl
		evs = append(evs, fl)
	}
	return evs
}

func logState(k string, g *app.Log) Log {
	l := Log{Key: k, Open: true, Find: g.Find, Status: g.Status, StatusErr: g.StatusErr,
		Searching: g.Searching, Loading: g.Loading, HistDone: g.HistDone, Selected: len(g.Selection())}
	if g.Start != nil {
		s := g.Start.Seq
		l.Start = &s
	}
	if g.End != nil {
		e := g.End.Seq
		l.End = &e
	}
	for _, ln := range g.Lines {
		if g.Excluded[ln] {
			l.Excluded = append(l.Excluded, ln.Seq)
		}
		if !g.Shows(ln) {
			l.Hidden = append(l.Hidden, ln.Seq)
		}
	}
	for _, m := range g.Matches() {
		l.Matches = append(l.Matches, m.Seq)
	}
	return l
}
