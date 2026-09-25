// Package conn is a MUCK connection: TCP or TLS (with trust-on-first-use
// pinning), telnet negotiation, and line splitting.
package conn

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"kiln/internal/telnet"
)

// Options configures Dial.
type Options struct {
	Host       string
	Port       int
	TLS        bool
	TLSTrust   string // "pin" or "ca"
	KnownHosts KnownHosts
	Width      int // reported via NAWS
	Height     int
}

// Conn is an open connection. Received lines arrive on Lines().
type Conn struct {
	nc     net.Conn
	lines  chan string
	mu     sync.Mutex // guards writes and parser
	parser *telnet.Parser
	err    error // set before lines is closed
}

// Dial connects and starts reading. The context bounds only the dial.
func Dial(ctx context.Context, o Options) (*Conn, error) {
	hostport := net.JoinHostPort(o.Host, strconv.Itoa(o.Port))
	d := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	var nc net.Conn
	var err error
	if o.TLS {
		td := &tls.Dialer{NetDialer: d, Config: tlsConfig(o, hostport)}
		nc, err = td.DialContext(ctx, "tcp", hostport)
	} else {
		nc, err = d.DialContext(ctx, "tcp", hostport)
	}
	if err != nil {
		var pin *PinMismatchError
		if errors.As(err, &pin) {
			return nil, pin
		}
		return nil, err
	}
	c := &Conn{nc: nc, lines: make(chan string, 64), parser: telnet.NewParser(o.Width, o.Height)}
	go c.readLoop()
	return c, nil
}

func tlsConfig(o Options, hostport string) *tls.Config {
	cfg := &tls.Config{ServerName: o.Host}
	if o.TLSTrust == "ca" {
		return cfg
	}
	// Pin mode: skip CA verification (MUCK certs are usually self-signed)
	// and verify against the pinned fingerprint instead.
	cfg.InsecureSkipVerify = true
	cfg.VerifyConnection = func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return errors.New("server sent no certificate")
		}
		got := Fingerprint(cs.PeerCertificates[0])
		pinned, ok, err := o.KnownHosts.Lookup(hostport)
		if err != nil {
			return fmt.Errorf("reading known_hosts: %w", err)
		}
		if !ok {
			return o.KnownHosts.Trust(hostport, got)
		}
		if pinned != got {
			return &PinMismatchError{HostPort: hostport, Pinned: pinned, Got: got}
		}
		return nil
	}
	return cfg
}

func (c *Conn) readLoop() {
	var s splitter
	buf := make([]byte, 16*1024)
	for {
		n, err := c.nc.Read(buf)
		if n > 0 {
			c.mu.Lock()
			data, reply := c.parser.Feed(buf[:n])
			if len(reply) > 0 {
				c.nc.Write(reply)
			}
			c.mu.Unlock()
			for _, line := range s.push(data) {
				c.lines <- line
			}
		}
		if err != nil {
			for _, line := range s.flush() {
				c.lines <- line
			}
			if !errors.Is(err, io.EOF) {
				c.err = err
			}
			close(c.lines)
			return
		}
	}
}

// Lines delivers received lines. It is closed when the connection ends;
// Err then reports why (nil for a clean close by the server).
func (c *Conn) Lines() <-chan string { return c.lines }

// Err is valid after Lines() is closed.
func (c *Conn) Err() error { return c.err }

// Send writes one line (IAC-escaped, CRLF-terminated).
func (c *Conn) Send(line string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.nc.Write(append(telnet.Escape([]byte(line)), '\r', '\n'))
	return err
}

// Resize reports a new window size to the server if NAWS was negotiated.
func (c *Conn) Resize(width, height int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if msg := c.parser.Resize(width, height); msg != nil {
		_, err := c.nc.Write(msg)
		return err
	}
	return nil
}

// Close closes the connection; Lines() will then close.
func (c *Conn) Close() error { return c.nc.Close() }
