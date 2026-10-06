package bridge

import (
	"bytes"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// probe reports the keys and sizes it sees, and quits on "q".
type probe struct{ got chan tea.Msg }

func (p probe) Init() tea.Cmd { return nil }

func (p probe) Update(m tea.Msg) (tea.Model, tea.Cmd) {
	switch m := m.(type) {
	case tea.KeyPressMsg:
		p.got <- m
		if m.String() == "q" {
			return p, tea.Quit
		}
	case tea.WindowSizeMsg:
		p.got <- m
	}
	return p, nil
}

func (p probe) View() tea.View { return tea.NewView("probe view") }

type screen struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *screen) write(p []byte) { s.mu.Lock(); s.buf.Write(p); s.mu.Unlock() }

// start runs a probe on a bridge and returns its message channel and a
// wait func that returns once the program has exited.
func start(t *testing.T, b *Bridge) (chan tea.Msg, func()) {
	t.Helper()
	got := make(chan tea.Msg, 64)
	p := tea.NewProgram(probe{got}, b.Options(80, 24)...)
	b.Attach(p)
	done := make(chan struct{})
	go func() { p.Run(); close(done) }()
	t.Cleanup(func() { p.Kill() })
	return got, func() {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("program didn't exit")
		}
	}
}

// next returns the next message of type T, skipping others.
func next[T tea.Msg](t *testing.T, got chan tea.Msg) T {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case m := <-got:
			if v, ok := m.(T); ok {
				return v
			}
		case <-timeout:
			var zero T
			t.Fatalf("no %T", zero)
			return zero
		}
	}
}

func TestInputArrivesInOrder(t *testing.T) {
	b := New(func([]byte) {})
	b.Input("x") // before the program starts: kept, not dropped
	got, _ := start(t, b)
	b.Input("a")
	b.Input("bc")
	var keys []string
	for range 4 {
		keys = append(keys, next[tea.KeyPressMsg](t, got).String())
	}
	if want := "x a b c"; joined(keys) != want {
		t.Errorf("keys %q, want %q", joined(keys), want)
	}
}

func TestStartSizeAndResize(t *testing.T) {
	b := New(func([]byte) {})
	got, _ := start(t, b)
	if m := next[tea.WindowSizeMsg](t, got); m.Width != 80 || m.Height != 24 {
		t.Errorf("start size %v", m)
	}
	b.Resize(100, 40)
	if m := next[tea.WindowSizeMsg](t, got); m.Width != 100 || m.Height != 40 {
		t.Errorf("resize %v", m)
	}
}

func TestResizeBeforeAttachDoesNotBlock(t *testing.T) {
	b := New(func([]byte) {})
	done := make(chan struct{})
	go func() { b.Resize(1, 1); b.Resize(2, 2); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Resize blocked")
	}
}

func TestOutputReachesScreen(t *testing.T) {
	var s screen
	b := New(s.write)
	got, wait := start(t, b)
	next[tea.WindowSizeMsg](t, got)
	b.Input("q")
	wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !bytes.Contains(s.buf.Bytes(), []byte("probe view")) {
		t.Errorf("screen %q", s.buf.String())
	}
}

func joined(ss []string) string {
	var b bytes.Buffer
	for i, s := range ss {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(s)
	}
	return b.String()
}
