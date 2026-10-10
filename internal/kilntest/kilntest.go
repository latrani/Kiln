// Package kilntest holds fakes the app and ui tests share: a scripted
// server connection, a log that keeps nothing, and a way to write logs.
// Only tests import it.
package kilntest

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/logstore"
)

// Conn is a scripted server connection: Feed puts a line on it, Sent is
// what was sent, Sizes the window sizes reported.
type Conn struct {
	lines     chan string
	prompts   chan string
	mu        sync.Mutex
	sent      []string
	sizes     [][2]int
	closeOnce sync.Once
}

func NewConn() *Conn {
	return &Conn{lines: make(chan string, 100), prompts: make(chan string, 1)}
}

func (c *Conn) Lines() <-chan string   { return c.lines }
func (c *Conn) Prompts() <-chan string { return c.prompts }
func (c *Conn) Err() error             { return nil }
func (c *Conn) Close() error           { c.closeOnce.Do(func() { close(c.lines) }); return nil }

// Feed has the server send line.
func (c *Conn) Feed(line string) { c.lines <- line }

// FeedPrompt has the server send an unterminated prompt.
func (c *Conn) FeedPrompt(p string) { c.prompts <- p }

func (c *Conn) Send(l string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, l)
	return nil
}

func (c *Conn) Resize(w, h int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sizes = append(c.sizes, [2]int{w, h})
	return nil
}

func (c *Conn) Sent() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.sent)
}

func (c *Conn) Sizes() [][2]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.sizes)
}

// NopLog is a session log that keeps nothing.
type NopLog struct{}

func (NopLog) Append(logstore.Entry) error { return nil }

// WriteLog logs lines one a minute from start where l says: a line
// starting "> " as sent, the rest as received.
func WriteLog(t testing.TB, l logstore.Layout, start time.Time, lines ...string) {
	t.Helper()
	w := logstore.NewWriter(l)
	defer w.Close()
	for i, s := range lines {
		dir := logstore.In
		if strings.HasPrefix(s, "> ") {
			dir, s = logstore.Out, strings.TrimPrefix(s, "> ")
		}
		if err := w.Append(logstore.Entry{Time: start.Add(time.Duration(i) * time.Minute), Dir: dir, Text: s}); err != nil {
			t.Fatal(err)
		}
	}
}
