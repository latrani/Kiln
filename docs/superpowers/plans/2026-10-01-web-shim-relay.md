# Web Kiln Part 1 (Shim, Bridge, Relay) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Kiln runs in a browser tab, drawing into xterm.js, and reaches MUCKs through a WebSocket-to-TCP relay, all testable on a Mac with no deploy.

**Architecture:** A new `web/` Go module (with its own `replace` for a patched Bubble Tea copy) builds Kiln for `GOOS=js`. A plain-Go `bridge` pipes xterm.js keystrokes in and screen bytes out; an in-memory JS filesystem (`memfs`) gives Go's `os` package real files; `wsdial` hands `conn.Dial` a `net.Conn` over a WebSocket, so TLS and telnet stay in Kiln. `kiln-relay` (root module) checks an allowlist and pipes bytes; with `-dev` it also serves the page.

**Tech Stack:** Go 1.27.1, Bubble Tea v2.0.10 (patched copy), `github.com/coder/websocket` v1.8.15, BurntSushi TOML, fsnotify, xterm.js 6.0.0 (+ addon-fit 0.11.0, addon-webgl 0.19.0, addon-clipboard 0.2.0), node (for wasm tests and `node --test`).

**Spec:** `docs/superpowers/specs/2026-10-01-web-shim-relay-design.md`

## Global Constraints

- Every string a person reads goes in `internal/str/locales/en.toml`; run `go generate ./internal/str` and call the generated function. **After generating, read the doc comment in `internal/str/keys_gen.go` for each new function and pass arguments in the order it shows**: the calls written below assume placeholder order of appearance; fix any call that differs.
- Literals that aren't for people (protocol tokens, header names, JS API names, URL paths, flag names) get `//str:ok`.
- Tests build expected text from the catalog (`str.X()`), never a copy; use `mark`/`upTo`/`after` helpers for partial matches.
- A commit that touches `internal/str/locales/` ends with a `Strings:` trailer listing keys, e.g. `Strings: relay.opened (new), relay.closed (new)`.
- Commit messages end with:
  ```
  Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_018MA2F4jiPfcDrTCiELjq8y
  ```
- Never use real character names in the repo: examples use Kit/Rook/Ash and world `fm`.
- The root module must stay `go install`-able: no `replace` directives in the root `go.mod`.
- No new theme roles; nothing here draws in `internal/ui`.
- Work on a branch (`web-part1`), not `main`.
- Relay close codes: 4403 not listed, 4426 needs TLS, 4429 too many, 4502 unreachable. Max 8 connections per IP. Dial timeout 15s. First-frame wait 15s.

## Review Focus

1. **A typo'd key in the allowlist** (`plaintxt = true`, `port = 8899`) → the relay refuses to load it, naming the key, instead of silently ignoring it. *(Task 2 test)*
2. **Host spelled differently from the allowlist** (`PFMuck.Example.` vs `pfmuck.example`) → still matches; the relay dials the normalized name. *(Task 2 test)*
3. **The allowlist is saved half-written or broken while the relay runs** → the old list stays in force and a line is logged. *(Task 4 test)*
4. **A client that keeps getting refused** (unlisted, not TLS) → never uses up its per-IP slots; its count goes back to zero. *(Task 3 test)*
5. **Kiln saves config by writing a temp file and renaming it over the old one** → memfs replaces the file atomically, contents intact. *(Task 6 tests, JS and Go)*

---

### Task 1: `conn.Dial` takes a custom dialer

**Files:**
- Modify: `internal/conn/conn.go:24-32` (Options), `internal/conn/conn.go:46-60` (Dial)
- Test: `internal/conn/conn_test.go`

**Interfaces:**
- Produces: `conn.Options.DialContext func(ctx context.Context, network, addr string) (net.Conn, error)`. nil means a plain TCP dial. `conn.Dial` bounds dial + TLS handshake together by 15s.

- [ ] **Step 1: Switch to the branch (it already holds the spec and this plan)**

```bash
git switch web-part1
```

- [ ] **Step 2: Write the failing tests** (append to `internal/conn/conn_test.go`)

```go
// redirect returns a DialContext that records the address Kiln asked for
// and connects to the test server instead.
func redirect(host string, port int, asked *string) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		*asked = addr
		var d net.Dialer
		return d.DialContext(ctx, network, net.JoinHostPort(host, strconv.Itoa(port)))
	}
}

func TestDialContextIsUsed(t *testing.T) {
	host, port := fakeServer(t, tcpListener(t), func(c net.Conn) { c.Write([]byte("hello\r\n")) })
	var asked string
	c, err := Dial(context.Background(), Options{Host: "muck.test", Port: 4201, DialContext: redirect(host, port, &asked)})
	if err != nil {
		t.Fatal(err)
	}
	if asked != "muck.test:4201" {
		t.Errorf("dialed %q, want muck.test:4201", asked)
	}
	if got := collect(t, c); len(got) != 1 || got[0] != "hello" {
		t.Errorf("got %q", got)
	}
}

func TestTLSPinsThroughDialContext(t *testing.T) {
	kh := KnownHosts{Path: filepath.Join(t.TempDir(), "known_hosts")}
	ln, cert := selfSignedListener(t)
	host, port := fakeServer(t, ln, greet)
	var asked string
	c, err := Dial(context.Background(), Options{Host: "muck.test", Port: 4201, TLS: true, TLSTrust: "pin",
		KnownHosts: kh, DialContext: redirect(host, port, &asked)})
	if err != nil {
		t.Fatal(err)
	}
	if got := collect(t, c); len(got) != 1 || got[0] != "secure hello" {
		t.Errorf("got %q", got)
	}
	if fp, ok, _ := kh.Lookup("muck.test:4201"); !ok || fp != Fingerprint(cert) {
		t.Errorf("pinned %q, want %q", fp, Fingerprint(cert))
	}
}

func TestTLSPinMismatchThroughDialContext(t *testing.T) {
	kh := KnownHosts{Path: filepath.Join(t.TempDir(), "known_hosts")}
	kh.Trust("muck.test:4201", "sha256:0000")
	ln, _ := selfSignedListener(t)
	host, port := fakeServer(t, ln, greet)
	var asked string
	_, err := Dial(context.Background(), Options{Host: "muck.test", Port: 4201, TLS: true, TLSTrust: "pin",
		KnownHosts: kh, DialContext: redirect(host, port, &asked)})
	var pin *PinMismatchError
	if !errors.As(err, &pin) {
		t.Fatalf("err = %v, want PinMismatchError", err)
	}
}
```

- [ ] **Step 3: Run them; they fail to compile**

Run: `go test ./internal/conn/ -run 'DialContext' -v`
Expected: FAIL, `unknown field DialContext in struct literal`

- [ ] **Step 4: Implement**

In `Options`, after `Height`:

```go
	// DialContext opens the raw connection (TLS, if on, runs over it).
	// nil: a plain TCP dial. The web build dials through kiln-relay.
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
```

Replace the start of `Dial` up to the `c := &Conn{` line with:

```go
func Dial(ctx context.Context, o Options) (*Conn, error) {
	hostport := net.JoinHostPort(o.Host, strconv.Itoa(o.Port))
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second) // dial and handshake together
	defer cancel()
	dial := o.DialContext
	if dial == nil {
		d := &net.Dialer{KeepAlive: 30 * time.Second}
		dial = d.DialContext
	}
	nc, err := dial(ctx, "tcp", hostport)
	if err != nil {
		return nil, err
	}
	if o.TLS {
		tc := tls.Client(nc, tlsConfig(o, hostport))
		if err := tc.HandshakeContext(ctx); err != nil {
			nc.Close()
			var pin *PinMismatchError
			if errors.As(err, &pin) {
				return nil, pin
			}
			return nil, err
		}
		nc = tc
	}
```

(`"tcp"` is already a protocol token the lint accepts; if it flags it, add `//str:ok`.)

- [ ] **Step 5: Run the whole package**

Run: `go test ./internal/conn/ -v`
Expected: PASS, including the existing `TestTLSPinsOnFirstUseThenAccepts`, `TestTLSPinMismatchRefuses` and `TestTLSCAModeRejectsSelfSigned`.

- [ ] **Step 6: Run everything and commit**

```bash
go test ./...
git add internal/conn
git commit -m "feat(conn): Dial takes a custom dialer, for the web build's relay"
```

---

### Task 2: The relay's allowlist

**Files:**
- Create: `internal/relay/allow.go`, `internal/relay/allow_test.go`, `internal/relay/catalog_test.go`
- Modify: `internal/str/locales/en.toml` (new `[relay]` section), regenerate `internal/str/keys_gen.go`

**Interfaces:**
- Produces:
  ```go
  type World struct { Host string; Ports []int; Plaintext bool }
  type Allowlist struct { Worlds []World; Origins []string; TrustedProxies []string }
  func ParseAllowlist(path string, b []byte) (*Allowlist, error)
  func LoadAllowlist(path string) (*Allowlist, error)
  func (a *Allowlist) Lookup(host string, port int) (World, bool) // host normalized: trimmed, lowercased, no trailing dot
  func (a *Allowlist) Trusted(ip netip.Addr) bool
  ```

- [ ] **Step 1: Add the strings** (new section at the end of `internal/str/locales/en.toml`)

```toml
[relay]
allow_reading = "reading allowlist {path}: {err}"
allow_unknown_key = "allowlist {path}: unknown setting {key:%q}"
allow_no_host = "allowlist {path}: world {n} has no host"
allow_no_ports = "allowlist {path}: {host} has no ports"
allow_bad_port = "allowlist {path}: {host} has bad port {port}"
allow_bad_proxy = "allowlist {path}: trusted_proxies has bad address {addr:%q}"
```

Run: `go generate ./internal/str` and check the signatures in `keys_gen.go` (`RelayAllowReading(path any, err error)`, `RelayAllowNoHost(path any, n int)`, …).

- [ ] **Step 2: Add the test helpers** (`internal/relay/catalog_test.go`)

```go
package relay

import "strings"

// Tests build the text they expect from the catalog (package str), so
// rewording a string doesn't break them. These cut a message around a
// placeholder filled with mark.
const mark = "\x00"

// upTo is s up to the first mark.
func upTo(s string) string { h, _, _ := strings.Cut(s, mark); return h }

// errMark is an error that prints as mark, for cutting messages with {err}.
type errMark struct{}

func (errMark) Error() string { return mark }
```

- [ ] **Step 3: Write the failing tests** (`internal/relay/allow_test.go`)

```go
package relay

import (
	"net/netip"
	"testing"

	"github.com/latrani/Kiln/internal/str"
)

const sample = `
origins = ["muck.example.org"]
trusted_proxies = ["127.0.0.1", "10.0.0.0/8"]

[[world]]
host = "muck.example.org"
ports = [8888, 8899]
plaintext = true

