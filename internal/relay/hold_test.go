package relay

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/relay/wire"
	"github.com/latrani/Kiln/internal/str"
)

// page is a test client speaking the relay's protocol as the web build
// does, without acking unless told to.
type page struct {
	t     *testing.T
	c     *websocket.Conn
	token string
}

// frame is one frame from the relay: a control message, or data.
type frame struct {
	msg  wire.Msg
	at   time.Time
	data []byte
	err  error
}

func (f frame) isData() bool { return f.msg.T == "" && f.err == nil }

// openPage opens a connection to 127.0.0.1:port and reads its open
// message.
func openPage(t *testing.T, base string, port int) *page {
	t.Helper()
	p := &page{t: t, c: open(t, base, "127.0.0.1", port)}
	f := p.next()
	if f.msg.T != wire.Open {
		t.Fatalf("first frame %+v, want open", f)
	}
	p.token = f.msg.Token
	return p
}

// resumePage attaches to the held connection token, having received got
// bytes, from a client claiming to be at xff (through a trusted proxy)
// unless it's "".
func resumePage(t *testing.T, base, token string, got int64, xff string) *page {
	t.Helper()
	var opts *websocket.DialOptions
	if xff != "" {
		opts = &websocket.DialOptions{HTTPHeader: http.Header{"X-Forwarded-For": {xff}}}
	}
	c, _, err := websocket.Dial(context.Background(), base+"/?resume=1", opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	p := &page{t: t, c: c, token: token}
	p.send(wire.Msg{T: wire.Resume, Token: token, Got: got})
	return p
}

func (p *page) send(m wire.Msg) {
	p.t.Helper()
	if err := p.c.Write(context.Background(), websocket.MessageText, wire.Encode(m)); err != nil {
		p.t.Fatal(err)
	}
}

func (p *page) write(s string) {
	p.t.Helper()
	if err := p.c.Write(context.Background(), websocket.MessageBinary, []byte(s)); err != nil {
		p.t.Fatal(err)
	}
}

// next reads one frame, waiting up to 5 s.
func (p *page) next() frame {
	p.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	typ, b, err := p.c.Read(ctx)
	if err != nil {
		return frame{err: err}
	}
	if typ == websocket.MessageText {
		m, err := wire.Decode(b)
		if err != nil {
			p.t.Fatalf("bad control frame %q", b)
		}
		return frame{msg: m}
	}
	at, d, err := wire.ReadDown(b)
	if err != nil {
		p.t.Fatal(err)
	}
	return frame{at: at, data: d}
}

// expect reads frames until one of type typ, skipping acks.
func (p *page) expect(typ string) wire.Msg {
	p.t.Helper()
	for {
		f := p.next()
		if f.err != nil {
			p.t.Fatalf("waiting for %s: %v", typ, f.err)
		}
		if f.msg.T == typ {
			return f.msg
		}
		if f.msg.T != wire.Ack {
			p.t.Fatalf("waiting for %s, got %+v", typ, f)
		}
	}
}

// data reads data frames until n bytes have come, skipping acks, and
// returns them with the last frame's time.
func (p *page) data(n int) (string, time.Time) {
	p.t.Helper()
	var b []byte
	var at time.Time
	for len(b) < n {
		f := p.next()
		switch {
		case f.err != nil:
			p.t.Fatalf("after %q: %v", b, f.err)
		case f.isData():
			b, at = append(b, f.data...), f.at
		case f.msg.T != wire.Ack:
			p.t.Fatalf("after %q: %+v", b, f.msg)
		}
	}
	return string(b), at
}

// muck is a fake world the test drives: it records what it reads and
// writes what the test says.
type muck struct {
	port  int
	conns chan *mconn
	mu    sync.Mutex
	read  bytes.Buffer
}

func newMuck(t *testing.T) *muck {
	m := &muck{conns: make(chan *mconn, 4)}
	m.port = serveEach(t, tcpListener(t), func(c net.Conn) {
		mc := &mconn{Conn: c, done: make(chan struct{})}
		m.conns <- mc
		defer close(mc.done)
		b := make([]byte, 4096)
		for {
			n, err := c.Read(b)
			m.mu.Lock()
			m.read.Write(b[:n])
			m.mu.Unlock()
			if err != nil {
				return
			}
		}
	})
	return m
}

// An mconn is the world's end of one connection; done closes when the
// relay closes it.
type mconn struct {
	net.Conn
	done chan struct{}
}

func (m *muck) conn(t *testing.T) *mconn {
	t.Helper()
	select {
	case c := <-m.conns:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("the relay never reached the world")
	}
	return nil
}

func (m *muck) got() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.read.String()
}

