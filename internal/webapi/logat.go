package webapi

import "github.com/latrani/Kiln/internal/app"

// LogAt says where a find, a step (n, N) or go-to-date landed in log
// mode, for the page to show: line Seq. It can come a turn or more after
// the action, once older days are read.
type LogAt struct {
	Key string
	Seq int
}

// ExportName is the file name an export in Format starts with: no
// directory, since the browser decides where downloads go.
type ExportName struct{ Key, Format, Name string }

// land is the callback for where k's log mode lands: it queues a LogAt
// for the turn it happens in (see Run).
func (l *Loop) land(k string) func(*app.LogLine) {
	return func(ln *app.LogLine) { l.landed = append(l.landed, LogAt{Key: k, Seq: ln.Seq}) }
}

// takeLanded is the LogAt events queued since the last turn was sent.
func (l *Loop) takeLanded() []any {
	out := l.landed
	l.landed = nil
	return out
}