[[world]]
host = "Other.Example."
ports = [7000]
`

func TestParseAndLookup(t *testing.T) {
	a, err := ParseAllowlist("allow.toml", []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	w, ok := a.Lookup("MUCK.example.org.", 8899)
	if !ok || !w.Plaintext || w.Host != "muck.example.org" {
		t.Errorf("Lookup = %+v, %v", w, ok)
	}
	if w, ok := a.Lookup("other.example", 7000); !ok || w.Plaintext || w.Host != "other.example" {
		t.Errorf("other = %+v, %v; want TLS-only other.example", w, ok)
	}
	if _, ok := a.Lookup("muck.example.org", 7000); ok {
		t.Error("an unlisted port matched")
	}
	if _, ok := a.Lookup("evil.example", 8888); ok {
		t.Error("an unlisted host matched")
	}
}

func TestTrusted(t *testing.T) {
	a, _ := ParseAllowlist("allow.toml", []byte(sample))
	for ip, want := range map[string]bool{
		"127.0.0.1": true, "::ffff:127.0.0.1": true, "10.2.3.4": true, "192.168.1.1": false,
	} {
		if got := a.Trusted(netip.MustParseAddr(ip)); got != want {
			t.Errorf("Trusted(%s) = %v, want %v", ip, got, want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, c := range []struct{ name, toml, want string }{
		{"typo", "[[world]]\nhost = \"a.example\"\nports = [1]\nplaintxt = true\n",
			str.RelayAllowUnknownKey("allow.toml", "world.plaintxt")},
		{"singular port", "[[world]]\nhost = \"a.example\"\nport = 1\n",
			str.RelayAllowUnknownKey("allow.toml", "world.port")},
		{"no host", "[[world]]\nports = [1]\n", str.RelayAllowNoHost("allow.toml", 1)},
		{"no ports", "[[world]]\nhost = \"a.example\"\n", str.RelayAllowNoPorts("allow.toml", "a.example")},
		{"bad port", "[[world]]\nhost = \"a.example\"\nports = [70000]\n", str.RelayAllowBadPort("allow.toml", "a.example", 70000)},
		{"bad proxy", "trusted_proxies = [\"nope\"]\n", str.RelayAllowBadProxy("allow.toml", "nope")},
	} {
		_, err := ParseAllowlist("allow.toml", []byte(c.toml))
		if err == nil || err.Error() != c.want {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
}

func TestParseBadTOML(t *testing.T) {
	_, err := ParseAllowlist("allow.toml", []byte("[[world]\n"))
	if err == nil || !strings.HasPrefix(err.Error(), upTo(str.RelayAllowReading("allow.toml", errMark{}))) {
		t.Errorf("err = %v", err)
	}
}
```

(Add `"strings"` to the imports.)

- [ ] **Step 4: Run; they fail to compile**

Run: `go test ./internal/relay/`
Expected: FAIL, `undefined: ParseAllowlist`

- [ ] **Step 5: Implement** (`internal/relay/allow.go`)

```go
// Package relay is kiln-relay's core: it carries a browser's WebSocket to
// a MUCK's TCP port, for the worlds on an allowlist, so Kiln's web build
// can reach MUCKs. TLS runs inside the browser; the relay passes bytes.
package relay

import (
	"errors"
	"net/netip"
	"os"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/latrani/Kiln/internal/str"
)

// World is a MUCK the relay will connect to.
type World struct {
	Host  string `toml:"host"`
	Ports []int  `toml:"ports"`
	// Plaintext allows non-TLS traffic. Off, the relay refuses anything
	// that doesn't start with a TLS handshake, so whoever runs the relay
	// never sees players' passwords. Meant for the operator's own MUCK.
	Plaintext bool `toml:"plaintext"`
}

// Allowlist is the relay's config file.
type Allowlist struct {
	Worlds []World `toml:"world"`
	// Origins are extra host patterns allowed to open the relay from a
	// page (the relay's own host always is).
	Origins []string `toml:"origins"`
	// TrustedProxies (addresses or CIDRs) may say who the client is in
	// X-Forwarded-For.
	TrustedProxies []string `toml:"trusted_proxies"`

	proxies []netip.Prefix
}

// LoadAllowlist reads and checks the allowlist at path.
func LoadAllowlist(path string) (*Allowlist, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, str.Wrap(str.RelayAllowReading(path, err), err)
	}
	return ParseAllowlist(path, b)
}

// ParseAllowlist checks an allowlist file's contents; path is for messages.
// Unknown settings are errors: a typo in a security setting must not be
// silently ignored.
func ParseAllowlist(path string, b []byte) (*Allowlist, error) {
	var a Allowlist
	md, err := toml.Decode(string(b), &a)
	if err != nil {
		return nil, str.Wrap(str.RelayAllowReading(path, err), err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		return nil, errors.New(str.RelayAllowUnknownKey(path, und[0].String()))
	}
	for i, w := range a.Worlds {
		h := normHost(w.Host)
		if h == "" {
			return nil, errors.New(str.RelayAllowNoHost(path, i+1))
		}
		if len(w.Ports) == 0 {
			return nil, errors.New(str.RelayAllowNoPorts(path, h))
		}
		for _, p := range w.Ports {
			if p < 1 || p > 65535 {
				return nil, errors.New(str.RelayAllowBadPort(path, h, p))
			}
		}
		a.Worlds[i].Host = h
	}
	for _, s := range a.TrustedProxies {
		p, err := parsePrefix(s)
		if err != nil {
			return nil, errors.New(str.RelayAllowBadProxy(path, s))
		}
		a.proxies = append(a.proxies, p)
	}
	return &a, nil
}

// Lookup finds the world for host:port, comparing host case-insensitively
// and ignoring a trailing dot.
func (a *Allowlist) Lookup(host string, port int) (World, bool) {
	h := normHost(host)
	for _, w := range a.Worlds {
		if w.Host == h && slices.Contains(w.Ports, port) {
			return w, true
		}
	}
	return World{}, false
}

// Trusted reports whether ip is a trusted proxy.
func (a *Allowlist) Trusted(ip netip.Addr) bool {
	ip = ip.Unmap()
	for _, p := range a.proxies {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

func normHost(h string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(h)), ".")
}

func parsePrefix(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		return netip.ParsePrefix(s)
	}
	ip, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	ip = ip.Unmap()
	return netip.PrefixFrom(ip, ip.BitLen()), nil
}
```

- [ ] **Step 6: Run the tests**

Run: `go test ./internal/relay/ ./internal/str/ -v`
Expected: PASS. If `TestParseErrors/typo` shows a different key spelling from `md.Undecoded()` (e.g. `world.plaintxt` vs `world[0].plaintxt`), make the test expect what `Key.String()` really prints. That's the format people will see.

- [ ] **Step 7: Commit**

```bash
git add internal/relay internal/str
git commit -m "feat(relay): the allowlist kiln-relay checks before dialing

Strings: relay.allow_reading, relay.allow_unknown_key, relay.allow_no_host, relay.allow_no_ports, relay.allow_bad_port, relay.allow_bad_proxy (new)"
```

---

### Task 3: The relay server

**Files:**
- Create: `internal/relay/server.go`, `internal/relay/server_test.go`
- Modify: `go.mod`/`go.sum` (add `github.com/coder/websocket`), `internal/str/locales/en.toml`

**Interfaces:**
- Consumes: `Allowlist`, `World` (Task 2); `conn.Dial` with `DialContext` (Task 1, in tests).
- Produces:
  ```go
  const CodeNotListed, CodeNeedsTLS, CodeTooMany, CodeUnreachable websocket.StatusCode // 4403, 4426, 4429, 4502
  type Server struct {
      Dial              func(ctx context.Context, network, addr string) (net.Conn, error) // nil: TCP
      Log               func(line string)  // nil: silent
      MaxPerIP          int                // 0: 8
      FirstFrameTimeout time.Duration      // 0: 15s
  }
  func (s *Server) SetAllowlist(a *Allowlist)
  func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request)
  ```
  Clients connect to `<relay URL>?host=H&port=P`.

- [ ] **Step 1: Add the dependency and strings**

```bash
go get github.com/coder/websocket@v1.8.15
```

Append to `[relay]` in `en.toml`:

```toml
opened = "open {ip} → {world}"
closed = "closed {ip} → {world}: {up} bytes up, {down} down, {dur}"
refused_not_listed = "refused {ip} → {world}: not on the allowlist"
refused_not_tls = "refused {ip} → {world}: not TLS"
refused_too_many = "refused {ip} → {world}: too many connections"
refused_unreachable = "refused {ip} → {world}: {err}"
```

Run `go generate ./internal/str`.

- [ ] **Step 2: Write the failing tests** (`internal/relay/server_test.go`)

```go
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
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
	a, err := ParseAllowlist("allow.toml", []byte(allow))
	if err != nil {
		t.Fatal(err)
	}
	logs := &logLines{}
	s := &Server{Log: logs.add, FirstFrameTimeout: 300 * time.Millisecond}
	s.SetAllowlist(a)
	hs := httptest.NewServer(s)
	t.Cleanup(hs.Close)
	return s, "ws" + strings.TrimPrefix(hs.URL, "http"), logs
}

type logLines struct {
	mu    sync.Mutex
	lines []string
}

func (l *logLines) add(s string) { l.mu.Lock(); l.lines = append(l.lines, s); l.mu.Unlock() }

// first is the first line logged, or "" (copied under the lock: -race).
func (l *logLines) first() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.lines) == 0 {
		return ""
	}
	return l.lines[0]
}

func world(port int, plaintext bool) string {
	return fmt.Sprintf("[[world]]\nhost = \"127.0.0.1\"\nports = [%d]\nplaintext = %v\n", port, plaintext)
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
	_, base, _ := relay(t, world(port, true))
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
	s, base, logs := relay(t, world(port, true))
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

func TestTLSOnlyRefusesPlainFirstFrame(t *testing.T) {
	port := serveEach(t, tcpListener(t), echoGreeter)
	s, base, _ := relay(t, world(port, false))
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
	_, base, _ := relay(t, world(port, false))
	if code := closeCode(t, open(t, base, "127.0.0.1", port)); code != CodeNeedsTLS {
		t.Errorf("code %d, want %d", code, CodeNeedsTLS)
	}
}

func TestTLSThroughRelayPins(t *testing.T) {
	ln, cert := selfSignedListener(t)
	port := serveEach(t, ln, func(c net.Conn) { c.Write([]byte("secure hello\r\n")) })
	_, base, logs := relay(t, world(port, false))
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
	_, base, _ := relay(t, world(port, true))
	if code := closeCode(t, open(t, base, "127.0.0.1", port)); code != CodeUnreachable {
		t.Errorf("code %d, want %d", code, CodeUnreachable)
	}
}

func TestWrongOriginRefused(t *testing.T) {
	port := serveEach(t, tcpListener(t), echoGreeter)
	_, base, _ := relay(t, world(port, true))
	_, resp, err := websocket.Dial(context.Background(), fmt.Sprintf("%s/?host=127.0.0.1&port=%d", base, port),
		&websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"https://evil.example"}}})
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("err %v, resp %v; want 403", err, resp)
	}
}

func TestPerIPCapAndRelease(t *testing.T) {
	port := serveEach(t, tcpListener(t), echoGreeter)
	s, base, _ := relay(t, world(port, true))
	s.MaxPerIP = 2
	// Refusals must not use up slots.
	for range 3 {
		closeCode(t, open(t, base, "127.0.0.1", port+1))
	}
	a := open(t, base, "127.0.0.1", port)
	open(t, base, "127.0.0.1", port)
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
	a, _ := ParseAllowlist("allow.toml", []byte(`trusted_proxies = ["127.0.0.1"]`))
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
		if got := s.clientIP(req(c.remote, c.xff), a); got != netip.MustParseAddr(c.want) {
			t.Errorf("%s via %q = %s, want %s", c.remote, c.xff, got, c.want)
		}
	}
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
```

