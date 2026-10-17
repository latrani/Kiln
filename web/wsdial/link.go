package wsdial

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/relay/wire"
	"github.com/latrani/Kiln/internal/str"
)

// socket is one WebSocket to the relay. *websocket.Conn is one; the
// browser build wraps the page's WebSocket (wsdial_js.go).
type socket interface {
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
	Write(ctx context.Context, typ websocket.MessageType, p []byte) error
	Close(code websocket.StatusCode, reason string) error
	CloseNow() error
}

// opener opens a socket to url.
type opener func(ctx context.Context, url string) (socket, error)

// How resuming paces itself; tests change them.
var (
	// retryDelays are the waits before each try to resume; the last
	// repeats.
	retryDelays = []time.Duration{0, time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 15 * time.Second}
	// wakeWait is how long Wake waits for the relay's pong.
	wakeWait = 3 * time.Second
	// lostAfter is how long resuming fails before the player is told.
	lostAfter = 30 * time.Second
	// resumeTimeout bounds one try to resume.
	resumeTimeout = 15 * time.Second
)

// maxFrame is the most data put in one frame, well under the relay's
// read limit (1 MiB), however much is written at once.
const maxFrame = 64 << 10

// errReplaced is another page resuming this connection: this one is done.
var errReplaced = errors.New(str.ConnRelayReplaced())

// ErrLetGo is the relay no longer holding the connection (its hold ran
// out, or it restarted). It isn't Permanent: the session's reconnect
// setting decides what happens next.
var ErrLetGo = errors.New(str.ConnRelayLetGo())

// A Link is a world connection through the relay that outlives any one
// WebSocket. When its socket dies without Close, it resumes the held
// connection on a new one, resending what the relay hadn't acknowledged,
// so the connection above it never sees a break. It's a net.Conn and a
// conn.RecordReader.
type Link struct {
	open      opener
	resumeURL string
	token     string
	ack       *wire.Acker

	// wmu orders every write to a socket: data, acks, pings, close.
	// Taken before mu.
	wmu sync.Mutex

	mu       sync.Mutex
	cond     *sync.Cond
	sock     socket // nil while resuming
	recs     []conn.Record
	rest     []byte // what Read has left of a record
	err      error  // what ReadRecord returns once recs drain
	got      int64  // down bytes received
	lastAt   time.Time
	out      []byte // up bytes the relay hasn't acknowledged
	outBase  int64  // the offset of out[0]
	closed   bool
	resuming bool
	durable  bool  // acks wait for Kept
	kept     int64 // the last offset acked as kept
	resumed  bool  // it picked up a hold rather than opening one
	onEnd    func(error)
	ended    bool          // onEnd has been called, or never will be (detached)
	poke     chan struct{} // Wake: try resuming now
	pong     chan struct{}
	pinging  bool        // a Wake's ping is waiting for its pong
	note     *resumeNote // shown once the replay reaches it
}

// A resumeNote waits for the down bytes the relay had at resume time.
type resumeNote struct {
	at   int64
	away time.Duration
}

var (
	linksMu sync.Mutex
	links   = map[*Link]bool{}
)

// Options say how Dial reaches a world.
type Options struct {
	TLS bool // the world file asks for TLS
	// Durable links ack only what Kept says the page has saved, so the
	// relay keeps the rest for a fresh tab. Otherwise a link acks what it
	// receives.
	Durable bool
	// Resume picks up this held connection (a fresh tab) instead of
	// opening a new one.
	Resume *Held
}

// Held is a held connection a page saved: its token, and the offset just
// past the last line it kept.
type Held struct {
	Token string
	Kept  int64
}

// Dial reaches addr (host:port) through the relay at relayURL (ws:// or
// wss://). It returns once the relay has reached the world, or resumed
// the held connection, so a refusal is a dial error; a hold the relay no
// longer has is ErrLetGo.
func Dial(ctx context.Context, relayURL, addr string, o Options) (*Link, error) {
	if o.Resume != nil {
		return resumeLink(ctx, openSocket, resumeTarget(relayURL), *o.Resume, o.Durable)
	}
	u, err := target(relayURL, addr, o.TLS)
	if err != nil {
		return nil, err
	}
	return dialLink(ctx, openSocket, u, resumeTarget(relayURL), o.Durable)
}

