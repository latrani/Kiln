package relay

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/relay/wire"
	"github.com/latrani/Kiln/internal/str"
)

// serveEach accepts connections on ln until the test ends, handing each
// to handle. It returns the port.
func serveEach(t *testing.T, ln net.Listener, handle func(net.Conn)) int {
	t.Helper()
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); handle(c) }()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func tcpListener(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

// echoGreeter says hello, then answers each line.
func echoGreeter(c net.Conn) {
	c.Write([]byte("hello\r\n"))
	sc := bufio.NewScanner(c)
	for sc.Scan() {
		fmt.Fprintf(c, "you said %s\r\n", sc.Text())
	}
}

// relay starts a Server with the given allowlist TOML and returns it, its
// ws:// URL, and its log.
func relay(t *testing.T, allow string) (*Server, string, *logLines) {
	t.Helper()
	a, err := ParseConfig("relay.toml", []byte(allow))
	if err != nil {
		t.Fatal(err)
	}
	logs := &logLines{}
	s := &Server{Log: logs.add}
	s.SetConfig(a)
	hs := httptest.NewServer(s)
	t.Cleanup(hs.Close)
	return s, "ws" + strings.TrimPrefix(hs.URL, "http"), logs
}

type logLines struct {
	mu    sync.Mutex
	lines []string
}

func (l *logLines) add(s string) { l.mu.Lock(); l.lines = append(l.lines, s); l.mu.Unlock() }

// all is every line logged so far.
func (l *logLines) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.lines)
}

// inUse is how many slots s holds.
func (s *Server) inUse() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.total
}

// first is the first line logged, or "" (copied under the lock: -race).
func (l *logLines) first() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.lines) == 0 {
		return ""
	}
	return l.lines[0]
}

// world is a relay.toml world for 127.0.0.1:port. trust is "" for a
// plaintext world, "ca", or a pin.
func world(port int, trust string) string {
	w := fmt.Sprintf("[[world]]\nid = \"w%d\"\nhost = \"127.0.0.1\"\nport = %d\n", port, port)
	switch trust {
	case "":
		return w
	case "ca":
		return w + "tls = true\ntls_trust = \"ca\"\n"
	}
	return w + fmt.Sprintf("tls = true\npin = %q\n", trust)
}

func open(t *testing.T, base, host string, port int) *websocket.Conn {
	t.Helper()
	return openAsking(t, base, host, port, false)
}

// openAsking is open, asking for TLS (tls=1) when tls is true, as the
// page does when its world file says tls.
func openAsking(t *testing.T, base, host string, port int, tls bool) *websocket.Conn {
	t.Helper()
	u := fmt.Sprintf("%s/?host=%s&port=%d", base, host, port)
	if tls {
		u += "&tls=1"
	}
	c, _, err := websocket.Dial(context.Background(), u, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	return c
}

// closeWhy reads until the relay refuses or closes c and returns the
// code and reason.
func closeWhy(t *testing.T, c *websocket.Conn) (websocket.StatusCode, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		typ, b, err := c.Read(ctx)
		if err != nil {
			var ce websocket.CloseError
			errors.As(err, &ce)
			return ce.Code, ce.Reason
		}
		if m, _ := wire.Decode(b); typ == websocket.MessageText && m.T == wire.Refused {
			return websocket.StatusCode(m.Code), m.Why
		}
	}
}

// closeCode reads until the relay refuses or closes c and returns the
// code.
func closeCode(t *testing.T, c *websocket.Conn) websocket.StatusCode {
	t.Helper()
	code, _ := closeWhy(t, c)
	return code
}

func TestPlaintextRoundTrip(t *testing.T) {
	port := serveEach(t, tcpListener(t), echoGreeter)
	_, base, _ := relay(t, world(port, ""))
	nc := piped(t, open(t, base, "127.0.0.1", port))
	r := bufio.NewReader(nc)
	if line, _ := r.ReadString('\n'); line != "hello\r\n" {
		t.Fatalf("greeting %q", line)
	}
	nc.Write([]byte("ping\n"))
	if line, _ := r.ReadString('\n'); line != "you said ping\r\n" {
		t.Errorf("reply %q", line)
	}
}

