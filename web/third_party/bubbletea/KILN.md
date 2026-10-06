# Kiln's copy of Bubble Tea

This is Bubble Tea v2.0.10, unmodified apart from one added file, `tty_js.go`, which gives the `js` build the TTY and signal hooks it lacks (no TTY, resizes arrive as `WindowSizeMsg`, no suspend). Kiln's browser build (`web/`, GOOS=js GOARCH=wasm on xterm.js) needs it because upstream doesn't build for `js/wasm` yet. It's wired in by a `replace` in `web/go.mod` only, never the root module, and goes away once upstream builds for `js/wasm`. Test files, testdata and the Taskfile were left out of the copy.
