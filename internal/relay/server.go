package relay

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/str"
)

// Close codes the relay ends a refused connection with. Kiln's web build
// (web/wsdial) turns them into messages.
const (
	CodeNotListed   websocket.StatusCode = 4403 // world not on the allowlist
	CodeNeedsTLS    websocket.StatusCode = 4426 // TLS-only world, client didn't start TLS
	CodeTooMany     websocket.StatusCode = 4429 // too many connections from this IP
	CodeUnreachable websocket.StatusCode = 4502 // the dial failed; the reason says why
	CodeBusy        websocket.StatusCode = 4503 // the relay is at max_conns
)

// Server is the relay's HTTP handler. Set a config before serving.
type Server struct {
	Dial              func(ctx context.Context, network, addr string) (net.Conn, error) // nil: TCP
	Log               func(line string)                                                 // nil: silent
	FirstFrameTimeout time.Duration                                                     // 0: 15s

	// pingEvery is how often an open connection is pinged; one that
	// doesn't answer within pingTimeout is closed, so a browser that
	// went away without a word (a sleeping laptop) frees its slot.
	pingEvery, pingTimeout time.Duration // 0: 30s, 20s

	cfg   atomic.Pointer[Config]
	mu    sync.Mutex
	open  map[netip.Prefix]int
	total int
}

// SetConfig replaces the config; connections already open stay open.
func (s *Server) SetConfig(a *Config) { s.cfg.Store(a) }

// ServeHTTP upgrades to a WebSocket and, if the world in ?host=&port= is
// allowed, pipes binary frames to and from it.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a := s.cfg.Load()
	if a == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	ip, ok := s.clientIP(r, a)
	if !ok {
		// Every such client would share one bucket, so none is let in.
		s.logf(str.RelayRefusedBadPeer(r.RemoteAddr))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: a.Origins})
	if err != nil {
		return // Accept has answered (403 for a foreign Origin)
	}
	defer c.CloseNow()
	c.SetReadLimit(1 << 20)

	host := r.URL.Query().Get("host")
	port, _ := strconv.Atoi(r.URL.Query().Get("port"))
	// addr is only for the log, which quotes it; host comes from the
	// client, so it's cut to a DNS name's longest.
	addr := net.JoinHostPort(cut(host, 253), strconv.Itoa(port))

	wl, ok := a.Lookup(host, port)
	if !ok {
		s.logf(str.RelayRefusedNotListed(ip, addr))
		c.Close(CodeNotListed, "")
		return
	}
	switch s.acquire(ip, a) {
	case CodeTooMany:
		s.logf(str.RelayRefusedTooMany(ip, addr))
		c.Close(CodeTooMany, "")
		return
	case CodeBusy:
		s.logf(str.RelayRefusedBusy(ip, addr))
		c.Close(CodeBusy, "")
		return
	}
	defer s.release(ip)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var first []byte
	if !wl.Plaintext {
		// A read deadline would make the library drop the connection without
		// a close frame, so a timer sends the refusal instead.
		var timedOut atomic.Bool
		timer := time.AfterFunc(or(s.FirstFrameTimeout, 15*time.Second), func() {
			timedOut.Store(true)
			c.Close(CodeNeedsTLS, "")
		})
		typ, b, err := c.Read(ctx)
		// Stop is false once the timer has fired: it's closing c, even if
		// it hasn't set timedOut yet.
		if !timer.Stop() || timedOut.Load() {
			s.logf(str.RelayRefusedNotTls(ip, addr))
			return
		}
		if err != nil {
			return // the client went away
		}
		if typ != websocket.MessageBinary || len(b) == 0 || b[0] != 0x16 { // 0x16: TLS handshake record
			s.logf(str.RelayRefusedNotTls(ip, addr))
			c.Close(CodeNeedsTLS, "")
			return
		}
		first = b
	}

	addr = net.JoinHostPort(wl.Host, strconv.Itoa(port))
	dctx, dcancel := context.WithTimeout(ctx, 15*time.Second)
	tc, err := s.dial(dctx, addr)
	dcancel()
	if err != nil {
		s.logf(str.RelayRefusedUnreachable(ip, addr, err))
		c.Close(CodeUnreachable, reason(err))
		return
	}
	defer tc.Close()
	if len(first) > 0 {
		if _, err := tc.Write(first); err != nil {
			return
		}
	}
	go s.keepAlive(ctx, c)
	start := time.Now()
	s.logf(str.RelayOpened(ip, addr))
	up, down := pipe(websocket.NetConn(ctx, c, websocket.MessageBinary), tc)
	s.logf(str.RelayClosed(ip, addr, up+int64(len(first)), down, time.Since(start).Round(time.Second)))
}