func TestUnlistedIsRefusedAndNeverDialed(t *testing.T) {
	port := serveEach(t, tcpListener(t), echoGreeter)
	s, base, logs := relay(t, world(port, ""))
	dialed := false
	s.Dial = func(context.Context, string, string) (net.Conn, error) { dialed = true; return nil, net.ErrClosed }
	for _, hp := range []struct {
		host string
		port int
	}{{"127.0.0.1", port + 1}, {"localhost", port}} {
		if code := closeCode(t, open(t, base, hp.host, hp.port)); code != wire.CodeNotListed {
			t.Errorf("%v: code %d, want %d", hp, code, wire.CodeNotListed)
		}
	}
	if dialed {
		t.Error("dialed an unlisted world")
	}
	if !strings.HasPrefix(logs.first(), upTo(str.RelayRefusedNotListed(mark, mark))) {
		t.Errorf("log %q", logs.first())
	}
}

func TestLoggedHostIsOneQuotedLine(t *testing.T) {
	port := serveEach(t, tcpListener(t), echoGreeter)
	_, base, logs := relay(t, world(port, ""))
	host := "fm.example\nrefused 10.0.0.1 → evil" + strings.Repeat("x", 1000)
	closeCode(t, open(t, base, url.QueryEscape(host), port))
	line := logs.first()
	if strings.ContainsAny(line, "\r\n") {
		t.Errorf("log has a raw line break: %q", line)
	}
	if len(line) > 600 {
		t.Errorf("log is %d bytes; the host wasn't cut", len(line))
	}
	if !strings.HasPrefix(line, upTo(str.RelayRefusedNotListed(mark, mark))) {
		t.Errorf("log %q", line)
	}
}

func TestReasonKeepsWholeRunes(t *testing.T) {
	r := reason(errors.New(strings.Repeat("a", 122) + "é and more"))
	if len(r) > 123 || !utf8.ValidString(r) {
		t.Errorf("reason %q (%d bytes)", r, len(r))
	}
	if r != strings.Repeat("a", 122) {
		t.Errorf("reason %q, want the a's without half an é", r)
	}
}

// tlsGreeter is a TLS MUCK on 127.0.0.1 that says "secure hello". It
// returns the port and the certificate.
func tlsGreeter(t *testing.T) (int, *x509.Certificate) {
	t.Helper()
	ln, cert := selfSignedListener(t)
	return serveEach(t, ln, func(c net.Conn) { c.Write([]byte("secure hello\r\n")) }), cert
}

func TestRelayDoesTLSWithPin(t *testing.T) {
	port, cert := tlsGreeter(t)
	_, base, logs := relay(t, world(port, conn.Fingerprint(cert)))
	if l := readLine(t, openAsking(t, base, "127.0.0.1", port, true)); l != "secure hello\r\n" {
		t.Errorf("line %q", l)
	}
	if !strings.HasPrefix(logs.first(), upTo(str.RelayOpened(mark, mark))) {
		t.Errorf("log %q", logs.first())
	}
}

func TestRelayDoesTLSWithoutClientAsking(t *testing.T) {
	port, cert := tlsGreeter(t)
	_, base, _ := relay(t, world(port, conn.Fingerprint(cert)))
	if l := readLine(t, open(t, base, "127.0.0.1", port)); l != "secure hello\r\n" {
		t.Errorf("line %q", l)
	}
}

func TestRelayPinMismatch(t *testing.T) {
	port, _ := tlsGreeter(t)
	_, base, logs := relay(t, world(port, testPin))
	if code, why := closeWhy(t, openAsking(t, base, "127.0.0.1", port, true)); code != wire.CodeBadCert || why != str.RelayCertNotPinned() {
		t.Errorf("close %d %q, want %d %q", code, why, wire.CodeBadCert, str.RelayCertNotPinned())
	}
	if !strings.HasPrefix(logs.first(), upTo(str.RelayRefusedBadCert(mark, mark, errMark{}))) {
		t.Errorf("log %q", logs.first())
	}
}

