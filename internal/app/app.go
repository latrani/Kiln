package app

import (
	"cmp"
	"context"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/session"
)

// Deps are the core's connections to the outside world. Tests substitute
// fakes; each front end wires the real ones.
type Deps struct {
	LogRoot  string // where log_dir is relative to; "" (and no absolute log_dir): no logs to read
	Dial     func(ctx context.Context, ch config.Character) (session.LineConn, error)
	NewLog   func(l logstore.Layout) session.Appender // l from LogLayout
	Password func(store, world, char string) (string, error)
	Now      func() time.Time
}

// Char is one open character. Front ends read its fields; only App
// changes them.
type Char struct {
	Key         string
	Ch          config.Character
	Sess        *session.Session
	State       session.State
	Rules       Rules
	Unread      int
	Attention   bool
	Pin         *conn.PinMismatchError
	Orphan      bool      // removed from the config; closed when it disconnects
	ConnectedAt time.Time // when the current connection came up
	cancel      context.CancelFunc
}

// compile builds c's rules from its configuration. On an error c keeps
// the rules it had.
func (c *Char) compile() error {
	cls, err := classify.New(c.Ch.Rules.Classify, c.Ch.Name, c.Ch.Aliases)
	if err != nil {
		return err
	}
	c.Rules = Rules{Classifier: cls, Judge: rules.Judge{Attention: c.Ch.Rules.Attention, Quiet: c.Ch.Rules.Quiet}}
	return nil
}

// Status is the statusline's message. Gen goes up with each new one, so
// a front end can time out the one it saw.
type Status struct {
	Text string
	Err  bool
	Gen  int
}

// App is what Kiln decides. It isn't safe for concurrent use: it runs on
// its front end's loop.
type App struct {
	d            Deps
	cfg          *config.Config
	chars        map[string]*Char
	order        []string // open characters, in sidebar order
	active       string   // a character's key, WorldSel(world), or ""
	recent       []string // open characters by when last active, most recent first; not the active one
	status       Status
	pwStore      atomic.Pointer[string] // password_store; sessions read it off the loop
	paneW, paneH int                    // where server text is shown; 0×0 until known
}

// Key is a character's key: its world and id.
func Key(world, char string) string { return world + "/" + char }

// WorldSel is what Active is while world's overview shows.
func WorldSel(world string) string { return "=" + world }

// CompareChars orders characters alphabetically, ignoring case: by world
// id, then by name.
func CompareChars(a, b config.Character) int {
	return cmp.Or(
		cmp.Compare(strings.ToLower(a.World), strings.ToLower(b.World)),
		cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
		cmp.Compare(Key(a.World, a.ID), Key(b.World, b.ID)),
	)
}

// New makes an App with nothing open and no config yet; ApplyConfig
// gives it one.
func New(d Deps) *App {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &App{d: d, chars: map[string]*Char{}}
}

// ConfigResult is what ApplyConfig did to the open characters.
type ConfigResult struct {
	Closed []string         // gone from the config and not connected, in sidebar order
	Errs   map[string]error // rules that didn't compile, by key; those characters keep their old ones
}

// ApplyConfig takes cfg and updates the open characters to match it. An
// open character that vanished from the config stays as an orphan while
// it's connected, until it next disconnects; otherwise it closes.
func (a *App) ApplyConfig(cfg *config.Config) ConfigResult {
	a.cfg = cfg
	store := cfg.PasswordStore
	a.pwStore.Store(&store)
	res := ConfigResult{Errs: map[string]error{}}
	for _, k := range slices.Clone(a.order) {
		c := a.chars[k]
		ch, ok := a.Find(k)
		if !ok {
			if c.Sess != nil && c.State != session.Disconnected && c.State != session.Failed {
				c.Orphan = true
			} else {
				a.Close(k)
				res.Closed = append(res.Closed, k)
			}
			continue
		}
		c.Ch, c.Orphan = ch, false
		if err := c.compile(); err != nil {
			res.Errs[k] = err
		}
		if c.Sess != nil {
			c.Sess.SetChar(ch)
		}
	}
	a.sortOrder() // names may have changed
	return res
}

// Config is the config last applied; nil before the first.
func (a *App) Config() *config.Config { return a.cfg }

// Find looks up a configured character by key.
func (a *App) Find(k string) (config.Character, bool) {
	if a.cfg != nil {
		for _, w := range a.cfg.Worlds {
			for _, ch := range w.Characters {
				if Key(ch.World, ch.ID) == k {
					return ch, true
				}
			}
		}
	}
	return config.Character{}, false
}

// AllChars is every configured character, in sidebar order.
func (a *App) AllChars() []config.Character {
	var all []config.Character
	if a.cfg != nil {
		for _, w := range a.cfg.Worlds {
			all = append(all, w.Characters...)
		}
	}
	slices.SortFunc(all, CompareChars)
	return all
}

func (a *App) sortOrder() {
	slices.SortFunc(a.order, func(x, y string) int { return CompareChars(a.chars[x].Ch, a.chars[y].Ch) })
}

// Open adds configured character k to the open ones, in sidebar order,
// and returns it, with the error if its rules didn't compile. An open
// character is returned as it is. It returns nil if k isn't configured.
// It doesn't connect. With nothing active, k becomes active.
func (a *App) Open(k string) (*Char, error) {
	if c := a.chars[k]; c != nil {
		return c, nil
	}
	ch, ok := a.Find(k)
	if !ok {
		return nil, nil
	}
	c := &Char{Key: k, Ch: ch}
	err := c.compile()
	a.chars[k] = c
	a.order = append(a.order, k)
	a.sortOrder()
	if a.active == "" {
		a.active = k
	}
	return c, err
}

