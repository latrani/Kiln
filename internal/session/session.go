// Package session runs one character's connection: dialing, auto-login,
// logging, and reconnecting with backoff.
package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"kiln/internal/config"
	"kiln/internal/conn"
	"kiln/internal/logstore"
)

// State is a session's connection state.
type State int

const (
	Disconnected State = iota
	Connecting
	Connected
	Failed // stopped retrying (e.g. certificate changed); waits for Reconnect
)

func (s State) String() string {
	return [...]string{"disconnected", "connecting", "connected", "failed"}[s]
}

// EventKind says which fields of an Event are set.
type EventKind int

const (
	EventLine     EventKind = iota // Entry is set (already logged)
	EventState                     // State is set; Err is set for Failed
	EventLogError                  // Err is set: writing the log failed
)

// Event is something the UI should hear about.
type Event struct {
	Kind  EventKind
	Entry logstore.Entry
	State State
	Err   error
}

// LineConn is the part of *conn.Conn a session uses. Lines must be closed
// when the connection ends, including after Close.
type LineConn interface {
	Lines() <-chan string
	Err() error
	Send(line string) error
	Close() error
}

// Appender is where entries are logged. It must be safe for concurrent
// use: Send logs from the caller's goroutine while Run logs from its own.
type Appender interface {
	Append(logstore.Entry) error
}

// Options configures a Session. Dial, Log and Char are required.
type Options struct {
	Char     config.Character
	Dial     func(ctx context.Context) (LineConn, error)
	Log      Appender
	Password func() (string, error)          // nil: no password available
	Now      func() time.Time                // default time.Now
	Backoff  func(attempt int) time.Duration // default DefaultBackoff
}

// DefaultBackoff is 1s, 2s, 4s, … capped at 60s.
func DefaultBackoff(attempt int) time.Duration {
	d := time.Second << min(attempt, 6)
	return min(d, 60*time.Second)
}

// ErrNotConnected is returned by Send while no connection is open.
var ErrNotConnected = errors.New("not connected")

// LogError is returned by Send when the line was sent but could not be logged.
type LogError struct{ Err error }

func (e *LogError) Error() string { return "sent, but not logged: " + e.Err.Error() }
func (e *LogError) Unwrap() error { return e.Err }

// Session is one character's connection lifecycle.
type Session struct {
	o      Options
	events chan Event
	kick   chan struct{}
	done   <-chan struct{} // Run's ctx.Done(); only Run's goroutine reads it

	mu       sync.Mutex
	c        LineConn
	quitting bool // user sent QUIT on the current connection
}

// New returns a Session; call Run to start it.
func New(o Options) *Session {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Backoff == nil {
		o.Backoff = DefaultBackoff
	}
	return &Session{o: o, events: make(chan Event, 256), kick: make(chan struct{}, 1)}
}

// Events delivers session events. It is closed when Run returns.
func (s *Session) Events() <-chan Event { return s.events }

// Reconnect skips the current backoff wait, or resumes a Failed session.
func (s *Session) Reconnect() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// QuitCommand is the MUCK command that ends a connection on purpose.
// When the server hangs up after it, the session does not reconnect.
const QuitCommand = "QUIT"

// Send sends one line to the server and logs it, returning the logged
// entry for the caller to display. It never emits an event, so it cannot
// block behind a backlogged Events channel. If the line was sent but not
// logged, the error is a *LogError.
func (s *Session) Send(line string) (logstore.Entry, error) {
	s.mu.Lock()
	c := s.c
	s.mu.Unlock()
	if c == nil {
		return logstore.Entry{}, ErrNotConnected
	}
	if err := c.Send(line); err != nil {
		return logstore.Entry{}, err
	}
	if strings.TrimSpace(line) == QuitCommand { // Fuzzball matches QUIT case-sensitively
		s.mu.Lock()
		s.quitting = true
		s.mu.Unlock()
	}
	e := logstore.Entry{Time: s.o.Now(), Dir: logstore.Out, Text: line}
	if err := s.o.Log.Append(e); err != nil {
		return e, &LogError{Err: err}
	}
	return e, nil
}

