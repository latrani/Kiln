# Web API, step 4b: the wasm build and a test page — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Kiln's core runs in the browser behind a bare test page that talks the JSON API; a headless Chrome smoke test drives it through the relay to a fake MUCK. Drafts survive the tab being killed. The xterm shim is gone.

**Architecture:**
- **`web/cmd/kiln-web`** is rewritten: it builds the `app.App` (as the old `main.go` built `ui.Deps`: config and data dirs in memfs, known hosts, the relay dialer, log writers, no saved passwords), runs a `webapi.Loop`, and exposes `kiln.start(relay, onBatch, saving)`, `kiln.act(json)`, `kiln.overLimit(text, max, mode)` and `kiln.storageFailed()` to the page. No `syscall/js` outside this command.
- **The test page** (`web/static/index.html`, `kiln.js`) keeps the tab lock, memfs, IndexedDB persistence and the preset, and replaces xterm with: a character list, an input (sends `input`/`submit` with a counter `N`, `older`/`newer` on arrow keys), and a log of every batch. It keeps every event in `window.kilnPage.events` for the smoke test. It's for development; part 3 builds the real page.
- **Drafts:** `webapi.Options.Drafts` (a file path). `{"Do":"save_drafts"}` writes every open character's input text (and the idle input's) there as JSON and syncs the file (so memfs flushes it to IndexedDB). Opening a character with a saved draft puts it in that character's input (`app.SetDraft`). The page sends its pending input, then `save_drafts`, on `visibilitychange` to hidden and on `pagehide`.
- **JSON shape (#141):** `Kind` and `Op` go out as names (`"server"`, `"append"`, …); actions with unknown fields are refused with `status.bad_action`; a zero `ConnectedAt` is left out; a test guards effect fields against colliding with `type`/`effect`.
- **Deleted:** `web/bridge`, the xterm vendor files and `web/vendor.sh`, the Bubble Tea `replace` in `web/go.mod` and its `require` if nothing in `web/` still needs it, `web/bubbletea_test.go`, `.github/workflows/bubbletea-watch.yml`.
- **Not in 4b (rulings):** backup/restore aren't wired to the new page (`Do{/backup}` and `Do{/restore}` show up in the event log): they were the xterm shim's UI, nobody uses the web build yet, and part 3 builds the page that offers them; `web/backup` stays, tested. #141's page-behaviour items (flush input before `switch`, Enter with nothing open, debouncing `resize`) are about the real page and move to part 3; the test page sends input on every keystroke and never resizes. #141 stays open for those and its cost item.

**Tech Stack:** Go (`GOOS=js GOARCH=wasm`), plain JS modules, `github.com/coder/websocket` (already in `web/go.mod`) for the smoke test's DevTools connection.

**Spec:** `docs/superpowers/specs/2026-10-11-web-api-design.md`, "The wasm side", "What goes away", and "For you to check" item 2 (drafts). Issues #139, #141.

## Global Constraints

- The TUI doesn't change; golden screens NOT updated.
- Strings: no new person-facing strings in Go except through the catalog. The test page's own text is a few fixed English labels marked as dev-only in a comment (the page isn't for people; part 3's is). `web.not_saving` (existing) is reused for storage failures.
- Run `go vet ./... && go test ./...`, `(cd web && go vet ./... && go test ./... && GOOS=js GOARCH=wasm go vet ./... && GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...)`, `node --test web/static/*.test.mjs`, and `web/build.sh` before each commit. No `-race`.
- End each commit with the session's attribution lines.

## Review Focus

1. **Nothing touches the core off the loop** in the wasm command: `kiln.act` only calls `Loop.Act`; `storageFailed` goes through `Loop.Say`; `overLimit` is pure.
2. **JS callbacks never block.** `kiln.act` and `kiln.storageFailed` must return at once (`Loop.Act` never blocks once the loop has ended; while it runs, its buffer is drained).
3. **Drafts:** a draft is restored only into a character that's opened, never over text the page has already sent; saving writes atomically enough that a half-written file can't break the next start (a bad file is ignored).
4. **Nothing still imports what was deleted** (`web/bridge`, Bubble Tea in `web/`), and `web/go.mod` is tidy.
5. **The smoke test is honest:** it fails if the page never connects or the echoed line never arrives, and it's skipped, saying why, without Chrome.

---

### Task 1: JSON shape

**Files:** `internal/webapi/encode.go`, `internal/webapi/loop.go`, `internal/webapi/encode_test.go`, `internal/webapi/loop_test.go`, `internal/feed/events.go` (`ConnectedAt` tag)

- [ ] **Tests first:**
  - `TestEncodeNamesKindsAndOps`: a `Lines` event with `Op: feed.Append` and a `Server` line encodes `"Op":"append"` and `"Kind":"server"`; `Reset`/`Replace`/`Prepend` and `Echo`/`Sys`/`Day`/`HistoryEnd` give `"reset"`, `"replace"`, `"prepend"`, `"echo"`, `"sys"`, `"day"`, `"history_end"`.
  - `TestEffectFieldsDontCollide`: for each of `app.Notify`, `app.Copy`, `app.SaveFile`, `app.Do`, `app.Quit`, no field is named `type` or `effect` in any case (`strings.EqualFold`).
  - `TestUnknownActionFieldsAreRefused`: `{"Do":"mark","Key":"fm/kit","Seqs":3}` sets the bad-action status.
  - `TestZeroConnectedAtIsLeftOut`: a `Char` for a character that never connected has no `ConnectedAt` key.
- [ ] **Implement:**
  - In `encode.go`: `func kindName(k app.Kind) string` and `func opName(o feed.Op) string` (the names above; `//str:ok`, vocabulary). `Line.Kind` becomes `string`; the anonymous structs for `Lines`/`LogLines` take `Op string`.
  - In `loop.go`'s `step`: decode with `json.NewDecoder(bytes.NewReader(m))` and `DisallowUnknownFields()`.
  - In `feed/events.go`: `ConnectedAt time.Time \`json:",omitzero"\``.
- [ ] Run everything; commit `webapi: kinds and ops by name; unknown action fields refused; no year-1 ConnectedAt`.

### Task 2: drafts and storage status

**Files:** `internal/app/submit.go` (`SetDraft`), `internal/app/submit_test.go`, `internal/webapi/loop.go`, `internal/webapi/actions.go`, `internal/webapi/loop_test.go`

**Interfaces:** `func (a *App) SetDraft(k, text string)` (sets open character k's input text; nothing if k isn't open); `type Options struct{ Autoconnect bool; Drafts string }`; `Run(ctx, Options)` replaces `Run(ctx, autoconnect bool)`; `func (l *Loop) Say(isErr bool, text string)` (posts a status onto the loop).

- [ ] **Tests first:**
  - app `TestSetDraft`: `SetDraft("fm/kit", "half")` on an open Kit → `Char("fm/kit").Text == "half"`; on a closed key, nothing (no panic).
  - webapi `TestDraftsSurviveARestart`: loop A with `Drafts: path`: `open` Kit, `input` "half a thought", `save_drafts`; stop it. Loop B over a fresh App with the same `Drafts`: `open` Kit → its batch has `Input{Key:"fm/kit", Text:"half a thought"}`. A garbage drafts file is ignored (`open` works, no draft).
  - webapi `TestSayPutsAStatusOnTheLoop`: `Say(true, "x")` → a batch with that `Status`.
- [ ] **Implement:**
  - `SetDraft` in `app/submit.go` (doc: "a draft saved from before, for when the front end restores one").
  - In the loop: `drafts map[string]string` loaded at `Run` start from `Options.Drafts` (missing or unreadable: none); after every successful `open` (and autoconnect open), `if d := l.drafts[k]; d != "" { a.SetDraft(k, d); delete(l.drafts, k) }`. The idle draft (key `""`) is restored at start with `a.SetInput` when nothing is open.
  - `save_drafts`: build `map[key]text` from `a.Order()` (`Char(k).Text`, non-empty only) plus `""` → `a.IdleText()` if non-empty; write it as JSON to `Drafts + ".tmp"`, `Sync`, rename over `Drafts`. Errors go nowhere (best effort; the page can't do anything about them).
  - `Say`: posts a `say{isErr, text}` message; `step` handles it with `SetStatus`.
- [ ] Run everything; commit `webapi: drafts saved on request and restored on open; Say`.

### Task 3: number the ui test helpers' lines (#139)

**Files:** `internal/ui/sidebar_test.go` (`addLine`), `internal/ui/filter_test.go` (`TestPanelIsFastOnLargeHistories`), `internal/ui/browse_test.go` (`TestBrowseFindIsFastOnLargeHistories`)

- [ ] Each helper that appends to `Lines` sets `Seq` one above the last line's (or 0 for the first), the way the core does. Add to each a check that the appended lines' numbers count up by one. Run everything (golden screens untouched); commit `ui: test helpers number the lines they add (#139)` with `Closes #139` in the body.

### Task 4: the wasm command and the test page; the shim goes

**Files:**
- Rewrite: `web/cmd/kiln-web/main.go`; keep `page.go`'s `download` (used for `SaveFile` effects? no: effects go to the page as JSON, and the page downloads) — delete `page.go`; keep `roots.go`, `roots_test.go`.
- Rewrite: `web/static/kiln.js`, `web/static/index.html`
- Delete: `web/bridge/`, `web/static/vendor/`, `web/vendor.sh`, `web/bubbletea_test.go`, `.github/workflows/bubbletea-watch.yml`
- Modify: `web/go.mod`, `web/go.sum` (`go mod tidy`), `docs/web-dev.md`

- [ ] **`main.go`** (build tag `js`): on `kiln.start(relay, onBatch, saving)`, build the App as the old `run` did (`config.Dir`, `EnsureDefaults`, `DataDir`, known hosts, `wsdial.Dialer(relay)`, a `conn.Dial` with `Width`/`Height` 80×24 until the page sends `resize`, log writers, `Password` returning `secrets.ErrNotFound`), apply the config (`a.ApplyConfig(cfg)`), make `webapi.New(a, out)` where `out` calls `onBatch` with the batch as a string, set `kiln.act = (json) => l.Act([]byte(json))`, `kiln.overLimit = (text, max, mode) => webapi.OverLimit(...)`, `kiln.storageFailed = () => l.Say(true, str.WebNotSaving())`, call `Say` once up front if `!saving`, and run `l.Run(ctx, webapi.Options{Autoconnect: true, Drafts: filepath.Join(dataDir, "drafts.json")})` in `main`'s goroutine (not the callback). Every JS-callback body returns at once.
- [ ] **`index.html`**: no xterm stylesheet; a `#chars` list, an `<input id="line">`, and `<pre id="log">`; the lock screen as before. Comment at the top: a development page; the real one is part 3.
- [ ] **`kiln.js`**: keep the lock, memfs, persist, preset and `kilnReady` flow; drop xterm. State: `events` (every event, for the smoke test, exposed as `window.kilnPage = { events, act }`), `order`, `active`, `chars`, and `n` (the input counter). `onBatch(json)` parses the batch, appends each event to `events` and to `#log`, and applies `Order`, `Active`, `Char` and `Input` (taking `Input.Text` for the active key only when `Ack === n`). Clicking a character sends `switch` (sending the line's text as `input` first). The input sends `input` on every `input` event, `submit` on Enter, `older`/`newer` on Up/Down, each with `++n` and the active key (`""` when no character is active). On `visibilitychange` to hidden and on `pagehide`: send `input` for the line, then `save_drafts`, then `persist.flush()`. `SaveFile` effects download as before (`kilnDownload` stays, fed from the event's base64 `Data`).
- [ ] **Delete** the shim files listed; `cd web && go mod tidy`; `grep -rn "bubbletea\|web/bridge\|xterm" web docs .github` finds nothing but this plan's history.
- [ ] **`docs/web-dev.md`**: the page is a development page for the web API (part 3 builds the real one); running it is as before; backup/restore and the xterm terminal are gone for now; drop the vendor step.
- [ ] Run everything, including `web/build.sh`; commit `web: kiln-web runs the web API behind a test page; the xterm shim goes`.

### Task 5: the smoke test

**Files:** Create `web/smoke/smoke_test.go`

- [ ] A Go test, skipped unless `KILN_SMOKE=1` (and skipped, saying so, when Chrome isn't at `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome` or `$KILN_CHROME`). It:
  1. builds the wasm and copies `web/static` and `wasm_exec.js` into a temp dir (as `build.sh` does);
  2. starts a fake MUCK: TLS on `127.0.0.1:0` with a self-signed cert (as `internal/conn/conn_test.go` makes one), which greets with `Welcome to the fake MUCK` and answers each line `L` with `You say, "L"`;
  3. writes a preset dir with `worlds/fake.toml` (`host = "127.0.0.1"`, the port, `tls = true`, `tls_trust = "pin"`, one character `tester`) and an allowlist naming the same host and port;
  4. runs `go run ./cmd/kiln-relay -dev -listen 127.0.0.1:<free port> -allow <file> -web <temp static> -preset <preset>` from the repo root, waiting for it to listen;
  5. starts headless Chrome (`--headless=new --remote-debugging-port=0 --user-data-dir=<temp>`), reads `DevToolsActivePort`, opens a target on the page, and drives it with `Runtime.evaluate` over the DevTools WebSocket;
  6. waits for `kilnPage.events` to hold an `Order`, then `act`s `open` `fake/tester`, `submit` `""` (connect), waits for a `Char` with `State` connected, `submit`s `hello`, and waits for a `Lines` event with a line whose `Text` contains `You say, "hello"`. Each wait gives up after 20 seconds with what the page has.
- [ ] Run it (`cd web && KILN_SMOKE=1 go test ./smoke -v`) and see it pass; run the rest; commit `web: a headless Chrome smoke test through the relay to a fake MUCK`.

### Task 6: review and PR

- [ ] Fresh review with this plan's Review Focus; fix Critical/Important with a failing test first; log minors.
- [ ] PR "Web API step 4b: the wasm build, a test page and a smoke test; the xterm shim goes". It closes #139; it says #141 stays open for the page-behaviour items (part 3) and the cost item; it says backup/restore aren't wired until part 3; it says drafts need a real-iPhone check (spec). Then stop.
