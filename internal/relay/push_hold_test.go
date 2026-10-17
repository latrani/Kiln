package relay

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/relay/wire"
	"github.com/latrani/Kiln/internal/str"
)

// pushService is a push service under test, answering status after
// wait; posts counts what it's been sent.
type pushService struct {
	url   string
	host  string
	posts atomic.Int32
}

// pushRelay is a hold relay for m's world with a VAPID key, whose pushes
// go to a test push service.
func pushRelay(t *testing.T, m *muck, status int, wait time.Duration) (*Server, string, *logLines, *pushService) {
	t.Helper()
	dir, _, _ := vapidDir(t)
	ps := &pushService{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ps.posts.Add(1)
		time.Sleep(wait)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	ps.url, ps.host = srv.URL, u.Hostname()
	s, base, logs := holdRelay(t, m, "vapid_key = \""+filepath.Join(dir, "vapid.pem")+"\"\npush_contact = \"https://fm.example\"\n")
	s.pushClient, s.pushHosts = srv.Client(), []string{ps.host}
	return s, base, logs, ps
}

// away subscribes p to ps, drops its WebSocket, and waits until held.
func away(t *testing.T, p *page, ps *pushService, logs *logLines, m *muck) {
	t.Helper()
	p.send(wire.Msg{T: wire.Push, Endpoint: ps.url + "/push/kit"})
	p.send(wire.Msg{T: wire.Ping})
	for f := p.next(); f.msg.T != wire.Pong; f = p.next() { // the push message has been read
		if f.err != nil {
			t.Fatal(f.err)
		}
	}
	n := logs.count(str.RelayHeld(local, m.addr()))
	p.c.CloseNow()
	eventually(t, "never held", func() bool { return logs.count(str.RelayHeld(local, m.addr())) > n })
}

func TestPushOncePerAbsence(t *testing.T) {
	m := newMuck(t)
	_, base, logs, ps := pushRelay(t, m, http.StatusCreated, 0)
	p := openPage(t, base, m.port)
	w := m.conn(t)
	away(t, p, ps, logs, m)
	go func() {
		for range 50 {
			w.Write([]byte("Rook says, \"hi\"\r\n"))
		}
	}()
	eventually(t, "never pushed", func() bool { return ps.posts.Load() > 0 })
	time.Sleep(500 * time.Millisecond)
	if n := ps.posts.Load(); n != 1 {
		t.Errorf("%d pushes, want 1", n)
	}
	eventually(t, "push not logged", func() bool { return logs.has(str.RelayPushed(local, m.addr())) })
}

func TestNoPushWhileAttached(t *testing.T) {
	m := newMuck(t)
	_, base, _, ps := pushRelay(t, m, http.StatusCreated, 0)
	p := openPage(t, base, m.port)
	w := m.conn(t)
	p.send(wire.Msg{T: wire.Push, Endpoint: ps.url + "/push/kit"})
	w.Write([]byte("hi\r\n"))
	p.data(4)
	time.Sleep(200 * time.Millisecond)
	if n := ps.posts.Load(); n != 0 {
		t.Errorf("%d pushes while attached", n)
	}
}

func TestPushAgainAfterResume(t *testing.T) {
	m := newMuck(t)
	_, base, logs, ps := pushRelay(t, m, http.StatusCreated, 0)
	p := openPage(t, base, m.port)
	w := m.conn(t)
	away(t, p, ps, logs, m)
	w.Write([]byte("one\r\n"))
	eventually(t, "never pushed", func() bool { return ps.posts.Load() == 1 })
	q := resumePage(t, base, p.token, 0, "")
	q.expect(wire.Resumed)
	away(t, q, ps, logs, m)
	w.Write([]byte("two\r\n"))
	eventually(t, "no second push", func() bool { return ps.posts.Load() == 2 })
}

func TestPushGoneDropsTheEndpoint(t *testing.T) {
	m := newMuck(t)
	_, base, logs, ps := pushRelay(t, m, http.StatusGone, 0)
	p := openPage(t, base, m.port)
	w := m.conn(t)
	away(t, p, ps, logs, m)
	w.Write([]byte("one\r\n"))
	eventually(t, "gone not logged", func() bool { return logs.has(str.RelayPushGone(local, m.addr())) })
	q := resumePage(t, base, p.token, 0, "")
	q.expect(wire.Resumed)
	q.c.CloseNow() // away again, without sending the endpoint again
	eventually(t, "never held again", func() bool { return logs.count(str.RelayHeld(local, m.addr())) == 2 })
	w.Write([]byte("two\r\n"))
	time.Sleep(300 * time.Millisecond)
	if n := ps.posts.Load(); n != 1 {
		t.Errorf("%d pushes, want 1: the gone endpoint was used again", n)
	}
}

func TestPushFailureLogs(t *testing.T) {
	m := newMuck(t)
	_, base, logs, ps := pushRelay(t, m, http.StatusInternalServerError, 0)
	p := openPage(t, base, m.port)
	w := m.conn(t)
	away(t, p, ps, logs, m)
	w.Write([]byte("one\r\n"))
	eventually(t, "failure not logged", func() bool { return logs.has(upTo(str.RelayPushFailed(local, m.addr(), errMark{}))) })
	w.Write([]byte("two\r\n"))
	time.Sleep(300 * time.Millisecond)
	if n := ps.posts.Load(); n != 1 {
		t.Errorf("%d pushes, want 1: no retry within an absence", n)
	}
}

func TestPushRefusesOtherHosts(t *testing.T) {
	m := newMuck(t)
	_, base, logs, ps := pushRelay(t, m, http.StatusCreated, 0)
	p := openPage(t, base, m.port)
	w := m.conn(t)
	for _, e := range []string{"http://" + ps.host + "/push/kit", "https://example.com/push/kit"} {
		p.send(wire.Msg{T: wire.Push, Endpoint: e})
	}
	eventually(t, "refusals not logged", func() bool {
		return logs.has(str.RelayPushRefused(local, ps.host)) && logs.has(str.RelayPushRefused(local, "example.com"))
	})
	p.c.CloseNow()
	eventually(t, "never held", func() bool { return logs.has(str.RelayHeld(local, m.addr())) })
	w.Write([]byte("one\r\n"))
	time.Sleep(300 * time.Millisecond)
	if n := ps.posts.Load(); n != 0 {
		t.Errorf("%d pushes to refused endpoints", n)
	}
}

func TestNoPushWithoutKey(t *testing.T) {
	m := newMuck(t)
	_, _, _, ps := pushRelay(t, m, http.StatusCreated, 0)
	_, base2, logs2 := holdRelay(t, m, "")
	p := openPage(t, base2, m.port)
	w := m.conn(t)
	away(t, p, ps, logs2, m)
	w.Write([]byte("one\r\n"))
	time.Sleep(300 * time.Millisecond)
	if n := ps.posts.Load(); n != 0 {
		t.Errorf("%d pushes with no vapid_key", n)
	}
	if logs2.has(upTo(str.RelayPushFailed(local, m.addr(), errMark{}))) {
		t.Errorf("logged: %q", logs2.all())
	}
}

func TestSlowPushDoesNotBlock(t *testing.T) {
	m := newMuck(t)
	_, base, logs, ps := pushRelay(t, m, http.StatusCreated, 5*time.Second)
	p := openPage(t, base, m.port)
	w := m.conn(t)
	away(t, p, ps, logs, m)
	w.Write([]byte("one\r\n"))
	eventually(t, "never pushed", func() bool { return ps.posts.Load() == 1 })
	w.Write([]byte("two\r\n"))
	start := time.Now()
	q := resumePage(t, base, p.token, 0, "")
	q.expect(wire.Resumed)
	if d, _ := q.data(10); d != "one\r\ntwo\r\n" {
		t.Errorf("resumed with %q", d)
	}
	if el := time.Since(start); el > 500*time.Millisecond {
		t.Errorf("resume took %v while the push hung", el)
	}
}

func TestEmptyEndpointStopsPushes(t *testing.T) {
	m := newMuck(t)
	_, base, logs, ps := pushRelay(t, m, http.StatusCreated, 0)
	p := openPage(t, base, m.port)
	w := m.conn(t)
	p.send(wire.Msg{T: wire.Push, Endpoint: ps.url + "/push/kit"})
	p.send(wire.Msg{T: wire.Push})
	p.send(wire.Msg{T: wire.Ping})
	for f := p.next(); f.msg.T != wire.Pong; f = p.next() {
	}
	p.c.CloseNow()
	eventually(t, "never held", func() bool { return logs.has(str.RelayHeld(local, m.addr())) })
	w.Write([]byte("one\r\n"))
	time.Sleep(300 * time.Millisecond)
	if n := ps.posts.Load(); n != 0 {
		t.Errorf("%d pushes after the endpoint was cleared", n)
	}
}

// A page that vanished without a word (a phone locking) looks attached
// until the relay's pings go unanswered. What the world said meanwhile,
// after the page last said anything, likely never reached it: the hold
// pushes for it as soon as it starts.
func TestPushForOutputBeforeTheDetach(t *testing.T) {
	m := newMuck(t)
	s, base, logs, ps := pushRelay(t, m, http.StatusCreated, 0)
	s.pingEvery, s.pingTimeout = 300*time.Millisecond, 300*time.Millisecond
	p := openPage(t, base, m.port)
	w := m.conn(t)
	p.send(wire.Msg{T: wire.Push, Endpoint: ps.url + "/push/kit"})
	time.Sleep(50 * time.Millisecond)
	// The page stops reading and answering, and the world pages Kit.
	w.Write([]byte("Rook pages: you there?\r\n"))
	eventually(t, "never held", func() bool { return logs.has(str.RelayHeld(local, m.addr())) })
	eventually(t, "no push for the page that came before the detach", func() bool { return ps.posts.Load() == 1 })
}

func TestNoPushForOutputThePageAcked(t *testing.T) {
	m := newMuck(t)
	_, base, logs, ps := pushRelay(t, m, http.StatusCreated, 0)
	p := openPage(t, base, m.port)
	w := m.conn(t)
	p.send(wire.Msg{T: wire.Push, Endpoint: ps.url + "/push/kit"})
	w.Write([]byte("hi\r\n"))
	p.data(4)
	time.Sleep(20 * time.Millisecond)
	p.send(wire.Msg{T: wire.Ack, Got: 4})
	time.Sleep(50 * time.Millisecond)
	p.c.CloseNow()
	eventually(t, "never held", func() bool { return logs.has(str.RelayHeld(local, m.addr())) })
	time.Sleep(300 * time.Millisecond)
	if n := ps.posts.Load(); n != 0 {
		t.Errorf("%d pushes for output the page had seen", n)
	}
}
