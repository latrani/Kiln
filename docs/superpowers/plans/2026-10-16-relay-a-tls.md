# Detachable relay A: the relay terminates TLS — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** kiln-relay opens the TLS connection to each TLS world itself, from trust its config names, and the web build stops doing TLS and pinning in the tab. The allowlist file becomes `relay.toml` (`-config`).

**Architecture:** `internal/relay` gains per-world trust (`tls_trust = "ca"` or `pin = "sha256:…"`) and dials `crypto/tls` after TCP. The page says what its world file asks for (`tls=1` in the relay URL), and the relay refuses a TLS request for a plaintext world. The web build dials with `conn.Options.TLS = false`. Resume (B1, B2) and push (C) are later plans; this one changes no buffering.

**Tech Stack:** Go (`crypto/tls`, `coder/websocket`, BurntSushi/toml), TypeScript page (`web/ui`), headless-Chrome smoke tests (`web/smoke`, `KILN_SMOKE=1`).

**Spec:** `docs/superpowers/specs/2026-10-16-detachable-relay-design.md`, sections "The big decision", "Documenting it", "Settings" (trust and the rename only), "Constraints".

## Global Constraints

- Every displayed string, relay log lines included, lives in `internal/str/locales/en.toml`; run `go generate ./internal/str`. Commits that touch locales end with a `Strings:` trailer listing keys.
- Tests build expected text from the catalog (`str.X()`, `mark`/`upTo`), never copies.
- Below 1.0, clean breaks: `-allow` is gone with no alias or hint; `known_hosts` in a preset stops meaning anything; no migration for saved pins.
- Desktop Kiln and the TUI behave exactly as before (`conn.Dial` with `TLS: true` still does TLS in-process).
- Names in tests and docs: Kit, Rook, Ash, world `fm`.
- The README is not touched.
- `run-web.sh` and `web/dev-allow.toml` are Indi's local, untracked files (not in git). Update them in place as Tasks 1 and 2 say (Indi asked); they never get committed.

## Review Focus

1. **A bad certificate retries forever.** A pin mismatch or CA failure at the relay must put the web session in `Failed` (like a desktop pin mismatch), not back off and re-dial every few seconds. Test in Task 4.
2. **A TLS world that never answers the handshake** (a plaintext port listed as TLS) must refuse within the dial timeout with `CodeUnreachable`, not hold a slot. Test in Task 3.
3. **Saved world files from before this change** (`tls = true`, `tls_trust = "pin"`) keep connecting: the page sends `tls=1`, the relay does TLS, and saved `known_hosts` in the browser is ignored. Covered by the smoke world in Task 5, which keeps `tls_trust = "pin"`.
4. **Every new refusal releases its slot** (no-TLS mismatch, bad cert, handshake failure). Test in Task 3.
5. **Config reload while connections are open:** changing a world's pin applies to new connections only; an open one keeps going. Test in Task 2's watcher test.

---

### Task 1: The allowlist becomes `relay.toml`

Pure rename; behavior unchanged.

**Files:**
- Rename: `internal/relay/allow.go` → `internal/relay/config.go`, `allow_test.go` → `config_test.go`
- Modify: `internal/relay/watch.go`, `watch_test.go`, `server.go`, `server_test.go`, `cmd/kiln-relay/main.go`, `main_test.go`, `web/smoke/smoke_test.go`, `web/wsdial/wsdial_test.go`
- Rename: `web/dev-allow.example.toml` → `web/dev-relay.example.toml`; `.gitignore` `/web/dev-allow.toml` → `/web/dev-relay.toml`
- Modify: `internal/str/locales/en.toml` (`[relay]`), `docs/web-relay.md`, `docs/web-dev.md`

**Interfaces:**
- Produces: `type Config` (was `Allowlist`), `LoadConfig(path string) (*Config, error)`, `ParseConfig(path string, b []byte) (*Config, error)`, `WatchConfig(s *Server, path string) (io.Closer, error)`, `(*Server).SetConfig(*Config)`. `Lookup`, `Trusted` unchanged.

