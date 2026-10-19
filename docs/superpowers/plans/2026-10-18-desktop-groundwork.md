# Desktop groundwork Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Phases 1–3 of desktop Kiln: Go tests on macOS and Windows in CI, a `Host` the page asks instead of reaching for the wasm or `boot.ts`, and async `check`/`over` behind it.

**Architecture:** Three PRs, in order. PR A is a workflow. PR B adds `web/ui/src/host.ts` (the interface and a no-op host) and `web/ui/src/webhost.ts` (the browser/wasm host), held by the `Store`; the page and its components call `store.host`, never `globalThis.kiln` or `boot.ts`. PR C makes `Host.check` and `Host.over` return promises and makes their callers keep the latest answer and drop stale ones.

**Tech Stack:** GitHub Actions; TypeScript + Preact page (`web/ui`, `node --test` via `npm test`, `tsc` via `npm run check`); Go 1.x wasm main (`web/cmd/kiln-web`); headless-Chrome smoke tests (`web/smoke`, `KILN_SMOKE=1`).

**Spec:** `docs/superpowers/specs/2026-10-18-desktop-app-design.md` (phases 1–3 of its Order). Phases 4–5 get their own plan.

## Global Constraints

- Strings people read live in `internal/str/locales/en.toml`; tests build expected text from the catalog. No new strings are expected here; if one appears, the commit gets a `Strings:` trailer.
- Names in tests: Kit, Rook, Ash, Mira, Sable; world `fm`.
- No aliases or shims for renamed things (pre-1.0). `bridge`'s `Sync` parameter goes away, not deprecated.
- Golden screens (`web/smoke/testdata/golden`) must not change. Never run `-update`.
- Commit subjects follow the repo: `CI: …`, `Web: …`, `Smoke: …`.
- Each PR is opened and left for Indi. Never merge.
- Phase 2 adds no behavior: web looks and acts the same after PR B.

## Review Focus

1. **Typing fast past the limit:** answers to `over` can arrive out of order; the highlight must always end up matching the latest text, never an older keystroke's answer. (Task 5 test.)
2. **Editing a field, then switching or closing the editor before `check` answers:** a late answer must not mark a field whose value has moved on, nor touch a different or closed editor. (Task 4 test.)
3. **Typing a character name, then Esc before `check` answers:** the late answer must not bring back an error on a closed add form. (Task 4 test.)
4. **The input before Kiln runs** (wasm not loaded, no host answer): no highlight and no thrown error. (Task 5 test.)
5. **A host without a capability** (no `restore`, no `storageFailed`): the matching effect is a no-op, not a crash. (Task 2 test.)

---

## PR A: CI on macOS and Windows (phase 1)

Branch `ci-matrix`. Locally on 2026-10-18, `go test ./...` passed on macOS (root and `web`) and `GOOS=windows go vet ./...` was clean, so failures, if any, are expected only on Windows at run time.

### Task 1: Go tests on macOS and Windows

**Files:**
- Create: `.github/workflows/go.yml`

**Interfaces:**
- Produces: a `go` workflow with a `test` job over `os: [macos-latest, windows-latest]` (Linux stays covered by `web.yml`).

- [ ] **Step 1: Write the workflow.** Triggers: push to `main`, and `pull_request` with paths `["**.go", "go.mod", "go.sum", "web/go.mod", "web/go.sum", ".github/workflows/go.yml"]`. `permissions: contents: read`. Steps: `actions/checkout@v7`, `actions/setup-go@v7` with `go-version-file: go.mod`, then `go test ./...` at the root and `go test ./...` with `working-directory: web`. `fail-fast: false` so one OS's failure doesn't hide the other's. A header comment in the style of `web.yml`'s says what it covers and why (keychain, paths and renames never tested off Linux).
- [ ] **Step 2: Push the branch and open the PR.** Run: `gh pr checks --watch`. Expected: both `test` jobs finish.
- [ ] **Step 3: Fix what fails, one commit per cause.** Each fix gets a test that failed on that OS first (a test that already fails there counts). If a failure needs a behavior decision (for example, what the keychain should do on Windows), stop and bring it to Indi instead of choosing.
- [ ] **Step 4: Verify.** Run: `gh pr checks`. Expected: `go / test (macos-latest)` and `go / test (windows-latest)` pass, `web` passes.
- [ ] **Step 5: Commit** (if Step 3 found nothing, the workflow is the only commit): `CI: test Go on macOS and Windows`.

