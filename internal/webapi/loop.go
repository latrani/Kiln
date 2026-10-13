package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"os"
	"sync"
	"time"
	"unicode/utf16"
	"unicode/utf8"

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
	mu            sync.Mutex
	queue         []any         // messages waiting for Run, in order; see post
	ended         bool          // Run has returned: post drops what comes
	wake          chan struct{} // nudges Run when queue was empty
	done          chan struct{} // closed when Run returns
	out           func([]byte)
	statusTimeout time.Duration
	timed         int               // the status Gen last timed out
	opts          Options           // as Run was given them
	drafts        map[string]string // saved drafts not yet restored, by key ("" for the idle input)
	onDisk        map[string]string // the drafts last written
	lk            looks             // the theme as last sent
}

// Options are how Run starts.
type Options struct {
	Autoconnect bool   // open and connect autoconnect characters first
	Drafts      string // the file drafts are saved to and restored from; "" for none
	ConfigDir   string // where themes are read from; "" for the built-in only
}

// say is a status for the loop to put up; see Say.
type say struct {
	err  bool
	text string
}

type tick struct{}

// expire takes down status Gen if it's still up.
type expire int

// New makes a loop over a that sends each turn's batch to out.
func New(a *app.App, out func([]byte)) *Loop {
	return &Loop{a: a, f: feed.New(a), wake: make(chan struct{}, 1), done: make(chan struct{}), out: out, statusTimeout: statusTimeout}
}

// Act hands the loop one action from the page, as JSON. It touches
// nothing of the core's; Run takes it in order. Once Run has returned it
// does nothing.
func (l *Loop) Act(js []byte) { l.post(js) }

// Say puts text on the status line from outside the loop (the page's
// storage failing, say). Like Act, it never touches the core itself.
func (l *Loop) Say(isErr bool, text string) { l.post(say{isErr, text}) }

// post puts msg on the loop, or drops it if the loop has ended. It never
// blocks: the queue has no limit, so a JS callback calling Act (even
// from inside a batch the loop is handing the page) can't wait on the
// loop that called it.
func (l *Loop) post(msg any) {
	l.mu.Lock()
	if l.ended {
		l.mu.Unlock()
		return
	}
	l.queue = append(l.queue, msg)
	l.mu.Unlock()
	select {
	case l.wake <- struct{}{}:
	default: // already nudged
	}
}

// take is the next queued message; false if there's none.
func (l *Loop) take() (any, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.queue) == 0 {
		return nil, false
	}
	msg := l.queue[0]
	l.queue[0] = nil
	l.queue = l.queue[1:]
	return msg, true
}

