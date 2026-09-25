package conn

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
	"math/big"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/telnet"
)

// fakeServer accepts one connection and hands it to handle.
func fakeServer(t *testing.T, ln net.Listener, handle func(net.Conn)) (host string, port int) {
	t.Helper()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		handle(c)
	}()
	t.Cleanup(func() { ln.Close() })
	h, p, _ := net.SplitHostPort(ln.Addr().String())
	port, _ = strconv.Atoi(p)
	return h, port
}

func tcpListener(t *testing.T) net.Listener {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func collect(t *testing.T, c *Conn) []string {
	t.Helper()
	var got []string
	timeout := time.After(5 * time.Second)
	for {
		select {
		case l, ok := <-c.Lines():
			if !ok {
				return got
			}
			got = append(got, l)
		case <-timeout:
			t.Fatal("timed out waiting for lines")
		}
	}
}

func TestPlainConnReceivesLinesAndNegotiates(t *testing.T) {
	replies := make(chan []byte, 1)
	host, port := fakeServer(t, tcpListener(t), func(c net.Conn) {
		c.Write([]byte{telnet.IAC, telnet.DO, telnet.OptNAWS})
		c.Write([]byte("#$#mcp version: 2.1 to: 2.1\r\nWelcome to \xe9t\xe9\r\n"))
		c.Write([]byte("Rook says, \"hi\"\r\nno newline"))
		buf := make([]byte, 64)
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _ := c.Read(buf)
		replies <- buf[:n]
	})
	c, err := Dial(context.Background(), Options{Host: host, Port: port, Width: 80, Height: 24})
	if err != nil {
		t.Fatal(err)
	}
	got := collect(t, c)
	want := []string{"Welcome to été", "Rook says, \"hi\"", "no newline"}
	if len(got) != len(want) {
		t.Fatalf("got %q want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q want %q", i, got[i], want[i])
		}
	}
	if c.Err() != nil {
		t.Errorf("Err = %v, want nil on clean close", c.Err())
	}
	r := <-replies
	if len(r) < 3 || r[0] != telnet.IAC || r[1] != telnet.WILL || r[2] != telnet.OptNAWS {
		t.Errorf("server got reply %v, want IAC WILL NAWS …", r)
	}
}

func TestSendTerminatesAndEscapes(t *testing.T) {
	got := make(chan string, 1)
	host, port := fakeServer(t, tcpListener(t), func(c net.Conn) {
		line, _ := bufio.NewReader(c).ReadString('\n')
		got <- line
	})
	c, err := Dial(context.Background(), Options{Host: host, Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Send(":grins.\xff"); err != nil {
		t.Fatal(err)
	}
	if line := <-got; line != ":grins.\xff\xff\r\n" {
		t.Errorf("server got %q", line)
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

func greet(c net.Conn) { c.Write([]byte("secure hello\r\n")) }

func TestTLSPinsOnFirstUseThenAccepts(t *testing.T) {
	kh := KnownHosts{Path: filepath.Join(t.TempDir(), "known_hosts")}
	ln, cert := selfSignedListener(t)
	host, port := fakeServer(t, ln, greet)
	c, err := Dial(context.Background(), Options{Host: host, Port: port, TLS: true, TLSTrust: "pin", KnownHosts: kh})
	if err != nil {
		t.Fatalf("first connect: %v", err)
	}
	if got := collect(t, c); len(got) != 1 || got[0] != "secure hello" {
		t.Errorf("got %q", got)
	}
	pinned, ok, _ := kh.Lookup(net.JoinHostPort(host, strconv.Itoa(port)))
	if !ok || pinned != Fingerprint(cert) {
		t.Errorf("pinned %q, want %q", pinned, Fingerprint(cert))
	}
}

func TestTLSPinMismatchRefuses(t *testing.T) {
	kh := KnownHosts{Path: filepath.Join(t.TempDir(), "known_hosts")}
	ln, cert := selfSignedListener(t)
	host, port := fakeServer(t, ln, greet)
	hp := net.JoinHostPort(host, strconv.Itoa(port))
	kh.Trust(hp, "sha256:0000")
	_, err := Dial(context.Background(), Options{Host: host, Port: port, TLS: true, TLSTrust: "pin", KnownHosts: kh})
	var pin *PinMismatchError
	if !errors.As(err, &pin) {
		t.Fatalf("err = %v, want PinMismatchError", err)
	}
	if pin.Pinned != "sha256:0000" || pin.Got != Fingerprint(cert) || pin.HostPort != hp {
		t.Errorf("mismatch = %+v", pin)
	}
	if fp, _, _ := kh.Lookup(hp); fp != "sha256:0000" {
		t.Error("mismatch must not overwrite the pin")
	}
}

func TestTLSCAModeRejectsSelfSigned(t *testing.T) {
	ln, _ := selfSignedListener(t)
	host, port := fakeServer(t, ln, greet)
	_, err := Dial(context.Background(), Options{Host: host, Port: port, TLS: true, TLSTrust: "ca"})
	if err == nil {
		t.Fatal("ca mode accepted a self-signed cert")
	}
}

func TestPromptReportedAfterIdleThenLineCompletes(t *testing.T) {
	proceed := make(chan struct{})
	host, port := fakeServer(t, tcpListener(t), func(c net.Conn) {
		c.Write([]byte("Password: "))
		<-proceed
		c.Write([]byte("\r\nWelcome!\r\n"))
	})
	c, err := Dial(context.Background(), Options{Host: host, Port: port})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-c.Prompts():
		if p != "Password: " {
			t.Errorf("prompt = %q", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no prompt reported")
	}
	close(proceed)
	got := collect(t, c)
	want := []string{"Password: ", "Welcome!"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("lines = %q, want %q", got, want)
	}
}

func TestNoPromptForPromptlyCompletedLines(t *testing.T) {
	host, port := fakeServer(t, tcpListener(t), func(c net.Conn) {
		c.Write([]byte("part"))
		c.Write([]byte("ial\r\n"))
	})
	c, err := Dial(context.Background(), Options{Host: host, Port: port})
	if err != nil {
		t.Fatal(err)
	}
	collect(t, c)
	select {
	case p := <-c.Prompts():
		t.Errorf("unexpected prompt %q", p)
	case <-time.After(PromptDelay + 100*time.Millisecond):
	}
}
