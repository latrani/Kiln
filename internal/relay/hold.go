package relay

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/relay/wire"
	"github.com/latrani/Kiln/internal/str"
)

// resumeWait is how long a resuming page has to say which connection.
const resumeWait = 10 * time.Second

// A held is one world connection. It outlives the WebSockets that carry
// it: when one dies without the page saying close, the world connection
// stays open, what it sends is buffered, and a page with the token can
// pick it up again until the hold runs out.
type held struct {
	s     *Server
	token string
	world string // host:port, for the log
	start time.Time
	tc    net.Conn

	ip netip.Addr // whose slot it holds; guarded by s.mu

	// upMu orders writes to the world against reporting how much has
	// been written, so a resume never hears less than was sent.
	upMu sync.Mutex
	up   int64 // bytes written to the world

	mu       sync.Mutex
	cond     *sync.Cond // the buffer, ended or ws changed
	buf      buffer
	ended    bool // the world closed; why says how
	why      string
	ws       *attachment // nil while detached
	detached time.Time
	timer    *time.Timer // the hold, while detached
	gen      int         // detaches so far; a timer names the one it's for
	expires  time.Time   // when the hold runs out, while detached
	short    bool        // its page said it closed: hold for short_hold
	warned   bool        // logged dropping since the last attach
	closed   bool
}

// An attachment is one WebSocket carrying a held connection.
type attachment struct {
	c      *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc
	gone   bool // detached or replaced; guarded by held.mu
}

func newToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// hold takes over tc, opened for ip, which already has its slot.
func (s *Server) hold(ip netip.Addr, world string, tc net.Conn) *held {
	h := &held{s: s, token: newToken(), world: world, start: time.Now(), tc: tc, ip: ip}
	h.cond = sync.NewCond(&h.mu)
	s.mu.Lock()
	if s.holds == nil {
		s.holds = map[string]*held{}
	}
	s.holds[h.token] = h
	s.mu.Unlock()
	go h.readWorld()
	return h
}

// readWorld buffers what the world sends until it closes.
func (h *held) readWorld() {
	b := make([]byte, 16<<10)
	for {
		n, err := h.tc.Read(b)
		if n > 0 {
			h.s.take(h, b[:n])
		}
		if err != nil {
			h.mu.Lock()
			h.ended = true
			if !errors.Is(err, io.EOF) && !h.closed {
				h.why = reason(err)
			}
			h.cond.Broadcast()
			h.mu.Unlock()
			return
		}
	}
}

// take buffers p for h, dropping the oldest lines past the caps.
func (s *Server) take(h *held, p []byte) {
	cfg := s.cfg.Load()
	s.mu.Lock()
	defer s.mu.Unlock()
	h.mu.Lock()
	before, dropped := h.buf.size(), h.buf.dropped
	h.buf.add(p, time.Now())
	for h.buf.size() > cfg.BufferCap() && h.buf.dropLine() {
	}
	s.buffered += h.buf.size() - before
	h.droppedLocked(dropped)
	h.cond.Broadcast()
	h.mu.Unlock()
	// Drop from the biggest hold that can; one whose buffer has no line
	// end yet is passed over.
	stuck := map[*held]bool{}
	for s.buffered > cfg.TotalBufferCap() {
		var big *held
		for _, o := range s.holds {
			if stuck[o] {
				continue
			}
			o.mu.Lock()
			if big == nil || o.buf.size() > big.buf.size() {
				big = o
			}
			o.mu.Unlock()
		}
		if big == nil {
			return
		}
		big.mu.Lock()
		before, dropped := big.buf.size(), big.buf.dropped
		if !big.buf.dropLine() {
			stuck[big] = true
		}
		s.buffered += big.buf.size() - before
		big.droppedLocked(dropped)
		big.mu.Unlock()
	}
}

// droppedLocked logs dropping once per attachment, if lines were dropped
// since the count was before. h.mu is held.
func (h *held) droppedLocked(before int) {
	if h.buf.dropped > before && !h.warned {
		h.warned = true
		h.s.logf(str.RelayDropping(h.ipLocked(), h.world))
	}
}

// ipLocked is h.ip for the log. s.mu is held.
func (h *held) ipLocked() netip.Addr { return h.ip }

