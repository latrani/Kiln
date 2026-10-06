//go:build !js

package wsdial

import (
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
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

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
	if _, err := firstLine(t, c); err == nil || !strings.Contains(err.Error(), str.ConnRelayNeedsTls()) {
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

func TestExplain(t *testing.T) {
	for _, tc := range []struct {
		code websocket.StatusCode
		want string // "" leaves the error as it was
	}{
		{codeTooMany, str.ConnRelayTooMany()},
		{websocket.StatusAbnormalClosure, str.ConnRelayDropped()},
		{websocket.StatusInternalError, str.ConnRelayDropped()},
		{websocket.StatusNormalClosure, ""},
		{websocket.StatusGoingAway, ""},
	} {
		in := fmt.Errorf("read: %w", websocket.CloseError{Code: tc.code, Reason: "bye"})
		got := explain(in)
		var ce websocket.CloseError
		if !errors.As(got, &ce) || ce.Code != tc.code {
			t.Errorf("%v: explain(%v) = %v, lost the CloseError", tc.code, in, got)
		}
		if tc.want == "" {
			if got != in {
				t.Errorf("%v: explain changed it to %v", tc.code, got)
			}
		} else if got.Error() != tc.want {
			t.Errorf("%v: explain(%v) = %v", tc.code, in, got)
		}
	}
	if err := explain(io.EOF); err != io.EOF {
		t.Errorf("explain(io.EOF) = %v", err)
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