---

## PR B: the page asks a Host (phase 2)

Branch `page-host`, from `main` after PR A merges (or from PR A if Indi says to stack).

### Task 2: `Host`, the no-op host, and the Store holding one

**Files:**
- Create: `web/ui/src/host.ts`
- Create: `web/ui/src/host.test.ts`
- Modify: `web/ui/src/store.ts:252-263` (the `Store` class and constructor)
- Modify: `web/ui/src/page.tsx:19-60` (`effect`)

**Interfaces:**
- Produces, in `host.ts`:
  ```ts
  export type Ranges = [number, number][];
  // Host is what the page needs from whatever runs Go: the wasm in a
  // browser, the desktop app, the golden test's server. An optional
  // member is a capability: the page offers what it does only if the
  // host has it.
  export interface Host {
    check(field: string, value: string): string;
    over(text: string, maxLineBytes: number, newlineMode: string): Ranges;
    saveFile(name: string, bytes: Uint8Array): void;
    notify(store: Store, e: Event): void;
    restore?(done: (bytes: Uint8Array) => void): void; // backup
    storageFailed?(): void;
  }
  export const noHost: Host; // check → "", over → [], saveFile/notify do nothing; no capabilities
  ```
- Produces, in `store.ts`: `constructor(private onEffect?: (e: Event) => void, public host: Host = noHost)`.
- `effect` uses `store.host.saveFile`, `store.host.restore?.(…)`, `store.host.storageFailed?.()`, `store.host.notify(store, e)`.

- [ ] **Step 1: Write the failing tests** in `host.test.ts`:
  - `"a Store without a host asks the no-op one"`: `new Store().host === noHost`; `noHost.check("port", "x") === ""`; `deepEqual(noHost.over("abc", 1, "batch"), [])`.
  - `"SaveFile goes to the host"`: a fake host records calls; `effect(store)({ effect: "SaveFile", Name: "logs/fm/Kit.txt", Data: btoa("hi") })` → one `saveFile` call with name `"Kit.txt"` and bytes of `"hi"`.
  - `"/restore without the backup capability does nothing"`: fake host with no `restore`; `effect(store)({ effect: "Do", Cmd: "/restore" })` doesn't throw and sends no action.
  - `"/restore with it hands the picked bytes to Go"`: fake `restore` calls `done(new Uint8Array([1,2]))` → `act` gets `{ Do: "restore", Data: "AQI=" }`.
  - `"Notify goes to the host"`: one `notify(store, e)` call with the same event.
- [ ] **Step 2: Run.** `cd web/ui && npm test`. Expected: FAIL (no `host.ts`).
- [ ] **Step 3: Implement** `host.ts`, the `Store` constructor parameter, and `effect`'s four call sites. `restore`'s max size is the host's business, not `effect`'s. `/reset`'s `storageFailed` call goes through `store.host.storageFailed?.()`.
- [ ] **Step 4: Run.** `npm test && npm run check`. Expected: PASS.
- [ ] **Step 5: Commit.** `Web: the page asks a Host for what Go's host does`.

### Task 3: the web host, and nothing on the page side imports `boot.ts`

**Files:**
- Create: `web/ui/src/webhost.ts`
- Create: `web/ui/src/webhost.test.ts`
- Create: `web/ui/src/progress.ts` (gets `loadingText` from `boot.ts`)
- Modify: `web/ui/src/boot.ts` (move `download`, `pickFile` to `webhost.ts`; `toBase64` to `host.ts`; `loadingText` to `progress.ts`)
- Modify: `web/ui/src/main.tsx`, `web/ui/src/golden.tsx`, `web/ui/src/bridge.ts`, `web/ui/src/bridge.test.ts`
- Modify: `web/ui/src/store.ts:488` (`editField`), `web/ui/src/ui/PickerForm.tsx:134`, `web/ui/src/ui/Input.tsx:26-30`, `web/ui/src/ui/StatusLine.tsx:1`, `web/ui/src/ui/App.test.tsx:7`, `web/ui/src/boot.test.ts:3`
- Modify: `web/ui/src/build.test.ts` (the guard)