// newLink is a link over s, registered, reading.
func newLink(open opener, resumeURL, token string, s socket, durable bool) *Link {
	l := &Link{open: open, resumeURL: resumeURL, token: token, sock: s, durable: durable,
		poke: make(chan struct{}, 1), pong: make(chan struct{}, 1)}
	l.cond = sync.NewCond(&l.mu)
	if !durable {
		l.ack = wire.NewAcker(0, l.sendAck)
	}
	return l
}

func (l *Link) start(s socket) {
	linksMu.Lock()
	links[l] = true
	linksMu.Unlock()
	go l.readFrom(s)
}

// resumeLink picks up held on a socket to resumeURL.
func resumeLink(ctx context.Context, open opener, resumeURL string, held Held, durable bool) (*Link, error) {
	s, err := open(ctx, resumeURL)
	if err != nil {
		return nil, err
	}
	if err := s.Write(ctx, websocket.MessageText, wire.Encode(wire.Msg{T: wire.Resume, Token: held.Token, Got: held.Kept})); err != nil {
		s.CloseNow()
		return nil, err
	}
	typ, b, err := s.Read(ctx)
	if err != nil {
		s.CloseNow()
		if websocket.CloseStatus(err) == wire.CodeGone {
			return nil, ErrLetGo
		}
		return nil, explain(err)
	}
	m, err := wire.Decode(b)
	if typ != websocket.MessageText || err != nil || m.T != wire.Resumed {
		s.CloseNow()
		return nil, errors.New(str.ConnRelayDropped())
	}
	l := newLink(open, resumeURL, held.Token, s, durable)
	l.got, l.outBase, l.resumed = held.Kept, m.Got, true
	if l.ack != nil {
		l.ack = wire.NewAcker(held.Kept, l.sendAck)
	}
	l.note = &resumeNote{at: m.At, away: time.Duration(m.Away) * time.Millisecond}
	l.noteLocked()
	l.start(s)
	return l, nil
}

// dialLink opens target and waits for the relay's open message. A refusal
// is a dial error. resumeURL is where it resumes.
func dialLink(ctx context.Context, open opener, target, resumeURL string, durable bool) (*Link, error) {
	s, err := open(ctx, target)
	if err != nil {
		return nil, err
	}
	typ, b, err := s.Read(ctx)
	if err != nil {
		s.CloseNow()
		if st := websocket.CloseStatus(err); st == websocket.StatusNormalClosure || st == websocket.StatusGoingAway {
			return nil, errors.New(str.ConnRelayDropped()) // closed without saying why
		}
		return nil, explain(err)
	}
	m, err := wire.Decode(b)
	if typ != websocket.MessageText || err != nil || m.T != wire.Open || m.Token == "" {
		s.CloseNow()
		return nil, errors.New(str.ConnRelayDropped())
	}
	l := newLink(open, resumeURL, m.Token, s, durable)
	l.start(s)
	return l, nil
}

// Token is the relay's name for the held connection.
func (l *Link) Token() string { return l.token }

// Resumed reports whether Dial picked up a held connection rather than
// opening a new one.
func (l *Link) Resumed() bool { return l.resumed }

// OnEnd has f called once when the link ends for good (closed, let go,
// replaced, the world gone), but not when DetachAll leaves it for
// another tab.
func (l *Link) OnEnd(f func(error)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onEnd = f
}

// Kept says the page has saved everything up to off: a durable link acks
// it, so the relay can forget it.
func (l *Link) Kept(off int64) {
	l.mu.Lock()
	if !l.durable || l.closed || off <= l.kept {
		l.mu.Unlock()
		return
	}
	l.kept = off
	l.mu.Unlock()
	l.sendAck(off)
}

// endLocked calls onEnd, once. l.mu is held.
func (l *Link) endLocked(err error) {
	if l.ended {
		return
	}
	l.ended = true
	if f := l.onEnd; f != nil {
		go f(err)
	}
}