`TestTLSThroughRelayPins` is the proof that TLS runs end to end in Kiln: the relay only ever forwards the handshake bytes.

- [ ] **Step 3: Run; they fail to compile**

Run: `go test ./internal/relay/`
Expected: FAIL, `undefined: Server`

- [ ] **Step 4: Implement** (`internal/relay/server.go`)

```go
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
		rctx, rcancel := context.WithTimeout(ctx, or(s.FirstFrameTimeout, 15*time.Second))
		typ, b, err := c.Read(rctx)
		rcancel()
		if err != nil && websocket.CloseStatus(err) != -1 {
			return // the client went away
		}
		if err != nil || typ != websocket.MessageBinary || len(b) == 0 || b[0] != 0x16 { // 0x16: TLS handshake record
			s.logf(str.RelayRefusedNotTLS(ip, addr))
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
```

Note the order: the allowlist check comes before `acquire`, so refused clients never hold a slot (Review Focus 4).

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/relay/ -race -v`
Expected: PASS. If `TestWrongOriginRefused` gets a non-403, check `coder/websocket`'s docs for how `Accept` answers a bad Origin and match that, but it must fail the dial.

- [ ] **Step 6: Lint and commit**

```bash
go test ./... 
git add go.mod go.sum internal/relay internal/str
git commit -m "feat(relay): WebSocket-to-TCP relay with TLS-only worlds and per-IP limits

Strings: relay.opened, relay.closed, relay.refused_not_listed, relay.refused_not_tls, relay.refused_too_many, relay.refused_unreachable (new)"
```

---

### Task 4: `kiln-relay` command, allowlist reload, dev server, deploy recipe

**Files:**
- Create: `internal/relay/watch.go`, `internal/relay/watch_test.go`, `cmd/kiln-relay/main.go`, `cmd/kiln-relay/dev.go`, `cmd/kiln-relay/main_test.go`, `docs/web-relay.md`
- Modify: `internal/str/locales/en.toml`

**Interfaces:**
- Consumes: `Server`, `LoadAllowlist` (Tasks 2–3).
- Produces: `func WatchAllowlist(s *Server, path string) (io.Closer, error)`. Command: `kiln-relay -allow PATH [-listen ADDR] [-dev -web DIR -preset DIR]`. Dev routes: `/kiln/` (page), `/kiln/relay`, `/kiln/preset/manifest.json` (JSON array of slash paths), `/kiln/preset/…`.

- [ ] **Step 1: Strings**

Append to `[relay]`:

```toml
allow_loaded = { one = "allowlist {path}: 1 world", other = "allowlist {path}: {n} worlds" }
allow_reload_failed = "allowlist not reloaded, keeping the old one: {err}"
listening = "kiln-relay listening on {addr}"
dev_serving = "dev: open {url}"
need_allow = "kiln-relay needs -allow PATH (the allowlist file)"
dev_needs_dirs = "-dev needs -web DIR and -preset DIR"
flag_listen = "address to listen on (default 127.0.0.1:7801, or localhost:8080 with -dev)"
flag_allow = "allowlist file (TOML)"
flag_dev = "also serve the web page and a preset config dir, for local testing"
flag_web = "with -dev: the built page (web/static)"
flag_preset = "with -dev: the config dir to preload in the browser"
```

Run `go generate ./internal/str`.

- [ ] **Step 2: Write the failing reload test** (`internal/relay/watch_test.go`)

```go
package relay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/str"
)

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatal(what)
}

func TestWatchAllowlistReloadsAndKeepsOldOnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "allow.toml")
	os.WriteFile(path, []byte(world(1000, true)), 0o644)
	logs := &logLines{}
	s := &Server{Log: logs.add}
	w, err := WatchAllowlist(s, path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, ok := s.allow.Load().Lookup("127.0.0.1", 1000); !ok {
		t.Fatal("initial list not loaded")
	}

	os.WriteFile(path, []byte(world(2000, true)), 0o644)
	eventually(t, "new list never loaded", func() bool { _, ok := s.allow.Load().Lookup("127.0.0.1", 2000); return ok })

	os.WriteFile(path, []byte("[[world]\nhost = "), 0o644) // half-saved
	eventually(t, "no reload error logged", func() bool {
		logs.mu.Lock()
		defer logs.mu.Unlock()
		for _, l := range logs.lines {
			if strings.HasPrefix(l, upTo(str.RelayAllowReloadFailed(errMark{}))) {
				return true
			}
		}
		return false
	})
	if _, ok := s.allow.Load().Lookup("127.0.0.1", 2000); !ok {
		t.Error("a broken file replaced the working list")
	}
}

func TestWatchAllowlistFailsOnBadStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "allow.toml")
	os.WriteFile(path, []byte("nope = 1\n"), 0o644)
	if _, err := WatchAllowlist(&Server{}, path); err == nil {
		t.Error("started with a bad allowlist")
	}
}
```

- [ ] **Step 3: Run; it fails**

Run: `go test ./internal/relay/ -run Watch`
Expected: FAIL, `undefined: WatchAllowlist`

- [ ] **Step 4: Implement** (`internal/relay/watch.go`)

```go
package relay

import (
	"io"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/latrani/Kiln/internal/str"
)

// WatchAllowlist loads path into s, then reloads it whenever it changes.
// A file that fails to load (half-saved, a typo) is logged and the old
// list kept. Close the result to stop watching.
func WatchAllowlist(s *Server, path string) (io.Closer, error) {
	a, err := LoadAllowlist(path)
	if err != nil {
		return nil, err
	}
	s.SetAllowlist(a)
	s.logf(str.RelayAllowLoaded(len(a.Worlds), path))
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	// Watch the directory: editors often save by replacing the file.
	if err := fw.Add(filepath.Dir(path)); err != nil {
		fw.Close()
		return nil, err
	}
	go func() {
		settle := time.NewTimer(time.Hour)
		settle.Stop()
		for {
			select {
			case ev, ok := <-fw.Events:
				if !ok {
					return
				}
				if filepath.Clean(ev.Name) == filepath.Clean(path) {
					settle.Reset(200 * time.Millisecond)
				}
			case _, ok := <-fw.Errors:
				if !ok {
					return
				}
			case <-settle.C:
				a, err := LoadAllowlist(path)
				if err != nil {
					s.logf(str.RelayAllowReloadFailed(err))
					continue
				}
				s.SetAllowlist(a)
				s.logf(str.RelayAllowLoaded(len(a.Worlds), path))
			}
		}
	}()
	return fw, nil
}
```

- [ ] **Step 5: Run**

Run: `go test ./internal/relay/ -race`
Expected: PASS

- [ ] **Step 6: Write the failing command tests** (`cmd/kiln-relay/main_test.go`)

```go
package main

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/latrani/Kiln/internal/str"
)

func TestManifestListsFilesAsSlashPaths(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "worlds"), 0o755)
	os.WriteFile(filepath.Join(dir, "config.toml"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), nil, 0o644)
	got, err := manifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"config.toml", "worlds/fm.toml"}; !slices.Equal(got, want) {
		t.Errorf("manifest = %q, want %q", got, want)
	}
}

func TestRunNeedsAllow(t *testing.T) {
	if err := run(nil, io.Discard); err == nil || err.Error() != str.RelayNeedAllow() {
		t.Errorf("err = %v", err)
	}
}

func TestDevNeedsDirs(t *testing.T) {
	if err := run([]string{"-allow", "x.toml", "-dev"}, io.Discard); err == nil || err.Error() != str.RelayDevNeedsDirs() {
		t.Errorf("err = %v", err)
	}
}
```

- [ ] **Step 7: Implement** (`cmd/kiln-relay/main.go`)

```go
// Command kiln-relay lets Kiln's web build reach MUCKs: browsers can't
// open TCP connections, so the page opens a WebSocket here and the relay
// connects to the MUCK, for the worlds in its allowlist. See
// docs/web-relay.md.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/latrani/Kiln/internal/relay"
	"github.com/latrani/Kiln/internal/str"
)

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, str.CliError(err))
		os.Exit(1)
	}
}

func run(args []string, logw io.Writer) error {
	fs := flag.NewFlagSet("kiln-relay", flag.ContinueOnError) //str:ok
	listen := fs.String("listen", "", str.RelayFlagListen())  //str:ok
	allow := fs.String("allow", "", str.RelayFlagAllow())     //str:ok
	dev := fs.Bool("dev", false, str.RelayFlagDev())          //str:ok
	web := fs.String("web", "", str.RelayFlagWeb())           //str:ok
	preset := fs.String("preset", "", str.RelayFlagPreset())  //str:ok
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *allow == "" {
		return errors.New(str.RelayNeedAllow())
	}
	if *dev && (*web == "" || *preset == "") {
		return errors.New(str.RelayDevNeedsDirs())
	}
	addr := *listen
	if addr == "" {
		addr = "127.0.0.1:7801"
		if *dev {
			addr = "localhost:8080"
		}
	}
	logf := func(line string) { fmt.Fprintln(logw, time.Now().Format(time.DateTime), line) }
	s := &relay.Server{Log: logf}
	w, err := relay.WatchAllowlist(s, *allow)
	if err != nil {
		return err
	}
	defer w.Close()
	mux := http.NewServeMux()
	mux.Handle("/kiln/relay", s) //str:ok
	if *dev {
		devRoutes(mux, *web, *preset)
		logf(str.RelayDevServing("http://" + addr + "/kiln/")) //str:ok
	}
	logf(str.RelayListening(addr))
	return http.ListenAndServe(addr, mux)
}
```

`cmd/kiln-relay/dev.go`:

```go
package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"path/filepath"
	"sort"
)

// devRoutes serves the page and a preset config dir from one origin, the
// way production's nginx will, so the whole thing runs on a laptop.
func devRoutes(mux *http.ServeMux, web, preset string) {
	mux.Handle("/kiln/", http.StripPrefix("/kiln/", http.FileServer(http.Dir(web)))) //str:ok
	mux.HandleFunc("/kiln/preset/manifest.json", func(w http.ResponseWriter, r *http.Request) { //str:ok
		m, err := manifest(preset)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json") //str:ok
		json.NewEncoder(w).Encode(m)
	})
	mux.Handle("/kiln/preset/", http.StripPrefix("/kiln/preset/", http.FileServer(http.Dir(preset)))) //str:ok
}

