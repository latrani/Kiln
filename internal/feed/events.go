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

// Character is one character in a World: aliases are what the picker's
// filter matches besides its name and world.
type Character struct {
	ID, Name string
	Aliases  []string `json:",omitempty"`
}

// Order is the open characters' keys, in sidebar order.
type Order struct{ Keys []string }

// Char is one open character: what the sidebar shows of it, and its
// input as the core holds it.
type Char struct {
	Key, World, ID, Name string
	State                session.State
	Unread               int
	Priority             bool
	CertChanged          bool      // its server's certificate changed; /trust to accept
	Orphan               bool      // removed from the config; closes when it disconnects
	ConnectedAt          time.Time `json:",omitzero"` // when the current connection came up
	ConnectedSince       string    // the statusline's "Connected since …"; "" unless connected
	LastSeen             string    // when its last line came, as people read it; "" for none
	MaxLineBytes         int       // max_line_bytes
	NewlineMode          string    // newline_mode
	More                 bool      // older history can be paged in
	Loading              bool      // a page of older history is being read
}

// Input is one input as the core holds it: a character's (Key), or with
// Key "" the input used while nothing is open. What the page sent is
// never echoed back (see Feed.Told). Ack is the last of the page's
// numbered input actions the core has taken for it: the page takes Text
// only when Ack is the newest it sent, so a reply to an older action
// never overwrites newer typing.
type Input struct {
	Key, Text string
	Ack       int
	Prompt    string // the server's unterminated prompt, sanitized; "" for none
	NeedPW    bool   // the password prompt is up
}

// History is the active input's history panel (Key "" for the idle
// input's): while Open, the sent lines it lists, oldest first, and the
// selected one's index (-1 for none). Query is what they're filtered by.
type History struct {
	Key   string
	Open  bool
	Query string
	Rows  []HistoryRow
	Sel   int
}

// HistoryRow is one line in the history panel, and when it was sent, as
// people read it.
type HistoryRow struct{ Text, Clock string }

// Start begins everything (All): the page drops what it holds first.
type Start struct{}

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
	Find                         string // "" with find off
	LastFind                     string // the term find last had, for the next to start with
	Status                       string
	StatusErr                    bool
	Searching, Loading, HistDone bool
	Start, End                   *int
	Selected                     int // lines an export or copy would take
	Excluded, Hidden, Matches    []int
	ExportFormat                 string // config's export_format: the format export picks on Enter; "" asks
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
func (Input) event()        {}
func (History) event()      {}
func (Start) event()        {}
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
