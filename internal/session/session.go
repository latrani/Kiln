// Package session runs one character's connection: dialing, auto-login,
// logging, and reconnecting with backoff.
package session

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/logstore"
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
	EventLine         EventKind = iota // Entry is set (already logged)
	EventState                         // State is set; Err is set for Failed
	EventLogError                      // Err is set: writing the log failed
	EventPrompt                        // Entry.Text is an unterminated prompt; not logged
	EventNeedPassword                  // auto-login needs a password; call Login
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

// prompter is implemented by connections that report unterminated
// prompts (*conn.Conn does).
type prompter interface {
	Prompts() <-chan string
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
	quitting bool // user sent QUIT (or Disconnect) on the current connection
	char     config.Character
	redact   *regexp.Regexp // matches typed login lines; nil if no template
}

// New returns a Session; call Run to start it.
func New(o Options) *Session {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Backoff == nil {
		o.Backoff = DefaultBackoff
	}
	s := &Session{o: o, events: make(chan Event, 256), kick: make(chan struct{}, 1)}
	s.SetChar(o.Char)
	return s
}

// Char returns the character's current configuration.
func (s *Session) Char() config.Character {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.char
}

// SetChar replaces the character's configuration (e.g. after a config
// reload). It takes effect for the next login and log redaction.
func (s *Session) SetChar(ch config.Character) {
	re := loginPattern(ch.Login)
	s.mu.Lock()
	s.char, s.redact = ch, re
	s.mu.Unlock()
}

// loginPattern turns a login template like "connect {name} {password}"
// into a regexp whose first group captures the password in a typed line.
// Literal words match case-insensitively; {name} matches any one word.
func loginPattern(tmpl string) *regexp.Regexp {
	if !strings.Contains(tmpl, "{password}") {
		return nil
	}
	var b strings.Builder
	b.WriteString(`(?i)^\s*`)
	for i, field := range strings.Fields(tmpl) {
		if i > 0 {
			b.WriteString(`\s+`)
		}
		switch field {
		case "{name}":
			b.WriteString(`\S+`)
		case "{password}":
			b.WriteString(`(\S+)`)
		default:
			b.WriteString(regexp.QuoteMeta(field))
		}
	}
	b.WriteString(`\s*$`)
	return regexp.MustCompile(b.String())
}

// redacted returns line with a typed password replaced by "***".
func (s *Session) redacted(line string) string {
	s.mu.Lock()
	re := s.redact
	s.mu.Unlock()
	if re == nil {
		return line
	}
	m := re.FindStringSubmatchIndex(line)
	if m == nil {
		return line
	}
	return line[:m[2]] + "***" + line[m[3]:]
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
	e := logstore.Entry{Time: s.o.Now(), Dir: logstore.Out, Text: s.redacted(line)}
	if err := s.o.Log.Append(e); err != nil {
		return e, &LogError{Err: err}
	}
	return e, nil
}

// Login sends the character's login line with password, logging it with
// the password redacted. Use it to answer EventNeedPassword.
func (s *Session) Login(password string) (logstore.Entry, error) {
	s.mu.Lock()
	c, ch := s.c, s.char
	s.mu.Unlock()
	if c == nil {
		return logstore.Entry{}, ErrNotConnected
	}
	wire, logged := loginLines(ch, password)
	if err := c.Send(wire); err != nil {
		return logstore.Entry{}, err
	}
	e := logstore.Entry{Time: s.o.Now(), Dir: logstore.Out, Text: logged}
	if err := s.o.Log.Append(e); err != nil {
		return e, &LogError{Err: err}
	}
	return e, nil
}

// loginLines returns the login line to send and the redacted one to log.
func loginLines(ch config.Character, password string) (wire, logged string) {
	withName := strings.ReplaceAll(ch.Login, "{name}", ch.Name)
	return strings.ReplaceAll(withName, "{password}", password),
		strings.ReplaceAll(withName, "{password}", "***")
}

// Disconnect closes the current connection on purpose: the session will
// not reconnect until Reconnect is called.
func (s *Session) Disconnect() {
	s.mu.Lock()
	c := s.c
	if c != nil {
		s.quitting = true
	}
	s.mu.Unlock()
	if c != nil {
		c.Close()
	}
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
		ch := s.Char()
		s.sys(fmt.Sprintf("connected to %s:%d", ch.Host, ch.Port))
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
	var prompts <-chan string // nil (never ready) unless c reports prompts
	if p, ok := c.(prompter); ok {
		prompts = p.Prompts()
	}
	for {
		select {
		case p := <-prompts:
			s.emit(Event{Kind: EventPrompt, Entry: logstore.Entry{Time: s.o.Now(), Dir: logstore.In, Text: p}})
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
// what goes over the wire; the log gets the template with "***". With no
// saved password it emits EventNeedPassword so the UI can ask.
func (s *Session) login(c LineConn) {
	ch := s.Char()
	if ch.Login == "" {
		return
	}
	pw := ""
	if strings.Contains(ch.Login, "{password}") {
		var err error
		if s.o.Password != nil {
			pw, err = s.o.Password()
		}
		if s.o.Password == nil || err != nil || pw == "" {
			s.sys(fmt.Sprintf("no saved password for %s/%s", ch.World, ch.ID))
			s.emit(Event{Kind: EventNeedPassword})
			return
		}
	}
	wire, logged := loginLines(ch, pw)
	if err := c.Send(wire); err != nil {
		return // the read side will notice the dead connection
	}
	s.logLine(logstore.Out, logged)
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