// readFrom takes what the relay sends on s until s dies.
func (l *Link) readFrom(s socket) {
	for {
		typ, b, err := s.Read(context.Background())
		if err != nil {
			l.lost(s, err)
			return
		}
		if typ == websocket.MessageBinary {
			at, p, err := wire.ReadDown(b)
			if err != nil {
				continue
			}
			l.mu.Lock()
			l.got += int64(len(p))
			l.recs = append(l.recs, conn.Record{Data: p, At: at, End: l.got})
			l.lastAt = at
			l.noteLocked()
			l.cond.Broadcast()
			l.mu.Unlock()
			if l.ack != nil {
				l.ack.Received(len(p))
			}
			continue
		}
		m, err := wire.Decode(b)
		if err != nil {
			continue
		}
		switch m.T {
		case wire.Ack:
			l.mu.Lock()
			if n := m.Got - l.outBase; n > 0 && n <= int64(len(l.out)) {
				l.out, l.outBase = l.out[n:], m.Got
			}
			l.mu.Unlock()
		case wire.Missed:
			l.mu.Lock()
			rec := conn.Record{At: l.lastAt, Gap: true}
			if m.Lines > 0 {
				rec.Note = str.ConnMissed(m.Lines)
			}
			l.recs = append(l.recs, rec)
			l.got = max(l.got, m.At)
			l.noteLocked()
			l.cond.Broadcast()
			l.mu.Unlock()
			if l.ack != nil {
				l.ack.Skip(m.At)
			}
		case wire.Pong:
			select {
			case l.pong <- struct{}{}:
			default:
			}
		case wire.End:
			var err error = io.EOF
			if m.Why != "" {
				err = errors.New(m.Why) // the relay's reason, as Go's net errors say it
			}
			l.fail(err)
			l.bye(s) // nobody else will: a session doesn't close a conn the world closed
			return
		}
	}
}

// noteLocked queues the resumed note once the replay has caught up.
// l.mu is held.
func (l *Link) noteLocked() {
	if l.note != nil && l.got >= l.note.at {
		l.recs = append(l.recs, conn.Record{Note: str.ConnResumed(l.note.away.Round(time.Second)), At: time.Now()})
		l.note = nil
	}
}

// lost handles s dying: resume, unless the link is done with it.
func (l *Link) lost(s socket, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.sock != s || l.closed || l.err != nil {
		return
	}
	l.sock = nil
	switch websocket.CloseStatus(err) {
	case wire.CodeReplaced:
		l.failLocked(errReplaced)
		return
	case wire.CodeGone:
		l.failLocked(ErrLetGo)
		return
	}
	if !l.resuming {
		l.resuming = true
		go l.resume()
	}
}

// resume tries to pick the held connection up on a new socket until it
// does, the relay says it's gone, or the link is closed.
func (l *Link) resume() {
	start := time.Now()
	told := false
	for try := 0; ; try++ {
		select {
		case <-time.After(retryDelays[min(try, len(retryDelays)-1)]):
		case <-l.poke:
		}
		l.mu.Lock()
		closed := l.closed || l.err != nil
		l.mu.Unlock()
		if closed {
			return
		}
		s, m, err := l.tryResume()
		switch {
		case err == nil:
			l.attach(s, m)
			return
		case websocket.CloseStatus(err) == wire.CodeGone:
			l.fail(ErrLetGo)
			return
		}
		if !told && time.Since(start) >= lostAfter {
			told = true
			l.mu.Lock()
			l.recs = append(l.recs, conn.Record{Note: str.ConnRelayLost(), At: time.Now()})
			l.cond.Broadcast()
			l.mu.Unlock()
		}
	}
}

// tryResume opens a socket and asks for the held connection.
func (l *Link) tryResume() (socket, wire.Msg, error) {
	ctx, cancel := context.WithTimeout(context.Background(), resumeTimeout)
	defer cancel()
	s, err := l.open(ctx, l.resumeURL)
	if err != nil {
		return nil, wire.Msg{}, err
	}
	l.mu.Lock()
	got := l.got
	l.mu.Unlock()
	if err := s.Write(ctx, websocket.MessageText, wire.Encode(wire.Msg{T: wire.Resume, Token: l.token, Got: got})); err != nil {
		s.CloseNow()
		return nil, wire.Msg{}, err
	}
	typ, b, err := s.Read(ctx)
	if err != nil {
		s.CloseNow()
		return nil, wire.Msg{}, err
	}
	m, err := wire.Decode(b)
	if typ != websocket.MessageText || err != nil || m.T != wire.Resumed {
		s.CloseNow()
		return nil, wire.Msg{}, errors.New(str.ConnRelayDropped())
	}
	return s, m, nil
}