// closed reports whether the relay has closed c, waiting up to wait.
func closed(c *mconn, wait time.Duration) bool {
	select {
	case <-c.done:
		return true
	case <-time.After(wait):
		return false
	}
}

// holdRelay is a relay for m's world with extra settings, pinging fast.
func holdRelay(t *testing.T, m *muck, settings string) (*Server, string, *logLines) {
	t.Helper()
	s, base, logs := relay(t, settings+world(m.port, ""))
	s.pingEvery, s.pingTimeout = 50*time.Millisecond, 100*time.Millisecond
	return s, base, logs
}

func (l *logLines) has(prefix string) bool {
	return slices.ContainsFunc(l.all(), func(s string) bool { return strings.HasPrefix(s, prefix) })
}

func (l *logLines) count(prefix string) int {
	n := 0
	for _, s := range l.all() {
		if strings.HasPrefix(s, prefix) {
			n++
		}
	}
	return n
}

var local = netip.MustParseAddr("127.0.0.1")

func (m *muck) addr() string { return net.JoinHostPort("127.0.0.1", strconv.Itoa(m.port)) }

func TestOpenSendsAToken(t *testing.T) {
	m := newMuck(t)
	_, base, _ := holdRelay(t, m, "")
	p := openPage(t, base, m.port)
	if len(p.token) != 43 {
		t.Errorf("token %q is %d characters, want 43", p.token, len(p.token))
	}
	if q := openPage(t, base, m.port); q.token == p.token {
		t.Error("two connections got the same token")
	}
}

func TestDetachKeepsTheMuck(t *testing.T) {
	m := newMuck(t)
	s, base, logs := holdRelay(t, m, "")
	p := openPage(t, base, m.port)
	w := m.conn(t)
	p.c.CloseNow()
	eventually(t, "never held", func() bool { return logs.has(str.RelayHeld(local, m.addr())) })
	gone := time.Now()
	w.Write([]byte("while away\r\n"))
	if closed(w, 200*time.Millisecond) {
		t.Fatal("the relay let the world go when the WebSocket died")
	}
	eventually(t, "never buffered", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.buffered == len("while away\r\n")+chunkCost
	})
	q := resumePage(t, base, p.token, 0, "")
	r := q.expect(wire.Resumed)
	if r.Got != 0 || r.Away <= 0 || r.At != int64(len("while away\r\n")) {
		t.Errorf("resumed %+v, want got 0, at the buffer's end, and some time away", r)
	}
	d, at := q.data(len("while away\r\n"))
	if d != "while away\r\n" || at.Before(gone.Truncate(time.Millisecond)) {
		t.Errorf("data %q at %v; want the line, stamped after %v", d, at, gone)
	}
	if !logs.has(upTo(str.RelayResumed(local, m.addr(), mark))) {
		t.Errorf("no resume logged: %q", logs.all())
	}
}

func TestResumeResendsUnacked(t *testing.T) {
	m := newMuck(t)
	_, base, _ := holdRelay(t, m, "")
	p := openPage(t, base, m.port)
	w := m.conn(t)
	w.Write([]byte("one\r\n"))
	p.data(5)
	p.c.CloseNow()
	q := resumePage(t, base, p.token, 0, "")
	q.expect(wire.Resumed)
	if d, _ := q.data(5); d != "one\r\n" {
		t.Errorf("resent %q", d)
	}
	w.Write([]byte("two\r\n"))
	if d, _ := q.data(5); d != "two\r\n" {
		t.Errorf("then %q, want two (once)", d)
	}
}