- [ ] **Step 1:** Rename `TestRunNeedsAllow` → `TestRunNeedsConfig`, asserting `run(nil, …)` returns `str.RelayNeedConfig()`; add `TestAllowFlagIsGone`: `run([]string{"-allow", "x"}, …)` returns a flag error mentioning `-allow` (flag package's "flag provided but not defined").
- [ ] **Step 2:** Run `go test ./cmd/kiln-relay` — FAIL (undefined `str.RelayNeedConfig`).
- [ ] **Step 3:** Rename the Go identifiers above and the `-allow` flag to `-config`. Catalog: replace `allow_*` keys with `config_*` (`config_reading`, `config_unknown_key`, `config_no_host`, `config_no_ports`, `config_bad_port`, `config_bad_proxy`, `config_bad_origin`, `config_duplicate`, `config_bad_limit`, `config_loaded`, `config_reload_failed`), each message starting `{path}:` instead of `allowlist {path}:`; `watch_failed` = `"watching the config: {err}"`; `need_allow` → `need_config = "kiln-relay needs -config PATH (relay.toml)"`; `flag_allow` → `flag_config = "the relay's config file (relay.toml)"`. `refused_not_listed` keeps "not on the allowlist" (the `[[world]]` list is still the allowlist). Docs: every `-allow`/allowlist-file mention becomes `-config`/`relay.toml`; `/etc/kiln-relay/relay.toml` in examples; the `[[world]]` list is "the allowlist". Package and type comments follow.
- [ ] **Step 4:** Run `go generate ./internal/str && go test ./... && (cd web/ui && npm test)` — PASS. `KILN_SMOKE=1 go test ./web/smoke` — PASS.
- [ ] **Step 5:** Local files: `mv web/dev-allow.toml web/dev-relay.toml`; in `run-web.sh`, `-allow web/dev-allow.toml` → `-config web/dev-relay.toml`.
- [ ] **Step 6:** Commit `relay: the allowlist file becomes relay.toml (-config)` with a `Strings:` trailer listing every renamed key.

### Task 2: Per-world trust in the config

**Files:**
- Modify: `internal/relay/config.go`, `config_test.go`, `watch_test.go`, `internal/str/locales/en.toml`, `internal/conn/pin.go` (no change expected; `ValidateFingerprint` is reused)

**Interfaces:**
- Produces: `World.TLSTrust string` (`toml:"tls_trust"`, only `"ca"`), `World.Pin string` (`toml:"pin"`, `conn.Fingerprint` format). After `ParseConfig`, a non-plaintext world has exactly one of the two set.

- [ ] **Step 1: Write the failing tests** in `config_test.go`:
  - `TestParseTrust`: a world with `tls_trust = "ca"` parses with `TLSTrust == "ca"`; one with `pin = "sha256:" + 64 hex` parses with that `Pin`; a `plaintext = true` world with neither parses.
  - Extend `TestParseErrors` table with, each matched against its catalog text via `upTo(str.X(mark…))`:
    - TLS world with neither → `str.RelayConfigNoTrust(path, host)`
    - both set → `str.RelayConfigBothTrust(path, host)`
    - `tls_trust = "pin"` or any value but `"ca"` → `str.RelayConfigBadTrust(path, host, value)`
    - `pin = "sha256:abc"` → `str.RelayConfigBadPin(path, host, value)`
    - `plaintext = true` with `pin` or `tls_trust` → `str.RelayConfigPlaintextTrust(path, host)`
  - `TestWatchConfigNewPinForNewConnections` in `watch_test.go`: after a reload that changes a world's pin, `Lookup` returns the new pin (the server reads the config per connection, so open ones are untouched; Task 3's tests cover the dial).
