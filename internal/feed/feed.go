// Package feed tells a page what changed. After each turn of a front
// end's loop it compares what the page shows with what it last sent and
// returns events for the differences, so no core code has to remember to
// announce a change. The TUI redraws from the core instead and doesn't
// use it.
package feed

import (
	"reflect"
	"slices"

	"github.com/latrani/Kiln/internal/app"
)

// Identities of per-character small things in Feed.small: never shown.
const (
	idChar   = "char "   //str:ok: a map key
	idLog    = "log "    //str:ok: a map key
	idFilter = "filter " //str:ok: a map key
)

// Feed is one page's view of the core. Like the core, it isn't safe for
// concurrent use: it runs on the front end's loop.
type Feed struct {
	a     *app.App
	small map[string]Event // the small things as last sent, by identity
	lines map[string]*sent // each character's scrollback as sent
	logs  map[string]*sent // each character's log-mode lines as sent
}

// sent is how far a list of lines has been sent.
type sent struct {
	owner  any // the *app.Char or *app.Log they belong to
	gen    int // its rules generation
	lo, hi int // the lowest and highest numbers sent
	any    bool
}

// New starts a feed over a; its first Turn sends everything.
func New(a *app.App) *Feed {
	f := &Feed{a: a}
	f.forget()
	return f
}

func (f *Feed) forget() {
	f.small, f.lines, f.logs = map[string]Event{}, map[string]*sent{}, map[string]*sent{}
}

// All forgets what was sent and returns everything.
func (f *Feed) All() []Event {
	f.forget()
	return f.Turn(nil)
}

// Since is open character k's scrollback after line seq.
func (f *Feed) Since(k string, seq int) []app.Line {
	c := f.a.Char(k)
	if c == nil {
		return nil
	}
	var out []app.Line
	for i := len(c.Lines) - 1; i >= 0 && c.Lines[i].Seq > seq; i-- {
		out = append(out, *c.Lines[i])
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
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
	put("order", Order{Keys: append([]string(nil), a.Order()...)})
	open := map[string]bool{}
	for _, k := range a.Order() {
		open[k] = true
		put(idChar+k, char(a.Char(k)))
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
		delete(f.small, idLog+k)
		delete(f.small, idFilter+k)
		delete(f.lines, k)
		delete(f.logs, k)
		evs = append(evs, Closed{Key: k})
	}
	put("active", Active{Key: a.Active()})
	st := a.Status()
	put("status", Status{Text: st.Text, Err: st.Err, Gen: st.Gen})
	put("confirm", Confirm{On: a.Confirming()})
	w, ch, asking := a.PendingSave()
	put("save", SaveQuestion{World: w, Char: ch, Asking: asking})
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

func worlds(a *app.App) Worlds {
	var out Worlds
	if cfg := a.Config(); cfg != nil {
		for _, w := range cfg.Worlds {
			ws := World{ID: w.ID}
			for _, c := range w.Characters {
				ws.Characters = append(ws.Characters, Character{ID: c.ID, Name: c.Name})
			}
			out.Worlds = append(out.Worlds, ws)
		}
	}
	return out
}

func char(c *app.Char) Char {
	return Char{
		Key: c.Key, World: c.Ch.World, ID: c.Ch.ID, Name: c.Ch.Name,
		State: c.State, Unread: c.Unread, Attention: c.Attention,
		CertChanged: c.Pin != nil, Orphan: c.Orphan,
		Text: c.Text, Prompt: c.Prompt, NeedPW: c.NeedPW, More: c.More, Loading: c.Loading,
	}
}

// scrollback is Task 2's; logMode is Task 3's.
func (f *Feed) scrollback(*app.Char) []Event { return nil }

func (f *Feed) logMode(*app.Char, func(string, Event)) []Event { return nil }
