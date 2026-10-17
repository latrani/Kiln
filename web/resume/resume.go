// Package resume is how the web build picks a character's held connection
// back up in a fresh tab (a reload, a handoff, a tab iOS discarded). It
// saves, beside the logs, each character's relay token, the stream offset
// just past the last line in its log, and the telnet options agreed, and
// acks the relay only up to offsets the browser has saved, so the relay
// still has whatever a fresh tab needs.
package resume

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"slices"
	"sync"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/telnet"
	"github.com/latrani/Kiln/web/wsdial"
)

// Entry is one character's held connection.
type Entry struct {
	Token  string       `json:"token"`
	Kept   int64        `json:"kept"`
	Telnet telnet.State `json:"telnet"`
}

// Store is the saved entries, in one file (relay.json).
type Store struct {
	path   string
	saving bool // the browser saves files; false: nothing outlasts the tab

	mu    sync.Mutex
	m     map[string]Entry
	dirty bool               // changed since the last Sync
	queue []map[string]int64 // kept offsets per Sync not yet saved, oldest first
	links map[string]*wsdial.Link
}

// Open reads the store at path; a missing or unreadable file is empty.
// saving says whether the browser saves files at all.
func Open(path string, saving bool) *Store {
	s := &Store{path: path, saving: saving, m: map[string]Entry{}, links: map[string]*wsdial.Link{}}
	if b, err := os.ReadFile(path); err == nil {
		json.Unmarshal(b, &s.m)
	}
	return s
}

// Path is the store's file.
func (s *Store) Path() string { return s.path }

// Keys are the characters with a held connection saved.
func (s *Store) Keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keys []string
	for k := range s.m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func (s *Store) Get(key string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[key]
	return e, ok
}

// Put saves e for key in the file (the browser saves it later; Sync says
// when).
func (s *Store) Put(key string, e Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = e
	s.writeLocked()
}

func (s *Store) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteLocked(key, "")
}

// deleteLocked deletes key's entry, if token is "" or still its token.
func (s *Store) deleteLocked(key, token string) {
	if e, ok := s.m[key]; ok && (token == "" || e.Token == token) {
		delete(s.m, key)
		delete(s.links, key)
		s.writeLocked()
	}
}

func (s *Store) writeLocked() {
	s.dirty = true
	b, _ := json.Marshal(s.m)
	os.WriteFile(s.path, b, 0o600)
}

// Sync, if anything changed since the last one, writes the file and has
// the browser save it. The kept offsets are acked once Synced says that
// save landed; with saving off, at once.
func (s *Store) Sync() {
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return
	}
	s.dirty = false
	kept := map[string]int64{}
	for k, e := range s.m {
		kept[k] = e.Kept
	}
	if !s.saving {
		s.mu.Unlock()
		s.ack(kept)
		return
	}
	s.queue = append(s.queue, kept)
	b, _ := json.Marshal(s.m)
	s.mu.Unlock()
	// The fsync has the page flush to IndexedDB; Synced hears it landed.
	// It's outside the lock: the page's callback could come back in.
	if f, err := os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600); err == nil {
		f.Write(b)
		f.Sync()
		f.Close()
	}
}

// Synced is the page saying a save that path's fsync asked for has
// landed. For this store's file, the oldest Sync's offsets are acked.
// Saves land in the order they were asked for.
func (s *Store) Synced(path string) {
	s.mu.Lock()
	if path != s.path || len(s.queue) == 0 {
		s.mu.Unlock()
		return
	}
	kept := s.queue[0]
	s.queue = s.queue[1:]
	s.mu.Unlock()
	s.ack(kept)
}

func (s *Store) ack(kept map[string]int64) {
	for k, off := range kept {
		s.mu.Lock()
		l := s.links[k]
		s.mu.Unlock()
		if l != nil {
			l.Kept(off)
		}
	}
}

// Dial connects ch (key is its app key) through the relay at relayURL:
// resuming its saved held connection if it has one, or opening a new one
// and saving its token. A saved hold the relay no longer has is deleted,
// and the dial fails with wsdial.ErrLetGo.
func (s *Store) Dial(ctx context.Context, relayURL, key string, ch config.Character) (*Conn, error) {
	e, saved := s.Get(key)
	o := wsdial.Options{TLS: ch.TLS, Durable: true}
	co := conn.Options{Host: ch.Host, Port: ch.Port, Width: 80, Height: 24}
	if saved {
		o.Resume = &wsdial.Held{Token: e.Token, Kept: e.Kept}
		co.Telnet = &e.Telnet
	}
	var link *wsdial.Link
	co.DialContext = func(ctx context.Context, _, addr string) (net.Conn, error) {
		l, err := wsdial.Dial(ctx, relayURL, addr, o)
		if err != nil {
			return nil, err
		}
		link = l
		return l, nil
	}
	cc, err := conn.Dial(ctx, co)
	if err != nil {
		if saved && errors.Is(err, wsdial.ErrLetGo) {
			s.Delete(key)
		}
		return nil, err
	}
	if !saved {
		s.Put(key, Entry{Token: link.Token()})
		s.Sync() // now, not at the next tick: a reload soon after can resume
	}
	s.mu.Lock()
	s.links[key] = link
	s.mu.Unlock()
	if saved {
		link.Kept(e.Kept) // it came from a save that landed
	}
	token := link.Token()
	link.OnEnd(func(error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.links[key] == link {
			delete(s.links, key)
		}
		s.deleteLocked(key, token)
	})
	return &Conn{Conn: cc, s: s, key: key, link: link}, nil
}

// Conn is a character's connection that a fresh tab can pick up: a
// *conn.Conn that keeps its place in the store.
type Conn struct {
	*conn.Conn
	s    *Store
	key  string
	link *wsdial.Link
}

// Keep saves that every line up to off is in the log.
func (c *Conn) Keep(off int64) {
	c.s.Put(c.key, Entry{Token: c.link.Token(), Kept: off, Telnet: c.TelnetState()})
}

// Resumed reports whether the connection picked up a held one.
func (c *Conn) Resumed() bool { return c.link.Resumed() }