- [ ] **Step 2:** Run `go test ./internal/relay` — FAIL.
- [ ] **Step 3:** Add the fields and checks in `ParseConfig`'s world loop; reuse `conn.ValidateFingerprint` for `pin`. New catalog entries:
  - `config_no_trust = '{path}: {host} needs tls_trust = "ca" or a pin (or plaintext = true)'`
  - `config_both_trust = "{path}: {host} has both tls_trust and pin · use one"`
  - `config_bad_trust = '{path}: {host} has tls_trust {value:%q} · the only value is "ca"; use pin = "sha256:…" for a pinned certificate'`
  - `config_bad_pin = '{path}: {host} has pin {value:%q} · use "sha256:" and 64 hex digits, as in Kiln''s known_hosts'`
  - `config_plaintext_trust = "{path}: {host} is plaintext, so it can't have tls_trust or pin"`
- [ ] **Step 4:** Run `go generate ./internal/str && go test ./internal/relay` — PASS. Existing server tests that build TLS worlds with `world(port, false)` now fail to parse: update the `world()` helper to take a trust string (`world(port int, trust string)`, `""` meaning plaintext) and pass `pin = …` where the test has a certificate, `tls_trust = "ca"` otherwise. Expect Task 3 to rewrite the TLS tests themselves.
- [ ] **Step 5:** Local file: add `pin = "sha256:76d3b9f3c88542b199af41a3d1639765f2a78c14b7ae5581d9be68270f9f9342"` to the `muck.postfurry.net` world in `web/dev-relay.toml` (PFMuck's cert is self-signed; this matches Indi's desktop `known_hosts`, checked 2026-10-16).
- [ ] **Step 6:** Commit `relay: per-world TLS trust in relay.toml` with `Strings:` trailer.

### Task 3: The relay dials TLS

**Files:**
- Modify: `internal/relay/server.go`, `server_test.go`, `internal/str/locales/en.toml`, `docs/web-relay.md`

**Interfaces:**
- Consumes: `World.TLSTrust`, `World.Pin` (Task 2).
- Produces:
  - Query parameter `tls=1`: the client's world file asks for TLS.
  - `CodeNoTLS websocket.StatusCode = 4426` (replaces `CodeNeedsTLS`; same number, new meaning): the client asked for TLS and the world is plaintext on this relay.
  - `CodeBadCert websocket.StatusCode = 4526`: the world's certificate failed its pin or CA check. Close reason is `reason(err)`.
  - `Server.RootCAs *x509.CertPool` — nil: system roots. Test hook for `"ca"` worlds.
  - A non-cert handshake failure (timeout, not TLS) closes with `CodeUnreachable` and the error as reason, as a failed dial does.