// attach carries h over c, from ip, for a page that has received got
// bytes, until c ends. resumed says it's a resume (not the first).
func (h *held) attach(c *websocket.Conn, ip netip.Addr, got int64, resumed bool) {
	s := h.s
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &attachment{c: c, ctx: ctx, cancel: cancel}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		c.Close(wire.CodeGone, "")
		return
	}
	old := h.ws
	if old != nil {
		old.gone = true
	}
	h.ws = a
	h.warned = false
	h.short = false
	if h.timer != nil {
		h.timer.Stop()
		h.timer = nil
	}
	away := time.Since(h.detached)
	end := h.buf.end()
	h.cond.Broadcast()
	h.mu.Unlock()
	if old != nil {
		// On its own: a socket a suspended phone left may never answer
		// the close handshake. It's gone, so it writes nothing meanwhile.
		go func() { old.c.Close(wire.CodeReplaced, ""); old.cancel() }()
	}
	s.move(h, ip)

	h.upMu.Lock()
	up := h.up
	h.upMu.Unlock()
	if resumed {
		if c.Write(ctx, websocket.MessageText, wire.Encode(wire.Msg{T: wire.Resumed, Got: up, At: end, Away: away.Milliseconds()})) != nil {
			h.detach(a)
			return
		}
		s.logf(str.RelayResumed(ip, h.world, away.Round(time.Second)))
	}
	ack := wire.NewAcker(up, func(got int64) {
		c.Write(ctx, websocket.MessageText, wire.Encode(wire.Msg{T: wire.Ack, Got: got}))
	})
	defer ack.Stop()
	go s.keepAlive(ctx, c, func() { h.detach(a) })
	go h.write(a, got)
	h.read(a, ack)
}

// read carries what the page sends until its WebSocket ends.
func (h *held) read(a *attachment, ack *wire.Acker) {
	for {
		typ, b, err := a.c.Read(a.ctx)
		if err != nil {
			h.detach(a)
			return
		}
		if typ == websocket.MessageBinary {
			h.upMu.Lock()
			h.mu.Lock()
			gone := a.gone
			h.mu.Unlock()
			if gone {
				h.upMu.Unlock()
				return
			}
			h.tc.SetWriteDeadline(time.Now().Add(or(h.s.worldWriteTimeout, 30*time.Second)))
			n, err := h.tc.Write(b)
			h.up += int64(n)
			h.upMu.Unlock()
			if err != nil {
				// The world is ending, or has stopped reading: either way
				// it's let go, and readWorld tells the page.
				h.tc.Close()
				continue
			}
			ack.Received(n)
			continue
		}
		m, err := wire.Decode(b)
		if err != nil {
			continue
		}
		switch m.T {
		case wire.Ack:
			h.s.acked(h, m.Got)
		case wire.Ping:
			a.c.Write(a.ctx, websocket.MessageText, wire.Encode(wire.Msg{T: wire.Pong}))
		case wire.Close:
			h.finish(false)
			return
		}
	}
}

// write sends the buffer to the page, from pos, until a detaches.
func (h *held) write(a *attachment, pos int64) {
	sentEnd := false
	h.mu.Lock()
	defer h.mu.Unlock()
	for !a.gone {
		missed, from, at, p, ok := h.buf.next(pos)
		switch {
		case ok:
			p = append([]byte(nil), p...)
			h.mu.Unlock()
			err := error(nil)
			if missed > 0 || from > pos {
				// Say where the data goes on from: past lines dropped, or
				// past bytes the page acked but says it didn't get.
				err = a.c.Write(a.ctx, websocket.MessageText, wire.Encode(wire.Msg{T: wire.Missed, Lines: missed, At: from}))
			}
			if err == nil && len(p) > 0 {
				err = a.c.Write(a.ctx, websocket.MessageBinary, wire.DownFrame(at, p))
			}
			h.mu.Lock()
			if err != nil {
				return // read sees the WebSocket end, and detaches
			}
			pos = from + int64(len(p))
		case h.ended && !sentEnd:
			m := wire.Msg{T: wire.End, At: h.buf.end(), Why: h.why}
			h.mu.Unlock()
			err := a.c.Write(a.ctx, websocket.MessageText, wire.Encode(m))
			h.mu.Lock()
			if err != nil {
				return
			}
			sentEnd = true
		default:
			h.cond.Wait()
		}
	}
}

// acked forgets what the page says it has received.
func (s *Server) acked(h *held, got int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h.mu.Lock()
	defer h.mu.Unlock()
	before := h.buf.size()
	h.buf.ack(got)
	s.buffered += h.buf.size() - before
}

