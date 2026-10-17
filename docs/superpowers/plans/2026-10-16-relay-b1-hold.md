# Detachable relay B1: hold and resume in a surviving tab — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When the WebSocket between a tab and kiln-relay dies without the page saying "close", the relay keeps the MUCK connection, buffers what arrives, and the same tab picks it back up: no reconnect, no log-in, no missing or doubled lines.

**Architecture:** The relay↔page connection stops being a bare byte pipe and speaks a small protocol (`internal/relay/wire`): binary frames for data (down frames carry the time the bytes reached the relay), JSON text frames for control (open + token, resume, acks, missed, ping, end, close). The relay keeps each MUCK connection in a `held` object that outlives any one WebSocket. In the tab, `wsdial` returns a resumable `link` (a `net.Conn` that also reads timestamped records) that redials and resumes underneath, so `conn` and `session` see one unbroken connection. `conn` lines gain a time and a note flag so buffered lines land with the relay's times and the "resumed"/"missed" lines show in scrollback.

**Tech Stack:** Go (`coder/websocket`, `crypto/rand`, BurntSushi/toml), TypeScript page (`web/ui`), headless-Chrome smoke tests (`web/smoke`, `KILN_SMOKE=1`).

**Spec:** `docs/superpowers/specs/2026-10-16-detachable-relay-design.md`: "How it behaves" (Hold, Buffer, Resume, Hold runs out; not the short hold or "same browser" yet), "Settings" (hold and buffer keys), "Limits", "Left to the builder" (wire protocol, seam, dropping at line boundaries, token, noticing a dead WebSocket, relay logs, tests). B2 does fresh-tab resume (token in `persist`, telnet state, skipping log-in, handoff, sendBeacon short hold); C does Web Push.

## Global Constraints

- Every displayed string, relay log lines included, lives in `internal/str/locales/en.toml`; run `go generate ./internal/str`. Commits that touch locales end with a `Strings:` trailer listing keys.
- Tests build expected text from the catalog (`str.X()`, `mark`/`upTo`), never copies.
- Below 1.0, clean breaks: the page↔relay protocol changes without versioning (the relay serves the page).
- Desktop Kiln and the TUI behave exactly as before. Golden screens are never `-update`d.
- The relay never writes traffic or the buffer to disk, and its log lines carry no traffic content.
- Names in tests and docs: Kit, Rook, Ash, world `fm`.
- The README is not touched.

## Decisions this plan makes (from "Left to the builder")

- **Seam:** `conn.RecordReader` is an optional interface on the `net.Conn` a `DialContext` returns. `conn` uses it when present; desktop TCP/TLS never has it.
- **Offsets, not per-frame sequence numbers:** each side counts the data bytes it has received on the connection since it opened (`got`). Acks and resumes speak in those byte offsets. Frames are atomic for the receiver.
- **Ack cadence (both sides):** send `ack` once 64 KiB is unacknowledged or 1 s after the first unacknowledged byte, whichever is first.
- **Times:** a line's time is the relay arrival time of the chunk holding its line end. Session keeps entry times monotonic per session: an entry's time is `max(At, previous entry's time)`.
- **Dropping:** the relay only cuts at a `\n` outside any telnet command or subnegotiation, or, in plain data, after 64 KiB without one (a forced cut counts as a line).
- **B1 isn't deployable alone** (Indi deploys once the whole spec is done): until B2, a reload leaves its connections held for the full hold, counting against `max_per_ip`. No stopgap.
- **Notes the tab shows (catalog text, Indi may reword):** "resumed · away {away}", "missed {n} lines while away · the relay's buffer was full", "lost the web relay · trying to get the connection back" (only after 30 s of failed resumes), and the let-go error "the web relay let go of this connection".

## Review Focus

