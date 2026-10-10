package app

// Effect is something the front end does for the core: each front end
// performs them its own way.
type Effect interface{ effect() }

// Run is blocking work for the front end to do off its loop: Func's
// result goes back to the core (a SessionMsg goes to Handle).
type Run struct{ Func func() any }

// Quit ends the program.
type Quit struct{}

// Do is a command for the front end to carry out: one about its own
// screens (/open, /log, /edit), or one the core doesn't handle yet
// (/backup, /restore). Key is the active character's, or "" with none.
type Do struct {
	Cmd  string
	Args []string
	Key  string
}

// Notify is a notification to show. Title is the character's name (with
// @world when needed); Body is the line's text as it arrived, server
// colors and all, for the front end to clean up for its medium.
type Notify struct{ Title, Body string }

// Copy puts Text on the clipboard.
type Copy struct{ Text string }

// SaveFile offers Data as a file: in a browser a download named after
// Name's last element; in the terminal a file at Name (relative to
// export_dir, "~/" expanded), never overwriting one. Key is the
// character whose log mode asked, for where to say how it went.
type SaveFile struct {
	Key, Name string
	Data      []byte
}

func (Do) effect()       {}
func (Run) effect()      {}
func (Quit) effect()     {}
func (Notify) effect()   {}
func (Copy) effect()     {}
func (SaveFile) effect() {}
