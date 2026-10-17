package session

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/telnet"
)

// fakeConn is a scripted LineConn.
type fakeConn struct {
	lines     chan conn.Line
	err       error
	mu        sync.Mutex
	sent      []string
	closeOnce sync.Once
}

func newFakeConn(lines ...string) *fakeConn {
	f := &fakeConn{lines: make(chan conn.Line, len(lines)+1)}
	for _, l := range lines {
		f.lines <- conn.Line{Text: l}
	}
	return f
}
func (f *fakeConn) Lines() <-chan conn.Line { return f.lines }
func (f *fakeConn) Err() error              { return f.err }
func (f *fakeConn) Close() error {
	f.closeOnce.Do(func() { close(f.lines) })
	return nil
}
func (f *fakeConn) Send(l string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, l)
	return nil
}
func (f *fakeConn) Sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sent...)
}

type memLog struct {
	mu       sync.Mutex
	entries  []logstore.Entry
	fail     error
	sessions []int // len(entries) at each NewSession
}

func (m *memLog) NewSession() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions = append(m.sessions, len(m.entries))
}

func (m *memLog) Append(e logstore.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, e)
	return m.fail
}
func (m *memLog) Texts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, e := range m.entries {
		out = append(out, string(rune(e.Dir))+" "+e.Text)
	}
	return out
}

var kit = config.Character{World: "fm", ID: "kit", Name: "Kit", Host: "h", Port: 1, Login: "connect {name} {password}", Reconnect: true}

// waitFor reads events until pred is true or times out.
func waitFor(t *testing.T, s *Session, pred func(Event) bool) Event {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-s.Events():
			if !ok {
				t.Fatal("events closed")
			}
			if pred(ev) {
				return ev
			}
		case <-timeout:
			t.Fatal("timed out")
		}
	}
}

func isState(st State) func(Event) bool {
	return func(e Event) bool { return e.Kind == EventState && e.State == st }
}

