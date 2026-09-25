package session

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"kiln/internal/config"
	"kiln/internal/conn"
	"kiln/internal/logstore"
)

// fakeConn is a scripted LineConn.
type fakeConn struct {
	lines     chan string
	err       error
	mu        sync.Mutex
	sent      []string
	closeOnce sync.Once
}

func newFakeConn(lines ...string) *fakeConn {
	f := &fakeConn{lines: make(chan string, len(lines)+1)}
	for _, l := range lines {
		f.lines <- l
	}
	return f
}
func (f *fakeConn) Lines() <-chan string { return f.lines }
func (f *fakeConn) Err() error           { return f.err }
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
	mu      sync.Mutex
	entries []logstore.Entry
	fail    error
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

var kit = config.Character{World: "fm", ID: "kit", Name: "Kit", Host: "h", Port: 1, Login: "connect {name} {password}"}

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
	if !containsPrefix(log.Texts(), "* no saved password for fm/kit") {
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
	if !containsPrefix(log.Texts(), "* disconnected (connection reset); retrying in") {
		t.Errorf("log = %q", log.Texts())
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
	if !contains(log.Texts(), "* disconnected (quit)") {
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