**Interfaces:**
- Consumes: `Host`, `noHost`, `Ranges` from Task 2.
- Produces: `export function webHost(): Host` in `webhost.ts`: `check`/`over` call `globalThis.kiln.check`/`.over` when present, else answer as `noHost`; `saveFile` = today's `download`; `notify` = `notify.ts`'s `show`; `restore(done)` = `pickFile(".zip,application/zip", kiln.maxBackup, done)`; `storageFailed` = `kiln.storageFailed?.()`.
- Produces: `bridge(store: Store, ws: WebSocket): void` (no `Sync`; it no longer sets `globalThis.kiln`). `golden.tsx` makes its `Store` with a host whose `check`/`over` use its synchronous `ask` and whose `saveFile`/`notify` do nothing.
- Callers: `store.editField` passes `this.host.check` to `edit`; `PickerForm` uses `store.host.check`; `Input`'s `over` takes the host (`over(host, text, c)`).

- [ ] **Step 1: Write the failing tests.**
  - `webhost.test.ts`, `"the web host asks the wasm's kiln once it's there"`: with `globalThis.kiln` unset, `webHost().check("port","x") === ""` and `over` is `[]`; set `globalThis.kiln = { check: (f, v) => f + "=" + v, over: () => [[1, 2]] }` → `"port=x"` and `[[1, 2]]`.
  - `webhost.test.ts`, `"the web host can back up"`: `typeof webHost().restore === "function"`.
  - `bridge.test.ts`: replace the two `kiln.check`/`kiln.over` tests with `"the bridge leaves the host alone"` (a Store with a fake host keeps it after `bridge(s, sock)`), and update the other calls to the two-argument `bridge`.
  - `build.test.ts`, `"the page's bundle carries no host"`: `esbuild.build({ entryPoints: ["src/page.tsx"], bundle: true, write: false, metafile: true, … })` (same loaders/JSX options as `build.mjs`); no input path in `metafile.inputs` ends in `boot.ts`, `memfs.mjs`, `persist.mjs`, `tablock.mjs`, `push.ts`, `sw.ts`, or `webhost.ts`.