1. **A suspended iOS tab whose socket still looks open.** On `visibilitychange` (visible) or `pageshow`, the link pings and, with no pong in 3 s, drops the socket and resumes, instead of waiting out the relay's 50 s ping. Test in Task 6 (`TestWakeResumesASilentSocket`).
2. **Lines typed while the socket is dead** go to the MUCK exactly once after resume: none lost, none doubled. Test in Task 6 (`TestWritesWhileDownAreSentOnce`).
3. **The relay restarting** (a deploy) lets every hold go: the tab must say the relay let go and then follow the character's `reconnect` setting, not retry resuming forever. Test in Task 6 (`TestRelayRestartLetsGo`).
4. **Buffered lines older than the last entry** (a line typed during the resume window, a note) keep the log in order and don't break day dividers. Test in Task 5 (`TestEntryTimesNeverGoBackwards`).
5. **A drop never cuts inside a telnet sequence**, including a NAWS subnegotiation whose width byte is `0x0A`. Test in Task 3 (`TestCutterSkipsNewlinesInsideTelnet`).

---

### Task 1: The wire protocol package

**Files:**
- Create: `internal/relay/wire/wire.go`, `internal/relay/wire/wire_test.go`
- Modify: `internal/relay/server.go` (codes come from `wire`), `web/wsdial/wsdial.go` (drop the copied codes and `openFrame`), delete `TestCodesMatchRelay` from `web/wsdial/wsdial_test.go`

**Interfaces:**
- Produces, in package `wire` (imports only `coder/websocket`, `encoding/json`, `encoding/binary`, `time`; no server code):
  - Close codes `CodeNotListed 4403`, `CodeNoTLS 4426`, `CodeTooMany 4429`, `CodeUnreachable 4502`, `CodeBusy 4503`, `CodeBadCert 4526` (moved), plus new `CodeGone 4410` (no such hold: unknown token, expired, or closed) and `CodeReplaced 4409` (a newer attachment took this connection).
  - `type Msg struct { T string \`json:"t"\`; Token string \`json:"token,omitempty"\`; Got int64 \`json:"got,omitempty"\`; At int64 \`json:"at,omitempty"\`; Lines int \`json:"lines,omitempty"\`; Away int64 \`json:"away,omitempty"\`; Why string \`json:"why,omitempty"\` }` with `T` one of the constants `Open, Resume, Resumed, Ack, Missed, Ping, Pong, End, Close` (`"open"`, `"resume"`, …, all `//str:ok protocol token`).
    - relay→page: `open{token}`, `resumed{got, away (ms)}`, `ack{got}`, `missed{lines, at}`, `pong`, `end{at, why}`.
    - page→relay: `resume{token, got}`, `ack{got}`, `ping`, `close`.
  - `func Encode(m Msg) []byte`, `func Decode(b []byte) (Msg, error)`.
  - `func DownFrame(at time.Time, p []byte) []byte` (8-byte big-endian Unix milliseconds, then `p`), `func ReadDown(b []byte) (at time.Time, p []byte, err error)`.
  - `const AckBytes = 64 << 10`, `const AckEvery = time.Second`.

- [ ] **Step 1:** Write `TestDownFrameRoundTrip` (a time with millisecond precision and a 3-byte payload survive; `ReadDown` of a 7-byte frame errors) and `TestMsgRoundTrip` (each `T` with its fields survives `Encode`/`Decode`; zero fields are omitted from the JSON).
- [ ] **Step 2:** Run `go test ./internal/relay/wire` — FAIL (package missing).
- [ ] **Step 3:** Write the package. Point `relay` and `wsdial` at `wire.Code*`; delete their copies and `TestCodesMatchRelay`.
- [ ] **Step 4:** Run `go test ./internal/relay/... ./web/wsdial` — PASS. `GOOS=js GOARCH=wasm go build ./web/...` — builds.
- [ ] **Step 5:** Commit `relay: the page↔relay protocol gets its own package`.

### Task 2: Hold and buffer settings in relay.toml

**Files:**
- Modify: `internal/relay/config.go`, `config_test.go`, `internal/str/locales/en.toml`, `web/dev-relay.example.toml` (commented examples)

