package app

import (
	"context"
	"errors"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
)

// SessionMsg is a session's next event, as a Run from Connect or Handle
// returns it. OK is false once the session has shut down.
type SessionMsg struct {
	Key  string
	Sess *session.Session
	Ev   session.Event
	OK   bool
}

// Event is what Handle made of a session event, for the front end's part.
type Event struct {
	Key    string
	Ev     session.Event
	Line   *Line // for an EventLine, what Kiln made of its entry
	Shown  bool  // the line went into the scrollback (Lines)
	Closed bool  // the character was an orphan and has closed
}

// wait is the Run that waits for s's next event.
func wait(k string, s *session.Session) Effect {
	return Run{Func: func() any {
		ev, ok := <-s.Events()
		return SessionMsg{Key: k, Sess: s, Ev: ev, OK: ok}
	}}
}

// Connect starts k's session, or restarts the one it has (and then there
// is nothing new to wait for). The Run it returns waits for the session's
// first event.
func (a *App) Connect(k string) []Effect {
	c := a.chars[k]
	if c == nil {
		return nil
	}
	if c.Sess != nil {
		c.Sess.Reconnect()
		return nil
	}
	l, _ := a.LogLayout(c.Ch)
	var s *session.Session
	s = session.New(session.Options{
		Char: c.Ch,
		Log:  a.d.NewLog(l),
		Dial: func(ctx context.Context) (session.LineConn, error) { return a.d.Dial(ctx, s.Char()) },
		Password: func() (string, error) {
			ch := s.Char()
			return a.d.Password(a.PasswordStore(), ch.World, ch.ID)
		},
	})
	if a.paneW > 0 {
		s.Resize(a.paneW, a.paneH)
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.Sess, c.cancel = s, cancel
	c.State = session.Connecting // until the session says otherwise
	go s.Run(ctx)
	return []Effect{wait(k, s)}
}

// Handle takes a session's event. ok is false for one from a session
// that has since been replaced or has shut down: there's nothing to do.
// Otherwise the effects wait for the session's next event, unless the
// event closed the character.
func (a *App) Handle(msg SessionMsg) (ev Event, ok bool, effs []Effect) {
	c := a.chars[msg.Key]
	if c == nil || c.Sess != msg.Sess || !msg.OK {
		return Event{}, false, nil
	}
	ev = Event{Key: msg.Key, Ev: msg.Ev}
	var note []Effect
	switch msg.Ev.Kind {
	case session.EventLine:
		l := c.Rules.Line(msg.Ev.Entry)
		ev.Line = &l
		if c.shows(msg.Ev.Entry) {
			ev.Line, ev.Shown = c.add(l), true
		}
		if msg.Key != a.active && msg.Ev.Entry.Dir == logstore.In && !ev.Line.Quiet {
			c.Unread++
			c.Attention = c.Attention || ev.Line.Attention
		}
		if c.Log != nil {
			c.Log.appendLive(msg.Ev.Entry)
		}
		note = a.notifyFor(c, *ev.Line)
	case session.EventState:
		c.State = msg.Ev.State
		if c.State == session.Connected {
			c.ConnectedAt = a.d.Now()
		}
		if c.Orphan && (c.State == session.Disconnected || c.State == session.Failed) {
			a.Close(msg.Key) // don't reconnect (and log in) a deleted character
			ev.Closed = true
			return ev, true, nil
		}
		var pin *conn.PinMismatchError
		if c.State == session.Failed && errors.As(msg.Ev.Err, &pin) {
			c.Pin = pin
			a.SetStatus(true, str.StatusCertChanged(c.Ch.Name))
		}
		if c.State == session.Connected {
			c.Pin = nil
		}
		if c.State != session.Connected && c.NeedPW {
			c.endPassword()
		}
	case session.EventPrompt:
		c.Prompt = ansi.Sanitize(msg.Ev.Entry.Text)
	case session.EventNeedPassword:
		c.startPassword()
	case session.EventLogError:
		a.SetStatus(true, str.StatusLogWriteFailed(c.Ch.Name, msg.Ev.Err))
	}
	return ev, true, append([]Effect{wait(msg.Key, msg.Sess)}, note...)
}

// Quit stops every session and ends the program.
func (a *App) Quit() []Effect {
	for _, c := range a.chars {
		if c.cancel != nil {
			c.cancel()
		}
	}
	return []Effect{Quit{}}
}

// SetPane records the size of the pane server text shows in, for
// sessions that connect from now on. ReportSize tells the open ones.
func (a *App) SetPane(w, h int) { a.paneW, a.paneH = w, h }

// ReportSize tells every session the pane's size. Sessions only send it
// to servers that negotiated NAWS.
func (a *App) ReportSize() {
	for _, c := range a.chars {
		if c.Sess != nil {
			c.Sess.Resize(a.paneW, a.paneH)
		}
	}
}
