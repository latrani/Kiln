# Allowlist Worlds Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The web build's worlds are exactly the relay's allowlist: relay.toml's `[[world]]` entries are Kiln world files (plus `id` and the relay's trust keys), the relay serves them as the preset's worlds, and the page has no + World and shows a world's host, port and TLS read-only (#237).

**Architecture:** `internal/relay` parses each `[[world]]` as a Kiln world file with an `id`, checks the Kiln part with a new `config.CheckWorld`, and keeps the world-file bytes it'll serve (relay-only keys stripped). `cmd/kiln-relay` serves those bytes at `/kiln/preset/config/worlds/<id>.toml` from the live (reloaded) config, merged into the manifest; a `-preset` dir's own `config/worlds/` is ignored with a startup warning. The page drops the + World row and draws the three fields as text.

**Tech Stack:** Go (BurntSushi/toml), Preact/TypeScript (web/ui).

**Spec:** this conversation's decisions, recorded here: the TUI keeps + World (it has no relay); world id is an `id` key on `[[world]]`; `port` is one int and `tls` a bool, as Kiln's (`ports` and `plaintext` are gone, no aliases: pre-1.0); `pin` and `tls_trust` stay the relay's trust keys; every other Kiln world key (`login`, `use`, `characters`, …) passes through to the served file; `-preset` stays for other config files, its `config/worlds/` is ignored with a warning; web world editor shows host, port and TLS read-only.

## Global Constraints