- [ ] **Step 1: Write the failing tests** (delete `TestTLSOnlyRefusesPlainFirstFrame`, `TestTLSOnlyRefusesSilentClient`, `TestTLSThroughRelayPins`; rewrite `TestTLSRefusalsReleaseSlots`). The `open` helper gains a `tls bool` that adds `tls=1`.
  - `TestRelayDoesTLSWithPin`: self-signed listener greeting `"secure hello\r\n"`, world `pin = conn.Fingerprint(cert)`; a client opening with `tls=1` reads plain `"secure hello\r\n"` from the WebSocket; the log's first line starts with `upTo(str.RelayOpened(mark, mark))`.
  - `TestRelayDoesTLSWithoutClientAsking`: same world, `open` without `tls=1`, still reads `"secure hello"` (TLS is the relay's call).
  - `TestRelayPinMismatch`: world pinned to `"sha256:" + 64 zeros` → close `CodeBadCert`; log line starts with `upTo(str.RelayRefusedBadCert(mark, mark, mark))`.
  - `TestRelayCATrust`: `tls_trust = "ca"`, `s.RootCAs` holds the test cert (generate it with `IPAddresses: 127.0.0.1` in `selfSignedListener`) → reads the greeting; with `s.RootCAs` an empty pool → `CodeBadCert`.
  - `TestRelayRefusesTLSForPlaintextWorld`: plaintext world, `tls=1` → `CodeNoTLS`, never dialed (the `dialed` flag pattern), log `upTo(str.RelayRefusedNoTls(mark, mark))`.
  - `TestRelayHandshakeTimeout`: TLS world whose listener accepts and never writes; with a short `s.HandshakeTimeout` (new field, 0: 15s, shared with the dial) the client gets `CodeUnreachable` within a second or two.
  - `TestTLSRefusalsReleaseSlots`: the no-TLS refusal, a pin mismatch and a handshake timeout each leave `s.inUse() == 0`.
  - `TestPlaintextRoundTrip` stays as is (no `tls=1`).
- [ ] **Step 2:** Run `go test ./internal/relay` — FAIL.
- [ ] **Step 3:** In `ServeHTTP`: drop the first-frame check, `FirstFrameTimeout` and `first`. After `acquire`, refuse `tls=1` on a plaintext world. After the TCP dial, for a non-plaintext world wrap in `tls.Client` with `ServerName: wl.Host`, `RootCAs: s.RootCAs`; for a pin, `InsecureSkipVerify` plus `VerifyConnection` comparing `conn.Fingerprint(PeerCertificates[0])` with `wl.Pin`. Handshake under the same 15s context as the dial (`HandshakeTimeout` overrides). Sort errors: `*tls.CertificateVerificationError`, the pin mismatch, `x509` verification errors → `CodeBadCert`; anything else → `CodeUnreachable`. Catalog: delete `refused_not_tls`; add `refused_no_tls = "refused {ip} → {world:%q}: client asked for TLS, world is plaintext"` and `refused_bad_cert = "refused {ip} → {world:%q}: certificate: {err}"`. Update the package doc and `World.Plaintext`'s comment: TLS now runs in the relay.
- [ ] **Step 4:** Docs (`docs/web-relay.md`): the intro paragraph no longer describes the first-frame check; add "What the relay can see" before Installing, word for word from the spec's "Documenting it"; replace "TLS trust in the browser" with a "TLS trust" section: each TLS world needs `tls_trust = "ca"` or `pin = "sha256:…"` (copy the pin from a desktop Kiln's `known_hosts` after it connects), and nothing is trusted on first use. The example config shows one of each, plus a plaintext world; the systemd unit gains `LimitCORE=0`.
- [ ] **Step 5:** Run `go generate ./internal/str && go test ./internal/relay ./cmd/kiln-relay` — PASS.
- [ ] **Step 6:** Commit `relay: terminate TLS at the relay` with `Strings:` trailer.

### Task 4: The web build stops doing TLS

**Files:**
- Modify: `web/wsdial/wsdial.go`, `wsdial_js.go`, `wsdial_other.go`, `wsdial_test.go`, `web/cmd/kiln-web/main.go`, `internal/session/session.go`, `session_test.go`, `internal/str/locales/en.toml`

**Interfaces:**
- Consumes: `tls=1`, `CodeNoTLS`, `CodeBadCert` (Task 3).
- Produces:
  - `wsdial.Dialer(relayURL string, tls bool) func(ctx context.Context, network, addr string) (net.Conn, error)`; `tls` adds `tls=1` in `target`.
  - `wsdial.CertError` — the error a `CodeBadCert` close explains to; message `str.ConnRelayBadCert(reason)`; method `Permanent() bool` returning true.
  - In `session.permanent`: `errors.As(err, &p)` for `p interface{ Permanent() bool }` with `p.Permanent()` counts as permanent. `session` does not import `wsdial`.

- [ ] **Step 1: Write the failing tests:**
  - `wsdial_test.go`: `TestTLSWorldThroughRelay` becomes: relay config pins the test cert; `conn.Dial` with `TLS: false` and `Dialer(url, true)` reads `"secure hello"`. Delete `TestPlainClientOnTLSWorldReadsAsCatalogText`. Add `TestTLSForPlaintextWorldReadsAsCatalogText` (`Dialer(url, true)` at a plaintext world → error containing `str.ConnRelayNoTls()`) and `TestBadCertReadsAsCatalogText` (zero pin → error containing `upTo(str.ConnRelayBadCert(mark))`, and `errors.As(err, new(*CertError))`). `TestCodesMatchRelay` lists `CodeNoTLS` and `CodeBadCert`. `TestExplain` covers both codes.
  - `session_test.go`: `TestPermanentDialErrorStopsRetrying` (or extend the existing permanent test): a dial returning an error with `Permanent() bool { return true }` puts the session in `Failed` with no backoff attempt.
