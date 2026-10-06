// Package bridge runs a Bubble Tea program on a terminal that isn't a
// TTY: xterm.js in a browser tab. Keystrokes come in as strings, screen
// bytes go out through a callback, and resizes become WindowSizeMsg.
// It's plain Go (no syscall/js), so it's tested natively; cmd/kiln-web
// connects it to the page.
package bridge

import (
	"io"
	"sync"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
)

// Bridge connects one program to one terminal.
type Bridge struct {
	out    func([]byte)
	pr     *io.PipeReader
	pw     *io.PipeWriter
	mu     sync.Mutex
	cond   *sync.Cond
	queue  []string
	closed bool
	size   chan [2]int // holds at most the latest size
	prog   atomic.Pointer[tea.Program]
}

// New makes a bridge whose screen output goes to out. out must copy p
// before returning.
func New(out func([]byte)) *Bridge {
	b := &Bridge{out: out, size: make(chan [2]int, 1)}
	b.pr, b.pw = io.Pipe()
	b.cond = sync.NewCond(&b.mu)
	go b.pump()
	go b.resizer()
	return b
}

// Options are the tea.NewProgram options that put the program on this
// bridge, starting at cols×rows. There's no environment to detect the
// terminal from, so they say what xterm.js is.
func (b *Bridge) Options(cols, rows int) []tea.ProgramOption {
	return []tea.ProgramOption{
		tea.WithInput(b.pr),
		tea.WithOutput(writerFunc(func(p []byte) (int, error) { b.out(p); return len(p), nil })),
		tea.WithWindowSize(cols, rows),
		tea.WithEnvironment([]string{"TERM=xterm-256color"}), //str:ok
		tea.WithColorProfile(colorprofile.TrueColor),
	}
}

// Attach tells the bridge which program to send resizes to.
func (b *Bridge) Attach(p *tea.Program) { b.prog.Store(p) }

// Input queues keystrokes. It never blocks: JS calls it from an event
// handler, and blocking there would stall the page.
func (b *Bridge) Input(s string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.queue = append(b.queue, s)
		b.cond.Signal()
	}
}

// Resize reports a new terminal size. It never blocks; if sizes arrive
// faster than the program takes them, only the latest is sent. Call it
// from one goroutine (JS has one).
func (b *Bridge) Resize(cols, rows int) {
	select {
	case <-b.size:
	default:
	}
	b.size <- [2]int{cols, rows}
}

// Close ends input: the program reads EOF.
func (b *Bridge) Close() {
	b.mu.Lock()
	b.closed = true
	b.cond.Signal()
	b.mu.Unlock()
}

func (b *Bridge) pump() {
	for {
		b.mu.Lock()
		for len(b.queue) == 0 && !b.closed {
			b.cond.Wait()
		}
		if len(b.queue) == 0 {
			b.mu.Unlock()
			b.pw.Close()
			return
		}
		s := b.queue[0]
		b.queue = b.queue[1:]
		b.mu.Unlock()
		if _, err := io.WriteString(b.pw, s); err != nil {
			return
		}
	}
}

func (b *Bridge) resizer() {
	for sz := range b.size {
		if p := b.prog.Load(); p != nil {
			p.Send(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		}
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