// manifest lists the files under dir as sorted slash paths relative to
// it: what the page copies into its in-memory config dir.
func manifest(dir string) ([]string, error) {
	out := []string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out, err
}
```

- [ ] **Step 8: Run**

Run: `go test ./cmd/kiln-relay/ ./internal/... -race`
Expected: PASS (the str lint included)

- [ ] **Step 9: Write `docs/web-relay.md`** (the production recipe; not deployed in this plan)

````markdown
# Running kiln-relay

Kiln's web build can't open TCP connections, so the page opens a
WebSocket to `kiln-relay`, which connects to the MUCK. It only connects to
worlds in its allowlist, and only passes TLS traffic unless a world says
`plaintext = true`. So whoever runs the relay never sees other worlds'
passwords.

## Allowlist

```toml
# Hosts (beyond the relay's own) whose pages may use this relay.
origins = ["muck.example.org"]
# Who may say who the client is (X-Forwarded-For): your nginx.
trusted_proxies = ["127.0.0.1"]

[[world]]
host = "muck.example.org"
ports = [8899]
plaintext = true   # only for your own MUCK

[[world]]
host = "friends.example.net"
ports = [8888]     # TLS only
```

The relay reloads the file when it changes. If the new file has an
error, it logs it and keeps the old list.

## nginx

```nginx
location /kiln/ {
    alias /srv/kiln/static/;
}
location = /kiln/relay {
    proxy_pass http://127.0.0.1:7801;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_read_timeout 1d;
    proxy_send_timeout 1d;
}
```

## systemd

```ini
[Unit]
Description=Kiln web relay
After=network-online.target

[Service]
ExecStart=/usr/local/bin/kiln-relay -allow /etc/kiln-relay/allow.toml
DynamicUser=yes
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

## Local testing

See `docs/web-dev.md`.
````

- [ ] **Step 10: Commit**

```bash
git add cmd/kiln-relay internal/relay internal/str docs/web-relay.md
git commit -m "feat(relay): kiln-relay command, allowlist reload, and a -dev server

Strings: relay.allow_loaded, relay.allow_reload_failed, relay.listening, relay.dev_serving, relay.need_allow, relay.dev_needs_dirs, relay.flag_listen, relay.flag_allow, relay.flag_dev, relay.flag_web, relay.flag_preset (new)"
```

---

### Task 5: The `web/` module, the Bubble Tea copy, and the bridge

**Files:**
- Create: `web/go.mod`, `web/third_party/bubbletea/**` (copy), `web/third_party/bubbletea/tty_js.go`, `web/bridge/bridge.go`, `web/bridge/bridge_test.go`
- Modify: `internal/str/lint_test.go` (walk `web/`, skip `web/third_party`), `.gitignore`

**Interfaces:**
- Produces:
  ```go
  package bridge // github.com/latrani/Kiln/web/bridge
  func New(out func([]byte)) *Bridge          // out must copy p before returning
  func (b *Bridge) Options(cols, rows int) []tea.ProgramOption
  func (b *Bridge) Attach(p *tea.Program)
  func (b *Bridge) Input(s string)            // never blocks
  func (b *Bridge) Resize(cols, rows int)     // never blocks; latest size wins
  func (b *Bridge) Close()                    // input EOF
  ```

- [ ] **Step 1: Copy Bubble Tea v2.0.10**

```bash
mkdir -p web/third_party
rsync -a --exclude '*_test.go' --exclude testdata --exclude Taskfile.yaml \
  "$(go env GOMODCACHE)/charm.land/bubbletea/v2@v2.0.10/" web/third_party/bubbletea/
chmod -R u+w web/third_party/bubbletea
```

- [ ] **Step 2: Add `web/third_party/bubbletea/tty_js.go`**

```go
//go:build js

package tea

// Added for Kiln's browser build (web/): GOOS=js has no TTY. Input
// arrives already raw from xterm.js, resizes arrive as WindowSizeMsg, and
// a browser tab can't be suspended. Shaped like tty_windows.go and
// signals_windows.go.

func (p *Program) initInput() error { return nil }

func (p *Program) listenForResize(done chan struct{}) { close(done) }

const suspendSupported = false

func suspendProcess() {}
```

Also add `web/third_party/bubbletea/KILN.md`: one paragraph saying this is v2.0.10 plus `tty_js.go`, why it's here, and that it goes away once upstream builds for `js/wasm`.

- [ ] **Step 3: Create `web/go.mod`**

```
module github.com/latrani/Kiln/web

go 1.27.1

require (
	charm.land/bubbletea/v2 v2.0.10
	github.com/latrani/Kiln v0.0.0
)

replace (
	// v2.0.10 plus tty_js.go: upstream doesn't build for GOOS=js yet.
	charm.land/bubbletea/v2 => ./third_party/bubbletea
	github.com/latrani/Kiln => ../
)
```

- [ ] **Step 4: Write the failing bridge tests** (`web/bridge/bridge_test.go`)

```go
package bridge

import (
	"bytes"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// probe reports the keys and sizes it sees, and quits on "q".
type probe struct{ got chan tea.Msg }

func (p probe) Init() tea.Cmd { return nil }

func (p probe) Update(m tea.Msg) (tea.Model, tea.Cmd) {
	switch m := m.(type) {
	case tea.KeyPressMsg:
		p.got <- m
		if m.String() == "q" {
			return p, tea.Quit
		}
	case tea.WindowSizeMsg:
		p.got <- m
	}
	return p, nil
}

func (p probe) View() tea.View { return tea.NewView("probe view") }

type screen struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *screen) write(p []byte) { s.mu.Lock(); s.buf.Write(p); s.mu.Unlock() }

// start runs a probe on a bridge and returns its message channel and a
// wait func that returns once the program has exited.
func start(t *testing.T, b *Bridge) (chan tea.Msg, func()) {
	t.Helper()
	got := make(chan tea.Msg, 64)
	p := tea.NewProgram(probe{got}, b.Options(80, 24)...)
	b.Attach(p)
	done := make(chan struct{})
	go func() { p.Run(); close(done) }()
	t.Cleanup(func() { p.Kill() })
	return got, func() {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("program didn't exit")
		}
	}
}

// next returns the next message of type T, skipping others.
func next[T tea.Msg](t *testing.T, got chan tea.Msg) T {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case m := <-got:
			if v, ok := m.(T); ok {
				return v
			}
		case <-timeout:
			var zero T
			t.Fatalf("no %T", zero)
			return zero
		}
	}
}

func TestInputArrivesInOrder(t *testing.T) {
	b := New(func([]byte) {})
	b.Input("x") // before the program starts: kept, not dropped
	got, _ := start(t, b)
	b.Input("a")
	b.Input("bc")
	var keys []string
	for range 4 {
		keys = append(keys, next[tea.KeyPressMsg](t, got).String())
	}
	if want := "x a b c"; joined(keys) != want {
		t.Errorf("keys %q, want %q", joined(keys), want)
	}
}

func TestStartSizeAndResize(t *testing.T) {
	b := New(func([]byte) {})
	got, _ := start(t, b)
	if m := next[tea.WindowSizeMsg](t, got); m.Width != 80 || m.Height != 24 {
		t.Errorf("start size %v", m)
	}
	b.Resize(100, 40)
	if m := next[tea.WindowSizeMsg](t, got); m.Width != 100 || m.Height != 40 {
		t.Errorf("resize %v", m)
	}
}

func TestResizeBeforeAttachDoesNotBlock(t *testing.T) {
	b := New(func([]byte) {})
	done := make(chan struct{})
	go func() { b.Resize(1, 1); b.Resize(2, 2); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Resize blocked")
	}
}

func TestOutputReachesScreen(t *testing.T) {
	var s screen
	b := New(s.write)
	got, wait := start(t, b)
	next[tea.WindowSizeMsg](t, got)
	b.Input("q")
	wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !bytes.Contains(s.buf.Bytes(), []byte("probe view")) {
		t.Errorf("screen %q", s.buf.String())
	}
}

func joined(ss []string) string {
	var b bytes.Buffer
	for i, s := range ss {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(s)
	}
	return b.String()
}
```

- [ ] **Step 5: Run; they fail to compile**

