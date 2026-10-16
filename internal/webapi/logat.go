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

// LineTags is what log mode's status line says of line Seq's tags (see
// app.LineTags), for the page's cursor; "" for none.
type LineTags struct {
	Key  string
	Seq  int
	Text string
}

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

// takeLogStatus moves a message the showing character's log mode just
// set to the status line, timed as any status, as the TUI's.
func (l *Loop) takeLogStatus() {
	c := l.a.Char(l.a.Active())
	if c == nil || c.Log == nil || c.Log.Status == "" {
		return
	}
	l.a.SetStatus(c.Log.StatusErr, c.Log.Status)
	l.logStatus = l.a.Status().Gen
	c.Log.ClearStatus()
}

// clearLogStatus drops log mode's message when you act in log mode
// again, as the TUI's. A newer message (log mode's or anyone's) stays:
// it's news.
func (l *Loop) clearLogStatus() {
	if l.logStatus != 0 && l.a.Status().Gen == l.logStatus {
		l.a.ClearStatus()
	}
	l.logStatus = 0
}
