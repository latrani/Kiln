//go:build !js

package wsdial

import (
	"context"
	"errors"
	"net"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/relay/wire"
	"github.com/latrani/Kiln/internal/str"
)

// Dialer returns a conn.Options.DialContext that reaches addr through the
// relay at relayURL (ws:// or wss://), asking for TLS if tls. It returns
// once the relay says it has reached the world, so a refusal is a dial
// error.
func Dialer(relayURL string, tls bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, _, addr string) (net.Conn, error) {
		u, err := target(relayURL, addr, tls)
		if err != nil {
			return nil, err
		}
		c, _, err := websocket.Dial(ctx, u, nil)
		if err != nil {
			return nil, err
		}
		c.SetReadLimit(1 << 20)
		typ, b, err := c.Read(ctx)
		if err != nil {
			c.CloseNow()
			return nil, explain(err)
		}
		if typ != websocket.MessageText || string(b) != wire.Open {
			c.CloseNow()
			return nil, errors.New(str.ConnRelayDropped())
		}
		// The connection outlives ctx, which only bounds the dial.
		return relayConn{websocket.NetConn(context.Background(), c, websocket.MessageBinary)}, nil
	}
}

// relayConn explains the relay's refusals.
type relayConn struct{ net.Conn }

func (c relayConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	return n, explain(err)
}

func (c relayConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	return n, explain(err)
}
