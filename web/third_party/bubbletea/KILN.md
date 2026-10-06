# Kiln's copy of Bubble Tea

This is Bubble Tea v2.0.10, unmodified apart from one added file, `tty_js.go`, which gives the `js` build the TTY and signal hooks it lacks (no TTY, resizes arrive as `WindowSizeMsg`, no suspend). Kiln's browser build (`web/`, GOOS=js GOARCH=wasm on xterm.js) needs it because upstream doesn't build for `js/wasm` yet. It's wired in by a `replace` in `web/go.mod` only, never the root module, and goes away once upstream builds for `js/wasm`. Test files, testdata and the Taskfile were left out of the copy.

When the root module moves to a new Bubble Tea, recopy upstream at that version, keep `tty_js.go`, and update this line and `web/go.mod`'s require to match (`web/bubbletea_test.go` checks the three agree):

upstream: charm.land/bubbletea/v2 v2.0.10
