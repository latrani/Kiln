package relay

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
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

	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/relay/wire"
	"github.com/latrani/Kiln/internal/str"
)

// defaultHandshakeTimeout bounds the relay's dial and TLS handshake. It's
// well under the page's conn.DialTimeout, whose clock starts first, so the
// page hears the relay's refusal rather than timing out itself.
const defaultHandshakeTimeout = 10 * time.Second

// Server is the relay's HTTP handler. Set a config before serving.
type Server struct {
	Dial func(ctx context.Context, network, addr string) (net.Conn, error) // nil: TCP
	Log  func(line string)                                                 // nil: silent
	// RootCAs are the CAs a tls_trust = "ca" world is checked against.
	// nil: the system's.
	RootCAs *x509.CertPool
	// HandshakeTimeout bounds the dial and TLS handshake together. 0:
	// defaultHandshakeTimeout.
	HandshakeTimeout time.Duration

	// pingEvery is how often an open connection is pinged; one that
	// doesn't answer within pingTimeout is closed, so a browser that
	// went away without a word (a sleeping laptop) frees its slot.
	pingEvery, pingTimeout time.Duration // 0: 30s, 20s
	// worldWriteTimeout is how long a write to a world may wait for it
	// to read; past it the world is let go. 0: 30s.
	worldWriteTimeout time.Duration

	cfg      atomic.Pointer[Config]
	mu       sync.Mutex
	open     map[netip.Prefix]int
	total    int
	holds    map[string]*held // by token
	buffered int              // bytes buffered across holds
}

// SetConfig replaces the config; connections already open stay open.
func (s *Server) SetConfig(a *Config) { s.cfg.Store(a) }

// ServeHTTP upgrades to a WebSocket and, if the world in ?host=&port= is
// allowed, connects to it, doing TLS with the world itself unless it's
// plaintext, and carries it as package wire says. tls=1 says the
// client's world file asks for TLS, which a plaintext world refuses.
// ?resume=1 picks up a held connection instead.
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
	if r.URL.Query().Get("resume") == "1" {
		s.resume(c, ip)
		return
	}

	host := r.URL.Query().Get("host")
	port, _ := strconv.Atoi(r.URL.Query().Get("port"))
	// addr is only for the log, which quotes it; host comes from the
	// client, so it's cut to a DNS name's longest.
	addr := net.JoinHostPort(cut(host, 253), strconv.Itoa(port))

	wl, ok := a.Lookup(host, port)
	if !ok {
		s.logf(str.RelayRefusedNotListed(ip, addr))
		c.Close(wire.CodeNotListed, "")
		return
	}
	if wl.Plaintext && r.URL.Query().Get("tls") == "1" {
		s.logf(str.RelayRefusedNoTls(ip, addr))
		c.Close(wire.CodeNoTLS, "")
		return
	}
	switch s.acquire(ip, a) {
	case wire.CodeTooMany:
		s.logf(str.RelayRefusedTooMany(ip, addr))
		c.Close(wire.CodeTooMany, "")
		return
	case wire.CodeBusy:
		s.logf(str.RelayRefusedBusy(ip, addr))
		c.Close(wire.CodeBusy, "")
		return
	}
	addr = net.JoinHostPort(wl.Host, strconv.Itoa(port))
	dctx, dcancel := context.WithTimeout(context.Background(), or(s.HandshakeTimeout, defaultHandshakeTimeout))
	tc, err := s.dial(dctx, addr)
	if err == nil && !wl.Plaintext {
		tc, err = s.handshake(dctx, tc, addr, wl)
		if certErr(err) {
			dcancel()
			s.logf(str.RelayRefusedBadCert(ip, addr, err))
			why := reason(err)
			var pin *conn.PinMismatchError
			if errors.As(err, &pin) {
				why = str.RelayCertNotPinned() // the log has the fingerprints
			}
			c.Close(wire.CodeBadCert, why)
			s.release(ip)
			return
		}
	}
	dcancel()
	if err != nil {
		s.logf(str.RelayRefusedUnreachable(ip, addr, err))
		c.Close(wire.CodeUnreachable, reason(err))
		s.release(ip)
		return
	}
	h := s.hold(ip, addr, tc)
	s.logf(str.RelayOpened(ip, addr))
	if err := c.Write(context.Background(), websocket.MessageText, wire.Encode(wire.Msg{T: wire.Open, Token: h.token})); err != nil {
		h.finish(false)
		return
	}
	h.attach(c, ip, 0, false)
}

// handshake runs TLS over nc with the world, trusting it as its config
// says. On failure nc is closed.
func (s *Server) handshake(ctx context.Context, nc net.Conn, addr string, wl World) (net.Conn, error) {
	cfg := &tls.Config{ServerName: wl.Host, RootCAs: s.RootCAs}
	if wl.Pin != "" {
		// A pinned world is usually self-signed: the pin is the check.
		cfg.InsecureSkipVerify = true
		cfg.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errNoCert
			}
			if got := conn.Fingerprint(cs.PeerCertificates[0]); got != wl.Pin {
				return &conn.PinMismatchError{HostPort: addr, Pinned: wl.Pin, Got: got}
			}
			return nil
		}
	}
	tc := tls.Client(nc, cfg)
	if err := tc.HandshakeContext(ctx); err != nil {
		nc.Close()
		return nil, err
	}
	return tc, nil
}

// errNoCert is a pinned world sending no certificate.
var errNoCert = errors.New(str.ConnNoCertificate())

// certErr reports whether err is the world's certificate failing its
// check, rather than the handshake failing some other way.
func certErr(err error) bool {
	var (
		pin      *conn.PinMismatchError
		verify   *tls.CertificateVerificationError
		unknown  x509.UnknownAuthorityError
		hostname x509.HostnameError
		invalid  x509.CertificateInvalidError
	)
	return errors.Is(err, errNoCert) || errors.As(err, &pin) || errors.As(err, &verify) ||
		errors.As(err, &unknown) || errors.As(err, &hostname) || errors.As(err, &invalid)
}

func (s *Server) dial(ctx context.Context, addr string) (net.Conn, error) {
	if s.Dial != nil {
		return s.Dial(ctx, "tcp", addr)
	}
	var d net.Dialer
	return d.DialContext(ctx, "tcp", addr)
}

// keepAlive pings c until ctx ends, calling dead when a ping goes
// unanswered (a sleeping laptop, a suspended phone). Pongs are read by
// the attachment's reads.
func (s *Server) keepAlive(ctx context.Context, c *websocket.Conn, dead func()) {
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
			if ctx.Err() == nil {
				dead()
			}
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
		return wire.CodeBusy
	}
	b := bucket(ip)
	if s.open[b] >= or(a.MaxPerIP, 8) {
		return wire.CodeTooMany
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
