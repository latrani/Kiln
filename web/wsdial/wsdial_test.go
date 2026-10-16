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
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/relay"
	"github.com/latrani/Kiln/internal/str"
)

func TestCodesMatchRelay(t *testing.T) {
	if codeNotListed != relay.CodeNotListed || codeNoTLS != relay.CodeNoTLS ||
		codeTooMany != relay.CodeTooMany || codeUnreachable != relay.CodeUnreachable ||
		codeBusy != relay.CodeBusy || codeBadCert != relay.CodeBadCert || openFrame != relay.OpenFrame {
		t.Error("wsdial's close codes drifted from internal/relay's")
	}
}

// relayURL starts a relay allowing 127.0.0.1:port and returns its ws://
// URL. trust is "" for a plaintext world, "ca", or a pin.
func relayURL(t *testing.T, port int, trust string) string {
	t.Helper()
	w := fmt.Sprintf("[[world]]\nhost = \"127.0.0.1\"\nports = [%d]\n", port)
	switch trust {
	case "":
		w += "plaintext = true\n"
	case "ca":
		w += "tls_trust = \"ca\"\n"
	default:
		w += fmt.Sprintf("pin = %q\n", trust)
	}
	a, err := relay.ParseConfig("relay.toml", []byte(w))
	if err != nil {
		t.Fatal(err)
	}
	s := &relay.Server{}
	s.SetConfig(a)
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
	c, err := conn.Dial(context.Background(), conn.Options{Host: "127.0.0.1", Port: port,
		DialContext: Dialer(relayURL(t, port, conn.Fingerprint(cert)), true)})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if l, err := firstLine(t, c); l != "secure hello" {
		t.Errorf("line %q, err %v", l, err)
	}
}

func TestNotListedIsADialError(t *testing.T) {
	port := serve(t, plainListener(t), "hello\r\n")
	_, err := conn.Dial(context.Background(), conn.Options{Host: "127.0.0.1", Port: port + 1,
		DialContext: Dialer(relayURL(t, port, ""), false)})
	if err == nil || !strings.Contains(err.Error(), str.ConnRelayNotListed()) {
		t.Errorf("err = %v", err)
	}
}

func TestTLSForPlaintextWorldReadsAsCatalogText(t *testing.T) {
	port := serve(t, plainListener(t), "hello\r\n")
	_, err := conn.Dial(context.Background(), conn.Options{Host: "127.0.0.1", Port: port,
		DialContext: Dialer(relayURL(t, port, ""), true)})
	if err == nil || !strings.Contains(err.Error(), str.ConnRelayNoTls()) {
		t.Errorf("err = %v", err)
	}
}

func TestBadCertIsPermanent(t *testing.T) {
	ln, _ := selfSigned(t)
	port := serve(t, ln, "secure hello\r\n")
	_, err := conn.Dial(context.Background(), conn.Options{Host: "127.0.0.1", Port: port,
		DialContext: Dialer(relayURL(t, port, "sha256:"+strings.Repeat("0", 64)), true)})
	if err == nil || !strings.Contains(err.Error(), upTo(str.ConnRelayBadCert(mark))) {
		t.Fatalf("err = %v", err)
	}
	var ce *CertError
	if !errors.As(err, &ce) || !ce.Permanent() {
		t.Errorf("err = %#v, want a permanent *CertError", err)
	}
}

func TestUnreachableReadsAsCatalogText(t *testing.T) {
	ln := plainListener(t)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	c, err := conn.Dial(context.Background(), conn.Options{Host: "127.0.0.1", Port: port,
		DialContext: Dialer(relayURL(t, port, ""), false)})
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
		{codeBusy, str.ConnRelayBusy()},
		{codeNoTLS, str.ConnRelayNoTls()},
		{codeBadCert, str.ConnRelayBadCert("bye")},
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