- [ ] **Step 2: Run.** `npm test`. Expected: FAIL (no `webhost.ts`; the guard finds `boot.ts`).
- [ ] **Step 3: Implement.** Move the helpers, build `webHost()`, construct `new Store(effect, webHost())` in `main.tsx` (the effect closure needs the store, so keep `main.tsx`'s existing pattern), switch the four call sites to `store.host`, and drop `Sync` from `bridge.ts`. Every page-side file (`page.tsx`, `ui/*`, `store.ts`, `editor.ts`) ends up with no `globalThis.kiln` and no `boot.ts` import; `grep -rn "kiln\.\(check\|over\|maxBackup\|storageFailed\)\|from \"\.\.\?/boot\.ts\"" web/ui/src --include='*.ts*'` hits only `webhost.ts`, `boot.ts`, `main.tsx` and tests.
- [ ] **Step 4: Run.** `npm test && npm run check && cd .. && ./build.sh`. Expected: PASS, build succeeds.
- [ ] **Step 5: Run the goldens and smoke.** `cd web && KILN_SMOKE=1 go test ./smoke`. Expected: PASS with no golden diffs.
- [ ] **Step 6: Commit.** `Web: the browser is one Host; the page imports none`. Then open PR B.

---

## PR C: async `check` and `over` (phase 3)

Branch `async-check-over`, after PR B.

### Task 4: async `check`, with stale answers dropped

**Files:**
- Modify: `web/ui/src/host.ts` (`check` returns `Promise<string>`; `noHost.check` resolves `""`)
- Modify: `web/ui/src/webhost.ts`, `web/ui/src/golden.tsx` (resolve the same answers; `golden.tsx` may switch its `ask` to `fetch`)
- Modify: `web/ui/src/editor.ts:126-134` (`edit` loses its `check` parameter; new `checked`)
- Modify: `web/ui/src/store.ts:484-490` (`editField`)
- Modify: `web/ui/src/ui/PickerForm.tsx:132-136`
- Test: `web/ui/src/editor.test.ts`, `web/ui/src/store.test.ts`, `web/ui/src/host.test.ts`

**Interfaces:**
- Produces in `editor.ts`:
  - `edit(ed: Editor, key: string, value: string | boolean): Editor`: sets the value, clears that field's save problem, disarms Delete; leaves `live[key]` as it was (the last answer stands until the new one comes).
  - `checked(ed: Editor, key: string, value: string, msg: string): Editor`: `ed` unchanged if `ed.values[key] !== value`; else `live[key] = msg`, or deletes it when `msg` is `""`.
- `store.editField(key, value)`: applies `edit`, then for a string asks `this.host.check(key, value)` and on the answer applies `checked` only if an editor is still open with the same `target` (compare `JSON.stringify(target)`).
- `PickerForm`'s `onInput`: sets `name`, then on the answer sets `error` only if `store.state.picker?.adding` is still set and `store.state.picker.name === name`.

- [ ] **Step 1: Write the failing tests.**
  - `editor.test.ts`: rewrite the live-check test at line 68 to use `checked` with `str.editorPortNumber()`: `checked(edit(fm(), "port", "88a"), "port", "88a", msg).live.port === msg`; then after `edit(…, "port", "88")`, `checked(…, "port", "88a", msg)` leaves `live.port` as it was (stale); `checked(…, "port", "88", "")` clears it.
  - `store.test.ts`, `"a late check for a field that moved on is dropped"`: a host whose `check` returns promises you resolve by hand; `editField("port", "88a")`, `editField("port", "88")`, resolve the first with `str.editorPortNumber()`, then the second with `""` → `live.port` is undefined. Resolve them in the other order too → same.
  - `store.test.ts`, `"a check that answers after the editor closed changes nothing"`: `editField`, `leaveEditor()`, resolve → `state.picker.edit` is still undefined and no throw.
  - `store.test.ts`, `"a name check after Esc doesn't bring back the error"`: drive the store as `PickerForm` does (export its handler's logic as `nameTyped(store, name)` from `PickerForm.tsx` if that's the only way to test it), close with `setPicker({ adding: undefined, name: undefined, error: undefined })`, resolve with a message → `picker.error` stays undefined.
- [ ] **Step 2: Run.** `npm test`. Expected: FAIL.
- [ ] **Step 3: Implement** the signatures above.
- [ ] **Step 4: Run.** `npm test && npm run check`. Expected: PASS.
- [ ] **Step 5: Commit.** `Web: Go's field check answers when it can`.

### Task 5: async `over`, latest answer wins

**Files:**
- Modify: `web/ui/src/host.ts` (`over` returns `Promise<Ranges>`; `noHost.over` resolves `[]`)
- Modify: `web/ui/src/webhost.ts`, `web/ui/src/golden.tsx`
- Modify: `web/ui/src/ui/Input.tsx:24-30, 82`
- Test: `web/ui/src/ui/Input.test.tsx` (create if absent; follow `App.test.tsx`'s render setup)

**Interfaces:**
- Produces in `Input.tsx`: `export function latest<T>(): (p: Promise<T>, apply: (v: T) => void) => void`: each call supersedes the previous one; `apply` runs only for the most recent call's promise, and a rejection is logged with `console.error`, never thrown.
- `Input` keeps `ranges` in component state (initially `[]`) and, in an effect on `[value, c?.MaxLineBytes, c?.NewlineMode]`, asks `store.host.over(value, …)` through one `latest` per component. With no character or empty text it sets `[]` without asking. Ranges drawn against shorter text are fine: `Overflow`'s `slice`s clamp.

- [ ] **Step 1: Write the failing tests.**
  - `"only the latest over answer is drawn"`: `latest()` with two hand-resolved promises; resolve the second then the first → `apply` called once, with the second's value. Resolve in order → `apply` called once (the first is superseded), with the second's.
  - `"a failed over answer is logged, not thrown"`: reject the latest → no unhandled rejection; `apply` not called.
  - `"no highlight before Kiln runs"`: render `Input` with a Store on `noHost` and no character → no `.overflow` span carries the `input.over_limit` role, and the host's `over` was never called.
- [ ] **Step 2: Run.** `npm test`. Expected: FAIL.
- [ ] **Step 3: Implement** `latest` and the effect.
- [ ] **Step 4: Run.** `npm test && npm run check && cd .. && ./build.sh`. Expected: PASS.
- [ ] **Step 5: Run the goldens and smoke.** `cd web && KILN_SMOKE=1 go test ./smoke`. Expected: PASS, no golden diffs. If a golden shows the over-limit highlight, the smoke test must wait for it (wait for the thing, not a timer) rather than the golden changing.
- [ ] **Step 6: Stress the new async tests.** `cd web/ui && for i in $(seq 40); do npm test || break; done`. Expected: 40 passes.
- [ ] **Step 7: Commit.** `Web: the over-limit highlight answers when it can`. Then open PR C.