**Interfaces:**
- Produces: `Config.Hold string` (`toml:"hold"`, default `"24h"`), `Config.MaxBufferMB int` (`toml:"max_buffer_mb"`, default 4), `Config.MaxTotalBufferMB int` (`toml:"max_total_buffer_mb"`, default 256), and methods `(*Config).HoldFor() time.Duration`, `(*Config).BufferCap() int`, `(*Config).TotalBufferCap() int` (bytes) that apply the defaults.

- [ ] **Step 1:** Tests: `TestParseHoldAndBuffers` (unset → 24h, 4 MiB, 256 MiB; `hold = "90m"`, `max_buffer_mb = 1`, `max_total_buffer_mb = 8` read back). Extend `TestParseErrors`: `hold = "soon"` and `hold = "-1h"` and `hold = "0s"` → `str.RelayConfigBadHold(path, value)`; `max_buffer_mb = -1` → `str.RelayConfigBadLimit(path, "max_buffer_mb")`, same for `max_total_buffer_mb`.
- [ ] **Step 2:** `go test ./internal/relay` — FAIL.
- [ ] **Step 3:** Implement. Parse `hold` once in `ParseConfig` (keep the `time.Duration` unexported). New entry: `config_bad_hold = '{path}: hold {value:%q} isn''t a length of time · use one like "24h" or "90m"'`.
- [ ] **Step 4:** `go generate ./internal/str && go test ./internal/relay` — PASS.
- [ ] **Step 5:** Commit `relay: hold and buffer settings` with `Strings: relay.config_bad_hold (new)`.

### Task 3: The relay's buffer

Pure data structure; no sockets.

**Files:**
- Create: `internal/relay/buffer.go`, `internal/relay/buffer_test.go`

