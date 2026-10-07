//go:build js

package main

import (
	"errors"
	"syscall/js"
)

// download hands bytes to the page, which offers them as a file.
func download(name string, data []byte) error {
	u := js.Global().Get("Uint8Array").New(len(data)) //str:ok
	js.CopyBytesToJS(u, data)
	js.Global().Call("kilnDownload", name, u) //str:ok
	return nil
}

// pickFile asks the page for a file and waits; nil, nil when the user
// cancels.
func pickFile() ([]byte, error) {
	type result struct {
		b   []byte
		err error
	}
	ch := make(chan result, 1)
	ok := js.FuncOf(func(_ js.Value, a []js.Value) any {
		if a[0].IsNull() {
			ch <- result{}
			return nil
		}
		b := make([]byte, a[0].Length())
		js.CopyBytesToGo(b, a[0])
		ch <- result{b: b}
		return nil
	})
	fail := js.FuncOf(func(_ js.Value, a []js.Value) any {
		ch <- result{err: errors.New(a[0].Call("toString").String())} //str:ok
		return nil
	})
	defer ok.Release()
	defer fail.Release()
	js.Global().Call("kilnPickFile").Call("then", ok, fail) //str:ok
	r := <-ch
	return r.b, r.err
}