func TestAckedIsNotResent(t *testing.T) {
	m := newMuck(t)
	_, base, _ := holdRelay(t, m, "")
	p := openPage(t, base, m.port)
	w := m.conn(t)
	w.Write([]byte("one\r\n"))
	p.data(5)
	p.send(wire.Msg{T: wire.Ack, Got: 5})
	w.Write([]byte("two\r\n"))
	p.data(5)
	p.c.CloseNow()
	q := resumePage(t, base, p.token, 5, "")
	q.expect(wire.Resumed)
	if d, _ := q.data(5); d != "two\r\n" {
		t.Errorf("resent %q, want only two", d)
	}
}

func TestResumeTellsTheUpOffset(t *testing.T) {
	m := newMuck(t)
	_, base, _ := holdRelay(t, m, "")
	p := openPage(t, base, m.port)
	m.conn(t)
	p.write("look\n")
	eventually(t, "the world never read it", func() bool { return m.got() == "look\n" })
	p.c.CloseNow()
	q := resumePage(t, base, p.token, 0, "")
	if r := q.expect(wire.Resumed); r.Got != 5 {
		t.Errorf("resumed got %d, want 5", r.Got)
	}
}

func TestRelayAcksWhatItWrote(t *testing.T) {
	m := newMuck(t)
	_, base, _ := holdRelay(t, m, "")
	p := openPage(t, base, m.port)
	m.conn(t)
	p.write("look\n")
	if a := p.expect(wire.Ack); a.Got != 5 {
		t.Errorf("ack %d, want 5", a.Got)
	}
}

func TestPingIsAnswered(t *testing.T) {
	m := newMuck(t)
	_, base, _ := holdRelay(t, m, "")
	p := openPage(t, base, m.port)
	p.send(wire.Msg{T: wire.Ping})
	p.expect(wire.Pong)
}

func TestUnansweredPingDetaches(t *testing.T) {
	m := newMuck(t)
	s, base, logs := holdRelay(t, m, "")
	open(t, base, "127.0.0.1", m.port) // never reads, so never answers a ping
	w := m.conn(t)
	eventually(t, "never held", func() bool { return logs.has(str.RelayHeld(local, m.addr())) })
	if closed(w, 200*time.Millisecond) || s.inUse() != 1 {
		t.Error("a silent page's world connection was let go")
	}
}

func TestCloseEndsTheMuck(t *testing.T) {
	m := newMuck(t)
	s, base, _ := holdRelay(t, m, "")
	p := openPage(t, base, m.port)
	w := m.conn(t)
	p.send(wire.Msg{T: wire.Close})
	if !closed(w, 2*time.Second) {
		t.Fatal("close didn't end the world connection")
	}
	eventually(t, "slot kept", func() bool { return s.inUse() == 0 })
	if code := closeCode(t, resumePage(t, base, p.token, 0, "").c); code != wire.CodeGone {
		t.Errorf("resume after close: code %d, want %d", code, wire.CodeGone)
	}
}

func TestUnknownTokenIsGone(t *testing.T) {
	m := newMuck(t)
	_, base, logs := holdRelay(t, m, "")
	if code := closeCode(t, resumePage(t, base, "nope", 0, "").c); code != wire.CodeGone {
		t.Errorf("code %d, want %d", code, wire.CodeGone)
	}
	if !logs.has(str.RelayRefusedGone(local)) {
		t.Errorf("no refusal logged: %q", logs.all())
	}
}

func TestHoldExpires(t *testing.T) {
	m := newMuck(t)
	s, base, logs := holdRelay(t, m, "hold = \"200ms\"\nmax_per_ip = 1\n")
	p := openPage(t, base, m.port)
	w := m.conn(t)
	p.c.CloseNow()
	if !closed(w, 3*time.Second) {
		t.Fatal("the hold never ran out")
	}
	eventually(t, "slot kept", func() bool { return s.inUse() == 0 })
	eventually(t, "no expiry logged", func() bool { return logs.has(upTo(str.RelayHoldExpired(local, m.addr(), mark))) })
	if code := closeCode(t, resumePage(t, base, p.token, 0, "").c); code != wire.CodeGone {
		t.Errorf("resume after expiry: code %d, want %d", code, wire.CodeGone)
	}
	openPage(t, base, m.port) // its slot is free
}

