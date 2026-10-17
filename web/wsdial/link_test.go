//go:build !js

package wsdial

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/relay"
	"github.com/latrani/Kiln/internal/str"
)

// world is a fake MUCK the test drives: it records what it reads and
// counts the connections it takes.
type world struct {
	port  int
	conns chan wconn
	n     atomic.Int32
	mu    sync.Mutex
	read  bytes.Buffer
}

func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{conns: make(chan wconn, 4)}
	ln := plainListener(t)
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			w.n.Add(1)
			wc := wconn{c, make(chan struct{})}
			w.conns <- wc
			go func() {
				defer close(wc.done)
				b := make([]byte, 4096)
				for {
					n, err := c.Read(b)
					w.mu.Lock()
					w.read.Write(b[:n])
					w.mu.Unlock()
					if err != nil {
						return
					}
				}
			}()
		}
	}()
	w.port = ln.Addr().(*net.TCPAddr).Port
	return w
}

// A wconn is the world's end of a connection; done closes when the relay
// closes it.
type wconn struct {
	net.Conn
	done chan struct{}
}

func (w *world) conn(t *testing.T) wconn {
	t.Helper()
	select {
	case c := <-w.conns:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("the relay never reached the world")
	}
	return wconn{}
}

func (w *world) got() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.read.String()
}

// swappable serves whichever relay is current, so a test can restart it.
type swappable struct{ s atomic.Pointer[relay.Server] }

func (h *swappable) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.s.Load().ServeHTTP(w, r) }

// holdRelay starts a relay for w's world (plaintext) with extra settings
// and returns its URL and a function that restarts it, forgetting every
// hold.
func holdRelay(t *testing.T, w *world, settings string, dial func(context.Context, string, string) (net.Conn, error)) (string, func()) {
	t.Helper()
	cfg, err := relay.ParseConfig("relay.toml", []byte(settings+fmt.Sprintf("[[world]]\nhost = \"127.0.0.1\"\nports = [%d]\nplaintext = true\n", w.port)))
	if err != nil {
		t.Fatal(err)
	}
	start := func() *relay.Server { s := &relay.Server{Dial: dial}; s.SetConfig(cfg); return s }
	h := &swappable{}
	h.s.Store(start())
	hs := httptest.NewServer(h)
	t.Cleanup(hs.Close)
	return "ws" + strings.TrimPrefix(hs.URL, "http") + "/kiln/relay", func() { h.s.Store(start()) }
}

