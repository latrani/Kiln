package app

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
)

// sentConn records what's sent.
type sentConn struct {
	lineConn
	mu   sync.Mutex
	sent []string
}

func (c *sentConn) Send(l string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, l)
	return nil
}

func (c *sentConn) Sent() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.sent)
}

// submitApp is an App over fm with Kit connected to a sentConn; saved
// collects SavePassword calls.
func submitApp(t *testing.T, world string) (*App, *sentConn, map[string]string) {
	t.Helper()
	dir := configDir(t, map[string]string{"fm": world})
	c := &sentConn{lineConn: lineConn{lines: make(chan string)}}
	saved := map[string]string{}
	a := New(Deps{
		ConfigDir: dir,
		LogRoot:   filepath.Join(dir, "logs"),
		Dial:      func(context.Context, config.Character) (session.LineConn, error) { return c, nil },
		NewLog:    func(logstore.Layout) session.Appender { return nopLog{} },
		Password:  func(string, string, string) (string, error) { return "", errors.New("none") },
		SavePassword: func(_, world, char, pw string) error {
			saved[Key(world, char)] = pw
			return nil
		},
		Now: func() time.Time { return now },
	})
	a.ApplyConfig(load(t, dir))
	openAll(t, a, "fm/kit")
	effs := a.Connect("fm/kit")
	t.Cleanup(func() { a.Quit() })
	// Run the session's events through Handle until it's connected.
	msg := effs[0].(Run).Func().(SessionMsg)
	for a.Char("fm/kit").State != session.Connected {
		_, _, effs = a.Handle(msg)
		msg = effs[0].(Run).Func().(SessionMsg)
	}
	return a, c, saved
}

func TestSubmitSendsAndRecords(t *testing.T) {
	a, c, _ := submitApp(t, fmWorld)
	a.SetInput("//foo")
	res, _ := a.Submit()
	if !res.Sent || !slices.Equal(c.Sent(), []string{"/foo"}) {
		t.Errorf("sent %q, result %+v", c.Sent(), res)
	}
	kit := a.Char("fm/kit")
	if kit.Text != "" || !slices.Equal(kit.History.Lines(), []string{"//foo"}) {
		t.Errorf("after sending: text %q history %q", kit.Text, kit.History.Lines())
	}
}

func TestOverLimitAsksFirst(t *testing.T) {
	a, c, _ := submitApp(t, strings.Replace(fmWorld, "tls = true\n", "tls = true\nmax_line_bytes = 5\n", 1))
	a.SetInput("too long")
	if res, _ := a.Submit(); res.Sent || !a.Confirming() || a.Status().Text != str.StatusOverLimit(5) {
		t.Fatalf("first Enter: %+v confirming %v status %q", res, a.Confirming(), a.Status().Text)
	}
	a.Unconfirm()
	if res, _ := a.Submit(); res.Sent {
		t.Error("an edit in between should ask again")
	}
	if res, _ := a.Submit(); !res.Sent || !slices.Equal(c.Sent(), []string{"too long"}) {
		t.Errorf("second Enter: %+v sent %q", res, c.Sent())
	}
}

