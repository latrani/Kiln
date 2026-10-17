//go:build !js

package wsdial

import (
	"context"
	"net"

	"github.com/coder/websocket"
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
		return dialLink(ctx, openSocket, u, resumeTarget(relayURL))
	}
}

func openSocket(ctx context.Context, url string) (socket, error) {
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	c.SetReadLimit(1 << 20)
	return c, nil
}