func dialWorld(t *testing.T, u string, w *world) *conn.Conn {
	t.Helper()
	c, err := conn.Dial(context.Background(), conn.Options{Host: "127.0.0.1", Port: w.port, DialContext: Dialer(u, false)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// lines reads n lines from c.
func lines(t *testing.T, c *conn.Conn, n int) []conn.Line {
	t.Helper()
	var got []conn.Line
	for len(got) < n {
		select {
		case l, ok := <-c.Lines():
			if !ok {
				t.Fatalf("closed after %+v: %v", got, c.Err())
			}
			got = append(got, l)
		case <-time.After(5 * time.Second):
			t.Fatalf("after %+v: nothing more", got)
		}
	}
	return got
}

func isResumed(l conn.Line) bool {
	return l.Note && strings.HasPrefix(l.Text, upTo(str.ConnResumed(mark)))
}

// texts is the world's lines in ls, notes left out.
func texts(ls []conn.Line) []string {
	var out []string
	for _, l := range ls {
		if !l.Note {
			out = append(out, l.Text)
		}
	}
	return out
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !ok(); {
		if time.Now().After(deadline) {
			t.Fatal(what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// fast makes resuming quick for a test, and puts it back after.
func fast(t *testing.T, delays ...time.Duration) {
	t.Helper()
	old, oldWake, oldLost := retryDelays, wakeWait, lostAfter
	if len(delays) == 0 {
		delays = []time.Duration{0, 50 * time.Millisecond}
	}
	retryDelays, wakeWait, lostAfter = delays, 300*time.Millisecond, time.Hour
	t.Cleanup(func() { retryDelays, wakeWait, lostAfter = old, oldWake, oldLost })
}

func TestDropMidStreamDown(t *testing.T) {
	fast(t)
	w := newWorld(t)
	u, _ := holdRelay(t, w, "", nil)
	c := dialWorld(t, u, w)
	wc := w.conn(t)
	wc.Write([]byte("1\r\n2\r\n3\r\n"))
	got := lines(t, c, 3)
	DropAll()
	wc.Write([]byte("4\r\n5\r\n6\r\n"))
	got = append(got, lines(t, c, 4)...)
	if want := []string{"1", "2", "3", "4", "5", "6"}; strings.Join(texts(got), " ") != strings.Join(want, " ") {
		t.Errorf("lines %q, want %q once each", texts(got), want)
	}
	i := len(got) - 1
	for i >= 0 && !isResumed(got[i]) {
		i--
	}
	if i < 3 {
		t.Errorf("no resumed note after line 3: %+v", got)
	}
	if w.n.Load() != 1 {
		t.Errorf("the world took %d connections, want 1", w.n.Load())
	}
}

func TestWritesWhileDownAreSentOnce(t *testing.T) {
	fast(t)
	w := newWorld(t)
	u, _ := holdRelay(t, w, "", nil)
	c := dialWorld(t, u, w)
	w.conn(t)
	DropAll()
	if err := c.Send("look"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the world never read it", func() bool { return w.got() == "look\r\n" })
	time.Sleep(300 * time.Millisecond)
	if got := w.got(); got != "look\r\n" {
		t.Errorf("the world read %q, want look once", got)
	}
}

// dropAfterWrite is a world connection that kills every page's WebSocket
// once the relay has written to the world, before the relay acks.
type dropAfterWrite struct{ net.Conn }

func (d dropAfterWrite) Write(p []byte) (int, error) {
	n, err := d.Conn.Write(p)
	DropAll()
	return n, err
}

func TestDropMidStreamUp(t *testing.T) {
	fast(t)
	w := newWorld(t)
	u, _ := holdRelay(t, w, "", func(ctx context.Context, network, addr string) (net.Conn, error) {
		var d net.Dialer
		c, err := d.DialContext(ctx, network, addr)
		return dropAfterWrite{c}, err
	})
	c := dialWorld(t, u, w)
	w.conn(t)
	c.Send("one")
	eventually(t, "never resumed", func() bool { return isResumed(lines(t, c, 1)[0]) })
	c.Send("two")
	eventually(t, "the world never read two", func() bool { return strings.Contains(w.got(), "two") })
	time.Sleep(300 * time.Millisecond)
	if got := w.got(); got != "one\r\ntwo\r\n" {
		t.Errorf("the world read %q, want each line once", got)
	}
}

// silencer wraps sockets; while on, a socket reads nothing and writes go
// nowhere, like one a suspended phone left open.
type silencer struct {
	on atomic.Bool
}

type silentSocket struct {
	socket
	s *silencer
}

func (q silentSocket) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	for {
		typ, b, err := q.socket.Read(ctx)
		if err != nil || !q.s.on.Load() {
			return typ, b, err
		}
	}
}

func (q silentSocket) Write(ctx context.Context, typ websocket.MessageType, p []byte) error {
	if q.s.on.Load() {
		return nil
	}
	return q.socket.Write(ctx, typ, p)
}

func TestWakeResumesASilentSocket(t *testing.T) {
	fast(t)
	w := newWorld(t)
	u, _ := holdRelay(t, w, "", nil)
	q := &silencer{}
	open := func(ctx context.Context, url string) (socket, error) {
		s, err := openSocket(ctx, url)
		if err != nil {
			return nil, err
		}
		return silentSocket{s, q}, nil
	}
	c, err := conn.Dial(context.Background(), conn.Options{Host: "127.0.0.1", Port: w.port,
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			t, _ := target(u, addr, false)
			return dialLink(ctx, open, t, resumeTarget(u))
		}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	w.conn(t)
	q.on.Store(true)
	start := time.Now()
	go func() { time.Sleep(50 * time.Millisecond); q.on.Store(false) }() // the next socket works
	Wake()
	if l := lines(t, c, 1)[0]; !isResumed(l) {
		t.Fatalf("got %+v, want a resumed note", l)
	}
	if d := time.Since(start); d > wakeWait+2*time.Second {
		t.Errorf("resumed after %v", d)
	}
}

func TestRelayRestartLetsGo(t *testing.T) {
	fast(t)
	w := newWorld(t)
	u, restart := holdRelay(t, w, "", nil)
	c := dialWorld(t, u, w)
	w.conn(t)
	restart()
	DropAll()
	select {
	case _, ok := <-c.Lines():
		for ok {
			_, ok = <-c.Lines()
		}
	case <-time.After(5 * time.Second):
		t.Fatal("never let go")
	}
	if c.Err() == nil || c.Err().Error() != str.ConnRelayLetGo() {
		t.Errorf("Err = %v, want %q", c.Err(), str.ConnRelayLetGo())
	}
}

func TestMissedNote(t *testing.T) {
	fast(t, time.Second)
	w := newWorld(t)
	u, _ := holdRelay(t, w, "max_buffer_mb = 1\n", nil)
	c := dialWorld(t, u, w)
	wc := w.conn(t)
	DropAll()
	line := strings.Repeat("x", 1017)
	for i := range 2048 {
		fmt.Fprintf(wc, "%04d %s\r\n", i, line)
	}
	// The relay drops about half: a megabyte's worth, its chunks' cost
	// counted too.
	missed := 0
	var after []string
	for len(after) < 3 {
		l := lines(t, c, 1)[0]
		if l.Note && missed == 0 {
			for n := 900; n <= 1100; n++ {
				if l.Text == str.ConnMissed(n) {
					missed = n
				}
			}
		} else if !l.Note && missed > 0 {
			after = append(after, l.Text[:4])
		}
	}
	if want := fmt.Sprintf("%04d %04d %04d", missed, missed+1, missed+2); strings.Join(after, " ") != want {
		t.Errorf("after missing %d: %q, want the lines past the drop, in order", missed, after)
	}
}

func TestCloseSaysClose(t *testing.T) {
	fast(t)
	w := newWorld(t)
	u, _ := holdRelay(t, w, "", nil)
	c := dialWorld(t, u, w)
	wc := w.conn(t)
	c.Close()
	select {
	case <-wc.done:
	case <-time.After(2 * time.Second):
		t.Error("closing didn't let the world go")
	}
}

// The world hanging up ends the link without anyone calling Close (a
// session doesn't), and must free the relay's slot.
func TestWorldClosingFreesTheSlot(t *testing.T) {
	fast(t)
	w := newWorld(t)
	u, _ := holdRelay(t, w, "max_per_ip = 1\n", nil)
	c, err := conn.Dial(context.Background(), conn.Options{Host: "127.0.0.1", Port: w.port, DialContext: Dialer(u, false)})
	if err != nil {
		t.Fatal(err)
	}
	w.conn(t).Close()
	for range c.Lines() {
	}
	eventually(t, "the slot stayed taken", func() bool {
		c, err := conn.Dial(context.Background(), conn.Options{Host: "127.0.0.1", Port: w.port, DialContext: Dialer(u, false)})
		if err == nil {
			c.Close()
		}
		return err == nil
	})
}

// Two wakes at once (visibilitychange and pageshow both fire on a
// bfcache restore) must not drop a socket that answers.
func TestDoubleWakeKeepsAHealthySocket(t *testing.T) {
	fast(t)
	w := newWorld(t)
	u, _ := holdRelay(t, w, "", nil)
	c := dialWorld(t, u, w)
	w.conn(t)
	for range 3 {
		for range 10 {
			go Wake()
		}
		time.Sleep(wakeWait + 300*time.Millisecond)
	}
	select {
	case l := <-c.Lines():
		t.Errorf("got %+v, want nothing: a healthy socket was dropped", l)
	default:
	}
}