// Close stops an open character's session and closes it. If it was
// active, or its world's overview was and it was the world's last
// character, the next one down becomes active, or the one above if it
// was last.
func (a *App) Close(k string) {
	i := slices.Index(a.order, k)
	if i < 0 {
		return
	}
	if c := a.chars[k]; c.cancel != nil {
		c.cancel()
	}
	delete(a.chars, k)
	a.order = slices.Delete(a.order, i, i+1)
	a.recent = slices.DeleteFunc(a.recent, func(r string) bool { return r == k })
	if w, ok := a.ActiveWorld(); ok && !a.WorldOpen(w) {
		k = a.active // its last character closed: the world's row goes too
	}
	if a.active != k {
		return
	}
	a.active = ""
	if len(a.order) > 0 {
		a.Switch(a.order[min(i, len(a.order)-1)])
	}
}

// Char is open character k; nil if it isn't open.
func (a *App) Char(k string) *Char { return a.chars[k] }

// Order is the open characters' keys in sidebar order. Don't change it.
func (a *App) Order() []string { return a.order }

// Active is the active sidebar item: a character's key, WorldSel(world)
// for a world's overview, or "" with nothing open.
func (a *App) Active() string { return a.active }

// ActiveWorld is the world whose overview is showing, when its sidebar
// row is the active one instead of a character.
func (a *App) ActiveWorld() (string, bool) { return strings.CutPrefix(a.active, WorldSel("")) }

// WorldOpen reports whether any of world's characters are open.
func (a *App) WorldOpen(world string) bool {
	return slices.ContainsFunc(a.order, func(k string) bool { return a.chars[k].Ch.World == world })
}

// CanSwitch reports whether Switch(k) would: k is an open character, or
// the overview of a world with one open.
func (a *App) CanSwitch(k string) bool {
	if a.chars[k] != nil {
		return true
	}
	w, isWorld := strings.CutPrefix(k, WorldSel(""))
	return isWorld && a.WorldOpen(w)
}

// Switch makes k active: a character's key, or WorldSel(world) for its
// overview. It reports false, changing nothing, if it can't (CanSwitch).
// Switching to a character clears its unread and attention; the
// character switched away from goes first in line for UnreadTarget.
func (a *App) Switch(k string) bool {
	if !a.CanSwitch(k) {
		return false
	}
	if a.chars[a.active] != nil && a.active != k {
		a.recent = slices.DeleteFunc(a.recent, func(r string) bool { return r == a.active })
		a.recent = append([]string{a.active}, a.recent...)
	}
	a.active = k
	if c := a.chars[k]; c != nil {
		a.recent = slices.DeleteFunc(a.recent, func(r string) bool { return r == k })
		c.Unread, c.Attention = 0, false
	}
	return true
}

// Stops are what stepping through the sidebar visits: each world with a
// character open, then its characters, in sidebar order, as values
// Active takes.
func (a *App) Stops() []string {
	var s []string
	last := ""
	for _, k := range a.order {
		if w := a.chars[k].Ch.World; w != last {
			last = w
			s = append(s, WorldSel(w))
		}
		s = append(s, k)
	}
	return s
}

// StepTarget is where stepping delta stops through Stops from the active
// one lands, wrapping around; "" with nothing open.
func (a *App) StepTarget(delta int) string {
	stops := a.Stops()
	if len(stops) == 0 {
		return ""
	}
	i := max(0, slices.Index(stops, a.active))
	return stops[(i+delta%len(stops)+len(stops))%len(stops)]
}

// UnreadTarget is the next character (dir 1) or previous one (dir -1) in
// sidebar order with unread lines, wrapping around. With nothing unread
// it's the character active before this one, so switching to it flips
// between the last two, like switching apps; "" when there's nowhere to go.
func (a *App) UnreadTarget(dir int) string {
	n := len(a.order)
	i := slices.Index(a.order, a.active) // -1 when no character is active
	for step := 1; step <= n; step++ {
		k := a.order[((i+dir*step)%n+n)%n]
		if k != a.active && a.chars[k].Unread > 0 {
			return k
		}
	}
	if len(a.recent) > 0 {
		return a.recent[0]
	}
	if n > 1 { // others are open but none has been active yet
		return a.order[((i+dir)%n+n)%n]
	}
	return ""
}

// SetStatus puts text on the statusline, as an error if isErr.
func (a *App) SetStatus(isErr bool, text string) {
	a.status = Status{Text: text, Err: isErr, Gen: a.status.Gen + 1}
}

// ClearStatus takes the message down. It isn't a new message, so Gen
// stays.
func (a *App) ClearStatus() { a.status.Text = "" }

// Status is the statusline's message.
func (a *App) Status() Status { return a.status }

// PasswordStore is the password_store setting. It's safe to call off the
// loop.
func (a *App) PasswordStore() string {
	if p := a.pwStore.Load(); p != nil && *p != "" {
		return *p
	}
	return config.DefaultPasswordStore
}

// LogLayout is where ch's logs go; ok is false when there's nowhere.
func (a *App) LogLayout(ch config.Character) (l logstore.Layout, ok bool) {
	l = logstore.Layout{Root: a.d.LogRoot, Dir: a.cfg.LogDir, Name: a.cfg.LogName,
		World: ch.World, Char: ch.ID, CharName: ch.Name}
	return l, a.d.LogRoot != "" || filepath.IsAbs(a.cfg.LogDir)
}