**Interfaces:**
- Produces:
  - `type buffer struct` holding the down bytes not yet acknowledged, from offset `base` to `end`, with each chunk's arrival time and the safe cut points.
  - `func (b *buffer) add(p []byte, at time.Time)`
  - `func (b *buffer) size() int` (bytes held)
  - `func (b *buffer) end() int64` (total down bytes ever added)
  - `func (b *buffer) ack(got int64)` (forget bytes and drop records below `got`)
  - `func (b *buffer) dropLine() bool` (drop the oldest whole line from the front; false if there's no safe cut in the buffer)
  - `func (b *buffer) next(pos int64) (missed int, from int64, at time.Time, p []byte, ok bool)`: what a writer at `pos` sends next. `missed` is the lines dropped at or after `pos` (so the writer sends `missed{lines, at: from}` first when it's > 0); `p` runs from `max(pos, base)` to the end of that chunk (a chunk is one `add`), stamped `at`. `ok` false when nothing is past `pos`.
  - `type cutter struct` tracking telnet state across chunks (data, IAC, option byte, SB, IAC-in-SB); `func (c *cutter) cuts(p []byte) []int` returns offsets in `p` just past each safe `\n`, plus a forced cut after 64 KiB in data state without one.

- [ ] **Step 1:** Tests in `buffer_test.go`:
  - `TestCutterSkipsNewlinesInsideTelnet`: `IAC SB NAWS 0 10 0 24 IAC SE` then `"hi\n"` gives one cut, after `hi\n`; a sequence split across two `cuts` calls gives the same.
  - `TestCutterForcesACutOnHugeLines`: 64 KiB + 10 bytes of `x` gives a cut at 64 KiB.
  - `TestNextResendsFromPos`: `add("a\n", t1)`, `add("b\n", t2)`; `next(0)` → `"a\n"` at t1; `next(2)` → `"b\n"` at t2; `next(4)` → not ok; `next(1)` → `"\n"` at t1.
  - `TestAckForgets`: after `ack(2)`, `size()` is 2 and `next(0)` returns `"b\n"` (the writer is behind `base` but nothing was dropped, so `missed` is 0).
  - `TestDropLineCountsMissed`: three lines, `dropLine()` twice; `next(0)` reports `missed 2`, `from` = start of the third line; `next(from)` reports `missed 0`; after `ack(from)` the drop records are gone.
  - `TestDropLineNeedsACut`: `add("prompt> ")` alone → `dropLine()` false.
- [ ] **Step 2:** `go test ./internal/relay -run 'Cutter|Next|Ack|DropLine'` — FAIL.
- [ ] **Step 3:** Implement. Keep drop records as `[]struct{ end int64; lines int }`, trimmed by `ack`.
- [ ] **Step 4:** Same command — PASS.
- [ ] **Step 5:** Commit `relay: a line-safe buffer for held connections`.

### Task 4: Held connections in the relay

**Files:**
- Create: `internal/relay/hold.go`, `internal/relay/hold_test.go`
- Modify: `internal/relay/server.go` (the open path hands the MUCK connection to a `held`; `pipe` and `keepAlive`'s close-on-timeout go), `server_test.go` (existing tests read the `open` message instead of the old bare frame), `internal/str/locales/en.toml`

**Interfaces:**
- Consumes: `wire` (Task 1), `Config.HoldFor/BufferCap/TotalBufferCap` (Task 2), `buffer` (Task 3).
- Produces: wire behavior the page relies on (Task 6):
  - New connection: after the dial (and TLS), the relay sends `open{token}`; `token` is 32 bytes from `crypto/rand`, base64url without padding.
  - Resume: `GET …?resume=1` (no host/port); the page's first frame, within 10 s, is `resume{token, got}`. Unknown, expired or closed token → close `CodeGone`. Otherwise the relay sends `resumed{got: up bytes it has written to the MUCK, away: ms since detach}` and resends down data from the page's `got`. If another WebSocket was attached, it's closed with `CodeReplaced` first.
  - Data: down binary frames are `wire.DownFrame`; up binary frames are raw bytes the relay writes to the MUCK. Acks both ways at `wire.AckBytes`/`wire.AckEvery`. `ping` gets `pong`.
  - `close` from the page: the relay closes the MUCK connection and releases everything.
  - The MUCK ends: once the buffer is all sent, the relay sends `end{at: buffer.end(), why: the read error's text or ""}` and keeps the hold until `close` or the hold runs out.
- Behavior:
  - A WebSocket ending any other way (read error, close without `close`, the relay's ping going unanswered) **detaches**: the MUCK connection stays, its bytes go on into the buffer, and a timer of `HoldFor()` (read from the config at detach) starts. Expiry closes the MUCK connection and releases the slot.
  - Per-connection cap: after each `add`, `dropLine` while `size() > BufferCap()`. Total cap: the server tracks bytes buffered across all held connections; while over `TotalBufferCap()`, `dropLine` on the held connection with the most bytes. Lock order is `s.mu` before a `held`'s own lock.
  - Slots: a held connection keeps its slot until it ends. A resume from another address moves its count to the new bucket without checking that bucket's cap.
  - Logs (new `[relay]` entries): `held = "held {ip} → {world:%q}"`, `resumed = "resumed {ip} → {world:%q} after {away}"`, `hold_expired = "let go {ip} → {world:%q}: held {dur} with no one back"`, `dropping = "dropping lines {ip} → {world:%q}: buffer full"` (once per detach), `refused_gone = "refused {ip}: no held connection for that token"`. `closed` stays, counting the whole connection.

- [ ] **Step 1:** Tests in `hold_test.go`, each over `httptest` with a fake TCP MUCK and a raw `websocket.Dial` client speaking `wire`; short `hold`, small caps and a fast `pingEvery` set through the config and the server's test fields:
  - `TestOpenSendsAToken`: first frame is `open` with a 43-character token.
  - `TestDetachKeepsTheMuck`: client CloseNow; the MUCK writes `"while away\r\n"`; the MUCK's side is still open after the client's gone; a resume with `got` gets `resumed` then a down frame with those bytes, stamped after the detach.
  - `TestResumeResendsUnacked`: read a frame without acking, CloseNow, resume with `got: 0` → the same bytes again, once.
  - `TestResumeTellsTheUpOffset`: write 5 bytes up, wait for the MUCK to read them, CloseNow, resume → `resumed.got == 5`.
  - `TestUnansweredPingDetaches`: a client that never reads; after `pingEvery + pingTimeout` the relay logs `held` and the MUCK is still connected.
  - `TestCloseEndsTheMuck`: send `close` → the MUCK sees EOF; a resume with that token → `CodeGone`.
  - `TestHoldExpires`: `hold = "200ms"`, detach, wait → MUCK sees EOF, log has `hold_expired`, resume → `CodeGone`, `max_per_ip = 1` lets a new connection in.
  - `TestEndAfterBuffered`: detach, the MUCK writes a line and closes; resume → the line, then `end` with `at` equal to the bytes sent.
  - `TestNewerAttachWins`: attach a second resume while the first is open → the first is closed with `CodeReplaced`, the second gets data.
  - `TestBufferCapDropsOldest`: `max_buffer_mb = 1`, detach, the MUCK writes 2 MiB of 1 KiB lines; resume → a `missed` with `lines` = the dropped count, then data starting at a line start; the log has `dropping` once.
  - `TestTotalCapDropsFromTheBiggest`: two held connections, total cap reached → only the bigger one drops.
  - `TestResumeFromNewAddressMovesTheSlot`: via `trusted_proxies` and `X-Forwarded-For`, open from A, resume from B with `max_per_ip = 1`; a new connection from A is now let in, one from B is refused `CodeTooMany`.
  - `TestResumeIsNotANewSlot`: at `max_conns = 1`, a resume of the one held connection succeeds.
- [ ] **Step 2:** `go test ./internal/relay` — FAIL.
- [ ] **Step 3:** Implement `held` (MUCK conn, token, bucket, world, buffer, up offset, the attached WebSocket if any, detach time, timer, start, byte totals) and the server's `holds map[string]*held`. One reader goroutine per `held` reads the MUCK into the buffer and wakes the current writer; one writer and one reader goroutine per attachment. Keep `keepAlive`'s ticker but have a failed ping detach.
- [ ] **Step 4:** `go generate ./internal/str && go test -race ./internal/relay` — PASS.
- [ ] **Step 5:** Commit `relay: hold a MUCK connection when its WebSocket dies` with `Strings: relay.held, relay.resumed, relay.hold_expired, relay.dropping, relay.refused_gone (new)`.

### Task 5: Lines carry a time and can be notes

**Files:**
- Modify: `internal/conn/conn.go`, `conn_test.go`, `internal/session/session.go`, `session_test.go`, `internal/kilntest/kilntest.go`, `internal/kilntest/kilntest_test.go`, and every `Lines()` caller the compiler finds

**Interfaces:**
- Produces, in `conn`:
  - `type Line struct { Text string; At time.Time /* zero: when it's read */; Note bool }`
  - `type Record struct { Data []byte; At time.Time; Note string }` — one read from a relay link: bytes and when they reached the relay, or (Data empty) a note about the connection.
  - `type RecordReader interface { ReadRecord() (Record, error) }`
  - `(*Conn).Lines() <-chan Line` (was `<-chan string`). If the `net.Conn` from `DialContext` is a `RecordReader` and TLS is off, `readLoop` uses `ReadRecord`; a line gets the `At` of the record holding its line end; a note becomes `Line{Text: note, At: rec.At, Note: true}` at once, between lines.
- Produces, in `session`: `LineConn.Lines() <-chan conn.Line`. `pump` logs a line as `logstore.In`, or a note as `logstore.Sys`, at `At` (zero: `Now()`). Every entry the session makes (in, out, sys, prompt) gets `max(time, previous entry's time)`.

- [ ] **Step 1:** Tests:
  - `conn`: `TestRecordsCarryTimes` (a fake `RecordReader` net.Conn returns `"a\r\nb"` at t1, `"\r\n"` at t2 → lines `a`@t1, `b`@t2) and `TestNotesComeBetweenLines` (`"a\r\n"`, note `"n"`, `"b\r\n"` → `a`, note `n`, `b` in that order).
  - `session`: `TestLinesUseTheirTimes` (a line with `At` an hour ago is logged with that time), `TestNotesAreSys`, `TestEntryTimesNeverGoBackwards` (a `Send` at 12:00, then a line stamped 11:30 → logged at 12:00; one stamped 12:05 → 12:05).
- [ ] **Step 2:** `go test ./internal/conn ./internal/session` — FAIL (build errors).
- [ ] **Step 3:** Implement; update the fakes and callers (mechanical: `Line{Text: s}`).
- [ ] **Step 4:** `go test ./...` — PASS, goldens untouched.
- [ ] **Step 5:** Commit `conn, session: lines carry their time, and relay notes become sys lines`.

### Task 6: wsdial's resumable link

**Files:**
- Create: `web/wsdial/link.go` (platform-neutral), `web/wsdial/link_test.go`
- Modify: `web/wsdial/wsdial.go`, `wsdial_other.go`, `wsdial_test.go`, `internal/str/locales/en.toml`

**Interfaces:**
- Consumes: `wire` (Task 1), the relay behavior (Task 4), `conn.Record`/`RecordReader` (Task 5).
- Produces:
  - `type socket interface { Read(ctx context.Context) (websocket.MessageType, []byte, error); Write(ctx context.Context, typ websocket.MessageType, p []byte) error; Close(code websocket.StatusCode, reason string) error; CloseNow() error }` — `*websocket.Conn` satisfies it natively; Task 7 writes the browser's.
  - `type link struct` implementing `net.Conn` and `conn.RecordReader`, built by `func dialLink(ctx context.Context, open func(ctx context.Context, url string) (socket, error), target string, resumeURL string) (*link, error)`. `Dialer` (both platforms) returns a `*link`.
  - Package functions over a registry of live links: `func Wake()`, `func DropAll()` (test hook: CloseNow every socket without `close`, as a dying network would).
  - `ErrLetGo`: the error `Read` returns after `CodeGone`, text `str.ConnRelayLetGo()`, not `Permanent` (the session's `reconnect` setting decides).
- Behavior:
  - Dial: open `target`, expect `open{token}` (a close is explained as today).
  - Down: each binary frame becomes a `Record` (`wire.ReadDown`), `got` advances, acks per cadence. `missed{lines, at}` queues a note `str.ConnMissed(lines)` and sets `got = at`. `end{why}` makes `Read` return `io.EOF` (or an error with `why`) after the queued records.
  - Up: `Write` appends to an unacked buffer and sends if attached; `ack{got}` trims it. `Write` never fails while resuming.
  - A socket error that isn't from `Close` → resume: dial `resumeURL`, send `resume{token, got}`, expect `resumed`; then queue note `str.ConnResumed(away rounded to the second)` and resend unacked up bytes past `resumed.got`. Retry delays `0, 1s, 2s, 4s, 8s`, then every 15 s. After 30 s of failing, queue `str.ConnRelayLost()` once per outage. `CodeGone` → `Read` returns `ErrLetGo`. `CodeReplaced` → `Read` returns `net.ErrClosed`.
  - `Wake()`: attached → send `ping`; no `pong` in 3 s → CloseNow the socket (which resumes). Waiting between retries → retry now.
  - `Close()`: send `close`, close the socket normally, stop resuming, unregister.
- New `[conn]` entries: `resumed = "resumed · away {away}"`, `missed = { one = "missed 1 line while away · the relay's buffer was full", other = "missed {n} lines while away · the relay's buffer was full" }`, `relay_lost = "lost the web relay · trying to get the connection back"`, `relay_let_go = "the web relay let go of this connection"`.

- [ ] **Step 1:** Tests in `link_test.go` against a real `relay.Server` on `httptest` with a fake TCP MUCK (as `wsdial_test.go` does today), reading through `conn.Dial` where lines matter:
  - `TestDropMidStreamDown`: the MUCK writes lines 1–3, `DropAll()`, writes 4–6 → `conn` delivers 1–6 once each, then a `resumed` note between 3 and 4 (`upTo(str.ConnResumed(mark))`).
  - `TestWritesWhileDownAreSentOnce`: `DropAll()`, `Send("look")` at once (before the resume) → the MUCK reads `look\r\n` exactly once.
  - `TestDropMidStreamUp`: a relay `Dial` hook that kills the WebSocket right after the relay reads an up frame but before it acks → the MUCK reads each line once.
  - `TestWakeResumesASilentSocket`: wrap the socket so it silently swallows reads and writes; `Wake()` → within 3 s plus a margin, a `resumed` note.
  - `TestRelayRestartLetsGo`: close the relay server's holds (new `relay.Server` on the same listener) → `Lines` closes with `Err()` matching `str.ConnRelayLetGo()`.
  - `TestMissedNote`: `max_buffer_mb = 1`, `DropAll()` with the retry held off (inject the retry delay), the MUCK writes 2 MiB of lines → a `missed` note naming the relay's count, and the lines after it in order.
  - `TestCloseSaysClose`: `Close()` → the MUCK sees EOF promptly (not after a hold).
- [ ] **Step 2:** `go test ./web/wsdial` — FAIL.
- [ ] **Step 3:** Implement `link.go`; `wsdial_other.go` shrinks to `Dialer` building the URLs and a `websocket.Dial`-based `open`. Retry delays and the wake timeout are fields a test can set.
- [ ] **Step 4:** `go generate ./internal/str && go test -race ./web/wsdial ./internal/relay/...` — PASS.
- [ ] **Step 5:** Commit `wsdial: resume a held connection when the WebSocket dies` with `Strings: conn.resumed, conn.missed, conn.relay_lost, conn.relay_let_go (new)`.

### Task 7: The browser side

**Files:**
- Modify: `web/wsdial/wsdial_js.go` (a `socket` over the page's WebSocket; `pageConn` goes), `web/cmd/kiln-web/main.go` (register `kiln.wake`, `kiln.dropLinks`), `web/ui/src/boot.ts`, `web/smoke/smoke_test.go` (new test; fake MUCK counts accepts), possibly `web/ui/src/*.d.ts` for the `kiln` handle

**Interfaces:**
- Consumes: `socket`, `Wake`, `DropAll` (Task 6).
- Produces: `kilnPage.dropLinks()` in `?record` mode (calls `kiln.dropLinks`).
- Behavior:
  - The browser socket queues incoming messages for `Read`, keeps the close code and reason (the reason the old code didn't use coder/websocket's browser build), and `Write`s with `ws.send`.
  - `boot.ts`: `visibilitychange` to visible and `pageshow` call `kiln.wake?.()`.

- [ ] **Step 1:** Smoke test `TestResumeKeepsTheCharacter` in `web/smoke/smoke_test.go`: connect, wait for the welcome line, `kilnPage.dropLinks()`, send `say hi`, wait for the echo; assert the echo shows once, a `resumed` note shows (built from `str.ConnResumed(mark)`), and the fake MUCK accepted exactly one connection.
- [ ] **Step 2:** `KILN_SMOKE=1 go test ./web/smoke -run Resume` — FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** `(cd web/ui && npm test) && GOOS=js GOARCH=wasm go build ./web/... && KILN_SMOKE=1 go test ./web/smoke` — PASS (the whole smoke suite: every test now goes through the link).
- [ ] **Step 5:** Commit `web: resume through the relay in the same tab`.

### Task 8: Docs

**Files:**
- Modify: `docs/web-relay.md` (`relay.toml` section: `hold`, `max_buffer_mb`, `max_total_buffer_mb`; a short "Held connections" section after it: what's held, for how long, that held connections count against `max_per_ip`/`max_conns`, the buffer drops oldest lines when full, a relay restart lets every held connection go), `docs/web-dev.md` if it describes the relay as a byte pipe.

- [ ] **Step 1:** Write it. The "What the relay can see" text from A already covers the buffer in memory; don't repeat it.
- [ ] **Step 2:** `go test ./...` — PASS (string lint).
- [ ] **Step 3:** Commit `docs: held connections and their settings`.