func (s *Server) dial(ctx context.Context, addr string) (net.Conn, error) {
	if s.Dial != nil {
		return s.Dial(ctx, "tcp", addr)
	}
	var d net.Dialer
	return d.DialContext(ctx, "tcp", addr)
}

// keepAlive pings c until ctx ends, closing it when a ping goes
// unanswered. Pongs are read by the pipe's reads.
func (s *Server) keepAlive(ctx context.Context, c *websocket.Conn) {
	t := time.NewTicker(or(s.pingEvery, 30*time.Second))
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		pctx, cancel := context.WithTimeout(ctx, or(s.pingTimeout, 20*time.Second))
		err := c.Ping(pctx)
		cancel()
		if err != nil {
			c.CloseNow()
			return
		}
	}
}

// clientIP is who's connecting: the peer, or, when the peer is a trusted
// proxy, the last address in X-Forwarded-For (the one the proxy saw).
// ok is false when the peer's address can't be read.
func (s *Server) clientIP(r *http.Request, a *Config) (ip netip.Addr, ok bool) {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}, false
	}
	ip = ap.Addr().Unmap()
	if !a.Trusted(ip) {
		return ip, true
	}
	xff := r.Header.Values("X-Forwarded-For") //str:ok
	if len(xff) == 0 {
		return ip, true
	}
	parts := strings.Split(xff[len(xff)-1], ",")
	if fwd, err := netip.ParseAddr(strings.TrimSpace(parts[len(parts)-1])); err == nil {
		return fwd.Unmap(), true
	}
	return ip, true
}

// bucket is what the per-IP cap counts against: an IPv4 address, or an
// IPv6 /64, the smallest block a client is usually given.
func bucket(ip netip.Addr) netip.Prefix {
	if ip.Is4() {
		return netip.PrefixFrom(ip, 32)
	}
	p, _ := ip.Prefix(64)
	return p
}

// acquire takes a slot for ip, or returns the code to refuse with.
func (s *Server) acquire(ip netip.Addr, a *Config) websocket.StatusCode {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.open == nil {
		s.open = map[netip.Prefix]int{}
	}
	if s.total >= or(a.MaxConns, 256) {
		return CodeBusy
	}
	b := bucket(ip)
	if s.open[b] >= or(a.MaxPerIP, 8) {
		return CodeTooMany
	}
	s.open[b]++
	s.total++
	return 0
}

func (s *Server) release(ip netip.Addr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total--
	b := bucket(ip)
	if s.open[b]--; s.open[b] <= 0 {
		delete(s.open, b)
	}
}

func (s *Server) logf(line string) {
	if s.Log != nil {
		s.Log(line)
	}
}

// pipe copies both ways until either side ends, then closes both. It
// reports bytes up (browser to MUCK) and down.
func pipe(ws, tc net.Conn) (up, down int64) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		down, _ = io.Copy(ws, tc)
		ws.Close()
	}()
	up, _ = io.Copy(tc, ws)
	tc.Close()
	wg.Wait()
	return up, down
}

// reason fits err into a close frame's reason (at most 123 bytes).
func reason(err error) string { return cut(err.Error(), 123) }

// cut shortens s to at most n bytes without splitting a UTF-8 sequence.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func or[T comparable](v, def T) T {
	var zero T
	if v == zero {
		return def
	}
	return v
}
