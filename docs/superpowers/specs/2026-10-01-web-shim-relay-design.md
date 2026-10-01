# Web Kiln, part 1: Bubble Tea in the browser, plus the relay

Epic #74 (web app), sub-projects 1 and 2 of 5. Desktop (#73) is deferred:
it becomes "the web build in a Wails shell" once the web build exists.

## Why

PFMuck newcomers should be able to open a link on the MUCK's site and be
playing in full Kiln, with PFMuck already added, with nothing to install.
This spec covers the foundation only: Kiln drawing into xterm.js in a
browser tab, and talking to a MUCK through a WebSocket-to-TCP relay. It
also covers a local dev loop, so none of this needs a deploy to try out.

## The five sub-projects (for context)

1. **Shim + bridge:** Bubble Tea builds for `js/wasm`; Kiln draws into
   xterm.js. *(this spec)*
2. **Relay:** `kiln-relay`, the allowlist, and an nginx recipe. *(this spec)*
3. **Storage:** config, logs and known_hosts saved across visits.
4. **DOM input:** a real `<textarea>` (spellcheck, emoji, IME, screen
   readers) with Kiln's history and multi-line, plus a real login form so
   the browser's password manager handles passwords.
5. **The PFMuck page:** the preset, a curated allowlist, browser
   notifications, and the `/kiln/` landing page.

Decisions already made for later parts: it's full Kiln with PFMuck added
ahead of time, not a single-world client. The relay allows a curated list
of worlds. Kiln never stores passwords in the browser (the browser's
password manager does, through sub-project 4's login form; otherwise Kiln
asks each time).

## Done means

- `kiln-relay -dev` on a Mac serves the page at `http://localhost:8080/kiln/`.
  Opening it shows Kiln in xterm.js with a dev preset, and connecting to a
  real MUCK (PFMuck, over the internet) works: a TLS handshake inside the
  browser, reading and sending lines, resizing, mouse, and links.
- `go test ./...` covers the relay and the new dial hook. A
  `GOOS=js GOARCH=wasm` test run under node covers the bridge.
- The `go install github.com/latrani/Kiln/cmd/kiln@latest` install path
  (README) still works, unchanged.

## Layout

```
cmd/kiln-relay/          relay + dev server (root module)
internal/relay/          allowlist, WebSocket↔TCP pipe, limits
web/                     its own Go module: github.com/latrani/Kiln/web
  go.mod                 replace bubbletea → third_party copy; replace Kiln → ../
  cmd/kiln-web/          main for GOOS=js: builds ui.Deps, starts the program
  bridge/                syscall/js glue: input pipe, output writer, resize
  wsdial/                net.Conn over a browser WebSocket to the relay
  static/                index.html, kiln.js, memfs.js, vendored xterm.js
  preset.example/        a dev preset (config dir), example world only
```

**Why `web/` is its own module:** the Bubble Tea fix needs a `replace`
directive, and `go install module@version` refuses modules with
`replace`. Keeping the replace in `web/go.mod` leaves the root module, and
the README's install line, untouched. Go's `internal/` rule is by import
path, so `github.com/latrani/Kiln/web/...` can still import
`github.com/latrani/Kiln/internal/...`.

## 1. The Bubble Tea shim

On `main` (a14fe59 and now), `GOOS=js GOARCH=wasm go build ./internal/ui`
fails only in Bubble Tea v2.0.10 itself: `listenForResize`, `initInput`,
`suspendSupported` and `suspendProcess` have no `js` version. The Windows
file already has the shape we need.

- **Fix:** one file, `tty_js.go` (`//go:build js`), in a copy of
  `charm.land/bubbletea/v2` v2.0.10:
  - `initInput` returns nil (no raw mode; xterm.js already sends raw bytes),
  - `listenForResize` returns at once (resizes arrive as messages, below),
  - `suspendSupported = false`, `suspendProcess() {}`.
- **Where the copy lives:** `web/third_party/bubbletea`, a copy of
  v2.0.10's non-test sources (428K, MIT, LICENSE kept) plus `tty_js.go`,
  used by `web/go.mod`'s directory `replace`. That's in-repo rather than a
  GitHub fork because a directory replace with a matching module path is
  certain to work, and it means nothing gets created outside the repo.
  Offering the patch upstream is Indi's call later; once upstream ships
  it, the copy and the `replace` go away.