// Run connects and keeps reconnecting until ctx is cancelled.
func (s *Session) Run(ctx context.Context) {
	s.done = ctx.Done()
	defer close(s.events)
	attempt := 0
	for ctx.Err() == nil {
		s.state(Connecting, nil)
		c, err := s.o.Dial(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			var pin *conn.PinMismatchError
			if errors.As(err, &pin) {
				s.sys("connect failed: " + err.Error())
				s.state(Failed, err)
				if !s.wait(ctx, -1) {
					return
				}
				continue
			}
			delay := s.o.Backoff(attempt)
			attempt++
			s.sys(fmt.Sprintf("connect failed: %v; retrying in %s", err, delay))
			s.state(Disconnected, err)
			if !s.wait(ctx, delay) {
				return
			}
			continue
		}

		attempt = 0
		s.setConn(c)
		s.sys(fmt.Sprintf("connected to %s:%d", s.o.Char.Host, s.o.Char.Port))
		s.state(Connected, nil)
		s.login(c)
		s.pump(ctx, c)
		s.setConn(nil)
		if ctx.Err() != nil {
			return
		}

		s.mu.Lock()
		quit := s.quitting
		s.quitting = false
		s.mu.Unlock()
		if quit {
			s.sys("disconnected (quit)")
			s.state(Disconnected, nil)
			if !s.wait(ctx, -1) { // stay down until Reconnect
				return
			}
			continue
		}

		reason := "closed by server"
		if err := c.Err(); err != nil {
			reason = err.Error()
		}
		delay := s.o.Backoff(attempt)
		attempt++
		s.sys(fmt.Sprintf("disconnected (%s); retrying in %s", reason, delay))
		s.state(Disconnected, c.Err())
		if !s.wait(ctx, delay) {
			return
		}
	}
}

// pump forwards lines until the connection ends or ctx is cancelled.
func (s *Session) pump(ctx context.Context, c LineConn) {
	for {
		select {
		case <-ctx.Done():
			c.Close()
			for range c.Lines() { // drain so the reader goroutine exits
			}
			return
		case line, ok := <-c.Lines():
			if !ok {
				return
			}
			s.logLine(logstore.In, line)
		}
	}
}

// login sends the auto-login line. The password is substituted only into
// what goes over the wire; the log gets the template with "***".
func (s *Session) login(c LineConn) {
	tmpl := s.o.Char.Login
	if tmpl == "" {
		return
	}
	withName := strings.ReplaceAll(tmpl, "{name}", s.o.Char.Name)
	wire := withName
	if strings.Contains(tmpl, "{password}") {
		var pw string
		var err error
		if s.o.Password != nil {
			pw, err = s.o.Password()
		}
		if s.o.Password == nil || err != nil || pw == "" {
			s.sys(fmt.Sprintf("no saved password for %s/%s; skipping auto-login (run: kiln passwd %s %s)",
				s.o.Char.World, s.o.Char.ID, s.o.Char.World, s.o.Char.ID))
			return
		}
		wire = strings.ReplaceAll(withName, "{password}", pw)
	}
	if err := c.Send(wire); err != nil {
		return // the read side will notice the dead connection
	}
	s.logLine(logstore.Out, strings.ReplaceAll(withName, "{password}", "***"))
}

// wait sleeps for d (forever if d < 0) or until Reconnect or ctx ends.
// It reports false if ctx ended.
func (s *Session) wait(ctx context.Context, d time.Duration) bool {
	var timer <-chan time.Time
	if d >= 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		timer = t.C
	}
	select {
	case <-ctx.Done():
		return false
	case <-s.kick:
		return true
	case <-timer:
		return true
	}
}

func (s *Session) setConn(c LineConn) {
	s.mu.Lock()
	s.c = c
	s.mu.Unlock()
}

// logLine logs first, then tells the UI, so a UI failure never loses data.
// Only Run's goroutine calls it.
func (s *Session) logLine(d logstore.Dir, text string) {
	e := logstore.Entry{Time: s.o.Now(), Dir: d, Text: text}
	if err := s.o.Log.Append(e); err != nil {
		s.emit(Event{Kind: EventLogError, Err: err})
	}
	s.emit(Event{Kind: EventLine, Entry: e})
}

func (s *Session) sys(text string) { s.logLine(logstore.Sys, text) }

func (s *Session) state(st State, err error) {
	s.emit(Event{Kind: EventState, State: st, Err: err})
}

// emit delivers an event, giving up if Run is being cancelled so a consumer
// that stopped reading can never wedge shutdown.
func (s *Session) emit(ev Event) {
	select {
	case s.events <- ev:
	case <-s.done:
	}
}
