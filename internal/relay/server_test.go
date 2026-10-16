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
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/conn"
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
	s := &Server{Log: logs.add, FirstFrameTimeout: 300 * time.Millisecond}
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
	w := fmt.Sprintf("[[world]]\nhost = \"127.0.0.1\"\nports = [%d]\n", port)
	switch trust {
	case "":
		return w + "plaintext = true\n"
	case "ca":
		return w + "tls_trust = \"ca\"\n"
	}
	return w + fmt.Sprintf("pin = %q\n", trust)
}

func open(t *testing.T, base, host string, port int) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.Dial(context.Background(), fmt.Sprintf("%s/?host=%s&port=%d", base, host, port), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	return c
}

// closeCode reads until the relay closes c and returns its code.
func closeCode(t *testing.T, c *websocket.Conn) websocket.StatusCode {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		if _, _, err := c.Read(ctx); err != nil {
			return websocket.CloseStatus(err)
		}
	}
}

func TestPlaintextRoundTrip(t *testing.T) {
	port := serveEach(t, tcpListener(t), echoGreeter)
	_, base, _ := relay(t, world(port, ""))
	nc := websocket.NetConn(context.Background(), open(t, base, "127.0.0.1", port), websocket.MessageBinary)
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
		if code := closeCode(t, open(t, base, hp.host, hp.port)); code != CodeNotListed {
			t.Errorf("%v: code %d, want %d", hp, code, CodeNotListed)
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

func TestTLSOnlyRefusesPlainFirstFrame(t *testing.T) {
	port := serveEach(t, tcpListener(t), echoGreeter)
	s, base, _ := relay(t, world(port, "ca"))
	dialed := false
	s.Dial = func(context.Context, string, string) (net.Conn, error) { dialed = true; return nil, net.ErrClosed }
	c := open(t, base, "127.0.0.1", port)
	c.Write(context.Background(), websocket.MessageBinary, []byte("connect kit hunter2\r\n"))
	if code := closeCode(t, c); code != CodeNeedsTLS {
		t.Errorf("code %d, want %d", code, CodeNeedsTLS)
	}
	if dialed {
		t.Error("dialed with a plaintext first frame")
	}
}

func TestTLSOnlyRefusesSilentClient(t *testing.T) {
	port := serveEach(t, tcpListener(t), echoGreeter)
	_, base, _ := relay(t, world(port, "ca"))
	if code := closeCode(t, open(t, base, "127.0.0.1", port)); code != CodeNeedsTLS {
		t.Errorf("code %d, want %d", code, CodeNeedsTLS)
	}
}

func TestTLSThroughRelayPins(t *testing.T) {
	ln, cert := selfSignedListener(t)
	port := serveEach(t, ln, func(c net.Conn) { c.Write([]byte("secure hello\r\n")) })
	_, base, logs := relay(t, world(port, "ca"))
	kh := conn.KnownHosts{Path: filepath.Join(t.TempDir(), "known_hosts")}
	viaRelay := func(ctx context.Context, _, addr string) (net.Conn, error) {
		h, p, _ := net.SplitHostPort(addr)
		c, _, err := websocket.Dial(ctx, base+"/?host="+h+"&port="+p, nil)
		if err != nil {
			return nil, err
		}
		return websocket.NetConn(context.Background(), c, websocket.MessageBinary), nil
	}
	c, err := conn.Dial(context.Background(), conn.Options{Host: "127.0.0.1", Port: port, TLS: true, TLSTrust: "pin",
		KnownHosts: kh, DialContext: viaRelay})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	select {
	case line := <-c.Lines():
		if line != "secure hello" {
			t.Errorf("line %q", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no line")
	}
	if fp, ok, _ := kh.Lookup(net.JoinHostPort("127.0.0.1", strconv.Itoa(port))); !ok || fp != conn.Fingerprint(cert) {
		t.Errorf("pinned %q", fp)
	}
	if !strings.HasPrefix(logs.first(), upTo(str.RelayOpened(mark, mark))) {
		t.Errorf("log %q", logs.first())
	}
}

func TestUnreachable(t *testing.T) {
	ln := tcpListener(t)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close() // nothing listens there now
	_, base, _ := relay(t, world(port, ""))
	if code := closeCode(t, open(t, base, "127.0.0.1", port)); code != CodeUnreachable {
		t.Errorf("code %d, want %d", code, CodeUnreachable)
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
	if code := closeCode(t, open(t, base, "127.0.0.1", port)); code != CodeTooMany {
		t.Errorf("third: code %d, want %d", code, CodeTooMany)
	}
	a.Close(websocket.StatusNormalClosure, "")
	deadline := time.Now().Add(2 * time.Second)
	for {
		c, _, err := websocket.Dial(context.Background(), fmt.Sprintf("%s/?host=127.0.0.1&port=%d", base, port), nil)
		if err != nil {
			t.Fatal(err)
		}
		nc := websocket.NetConn(context.Background(), c, websocket.MessageBinary)
		nc.SetReadDeadline(time.Now().Add(time.Second))
		line, _ := bufio.NewReader(nc).ReadString('\n')
		c.CloseNow()
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
	if code := take("2001:db8:1:2:ffff::9"); code != CodeTooMany {
		t.Errorf("same /64: code %d, want %d", code, CodeTooMany)
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
	if code := closeCode(t, open(t, base, "127.0.0.1", port)); code != CodeBusy {
		t.Errorf("second: code %d, want %d", code, CodeBusy)
	}
	want := str.RelayRefusedBusy(netip.MustParseAddr("127.0.0.1"), net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if !slices.Contains(logs.all(), want) {
		t.Errorf("no busy refusal logged: %q", logs.all())
	}
	a.Close(websocket.StatusNormalClosure, "")
	eventually(t, "slot never released", func() bool { return s.inUse() == 0 })
}

func TestTLSRefusalsReleaseSlots(t *testing.T) {
	port := serveEach(t, tcpListener(t), echoGreeter)
	s, base, _ := relay(t, world(port, "ca"))
	c := open(t, base, "127.0.0.1", port)
	c.Write(context.Background(), websocket.MessageBinary, []byte("connect kit\r\n"))
	if code := closeCode(t, c); code != CodeNeedsTLS {
		t.Fatalf("plain: code %d, want %d", code, CodeNeedsTLS)
	}
	if code := closeCode(t, open(t, base, "127.0.0.1", port)); code != CodeNeedsTLS {
		t.Fatalf("silent: code %d, want %d", code, CodeNeedsTLS)
	}
	eventually(t, "a refused connection kept its slot", func() bool { return s.inUse() == 0 })
}

func TestPingsKeepLiveClients(t *testing.T) {
	port := serveEach(t, tcpListener(t), echoGreeter)
	s, base, _ := relay(t, world(port, ""))
	s.pingEvery, s.pingTimeout = 30*time.Millisecond, 100*time.Millisecond
	c := open(t, base, "127.0.0.1", port)
	nc := websocket.NetConn(context.Background(), c, websocket.MessageBinary)
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

func TestPingsReapSilentClients(t *testing.T) {
	port := serveEach(t, tcpListener(t), echoGreeter)
	s, base, _ := relay(t, world(port, ""))
	s.pingEvery, s.pingTimeout = 30*time.Millisecond, 100*time.Millisecond
	open(t, base, "127.0.0.1", port) // never reads, so never answers a ping
	eventually(t, "connection opened", func() bool { return s.inUse() == 1 })
	eventually(t, "silent client never reaped", func() bool { return s.inUse() == 0 })
}

// readLine reads one line relayed to c.
func readLine(t *testing.T, c *websocket.Conn) string {
	t.Helper()
	nc := websocket.NetConn(context.Background(), c, websocket.MessageBinary)
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