- No other package in the import graph fails for `js/wasm` (per #74's
  findings and today's build). `fsnotify` compiles, and the web build
  never calls `config.Watch`.

## 2. The bridge (Go ⇄ xterm.js)

`web/bridge` is plain Go (no `syscall/js`, so its tests run natively);
`cmd/kiln-web` wraps it in a global `kiln` object for JS. Startup, from `kiln.js`:

1. Fill `globalThis.fs` (see §4), load `kiln.wasm` with Go's
   `wasm_exec.js` (copied from `$(go env GOROOT)/lib/wasm` at build time).
2. Create the xterm.js `Terminal` (with addons fit, webgl and clipboard;
   no web-links addon, since Kiln opens links itself through `OpenURL`) and pass its size to `kiln.start(cols, rows, relayURL, write)`.
3. `term.onData(d => kiln.input(d))`, `term.onBinary` the same,
   `term.onResize(({cols, rows}) => kiln.resize(cols, rows))`.

Go side:

- **Input:** `kiln.input` copies its string into a buffered channel, and
  a goroutine drains that channel into an `io.Pipe` used as `tea.WithInput`. JS callbacks
  never block.
- **Output:** a writer passed to `tea.WithOutput` calls `write(Uint8Array)`.
- **Size:** `tea.WithWindowSize(cols, rows)` at start; `kiln.resize` calls
  `p.Send(tea.WindowSizeMsg{...})`.
- **Program options:** `tea.WithEnvironment([]string{"TERM=xterm-256color"})`
  and `tea.WithColorProfile(colorprofile.TrueColor)`, since there's no
  real environment to detect from.
- **Appearance:** Kiln already asks with OSC 11 and falls back when
  nobody answers (mosh does that today). Whatever xterm.js does here,
  Kiln stays correct. Checked by hand in the dev loop.

`cmd/kiln-web` builds `ui.Deps` like `cmd/kiln`'s `tui` does, with these
differences:

| Dep | Web build (this spec) |
|---|---|
| `ConfigDir` | `/home/kiln/.config/kiln` in memfs |
| `LogRoot` | `/home/kiln/.local/share/kiln/logs` in memfs (history lasts the visit until sub-project 3) |
| `Load` | `config.Load` unchanged (memfs makes the files real) |
| `Dial` | `conn.Dial` with `DialContext: wsdial.Dialer(relayURL)` |
| `Password` | "not found" (Kiln asks; the login form is sub-project 4) |
| `SavePassword`, `DeletePassword` | nil (never offered) |
| `Changes` | nil (no hot reload) |
| `OpenURL` | `window.open(url, "_blank", "noopener")` |
| `Tmux`, `Remote` | false |

## 3. Dialing through the relay

- **`conn.Options` gets `DialContext func(ctx, network, addr string) (net.Conn, error)`.**
  nil means today's `net.Dialer`. `conn.Dial` always dials the raw
  connection first, then, if TLS is on, wraps it with `tls.Client(raw, cfg)`
  and calls `HandshakeContext(ctx)`. Pinning and `PinMismatchError` behave
  as before. So **TLS still happens in Kiln**, and the relay only ever
  passes ciphertext.
- **`web/wsdial`:** opens `relayURL?host=H&port=P` with
  `github.com/coder/websocket` (which wraps the browser's WebSocket under
  `js/wasm`) and returns `websocket.NetConn(ctx, c, websocket.MessageBinary)`.
- **Close codes from the relay** become errors with catalog text:
  4403 → "this world isn't on the web relay's list", 4426 → "this world
  only works over TLS on the web", 4429 → "too many connections from your address", 4502 → "the relay
  couldn't reach this world: {reason}" (the reason is the relay's dial error).

## 4. Files in the browser: memfs

Go's `wasm_exec.js` expects the page to provide `globalThis.fs` (a
Node-style callback API) and ships only a stub that fails with ENOSYS.
`static/memfs.js` provides an in-memory one with the calls Go's `syscall`
for js uses: `open`, `close`, `read`, `write`, `fstat`, `stat`, `lstat`,
`mkdir`, `readdir`, `rename`, `unlink`, `rmdir`, `fsync`, `ftruncate`,
`chmod`, `fchmod`, `utimes`.

- On load, `kiln.js` fetches `preset/manifest.json` (a list of paths) and
  writes those files into memfs under the config dir.
- known_hosts (`/home/kiln/.local/share/kiln/known_hosts`) works too, but
  pins only last for the visit. Saving them is sub-project 3.
- **Why this matters beyond this spec:** if memfs holds up, sub-project 3
  shrinks from "put a storage interface behind config, logstore and
  known_hosts" to "save memfs to IndexedDB". Config edits from the
  add/edit forms would work through the same path. Treat this as the
  spike's second question, next to the shim.

## 5. The relay

`kiln-relay` (root module, `cmd/kiln-relay` + `internal/relay`):

- **Endpoint:** `GET /kiln/relay?host=H&port=P`, upgraded to WebSocket.
  Binary frames carry raw TCP bytes in both directions. When either side
  closes, the relay closes the other.
- **Allowlist:** a TOML file, reloaded when it changes (fsnotify, which
  Kiln already uses):
  ```toml
  [[world]]
  host = "muck.example.org"
  ports = [8888, 8899]
  ```
  The check uses the exact host and port the client asked for. Anything
  else is refused before dialing, with close code 4403.
