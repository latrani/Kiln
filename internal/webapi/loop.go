package webapi

import (
	"context"
	"encoding/json"
	"time"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/feed"
	"github.com/latrani/Kiln/internal/str"
)

// statusTimeout is how long a status message stays, as in the TUI.
const statusTimeout = 30 * time.Second

// tickEvery is how often a turn runs with nothing else happening, so
// what changes with time alone (presence going away) reaches the page.
const tickEvery = 30 * time.Second

// Loop owns the core for one page. Everything that touches the core
// happens in Run, one message at a time; each message is one turn, and
// each turn is one batch of events for the page, to apply together.
type Loop struct {
	a             *app.App
	f             *feed.Feed
	in            chan any
	done          chan struct{} // closed when Run returns
	out           func([]byte)
	statusTimeout time.Duration
	timed         int // the status Gen last timed out
}

type tick struct{}

// expire takes down status Gen if it's still up.
type expire int

// New makes a loop over a that sends each turn's batch to out.
func New(a *app.App, out func([]byte)) *Loop {
	return &Loop{a: a, f: feed.New(a), in: make(chan any, 256), done: make(chan struct{}), out: out, statusTimeout: statusTimeout}
}

// Act hands the loop one action from the page, as JSON. It touches
// nothing of the core's; Run takes it in order. Once Run has returned it
// does nothing.
func (l *Loop) Act(js []byte) { l.post(js) }

// post puts msg on the loop, or drops it if the loop has ended.
func (l *Loop) post(msg any) {
	select {
	case l.in <- msg:
	case <-l.done:
	}
}

// Run drains the loop until ctx is done or the core quits. It first
// opens and connects autoconnect characters if asked, then sends
// everything.
func (l *Loop) Run(ctx context.Context, autoconnect bool) {
	defer close(l.done)
	var effs []app.Effect
	if autoconnect {
		for _, ch := range l.a.AllChars() {
			if ch.Autoconnect {
				k := app.Key(ch.World, ch.ID)
				if _, err := l.a.Open(k); err != nil {
					l.a.SetStatus(true, str.StatusCharError(k, err))
				}
				effs = append(effs, l.a.Connect(k)...)
			}
		}
	}
	l.perform(effs)
	l.send(anys(l.f.All()))
	t := time.NewTicker(tickEvery)
	defer t.Stop()
	for {
		var msg any
		select {
		case <-ctx.Done():
			l.a.Quit() // hang up every session
			return
		case <-t.C:
			msg = tick{}
		case msg = <-l.in:
		}
		extra, effs, all := l.step(msg)
		quit := l.perform(effs)
		l.send(append(l.events(effs, all), extra...))
		if quit {
			return
		}
	}
}

// events is a turn's events, or with all everything, followed either way
// by the turn's effects for the page.
func (l *Loop) events(effs []app.Effect, all bool) []any {
	if !all {
		return anys(l.f.Turn(effs))
	}
	evs := anys(l.f.All())
	for _, e := range effs {
		if _, ok := e.(app.Run); !ok {
			evs = append(evs, feed.Effect{Effect: e})
		}
	}
	return evs
}

func anys(evs []feed.Event) []any {
	out := make([]any, len(evs))
	for i, e := range evs {
		out[i] = e
	}
	return out
}

// perform runs the loop's own effects (Run, in a goroutine that only
// posts its result back) and reports whether one was Quit. The rest go
// to the page through the feed.
func (l *Loop) perform(effs []app.Effect) (quit bool) {
	for _, e := range effs {
		switch e := e.(type) {
		case app.Run:
			go func() { l.post(e.Func()) }()
		case app.Quit:
			quit = true
		}
	}
	return quit
}

func (l *Loop) send(evs []any) {
	b, err := Encode(evs)
	if err != nil {
		return // a value that won't marshal is a bug here, not news for the page
	}
	l.out(b)
	if st := l.a.Status(); st.Gen != l.timed && st.Text != "" { // a new status: time it out
		l.timed = st.Gen
		time.AfterFunc(l.statusTimeout, func() { l.post(expire(st.Gen)) })
	}
}

// step handles one message: the page's action, a Run's result, or a
// tick. extra are events the loop adds (settings results); all asks for
// everything instead of a turn.
func (l *Loop) step(msg any) (extra []any, effs []app.Effect, all bool) {
	switch m := msg.(type) {
	case []byte:
		var act Action
		if err := json.Unmarshal(m, &act); err != nil {
			l.a.SetStatus(true, str.StatusBadAction(err))
			return nil, nil, false
		}
		return l.do(act)
	case app.SessionMsg:
		_, _, effs := l.a.Handle(m)
		return nil, effs, false
	case app.OlderMsg:
		l.a.HandleOlder(m)
	case app.LogOlderMsg:
		return nil, l.a.HandleLogOlder(m), false
	case expire:
		if l.a.Status().Gen == int(m) {
			l.a.ClearStatus()
		}
	}
	return nil, nil, false
}

// OverLimit reports whether text is too long to send under a Char
// event's MaxLineBytes and NewlineMode: app.OverLimit, for the page to
// mark the overflow as you type.
func OverLimit(text string, maxLineBytes int, newlineMode string) bool {
	return app.OverLimit(text, config.Character{MaxLineBytes: maxLineBytes, NewlineMode: newlineMode})
}