// detach ends a, keeping h for the hold, unless a was already replaced.
func (h *held) detach(a *attachment) {
	s := h.s
	cfg := s.cfg.Load()
	h.mu.Lock()
	if h.ws != a || h.closed {
		h.mu.Unlock()
		a.cancel()
		return
	}
	a.gone = true
	h.ws = nil
	h.detached = time.Now()
	h.gen++
	hold := cfg.HoldFor()
	if h.short {
		hold = cfg.ShortHoldFor()
	}
	h.expires = h.detached.Add(hold)
	gen := h.gen
	h.timer = time.AfterFunc(hold, func() { h.expire(gen) })
	h.cond.Broadcast()
	h.mu.Unlock()
	a.cancel()
	a.c.CloseNow()
	s.mu.Lock()
	ip := h.ip
	s.mu.Unlock()
	s.logf(str.RelayHeld(ip, h.world))
}

// expire lets h go if it's still in the detach gen began.
func (h *held) expire(gen int) {
	h.mu.Lock()
	still := h.ws == nil && !h.closed && h.gen == gen
	h.mu.Unlock()
	if still {
		h.finish(true)
	}
}

// finish lets the world go and frees h's slot. expired says the hold ran
// out (rather than the page saying close).
func (h *held) finish(expired bool) {
	s := h.s
	s.mu.Lock()
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		s.mu.Unlock()
		return
	}
	h.closed = true
	delete(s.holds, h.token)
	s.buffered -= h.buf.size()
	ip := h.ip
	a := h.ws
	h.ws = nil
	if a != nil {
		a.gone = true
	}
	if h.timer != nil {
		h.timer.Stop()
	}
	since := h.detached
	down := h.buf.end()
	h.cond.Broadcast()
	h.mu.Unlock()
	s.mu.Unlock()

	h.tc.Close()
	s.release(ip)
	if a != nil {
		a.c.Close(websocket.StatusNormalClosure, "")
		a.cancel()
	}
	h.upMu.Lock()
	up := h.up
	h.upMu.Unlock()
	if expired {
		s.logf(str.RelayHoldExpired(ip, h.world, time.Since(since).Round(time.Second)))
		return
	}
	s.logf(str.RelayClosed(ip, h.world, up, down, time.Since(h.start).Round(time.Second)))
}

// move gives h's slot to ip's bucket, if it's another. The new bucket's
// cap isn't checked: it gates new connections, not existing ones.
func (s *Server) move(h *held, ip netip.Addr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if bucket(h.ip) == bucket(ip) {
		h.ip = ip
		return
	}
	if s.open[bucket(h.ip)]--; s.open[bucket(h.ip)] <= 0 {
		delete(s.open, bucket(h.ip))
	}
	s.open[bucket(ip)]++
	h.ip = ip
}

// resume attaches c, from ip, to the held connection its first message
// names.
func (s *Server) resume(c *websocket.Conn, ip netip.Addr) {
	ctx, cancel := context.WithTimeout(context.Background(), resumeWait)
	typ, b, err := c.Read(ctx)
	cancel()
	if err != nil {
		return
	}
	m, err := wire.Decode(b)
	if typ != websocket.MessageText || err != nil || m.T != wire.Resume {
		c.Close(websocket.StatusProtocolError, "")
		return
	}
	s.mu.Lock()
	h := s.holds[m.Token]
	s.mu.Unlock()
	if h == nil {
		s.logf(str.RelayRefusedGone(ip))
		c.Close(wire.CodeGone, "")
		return
	}
	h.attach(c, ip, m.Got, true)
}

// shorten holds h only for short_hold from now, or less if its hold runs
// out sooner: its page said it closed.
func (h *held) shorten() {
	cfg := h.s.cfg.Load()
	short := cfg.ShortHoldFor()
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.short = true
	if h.ws == nil && h.timer != nil && time.Until(h.expires) > short {
		h.timer.Stop()
		h.expires = time.Now().Add(short)
		gen := h.gen
		h.timer = time.AfterFunc(short, func() { h.expire(gen) })
	}
	h.mu.Unlock()
	h.s.mu.Lock()
	ip := h.ip
	h.s.mu.Unlock()
	h.s.logf(str.RelayLettingGo(ip, h.world, short))
}

// beacon reads the tokens a closing page sends (sendBeacon), one per
// line, and shortens their holds.
func (s *Server) beacon(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	for _, tok := range strings.Split(string(b), "\n") {
		s.mu.Lock()
		h := s.holds[strings.TrimSpace(tok)]
		s.mu.Unlock()
		if h != nil {
			h.shorten()
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