- **TLS only, unless a world says otherwise:** so the relay's operator
  never sees anyone's plaintext (passwords included), each entry is
  TLS-only by default. Before dialing a TLS-only world, the relay reads
  the client's first frame and requires a TLS handshake record (first byte
  `0x16`). Anything else, or no frame within 15s, gets close code 4426
  and is never dialed. An
  entry can opt out with `plaintext = true`, meant for the operator's own
  MUCK, whose traffic they can already see. A plaintext world is dialed
  at once, without waiting for a frame: a telnet server talks first, so
  its client may send nothing. The client maps 4426 to catalog text: "this world
  only works over TLS on the web".
- **Origin:** `coder/websocket`'s default same-origin check, plus an
  optional `origins = [...]` list in the same file. Without it, another
  site could use the relay from its own page.
- **Limits:** at most 8 open connections per client IP; more get close
  code 4429. The IP comes from `X-Forwarded-For` only when the request
  comes from a trusted proxy (`trusted_proxies = ["127.0.0.1"]`). A dial times out after 15s. There's
  no idle timeout, because idling is normal on a MUCK.
- **Logs:** one line per connection open and close (client IP, world,
  bytes each way, how long). Never any payload.
- **Flags:** `-listen 127.0.0.1:7801`, `-allow PATH`, and `-dev`.
  `-dev` also serves `-web DIR` at `/kiln/` and `-preset DIR` at
  `/kiln/preset/` (which generates `manifest.json`), all on
  `localhost:8080`, all same-origin.
- **Production recipe** (documented, not deployed in this spec): nginx
  serves the built `static/` at `/kiln/` and proxies `/kiln/relay` to
  `127.0.0.1:7801` with `Upgrade`/`Connection` headers and
  `proxy_read_timeout` raised (e.g. `1d`); a systemd unit runs the relay.

## Strings

All text people read goes through `internal/str/locales/en.toml`:

- the relay's CLI and log lines go in `[relay]`,
- the client-side close-code errors go in `[conn]`,
- the dev page has no prose of its own (the terminal is the whole page).
  Landing-page text is sub-project 5's.

The `Strings:` trailers follow CLAUDE.md. `TestNoStrayStrings` and
`TestTestsReadTheCatalog` gain `web/` in the directories they walk.

## Testing (all local)

- **`internal/relay` (plain `go test`):** reuse the `fakeServer` /
  `selfSignedListener` patterns from `internal/conn/conn_test.go`, plus
  `httptest`. Cases:
  - bytes round-trip,
  - an unlisted host or port gets 4403 and is never dialed,
  - a non-TLS first frame to a TLS-only world gets 4426 and is never
    dialed, while a `plaintext = true` world accepts it,
  - an unreachable upstream gets 4502,
  - a wrong Origin is refused,
  - the 9th connection from one IP is refused,
  - XFF is ignored from an untrusted peer,
  - a TLS handshake through the relay matches the pin (the relay passes
    ciphertext through untouched),
  - the allowlist reloads.
- **`internal/conn`:** `DialContext` is used when set (over `net.Pipe`),
  and TLS through a custom dialer still pins.
- **`web/` (`GOOS=js GOARCH=wasm go test ./...`, run by Go's bundled
  `go_js_wasm_exec` under node):** the bridge runs a program on its pipe;
  input bytes reach `Update`, output reaches the write callback,
  and `resize` sends a `WindowSizeMsg`. The expected text comes from the
  catalog.
- **`static/memfs.js`:** `node --test` covers the calls listed in §4, and
  a wasm test runs `config.Load` against memfs.
- **Dev loop (by hand):** `go run ./cmd/kiln-relay -dev -allow dev-allow.toml
  -web web/static -preset web/preset.example` (after building `kiln.wasm`),
  then connect to PFMuck from the browser.
- **CI:** the strings workflow stays as is. The root's `go test ./...`
  skips `web/` (it's a separate module), so add a job that runs
  `cd web && GOOS=js GOARCH=wasm go test ./...` with node set up.

## Not in this spec

Saving storage across visits (3), the textarea and the login form (4),
browser notifications, the PFMuck preset, the curated allowlist, and an
actual deploy (5). Also: phones, playing offline, and Wails.

## Risks

- **The shim works for building but Bubble Tea misbehaves at runtime**
  (cancelreader, how it finds the terminal size). Bubble Tea gets plain
  readers and writers here, never a TTY, which is the mode its own tests use.
  If it misbehaves, fall back to the prior art in #74 (bubbweb).
- **memfs is missing a call Go needs.** You'd see ENOSYS from a specific
  call; add that call. Go's `fs_js.go` lists every call it makes.
- **coder/websocket under `js/wasm`:** `Ping` is a no-op and `Accept`
  errors; neither matters for a client.