func TestEndAfterBuffered(t *testing.T) {
	m := newMuck(t)
	_, base, _ := holdRelay(t, m, "")
	p := openPage(t, base, m.port)
	w := m.conn(t)
	p.c.CloseNow()
	time.Sleep(50 * time.Millisecond)
	w.Write([]byte("bye\r\n"))
	w.Close()
	q := resumePage(t, base, p.token, 0, "")
	q.expect(wire.Resumed)
	if d, _ := q.data(5); d != "bye\r\n" {
		t.Errorf("data %q", d)
	}
	if e := q.expect(wire.End); e.At != 5 {
		t.Errorf("end at %d, want 5", e.At)
	}
}

func TestNewerAttachWins(t *testing.T) {
	m := newMuck(t)
	_, base, _ := holdRelay(t, m, "")
	p := openPage(t, base, m.port)
	w := m.conn(t)
	start := time.Now()
	q := resumePage(t, base, p.token, 0, "")
	q.expect(wire.Resumed)
	if d := time.Since(start); d > time.Second {
		t.Errorf("resumed after %v: replacing a silent attachment shouldn't wait on it", d)
	}
	if code := closeCode(t, p.c); code != wire.CodeReplaced {
		t.Errorf("older: code %d, want %d", code, wire.CodeReplaced)
	}
	w.Write([]byte("hi\r\n"))
	if d, _ := q.data(4); d != "hi\r\n" {
		t.Errorf("newer got %q", d)
	}
}

// fill has the world write n bytes in 1 KiB lines, numbered.
func fill(w *mconn, n int) {
	line := strings.Repeat("x", 1017)
	for i := 0; i < n/1024; i++ {
		fmt.Fprintf(w, "%04d %s\r\n", i%10000, line)
	}
}

func TestBufferCapDropsOldest(t *testing.T) {
	m := newMuck(t)
	_, base, logs := holdRelay(t, m, "max_buffer_mb = 1\n")
	p := openPage(t, base, m.port)
	w := m.conn(t)
	p.c.CloseNow()
	eventually(t, "never held", func() bool { return logs.has(str.RelayHeld(local, m.addr())) })
	fill(w, 2<<20)
	eventually(t, "never dropped", func() bool { return logs.has(str.RelayDropping(local, m.addr())) })
	time.Sleep(100 * time.Millisecond) // let the rest arrive
	q := resumePage(t, base, p.token, 0, "")
	q.expect(wire.Resumed)
	missed := q.expect(wire.Missed)
	if missed.Lines < 1024 || missed.Lines > 2048 {
		t.Errorf("missed %d lines, want about 1024", missed.Lines)
	}
	if missed.At != int64(missed.Lines)*1024 {
		t.Errorf("missed says data goes on from %d, want %d", missed.At, missed.Lines*1024)
	}
	d, _ := q.data(1024)
	if want := fmt.Sprintf("%04d ", missed.Lines); !strings.HasPrefix(d, want) {
		t.Errorf("data after the drop starts %q, want line %q", d[:5], want)
	}
	if n := logs.count(str.RelayDropping(local, m.addr())); n != 1 {
		t.Errorf("logged dropping %d times, want once", n)
	}
}

func TestTotalCapDropsFromTheBiggest(t *testing.T) {
	m := newMuck(t)
	_, base, logs := holdRelay(t, m, "max_buffer_mb = 4\nmax_total_buffer_mb = 3\n")
	small := openPage(t, base, m.port)
	ws := m.conn(t)
	big := openPage(t, base, m.port)
	wb := m.conn(t)
	small.c.CloseNow()
	big.c.CloseNow()
	eventually(t, "never held", func() bool { return logs.count(str.RelayHeld(local, m.addr())) == 2 })
	fill(ws, 1<<20)
	time.Sleep(100 * time.Millisecond)
	fill(wb, 3<<20)
	eventually(t, "never dropped", func() bool { return logs.has(str.RelayDropping(local, m.addr())) })
	time.Sleep(100 * time.Millisecond)
	q := resumePage(t, base, small.token, 0, "")
	q.expect(wire.Resumed)
	if f := q.next(); !f.isData() {
		t.Errorf("the smaller connection's first frame is %+v, want data (nothing dropped)", f.msg)
	}
	r := resumePage(t, base, big.token, 0, "")
	r.expect(wire.Resumed)
	r.expect(wire.Missed)
}

