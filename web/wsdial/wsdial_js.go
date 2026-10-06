//go:build js

package wsdial

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"syscall/js"
	"time"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/str"
)

// Dialer returns a conn.Options.DialContext that reaches addr through the
// relay at relayURL (ws:// or wss://), using the page's WebSocket.
// coder/websocket's browser build drops the close code once the socket
// closes, and the code is how the relay says why it refused.
func Dialer(relayURL string) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, _, addr string) (net.Conn, error) {
		u, err := target(relayURL, addr)
		if err != nil {
			return nil, err
		}
		return dial(ctx, u)
	}
}

// pageConn is a browser WebSocket as a net.Conn. Writes don't wait for
// the browser to send (MUCK traffic is small), and deadlines are ignored:
// nothing in Kiln's connection sets one it relies on.
type pageConn struct {
	ws    js.Value
	funcs []js.Func

	mu     sync.Mutex
	cond   *sync.Cond
	open   bool
	buf    []byte
	closed bool
	err    error // what Read and Write return once closed and drained
}

func dial(ctx context.Context, u string) (net.Conn, error) {
	c := &pageConn{ws: js.Global().Get("WebSocket").New(u)} //str:ok
	c.cond = sync.NewCond(&c.mu)
	c.ws.Set("binaryType", "arraybuffer") //str:ok
	settled := make(chan struct{})
	var once sync.Once
	settle := func() { once.Do(func() { close(settled) }) }
	c.on("open", func(js.Value) { //str:ok
		c.mu.Lock()
		c.open = true
		c.mu.Unlock()
		settle()
	})
	c.on("message", func(e js.Value) { //str:ok
		b := js.Global().Get("Uint8Array").New(e.Get("data")) //str:ok
		p := make([]byte, b.Length())
		js.CopyBytesToGo(p, b)
		c.mu.Lock()
		c.buf = append(c.buf, p...)
		c.mu.Unlock()
		c.cond.Broadcast()
	})
	// An error event is always followed by close, which settles things.
	c.on("close", func(e js.Value) { //str:ok
		code := websocket.StatusCode(e.Get("code").Int())                                  //str:ok
		var err error = websocket.CloseError{Code: code, Reason: e.Get("reason").String()} //str:ok
		if code == websocket.StatusNormalClosure || code == websocket.StatusGoingAway {
			err = io.EOF
		}
		c.shut(explain(err))
		c.release()
		settle()
	})
	select {
	case <-settled:
	case <-ctx.Done():
		c.Close()
		return nil, ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.open {
		return nil, errors.New(str.ConnRelayDown())
	}
	return c, nil
}

func (c *pageConn) on(event string, f func(js.Value)) {
	jf := js.FuncOf(func(_ js.Value, a []js.Value) any { f(a[0]); return nil })
	c.funcs = append(c.funcs, jf)
	c.ws.Call("addEventListener", event, jf) //str:ok
}

// release frees the event handlers once the socket has closed, when no
// more events can come. The close handler is still running, so it waits
// for that to return.
func (c *pageConn) release() {
	funcs := c.funcs
	go func() {
		for _, f := range funcs {
			f.Release()
		}
	}()
}

// shut marks the conn closed with err, unless it already was.
func (c *pageConn) shut(err error) {
	c.mu.Lock()
	if !c.closed {
		c.closed, c.err = true, err
	}
	c.mu.Unlock()
	c.cond.Broadcast()
}

func (c *pageConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.buf) == 0 && !c.closed {
		c.cond.Wait()
	}
	if len(c.buf) > 0 {
		n := copy(p, c.buf)
		c.buf = c.buf[n:]
		return n, nil
	}
	return 0, c.err
}

func (c *pageConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	closed, err := c.closed, c.err
	c.mu.Unlock()
	if closed {
		if err == io.EOF {
			err = net.ErrClosed
		}
		return 0, err
	}
	b := js.Global().Get("Uint8Array").New(len(p)) //str:ok
	js.CopyBytesToJS(b, p)
	c.ws.Call("send", b) //str:ok
	return len(p), nil
}

func (c *pageConn) Close() error {
	c.shut(net.ErrClosed)
	c.ws.Call("close") //str:ok
	return nil
}

func (c *pageConn) LocalAddr() net.Addr                { return pageAddr{} }
func (c *pageConn) RemoteAddr() net.Addr               { return pageAddr{} }
func (c *pageConn) SetDeadline(t time.Time) error      { return nil }
func (c *pageConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *pageConn) SetWriteDeadline(t time.Time) error { return nil }

type pageAddr struct{}

func (pageAddr) Network() string { return "websocket" } //str:ok
func (pageAddr) String() string  { return "websocket" } //str:ok
