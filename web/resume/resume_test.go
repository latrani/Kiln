//go:build !js

package resume

import (
	"context"
	"fmt"
	"net"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/notify"
	"github.com/latrani/Kiln/internal/relay"
	"github.com/latrani/Kiln/web/wsdial"
)

// world is a fake MUCK: the test writes to its connections.
type world struct {
	port  int
	conns chan net.Conn
	n     atomic.Int32
}

func newWorld(t *testing.T) *world {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	w := &world{port: ln.Addr().(*net.TCPAddr).Port, conns: make(chan net.Conn, 4)}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			w.n.Add(1)
			w.conns <- c
			go func() {
				b := make([]byte, 4096)
				for {
					if _, err := c.Read(b); err != nil {
						return
					}
				}
			}()
		}
	}()
	return w
}

func (w *world) conn(t *testing.T) net.Conn {
	t.Helper()
	select {
	case c := <-w.conns:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("the relay never reached the world")
	}
	return nil
}

func relayFor(t *testing.T, w *world) string {
	t.Helper()
	cfg, err := relay.ParseConfig("relay.toml", []byte(fmt.Sprintf("[[world]]\nid = \"fm\"\nhost = \"127.0.0.1\"\nport = %d\n", w.port)))
	if err != nil {
		t.Fatal(err)
	}
	s := &relay.Server{}
	s.SetConfig(cfg)
	hs := httptest.NewServer(s)
	t.Cleanup(hs.Close)
	return "ws" + strings.TrimPrefix(hs.URL, "http")
}

var kit = config.Character{World: "fm", ID: "kit", Name: "Kit", Host: "127.0.0.1"}

func charFor(w *world) config.Character { ch := kit; ch.Port = w.port; return ch }

