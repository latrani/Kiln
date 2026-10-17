// Package wsdial dials MUCKs through kiln-relay: a WebSocket to the relay,
// speaking internal/relay/wire, as a net.Conn that survives the WebSocket
// dying (link.go). The relay does TLS with the world; telnet happens in
// Kiln. In the browser it uses the page's WebSocket
// directly (wsdial_js.go); elsewhere coder/websocket (wsdial_other.go),
// which is how it's tested natively.
package wsdial

import (
	"context"
	"errors"
	"net"
	"net/url"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/relay/wire"
	"github.com/latrani/Kiln/internal/str"
)

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

// Dialer returns a conn.Options.DialContext that reaches addr through the
// relay at relayURL, asking for TLS if tls: Dial, with nothing saved.
func Dialer(relayURL string, tls bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, _, addr string) (net.Conn, error) {
		l, err := Dial(ctx, relayURL, addr, Options{TLS: tls})
		if err != nil {
			return nil, err
		}
		return l, nil
	}
}

// resumeTarget is the relay URL a held connection resumes at.
func resumeTarget(relayURL string) string {
	u, err := url.Parse(relayURL)
	if err != nil {
		return relayURL
	}
	u.RawQuery = url.Values{"resume": {"1"}}.Encode()
	return u.String()
}

// explain turns the relay's refusals, and other unexpected closes, into
// errors people can read. Normal closures pass through.
func explain(err error) error {
	var ce websocket.CloseError
	if err == nil || !errors.As(err, &ce) {
		return err
	}
	switch ce.Code {
	case wire.CodeNotListed:
		return str.Wrap(str.ConnRelayNotListed(), err)
	case wire.CodeNoTLS:
		return str.Wrap(str.ConnRelayNoTls(), err)
	case wire.CodeBadCert:
		return &CertError{err}
	case wire.CodeTooMany:
		return str.Wrap(str.ConnRelayTooMany(), err)
	case wire.CodeUnreachable:
		return str.Wrap(str.ConnRelayUnreachable(ce.Reason), err)
	case wire.CodeBusy:
		return str.Wrap(str.ConnRelayBusy(), err)
	case websocket.StatusNormalClosure, websocket.StatusGoingAway:
		return err
	}
	return str.Wrap(str.ConnRelayDropped(), err)
}
