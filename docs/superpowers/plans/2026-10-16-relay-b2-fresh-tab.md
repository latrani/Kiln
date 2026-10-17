# Detachable relay B2: resume in a fresh tab — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A reload, a tab handoff ("Use Kiln here"), or iOS discarding the tab picks up every held character from where the saved log ends: no reconnect, no log-in, no duplicated or skipped lines. A page that really closes tells the relay to let go in 10 minutes.

**Architecture:** The page saves, per character, the relay token, the stream offset of the last line it has logged, and the agreed telnet options, in `relay.json` beside the logs (memfs → IndexedDB through `persist`). The page acks the relay only up to an offset that IndexedDB has saved, so the relay always still holds what a fresh tab needs. On boot, every character with a saved token is resumed from its saved offset; the session skips log-in for a resumed connection. `pagehide` (not persisted, not a handoff) sends the tokens in a `sendBeacon`, and the relay shortens those holds to `short_hold`.

**Tech Stack:** Go (`internal/relay`, `internal/conn`, `internal/telnet`, `internal/session`, `web/wsdial`, new `web/resume`), TypeScript page (`web/ui`, `web/static/persist.mjs`), headless-Chrome smoke tests.

**Spec:** `docs/superpowers/specs/2026-10-16-detachable-relay-design.md`: "Short hold on a real unload", "Resume" (fresh tab, discard, handoff), "Same browser only", "Hold runs out", "Settings" (short hold), and from "Left to the builder": telnet state, the token, what a fresh tab resumes, the handoff. B1 (merged, #220) did the relay hold and same-tab resume; C does Web Push.

## Global Constraints

- Every displayed string lives in `internal/str/locales/en.toml`; run `go generate ./internal/str`. Commits touching locales end with a `Strings:` trailer.
- Tests build expected text from the catalog (`str.X()`, `mark`/`upTo`).
- Below 1.0, clean breaks; the page↔relay protocol changes freely (the relay serves the page).
- Desktop Kiln and the TUI behave exactly as before. Golden screens are never `-update`d.
- The relay never writes traffic to disk; its log lines carry no traffic content.
- Names in tests and docs: Kit, Rook, Ash, world `fm`. The README is not touched.
- Verify on a clean worktree when the main checkout has someone's uncommitted edits.

## Decisions this plan makes

- **Kept, not received, is what the page acks.** `ack{got}` from the page already lets the relay forget below `got`; in B2 the page sends it only for offsets IndexedDB has saved. Same-tab resume still sends the received offset in `resume{got}` (the relay resends from `max(got, base)`), so B1 behaviour is unchanged. No protocol change.
- **Where a line ends in the stream.** `conn` feeds each record to the telnet parser one raw `\n` at a time; a `\n` that leaves the parser in its data state is a line end at a known stream offset, carried as `Line.Off`. Lines cut at `maxLine`, MCP lines and prompts carry no offset. A saved offset is therefore always a clean point: an empty splitter and the parser in data state.
- **Log first, then `relay.json`, in the same Go stretch.** The session appends the line to the log, then calls `Keep(off)`, which rewrites `relay.json` in memfs. memfs calls back synchronously, so Go doesn't yield between the two writes and no `persist` flush can split them: whatever IndexedDB has, its saved offset matches its last saved line.
- **Knowing a save landed.** Once a second, if a kept offset moved, Go writes `relay.json` and fsyncs it. memfs's `onSync` makes `persist` flush, and the page then calls `kiln.synced(path)`. Flushes resolve in order, so each `synced` for `relay.json` matches the oldest queued snapshot, and its offsets go to the relay as acks. With saving off (no IndexedDB), `onSync` calls `kiln.synced` at once.
- **Telnet state** (`telnet.State`: NAWS agreed, CHARSET will/do, UTF-8) is saved with each kept offset and restored in the fresh tab's parser. While a connection is held, nobody answers the MUCK's telnet requests: Fuzzball negotiates at connect, and a request that arrives while held is answered late when the page reads it. The relay doesn't answer for the page.
- **Handoff:** the old tab, before `persist.handOff()`, detaches every link (sockets closed without `close`, no resuming, no more acks), so nothing it reads after the final flush can be acked. The new tab loads after the handoff and resumes. `web.use_here_warning` goes (#184).
- **`/reset`** closes every link explicitly (the relay lets go now) before the wipe.
- **The beacon:** `POST <relay>?short=1`, body the tokens one per line (at most 64 KiB), answered 204. The token is the authority; no Origin check. A shortened hold that's resumed goes back to the full `hold` for its next detach.
- **A dead token at boot** (the hold ran out, the relay restarted): the resume's `ErrLetGo` is the dial error, the entry is deleted, and the session's `reconnect` setting decides, as for any failed connect.

## Review Focus

1. **A reload during busy output** must log each line once in the new tab: none replayed that the old tab's saved log already had, none skipped that it didn't. Smoke test in Task 8 (`TestReloadResumesTheCharacter`, with lines arriving across the reload) and the store test `TestKeptOnlyAfterSave` in Task 6.
2. **A handoff while lines arrive:** the old tab must not ack anything after its last flush. Smoke test in Task 8 (`TestHandoffResumesTheCharacter`).
3. **Saving off** (a private window, storage blocked) must still ack, or the relay's buffer fills and drops lines. Test in Task 6 (`TestSavingOffAcksAtOnce`).
4. **A dead token at boot** for a character with `reconnect = false` must show the relay let go and stay down, without a log-in; with `reconnect = true` it connects and logs in fresh. Test in Task 6 (`TestLetGoAtBootDeletesTheToken`) plus Task 5's session tests.
5. **Beacon then resume** (a reload): the shortened hold must not carry over, so a later app switch gets the full hold. Test in Task 1 (`TestResumeClearsTheShortHold`).

---

### Task 1: The relay's short hold

**Files:**
- Modify: `internal/relay/config.go`, `config_test.go`, `internal/relay/hold.go`, `hold_test.go`, `internal/relay/server.go`, `internal/str/locales/en.toml`, `web/dev-relay.example.toml`

**Interfaces:**
- Produces: `Config.ShortHold string` (`toml:"short_hold"`, default `"10m"`), `(*Config).ShortHoldFor() time.Duration`; `POST …?short=1` with tokens one per line → 204; `held.short bool`.

- [ ] **Step 1:** Tests: `TestParseShortHold` (unset → 10m, `"5m"` → 5m; `"soon"` → `str.RelayConfigBadShortHold(path, "soon")`). In `hold_test.go`: `TestBeaconShortensTheHold` (`hold = "1h"`, `short_hold = "200ms"`; open, POST the token, CloseNow → the world is let go within 3 s; the log has `upTo(str.RelayLettingGo(local, world, mark))`); `TestBeaconForADetachedHold` (detach first, then POST → let go within 3 s); `TestResumeClearsTheShortHold` (POST, CloseNow, resume, CloseNow again with `hold = "1h"` → still held after 500 ms); `TestBeaconIgnoresUnknownTokens` (204, nothing logged but nothing breaks); a GET with `short=1` is not a beacon (400).
- [ ] **Step 2:** `go test ./internal/relay` — FAIL.
- [ ] **Step 3:** Implement. `ServeHTTP` answers `POST` with `short=1` before upgrading. `shorten(h)`: set `h.short`; if detached, reset the timer to `min(left, ShortHoldFor())` (keep the `gen` check). `detach` uses `ShortHoldFor()` when `h.short`. `attach` clears `h.short`. New entries: `config_bad_short_hold = """{path}: short_hold {value:%q} isn't a length of time · use one like "10m\""""`, `letting_go = "letting go {ip} → {world:%q} in {dur}: its page closed"`.
- [ ] **Step 4:** `go generate ./internal/str && go test -race ./internal/relay/...` — PASS.
- [ ] **Step 5:** Commit `relay: a short hold when the page says it closed` with `Strings: relay.config_bad_short_hold, relay.letting_go (new)`.

### Task 2: Telnet state that can be saved

**Files:**
- Modify: `internal/telnet/telnet.go`, `telnet_test.go`

**Interfaces:**
- Produces: `type State struct { NAWS, CharsetWill, CharsetDo, UTF8 bool }` (JSON field names lowercase), `(*Parser).State() State`, `(*Parser).Restore(State)`, `(*Parser).InData() bool`.

- [ ] **Step 1:** Tests: `TestStateRoundTrip` (negotiate NAWS and CHARSET UTF-8 on one parser; a fresh parser `Restore`d from its `State` answers `Resize` with a NAWS message and reports `UTF8()`); `TestInData` (true at start, false after `IAC SB`, true after `IAC SE`).
- [ ] **Step 2:** `go test ./internal/telnet` — FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Same — PASS.
- [ ] **Step 5:** Commit `telnet: state a page can save and restore`.

### Task 3: Lines know where they end in the stream

**Files:**
- Modify: `internal/conn/conn.go`, `conn_test.go`

**Interfaces:**
- Consumes: `telnet.State`, `InData`, `Restore` (Task 2).
- Produces: `Record.End int64` (stream offset just past the record; the link sets it), `Line.Off int64` (offset just past this line's `\n`; 0 for none), `Options.Telnet *telnet.State` (restored into the new parser), `(*Conn).TelnetState() telnet.State`.

- [ ] **Step 1:** Tests with `dialRecords`: `TestLinesKnowTheirOffsets` (records `"a\r\nb"` End 4 and `"\r\nc\r\n"` End 9 → `a`@3, `b`@6, `c`@9); `TestNewlineInSubnegotiationIsNoLineEnd` (`IAC SB 31 0 10 0 24 IAC SE` then `"hi\r\n"` → one line `hi` with Off at the record's end); `TestMCPLineHasNoLine` (unchanged lines, offsets of real lines right); `TestTelnetRestored` (Options.Telnet with NAWS → `Resize` writes NAWS without a negotiation).
- [ ] **Step 2:** `go test ./internal/conn` — FAIL.
- [ ] **Step 3:** Implement the piecewise feed in `processLoop`: for each record, feed up to and including each raw `\n`, and when `parser.InData()` after it, give the last line `push` returned that piece's offset. Desktop TCP reads (no records) leave `Off` 0.
- [ ] **Step 4:** `go test -race ./internal/conn` — PASS.
- [ ] **Step 5:** Commit `conn: lines carry where they end in the relay's stream`.

### Task 4: wsdial for a page that keeps what it reads

**Files:**
- Modify: `web/wsdial/link.go`, `wsdial_other.go`, `wsdial_js.go`, `wsdial.go`, `link_test.go`, `wsdial_test.go`

**Interfaces:**
- Consumes: `Record.End` (Task 3).
- Produces:
  - `type Options struct { TLS bool; Durable bool; Resume *Held }`, `type Held struct { Token string; Kept int64 }`.
  - `func Dial(ctx context.Context, relayURL, addr string, o Options) (*Link, error)`: opens a new connection, or with `Resume` resumes that hold from `Kept` (a fresh tab). `Dialer(relayURL, tls)` stays, as `Dial` with neither option.
  - `type Link` (was `link`) with `Token() string`, `Kept(off int64)` (ack up to `off`; only `Durable` links wait for it, others ack on receipt as in B1), `Resumed() bool` (it picked up a hold rather than opening one), `OnEnd(func(error))` (the link ended for good: closed, let go, replaced, the world gone; not on `DetachAll`).
  - `func DetachAll()`: close every socket without `close`, stop resuming and acking (lines already read are still logged and kept before the handoff's last save); links read as ended with an error that says `Detached`, and never call `OnEnd`.
  - `func CloseAll()`: `Close` every link (the relay lets go now).
  - `func Tokens() []string`: live links' tokens.
  - Records get `End` set from the link's received offset.

- [ ] **Step 1:** Tests: `TestDurableAcksOnlyWhatsKept` (a durable link reads 3 lines; the relay's buffer, checked through a resume with `Kept: 0` from a second `Dial`, still has them all; after `Kept(n)` and a moment, a resume from 0 gets a `missed`-free replay starting at `n`); `TestFreshResume` (dial, read two lines, `DetachAll()`, the world writes a third; `Dial` with `Resume{token, kept after line 1}` → `Resumed()` true, lines 2 and 3, the world saw one connection); `TestFreshResumeLetGo` (unknown token → `ErrLetGo`); `TestOnEndNotOnDetach`; `TestCloseAllLetsGo`; `TestTokens`.
- [ ] **Step 2:** `go test ./web/wsdial` — FAIL.
- [ ] **Step 3:** Implement. A fresh resume reuses `tryResume`'s socket handshake with `got = Kept` and then `attach`. Rename internal uses of `link` to `Link`.
- [ ] **Step 4:** `go test -race ./web/wsdial && GOOS=js GOARCH=wasm go vet ./...` (in `web`) — PASS.
- [ ] **Step 5:** Commit `wsdial: resume from a saved token, and ack only what's kept`.

### Task 5: The session keeps lines and skips log-in on a resume

**Files:**
- Modify: `internal/session/session.go`, `session_test.go`

**Interfaces:**
- Consumes: `Line.Off` (Task 3).
- Produces: optional `LineConn` interfaces `interface{ Keep(off int64) }` (called after a line with `Off > 0` is appended to the log, even if the append failed: the log failing is reported as today) and `interface{ Resumed() bool }` (true: no `NewSession`, no "connected" line, no log-in; the state still goes to `Connected`).

- [ ] **Step 1:** Tests: `TestKeepAfterTheLog` (a fake conn records the log length when `Keep` is called; it's after the line's entry, with the line's `Off`); `TestNoKeepWithoutOffset`; `TestResumedSkipsLogin` (a resumed fake conn: nothing sent, no `str.SessionConnected` line, the log saw no `NewSession`, state `Connected`); `TestResumedThenDroppedLogsInNextTime` (the resumed conn ends; the redial is a plain conn and logs in).
- [ ] **Step 2:** `go test ./internal/session` — FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** `go test -race ./internal/session ./internal/app ./internal/webapi` — PASS.
- [ ] **Step 5:** Commit `session: keep logged lines, and don't log in again on a resume`.

### Task 6: `web/resume`: the saved tokens and the dial

**Files:**
- Create: `web/resume/resume.go`, `web/resume/resume_test.go`

**Interfaces:**
- Consumes: `wsdial.Dial`, `Options`, `Held`, `Link` (Task 4), `conn.Dial`, `Options.Telnet`, `TelnetState` (Task 3), the session's `Keep`/`Resumed` (Task 5), `config.Character`, `session.LineConn`.
- Produces:
  - `type Entry struct { Token string \`json:"token"\`; Kept int64 \`json:"kept"\`; Telnet telnet.State \`json:"telnet"\` }`.
  - `func Open(path string, saving bool) *Store` (a missing or unreadable file is empty), `(*Store) Keys() []string` (sorted), `Get(key) (Entry, bool)`, `Put(key string, e Entry)` and `Delete(key string)` (each rewrites the file, no fsync), `Sync()` (if anything changed since the last Sync: rewrite, fsync, queue the kept offsets; with `saving` false, ack them at once instead), `Synced(path string)` (for this file's path, ack the oldest queued offsets; other paths do nothing), `Path() string`.
  - `func (s *Store) Dial(ctx context.Context, relayURL string, ch config.Character) (session.LineConn, error)`: with a saved entry, `wsdial.Dial` with `Resume` and `Durable`, restoring its telnet state; `ErrLetGo` deletes the entry and is returned. Otherwise a new durable dial, and the token is `Put` with `Kept 0`. The returned conn embeds `*conn.Conn`, implements `Keep` (Put the entry with that offset and the conn's telnet state; the link is told at `Synced`) and `Resumed`, and the link's `OnEnd` deletes the entry. The key is `app.Key(ch.World, ch.ID)`.

- [ ] **Step 1:** Tests over a real `relay.Server` and fake world (as wsdial's tests), with a temp dir: `TestKeptOnlyAfterSave` (lines logged and kept; before `Synced`, a second resume of the token from 0 still replays them; after `Sync` and `Synced`, it starts past them); `TestSavingOffAcksAtOnce`; `TestStoreSurvivesReopen` (Put, Sync, Open again → same entries); `TestFreshDialSavesTheToken`; `TestSavedTokenResumes` (an entry from a first store's dial, `DetachAll`, a second store opened on the same file dials and gets `Resumed()` with telnet restored); `TestLetGoAtBootDeletesTheToken`; `TestEndDeletesTheEntry` (world closes → entry gone); `TestSyncedOtherPathIgnored`.
- [ ] **Step 2:** `cd web && go test ./resume` — FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** `go test -race ./resume` — PASS.
- [ ] **Step 5:** Commit `web/resume: save relay tokens beside the logs, and dial from them`.

### Task 7: Held characters come back at boot

**Files:**
- Modify: `internal/webapi/loop.go`, `loop_test.go`

**Interfaces:**
- Produces: `webapi.Options.Resume []string`: character keys to open and connect at start whatever their `autoconnect` (each once, alongside autoconnect ones).

- [ ] **Step 1:** Test `TestResumeConnectsHeldCharacters`: Rook (no autoconnect) in `Resume` → opened and connecting at start; a key that no longer exists is skipped; a character both autoconnect and in `Resume` connects once.
- [ ] **Step 2:** `go test ./internal/webapi` — FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** `go test ./internal/webapi` — PASS.
- [ ] **Step 5:** Commit `webapi: open and connect held characters at start`.

### Task 8: The page: saving, beacon, handoff, reset

**Files:**
- Modify: `web/cmd/kiln-web/main.go`, `web/ui/src/boot.ts`, `web/ui/src/boot.test.ts` (if the hooks are testable there), `web/ui/src/ui/Splash.tsx`, `internal/str/locales/en.toml` (drop `web.use_here_warning`), `web/smoke/resume_test.go`

**Interfaces:**
- Consumes: `resume.Store` (Task 6), `wsdial.DetachAll/CloseAll/Tokens` (Task 4), `Options.Resume` (Task 7).
- Produces, on the `kiln` handle: `synced(path)`, `tokens() string[]`, `detachLinks()`, `closeLinks()`.
- Behavior:
  - `main.go`: open the store at `<data>/relay.json` with `saving` from the page; Dial through it; pass `Keys()` as `Options.Resume`; a 1 s ticker calls `Sync`.
  - `boot.ts`: `onSync: (p) => persist ? persist.flush().then(() => g.kiln?.synced?.(p)) : g.kiln?.synced?.(p)`. `pagehide` with `persisted` false and no handoff underway: `navigator.sendBeacon(<relay as http(s)>?short=1, tokens.join("\n"))` when there are tokens. The handoff's `beforeRelease` first sets the handoff flag and calls `kiln.detachLinks()`, then drafts and `persist.handOff()`. `/reset`'s wipe calls `kiln.closeLinks()` first.
  - The lock screen loses its warning.

- [ ] **Step 1:** Smoke tests in `web/smoke/resume_test.go`: `TestReloadResumesTheCharacter` (log in; ask the fake MUCK for `later 3 after 400`; reload the tab at once through DevTools `Page.reload`; after boot: each "late" line once in the scrollback, the login once in the log, a resumed line, `muckAccepts` unchanged); `TestHandoffResumesTheCharacter` (log in; `later 3 after 400`; another tab's "Use here"; the new tab shows each late line once, no second log-in, `muckAccepts` unchanged); `TestClosedTabLetsGoSoon` (relay with `short_hold = "1s"`; close the tab through DevTools `Target.closeTarget`; the fake MUCK sees the connection end within 5 s).
- [ ] **Step 2:** `KILN_SMOKE=1 go test ./smoke -run 'Reload|Handoff|ClosedTab'` — FAIL.
- [ ] **Step 3:** Implement. Remove the warning string and its use.
- [ ] **Step 4:** `(cd ui && npm test) && GOOS=js GOARCH=wasm go vet ./... && KILN_SMOKE=1 go test ./smoke` — PASS (whole suite).
- [ ] **Step 5:** Commit `web: resume held characters in a fresh tab` with `Strings: web.use_here_warning (removed)`.

### Task 9: Docs

**Files:**
- Modify: `docs/web-relay.md` (`short_hold` in the example and the held-connections section: a reload, a handoff and a discarded tab resume; a closed tab lets go after `short_hold`), `docs/web-dev.md` if it describes the lock screen or reloads dropping connections.

- [ ] **Step 1:** Write it.
- [ ] **Step 2:** `go test ./internal/str` — PASS.
- [ ] **Step 3:** Commit `docs: fresh-tab resume and the short hold`.
