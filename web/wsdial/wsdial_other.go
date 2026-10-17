//go:build !js

package wsdial

import (
	"context"

	"github.com/coder/websocket"
)

func openSocket(ctx context.Context, url string) (socket, error) {
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	c.SetReadLimit(1 << 20)
	return c, nil
}
