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
		return str.Wrap(str.ConnRelayNeedsTls(), err)
	case codeTooMany:
		return str.Wrap(str.ConnRelayTooMany(), err)
	case codeUnreachable:
		return str.Wrap(str.ConnRelayUnreachable(ce.Reason), err)
	}
	return err
}