// attach carries on over s, which the relay resumed with m: resend what
// it hadn't written to the world.
func (l *Link) attach(s socket, m wire.Msg) {
	l.wmu.Lock()
	defer l.wmu.Unlock()
	l.mu.Lock()
	if l.closed || l.err != nil {
		l.mu.Unlock()
		s.Close(websocket.StatusNormalClosure, "")
		return
	}
	if n := m.Got - l.outBase; n > 0 && n <= int64(len(l.out)) {
		l.out, l.outBase = l.out[n:], m.Got
	}
	resend := append([]byte(nil), l.out...)
	l.sock = s
	l.resuming = false
	l.note = &resumeNote{at: m.At, away: time.Duration(m.Away) * time.Millisecond}
	l.noteLocked()
	l.cond.Broadcast()
	l.mu.Unlock()
	go l.readFrom(s)
	sendData(s, resend)
}

// sendData writes p to s in frames of at most maxFrame. A failure shows
// up as the socket dying.
func sendData(s socket, p []byte) {
	for len(p) > 0 {
		n := min(len(p), maxFrame)
		if s.Write(context.Background(), websocket.MessageBinary, p[:n]) != nil {
			return
		}
		p = p[n:]
	}
}

// bye tells the relay, over s, to let the world go, and closes s.
func (l *Link) bye(s socket) {
	l.wmu.Lock()
	defer l.wmu.Unlock()
	l.mu.Lock()
	if l.sock == s {
		l.sock = nil
	}
	l.mu.Unlock()
	s.Write(context.Background(), websocket.MessageText, wire.Encode(wire.Msg{T: wire.Close}))
	s.Close(websocket.StatusNormalClosure, "")
	if l.ack != nil {
		l.ack.Stop()
	}
}

// fail ends the link with err once what's queued has been read.
func (l *Link) fail(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failLocked(err)
}

func (l *Link) failLocked(err error) {
	if l.err == nil {
		l.err = err
	}
	l.endLocked(l.err)
	l.cond.Broadcast()
	linksMu.Lock()
	delete(links, l)
	linksMu.Unlock()
}

func (l *Link) sendAck(got int64) {
	l.wmu.Lock()
	defer l.wmu.Unlock()
	l.mu.Lock()
	s := l.sock
	l.mu.Unlock()
	if s != nil {
		s.Write(context.Background(), websocket.MessageText, wire.Encode(wire.Msg{T: wire.Ack, Got: got}))
	}
}

func (l *Link) ReadRecord() (conn.Record, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for len(l.recs) == 0 && l.err == nil {
		l.cond.Wait()
	}
	if len(l.recs) > 0 {
		r := l.recs[0]
		l.recs = l.recs[1:]
		return r, nil
	}
	return conn.Record{}, l.err
}

// Read is ReadRecord's bytes, notes left out.
func (l *Link) Read(p []byte) (int, error) {
	for len(l.rest) == 0 {
		r, err := l.ReadRecord()
		if err != nil {
			return 0, err
		}
		l.rest = r.Data
	}
	n := copy(p, l.rest)
	l.rest = l.rest[n:]
	return n, nil
}

// Write sends p, or keeps it to send once the link has resumed. It fails
// only once the link has ended.
func (l *Link) Write(p []byte) (int, error) {
	l.wmu.Lock()
	defer l.wmu.Unlock()
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return 0, net.ErrClosed
	}
	if l.err != nil {
		err := l.err
		l.mu.Unlock()
		if err == io.EOF {
			err = net.ErrClosed
		}
		return 0, err
	}
	l.out = append(l.out, p...)
	s := l.sock
	l.mu.Unlock()
	if s != nil {
		sendData(s, p)
	}
	return len(p), nil
}