// Run drains the loop until ctx is done or the core quits. It first
// loads saved drafts, opens and connects autoconnect characters if
// asked, then sends everything.
func (l *Loop) Run(ctx context.Context, opts Options) {
	defer func() {
		l.mu.Lock()
		l.ended, l.queue = true, nil
		l.mu.Unlock()
		close(l.done)
	}()
	l.opts = opts
	l.drafts = loadDrafts(opts.Drafts)
	l.onDisk = maps.Clone(l.drafts)
	var effs []app.Effect
	if opts.Autoconnect {
		for _, ch := range l.a.AllChars() {
			if ch.Autoconnect {
				k := app.Key(ch.World, ch.ID)
				l.open(k)
				effs = append(effs, l.a.Connect(k)...)
			}
		}
	}
	if d := l.drafts[""]; d != "" && l.a.Char(l.a.Active()) == nil {
		l.a.SetInput(d)
		delete(l.drafts, "")
	}
	l.perform(effs)
	l.send(l.events(nil, true))
	t := time.NewTicker(tickEvery)
	defer t.Stop()
	for {
		msg, ok := l.take()
		for !ok {
			select {
			case <-ctx.Done():
				l.a.Quit() // hang up every session
				return
			case <-t.C:
				msg, ok = tick{}, true
			case <-l.wake:
				msg, ok = l.take()
			}
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
// by the turn's effects for the page. The theme goes first when the
// config changed it, and with all right after Start.
func (l *Loop) events(effs []app.Effect, all bool) []any {
	themed := l.restyle()
	if !all {
		evs := anys(l.f.Turn(effs))
		if themed {
			evs = append([]any{l.lk.ev}, evs...)
		}
		return evs
	}
	evs := anys(l.f.All())
	evs = append(evs[:1], append([]any{l.lk.ev}, evs[1:]...)...) // after Start
	for _, e := range effs {
		if _, ok := e.(app.Run); !ok {
			evs = append(evs, feed.Effect{Effect: e})
		}
	}
	return evs
}

// restyle loads the theme again if the config changed since it was
// last loaded, saying on the status line why a theme or a character's
// looks didn't load, as the TUI does. It reports whether the theme the
// page has changed.
func (l *Loop) restyle() bool {
	if l.lk.loaded && l.a.Config() == l.lk.cfg {
		return false
	}
	changed, restyle, err := l.lk.load(l.opts.ConfigDir, l.a.Config(), l.a.AllChars())
	for _, k := range restyle {
		l.f.Restyle(k)
	}
	for _, k := range l.a.Order() {
		if err := l.lk.lookErrs[k]; err != nil {
			l.a.SetStatus(true, str.StatusCharError(k, err))
		}
	}
	if err != nil {
		l.a.SetStatus(true, str.StatusThemeNotLoaded(err))
	}
	return changed
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
	b, err := Encode(evs, l.lk.highlighter)
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
		d := json.NewDecoder(bytes.NewReader(m))
		d.DisallowUnknownFields()
		err := json.Unmarshal(m, new(json.RawMessage)) // one JSON value, nothing after it
		if err == nil {
			err = d.Decode(&act)
		}
		if err != nil {
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
	case say:
		l.a.SetStatus(m.err, m.text)
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

// OverRanges is where text runs past a Char event's MaxLineBytes, for
// the page to mark as the TUI's input does: [start, end) in UTF-16 code
// units (the browser's string offsets), from the character that takes a
// line past the limit to the line's end. With newline_mode "flatten" the
// lines count as one joined by spaces, so once past, the rest is over.
// nil when nothing is.
func OverRanges(text string, maxLineBytes int, newlineMode string) [][2]int {
	if maxLineBytes <= 0 {
		return nil
	}
	joined := newlineMode == "flatten" //str:ok
	var out [][2]int
	bytes, at, start := 0, 0, -1 // start: where the open range began
	for _, r := range text {
		if r == '\n' {
			if joined {
				bytes++ // the joining space
			} else {
				if start >= 0 {
					out = append(out, [2]int{start, at})
				}
				bytes, start = 0, -1
			}
			at++
			continue
		}
		bytes += utf8.RuneLen(r)
		if start < 0 && bytes > maxLineBytes {
			start = at
		}
		at += utf16.RuneLen(r)
	}
	if start >= 0 {
		out = append(out, [2]int{start, at})
	}
	return out
}

// CheckField is what to say about a settings field (a config key) as
// value is typed, "" when it's fine: app.CheckField, for the page's live
// hints.
func CheckField(field, value string) string {
	if err := app.CheckField(field, value); err != nil {
		return err.Error()
	}
	return ""
}

// open opens character k, saying why on the status line if it couldn't
// load, and restores its saved draft.
func (l *Loop) open(k string) {
	was := l.a.Char(k) != nil
	c, err := l.a.Open(k)
	if c != nil && err != nil {
		l.a.SetStatus(true, str.StatusCharError(k, err))
	} else if lerr := l.lk.lookErrs[k]; c != nil && !was && lerr != nil { // said once, as the TUI does
		l.a.SetStatus(true, str.StatusCharError(k, lerr))
	}
	if d := l.drafts[k]; c != nil && d != "" {
		l.a.SetDraft(k, d)
		delete(l.drafts, k)
	}
}

// loadDrafts reads the drafts file; a missing or unreadable one is none.
func loadDrafts(path string) map[string]string {
	drafts := map[string]string{}
	if path == "" {
		return drafts
	}
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &drafts) != nil {
		return map[string]string{}
	}
	return drafts
}

// saveDrafts writes the drafts to the drafts file and syncs it, which
// in the browser saves it to IndexedDB at once: every open input's draft
// (never a password at the prompt), and the saved drafts not yet
// restored. A torn file is ignored on the next start (loadDrafts). Best
// effort: a failure here has nobody to tell.
func (l *Loop) saveDrafts() {
	if l.opts.Drafts == "" {
		return
	}
	drafts := maps.Clone(l.drafts)
	for _, k := range l.a.Order() {
		if d := l.a.Draft(k); d != "" {
			drafts[k] = d
		} else {
			delete(drafts, k)
		}
	}
	if t := l.a.IdleText(); t != "" {
		drafts[""] = t
	}
	l.onDisk = drafts
	b, err := json.Marshal(drafts)
	if err != nil {
		return
	}
	f, err := os.Create(l.opts.Drafts)
	if err != nil {
		return
	}
	if _, err := f.Write(b); err == nil {
		f.Sync()
	}
	f.Close()
}