- Every user-facing string in `internal/str/locales/en.toml`; `go generate ./internal/str`; commits touching it end with a `Strings:` trailer listing keys.
- Config vocabulary (`id`, `port`, `tls`, `pin`, `tls_trust`, `ca`) stays verbatim.
- Breaking change, no aliases or "renamed to" hints for `ports`/`plaintext`.
- Names in tests and docs: Kit, Rook, Ash, Mira, Sable; world `fm`.
- Tests build expected text from the catalog.
- Tests needing a network stay `//go:build !js`; run the wasm test command in `web/` before pushing.
- TUI goldens never `-update`d (the TUI doesn't change).

## Review Focus

- A relay.toml reload (edit while running) must change the served worlds and manifest on the next page load, not only the allowlist. Test in Task 3.
- A `[[world]]` typo in a Kiln key (`logn = …`) is an error naming the world, not silently served. Test in Task 2.
- The served world file never carries `pin` (relay-only) and is a valid Kiln world file the page loads. Test in Task 2 (round-trip through `config.CheckWorld`).
- Two `[[world]]` with the same id (case-insensitively, as world ids name log folders), or the same host:port, are refused. Test in Task 2.
- With no `-preset`, worlds are still served; with a `-preset` holding `config/worlds/`, those files are neither listed nor served. Test in Task 3.

---

### Task 1: `config.CheckWorld`

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `func CheckWorld(id string, data []byte) error` — what loading `worlds/<id>.toml` with contents `data` would refuse (bad id, unknown key, no host, bad port, bad `tls_trust`, bad characters, bad looks), with the same messages. It does not resolve `use` packs (they're the player's config; the page reports a missing one when it loads).

- [ ] **Step 1: Tests** `TestCheckWorld`: `fm` with `host`/`port`/a character → nil; `logn = "x"` → error equal to `str.ConfigUnknownKey("fm.toml", "logn")`; no host → `str.ConfigHostRequired(filepath.Join("worlds", "fm.toml"))`; id `"bad id"` → `str.ConfigBadWorldId(...)`; `use = ["nosuch"]` → nil.
- [ ] **Step 2:** Run `go test ./internal/config -run TestCheckWorld`; FAIL (undefined).
- [ ] **Step 3:** Implement by calling `loadWorldData` with the defaults `loadGlobal` builds for an empty config and a flag (or nil packs map path) that skips `loadPack`; refactor the minimum to allow that.
- [ ] **Step 4:** `go test ./internal/config` PASS.
- [ ] **Step 5:** Commit.

### Task 2: relay.toml worlds are Kiln world files

**Files:**
- Modify: `internal/relay/config.go`, `internal/relay/server.go:137-161`, `internal/str/locales/en.toml` (`[relay]`)
- Test: `internal/relay/config_test.go`, `internal/relay/server_test.go:119-125`; fixtures in `web/resume/resume_test.go:72`, `web/wsdial/link_test.go:105`, `web/wsdial/wsdial_test.go`, `web/smoke/smoke_test.go:466-483`

**Interfaces:**
- Consumes: `config.CheckWorld`.
- Produces: `type World struct { ID, Host string; Port int; TLS bool; TLSTrust, Pin string; file []byte }`; `func (w World) File() []byte` — the world file to serve: the `[[world]]` table minus `id` and `pin`, TOML-encoded; `func (a *Config) Lookup(host string, port int) (World, bool)` unchanged in signature, one port per world.

- [ ] **Step 1: Tests** (rewrite `sample` in the new format; keep the existing cases' intent):
  - parse + Lookup: `tls = false` world found with `!w.TLS`; a `tls = true`, `tls_trust = "ca"` world found.
  - `File()` of a world with `login` and a `[[world.characters]]` Kit: contains `login`, `characters`, `host`, `port`, `tls`, `tls_trust`; no `pin`, no `id`; `config.CheckWorld(w.ID, w.File())` is nil.
  - refusals, each `==` its catalog string: no id (`relay.config_no_id`), bad id (`config.CheckWorld`'s message wrapped in `relay.config_bad_world`), same id twice ignoring case (`relay.config_duplicate_id`), same host:port twice (`relay.config_duplicate`), bad port incl. missing (`relay.config_bad_port`), Kiln-key typo (`relay.config_bad_world`), `tls = false` with `pin` (`relay.config_plaintext_trust`), TLS with no trust (`relay.config_no_trust`), the old `ports = [1]` (unknown setting, via `relay.config_bad_world`: no hint).
  - server: `worldConfig` helper writes `id`, `port`, `tls`; the "TLS asked of a plaintext world" test still refuses.
- [ ] **Step 2:** `go test ./internal/relay` FAIL.
- [ ] **Step 3:** Implement. Decode twice: into `Config` (struct `World` fields tagged `id`, `host`, `port`, `tls`, `tls_trust`, `pin`), and the `world` array as `[]map[string]any` for `File()`. Unknown-key check skips `world.*` keys (CheckWorld owns those). Per world: id required; `CheckWorld(id, file)` wrapped `str.RelayConfigBadWorld(path, id, err)`; `checkTrust` on `TLS`. Strings: drop `config_no_ports`; reword `config_no_trust` (`… or tls = false`), `config_plaintext_trust` (`{host} has tls = false, so …`), `config_bad_port`; add `config_no_id = "{path}: world {n} has no id"`, `config_duplicate_id = "{path}: world id {id:%q} is listed twice"`, `config_bad_world = "{path}: world {id:%q}: {err}"`. Errors name the world by id where they have one.
- [ ] **Step 4:** Update the four fixture files to the new format (smoke: the relay `[[world]]` now carries `id = "fake"` and the three characters; Task 3 drops the preset world file). `go test ./...`, `(cd web && go test ./...)`, and the wasm command PASS.
- [ ] **Step 5:** Commit with `Strings:` trailer.

### Task 3: the relay serves the allowlist as the preset's worlds

**Files:**
- Modify: `cmd/kiln-relay/web.go`, `cmd/kiln-relay/main.go`, `internal/relay/server.go` (add `func (s *Server) Config() *Config`), `en.toml` (`[relay]`)
- Test: `cmd/kiln-relay/main_test.go`; `web/smoke/smoke_test.go` (`config` writes no preset world)

**Interfaces:**
- Consumes: `World.File()`, `Server.Config()`.
- Produces: `func webRoutes(mux *http.ServeMux, web, preset string, worlds func() []relay.World, logf func(string))`.

- [ ] **Step 1: Tests** (httptest against `webRoutes`, `worlds` a func the test swaps):
  - manifest lists `config/worlds/fm.toml` with no `-preset`; GET it returns `File()`'s bytes.
  - swapping `worlds` to a list with `Rook`'s world `bay` instead: manifest and files follow on the next request (the reload case); `fm.toml` is then 404.
  - a `-preset` with `config/config.toml` and `config/worlds/old.toml`: manifest has `config/config.toml` and the allowlist's worlds, not `old.toml`; GET `preset/config/worlds/old.toml` 404.
  - startup: `checkPresetWorlds(preset)` (new, in web.go) logs `str.RelayPresetWorldsIgnored(dir)` when `config/worlds` exists, nothing otherwise.
- [ ] **Step 2:** `go test ./cmd/kiln-relay` FAIL.
- [ ] **Step 3:** Implement: generated worlds take precedence at `/kiln/preset/config/worlds/`; the dir handler 404s that subtree. String: `preset_worlds_ignored = "{dir}: config/worlds is ignored · the page's worlds come from relay.toml"`. main.go passes `func() []relay.World { return s.Config().Worlds }` and calls `checkPresetWorlds` once at start.
- [ ] **Step 4:** Smoke `config` writes only relay.toml; `startRelay` passes no `-preset`. `go test ./...` and `(cd web && KILN_SMOKE=1 go test ./smoke)` PASS (smoke last, after Task 4).
- [ ] **Step 5:** Commit with `Strings:` trailer.

### Task 4: the page has no + World; host, port and TLS read-only

**Files:**
- Modify: `web/ui/src/picker.ts` (rows, `choose`, `selKey`), `web/ui/src/ui/Sidebar.tsx:128-132`, `web/ui/src/layout.ts:9`, `web/ui/src/editor.ts` (drop `new_world` target, `add_world` action, `id` field; mark fields), `web/ui/src/ui/EditorForm.tsx` (`Row`), `web/ui/src/page.tsx:33` if it can open a new world, `web/ui/src/kiln.css`
- Test: `web/ui/src/picker.test.ts`, `layout.test.ts`, `editor.test.ts`, `ui/App.test.tsx`; `web/smoke/editor_test.go:103-…` (the + World flow goes; the world-edit flow stays)

**Interfaces:**
- Produces: `Field` gains `fixed?: boolean` (shown, not editable); `worldFields(inh)` marks `host`, `port`, `tls` fixed.

- [ ] **Step 1: Tests:** `pickable(rows)` ends at the last world's `+fm`, no `"+"`; `chipsWidth()` is `2 + max(openConnection, 1 + addCharacter)`; the picker list HTML has no `str.pickerAddWorld()`; a world editor renders host's value as text in `form.hint`'s class with no `<input id="ed-host"`, TLS as its box with no checkbox; `saveAction` still sends the world's Host/Port/TLS unchanged.
- [ ] **Step 2:** `(cd web/ui && npm test)` FAIL.
- [ ] **Step 3:** Implement. A fixed field is its label then its value (`[x]`/`[ ]` for the toggle) as text in `form.hint` (dim: can't change), skipped by focus and arrow movement. Remove `new_world` paths on the page; Go's `add_world` action stays (unused by the page; TUI's AddWorld shares it).
- [ ] **Step 4:** `npm run check && npm test` PASS; rebuild; smoke PASS.
- [ ] **Step 5:** Commit.

### Task 5: docs and examples

**Files:**
- Modify: `docs/web-relay.md` (allowlist section, Preset section, examples), `docs/web-dev.md` (setup: no preset.local; edit + World/`+ World` mention), `web/dev-relay.example.toml`
- Delete: `web/preset.example/` (it held only a world)

- [ ] **Step 1:** Rewrite the relay.toml examples in the new format with `id = "fm"`, a `[[world.characters]]` Kit; say Kiln world keys pass through to players, `pin`/`tls_trust` are the relay's, `-preset` is for other config and its `config/worlds` is ignored; dev setup is `cp web/dev-relay.example.toml web/dev-relay.toml` and run without `-preset`.
- [ ] **Step 2:** `go test ./internal/str` (no stray literals) PASS; `git grep -n "ports = \[\|plaintext = true\|preset.local"` returns nothing outside `docs/superpowers`.
- [ ] **Step 3:** Commit.