// Close tells the relay to let the world go, and stops resuming.
func (l *Link) Close() error {
	l.wmu.Lock()
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		l.wmu.Unlock()
		return nil
	}
	l.closed = true
	if l.err == nil {
		l.err = net.ErrClosed
	}
	s, got := l.sock, l.got
	l.sock = nil
	l.endLocked(net.ErrClosed)
	l.cond.Broadcast()
	l.mu.Unlock()
	bye := wire.Encode(wire.Msg{T: wire.Close})
	if s != nil {
		s.Write(context.Background(), websocket.MessageText, bye)
		s.Close(websocket.StatusNormalClosure, "")
	}
	l.wmu.Unlock()
	if l.ack != nil {
		l.ack.Stop()
	}
	linksMu.Lock()
	delete(links, l)
	linksMu.Unlock()
	select {
	case l.poke <- struct{}{}:
	default:
	}
	if s == nil {
		// Down: say close on a socket of its own, or the relay holds the
		// world for the whole hold.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), resumeTimeout)
			defer cancel()
			s, err := l.open(ctx, l.resumeURL)
			if err != nil {
				return
			}
			s.Write(ctx, websocket.MessageText, wire.Encode(wire.Msg{T: wire.Resume, Token: l.token, Got: got}))
			s.Write(ctx, websocket.MessageText, bye)
			s.Close(websocket.StatusNormalClosure, "")
		}()
	}
	return nil
}

// ping checks s answers within wakeWait, and drops it if not. One runs
// at a time, so two wakes can't take each other's pong.
func (l *Link) ping(s socket) {
	l.mu.Lock()
	if l.pinging {
		l.mu.Unlock()
		return
	}
	l.pinging = true
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		l.pinging = false
		l.mu.Unlock()
	}()
	select {
	case <-l.pong:
	default:
	}
	l.wmu.Lock()
	err := s.Write(context.Background(), websocket.MessageText, wire.Encode(wire.Msg{T: wire.Ping}))
	l.wmu.Unlock()
	if err == nil {
		select {
		case <-l.pong:
			return
		case <-time.After(wakeWait):
		}
	}
	s.CloseNow() // readFrom sees it die, and resumes
}

// Wake checks every link's socket (the page came back into view, when a
// suspended tab's socket can look open but be dead), and has any that's
// waiting to resume try now.
func Wake() {
	for _, l := range live() {
		l.mu.Lock()
		s := l.sock
		l.mu.Unlock()
		if s != nil {
			go l.ping(s)
			continue
		}
		select {
		case l.poke <- struct{}{}:
		default:
		}
	}
}

// errDetached is how a link left to another tab ends. Detached tells a
// session it isn't a disconnect: the other tab carries on.
type errDetached struct{}

func (errDetached) Error() string  { return "left to another tab" } //str:ok never shown: sessions say nothing for it
func (errDetached) Detached() bool { return true }

// DetachAll leaves every link's held connection to another tab: its
// socket closes without a word to the relay, and it stops resuming and
// acking. Its conn ends (errDetached) without OnEnd.
func DetachAll() {
	for _, l := range live() {
		l.wmu.Lock()
		l.mu.Lock()
		l.closed, l.ended = true, true
		if l.err == nil {
			l.err = errDetached{}
		}
		s := l.sock
		l.sock = nil
		l.cond.Broadcast()
		l.mu.Unlock()
		l.wmu.Unlock()
		if l.ack != nil {
			l.ack.Stop()
		}
		linksMu.Lock()
		delete(links, l)
		linksMu.Unlock()
		select {
		case l.poke <- struct{}{}:
		default:
		}
		if s != nil {
			s.CloseNow()
		}
	}
}

// CloseAll closes every link: the relay lets their worlds go now.
func CloseAll() {
	for _, l := range live() {
		l.Close()
	}
}

// Tokens are the live links' tokens.
func Tokens() []string {
	var out []string
	for _, l := range live() {
		out = append(out, l.token)
	}
	return out
}

// DropAll kills every link's socket without a word to the relay, as a
// dying network would. Tests use it; the links resume.
func DropAll() {
	for _, l := range live() {
		l.mu.Lock()
		s := l.sock
		l.mu.Unlock()
		if s != nil {
			s.CloseNow()
		}
	}
}

func live() []*Link {
	linksMu.Lock()
	defer linksMu.Unlock()
	out := make([]*Link, 0, len(links))
	for l := range links {
		out = append(out, l)
	}
	return out
}

func (l *Link) LocalAddr() net.Addr                { return pageAddr{} }
func (l *Link) RemoteAddr() net.Addr               { return pageAddr{} }
func (l *Link) SetDeadline(t time.Time) error      { return nil }
func (l *Link) SetReadDeadline(t time.Time) error  { return nil }
func (l *Link) SetWriteDeadline(t time.Time) error { return nil }

type pageAddr struct{}

func (pageAddr) Network() string { return "websocket" } //str:ok
func (pageAddr) String() string  { return "websocket" } //str:ok