func TestCommandsInTheCore(t *testing.T) {
	a, _, _ := submitApp(t, fmWorld)
	for _, c := range []struct {
		text string
		do   string // a Do the front end carries out; "" for none
	}{{"/log", "/log"}, {"/edit world", "/edit"}, {"/notify all", "/notify"}, {"/away", "/away"}, {"/backup", "/backup"}, {"/frob", ""}} {
		a.SetInput(c.text)
		_, effs := a.Submit()
		var got string
		for _, e := range effs {
			if d, ok := e.(Do); ok {
				got = d.Cmd
				if d.Key != "fm/kit" {
					t.Errorf("%s: Do for %q", c.text, d.Key)
				}
			}
		}
		if got != c.do {
			t.Errorf("%s: Do %q, want %q", c.text, got, c.do)
		}
	}
	if a.Status().Text != str.StatusUnknownCommand("/frob") {
		t.Errorf("status = %q", a.Status().Text)
	}
	a.SetInput("/connect")
	a.Submit()
	if a.Status().Text != str.StatusAlreadyConnected("Kit") {
		t.Errorf("/connect while connected: %q", a.Status().Text)
	}
	a.SetInput("/close")
	a.Submit()
	if a.Char("fm/kit") != nil {
		t.Error("/close left Kit open")
	}
	a.SetInput("/connect")
	if _, effs := a.Submit(); effs != nil || a.Status().Text != str.StatusNeedsCharacter("/connect") {
		t.Errorf("with nothing open: effects %v status %q", effs, a.Status().Text)
	}
	a.SetInput("/open")
	if _, effs := a.Submit(); len(effs) != 1 || effs[0].(Do).Cmd != "/open" {
		t.Errorf("/open with nothing open: %v", effs)
	}
}

func TestPasswordPromptLogsInAndAsksToSave(t *testing.T) {
	a, c, saved := submitApp(t, fmWorld)
	kit := a.Char("fm/kit")
	a.SetInput("half a pose")
	a.Handle(SessionMsg{Key: "fm/kit", Sess: kit.Sess, OK: true, Ev: session.Event{Kind: session.EventNeedPassword}})
	if !kit.NeedPW || kit.Text != "" {
		t.Fatalf("prompt: needPW %v text %q; want the draft stashed", kit.NeedPW, kit.Text)
	}
	a.SetInput("s3cret")
	a.Submit()
	if kit.NeedPW || kit.Text != "half a pose" || slices.Contains(kit.History.Lines(), "s3cret") {
		t.Errorf("after logging in: needPW %v text %q history %q", kit.NeedPW, kit.Text, kit.History.Lines())
	}
	if sent := c.Sent(); len(sent) == 0 || !strings.Contains(sent[len(sent)-1], "s3cret") {
		t.Errorf("sent %q, want the login line", sent)
	}
	w, ch, ok := a.PendingSave()
	if !ok || w != "fm" || ch != "kit" {
		t.Fatalf("PendingSave = %q %q %v", w, ch, ok)
	}
	a.AnswerSave(true)
	if saved["fm/kit"] != "s3cret" || a.Status().Text != str.StatusPasswordSaved() {
		t.Errorf("saved %q status %q", saved, a.Status().Text)
	}
	if _, _, ok := a.PendingSave(); ok {
		t.Error("still pending after the answer")
	}
}

func TestSkippingTheLogin(t *testing.T) {
	a, _, _ := submitApp(t, fmWorld)
	kit := a.Char("fm/kit")
	a.SetInput("draft")
	a.Handle(SessionMsg{Key: "fm/kit", Sess: kit.Sess, OK: true, Ev: session.Event{Kind: session.EventNeedPassword}})
	if !a.SkipLogin() || kit.NeedPW || kit.Text != "draft" || a.Status().Text != str.StatusSkippedLogin() {
		t.Errorf("skip: needPW %v text %q status %q", kit.NeedPW, kit.Text, a.Status().Text)
	}
	if a.SkipLogin() {
		t.Error("SkipLogin with no prompt up did something")
	}
}

// Closing the last character asks again before an over-limit line, as
// switching does: nothing carries over to whatever opens next.
func TestClosingTheLastCharacterUnconfirms(t *testing.T) {
	a, _, _ := submitApp(t, strings.Replace(fmWorld, "tls = true\n", "tls = true\nmax_line_bytes = 5\n", 1))
	a.SetInput("too long")
	a.Submit()
	if !a.Confirming() {
		t.Fatal("Enter on an over-limit line didn't ask first")
	}
	a.Close("fm/kit")
	if a.Active() != "" || a.Confirming() {
		t.Errorf("active %q confirming %v after closing the last character", a.Active(), a.Confirming())
	}
}