func lines(t *testing.T, c interface{ Lines() <-chan conn.Line }, n int) []conn.Line {
	t.Helper()
	var got []conn.Line
	for len(got) < n {
		select {
		case l, ok := <-c.Lines():
			if !ok {
				t.Fatalf("closed after %+v", got)
			}
			if !l.Note {
				got = append(got, l)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("after %+v: nothing more", got)
		}
	}
	return got
}

// replayFromZero resumes token from 0 on its own and returns what the
// relay still had, waiting a moment for it.
func replayFromZero(t *testing.T, u string, w *world, token string) []string {
	t.Helper()
	c, err := conn.Dial(context.Background(), conn.Options{Host: "127.0.0.1", Port: w.port,
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			l, err := wsdial.Dial(ctx, u, addr, wsdial.Options{Durable: true, Resume: &wsdial.Held{Token: token}})
			if err != nil {
				return nil, err
			}
			return l, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	defer wsdial.DetachAll()
	var got []string
	for {
		select {
		case l := <-c.Lines():
			if !l.Note {
				got = append(got, l.Text)
			}
		case <-time.After(500 * time.Millisecond):
			return got
		}
	}
}

func TestFreshDialSavesTheToken(t *testing.T) {
	w := newWorld(t)
	u := relayFor(t, w)
	s := Open(filepath.Join(t.TempDir(), "relay.json"), true)
	c, err := s.Dial(context.Background(), u, "fm/kit", charFor(w))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	e, ok := s.Get("fm/kit")
	if !ok || len(e.Token) != 43 || e.Kept != 0 {
		t.Errorf("entry %+v, %v; want a token, kept 0", e, ok)
	}
}

func TestKeptOnlyAfterSave(t *testing.T) {
	w := newWorld(t)
	u := relayFor(t, w)
	path := filepath.Join(t.TempDir(), "relay.json")
	s := Open(path, true)
	c, err := s.Dial(context.Background(), u, "fm/kit", charFor(w))
	if err != nil {
		t.Fatal(err)
	}
	w.conn(t).Write([]byte("1\r\n2\r\n"))
	got := lines(t, c, 2)
	c.Keep(got[1])
	s.Sync()
	time.Sleep(200 * time.Millisecond)
	token, _ := s.Get("fm/kit")
	wsdial.DetachAll()
	if r := replayFromZero(t, u, w, token.Token); strings.Join(r, " ") != "1 2" {
		t.Fatalf("before the save landed, the relay had %q, want 1 2", r)
	}

	s2 := Open(path, true)
	c2, err := s2.Dial(context.Background(), u, "fm/kit", charFor(w))
	if err != nil {
		t.Fatal(err)
	}
	if !c2.Resumed() {
		t.Fatal("not resumed")
	}
	s2.Sync()
	s2.Synced(path)
	time.Sleep(200 * time.Millisecond)
	wsdial.DetachAll()
	if r := replayFromZero(t, u, w, token.Token); len(r) != 0 {
		t.Errorf("after the save landed, the relay still had %q", r)
	}
}

func TestSavingOffAcksAtOnce(t *testing.T) {
	w := newWorld(t)
	u := relayFor(t, w)
	s := Open(filepath.Join(t.TempDir(), "relay.json"), false)
	c, err := s.Dial(context.Background(), u, "fm/kit", charFor(w))
	if err != nil {
		t.Fatal(err)
	}
	w.conn(t).Write([]byte("1\r\n"))
	got := lines(t, c, 1)
	c.Keep(got[0])
	s.Sync()
	time.Sleep(200 * time.Millisecond)
	e, _ := s.Get("fm/kit")
	wsdial.DetachAll()
	if r := replayFromZero(t, u, w, e.Token); len(r) != 0 {
		t.Errorf("with saving off, the relay still had %q after a Sync", r)
	}
}

func TestStoreSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.json")
	s := Open(path, true)
	s.Put("fm/kit", Entry{Token: "t", Kept: 7})
	s.Put("fm/rook", Entry{Token: "u"})
	s.Delete("fm/rook")
	s2 := Open(path, true)
	if e, ok := s2.Get("fm/kit"); !ok || e != (Entry{Token: "t", Kept: 7}) {
		t.Errorf("reopened entry %+v, %v", e, ok)
	}
	if keys := s2.Keys(); len(keys) != 1 || keys[0] != "fm/kit" {
		t.Errorf("keys %q", keys)
	}
	if Open(filepath.Join(t.TempDir(), "missing.json"), true).Keys() != nil {
		t.Error("a missing file isn't empty")
	}
}

func TestSavedTokenResumes(t *testing.T) {
	w := newWorld(t)
	u := relayFor(t, w)
	path := filepath.Join(t.TempDir(), "relay.json")
	s := Open(path, true)
	c, err := s.Dial(context.Background(), u, "fm/kit", charFor(w))
	if err != nil {
		t.Fatal(err)
	}
	wc := w.conn(t)
	wc.Write([]byte("1\r\n"))
	got := lines(t, c, 1)
	c.Keep(got[0])
	wsdial.DetachAll()
	wc.Write([]byte("2\r\n"))
	c2, err := Open(path, true).Dial(context.Background(), u, "fm/kit", charFor(w))
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	if !c2.Resumed() {
		t.Error("a saved token didn't resume")
	}
	if l := lines(t, c2, 1); l[0].Text != "2" {
		t.Errorf("resumed with %q, want 2", l[0].Text)
	}
	if w.n.Load() != 1 {
		t.Errorf("the world took %d connections", w.n.Load())
	}
}

func TestLetGoAtBootDeletesTheToken(t *testing.T) {
	w := newWorld(t)
	u := relayFor(t, w)
	s := Open(filepath.Join(t.TempDir(), "relay.json"), true)
	s.Put("fm/kit", Entry{Token: "gone"})
	if _, err := s.Dial(context.Background(), u, "fm/kit", charFor(w)); err == nil || err.Error() != wsdial.ErrLetGo.Error() {
		t.Errorf("err = %v, want %v", err, wsdial.ErrLetGo)
	}
	if _, ok := s.Get("fm/kit"); ok {
		t.Error("a token the relay let go of is still saved")
	}
}

func TestEndDeletesTheEntry(t *testing.T) {
	w := newWorld(t)
	u := relayFor(t, w)
	s := Open(filepath.Join(t.TempDir(), "relay.json"), true)
	c, err := s.Dial(context.Background(), u, "fm/kit", charFor(w))
	if err != nil {
		t.Fatal(err)
	}
	w.conn(t).Close()
	for range c.Lines() {
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := s.Get("fm/kit"); !ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the world went, and its token is still saved")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSyncedOtherPathIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.json")
	s := Open(path, true)
	s.Put("fm/kit", Entry{Token: "t", Kept: 1})
	s.Sync()
	s.Synced("/home/kiln/drafts.json")
	if len(s.queue) != 1 {
		t.Errorf("another file's sync took this one's turn: queue %v", s.queue)
	}
}

// Lines logged after the link ended (the world said goodbye and closed)
// mustn't save its dead token back.
func TestKeepAfterEndSavesNothing(t *testing.T) {
	w := newWorld(t)
	u := relayFor(t, w)
	s := Open(filepath.Join(t.TempDir(), "relay.json"), true)
	c, err := s.Dial(context.Background(), u, "fm/kit", charFor(w))
	if err != nil {
		t.Fatal(err)
	}
	wc := w.conn(t)
	wc.Write([]byte("Goodbye\r\n"))
	wc.Close()
	got := lines(t, c, 1)
	for range c.Lines() {
	}
	deadline := time.Now().Add(2 * time.Second)
	for _, ok := s.Get("fm/kit"); ok; _, ok = s.Get("fm/kit") {
		if time.Now().After(deadline) {
			t.Fatal("never deleted")
		}
		time.Sleep(10 * time.Millisecond)
	}
	c.Keep(got[0])
	if e, ok := s.Get("fm/kit"); ok {
		t.Errorf("a Keep after the end saved %+v", e)
	}
}

// A snapshot queued for a connection since replaced (same character, new
// token) mustn't ack the new one.
func TestQueuedSnapshotOnlyAcksItsToken(t *testing.T) {
	w := newWorld(t)
	u := relayFor(t, w)
	path := filepath.Join(t.TempDir(), "relay.json")
	s := Open(path, true)
	c, err := s.Dial(context.Background(), u, "fm/kit", charFor(w))
	if err != nil {
		t.Fatal(err)
	}
	w.conn(t).Write([]byte("1\r\n"))
	lines(t, c, 1)
	s.mu.Lock()
	s.queue = []map[string]Entry{{"fm/kit": {Token: "an older one", Kept: 1 << 20}}}
	s.mu.Unlock()
	s.Synced(path)
	time.Sleep(200 * time.Millisecond)
	e, _ := s.Get("fm/kit")
	wsdial.DetachAll()
	if r := replayFromZero(t, u, w, e.Token); len(r) != 1 {
		t.Errorf("another token's snapshot acked this one: the relay has %q", r)
	}
}

// When the browser stops saving (a save failed), offsets waiting on a
// save are acked, and later ones at once: the relay mustn't fill up
// waiting for saves that won't come.
func TestSavingOffAcksWhatWaits(t *testing.T) {
	w := newWorld(t)
	u := relayFor(t, w)
	s := Open(filepath.Join(t.TempDir(), "relay.json"), true)
	c, err := s.Dial(context.Background(), u, "fm/kit", charFor(w))
	if err != nil {
		t.Fatal(err)
	}
	w.conn(t).Write([]byte("1\r\n"))
	got := lines(t, c, 1)
	c.Keep(got[0])
	s.Sync()
	s.SavingOff()
	time.Sleep(200 * time.Millisecond)
	e, _ := s.Get("fm/kit")
	wsdial.DetachAll()
	if r := replayFromZero(t, u, w, e.Token); len(r) != 0 {
		t.Errorf("after saving stopped, the relay still had %q", r)
	}
}

// Entries for characters that no longer exist go, rather than being read
// at every start.
func TestPruneDropsGoneCharacters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.json")
	s := Open(path, true)
	s.Put("fm/kit", Entry{Token: "t"})
	s.Put("fm/ash", Entry{Token: "u"})
	s.Prune(func(key string) bool { return key == "fm/kit" })
	if keys := Open(path, true).Keys(); len(keys) != 1 || keys[0] != "fm/kit" {
		t.Errorf("keys after pruning %q, want just fm/kit", keys)
	}
}

func TestDialPushFollowsNotify(t *testing.T) {
	for _, c := range []struct {
		level notify.Level
		push  bool
	}{{notify.None, false}, {notify.First, true}, {notify.All, true}, {notify.Priority, true}} {
		ch := kit
		ch.Notify = c.level
		if o := options(ch); o.Push != c.push || !o.Durable {
			t.Errorf("notify %q: options %+v, want push %v", c.level, o, c.push)
		}
	}
}
