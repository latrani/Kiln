package feed

import (
	"time"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/session"
)

// Event is one change for a page to apply.
type Event interface{ event() }

// Worlds is the config's worlds and their characters, for the picker.
type Worlds struct{ Worlds []World }

// World is one world in Worlds.
type World struct {
	ID         string
	Characters []Character
}

// Character is one character in a World.
type Character struct{ ID, Name string }

// Order is the open characters' keys, in sidebar order.
type Order struct{ Keys []string }

// Char is one open character: what the sidebar shows of it, and its
// input as the core holds it.
type Char struct {
	Key, World, ID, Name string
	State                session.State
	Unread               int
	Attention            bool
	CertChanged          bool      // its server's certificate changed; /trust to accept
	Orphan               bool      // removed from the config; closes when it disconnects
	ConnectedAt          time.Time // when the current connection came up
	Text                 string    // the input's text, as the core holds it
	Prompt               string    // the server's unterminated prompt, sanitized; "" for none
	NeedPW               bool      // the password prompt is up
	More                 bool      // older history can be paged in
	Loading              bool      // a page of older history is being read
}

// Idle is the input used while nothing is open.
type Idle struct{ Text string }

// Presence is whether you're here, away, or not known yet. It can also
// change with time alone (notify_idle passing), which a loop catches by
// running a turn on a timer.
type Presence struct{ State app.Presence }

// Closed is an open character gone.
type Closed struct{ Key string }

// Active is what's showing: a character's key, app.WorldSel(world) for a
// world's overview, or "".
type Active struct{ Key string }

// Status is the status line. Gen goes up with each new message.
type Status struct {
	Text string
	Err  bool
	Gen  int
}

// Confirm says whether an over-limit line is waiting for Enter again.
type Confirm struct{ On bool }

// SaveQuestion is the "save this password?" question, while Asking.
// Store is where it would go (password_store).
type SaveQuestion struct {
	World, Char, Store string
	Asking             bool
}

// Op is how a Lines or LogLines event changes the page's lines.
type Op int

const (
	Reset   Op = iota // these are all the lines now
	Replace           // the same lines (same numbers), made again: their tags changed
	Append            // these go after the last
	Prepend           // these go before the first
)

// Lines changes a character's scrollback. Epoch names the numbering the
// lines belong to: a Reset starts a new one (the character reopened, its
// history loaded again), so line N in an older epoch isn't line N now.
type Lines struct {
	Key   string
	Epoch int
	Op    Op
	Lines []app.Line
}

// Log is a character's log mode. Lines are named by number: the marked
// range, the lines taken out of it, the lines the filter (or local_echo)
// hides, and the find's matches, oldest first.
type Log struct {
	Key                          string
	Open                         bool
	Find                         string
	Status                       string
	StatusErr                    bool
	Searching, Loading, HistDone bool
	Start, End                   *int
	Selected                     int // lines an export or copy would take
	Excluded, Hidden, Matches    []int
}

// LogLines changes a character's log-mode lines; Epoch as in Lines.
type LogLines struct {
	Key   string
	Epoch int
	Op    Op
	Lines []app.Line
}

// Filter is log mode's filter panel list.
type Filter struct {
	Key     string
	Entries []FilterEntry
}

// FilterEntry is one item in Filter, with its buttons' state: Hidden and
// Only lit, Folded for a parent folded shut, FilteredUnder for a folded
// parent with something under it hidden or Only.
type FilterEntry struct {
	app.PanelEntry
	Hidden, Only, Folded, FilteredUnder bool
}

// Effect is something the core asks the page to do.
type Effect struct{ app.Effect }

func (Worlds) event()       {}
func (Order) event()        {}
func (Char) event()         {}
func (Closed) event()       {}
func (Idle) event()         {}
func (Presence) event()     {}
func (Active) event()       {}
func (Status) event()       {}
func (Confirm) event()      {}
func (SaveQuestion) event() {}
func (Lines) event()        {}
func (Log) event()          {}
func (LogLines) event()     {}
func (Filter) event()       {}
func (Effect) event()       {}