Run: `cd web && go mod tidy && go test ./bridge/`
Expected: FAIL, `undefined: New`. (If `go mod tidy` complains first, write Step 6's file and rerun.)

- [ ] **Step 6: Implement** (`web/bridge/bridge.go`)

```go
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
```

- [ ] **Step 7: Run natively, then build everything for js**

```bash
cd web && go mod tidy && go test ./bridge/ -race -v
GOOS=js GOARCH=wasm go build ./... github.com/latrani/Kiln/internal/ui
```

Expected: tests PASS; the js build succeeds. That proves the shim, since on `main` `internal/ui` fails to build for js with `p.listenForResize undefined`. If `TestOutputReachesScreen` finds the view text split by escape codes, assert on `probe` and `view` separately.

- [ ] **Step 8: Teach the str lint about `web/`**

In `internal/str/lint_test.go`, both walks (`TestNoStrayStrings` and the phrase walk in `TestTestsReadTheCatalog`) loop over `[]string{"cmd", "internal"}`. Make both `[]string{"cmd", "internal", "web"}`, and in each `WalkDir` callback skip the copy:

```go
			if d.IsDir() && path == filepath.Join(root, "web", "third_party") {
				return filepath.SkipDir
			}
```

Then: `go test ./internal/str/`. Expected: PASS (the `//str:ok` on `TERM=` covers bridge.go). Fix any line it flags the way Global Constraints say.

- [ ] **Step 9: Ignore build outputs** (append to `.gitignore`)

```
/web/static/kiln.wasm
/web/static/wasm_exec.js
/web/preset.local/
/web/dev-allow.toml
```

- [ ] **Step 10: Commit**

```bash
git add web .gitignore internal/str/lint_test.go
git commit -m "feat(web): web module with a js-buildable Bubble Tea copy, and the terminal bridge"
```

---

### Task 6: memfs, the browser's filesystem

**Files:**
- Create: `web/static/memfs.mjs`, `web/static/memfs.test.mjs`, `web/testdata/memfs_exec.mjs`, `web/testdata/memfs_exec.sh`, `web/fstest/fs_test.go`

**Interfaces:**
- Produces (JS):
  ```js
  export const constants;               // O_* flags
  export function createFS({ stdout, stderr } = {}) // returns the fs object for globalThis.fs
  // plus, on that object: mkdirAll(path), writeFileAll(path, Uint8Array)
  ```
  Test runner: `GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...` (run in `web/`), with `HOME=/home/kiln`, `TMPDIR=/tmp`.

- [ ] **Step 1: Write the failing node tests** (`web/static/memfs.test.mjs`)

```js
import { test } from "node:test";
import assert from "node:assert/strict";
import { createFS, constants as C } from "./memfs.mjs";

// call runs an fs method and returns its result, or throws its error.
function call(fs, name, ...args) {
  let out;
  fs[name](...args, (err, res) => { if (err) throw err; out = res; });
  return out;
}
const enc = (s) => new TextEncoder().encode(s);
const dec = (b) => new TextDecoder().decode(b);

function writeFile(fs, path, s, flags = C.O_WRONLY | C.O_CREAT | C.O_TRUNC) {
  const fd = call(fs, "open", path, flags, 0o644);
  const b = enc(s);
  call(fs, "write", fd, b, 0, b.length, null);
  call(fs, "close", fd);
}
function readFile(fs, path) {
  const fd = call(fs, "open", path, C.O_RDONLY, 0);
  const size = call(fs, "fstat", fd).size;
  const buf = new Uint8Array(size);
  call(fs, "read", fd, buf, 0, size, null);
  call(fs, "close", fd);
  return dec(buf);
}

test("write then read", () => {
  const fs = createFS();
  fs.mkdirAll("/a/b");
  writeFile(fs, "/a/b/f.txt", "hello");
  assert.equal(readFile(fs, "/a/b/f.txt"), "hello");
});

test("append and truncate", () => {
  const fs = createFS();
  writeFile(fs, "/f", "ab");
  writeFile(fs, "/f", "cd", C.O_WRONLY | C.O_APPEND);
  assert.equal(readFile(fs, "/f"), "abcd");
  writeFile(fs, "/f", "x");
  assert.equal(readFile(fs, "/f"), "x");
});

test("positional write past the end zero-fills", () => {
  const fs = createFS();
  writeFile(fs, "/f", "abcdef");
  const fd = call(fs, "open", "/f", C.O_RDWR, 0);
  call(fs, "ftruncate", fd, 1);
  call(fs, "write", fd, enc("z"), 0, 1, 3);
  call(fs, "close", fd);
  assert.deepEqual([...enc(readFile(fs, "/f"))], [0x61, 0, 0, 0x7a]);
});

test("errors carry node codes", () => {
  const fs = createFS();
  const code = (fn) => { try { fn(); } catch (e) { return e.code; } return "none"; };
  assert.equal(code(() => call(fs, "open", "/missing/f", C.O_WRONLY | C.O_CREAT, 0o644)), "ENOENT");
  writeFile(fs, "/f", "x");
  assert.equal(code(() => call(fs, "open", "/f", C.O_WRONLY | C.O_CREAT | C.O_EXCL, 0o644)), "EEXIST");
  assert.equal(code(() => call(fs, "mkdir", "/f", 0o755)), "EEXIST");
  assert.equal(code(() => call(fs, "close", 99)), "EBADF");
  fs.mkdirAll("/d/e");
  assert.equal(code(() => call(fs, "rmdir", "/d")), "ENOTEMPTY");
  assert.equal(code(() => call(fs, "unlink", "/d")), "EISDIR");
});

test("rename replaces an existing file", () => {
  const fs = createFS();
  writeFile(fs, "/config.toml", "old");
  writeFile(fs, "/config.toml.tmp", "new");
  call(fs, "rename", "/config.toml.tmp", "/config.toml");
  assert.equal(readFile(fs, "/config.toml"), "new");
  assert.deepEqual(call(fs, "readdir", "/"), ["config.toml"]);
});

test("stat says what a node is", () => {
  const fs = createFS();
  fs.mkdirAll("/d");
  writeFile(fs, "/d/f", "abc");
  const d = call(fs, "stat", "/d"), f = call(fs, "stat", "/d/f");
  assert.ok(d.isDirectory());
  assert.ok(!f.isDirectory());
  assert.equal(f.size, 3);
  assert.equal(d.mode & 0o170000, 0o040000);
  assert.equal(f.mode & 0o170000, 0o100000);
});

test("readdir is sorted; paths normalize", () => {
  const fs = createFS();
  fs.mkdirAll("/d");
  writeFile(fs, "/d/b", "");
  writeFile(fs, "/d/./a", "");
  writeFile(fs, "/d/x/../c", "x".slice(1)); // /d/x doesn't exist: still resolves to /d/c
  assert.deepEqual(call(fs, "readdir", "/d"), ["a", "b", "c"]);
});

test("stdout and stderr go to the callbacks", () => {
  const seen = [];
  const fs = createFS({ stdout: (b) => seen.push(["out", dec(b)]), stderr: (b) => seen.push(["err", dec(b)]) });
  fs.writeSync(1, enc("hi\n"));
  call(fs, "write", 2, enc("oops"), 0, 4, null);
  assert.deepEqual(seen, [["out", "hi\n"], ["err", "oops"]]);
});

test("writeFileAll makes parents", () => {
  const fs = createFS();
  fs.writeFileAll("/home/kiln/.config/kiln/worlds/fm.toml", enc("host = 1"));
  assert.equal(readFile(fs, "/home/kiln/.config/kiln/worlds/fm.toml"), "host = 1");
});
```

Note on "paths normalize": `/d/x/../c` is resolved lexically, like Go's `filepath.Clean`, which is all Go ever sends.

- [ ] **Step 2: Run; it fails**

Run: `node --test web/static/memfs.test.mjs`
Expected: FAIL, cannot find `memfs.mjs`

- [ ] **Step 3: Implement** (`web/static/memfs.mjs`)

```js
// memfs: an in-memory filesystem with the Node-style callback API that
// Go's syscall package calls through globalThis.fs under GOOS=js (see
// $(go env GOROOT)/src/syscall/fs_js.go). It gives Kiln real files in the
// browser: config, known_hosts and logs, for the length of a visit.
//
// Errors carry Node's codes (ENOENT, ...): Go maps them to errnos, and
// panics on a code it doesn't know. Callbacks run synchronously, which
// Go's fsCall allows. Paths resolve lexically; the cwd is "/".

const S_IFDIR = 0o040000;
const S_IFREG = 0o100000;

export const constants = {
  O_RDONLY: 0, O_WRONLY: 1, O_RDWR: 2, O_CREAT: 0o100, O_EXCL: 0o200,
  O_TRUNC: 0o1000, O_APPEND: 0o2000, O_DIRECTORY: 0o200000,
};

function fail(code, call, path = "") {
  const e = new Error(`${code}: ${call} ${path}`);
  e.code = code;
  return e;
}

function lineLogger(log) {
  let pending = "";
  const dec = new TextDecoder();
  return (bytes) => {
    pending += dec.decode(bytes, { stream: true });
    const nl = pending.lastIndexOf("\n");
    if (nl >= 0) {
      log(pending.slice(0, nl));
      pending = pending.slice(nl + 1);
    }
  };
}

export function createFS({ stdout = lineLogger(console.log), stderr = lineLogger(console.error) } = {}) {
  let nextIno = 1;
  let nextFd = 3; // 0-2 are stdio
  const now = () => Date.now();
  const dir = (perm) => ({ kind: "dir", mode: S_IFDIR | (perm & 0o7777), ino: nextIno++, entries: new Map(), mtimeMs: now() });
  const file = (perm) => ({ kind: "file", mode: S_IFREG | (perm & 0o7777), ino: nextIno++, data: new Uint8Array(0), size: 0, mtimeMs: now() });
  const root = dir(0o755);
  const fds = new Map();

  function parts(path) {
    const out = [];
    for (const p of String(path).split("/")) {
      if (!p || p === ".") continue;
      if (p === "..") out.pop(); else out.push(p);
    }
    return out;
  }
  function walk(ps, call, path) {
    let n = root;
    for (const p of ps) {
      if (n.kind !== "dir") throw fail("ENOTDIR", call, path);
      n = n.entries.get(p);
      if (!n) throw fail("ENOENT", call, path);
    }
    return n;
  }
  const lookup = (path, call) => walk(parts(path), call, path);
  function parentOf(path, call) {
    const ps = parts(path);
    if (ps.length === 0) throw fail("EEXIST", call, path);
    const d = walk(ps.slice(0, -1), call, path);
    if (d.kind !== "dir") throw fail("ENOTDIR", call, path);
    return [d, ps[ps.length - 1]];
  }
  function open(fd, call) {
    const f = fds.get(fd);
    if (!f) throw fail("EBADF", call);
    return f;
  }
  function ensure(n, len) {
    if (len <= n.data.length) return;
    const d = new Uint8Array(Math.max(len, n.data.length * 2));
    d.set(n.data.subarray(0, n.size));
    n.data = d;
  }
  function resize(n, len, call) {
    if (n.kind === "dir") throw fail("EISDIR", call);
    ensure(n, len);
    if (len > n.size) n.data.fill(0, n.size, len);
    n.size = len;
    n.mtimeMs = now();
  }
  function stat(n) {
    const size = n.kind === "dir" ? 0 : n.size;
    const isDir = n.kind === "dir";
    return {
      dev: 1, ino: n.ino, mode: n.mode, nlink: 1, uid: 0, gid: 0, rdev: 0, size,
      blksize: 4096, blocks: Math.ceil(size / 512),
      atimeMs: n.mtimeMs, mtimeMs: n.mtimeMs, ctimeMs: n.mtimeMs,
      isDirectory: () => isDir,
    };
  }
  // run calls fn and hands its result or error to a Node-style callback.
  // Errors without a code are bugs, not filesystem errors: rethrow them.
  function run(callback, fn) {
    let res, err = null;
    try { res = fn(); } catch (e) { if (!e.code) throw e; err = e; }
    callback(err, res);
  }
  function stdio(fd, bytes) {
    (fd === 1 ? stdout : stderr)(bytes);
    return bytes.length;
  }

  const fs = {
    constants,

    writeSync(fd, buf) {
      if (fd === 1 || fd === 2) return stdio(fd, buf);
      let n;
      fs.write(fd, buf, 0, buf.length, null, (err, res) => { if (err) throw err; n = res; });
      return n;
    },

    open(path, flags, perm, cb) {
      run(cb, () => {
        let n;
        try {
          n = lookup(path, "open");
          if ((flags & constants.O_CREAT) && (flags & constants.O_EXCL)) throw fail("EEXIST", "open", path);
        } catch (e) {
          if (e.code !== "ENOENT" || !(flags & constants.O_CREAT)) throw e;
          const [d, name] = parentOf(path, "open");
          n = file(perm);
          d.entries.set(name, n);
          d.mtimeMs = now();
        }
        if (n.kind === "dir" && (flags & 3) !== constants.O_RDONLY) throw fail("EISDIR", "open", path);
        if ((flags & constants.O_DIRECTORY) && n.kind !== "dir") throw fail("ENOTDIR", "open", path);
        if ((flags & constants.O_TRUNC) && n.kind === "file") resize(n, 0, "open");
        const fd = nextFd++;
        fds.set(fd, { node: n, pos: 0, append: !!(flags & constants.O_APPEND) });
        return fd;
      });
    },
    close(fd, cb) { run(cb, () => { if (!fds.delete(fd)) throw fail("EBADF", "close"); }); },
    fsync(fd, cb) { run(cb, () => { open(fd, "fsync"); }); },

    read(fd, buf, offset, length, position, cb) {
      run(cb, () => {
        const f = open(fd, "read");
        if (f.node.kind === "dir") throw fail("EISDIR", "read");
        const pos = position ?? f.pos;
        const k = Math.max(0, Math.min(length, f.node.size - pos));
        buf.set(f.node.data.subarray(pos, pos + k), offset);
        if (position == null) f.pos += k;
        return k;
      });
    },
    write(fd, buf, offset, length, position, cb) {
      run(cb, () => {
        if (fd === 1 || fd === 2) return stdio(fd, buf.subarray(offset, offset + length));
        const f = open(fd, "write");
        const n = f.node;
        const pos = position ?? (f.append ? n.size : f.pos);
        if (pos + length > n.size) resize(n, pos + length, "write");
        n.data.set(buf.subarray(offset, offset + length), pos);
        n.mtimeMs = now();
        if (position == null) f.pos = pos + length;
        return length;
      });
    },
    ftruncate(fd, len, cb) { run(cb, () => resize(open(fd, "ftruncate").node, len, "ftruncate")); },
    truncate(path, len, cb) { run(cb, () => resize(lookup(path, "truncate"), len, "truncate")); },

    fstat(fd, cb) { run(cb, () => stat(open(fd, "fstat").node)); },
    stat(path, cb) { run(cb, () => stat(lookup(path, "stat"))); },
    lstat(path, cb) { run(cb, () => stat(lookup(path, "lstat"))); },

    mkdir(path, perm, cb) {
      run(cb, () => {
        const [d, name] = parentOf(path, "mkdir");
        if (d.entries.has(name)) throw fail("EEXIST", "mkdir", path);
        d.entries.set(name, dir(perm));
        d.mtimeMs = now();
      });
    },
    readdir(path, cb) {
      run(cb, () => {
        const n = lookup(path, "readdir");
        if (n.kind !== "dir") throw fail("ENOTDIR", "readdir", path);
        return [...n.entries.keys()].sort();
      });
    },
    rename(from, to, cb) {
      run(cb, () => {
        const [fd, fname] = parentOf(from, "rename");
        const n = fd.entries.get(fname);
        if (!n) throw fail("ENOENT", "rename", from);
        const [td, tname] = parentOf(to, "rename");
        const old = td.entries.get(tname);
        if (old && old !== n) {
          if (old.kind === "dir" && n.kind !== "dir") throw fail("EISDIR", "rename", to);
          if (old.kind !== "dir" && n.kind === "dir") throw fail("ENOTDIR", "rename", to);
          if (old.kind === "dir" && old.entries.size) throw fail("ENOTEMPTY", "rename", to);
        }
        fd.entries.delete(fname);
        td.entries.set(tname, n);
        fd.mtimeMs = td.mtimeMs = now();
      });
    },
    unlink(path, cb) {
      run(cb, () => {
        const [d, name] = parentOf(path, "unlink");
        const n = d.entries.get(name);
        if (!n) throw fail("ENOENT", "unlink", path);
        if (n.kind === "dir") throw fail("EISDIR", "unlink", path);
        d.entries.delete(name);
      });
    },
    rmdir(path, cb) {
      run(cb, () => {
        const [d, name] = parentOf(path, "rmdir");
        const n = d.entries.get(name);
        if (!n) throw fail("ENOENT", "rmdir", path);
        if (n.kind !== "dir") throw fail("ENOTDIR", "rmdir", path);
        if (n.entries.size) throw fail("ENOTEMPTY", "rmdir", path);
        d.entries.delete(name);
      });
    },

    chmod(path, mode, cb) { run(cb, () => { const n = lookup(path, "chmod"); n.mode = (n.mode & ~0o7777) | (mode & 0o7777); }); },
    fchmod(fd, mode, cb) { run(cb, () => { const n = open(fd, "fchmod").node; n.mode = (n.mode & ~0o7777) | (mode & 0o7777); }); },
    utimes(path, atime, mtime, cb) { run(cb, () => { lookup(path, "utimes").mtimeMs = mtime * 1000; }); },
    chown(path, uid, gid, cb) { run(cb, () => { lookup(path, "chown"); }); },
    fchown(fd, uid, gid, cb) { run(cb, () => { open(fd, "fchown"); }); },
    lchown(path, uid, gid, cb) { run(cb, () => { lookup(path, "lchown"); }); },
    link(from, to, cb) { run(cb, () => { throw fail("ENOSYS", "link"); }); },
    symlink(from, to, cb) { run(cb, () => { throw fail("ENOSYS", "symlink"); }); },
    readlink(path, cb) { run(cb, () => { throw fail("ENOSYS", "readlink"); }); },

    // mkdirAll and writeFileAll are for the page and test runner, not Go.
    mkdirAll(path) {
      let n = root;
      for (const p of parts(path)) {
        if (!n.entries.has(p)) n.entries.set(p, dir(0o755));
        n = n.entries.get(p);
        if (n.kind !== "dir") throw fail("ENOTDIR", "mkdirAll", path);
      }
    },
    writeFileAll(path, bytes) {
      fs.mkdirAll(parts(path).slice(0, -1).join("/"));
      const [d, name] = parentOf(path, "writeFileAll");
      const n = file(0o644);
      ensure(n, bytes.length);
      n.data.set(bytes);
      n.size = bytes.length;
      d.entries.set(name, n);
    },
  };
  return fs;
}
```

- [ ] **Step 4: Run the node tests**

Run: `node --test web/static/memfs.test.mjs`
Expected: PASS (9 tests)

- [ ] **Step 5: Add the wasm test runner**

`web/testdata/memfs_exec.mjs`:

```js
// Runs a GOOS=js test binary like Go's go_js_wasm_exec, but on memfs, so
// tests see the filesystem the browser page gives Kiln.
// Use (in web/): GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...
import { createRequire } from "node:module";
import { readFileSync } from "node:fs";
import { createFS } from "../static/memfs.mjs";

const require = createRequire(import.meta.url);
const [, , wasmPath, ...args] = process.argv;
const wasm = readFileSync(wasmPath); // with the real fs, before the swap

const fs = createFS({ stdout: (b) => process.stdout.write(b), stderr: (b) => process.stderr.write(b) });
fs.mkdirAll(process.cwd()); // go test runs the binary in the package dir
fs.mkdirAll("/tmp");
fs.mkdirAll("/home/kiln");
globalThis.fs = fs;
globalThis.path = require("node:path");
require(`${process.env.GOROOT}/lib/wasm/wasm_exec.js`);

const go = new Go();
go.argv = [wasmPath, ...args];
go.env = { ...process.env, HOME: "/home/kiln", TMPDIR: "/tmp" };
go.exit = process.exit;
const { instance } = await WebAssembly.instantiate(wasm, go.importObject);
process.on("exit", (code) => { // as in wasm_exec_node.js: report a deadlock
  if (code === 0 && !go.exited) {
    go._pendingEvent = { id: 0 };
    go._resume();
  }
});
await go.run(instance);
```

`web/testdata/memfs_exec.sh` (then `chmod +x`):

```sh
#!/bin/sh
# go test -exec wrapper: runs a js/wasm test binary on memfs under node.
GOROOT="$(go env GOROOT)" exec node --stack-size=8192 "$(dirname "$0")/memfs_exec.mjs" "$@"
```

- [ ] **Step 6: Write the Go-side memfs tests** (`web/fstest/fs_test.go`)

```go
//go:build js

// Package fstest checks that Kiln's file-using packages work on memfs,
// the browser page's filesystem. Run with web/testdata/memfs_exec.sh.
package fstest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
)

func TestConfigOnMemfs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kiln")
	if err := config.EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	world := "host = \"example.org\"\nport = 8899\ntls = true\n\n[[characters]]\nid = \"kit\"\nname = \"Kit\"\n"
	if err := os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte(world), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Find("fm", "kit"); !ok {
		t.Errorf("fm/kit not loaded: %+v", cfg.Worlds)
	}
}

func TestKnownHostsOnMemfs(t *testing.T) {
	kh := conn.KnownHosts{Path: filepath.Join(t.TempDir(), "known_hosts")}
	if err := kh.Trust("example.org:8899", "sha256:abcd"); err != nil {
		t.Fatal(err)
	}
	if fp, ok, err := kh.Lookup("example.org:8899"); err != nil || !ok || fp != "sha256:abcd" {
		t.Errorf("Lookup = %q, %v, %v", fp, ok, err)
	}
}

func TestRenameReplacesOnMemfs(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "config.toml"), filepath.Join(dir, "config.toml.tmp")
	os.WriteFile(a, []byte("old"), 0o644)
	os.WriteFile(b, []byte("new"), 0o644)
	if err := os.Rename(b, a); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(a); string(got) != "new" {
		t.Errorf("got %q", got)
	}
}
```

If `cfg.Find`'s signature differs (check `internal/config/config.go:103`), adapt the call. The point is that the world loaded.

- [ ] **Step 7: Run all `web/` tests under wasm on memfs**

```bash
cd web && GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...
```

Expected: PASS for `fstest` and `bridge`. If Go panics with a JS error object, memfs threw a code Go doesn't know or is missing a call. Read the stack, then add or fix that call (and a node test for it).

- [ ] **Step 8: Commit**

```bash
git add web
git commit -m "feat(web): memfs, an in-memory filesystem for Go in the browser, with a wasm test runner"
```

---

### Task 7: `wsdial`, Kiln's side of the relay

**Files:**
- Create: `web/wsdial/wsdial.go`, `web/wsdial/wsdial_test.go`, `web/wsdial/catalog_test.go`
- Modify: `internal/str/locales/en.toml` (`[conn]`), `web/go.mod`/`go.sum`

**Interfaces:**
- Consumes: `relay.Server`, `relay.ParseAllowlist`, `relay.Code*` (Tasks 2–3, tests only); `conn.Options.DialContext` (Task 1).
- Produces: `func Dialer(relayURL string) func(ctx context.Context, network, addr string) (net.Conn, error)`. Errors from refused connections read as catalog text.

- [ ] **Step 1: Strings** (append to `[conn]`)

```toml
relay_not_listed = "this world isn't on the web relay's list"
relay_needs_tls = "this world only works over TLS on the web"
relay_too_many = "too many connections from your address; close one and try again"
relay_unreachable = "the relay couldn't reach this world: {reason}"
```

Run `go generate ./internal/str`.

- [ ] **Step 2: Write the failing tests**

`web/wsdial/catalog_test.go`:

```go
package wsdial

import "strings"

// Tests build expected text from the catalog; mark fills a placeholder.
const mark = "\x00"

// upTo is s up to the first mark.
func upTo(s string) string { h, _, _ := strings.Cut(s, mark); return h }
```

`web/wsdial/wsdial_test.go`:

```go
package wsdial

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/relay"
	"github.com/latrani/Kiln/internal/str"
)

func TestCodesMatchRelay(t *testing.T) {
	if codeNotListed != relay.CodeNotListed || codeNeedsTLS != relay.CodeNeedsTLS ||
		codeTooMany != relay.CodeTooMany || codeUnreachable != relay.CodeUnreachable {
		t.Error("wsdial's close codes drifted from internal/relay's")
	}
}

// relayURL starts a relay allowing 127.0.0.1:port and returns its ws:// URL.
func relayURL(t *testing.T, port int, plaintext bool) string {
	t.Helper()
	a, err := relay.ParseAllowlist("allow.toml", []byte(fmt.Sprintf(
		"[[world]]\nhost = \"127.0.0.1\"\nports = [%d]\nplaintext = %v\n", port, plaintext)))
	if err != nil {
		t.Fatal(err)
	}
	s := &relay.Server{FirstFrameTimeout: 300 * time.Millisecond}
	s.SetAllowlist(a)
	hs := httptest.NewServer(s)
	t.Cleanup(hs.Close)
	return "ws" + strings.TrimPrefix(hs.URL, "http") + "/kiln/relay"
}

func serve(t *testing.T, ln net.Listener, greeting string) int {
	t.Helper()
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); c.Write([]byte(greeting)); c.Read(make([]byte, 1)) }()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func plainListener(t *testing.T) net.Listener {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func firstLine(t *testing.T, c *conn.Conn) (string, error) {
	t.Helper()
	select {
	case l, ok := <-c.Lines():
		if !ok {
			return "", c.Err()
		}
		return l, nil
	case <-time.After(5 * time.Second):
		t.Fatal("nothing arrived")
		return "", nil
	}
}

func TestTLSWorldThroughRelay(t *testing.T) {
	ln, cert := selfSigned(t)
	port := serve(t, ln, "secure hello\r\n")
	kh := conn.KnownHosts{Path: filepath.Join(t.TempDir(), "known_hosts")}
	c, err := conn.Dial(context.Background(), conn.Options{Host: "127.0.0.1", Port: port, TLS: true, TLSTrust: "pin",
		KnownHosts: kh, DialContext: Dialer(relayURL(t, port, false))})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if l, err := firstLine(t, c); l != "secure hello" {
		t.Errorf("line %q, err %v", l, err)
	}
	if fp, ok, _ := kh.Lookup(fmt.Sprintf("127.0.0.1:%d", port)); !ok || fp != conn.Fingerprint(cert) {
		t.Errorf("pin %q", fp)
	}
}

func TestNotListedReadsAsCatalogText(t *testing.T) {
	port := serve(t, plainListener(t), "hello\r\n")
	url := relayURL(t, port, true)
	_, err := conn.Dial(context.Background(), conn.Options{Host: "127.0.0.1", Port: port + 1, TLS: true,
		TLSTrust: "ca", DialContext: Dialer(url)})
	if err == nil || !strings.Contains(err.Error(), str.ConnRelayNotListed()) {
		t.Errorf("err = %v", err)
	}
}

func TestPlainClientOnTLSWorldReadsAsCatalogText(t *testing.T) {
	port := serve(t, plainListener(t), "hello\r\n")
	c, err := conn.Dial(context.Background(), conn.Options{Host: "127.0.0.1", Port: port,
		DialContext: Dialer(relayURL(t, port, false))})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := firstLine(t, c); err == nil || !strings.Contains(err.Error(), str.ConnRelayNeedsTLS()) {
		t.Errorf("err = %v", err)
	}
}

func TestUnreachableReadsAsCatalogText(t *testing.T) {
	ln := plainListener(t)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	c, err := conn.Dial(context.Background(), conn.Options{Host: "127.0.0.1", Port: port,
		DialContext: Dialer(relayURL(t, port, true))})
	if err == nil {
		defer c.Close()
		_, err = firstLine(t, c)
	}
	if err == nil || !strings.Contains(err.Error(), upTo(str.ConnRelayUnreachable(mark))) {
		t.Errorf("err = %v", err)
	}
}

func selfSigned(t *testing.T) (net.Listener, *x509.Certificate) {
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
```

`TestPlainClientOnTLSWorldReadsAsCatalogText` relies on `conn.Conn.Err()` returning the read error after `Lines()` closes. Check `internal/conn/conn.go:181`. If it returns nil on a closed read, assert on whatever path the session uses to show the disconnect reason instead.

- [ ] **Step 3: Run; they fail to compile**

Run: `cd web && go test ./wsdial/`
Expected: FAIL, `undefined: Dialer`

- [ ] **Step 4: Implement** (`web/wsdial/wsdial.go`)

```go
// Package wsdial dials MUCKs through kiln-relay: a WebSocket to the relay,
// used as a plain net.Conn, so TLS and telnet still happen in Kiln and the
// relay only passes bytes. It works natively too, which is how it's tested.
package wsdial

import (
	"context"
	"errors"
	"net"
	"net/url"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/str"
)

// The close codes kiln-relay refuses with. They match internal/relay's
// Code* constants (TestCodesMatchRelay); they're copied so the browser
// build doesn't link the relay's server code.
const (
	codeNotListed   websocket.StatusCode = 4403
	codeNeedsTLS    websocket.StatusCode = 4426
	codeTooMany     websocket.StatusCode = 4429
	codeUnreachable websocket.StatusCode = 4502
)

// Dialer returns a conn.Options.DialContext that reaches addr through the
// relay at relayURL (ws:// or wss://).
func Dialer(relayURL string) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, _, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		u, err := url.Parse(relayURL)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("host", host)
		q.Set("port", port)
		u.RawQuery = q.Encode()
		c, _, err := websocket.Dial(ctx, u.String(), nil)
		if err != nil {
			return nil, err
		}
		c.SetReadLimit(1 << 20)
		// The connection outlives ctx, which only bounds the dial.
		return relayConn{websocket.NetConn(context.Background(), c, websocket.MessageBinary)}, nil
	}
}

// relayConn turns the relay's refusals into errors people can read.
type relayConn struct{ net.Conn }

func (c relayConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	return n, explain(err)
}

func (c relayConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	return n, explain(err)
}

func explain(err error) error {
	var ce websocket.CloseError
	if err == nil || !errors.As(err, &ce) {
		return err
	}
	switch ce.Code {
	case codeNotListed:
		return str.Wrap(str.ConnRelayNotListed(), err)
	case codeNeedsTLS:
		return str.Wrap(str.ConnRelayNeedsTLS(), err)
	case codeTooMany:
		return str.Wrap(str.ConnRelayTooMany(), err)
	case codeUnreachable:
		return str.Wrap(str.ConnRelayUnreachable(ce.Reason), err)
	}
	return err
}
```

- [ ] **Step 5: Run natively, then on wasm**

```bash
cd web && go mod tidy && go test ./wsdial/ -race -v
GOOS=js GOARCH=wasm go build ./wsdial/
```

Expected: PASS; the js build succeeds. (The wasm *tests* for wsdial would need a relay inside node; native coverage is enough, since `coder/websocket` switches to the browser's WebSocket under js. The dev loop in Task 8 covers the browser.)

If the TLS-world test fails because crypto/tls hides the close error from `explain` (the message shows `remote error`/`EOF` instead), make `explain` also run on the error `conn.Dial` returns: wrap in Kiln's `Dial` closure in `cmd/kiln-web` instead. Note what you saw in the commit.

- [ ] **Step 6: Commit**

```bash
git add web internal/str
git commit -m "feat(web): wsdial, dialing MUCKs through kiln-relay with readable refusals

Strings: conn.relay_not_listed, conn.relay_needs_tls, conn.relay_too_many, conn.relay_unreachable (new)"
```

---

### Task 8: `kiln-web`, the page, and the local dev loop

**Files:**
- Create: `web/cmd/kiln-web/main.go`, `web/static/index.html`, `web/static/kiln.js`, `web/static/vendor/*` (vendored xterm), `web/vendor.sh`, `web/build.sh`, `web/preset.example/worlds/fm.toml`, `web/dev-allow.example.toml`, `docs/web-dev.md`

**Interfaces:**
- Consumes: `bridge.New/Options/Attach/Input/Resize/Close` (Task 5), memfs `createFS/mkdirAll/writeFileAll` (Task 6), `wsdial.Dialer` (Task 7), `ui.New/ui.Deps/ui.SidebarWidth`, `config.Dir/DataDir/EnsureDefaults/Load`, `logstore.NewWriter`, `secrets.ErrNotFound`.
- Produces: global JS `kiln` with `start(cols, rows, relayURL, write)`, then `input(s)`, `resize(cols, rows)`; Go calls `globalThis.kilnReady()` once `kiln.start` exists.

- [ ] **Step 1: Vendor xterm.js** (`web/vendor.sh`, then run it and commit the output)

```sh
#!/bin/sh
# Copies pinned xterm.js builds into web/static/vendor. Rerun after
# changing a version; commit the result.
set -eu
cd "$(dirname "$0")"
out=static/vendor
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$out"
for pkg in @xterm/xterm@6.0.0 @xterm/addon-fit@0.11.0 @xterm/addon-webgl@0.19.0 @xterm/addon-clipboard@0.2.0; do
  (cd "$tmp" && npm pack -q "$pkg" >/dev/null)
done
for f in "$tmp"/*.tgz; do
  name=$(basename "$f" .tgz)
  mkdir -p "$tmp/$name"
  tar xzf "$f" -C "$tmp/$name"
  cp "$tmp/$name"/package/lib/*.mjs "$out/"
  cp "$tmp/$name"/package/LICENSE "$out/LICENSE-$name"
done
cp "$tmp"/xterm-xterm-*/package/css/xterm.css "$out/"
```

```bash
chmod +x web/vendor.sh && web/vendor.sh && ls web/static/vendor
```

Expected: `xterm.mjs addon-fit.mjs addon-webgl.mjs addon-clipboard.mjs xterm.css` plus LICENSE files.

- [ ] **Step 2: Build script** (`web/build.sh`, `chmod +x`)

```sh
#!/bin/sh
# Builds the page into web/static: kiln.wasm and Go's matching wasm_exec.js.
set -eu
cd "$(dirname "$0")"
GOOS=js GOARCH=wasm go build -trimpath -o static/kiln.wasm ./cmd/kiln-web
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" static/wasm_exec.js
```

- [ ] **Step 3: `web/cmd/kiln-web/main.go`**

```go
//go:build js

// Command kiln-web is Kiln in a browser tab: built for GOOS=js, drawing
// into xterm.js through package bridge, keeping files in the page's memfs,
// and reaching MUCKs through kiln-relay. web/static/kiln.js starts it; see
// docs/web-dev.md.
package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"syscall/js"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/secrets"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/ui"
	"github.com/latrani/Kiln/web/bridge"
	"github.com/latrani/Kiln/web/wsdial"
)

type page struct {
	b          *bridge.Bridge
	cols, rows int
	relay      string
}

func main() {
	started := make(chan page, 1)
	k := js.Global().Get("Object").New() //str:ok
	// start is a JS callback, so it must not block: it sets everything up
	// and hands off to main.
	k.Set("start", js.FuncOf(func(_ js.Value, a []js.Value) any { //str:ok
		write := a[3]
		b := bridge.New(func(p []byte) {
			u := js.Global().Get("Uint8Array").New(len(p)) //str:ok
			js.CopyBytesToJS(u, p)
			write.Invoke(u)
		})
		k.Set("input", js.FuncOf(func(_ js.Value, v []js.Value) any { b.Input(v[0].String()); return nil }))         //str:ok
		k.Set("resize", js.FuncOf(func(_ js.Value, v []js.Value) any { b.Resize(v[0].Int(), v[1].Int()); return nil })) //str:ok
		started <- page{b: b, cols: a[0].Int(), rows: a[1].Int(), relay: a[2].String()}
		return nil
	}))
	js.Global().Set("kiln", k)        //str:ok
	js.Global().Call("kilnReady")     //str:ok
	p := <-started
	if err := run(p); err != nil {
		p.b.Close()
		js.Global().Get("console").Call("error", str.CliError(err)) //str:ok
	}
}

func run(p page) error {
	cfgDir, err := config.Dir()
	if err != nil {
		return err
	}
	if err := config.EnsureDefaults(cfgDir); err != nil {
		return err
	}
	cfg, err := config.Load(cfgDir)
	if err != nil {
		return err
	}
	dataDir, err := config.DataDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	kh := conn.KnownHosts{Path: filepath.Join(dataDir, "known_hosts")} //str:ok
	dial := wsdial.Dialer(p.relay)
	var mu sync.Mutex
	writers := map[logstore.Layout]*logstore.Writer{}
	m := ui.New(ui.Deps{
		ConfigDir:  cfgDir,
		LogRoot:    filepath.Join(dataDir, "logs"), //str:ok
		KnownHosts: kh,
		Load:       config.Load,
		Dial: func(ctx context.Context, ch config.Character) (session.LineConn, error) {
			// Roughly the right pane's size; the UI sends the exact size.
			return conn.Dial(ctx, conn.Options{
				Host: ch.Host, Port: ch.Port, TLS: ch.TLS, TLSTrust: ch.TLSTrust,
				KnownHosts: kh, Width: p.cols - ui.SidebarWidth(p.cols) - 1, Height: p.rows,
				DialContext: dial,
			})
		},
		NewLog: func(l logstore.Layout) session.Appender {
			mu.Lock()
			defer mu.Unlock()
			if writers[l] == nil {
				writers[l] = logstore.NewWriter(l)
			}
			return writers[l]
		},
		// No saved passwords in the browser: Kiln asks. (Sub-project 4 adds
		// a login form the browser's password manager can fill.)
		Password: func(string, string, string) (string, error) { return "", secrets.ErrNotFound },
		OpenURL: func(u string) error {
			js.Global().Call("open", u, "_blank", "noopener") //str:ok
			return nil
		},
	}, cfg)
	prog := tea.NewProgram(m, p.b.Options(p.cols, p.rows)...)
	p.b.Attach(prog)
	_, err = prog.Run()
	return err
}
```

If `logstore.Layout` isn't comparable (it holds only strings today, so it is), key the map with `fmt.Sprintf("%+v", l)` like `cmd/kiln` does.

- [ ] **Step 4: `web/static/index.html`**

```html
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Kiln</title>
<link rel="stylesheet" href="vendor/xterm.css">
<style>
  html, body { margin: 0; height: 100%; background: #000; }
  #term { position: fixed; inset: 0; padding: 4px; }
</style>
</head>
<body>
<div id="term"></div>
<noscript>Kiln needs JavaScript.</noscript>
<script type="module" src="kiln.js"></script>
</body>
</html>
```

- [ ] **Step 5: `web/static/kiln.js`**

```js
// Boots Kiln in the page: memfs seeded with the preset config, Go's wasm
// runtime, and xterm.js as Kiln's terminal. memfs must be in place before
// wasm_exec.js loads: wasm_exec only stubs globalThis.fs if it's missing.
import { Terminal } from "./vendor/xterm.mjs";
import { FitAddon } from "./vendor/addon-fit.mjs";
import { WebglAddon } from "./vendor/addon-webgl.mjs";
import { ClipboardAddon } from "./vendor/addon-clipboard.mjs";
import { createFS } from "./memfs.mjs";

const HOME = "/home/kiln";
const CONFIG = `${HOME}/.config/kiln`;

function loadScript(src) {
  return new Promise((ok, fail) => {
    const s = document.createElement("script");
    s.src = src;
    s.onload = ok;
    s.onerror = () => fail(new Error(`loading ${src}`));
    document.head.append(s);
  });
}

async function seedPreset(fs) {
  const files = await (await fetch("preset/manifest.json")).json();
  for (const f of files) {
    const res = await fetch(`preset/${f}`);
    if (!res.ok) throw new Error(`preset/${f}: ${res.status}`);
    fs.writeFileAll(`${CONFIG}/${f}`, new Uint8Array(await res.arrayBuffer()));
  }
}

const term = new Terminal({ fontFamily: "ui-monospace, Menlo, Consolas, monospace", fontSize: 14 });
const fit = new FitAddon();
term.loadAddon(fit);
term.loadAddon(new ClipboardAddon()); // OSC 52: Kiln's copy
term.open(document.getElementById("term"));
try { term.loadAddon(new WebglAddon()); } catch { /* the DOM renderer is fine */ }
fit.fit();
addEventListener("resize", () => fit.fit());

const fs = createFS();
fs.mkdirAll("/tmp");
fs.mkdirAll(HOME);
globalThis.fs = fs;
const enosys = () => Object.assign(new Error("ENOSYS"), { code: "ENOSYS" });
globalThis.process = {
  getuid: () => -1, getgid: () => -1, geteuid: () => -1, getegid: () => -1,
  getgroups() { throw enosys(); }, pid: -1, ppid: -1,
  umask: () => 0o022, cwd: () => "/", chdir() { throw enosys(); },
};
await seedPreset(fs);
await loadScript("wasm_exec.js");

const relay = new URL("relay", location.href);
relay.protocol = location.protocol === "https:" ? "wss:" : "ws:";

globalThis.kilnReady = () => {
  kiln.start(term.cols, term.rows, relay.href, (bytes) => term.write(bytes));
  term.onData((d) => kiln.input(d));
  term.onBinary((d) => kiln.input(d));
  term.onResize(({ cols, rows }) => kiln.resize(cols, rows));
  term.focus();
};

const go = new Go();
go.env = { HOME, TMPDIR: "/tmp" };
const wasm = await (await fetch("kiln.wasm")).arrayBuffer();
const { instance } = await WebAssembly.instantiate(wasm, go.importObject);
await go.run(instance);
```

- [ ] **Step 6: Dev preset and dev allowlist**

`web/preset.example/worlds/fm.toml`:

```toml
# An example world for the dev page. Copy preset.example to preset.local
# (gitignored) and point it at a real MUCK to try one.
host = "muck.example.org"
port = 8899
tls = true

[[characters]]
id = "kit"
name = "Kit"
```

`web/dev-allow.example.toml`:

```toml
# Copy to dev-allow.toml (gitignored) and list the worlds your
# preset.local uses. See docs/web-relay.md.
[[world]]
host = "muck.example.org"
ports = [8899]
```

- [ ] **Step 7: `docs/web-dev.md`**

````markdown
# Trying Kiln's web build locally

Needs Go and node. Everything runs on your machine; the relay connects
to real MUCKs over the internet.

```sh
web/build.sh
cp -R web/preset.example web/preset.local      # edit worlds/*.toml
cp web/dev-allow.example.toml web/dev-allow.toml  # list the same worlds
go run ./cmd/kiln-relay -dev -allow web/dev-allow.toml \
  -web web/static -preset web/preset.local
```

Open http://localhost:8080/kiln/. Rebuild with `web/build.sh` and reload
after changing Go code; the relay reloads `dev-allow.toml` by itself.

Tests:

```sh
go test ./...                                   # root: relay, conn, …
(cd web && go test ./...)                       # bridge, wsdial (native)
(cd web && GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...)
node --test web/static/memfs.test.mjs
```

Files live in memory: config edits, pins and logs last until the tab
closes.
````

- [ ] **Step 8: Build and run the str lint**

```bash
web/build.sh && go test ./internal/str/
```

Expected: the build succeeds and the lint passes. Fix flagged literals per Global Constraints.

- [ ] **Step 9: Smoke-test the dev loop by hand**

Use a scratch preset (never Indi's real config): `cp -R web/preset.example web/preset.local`, then set `web/preset.local/worlds/fm.toml` and `web/dev-allow.toml` to a MUCK you can reach. **Ask Indi for PFMuck's host, port and TLS setting.** Run the command from `docs/web-dev.md` and open the page. Check:
- Kiln draws: sidebar, input, statusline.
- Opening fm/kit connects (TLS through the relay), lines arrive, a typed line sends, and Kiln asks for the password.
- Resizing the window reflows Kiln.
- Clicking a link opens a tab; copying works (OSC 52).
- The relay's terminal shows `open …` and, on disconnect, `closed …`.
- Kiln picks light or dark as it does in a terminal (OSC 11). If xterm.js doesn't answer, Kiln's fallback should hold, as it does under mosh.
- A world not in `dev-allow.toml` shows "this world isn't on the web relay's list" in Kiln.

Record anything broken as a note in the commit or a follow-up issue. Don't fix shim-level surprises here; report them.

- [ ] **Step 10: Commit**

```bash
git add web docs/web-dev.md
git commit -m "feat(web): kiln-web, the dev page, and a local dev loop"
```

---

### Task 9: CI for the web module

**Files:**
- Create: `.github/workflows/web.yml`

- [ ] **Step 1: Write the workflow**

```yaml
# Tests the relay, the web module natively and under wasm on memfs, and
# memfs itself, on PRs and pushes to main that touch them.
name: web

on:
  push:
    branches: [main]
  pull_request:
    paths: ["web/**", "internal/**", "cmd/kiln-relay/**", ".github/workflows/web.yml"]

permissions:
  contents: read

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version-file: go.mod
      - uses: actions/setup-node@v6
        with:
          node-version: lts/*
      - name: Root tests
        run: go test ./...
      - name: Web, native
        working-directory: web
        run: go test ./...
      - name: Web, wasm on memfs
        working-directory: web
        run: GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...
      - name: memfs
        run: node --test web/static/memfs.test.mjs
      - name: Page builds
        run: web/build.sh
```

Before committing, check the current major versions of `actions/checkout`, `actions/setup-go` and `actions/setup-node` against `release.yml` (it uses `@v7` for the first two) and match.

- [ ] **Step 2: Lint the YAML locally**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/web.yml'))"`
Expected: no output

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/web.yml
git commit -m "ci: test the relay and the web module, natively and under wasm"
```

---

## After the plan

- Update issue #74 with what changed: memfs replaces sub-project 3's storage interface if it held up, and the vendored Bubble Tea copy. Update #73 to "later: the web build in a Wails shell". **Ask Indi before editing issues.**
- Opening a PR follows superpowers:finishing-a-development-branch. The PR description lists the strings and why each exists.
