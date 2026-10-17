//go:build !js

package wsdial

import (
	"context"
	"sync"
	"testing"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/relay/wire"
)

// pushLog records the push messages links send.
type pushLog struct {
	mu   sync.Mutex
	sent []string
}

// all is the endpoints sent, leaving out "" (push nowhere).
func (p *pushLog) all() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, e := range p.sent {
		if e != "" {
			out = append(out, e)
		}
	}
	return out
}

// raw is every push message sent, "" included.
func (p *pushLog) raw() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.sent...)
}

type recordingSocket struct {
	socket
	log *pushLog
}

func (r recordingSocket) Write(ctx context.Context, typ websocket.MessageType, b []byte) error {
	if typ == websocket.MessageText {
		if m, err := wire.Decode(b); err == nil && m.T == wire.Push {
			r.log.mu.Lock()
			r.log.sent = append(r.log.sent, m.Endpoint)
			r.log.mu.Unlock()
		}
	}
	return r.socket.Write(ctx, typ, b)
}

// recordPushes has Dial's sockets record push messages, and clears the
// endpoint after the test.
func recordPushes(t *testing.T) *pushLog {
	t.Helper()
	log := &pushLog{}
	old := dialOpen
	dialOpen = func(ctx context.Context, url string) (socket, error) {
		s, err := openSocket(ctx, url)
		if err != nil {
			return nil, err
		}
		return recordingSocket{s, log}, nil
	}
	t.Cleanup(func() { dialOpen = old; SetPush("") })
	return log
}

const kitEndpoint = "https://fcm.googleapis.com/fcm/send/kit"

func TestLinkSendsPushAfterOpen(t *testing.T) {
	fast(t)
	log := recordPushes(t)
	w := newWorld(t)
	u, _ := holdRelay(t, w, "", nil)
	SetPush(kitEndpoint)
	if _, _, err := dialWith(t, u, w, Options{Push: true}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "no push message", func() bool { return len(log.all()) == 1 && log.all()[0] == kitEndpoint })
}

func TestLinkSendsPushAfterResume(t *testing.T) {
	fast(t)
	log := recordPushes(t)
	w := newWorld(t)
	u, _ := holdRelay(t, w, "", nil)
	SetPush(kitEndpoint)
	c, l, err := dialWith(t, u, w, Options{Push: true, Durable: true})
	if err != nil {
		t.Fatal(err)
	}
	w.conn(t)
	eventually(t, "no push after open", func() bool { return len(log.all()) == 1 })
	DropAll()
	if l := lines(t, c, 1)[0]; !isResumed(l) {
		t.Fatalf("got %+v, want a resumed note", l)
	}
	eventually(t, "no push after a same-tab resume", func() bool { return len(log.all()) == 2 })
	token := l.Token()
	DetachAll()
	if _, _, err := dialWith(t, u, w, Options{Push: true, Durable: true, Resume: &Held{Token: token}}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "no push after a fresh-tab resume", func() bool { return len(log.all()) == 3 })
}

func TestSetPushReachesLiveLinks(t *testing.T) {
	fast(t)
	log := recordPushes(t)
	w := newWorld(t)
	u, _ := holdRelay(t, w, "", nil)
	if _, _, err := dialWith(t, u, w, Options{Push: true}); err != nil {
		t.Fatal(err)
	}
	w.conn(t)
	if got := log.all(); len(got) != 0 {
		t.Fatalf("sent %q with no endpoint", got)
	}
	SetPush(kitEndpoint)
	eventually(t, "the live link never sent it", func() bool { return len(log.all()) == 1 && log.all()[0] == kitEndpoint })
}

func TestNoPushWhenOff(t *testing.T) {
	fast(t)
	log := recordPushes(t)
	w := newWorld(t)
	u, _ := holdRelay(t, w, "", nil)
	SetPush(kitEndpoint)
	c, _, err := dialWith(t, u, w, Options{})
	if err != nil {
		t.Fatal(err)
	}
	w.conn(t)
	SetPush(kitEndpoint + "2")
	DropAll()
	lines(t, c, 1)
	if got := log.all(); len(got) != 0 {
		t.Errorf("sent %q for a character with notifications off", got)
	}
}

func TestNotifyOffClearsTheEndpoint(t *testing.T) {
	fast(t)
	log := recordPushes(t)
	w := newWorld(t)
	u, _ := holdRelay(t, w, "", nil)
	SetPush(kitEndpoint)
	_, l, err := dialWith(t, u, w, Options{Push: true, Durable: true})
	if err != nil {
		t.Fatal(err)
	}
	w.conn(t)
	eventually(t, "no push after open", func() bool { return len(log.raw()) == 1 })
	token := l.Token()
	DetachAll()
	// The page reloads with Kit's notify now "none": the relay must stop
	// pushing for the hold it still has.
	if _, _, err := dialWith(t, u, w, Options{Durable: true, Resume: &Held{Token: token}}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the endpoint never cleared", func() bool { r := log.raw(); return len(r) == 2 && r[1] == "" })
}
