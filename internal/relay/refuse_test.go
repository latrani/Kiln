package relay

import (
	"context"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/relay/wire"
	"github.com/latrani/Kiln/internal/str"
)

// A refusal is a message saying why, then a hang-up: no close handshake
// for a sender that needn't answer to keep the socket open.
func TestRefusalIsAMessageThenAHangUp(t *testing.T) {
	_, base, _ := relay(t, world(1, ""))
	c := open(t, base, "unlisted.example", 7)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	typ, b, err := c.Read(ctx)
	if err != nil || typ != websocket.MessageText {
		t.Fatalf("first frame: %v %v", typ, err)
	}
	m, _ := wire.Decode(b)
	if m.T != wire.Refused || m.Code != int(wire.CodeNotListed) {
		t.Fatalf("first message %+v, want refused %d", m, wire.CodeNotListed)
	}
	start := time.Now()
	_, _, err = c.Read(ctx)
	if err == nil || websocket.CloseStatus(err) != -1 {
		t.Errorf("after the refusal: %v, want the connection dropped without a close", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("hung up after %v", d)
	}
}

// A resume closing before it names a hold frees its waiting slot.
func TestWaitingResumeThatHangsUpFreesItsSlot(t *testing.T) {
	m := newMuck(t)
	_, base, _ := holdRelay(t, m, "max_per_ip = 1\n")
	silentResume(t, base).CloseNow()
	eventually(t, "the slot never came back", func() bool {
		c, _ := tryResume(t, base)
		if c != nil {
			c.CloseNow()
		}
		return c != nil
	})
}

func TestWaitingResumesBusyHasItsOwnLine(t *testing.T) {
	m := newMuck(t)
	_, base, logs := holdRelay(t, m, "max_conns = 1\nmax_per_ip = 8\n")
	silentResume(t, base)
	tryResume(t, base)
	eventually(t, "busy refusal not logged", func() bool { return logs.has(str.RelayRefusedResumingBusy(local)) })
	if logs.has(str.RelayRefusedResuming(local)) {
		t.Error("a relay-wide refusal logged the per-client line")
	}
}
