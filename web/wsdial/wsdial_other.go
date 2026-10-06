//go:build !js

package wsdial

import (
	"context"
	"net"

	"github.com/coder/websocket"
)

// Dialer returns a conn.Options.DialContext that reaches addr through the
// relay at relayURL (ws:// or wss://).
func Dialer(relayURL string) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, _, addr string) (net.Conn, error) {
		u, err := target(relayURL, addr)
		if err != nil {
			return nil, err
		}
		c, _, err := websocket.Dial(ctx, u, nil)
		if err != nil {
			return nil, err
		}
		c.SetReadLimit(1 << 20)
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