func TestRelayCATrust(t *testing.T) {
	port, cert := tlsGreeter(t)
	s, base, _ := relay(t, world(port, "ca"))
	s.RootCAs = x509.NewCertPool()
	s.RootCAs.AddCert(cert)
	if l := readLine(t, openAsking(t, base, "127.0.0.1", port, true)); l != "secure hello\r\n" {
		t.Errorf("line %q", l)
	}
	s.RootCAs = x509.NewCertPool()
	if code := closeCode(t, openAsking(t, base, "127.0.0.1", port, true)); code != wire.CodeBadCert {
		t.Errorf("unknown CA: code %d, want %d", code, wire.CodeBadCert)
	}
}

func TestRelayRefusesTLSForPlaintextWorld(t *testing.T) {
	port := serveEach(t, tcpListener(t), echoGreeter)
	s, base, logs := relay(t, world(port, ""))
	dialed := false
	s.Dial = func(context.Context, string, string) (net.Conn, error) { dialed = true; return nil, net.ErrClosed }
	if code := closeCode(t, openAsking(t, base, "127.0.0.1", port, true)); code != wire.CodeNoTLS {
		t.Errorf("code %d, want %d", code, wire.CodeNoTLS)
	}
	if dialed {
		t.Error("dialed a plaintext world for a client that asked for TLS")
	}
	if !strings.HasPrefix(logs.first(), upTo(str.RelayRefusedNoTls(mark, mark))) {
		t.Errorf("log %q", logs.first())
	}
}

// silent accepts connections and never says anything: a plaintext port
// listed as TLS, say.
func silent(c net.Conn) { io.Copy(io.Discard, c) }

func TestRelayHandshakeTimeout(t *testing.T) {
	port := serveEach(t, tcpListener(t), silent)
	s, base, _ := relay(t, world(port, testPin))
	s.HandshakeTimeout = 300 * time.Millisecond
	start := time.Now()
	if code := closeCode(t, openAsking(t, base, "127.0.0.1", port, true)); code != wire.CodeUnreachable {
		t.Errorf("code %d, want %d", code, wire.CodeUnreachable)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("took %v", d)
	}
}

func TestTLSRefusalsReleaseSlots(t *testing.T) {
	plain := serveEach(t, tcpListener(t), echoGreeter)
	pinned, _ := tlsGreeter(t)
	quiet := serveEach(t, tcpListener(t), silent)
	s, base, _ := relay(t, world(plain, "")+world(pinned, testPin)+world(quiet, testPin))
	s.HandshakeTimeout = 300 * time.Millisecond
	for _, c := range []struct {
		port int
		want websocket.StatusCode
	}{{plain, wire.CodeNoTLS}, {pinned, wire.CodeBadCert}, {quiet, wire.CodeUnreachable}} {
		if code := closeCode(t, openAsking(t, base, "127.0.0.1", c.port, true)); code != c.want {
			t.Fatalf("port %d: code %d, want %d", c.port, code, c.want)
		}
	}
	eventually(t, "a refused connection kept its slot", func() bool { return s.inUse() == 0 })
}

func TestRelaySaysOpenBeforeData(t *testing.T) {
	port := serveEach(t, tcpListener(t), echoGreeter)
	_, base, _ := relay(t, world(port, ""))
	c := open(t, base, "127.0.0.1", port)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if typ, b, err := c.Read(ctx); err != nil || typ != websocket.MessageText || !strings.Contains(string(b), wire.Open) {
		t.Errorf("first frame %v %q, %v", typ, b, err)
	}
	typ, b, _ := c.Read(ctx)
	if _, d, _ := wire.ReadDown(b); typ != websocket.MessageBinary || string(d) != "hello\r\n" {
		t.Errorf("second frame %v %q", typ, b)
	}
}

func TestUnreachable(t *testing.T) {
	ln := tcpListener(t)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close() // nothing listens there now
	_, base, _ := relay(t, world(port, ""))
	if code := closeCode(t, open(t, base, "127.0.0.1", port)); code != wire.CodeUnreachable {
		t.Errorf("code %d, want %d", code, wire.CodeUnreachable)
	}
}

func TestWrongOriginRefused(t *testing.T) {
	port := serveEach(t, tcpListener(t), echoGreeter)
	_, base, _ := relay(t, world(port, ""))
	_, resp, err := websocket.Dial(context.Background(), fmt.Sprintf("%s/?host=127.0.0.1&port=%d", base, port),
		&websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"https://evil.example"}}})
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("err %v, resp %v; want 403", err, resp)
	}
}