- [ ] **Step 2:** Run `go test ./web/wsdial ./internal/session` — FAIL.
- [ ] **Step 3:** Implement. Catalog `[conn]`: delete `relay_needs_tls`; add `relay_no_tls = "the web relay doesn't use TLS for this world · turn tls off in its settings, or ask the relay's operator"` and `relay_bad_cert = "the web relay refused this world's certificate: {reason}"`. In `kiln-web/main.go`, `Dial` passes `TLS: false`, no `KnownHosts`, and `DialContext: wsdial.Dialer(p.relay, ch.TLS)` per call. Update `wsdial`'s package doc (the relay does TLS now) and `conn.Options.DialContext`'s comment if it says TLS runs over the relay.
- [ ] **Step 4:** Run `go generate ./internal/str && go test ./... && GOOS=js GOARCH=wasm go vet ./web/...` — PASS.
- [ ] **Step 5:** Commit `web: dial without TLS; the relay does it` with `Strings:` trailer.

### Task 5: The page drops browser pins

**Files:**
- Modify: `web/ui/src/boot.ts`, `boot.test.ts`, `web/static/memfs.mjs` (comment), `internal/str/locales/en.toml` (`relay.flag_preset`), `web/smoke/smoke_test.go`, `docs/web-relay.md` (Preset section), `docs/web-dev.md`

**Interfaces:**
- Consumes: `relay.toml` `pin` (Task 2), the relay doing TLS (Task 3), `Dialer(url, tls)` (Task 4).

- [ ] **Step 1: Write the failing smoke test:** replace `TestPresetPinRefusesOtherCert` with `TestRelayPinRefusesOtherCert`: `launchWith` now takes the relay config's pin (`func(port int) string` returning the `pin` value) instead of preset `known_hosts`; with a zero pin, opening and connecting `fake/tester` shows `str.ConnRelayBadCert` text (cut with `upTo(… mark)`) in the character's lines or status, wherever the web build shows a session's failure today (find it in the existing smoke tests of failed connects), and never "Welcome". `config()` writes `pin = <fake cert's fingerprint>` for the default launch, and no preset `known_hosts`. The world file keeps `tls = true` and `tls_trust = "pin"` (Review Focus 3).
- [ ] **Step 2:** Run `KILN_SMOKE=1 go test ./web/smoke -run 'TestRelayPin|TestConnect'` — FAIL.
- [ ] **Step 3:** Remove `KNOWN_HOSTS`, the `known_hosts` branch of `seedPreset`, and `mergeKnownHosts` with its tests; update `seedPreset`'s comment. `flag_preset = "serve these files at /kiln/preset/ for the page to preload (config/…)"`. memfs comment: config and logs. Docs: the preset layout loses `known_hosts`; `web-dev.md` says to put pins in `web/dev-relay.toml`.
- [ ] **Step 4:** Run `go generate ./internal/str && (cd web/ui && npm test) && web/build.sh && KILN_SMOKE=1 go test ./web/smoke` — PASS. Golden screens unchanged; never `-update`.
- [ ] **Step 5:** Commit `web: drop browser TLS pins; the relay's config pins worlds` with `Strings:` trailer.

## PR notes

- Indi's local `run-web.sh` and `web/dev-relay.toml` are already updated (untracked). The server's systemd unit needs `-config` and `LimitCORE=0`, and the tap formula (latrani/homebrew-tap) any `-allow` mention.
- Strings list with a one-line reason each, as CLAUDE.md asks.
