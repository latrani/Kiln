// Package wsdial dials MUCKs through kiln-relay: a WebSocket to the relay,
// used as a plain net.Conn. The relay does TLS with the world; telnet
// happens in Kiln. In the browser it uses the page's WebSocket
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
	codeNoTLS       websocket.StatusCode = 4426
	codeTooMany     websocket.StatusCode = 4429
	codeUnreachable websocket.StatusCode = 4502
	codeBusy        websocket.StatusCode = 4503
	codeBadCert     websocket.StatusCode = 4526
)

// openFrame is relay.OpenFrame: the relay has reached the world.
const openFrame = "open" //str:ok protocol token

// CertError is the relay refusing the world's certificate. It won't
// change by retrying, so it's Permanent: the session waits for the
// player to reconnect.
type CertError struct{ err error }

func (e *CertError) Error() string {
	var ce websocket.CloseError
	errors.As(e.err, &ce)
	return str.ConnRelayBadCert(ce.Reason)
}
func (e *CertError) Unwrap() error   { return e.err }
func (e *CertError) Permanent() bool { return true }

// target is the relay URL that asks for addr (host:port), and for TLS
// when the world file does.
func target(relayURL, addr string, tls bool) (string, error) {
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
	if tls {
		q.Set("tls", "1")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// explain turns the relay's refusals, and other unexpected closes, into
// errors people can read. Normal closures pass through.
func explain(err error) error {
	var ce websocket.CloseError
	if err == nil || !errors.As(err, &ce) {
		return err
	}
	switch ce.Code {
	case codeNotListed:
		return str.Wrap(str.ConnRelayNotListed(), err)
	case codeNoTLS:
		return str.Wrap(str.ConnRelayNoTls(), err)
	case codeBadCert:
		return &CertError{err}
	case codeTooMany:
		return str.Wrap(str.ConnRelayTooMany(), err)
	case codeUnreachable:
		return str.Wrap(str.ConnRelayUnreachable(ce.Reason), err)
	case codeBusy:
		return str.Wrap(str.ConnRelayBusy(), err)
	case websocket.StatusNormalClosure, websocket.StatusGoingAway:
		return err
	}
	return str.Wrap(str.ConnRelayDropped(), err)
}
