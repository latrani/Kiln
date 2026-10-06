// Package wsdial dials MUCKs through kiln-relay: a WebSocket to the relay,
// used as a plain net.Conn, so TLS and telnet still happen in Kiln and the
// relay only passes bytes. In the browser it uses the page's WebSocket
// directly (wsdial_js.go); elsewhere coder/websocket (wsdial_other.go),
// which is how it's tested natively.
package wsdial

import (
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

// target is the relay URL that asks for addr (host:port).
func target(relayURL, addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(relayURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("host", host)
	q.Set("port", port)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// explain turns the relay's refusals into errors people can read.
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
