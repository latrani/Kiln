// Package wire is what kiln-relay and Kiln's web build (web/wsdial) say to
// each other over the WebSocket. Binary frames carry the world's bytes:
// down (relay to page) each starts with the time those bytes reached the
// relay; up they're bare. Text frames are JSON control messages (Msg).
//
// Each side counts the data bytes it has received since the connection
// opened, and acks and resumes are in those offsets, so a WebSocket can
// die and another pick up where it left off.
package wire

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Close codes the relay ends a WebSocket with. The web build turns them
// into messages.
const (
	CodeNotListed   websocket.StatusCode = 4403 // world not on the allowlist
	CodeReplaced    websocket.StatusCode = 4409 // a newer attachment took this connection
	CodeGone        websocket.StatusCode = 4410 // no such hold: unknown token, expired or closed
	CodeNoTLS       websocket.StatusCode = 4426 // client asked for TLS, world is plaintext here
	CodeTooMany     websocket.StatusCode = 4429 // too many connections from this IP
	CodeUnreachable websocket.StatusCode = 4502 // the dial failed; the reason says why
	CodeBusy        websocket.StatusCode = 4503 // the relay is at max_conns
	CodeBadCert     websocket.StatusCode = 4526 // the world's certificate failed its pin or CA check
)

// Control message types.
const (
	// Relay to page.
	Open    = "open"    //str:ok protocol token: reached the world; Token
	Resumed = "resumed" //str:ok protocol token: Got (up bytes written), At (down bytes so far), Away (ms)
	Missed  = "missed"  //str:ok protocol token: Lines dropped; data goes on from At
	Pong    = "pong"    //str:ok protocol token
	End     = "end"     //str:ok protocol token: the world closed after At bytes; Why
	// Page to relay.
	Resume = "resume" //str:ok protocol token: Token, Got (down bytes received)
	Ping   = "ping"   //str:ok protocol token
	Close  = "close"  //str:ok protocol token: let the world go now
	// Both ways.
	Ack = "ack" //str:ok protocol token: Got
)

// Msg is a control message; T says which, and only its fields are set.
type Msg struct {
	T     string `json:"t"`
	Token string `json:"token,omitempty"`
	Got   int64  `json:"got,omitempty"`
	At    int64  `json:"at,omitempty"`
	Lines int    `json:"lines,omitempty"`
	Away  int64  `json:"away,omitempty"`
	Why   string `json:"why,omitempty"`
}

// A side acks once AckBytes are unacknowledged, or AckEvery after the
// first unacknowledged byte, whichever comes first.
const (
	AckBytes = 64 << 10
	AckEvery = time.Second
)

// Encode is m as a text frame.
func Encode(m Msg) []byte {
	b, _ := json.Marshal(m)
	return b
}

// Decode reads a text frame.
func Decode(b []byte) (Msg, error) {
	var m Msg
	err := json.Unmarshal(b, &m)
	return m, err
}

// errShort is a down frame too short to hold its time. The web build
// reports any protocol error as a dropped relay connection.
var errShort = errors.New("short down frame") //str:ok protocol error, never shown

// DownFrame is p as a down frame, stamped at.
func DownFrame(at time.Time, p []byte) []byte {
	b := make([]byte, 8+len(p))
	binary.BigEndian.PutUint64(b, uint64(at.UnixMilli()))
	copy(b[8:], p)
	return b
}

// ReadDown splits a down frame into its time and bytes.
func ReadDown(b []byte) (at time.Time, p []byte, err error) {
	if len(b) < 8 {
		return time.Time{}, nil, errShort
	}
	return time.UnixMilli(int64(binary.BigEndian.Uint64(b))), b[8:], nil
}

// An Acker decides when a side acknowledges what it has received: once
// AckBytes are unacknowledged, or AckEvery after the first unacknowledged
// byte. Send is called with no lock held.
type Acker struct {
	Send func(got int64)

	mu       sync.Mutex
	got, ack int64
	t        *time.Timer
	stopped  bool
}

// NewAcker starts counting from got, which needs no ack.
func NewAcker(got int64, send func(got int64)) *Acker {
	return &Acker{Send: send, got: got, ack: got}
}

// Received counts n more bytes.
func (a *Acker) Received(n int) { a.Skip(a.Got() + int64(n)) }

// Skip moves what's been received on to got (past bytes the other side
// dropped), if that's further.
func (a *Acker) Skip(got int64) {
	a.mu.Lock()
	if a.stopped || got <= a.got {
		a.mu.Unlock()
		return
	}
	a.got = got
	if a.got-a.ack >= AckBytes {
		a.ack = a.got
		a.mu.Unlock()
		a.Send(got)
		return
	}
	if a.t == nil {
		a.t = time.AfterFunc(AckEvery, a.flush)
	}
	a.mu.Unlock()
}

// Got is how much has been received.
func (a *Acker) Got() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.got
}

func (a *Acker) flush() {
	a.mu.Lock()
	a.t = nil
	if a.stopped || a.got == a.ack {
		a.mu.Unlock()
		return
	}
	a.ack = a.got
	got := a.got
	a.mu.Unlock()
	a.Send(got)
}

// Stop sends no more acks.
func (a *Acker) Stop() {
	a.mu.Lock()
	a.stopped = true
	if a.t != nil {
		a.t.Stop()
	}
	a.mu.Unlock()
}
