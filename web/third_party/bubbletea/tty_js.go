//go:build js

package tea

// Added for Kiln's browser build (web/): GOOS=js has no TTY. Input
// arrives already raw from xterm.js, resizes arrive as WindowSizeMsg, and
// a browser tab can't be suspended. Shaped like tty_windows.go and
// signals_windows.go.

func (p *Program) initInput() error { return nil }

func (p *Program) listenForResize(done chan struct{}) { close(done) }

const suspendSupported = false

func suspendProcess() {}