func TestLoginRedactsPasswordInLog(t *testing.T) {
	fc := newFakeConn("Welcome!")
	log := &memLog{}
	pw := `hunter2 {name} $1`
	s := New(Options{
		Char:     kit,
		Dial:     func(context.Context) (LineConn, error) { return fc, nil },
		Log:      log,
		Password: func() (string, error) { return pw, nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, func(e Event) bool { return e.Kind == EventLine && e.Entry.Text == "Welcome!" })

	if sent := fc.Sent(); len(sent) != 1 || sent[0] != "connect Kit "+pw {
		t.Errorf("wire = %q", sent)
	}
	for _, line := range log.Texts() {
		if strings.Contains(line, "hunter2") {
			t.Errorf("password leaked into log: %q", line)
		}
	}
	if got := log.Texts(); !contains(got, "> connect Kit ***") {
		t.Errorf("log = %q, want redacted login line", got)
	}
}

func TestMissingPasswordSkipsLogin(t *testing.T) {
	fc := newFakeConn("Welcome!")
	log := &memLog{}
	s := New(Options{
		Char:     kit,
		Dial:     func(context.Context) (LineConn, error) { return fc, nil },
		Log:      log,
		Password: func() (string, error) { return "", errors.New("not found") },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, func(e Event) bool { return e.Kind == EventLine && e.Entry.Text == "Welcome!" })
	if sent := fc.Sent(); len(sent) != 0 {
		t.Errorf("sent %q, want nothing", sent)
	}
	if !containsPrefix(log.Texts(), "* "+str.SessionNoSavedPassword("fm", "kit")) {
		t.Errorf("log = %q", log.Texts())
	}
}

func TestNoLoginTemplateSendsNothing(t *testing.T) {
	fc := newFakeConn("hi")
	ch := kit
	ch.Login = ""
	s := New(Options{Char: ch, Dial: func(context.Context) (LineConn, error) { return fc, nil }, Log: &memLog{}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, func(e Event) bool { return e.Kind == EventLine && e.Entry.Text == "hi" })
	if len(fc.Sent()) != 0 {
		t.Errorf("sent %q", fc.Sent())
	}
}

func TestSendLogsOutgoing(t *testing.T) {
	fc := newFakeConn()
	ch := kit
	ch.Login = ""
	log := &memLog{}
	s := New(Options{Char: ch, Dial: func(context.Context) (LineConn, error) { return fc, nil }, Log: log})
	if _, err := s.Send("x"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Send before connect = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, isState(Connected))
	e, err := s.Send(":grins.")
	if err != nil {
		t.Fatal(err)
	}
	if e.Dir != logstore.Out || e.Text != ":grins." {
		t.Errorf("Send returned %+v", e)
	}
	if !contains(log.Texts(), "> :grins.") || fc.Sent()[0] != ":grins." {
		t.Errorf("log=%q sent=%q", log.Texts(), fc.Sent())
	}
}

func TestSendReportsLogFailureAfterSending(t *testing.T) {
	fc := newFakeConn()
	ch := kit
	ch.Login = ""
	s := New(Options{Char: ch, Dial: func(context.Context) (LineConn, error) { return fc, nil }, Log: &memLog{fail: errors.New("disk full")}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, isState(Connected))
	_, err := s.Send("hi")
	var le *LogError
	if !errors.As(err, &le) {
		t.Fatalf("err = %v, want *LogError", err)
	}
	if len(fc.Sent()) != 1 {
		t.Error("line should still have been sent")
	}
}

func TestSendDoesNotBlockWhenEventsBacklogged(t *testing.T) {
	flood := make([]string, 1000) // far more than the events buffer
	for i := range flood {
		flood[i] = "spam"
	}
	fc := newFakeConn(flood...)
	ch := kit
	ch.Login = ""
	s := New(Options{Char: ch, Dial: func(context.Context) (LineConn, error) { return fc, nil }, Log: &memLog{}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx) // nobody reads Events(): the buffer fills and Run blocks
	time.Sleep(100 * time.Millisecond)
	done := make(chan error, 1)
	go func() { _, err := s.Send("hello"); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Send = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Send blocked behind a full events channel")
	}
}

func TestRunStopsOnCancelWithoutConsumer(t *testing.T) {
	flood := make([]string, 1000)
	for i := range flood {
		flood[i] = "spam"
	}
	ch := kit
	ch.Login = ""
	s := New(Options{Char: ch, Log: &memLog{}, Dial: func(context.Context) (LineConn, error) { return newFakeConn(flood...), nil }})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	time.Sleep(100 * time.Millisecond) // events buffer is full by now
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run wedged on a full events channel after cancel")
	}
}

func TestReconnectsWithBackoffAfterDrop(t *testing.T) {
	first := newFakeConn("one")
	first.Close() // server hangs up after one line
	first.err = errors.New("connection reset")
	second := newFakeConn("two")
	conns := []*fakeConn{first, second}
	var mu sync.Mutex
	var delays []int
	ch := kit
	ch.Login = ""
	log := &memLog{}
	s := New(Options{
		Char: ch,
		Log:  log,
		Dial: func(context.Context) (LineConn, error) {
			mu.Lock()
			defer mu.Unlock()
			c := conns[0]
			conns = conns[1:]
			return c, nil
		},
		Backoff: func(a int) time.Duration {
			mu.Lock()
			delays = append(delays, a)
			mu.Unlock()
			return time.Millisecond
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, func(e Event) bool { return e.Kind == EventLine && e.Entry.Text == "two" })
	if !containsPrefix(log.Texts(), "* "+upTo(str.SessionDisconnectedRetrying("connection reset", mark))) {
		t.Errorf("log = %q", log.Texts())
	}
	// Each connection starts a new log session, just before "connected".
	texts := log.Texts()
	log.mu.Lock()
	sessions := slices.Clone(log.sessions)
	log.mu.Unlock()
	if len(sessions) != 2 {
		t.Fatalf("NewSession at %v, want once per connection", sessions)
	}
	for _, i := range sessions {
		if !strings.HasPrefix(texts[i], "* "+upTo(str.SessionConnected(mark, mark))) {
			t.Errorf("session starts at %q, want the connected line", texts[i])
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(delays) != 1 || delays[0] != 0 {
		t.Errorf("backoff attempts = %v, want [0]", delays)
	}
}

func TestDialFailuresBackOffIncreasingly(t *testing.T) {
	var mu sync.Mutex
	var attempts []int
	calls := 0
	s := New(Options{
		Char: kit,
		Log:  &memLog{},
		Dial: func(context.Context) (LineConn, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			if calls <= 3 {
				return nil, errors.New("refused")
			}
			return newFakeConn(), nil
		},
		Backoff: func(a int) time.Duration {
			mu.Lock()
			attempts = append(attempts, a)
			mu.Unlock()
			return time.Millisecond
		},
		Password: func() (string, error) { return "pw", nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, isState(Connected))
	mu.Lock()
	defer mu.Unlock()
	if len(attempts) != 3 || attempts[0] != 0 || attempts[2] != 2 {
		t.Errorf("attempts = %v, want [0 1 2]", attempts)
	}
}

func TestReconnectOffStaysDownAfterDrop(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	first := newFakeConn()
	first.err = errors.New("connection reset")
	ch := kit
	ch.Login = ""
	ch.Reconnect = false
	log := &memLog{}
	s := New(Options{
		Char: ch,
		Log:  log,
		Dial: func(context.Context) (LineConn, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			if calls == 1 {
				return first, nil
			}
			return newFakeConn(), nil
		},
		Backoff: func(int) time.Duration { t.Error("reconnect off must not use backoff"); return 0 },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, isState(Connected))
	first.Close() // server hangs up
	ev := waitFor(t, s, isState(Disconnected))
	if ev.Err == nil {
		t.Error("Disconnected without the connection error")
	}
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if calls != 1 {
		t.Errorf("redialed %d times with reconnect off", calls-1)
	}
	mu.Unlock()
	if !contains(log.Texts(), "* "+str.SessionDisconnected("connection reset")) {
		t.Errorf("log = %q", log.Texts())
	}
	s.Reconnect()
	waitFor(t, s, isState(Connected))
}

func TestReconnectOffStaysDownAfterDialFailure(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	ch := kit
	ch.Reconnect = false
	log := &memLog{}
	s := New(Options{
		Char: ch,
		Log:  log,
		Dial: func(context.Context) (LineConn, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			if calls == 1 {
				return nil, errors.New("refused")
			}
			return newFakeConn(), nil
		},
		Backoff:  func(int) time.Duration { t.Error("reconnect off must not use backoff"); return 0 },
		Password: func() (string, error) { return "pw", nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, isState(Disconnected))
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if calls != 1 {
		t.Errorf("redialed %d times with reconnect off", calls-1)
	}
	mu.Unlock()
	if !contains(log.Texts(), "* "+str.SessionConnectFailed(errors.New("refused"))) {
		t.Errorf("log = %q", log.Texts())
	}
	s.Reconnect()
	waitFor(t, s, isState(Connected))
}

func TestQuitDoesNotReconnect(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	first := newFakeConn()
	ch := kit
	ch.Login = ""
	log := &memLog{}
	s := New(Options{
		Char: ch,
		Log:  log,
		Dial: func(context.Context) (LineConn, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			if calls == 1 {
				return first, nil
			}
			return newFakeConn(), nil
		},
		Backoff: func(int) time.Duration { t.Error("quit must not use backoff"); return 0 },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, isState(Connected))
	if _, err := s.Send("QUIT"); err != nil {
		t.Fatal(err)
	}
	first.Close() // server hangs up in response
	waitFor(t, s, isState(Disconnected))
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if calls != 1 {
		t.Errorf("redialed %d times after QUIT", calls-1)
	}
	mu.Unlock()
	if !contains(log.Texts(), "* "+str.SessionQuit()) {
		t.Errorf("log = %q", log.Texts())
	}
	s.Reconnect()
	waitFor(t, s, isState(Connected))
}

func TestPinMismatchFailsUntilReconnect(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	s := New(Options{
		Char: kit,
		Log:  &memLog{},
		Dial: func(context.Context) (LineConn, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			if calls == 1 {
				return nil, &conn.PinMismatchError{HostPort: "h:1", Pinned: "sha256:a", Got: "sha256:b"}
			}
			return newFakeConn(), nil
		},
		Backoff: func(int) time.Duration { t.Error("pin mismatch must not use backoff"); return 0 },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	ev := waitFor(t, s, isState(Failed))
	var pin *conn.PinMismatchError
	if !errors.As(ev.Err, &pin) {
		t.Errorf("Failed err = %v", ev.Err)
	}
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if calls != 1 {
		t.Errorf("redialed %d times without Reconnect", calls-1)
	}
	mu.Unlock()
	s.Reconnect()
	waitFor(t, s, isState(Connected))
}

func TestCertificateErrorsFailWithoutRetry(t *testing.T) {
	for name, dialErr := range map[string]error{
		"unknown authority": &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}},
		"hostname":          fmt.Errorf("dial: %w", x509.HostnameError{Host: "h"}),
		"expired":           x509.CertificateInvalidError{Reason: x509.Expired},
	} {
		t.Run(name, func(t *testing.T) {
			s := New(Options{
				Char:    kit,
				Log:     &memLog{},
				Dial:    func(context.Context) (LineConn, error) { return nil, dialErr },
				Backoff: func(int) time.Duration { t.Error("certificate error must not use backoff"); return 0 },
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go s.Run(ctx)
			ev := waitFor(t, s, isState(Failed))
			if ev.Err != dialErr {
				t.Errorf("Failed err = %v", ev.Err)
			}
		})
	}
}

// permanentErr is a dial error that says it will recur, as the web
// build's refused-certificate error does.
type permanentErr struct{}

func (permanentErr) Error() string   { return "refused" }
func (permanentErr) Permanent() bool { return true }

func TestPermanentDialErrorFailsWithoutRetry(t *testing.T) {
	dialErr := fmt.Errorf("dial: %w", permanentErr{})
	s := New(Options{
		Char:    kit,
		Log:     &memLog{},
		Dial:    func(context.Context) (LineConn, error) { return nil, dialErr },
		Backoff: func(int) time.Duration { t.Error("a permanent error must not use backoff"); return 0 },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	if ev := waitFor(t, s, isState(Failed)); ev.Err != dialErr {
		t.Errorf("Failed err = %v", ev.Err)
	}
}

func TestLogFailureStillDeliversLine(t *testing.T) {
	ch := kit
	ch.Login = ""
	s := New(Options{
		Char: ch,
		Log:  &memLog{fail: errors.New("disk full")},
		Dial: func(context.Context) (LineConn, error) { return newFakeConn("still here"), nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, func(e Event) bool { return e.Kind == EventLogError })
	waitFor(t, s, func(e Event) bool { return e.Kind == EventLine && e.Entry.Text == "still here" })
}

func TestRunStopsOnCancel(t *testing.T) {
	ch := kit
	ch.Login = ""
	s := New(Options{Char: ch, Log: &memLog{}, Dial: func(context.Context) (LineConn, error) { return newFakeConn(), nil }})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	waitFor(t, s, isState(Connected))
	cancel()
	go func() {
		for range s.Events() {
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func containsPrefix(xs []string, prefix string) bool {
	for _, x := range xs {
		if strings.HasPrefix(x, prefix) {
			return true
		}
	}
	return false
}

func TestTypedLoginIsRedacted(t *testing.T) {
	cases := []struct{ tmpl, typed, want string }{
		{"connect {name} {password}", "connect Kit hunter2", "connect Kit ***"},
		{"connect {name} {password}", "  CONNECT rook s3cr3t  ", "  CONNECT rook ***  "},
		{"connect {name} {password}", "connect Kit", "connect Kit"},               // no password given
		{"connect {name} {password}", "say connect Kit pw", "say connect Kit pw"}, // not a login
		{"co {name}={password}", "co Kit=pw", "co Kit=pw"},                        // template tokens must be whole words
		{"", "connect Kit hunter2", "connect Kit ***"},                            // no template: default connect pattern
	}
	for _, c := range cases {
		fc := newFakeConn()
		ch := kit
		ch.Login = c.tmpl
		log := &memLog{}
		s := New(Options{Char: ch, Dial: func(context.Context) (LineConn, error) { return fc, nil }, Log: log,
			Password: func() (string, error) { return "", errors.New("none") }})
		ctx, cancel := context.WithCancel(context.Background())
		go s.Run(ctx)
		waitFor(t, s, isState(Connected))
		e, err := s.Send(c.typed)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if e.Text != c.want {
			t.Errorf("tmpl %q typed %q: logged %q, want %q", c.tmpl, c.typed, e.Text, c.want)
		}
		if sent := fc.Sent(); sent[len(sent)-1] != c.typed {
			t.Errorf("wire = %q, want the typed line unchanged", sent)
		}
	}
}

func TestNeedPasswordThenLogin(t *testing.T) {
	fc := newFakeConn()
	log := &memLog{}
	s := New(Options{Char: kit, Dial: func(context.Context) (LineConn, error) { return fc, nil }, Log: log})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, func(e Event) bool { return e.Kind == EventNeedPassword })
	e, err := s.Login("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if e.Text != "connect Kit ***" {
		t.Errorf("logged %q", e.Text)
	}
	if sent := fc.Sent(); len(sent) != 1 || sent[0] != "connect Kit hunter2" {
		t.Errorf("wire = %q", sent)
	}
	for _, l := range log.Texts() {
		if strings.Contains(l, "hunter2") {
			t.Errorf("password in log: %q", l)
		}
	}
}

func TestLoginWhenDisconnected(t *testing.T) {
	s := New(Options{Char: kit, Log: &memLog{}, Dial: func(context.Context) (LineConn, error) { return newFakeConn(), nil }})
	if _, err := s.Login("pw"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("err = %v", err)
	}
}

func TestDisconnectStaysDown(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	ch := kit
	ch.Login = ""
	log := &memLog{}
	s := New(Options{Char: ch, Log: log,
		Dial: func(context.Context) (LineConn, error) {
			mu.Lock()
			calls++
			mu.Unlock()
			return newFakeConn(), nil
		},
		Backoff: func(int) time.Duration { t.Error("Disconnect must not back off"); return 0 },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, isState(Connected))
	s.Disconnect()
	waitFor(t, s, isState(Disconnected))
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if calls != 1 {
		t.Errorf("dialed %d times", calls)
	}
	mu.Unlock()
	if !contains(log.Texts(), "* "+str.SessionQuit()) {
		t.Errorf("log = %q", log.Texts())
	}
	s.Reconnect()
	waitFor(t, s, isState(Connected))
}

// promptConn adds Prompts() to fakeConn.
type promptConn struct {
	*fakeConn
	prompts chan string
}

func (p promptConn) Prompts() <-chan string { return p.prompts }

func TestPromptsAreEmittedNotLogged(t *testing.T) {
	pc := promptConn{newFakeConn(), make(chan string, 1)}
	pc.prompts <- "Password: "
	ch := kit
	ch.Login = ""
	log := &memLog{}
	s := New(Options{Char: ch, Log: log, Dial: func(context.Context) (LineConn, error) { return pc, nil }})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	ev := waitFor(t, s, func(e Event) bool { return e.Kind == EventPrompt })
	if ev.Entry.Text != "Password: " {
		t.Errorf("prompt = %q", ev.Entry.Text)
	}
	for _, l := range log.Texts() {
		if strings.Contains(l, "Password:") {
			t.Errorf("prompt was logged: %q", l)
		}
	}
}

func TestSetCharChangesRedaction(t *testing.T) {
	fc := newFakeConn()
	ch := kit
	ch.Login = ""
	s := New(Options{Char: ch, Log: &memLog{}, Dial: func(context.Context) (LineConn, error) { return fc, nil }})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, isState(Connected))
	ch.Login = "login {name} {password}"
	s.SetChar(ch)
	if s.Char().Login != ch.Login {
		t.Errorf("Char() not updated")
	}
	if e, _ := s.Send("login Kit pw"); e.Text != "login Kit ***" {
		t.Errorf("logged %q", e.Text)
	}
}

func TestReconnectWhileConnectedThenQuitStaysDown(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	first := newFakeConn()
	ch := kit
	ch.Login = ""
	s := New(Options{Char: ch, Log: &memLog{},
		Dial: func(context.Context) (LineConn, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			if calls == 1 {
				return first, nil
			}
			return newFakeConn(), nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, isState(Connected))
	s.Reconnect() // e.g. /connect on an already-connected character
	s.Send("QUIT")
	first.Close()
	waitFor(t, s, isState(Disconnected))
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Errorf("redialed %d times after QUIT", calls-1)
	}
}

func TestDisconnectDuringBackoffStops(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	s := New(Options{Char: kit, Log: &memLog{},
		Dial: func(context.Context) (LineConn, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			if calls == 1 {
				return nil, errors.New("refused")
			}
			return newFakeConn(), nil
		},
		Backoff: func(int) time.Duration { return time.Hour },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, isState(Disconnected)) // first dial failed; now in a 1h backoff
	s.Disconnect()
	waitFor(t, s, func(e Event) bool { return e.Kind == EventLine && e.Entry.Text == str.SessionQuit() })
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if calls != 1 {
		t.Errorf("dialed %d times after Disconnect", calls)
	}
	mu.Unlock()
	s.Reconnect()
	waitFor(t, s, isState(Connected))
}

// nawsServer accepts one telnet connection. If ask, it sends DO NAWS and
// signals negotiated once the client's first NAWS report arrives. It then
// returns every byte received up to and including "marker\r\n".
func nawsServer(t *testing.T, ask bool) (port int, negotiated <-chan struct{}, received <-chan []byte) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	neg, got := make(chan struct{}), make(chan []byte, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		if ask {
			c.Write([]byte{telnet.IAC, telnet.DO, telnet.OptNAWS})
		}
		c.SetReadDeadline(time.Now().Add(5 * time.Second))
		var all []byte
		buf := make([]byte, 256)
		signalled := false
		for !bytes.HasSuffix(all, []byte("marker\r\n")) {
			n, err := c.Read(buf)
			all = append(all, buf[:n]...)
			if ask && !signalled && bytes.Contains(all, []byte{telnet.IAC, telnet.SE}) {
				signalled = true
				close(neg)
			}
			if err != nil {
				break
			}
		}
		got <- all
	}()
	return ln.Addr().(*net.TCPAddr).Port, neg, got
}

func TestResizeNoOpWithoutNAWS(t *testing.T) {
	port, _, received := nawsServer(t, false)
	ch := kit
	ch.Login = ""
	s := New(Options{Char: ch, Log: &memLog{}, Dial: func(ctx context.Context) (LineConn, error) {
		return conn.Dial(ctx, conn.Options{Host: "127.0.0.1", Port: port, Width: 80, Height: 24})
	}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, isState(Connected))
	s.Resize(100, 40)
	s.Resize(120, 50)
	if _, err := s.Send("marker"); err != nil {
		t.Fatal(err)
	}
	if got := <-received; string(got) != "marker\r\n" {
		t.Errorf("server received %q, want only the line (no NAWS)", got)
	}
}

func TestResizeReportsAfterNAWS(t *testing.T) {
	port, negotiated, received := nawsServer(t, true)
	ch := kit
	ch.Login = ""
	s := New(Options{Char: ch, Log: &memLog{}, Dial: func(ctx context.Context) (LineConn, error) {
		return conn.Dial(ctx, conn.Options{Host: "127.0.0.1", Port: port, Width: 80, Height: 24})
	}})
	s.Resize(90, 30) // before connecting: applied as soon as the conn exists
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, isState(Connected))
	select {
	case <-negotiated:
	case <-time.After(5 * time.Second):
		t.Fatal("NAWS never negotiated")
	}
	s.Resize(100, 40)
	if _, err := s.Send("marker"); err != nil {
		t.Fatal(err)
	}
	got := <-received
	initial := []byte{telnet.IAC, telnet.SB, telnet.OptNAWS, 0, 90, 0, 30, telnet.IAC, telnet.SE}
	resized := []byte{telnet.IAC, telnet.SB, telnet.OptNAWS, 0, 100, 0, 40, telnet.IAC, telnet.SE}
	if !bytes.Contains(got, initial) {
		t.Errorf("server received %v, want the pre-connect size %v in the first report", got, initial)
	}
	if i, j := bytes.Index(got, resized), bytes.Index(got, []byte("marker")); i < 0 || i > j {
		t.Errorf("server received %v, want %v before the marker", got, resized)
	}
}

// timedSession runs a session for Rook (no login) over fc, its clock
// reading *now.
func timedSession(t *testing.T, fc *fakeConn, now *time.Time) (*Session, *memLog) {
	t.Helper()
	log := &memLog{}
	rook := kit
	rook.Login = ""
	s := New(Options{
		Char: rook,
		Dial: func(context.Context) (LineConn, error) { return fc, nil },
		Log:  log,
		Now:  func() time.Time { return *now },
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.Run(ctx)
	waitFor(t, s, isState(Connected))
	return s, log
}

func lineEvent(text string) func(Event) bool {
	return func(e Event) bool { return e.Kind == EventLine && e.Entry.Text == text }
}

func TestLinesUseTheirTimes(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	fc := newFakeConn()
	s, _ := timedSession(t, fc, &now)
	arrived := now.Add(time.Hour)
	now = now.Add(2 * time.Hour) // read by the session after the send below
	fc.lines <- conn.Line{Text: "earlier", At: arrived}
	if e := waitFor(t, s, lineEvent("earlier")); !e.Entry.Time.Equal(arrived) {
		t.Errorf("logged at %v, want %v (when it arrived, not now)", e.Entry.Time, arrived)
	}
}

func TestNotesAreSys(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	fc := newFakeConn()
	s, _ := timedSession(t, fc, &now)
	fc.lines <- conn.Line{Text: "resumed", At: now, Note: true}
	if e := waitFor(t, s, lineEvent("resumed")); e.Entry.Dir != logstore.Sys {
		t.Errorf("a note logged as %q, want sys", e.Entry.Dir)
	}
}

func TestEntryTimesNeverGoBackwards(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	fc := newFakeConn()
	s, _ := timedSession(t, fc, &now)
	if _, err := s.Send("look"); err != nil {
		t.Fatal(err)
	}
	fc.lines <- conn.Line{Text: "stale", At: now.Add(-30 * time.Minute)}
	if e := waitFor(t, s, lineEvent("stale")); !e.Entry.Time.Equal(now) {
		t.Errorf("a line older than the last entry logged at %v, want %v", e.Entry.Time, now)
	}
	fc.lines <- conn.Line{Text: "fresh", At: now.Add(5 * time.Minute)}
	if e := waitFor(t, s, lineEvent("fresh")); !e.Entry.Time.Equal(now.Add(5 * time.Minute)) {
		t.Errorf("a newer line logged at %v, want its own time", e.Entry.Time)
	}
}

// keepConn records each Keep with how many entries the log had then.
type keepConn struct {
	*fakeConn
	log     *memLog
	resumed bool
	mu      sync.Mutex
	kept    [][2]int64 // off, log length
}

func (k *keepConn) Keep(off int64) {
	k.log.mu.Lock()
	n := len(k.log.entries)
	k.log.mu.Unlock()
	k.mu.Lock()
	k.kept = append(k.kept, [2]int64{off, int64(n)})
	k.mu.Unlock()
}

func (k *keepConn) Resumed() bool { return k.resumed }

func (k *keepConn) keeps() [][2]int64 {
	k.mu.Lock()
	defer k.mu.Unlock()
	return slices.Clone(k.kept)
}

func runKeep(t *testing.T, kc *keepConn, dials ...LineConn) *Session {
	t.Helper()
	i := 0
	s := New(Options{
		Char: kit,
		Dial: func(context.Context) (LineConn, error) {
			if i == 0 {
				i++
				return kc, nil
			}
			return dials[min(i, len(dials))-1], nil
		},
		Log:      kc.log,
		Password: func() (string, error) { return "pw", nil },
		Backoff:  func(int) time.Duration { return time.Millisecond },
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.Run(ctx)
	return s
}

func TestKeepAfterTheLog(t *testing.T) {
	kc := &keepConn{fakeConn: newFakeConn(), log: &memLog{}}
	s := runKeep(t, kc)
	waitFor(t, s, isState(Connected))
	kc.lines <- conn.Line{Text: "Rook waves.", Off: 42}
	waitFor(t, s, lineEvent("Rook waves."))
	ks := kc.keeps()
	if len(ks) != 1 || ks[0][0] != 42 {
		t.Fatalf("keeps %v, want one at 42", ks)
	}
	kc.log.mu.Lock()
	logged := slices.IndexFunc(kc.log.entries, func(e logstore.Entry) bool { return e.Text == "Rook waves." })
	kc.log.mu.Unlock()
	if logged < 0 || ks[0][1] <= int64(logged) {
		t.Errorf("kept with %d entries logged, before the line (entry %d)", ks[0][1], logged)
	}
}

func TestNoKeepWithoutOffset(t *testing.T) {
	kc := &keepConn{fakeConn: newFakeConn(), log: &memLog{}}
	s := runKeep(t, kc)
	waitFor(t, s, isState(Connected))
	kc.lines <- conn.Line{Text: "no offset"}
	waitFor(t, s, lineEvent("no offset"))
	if ks := kc.keeps(); len(ks) != 0 {
		t.Errorf("keeps %v for a line with no offset", ks)
	}
}

func TestResumedSkipsLogin(t *testing.T) {
	kc := &keepConn{fakeConn: newFakeConn(), log: &memLog{}, resumed: true}
	s := runKeep(t, kc)
	waitFor(t, s, isState(Connected))
	kc.lines <- conn.Line{Text: "still here"}
	waitFor(t, s, lineEvent("still here"))
	if sent := kc.Sent(); len(sent) != 0 {
		t.Errorf("a resumed connection sent %q", sent)
	}
	for _, l := range kc.log.Texts() {
		if strings.Contains(l, upTo(str.SessionConnected(mark, 0))) {
			t.Errorf("a resumed connection logged %q", l)
		}
	}
	if len(kc.log.sessions) != 0 {
		t.Errorf("a resumed connection started a new log session")
	}
}

func TestResumedThenDroppedLogsInNextTime(t *testing.T) {
	kc := &keepConn{fakeConn: newFakeConn(), log: &memLog{}, resumed: true}
	next := newFakeConn()
	s := runKeep(t, kc, next)
	waitFor(t, s, isState(Connected))
	kc.Close()
	waitFor(t, s, isState(Connected))
	eventually(t, "never logged in", func() bool { return len(next.Sent()) == 1 })
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal(what)
		}
	}
}

// detachedErr is a connection left to another tab (wsdial.DetachAll).
type detachedErr struct{}

func (detachedErr) Error() string  { return "left to another tab" }
func (detachedErr) Detached() bool { return true }

// A connection left to another tab isn't a disconnect: nothing is logged
// (the other tab's scrollback would show it), and the session doesn't
// redial.
func TestDetachedIsNoDisconnect(t *testing.T) {
	fc := newFakeConn()
	fc.err = detachedErr{}
	log := &memLog{}
	var dials atomic.Int32
	ch := kit
	ch.Login = ""
	s := New(Options{Char: ch, Log: log, Backoff: func(int) time.Duration { return time.Millisecond },
		Dial: func(context.Context) (LineConn, error) { dials.Add(1); return fc, nil }})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, isState(Connected))
	n := len(log.Texts())
	fc.Close()
	waitFor(t, s, isState(Disconnected))
	time.Sleep(100 * time.Millisecond)
	if got := log.Texts(); len(got) != n {
		t.Errorf("logged %q when the connection was left to another tab", got[n:])
	}
	if n := dials.Load(); n != 1 {
		t.Errorf("dialed %d times, want 1", n)
	}
}