func TestResumeFromNewAddressMovesTheSlot(t *testing.T) {
	m := newMuck(t)
	_, base, _ := holdRelay(t, m, "trusted_proxies = [\"127.0.0.1\"]\nmax_per_ip = 1\n")
	dial := func(xff string) *websocket.Conn {
		c, _, err := websocket.Dial(context.Background(), fmt.Sprintf("%s/?host=127.0.0.1&port=%d", base, m.port),
			&websocket.DialOptions{HTTPHeader: http.Header{"X-Forwarded-For": {xff}}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.CloseNow() })
		return c
	}
	a := dial("192.0.2.1")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, b, err := a.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	open, _ := wire.Decode(b)
	a.CloseNow()
	q := resumePage(t, base, open.Token, 0, "192.0.2.2")
	q.expect(wire.Resumed)
	if _, b, err := dial("192.0.2.1").Read(ctx); err != nil || !strings.Contains(string(b), wire.Open) {
		t.Errorf("the old address is still counted: %q, %v", b, err)
	}
	if code := closeCode(t, dial("192.0.2.2")); code != wire.CodeTooMany {
		t.Errorf("new address: code %d, want %d", code, wire.CodeTooMany)
	}
}

func TestResumeIsNotANewSlot(t *testing.T) {
	m := newMuck(t)
	_, base, _ := holdRelay(t, m, "max_conns = 1\n")
	p := openPage(t, base, m.port)
	m.conn(t)
	p.c.CloseNow()
	q := resumePage(t, base, p.token, 0, "")
	q.expect(wire.Resumed)
}

// bare is a held connection with no sockets, for testing the buffer
// accounting directly.
func bare(s *Server) *held {
	h := &held{s: s, token: newToken(), world: "fm:1", ip: local}
	h.cond = sync.NewCond(&h.mu)
	s.mu.Lock()
	if s.holds == nil {
		s.holds = map[string]*held{}
	}
	s.holds[h.token] = h
	s.mu.Unlock()
	return h
}

// Over the total cap, the relay drops from the biggest hold that can
// drop, not just the biggest.
func TestTotalCapSkipsHoldsThatCantDrop(t *testing.T) {
	a, _ := ParseConfig("relay.toml", []byte("max_total_buffer_mb = 1\n"))
	s := &Server{}
	s.SetConfig(a)
	prompt := bytes.Repeat([]byte("x"), 60000) // no line end: can't drop
	for range 18 {
		s.take(bare(s), prompt)
	}
	b := bare(s)
	s.take(b, bytes.Repeat([]byte("x\n"), 5000))
	if b.buf.dropped == 0 {
		t.Error("the hold that could drop didn't")
	}
}

// A hold's timer from an earlier detach mustn't end a later one: detach,
// resume, detach again, and the first timer fires late.
func TestAnOldHoldTimerDoesNothing(t *testing.T) {
	a, _ := ParseConfig("relay.toml", []byte(""))
	s := &Server{}
	s.SetConfig(a)
	h := bare(s)
	h.tc, _ = net.Pipe()
	h.mu.Lock()
	h.gen = 2 // detached twice
	h.mu.Unlock()
	h.expire(1)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		t.Error("the first detach's timer let go of the second detach's hold")
	}
}

