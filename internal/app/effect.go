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
// (/notify, /away, /backup, /restore). Key is the active character's, or
// "" with none.
type Do struct {
	Cmd  string
	Args []string
	Key  string
}

func (Do) effect()   {}
func (Run) effect()  {}
func (Quit) effect() {}
