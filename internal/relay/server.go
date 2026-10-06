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
)

// Server is the relay's HTTP handler. Set an allowlist before serving.
type Server struct {
	Dial              func(ctx context.Context, network, addr string) (net.Conn, error) // nil: TCP
	Log               func(line string)                                                 // nil: silent
	MaxPerIP          int                                                               // 0: 8
	FirstFrameTimeout time.Duration                                                     // 0: 15s

	allow atomic.Pointer[Allowlist]
	mu    sync.Mutex
	open  map[netip.Addr]int
}

// SetAllowlist replaces the allowlist; connections already open stay open.
func (s *Server) SetAllowlist(a *Allowlist) { s.allow.Store(a) }

// ServeHTTP upgrades to a WebSocket and, if the world in ?host=&port= is
// allowed, pipes binary frames to and from it.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a := s.allow.Load()
	if a == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: a.Origins})
	if err != nil {
		return // Accept has answered (403 for a foreign Origin)
	}
	defer c.CloseNow()
	c.SetReadLimit(1 << 20)

	ip := s.clientIP(r, a)
	host := r.URL.Query().Get("host")
	port, _ := strconv.Atoi(r.URL.Query().Get("port"))
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	wl, ok := a.Lookup(host, port)
	if !ok {
		s.logf(str.RelayRefusedNotListed(ip, addr))
		c.Close(CodeNotListed, "")
		return
	}
	if !s.acquire(ip) {
		s.logf(str.RelayRefusedTooMany(ip, addr))
		c.Close(CodeTooMany, "")
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
		timer.Stop()
		if timedOut.Load() {
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

// clientIP is who's connecting: the peer, or, when the peer is a trusted
// proxy, the last address in X-Forwarded-For (the one the proxy saw).
func (s *Server) clientIP(r *http.Request, a *Allowlist) netip.Addr {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	ip := ap.Addr().Unmap()
	if !a.Trusted(ip) {
		return ip
	}
	xff := r.Header.Values("X-Forwarded-For") //str:ok
	if len(xff) == 0 {
		return ip
	}
	parts := strings.Split(xff[len(xff)-1], ",")
	if fwd, err := netip.ParseAddr(strings.TrimSpace(parts[len(parts)-1])); err == nil {
		return fwd.Unmap()
	}
	return ip
}

func (s *Server) acquire(ip netip.Addr) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.open == nil {
		s.open = map[netip.Addr]int{}
	}
	if s.open[ip] >= or(s.MaxPerIP, 8) {
		return false
	}
	s.open[ip]++
	return true
}

func (s *Server) release(ip netip.Addr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.open[ip]--; s.open[ip] <= 0 {
		delete(s.open, ip)
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
func reason(err error) string {
	s := err.Error()
	if len(s) > 123 {
		s = s[:123]
	}
	return s
}

func or[T comparable](v, def T) T {
	var zero T
	if v == zero {
		return def
	}
	return v
}