// A world that stops reading can't hold a resume up: writes to it give
// up, and the page hears the world went.
func TestStuckWorldDoesntBlockResume(t *testing.T) {
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	port := serveEach(t, tcpListener(t), func(net.Conn) { <-stop }) // never reads
	s, base, _ := relay(t, world(port, ""))
	s.worldWriteTimeout = 200 * time.Millisecond
	p := openPage(t, base, port)
	chunk := strings.Repeat("x", 64<<10)
	go func() {
		for range 256 { // far past what the kernel will buffer
			if p.c.Write(context.Background(), websocket.MessageBinary, []byte(chunk)) != nil {
				return
			}
		}
	}()
	time.Sleep(500 * time.Millisecond)
	p.c.CloseNow()
	q := resumePage(t, base, p.token, 0, "")
	done := make(chan frame, 1)
	go func() { done <- q.next() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the resume waited on a world that doesn't read")
	}
}

// beacon posts tokens as a closing page's sendBeacon does.
func beacon(t *testing.T, base string, tokens ...string) int {
	t.Helper()
	u := "http" + strings.TrimPrefix(base, "ws") + "/?short=1"
	resp, err := http.Post(u, "text/plain", strings.NewReader(strings.Join(tokens, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestBeaconShortensTheHold(t *testing.T) {
	m := newMuck(t)
	_, base, logs := holdRelay(t, m, "hold = \"1h\"\nshort_hold = \"200ms\"\n")
	p := openPage(t, base, m.port)
	w := m.conn(t)
	if code := beacon(t, base, p.token); code != http.StatusNoContent {
		t.Errorf("beacon: %d, want 204", code)
	}
	p.c.CloseNow()
	if !closed(w, 3*time.Second) {
		t.Fatal("a closed page's hold wasn't shortened")
	}
	eventually(t, "nothing logged", func() bool { return logs.has(upTo(str.RelayLettingGo(local, m.addr(), mark))) })
}

func TestBeaconForADetachedHold(t *testing.T) {
	m := newMuck(t)
	_, base, logs := holdRelay(t, m, "hold = \"1h\"\nshort_hold = \"200ms\"\n")
	p := openPage(t, base, m.port)
	w := m.conn(t)
	p.c.CloseNow()
	eventually(t, "never held", func() bool { return logs.has(str.RelayHeld(local, m.addr())) })
	beacon(t, base, p.token)
	if !closed(w, 3*time.Second) {
		t.Fatal("a detached hold wasn't shortened")
	}
}

func TestResumeClearsTheShortHold(t *testing.T) {
	m := newMuck(t)
	_, base, logs := holdRelay(t, m, "hold = \"1h\"\nshort_hold = \"200ms\"\n")
	p := openPage(t, base, m.port)
	w := m.conn(t)
	beacon(t, base, p.token)
	p.c.CloseNow()
	q := resumePage(t, base, p.token, 0, "")
	q.expect(wire.Resumed)
	q.c.CloseNow()
	eventually(t, "never held twice", func() bool { return logs.count(str.RelayHeld(local, m.addr())) == 2 })
	if closed(w, 500*time.Millisecond) {
		t.Error("a resumed hold kept the short hold")
	}
}

func TestBeaconIgnoresUnknownTokens(t *testing.T) {
	m := newMuck(t)
	_, base, _ := holdRelay(t, m, "")
	if code := beacon(t, base, "nope", ""); code != http.StatusNoContent {
		t.Errorf("beacon: %d, want 204", code)
	}
	resp, err := http.Get("http" + strings.TrimPrefix(base, "ws") + "/?short=1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("GET short=1: %d, want 400", resp.StatusCode)
	}
}

// A page resuming from behind what the relay still has (it acked more
// than it says it got) hears where the data starts, so its offsets stay
// right.
func TestResumeBehindTheBufferSaysWhere(t *testing.T) {
	m := newMuck(t)
	_, base, _ := holdRelay(t, m, "")
	p := openPage(t, base, m.port)
	w := m.conn(t)
	w.Write([]byte("one\r\n"))
	p.data(5)
	p.send(wire.Msg{T: wire.Ack, Got: 5})
	time.Sleep(100 * time.Millisecond)
	p.c.CloseNow()
	q := resumePage(t, base, p.token, 0, "")
	q.expect(wire.Resumed)
	if w2 := q.expect(wire.Missed); w2.Lines != 0 || w2.At != 5 {
		t.Errorf("missed %+v, want no lines, data from 5", w2)
	}
}
