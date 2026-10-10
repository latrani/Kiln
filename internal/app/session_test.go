package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
)

type nopLog struct{}

func (nopLog) Append(logstore.Entry) error { return nil }

// lineConn is a connection that sends nothing until it's closed.
type lineConn struct {
	lines chan string
	once  sync.Once
}

func (c *lineConn) Lines() <-chan string { return c.lines }
func (c *lineConn) Err() error           { return nil }
func (c *lineConn) Send(string) error    { return nil }
func (c *lineConn) Close() error         { c.once.Do(func() { close(c.lines) }); return nil }

var now = time.Date(2026, 9, 24, 21, 14, 0, 0, time.Local)

// sessionApp is an App over fm and zz whose sessions dial lineConns.
func sessionApp(t *testing.T, worlds map[string]string) *App {
	t.Helper()
	return sessionAppAt(t, worlds, func() time.Time { return now })
}

// sessionAppAt is sessionApp on the clock clock.
func sessionAppAt(t *testing.T, worlds map[string]string, clock func() time.Time) *App {
	t.Helper()
	dir := configDir(t, worlds)
	a := New(Deps{
		LogRoot: filepath.Join(dir, "logs"),
		Dial: func(context.Context, config.Character) (session.LineConn, error) {
			return &lineConn{lines: make(chan string)}, nil
		},
		NewLog:   func(logstore.Layout) session.Appender { return nopLog{} },
		Password: func(string, string, string) (string, error) { return "", errors.New("none") },
		Now:      clock,
	})
	a.ApplyConfig(load(t, dir))
	return a
}

// attach gives k a session that never runs, as Connect would, so events
// can be handed to Handle.
func attach(a *App, k string) *session.Session {
	c := a.Char(k)
	c.Sess = session.New(session.Options{Char: c.Ch, Log: nopLog{}})
	return c.Sess
}

func lineMsg(k string, s *session.Session, text string) SessionMsg {
	return SessionMsg{Key: k, Sess: s, OK: true, Ev: session.Event{Kind: session.EventLine, Entry: logstore.Entry{Dir: logstore.In, Text: text}}}
}

func stateMsg(k string, s *session.Session, st session.State, err error) SessionMsg {
	return SessionMsg{Key: k, Sess: s, OK: true, Ev: session.Event{Kind: session.EventState, State: st, Err: err}}
}

func TestHandleCountsUnreadOnlyOffScreen(t *testing.T) {
	// quiet goes above [[characters]], or TOML hands it to the last one.
	quiet := strings.Replace(fmWorld, "login = \"connect {name} {password}\"\n", "login = \"connect {name} {password}\"\nquiet = [\"wiki\"]\n", 1) +
		"\n[[classify]]\ntag = \"wiki\"\npattern = '^\\[Wiki\\]'\n"
	a := sessionApp(t, map[string]string{"fm": quiet})
	openAll(t, a, "fm/kit", "fm/rook")
	kit, rook := attach(a, "fm/kit"), attach(a, "fm/rook")
	for _, m := range []SessionMsg{lineMsg("fm/kit", kit, "hi"), lineMsg("fm/rook", rook, "hi"), lineMsg("fm/rook", rook, "[Wiki] edited")} {
		ev, ok, effs := a.Handle(m)
		if !ok || ev.Line.Entry.Text != m.Ev.Entry.Text {
			t.Fatalf("Handle(%q) = %+v, %v", m.Ev.Entry.Text, ev, ok)
		}
		if len(effs) != 1 {
			t.Errorf("Handle(%q) effects = %v, want the next wait", m.Ev.Entry.Text, effs)
		}
	}
	if u := a.Char("fm/kit").Unread; u != 0 {
		t.Errorf("the active character counted %d unread", u)
	}
	if u := a.Char("fm/rook").Unread; u != 1 {
		t.Errorf("Rook unread = %d, want 1 (the quiet line doesn't count)", u)
	}
}

func TestHandleIgnoresStaleSessions(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	attach(a, "fm/kit")
	old := session.New(session.Options{Char: a.Char("fm/kit").Ch, Log: nopLog{}})
	if _, ok, effs := a.Handle(lineMsg("fm/kit", old, "hi")); ok || effs != nil {
		t.Error("handled an event from a replaced session")
	}
	cur := a.Char("fm/kit").Sess
	if _, ok, effs := a.Handle(SessionMsg{Key: "fm/kit", Sess: cur, OK: false}); ok || effs != nil {
		t.Error("handled a shut-down session's last receive")
	}
}

func TestHandleClosesAnOrphanWhenItDisconnects(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit", "fm/rook")
	s := attach(a, "fm/rook")
	rook := a.Char("fm/rook")
	rook.State, rook.Orphan = session.Connected, true
	ev, ok, effs := a.Handle(stateMsg("fm/rook", s, session.Disconnected, nil))
	if !ok || !ev.Closed || effs != nil || a.Char("fm/rook") != nil {
		t.Errorf("ev %+v ok %v effs %v open %v; want closed with nothing more to wait for", ev, ok, effs, a.Char("fm/rook") != nil)
	}
}

func TestHandleTracksAChangedCertificate(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	pin := &conn.PinMismatchError{HostPort: "muck.test:8888", Pinned: "sha256:aa", Got: "sha256:bb"}
	a.Handle(stateMsg("fm/kit", s, session.Failed, pin))
	kit := a.Char("fm/kit")
	if kit.Pin != pin || kit.State != session.Failed {
		t.Errorf("pin %v state %v after a mismatch", kit.Pin, kit.State)
	}
	if st := a.Status(); !st.Err || st.Text != str.StatusCertChanged("Kit") {
		t.Errorf("status = %+v", st)
	}
	a.Handle(stateMsg("fm/kit", s, session.Connected, nil))
	if kit.Pin != nil || !kit.ConnectedAt.Equal(now) {
		t.Errorf("after connecting: pin %v, connected at %v", kit.Pin, kit.ConnectedAt)
	}
}

func TestConnectWaitsForTheSessionsEvents(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	effs := a.Connect("fm/kit")
	defer a.Quit()
	kit := a.Char("fm/kit")
	if kit.Sess == nil || kit.State != session.Connecting {
		t.Fatalf("after Connect: session %v state %v", kit.Sess, kit.State)
	}
	run, ok := effs[0].(Run)
	if len(effs) != 1 || !ok {
		t.Fatalf("Connect effects = %v, want one Run", effs)
	}
	msg, ok := run.Func().(SessionMsg)
	if !ok || msg.Key != "fm/kit" || msg.Sess != kit.Sess || !msg.OK {
		t.Errorf("the wait returned %+v", msg)
	}
	if again := a.Connect("fm/kit"); again != nil {
		t.Errorf("Connect on a live session = %v, want a reconnect and nothing to wait for", again)
	}
}

func TestQuitStopsEverySession(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	a.Connect("fm/kit")
	effs := a.Quit()
	if _, ok := effs[0].(Quit); len(effs) != 1 || !ok {
		t.Errorf("Quit effects = %v", effs)
	}
}