func TestPerIPCapAndRelease(t *testing.T) {
	port := serveEach(t, tcpListener(t), echoGreeter)
	_, base, _ := relay(t, "max_per_ip = 2\n"+world(port, ""))
	// Refusals must not use up slots.
	for range 3 {
		closeCode(t, open(t, base, "127.0.0.1", port+1))
	}
	// Each holds its slot once the MUCK's hello comes through.
	a := open(t, base, "127.0.0.1", port)
	readLine(t, a)
	readLine(t, open(t, base, "127.0.0.1", port))
	if code := closeCode(t, open(t, base, "127.0.0.1", port)); code != wire.CodeTooMany {
		t.Errorf("third: code %d, want %d", code, wire.CodeTooMany)
	}
	bye(t, a)
	deadline := time.Now().Add(2 * time.Second)
	for {
		c, _, err := websocket.Dial(context.Background(), fmt.Sprintf("%s/?host=127.0.0.1&port=%d", base, port), nil)
		if err != nil {
			t.Fatal(err)
		}
		nc := piped(t, c)
		nc.SetReadDeadline(time.Now().Add(time.Second))
		line, _ := bufio.NewReader(nc).ReadString('\n')
		bye(t, c)
		if line == "hello\r\n" {
			return // a slot came back
		}
		if time.Now().After(deadline) {
			t.Fatal("slot never released")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestClientIPTrustsXFFOnlyFromProxies(t *testing.T) {
	a, _ := ParseConfig("relay.toml", []byte(`trusted_proxies = ["127.0.0.1"]`))
	s := &Server{}
	req := func(remote, xff string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return r
	}
	for _, c := range []struct{ remote, xff, want string }{
		{"127.0.0.1:5000", "203.0.113.9", "203.0.113.9"},
		{"127.0.0.1:5000", "198.51.100.1, 203.0.113.9", "203.0.113.9"}, // nginx appends; last is what it saw
		{"192.0.2.7:5000", "203.0.113.9", "192.0.2.7"},                 // not a proxy: ignore XFF
		{"127.0.0.1:5000", "", "127.0.0.1"},
	} {
		if got, ok := s.clientIP(req(c.remote, c.xff), a); !ok || got != netip.MustParseAddr(c.want) {
			t.Errorf("%s via %q = %s, want %s", c.remote, c.xff, got, c.want)
		}
	}
}

func TestUnreadablePeerRefused(t *testing.T) {
	logs := &logLines{}
	s := &Server{Log: logs.add}
	a, _ := ParseConfig("relay.toml", []byte(world(1, "")))
	s.SetConfig(a)
	r := httptest.NewRequest("GET", "/?host=127.0.0.1&port=1", nil)
	r.RemoteAddr = "@"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if got, want := logs.first(), str.RelayRefusedBadPeer("@"); got != want {
		t.Errorf("log %q, want %q", got, want)
	}
}

func TestPerIPCapBucketsIPv6By64(t *testing.T) {
	a, _ := ParseConfig("relay.toml", []byte("max_per_ip = 1\n"))
	s := &Server{}
	take := func(ip string) websocket.StatusCode { return s.acquire(netip.MustParseAddr(ip), a) }
	if take("2001:db8:1:2::1") != 0 {
		t.Fatal("first IPv6 client refused")
	}
	if code := take("2001:db8:1:2:ffff::9"); code != wire.CodeTooMany {
		t.Errorf("same /64: code %d, want %d", code, wire.CodeTooMany)
	}
	if take("2001:db8:1:3::1") != 0 {
		t.Error("another /64 shared the cap")
	}
	if take("192.0.2.1") != 0 || take("192.0.2.2") != 0 {
		t.Error("IPv4 addresses shared a cap")
	}
	s.release(netip.MustParseAddr("2001:db8:1:2::1"))
	if take("2001:db8:1:2::5") != 0 {
		t.Error("the /64's slot never came back")
	}
}

func TestGlobalCap(t *testing.T) {
	port := serveEach(t, tcpListener(t), echoGreeter)
	s, base, logs := relay(t, "max_conns = 1\n"+world(port, ""))
	a := open(t, base, "127.0.0.1", port)
	readLine(t, a)
	if code := closeCode(t, open(t, base, "127.0.0.1", port)); code != wire.CodeBusy {
		t.Errorf("second: code %d, want %d", code, wire.CodeBusy)
	}
	want := str.RelayRefusedBusy(netip.MustParseAddr("127.0.0.1"), net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if !slices.Contains(logs.all(), want) {
		t.Errorf("no busy refusal logged: %q", logs.all())
	}
	bye(t, a)
	eventually(t, "slot never released", func() bool { return s.inUse() == 0 })
}

func TestPingsKeepLiveClients(t *testing.T) {
	port := serveEach(t, tcpListener(t), echoGreeter)
	s, base, _ := relay(t, world(port, ""))
	s.pingEvery, s.pingTimeout = 30*time.Millisecond, 100*time.Millisecond
	c := open(t, base, "127.0.0.1", port)
	nc := piped(t, c)
	br := bufio.NewReader(nc)
	br.ReadString('\n') // hello; reading also answers pings
	lines := make(chan string, 1)
	go func() { l, _ := br.ReadString('\n'); lines <- l }()
	time.Sleep(300 * time.Millisecond)
	nc.Write([]byte("hi\r\n"))
	select {
	case l := <-lines:
		if l != "you said hi\r\n" {
			t.Errorf("got %q after pings", l)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a live client was dropped")
	}
}

// piped reads the relay's open message from c, then returns c as a byte
// stream to and from the world, unwrapping down frames.
func piped(t *testing.T, c *websocket.Conn) net.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	typ, b, err := c.Read(ctx)
	if m, _ := wire.Decode(b); err != nil || typ != websocket.MessageText || m.T != wire.Open {
		t.Fatalf("first frame %v %q, %v; want an open message", typ, b, err)
	}
	return &stream{Conn: websocket.NetConn(context.Background(), c, websocket.MessageBinary), c: c}
}

// stream is a relayed connection as bytes: Read takes the data out of
// down frames and skips control messages.
type stream struct {
	net.Conn
	c        *websocket.Conn
	deadline time.Time
	rest     []byte
}

func (s *stream) SetReadDeadline(t time.Time) error { s.deadline = t; return nil }

func (s *stream) Read(p []byte) (int, error) {
	for len(s.rest) == 0 {
		ctx := context.Background()
		if !s.deadline.IsZero() {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, s.deadline)
			defer cancel()
		}
		typ, b, err := s.c.Read(ctx)
		if err != nil {
			return 0, err
		}
		if typ == websocket.MessageBinary {
			_, s.rest, _ = wire.ReadDown(b)
		}
	}
	n := copy(p, s.rest)
	s.rest = s.rest[n:]
	return n, nil
}

// bye tells the relay to let c's world go, as the page does when the
// player quits.
func bye(t *testing.T, c *websocket.Conn) {
	t.Helper()
	c.Write(context.Background(), websocket.MessageText, wire.Encode(wire.Msg{T: wire.Close}))
}

// readLine reads one line relayed to c.
func readLine(t *testing.T, c *websocket.Conn) string {
	t.Helper()
	nc := piped(t, c)
	nc.SetReadDeadline(time.Now().Add(2 * time.Second))
	l, err := bufio.NewReader(nc).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func selfSignedListener(t *testing.T) (net.Listener, *x509.Certificate) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "muck.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ln, cert
}

// The relay must give up on a slow world before the page's dial does, so
// the page hears the relay's refusal (catalog text) instead of its own
// timeout. The page's clock starts first: it covers the WebSocket too.
func TestRelayGivesUpBeforeTheClient(t *testing.T) {
	if margin := conn.DialTimeout - defaultHandshakeTimeout; margin < 3*time.Second {
		t.Errorf("relay gives up %v before the client; want 3s or more", margin)
	}
}

// A pinned world that sends no certificate can't be trusted on any retry,
// so it's refused as a bad certificate.
func TestNoCertificateIsACertError(t *testing.T) {
	if !certErr(fmt.Errorf("handshake: %w", errNoCert)) {
		t.Error("no certificate isn't a certificate error")
	}
}
