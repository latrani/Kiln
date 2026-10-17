package relay

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/relay/wire"
	"github.com/latrani/Kiln/internal/str"
)

// silentResume opens a ?resume=1 socket that names nothing yet.
func silentResume(t *testing.T, base string) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.Dial(context.Background(), base+"/?resume=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	return c
}

// closedWith is the code c was closed with within wait, or -1 if it's
// still open.
func closedWith(c *websocket.Conn, wait time.Duration) websocket.StatusCode {
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	_, _, err := c.Read(ctx)
	if ctx.Err() != nil {
		return -1
	}
	return websocket.CloseStatus(err)
}

func TestWaitingResumesAreCapped(t *testing.T) {
	m := newMuck(t)
	_, base, logs := holdRelay(t, m, "max_per_ip = 2\n")
	cs := []*websocket.Conn{silentResume(t, base), silentResume(t, base), silentResume(t, base)}
	refused := 0
	for _, c := range cs {
		switch code := closedWith(c, 300*time.Millisecond); code {
		case wire.CodeTooMany:
			refused++
		case -1:
		default:
			t.Errorf("closed with %d", code)
		}
	}
	if refused != 1 {
		t.Errorf("%d of 3 waiting resumes refused, want 1", refused)
	}
	eventually(t, "refusal not logged", func() bool { return logs.has(str.RelayRefusedResuming(local)) })
}

func TestWaitingResumeSlotFrees(t *testing.T) {
	m := newMuck(t)
	_, base, _ := holdRelay(t, m, "max_per_ip = 2\n")
	a := silentResume(t, base)
	silentResume(t, base)
	time.Sleep(100 * time.Millisecond) // both counted
	a.Write(context.Background(), websocket.MessageText, wire.Encode(wire.Msg{T: wire.Resume, Token: "nope"}))
	if code := closedWith(a, 2*time.Second); code != wire.CodeGone {
		t.Fatalf("bad token: closed with %d", code)
	}
	if code := closedWith(silentResume(t, base), 300*time.Millisecond); code != -1 {
		t.Errorf("a waiting resume after one named its token was closed with %d", code)
	}
}

func TestResumeAtTheCapStillWorks(t *testing.T) {
	m := newMuck(t)
	_, base, logs := holdRelay(t, m, "max_per_ip = 2\n")
	p, q := openPage(t, base, m.port), openPage(t, base, m.port)
	m.conn(t)
	m.conn(t)
	p.c.CloseNow()
	q.c.CloseNow()
	eventually(t, "never held", func() bool { return logs.count(str.RelayHeld(local, m.addr())) == 2 })
	done := make(chan string, 2)
	for _, tok := range []string{p.token, q.token} {
		go func() {
			c, _, err := websocket.Dial(context.Background(), base+"/?resume=1", nil)
			if err != nil {
				done <- err.Error()
				return
			}
			defer c.CloseNow()
			c.Write(context.Background(), websocket.MessageText, wire.Encode(wire.Msg{T: wire.Resume, Token: tok}))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, b, err := c.Read(ctx)
			if err != nil {
				done <- err.Error()
				return
			}
			m, _ := wire.Decode(b)
			done <- m.T
		}()
	}
	for range 2 {
		if got := <-done; got != wire.Resumed {
			t.Errorf("resuming at the cap: %s", got)
		}
	}
}

func TestWaitingResumesGlobalCap(t *testing.T) {
	a, _ := ParseConfig("relay.toml", []byte("max_conns = 2\n"))
	s := &Server{}
	wait := func(ip string) websocket.StatusCode { return s.waitResume(netip.MustParseAddr(ip), a) }
	if wait("192.0.2.1") != 0 || wait("192.0.2.2") != 0 {
		t.Fatal("first two refused")
	}
	if code := wait("192.0.2.3"); code != wire.CodeBusy {
		t.Errorf("third: code %d, want %d", code, wire.CodeBusy)
	}
	s.doneResume(netip.MustParseAddr("192.0.2.1"))
	if wait("192.0.2.3") != 0 {
		t.Error("the slot never came back")
	}
}
