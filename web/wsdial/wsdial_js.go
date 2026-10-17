//go:build js

package wsdial

import (
	"context"
	"errors"
	"net"
	"sync"
	"syscall/js"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/str"
)

// pageSocket is the page's WebSocket as a socket. Writes don't wait for
// the browser to send (MUCK traffic is small).
type pageSocket struct {
	ws    js.Value
	funcs []js.Func
	wake  chan struct{} // a message or the close came

	mu   sync.Mutex
	msgs []message
	err  error // the close, once it comes
}

type message struct {
	typ  websocket.MessageType
	data []byte
}

// openSocket opens a WebSocket to url and waits for it to open.
func openSocket(ctx context.Context, url string) (socket, error) {
	s := &pageSocket{ws: js.Global().Get("WebSocket").New(url), wake: make(chan struct{}, 1)} //str:ok
	s.ws.Set("binaryType", "arraybuffer")                                                     //str:ok
	opened := make(chan bool, 1)
	s.on("open", func(js.Value) { opened <- true }) //str:ok
	s.on("message", func(e js.Value) {              //str:ok
		var m message
		if d := e.Get("data"); d.Type() == js.TypeString { //str:ok
			m = message{websocket.MessageText, []byte(d.String())}
		} else {
			b := js.Global().Get("Uint8Array").New(d) //str:ok
			m = message{websocket.MessageBinary, make([]byte, b.Length())}
			js.CopyBytesToGo(m.data, b)
		}
		s.mu.Lock()
		s.msgs = append(s.msgs, m)
		s.mu.Unlock()
		s.poke()
	})
	// An error event is always followed by close.
	s.on("close", func(e js.Value) { //str:ok
		s.mu.Lock()
		if s.err == nil {
			s.err = websocket.CloseError{Code: websocket.StatusCode(e.Get("code").Int()), Reason: e.Get("reason").String()} //str:ok
		}
		s.mu.Unlock()
		s.poke()
		select {
		case opened <- false:
		default:
		}
		s.release()
	})
	select {
	case ok := <-opened:
		if !ok {
			return nil, errors.New(str.ConnRelayDown())
		}
		return s, nil
	case <-ctx.Done():
		s.CloseNow()
		return nil, ctx.Err()
	}
}

func (s *pageSocket) on(event string, f func(js.Value)) {
	jf := js.FuncOf(func(_ js.Value, a []js.Value) any { f(a[0]); return nil })
	s.funcs = append(s.funcs, jf)
	s.ws.Call("addEventListener", event, jf) //str:ok
}

func (s *pageSocket) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// release frees the event handlers once the socket has closed, when no
// more events can come. The close handler is still running, so it waits
// for that to return.
func (s *pageSocket) release() {
	funcs := s.funcs
	go func() {
		for _, f := range funcs {
			f.Release()
		}
	}()
}

func (s *pageSocket) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	for {
		s.mu.Lock()
		if len(s.msgs) > 0 {
			m := s.msgs[0]
			s.msgs = s.msgs[1:]
			s.mu.Unlock()
			return m.typ, m.data, nil
		}
		err := s.err
		s.mu.Unlock()
		if err != nil {
			return 0, nil, err
		}
		select {
		case <-s.wake:
		case <-ctx.Done():
			return 0, nil, ctx.Err()
		}
	}
}

func (s *pageSocket) Write(_ context.Context, typ websocket.MessageType, p []byte) error {
	s.mu.Lock()
	err := s.err
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if typ == websocket.MessageText {
		s.ws.Call("send", string(p)) //str:ok
		return nil
	}
	b := js.Global().Get("Uint8Array").New(len(p)) //str:ok
	js.CopyBytesToJS(b, p)
	s.ws.Call("send", b) //str:ok
	return nil
}

func (s *pageSocket) Close(code websocket.StatusCode, reason string) error {
	s.ws.Call("close", int(code), reason) //str:ok
	return nil
}

// CloseNow closes s at once, as far as Kiln is concerned: Read returns
// now, without waiting for the browser's close event.
func (s *pageSocket) CloseNow() error {
	s.mu.Lock()
	if s.err == nil {
		s.err = net.ErrClosed
	}
	s.mu.Unlock()
	s.poke()
	s.ws.Call("close") //str:ok
	return nil
}
