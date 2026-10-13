# Web UI, step 3b: the input and the password form — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** You can type on the web page. The input sends lines and commands, recalls history with ↑/↓, keeps a draft through switches, start-up and leaving the tab, marks what runs past the line limit as the TUI does, and shows the TUI's hints when there's nothing to send. The password form logs in with a password manager. `/backup` and `/restore` work again.

**Architecture:**
- **Go:**
  - An `input` for an open character that isn't active any more sets that character's draft instead of being dropped, so typing survives a switch in flight.
  - `webapi.OverRanges` says where text runs past the line limit, in the browser's string offsets. It's exported to the page as `kiln.over`.
  - The loop does `/backup` itself (zip, then a `SaveFile` effect) and takes a `restore` action with the file's bytes. kiln-web passes `web/backup`'s `Zip` and `Unzip` in through `Options`.
- **`src/input.ts`:** the input's rules as pure functions:
  - when to take Go's text, which follows the `Ack` numbering;
  - which key does what;
  - when ↑/↓ mean history.
- **`src/ui/Input.tsx`:** a `<textarea>` that grows with what you type, the `›` gutter, the hint as its placeholder, and an overlay that draws the over-limit part.
- **`src/ui/PasswordForm.tsx`:** a real `<form>` with a hidden `char@world` username.
- **`src/typing.ts`:** the page's typing state, which lives outside Preact so `boot.ts` and the switching code can flush it. It covers the debounced `input`, flushing before `switch` and before `save_drafts`, and the `N` counter.

**Tech Stack:** as step 3a; no new packages.

**Spec:** `docs/superpowers/specs/2026-10-12-web-ui-design.md`, sections:
- "Keys": Enter / Shift+Enter, ↑/↓, PgUp/PgDn, Esc.
- "States around the main screen": Password, Backup and restore.
- "The page's code": the #141 items.
- "Accessibility".

The TUI is the reference for everything the spec doesn't say:
- `internal/ui/input.go`: `Render`'s over-limit marking.
- `internal/ui/view.go`: `prompt`, which picks the hint.
- `internal/ui/model.go`: the keys.

The part 2 API spec covers `Ack`, drafts and the over-limit confirmation.

**Rulings (for review):**
1. **The hints are the textarea's placeholder** (`Disconnected · Enter to connect`, `Connecting…`, `Connection failed · Enter to retry`, `Certificate changed · /trust to accept it`, `Hit Enter or Ctrl+O to choose a connection`). The TUI shows them only while the input is empty, and a placeholder behaves the same way. They're drawn in `input.hint`.
2. **The over-limit marking is the TUI's.** The part of a line past `max_line_bytes` is drawn in `input.over_limit`; with `newline_mode = "flatten"`, it's everything past the limit counted across lines. A textarea can't color part of its text. While something is over, an overlay behind the textarea draws the text, with the textarea's own text transparent. The rest of the time it's a plain textarea.
3. **Enter with nothing open, and an empty input, does nothing for now.** The TUI opens the picker, which comes in step 4. Enter with a command typed still sends it.
4. **Focus goes to the input on load and after a switch**, as a chat app does and as the TUI's always-typing works. A printable key pressed with focus outside any control (the scrollback, the page) goes to the input. Keys pressed in buttons, links and the sidebar don't.
5. **`/backup` is done in Go and the page downloads the file. For `/restore`, the page opens a file picker.** The picker opens from the effect, a moment after your Enter. Chrome and Firefox allow that, since the keypress still counts as recent. Safari may not; that's in the "for you to check" list.
6. **Not in 3b:**
   - The `Notify` effect (best effort on the web, decided in part 2; it needs a permission prompt the TUI doesn't have). I'll file an issue.
   - `Copy` (log mode, step 5).
   - The `/open`, `/log` and `/edit` effects (steps 4 and 5).
7. **Strings:**
   - **New:** `web.input` ("Send to {name}") and `web.input_idle` ("Command"), the textarea's accessible names.
   - **Brought back:** `status.backup_failed`, `status.restore_failed`, `status.restored` (a plural table: "1 file" / "n files" does change), and `backup.file_name`, which part 2 removed with the shim.

## Global Constraints

- Every person-facing string from the catalog (`[web]` for new ones; reuse `view.*` and `status.*` where the TUI has the text). Commits that change `en.toml` end with `Strings:` listing the keys.
- TS tests build expected text from `str.gen.ts`; Go tests from `str`.
- Colors only from the theme (`roleClass`, the role variables); glyphs only from `str.glyph`.
- Go decides; the page draws. The page never decides what a line, a command or a limit is.
- The TUI is unchanged; golden screens NOT updated.
- Before each commit: `go vet ./... && go test ./...`; `(cd web && go vet ./... && go test ./... && GOOS=js GOARCH=wasm go vet ./...)`; `(cd web/ui && npm run check && npm test)`; `web/build.sh`. For Tasks 5–9, also `(cd web && KILN_SMOKE=1 go test -count=1 ./smoke)`.
- Never `git checkout` a file to undo a mutation test; undo it with the same edit in reverse.
- The password never goes through `input` (drafts), never into the store's history, never into `kilnPage.sent`. It reaches Go only as `submit`'s text while the form is up.

## Review Focus

1. **Typing while Go's reply is in flight.**
   - You type during a history recall: the recalled text mustn't overwrite what you typed after pressing ↑.
   - You type between clicking another character and its `Active`: those keys belong to the first character, and its draft keeps them.
   - Covered by Task 4's tests and Task 9's smoke.
2. **The password and the record.** It isn't in `kilnPage.sent` (the ?record handle) or in any `input` action. → Task 6's test and smoke.
3. **Over-limit marking with multi-byte text** (é, emoji) and flatten mode: offsets in UTF-16, the bytes counted in UTF-8. → Task 2's Go table.
4. **Leaving the tab with unsent typing.** `save_drafts` must come after the pending `input`. → Task 7's test.
5. **↑ in a multi-line draft** moves the caret up a row, and only on the first row does it recall history. → Task 4's test and the smoke step.

---

### Task 1: an `input` for a character that's no longer active keeps its draft

**Files:** Modify `internal/webapi/actions.go`, `internal/webapi/loop_test.go`

- [ ] **Step 1: Failing test:**

```go
func TestInputForACharacterSwitchedAwayKeepsItsDraft(t *testing.T) {
	h := start(t)
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	h.act(`{"Do":"open","Key":"fm/rook"}`)
	h.act(`{"Do":"switch","Key":"fm/rook"}`)
	h.act(`{"Do":"input","Key":"fm/kit","Text":"half a pose","N":7}`) // typed before the page heard of the switch
	if got := h.a.Draft("fm/kit"); got != "half a pose" {
		t.Errorf("Kit's draft %q", got)
	}
	if got := h.a.Text(); got != "" {
		t.Errorf("Rook's input %q: Kit's typing went to Rook", got)
	}
}
```

Run `go test ./internal/webapi -run SwitchedAway` → FAIL (the draft is "").

- [ ] **Step 2:** In `do`, in the `input`/`submit`/`older`/`newer` case, before the `break`:

```go
		if act.Key != k {
			// The page hadn't heard the active input changed. Typing for a
			// character that's still open is its draft; anything else is
			// dropped rather than sent somewhere else.
			if act.Do == "input" && a.Char(act.Key) != nil { //str:ok
				a.SetDraft(act.Key, act.Text)
				l.f.Told(act.Key, act.N)
			}
			break
		}
```

Check that `SetDraft` while that character's password prompt is up doesn't touch the password. If it does, skip characters with `NeedPW`, and add that case to the test. Run → PASS. **Commit** `webapi: typing for a character switched away from is its draft (#141)`.

### Task 2: where text runs past the line limit

**Files:** Modify `internal/webapi/loop.go` (beside `OverLimit`, line ~279), `internal/webapi/loop_test.go`, `web/cmd/kiln-web/main.go`

**Interfaces:**
- Produces: `func OverRanges(text string, max int, mode string) [][2]int`, which gives `[start, end)` in UTF-16 code units, with `nil` meaning nothing is over. `kiln.over(text, max, mode)` returns it as an array of pairs.

- [ ] **Step 1: Failing table test**, mirroring `internal/ui/input.go`'s `Render`:
  - Per line, red from the first character whose last byte goes past `max` to the end of that line.
  - With `mode == "flatten"`, the count runs across lines with one byte per joining newline, and once red, everything after is red.
  - `max <= 0` means never.

```go
func TestOverRanges(t *testing.T) {
	for _, c := range []struct {
		text string
		max  int
		mode string
		want [][2]int
	}{
		{"12345", 5, "batch", nil},
		{"123456", 5, "batch", [][2]int{{5, 6}}},
		{"123\n456789", 5, "batch", [][2]int{{9, 10}}},
		{"123\n456", 5, "flatten", [][2]int{{5, 7}}}, // "123 456": the 6th byte is "5"
		{"éééé", 5, "batch", [][2]int{{2, 4}}},       // é is 2 bytes: the 3rd é ends at byte 6
		{"ab😀", 3, "batch", [][2]int{{2, 4}}},       // the emoji is 4 bytes, 2 UTF-16 units
		{"anything", 0, "batch", nil},
	} {
		if got := OverRanges(c.text, c.max, c.mode); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q max %d %s: %v, want %v", c.text, c.max, c.mode, got, c.want)
		}
	}
}
```

Run → FAIL (undefined).

- [ ] **Step 2: Implement.** Walk the runes, keeping a byte count (per line, or the running joined total) and a UTF-16 offset (`utf16.RuneLen`). Open a range at the first rune that takes the count past `max`, and close it at the line's end (or, flattened, at the text's end). Export it in kiln-web beside `overLimit`:

```go
	k.Set("over", js.FuncOf(func(_ js.Value, a []js.Value) any { //str:ok
		out := []any{}
		for _, r := range webapi.OverRanges(a[0].String(), a[1].Int(), a[2].String()) {
			out = append(out, []any{r[0], r[1]})
		}
		return out
	}))
```

Run → PASS. **Commit** `webapi: where text runs past the line limit, for the page to mark`.

### Task 3: `/backup` and `/restore` again

**Files:** Modify `internal/webapi/loop.go` (`Options`), `internal/webapi/actions.go`, `internal/webapi/loop_test.go`, `web/cmd/kiln-web/main.go`, `internal/str/locales/en.toml`

**Interfaces:**
- Produces:
  - `Options.Backup func() ([]byte, error)` and `Options.Restore func([]byte) (int, error)`.
  - Action field `Data []byte` (base64 in JSON).
  - The `restore` action.
  - `/backup` becomes a `SaveFile{Name: str.BackupFileName(date), Data}` effect for the page.
  - `/restore` stays a `Do` effect for the page.

- [ ] **Step 1: Catalog:**
  - `[status]`: `backup_failed = "couldn't back up: {err}"`, `restore_failed = "couldn't restore: {err}"`, and `restored = { one = "restored 1 file", other = "restored {n} files" }`.
  - `[backup]`: `file_name = "kiln-backup-{date}.zip"`.

  These are the texts part 2 removed (`git show a4f639b^:internal/str/locales/en.toml`). Run `go generate ./internal/str`.
- [ ] **Step 2: Failing tests** (`startOn` with Options that carry fakes):
  - `/backup` submitted gives an `Effect` event with `effect: "SaveFile"`, `Name` equal to `str.BackupFileName(<today>)` and the fake's bytes.
  - A failing `Backup` sets `str.StatusBackupFailed(err)`.
  - `{"Do":"restore","Data":"<base64>"}` calls `Restore` with the bytes and says `str.StatusRestored(n)`.
  - A failing `Restore` says `StatusRestoreFailed`.

  Run → FAIL.
- [ ] **Step 3: Implement.**
  - In `do`, after `Submit`, pass `effs` through `l.backup(effs)`. That replaces a `Do{Cmd: "/backup"}` with a `SaveFile` (or a status error) when `opts.Backup` is set.
  - The `restore` case: call `opts.Restore(act.Data)`. On error, say `StatusRestoreFailed`. When n > 0 and `a.Reload()` succeeds, say `StatusRestored(n)` (Reload explains itself when it fails, as the TUI's `reloadNow` did).
  - kiln-web passes `backup.Zip(cfgDir, knownHosts)` and `backup.Unzip(data, cfgDir, knownHosts)`.

  Run → PASS. **Commit** `webapi: /backup and /restore for the page` with `Strings: status.backup_failed, status.restore_failed, status.restored, backup.file_name (restored from part 2)`.

### Task 4: the input's rules, as pure functions

**Files:** Create `web/ui/src/input.ts`, `web/ui/src/input.test.ts`

**Interfaces:**
- Produces:

```ts
// Typed is the page's own record of one input (a character's key, or ""):
// the last action number it sent, and whether it's been typed in since.
export interface Typed { n: number; dirty: boolean }
// takes reports whether the page takes Go's text for an input: only
// when Go has taken the page's newest action and nothing's been typed
// since (part 2: Ack).
export function takes(t: Typed | undefined, ack: number): boolean;
// keyAction is what a key in the input does: send, history, cancel,
// scroll, or nothing (the browser's own).
export type KeyDo = "submit" | "older" | "newer" | "cancel" | "page_up" | "page_down" | undefined;
export function keyAction(e: { key: string; shiftKey: boolean; altKey: boolean; ctrlKey: boolean; metaKey: boolean; isComposing: boolean },
  caret: { start: number; end: number; value: string }): KeyDo;
```

- [ ] **Step 1: Failing tests:**
  - `takes`:
    - `takes(undefined, 0)` is true (nothing typed yet).
    - `takes({n: 3, dirty: false}, 3)` is true.
    - `takes({n: 3, dirty: false}, 2)` is false (an older reply).
    - `takes({n: 3, dirty: true}, 3)` is false (typed since).
  - `keyAction` (caret at `start`/`end` in `value`):
    - Enter → submit, and Shift+Enter → undefined (the browser's newline).
    - Enter while composing (IME) → undefined.
    - ↑ with the caret on the first row (no `\n` before `start`) → older; ↑ on a later row → undefined. ↓ on the last row → newer; ↓ before the last row → undefined.
    - ↑ with a selection (`start !== end`) → undefined.
    - Esc → cancel. PgUp → page_up and PgDn → page_down.
    - Ctrl/Alt/Meta+↑ → undefined, since they're the switching keys.

  Run → FAIL.
- [ ] **Step 2: Implement.** "Row" means the textarea's logical line (split on `\n`), as the spec says ("the input's first or last row"). Run → PASS. **Commit** `web/ui: the input's rules`.

### Task 5: the input

**Files:** Create `web/ui/src/typing.ts`, `web/ui/src/typing.test.ts`, `web/ui/src/ui/Input.tsx`; modify `web/ui/src/ui/BottomPane.tsx`, `web/ui/src/ui/Sidebar.tsx`, `web/ui/src/presence.ts`, `web/ui/src/ui/App.test.tsx`, `web/ui/src/kiln.css`, `internal/str/locales/en.toml`, `web/smoke/smoke_test.go`

**Interfaces:**
- Consumes: `takes` and `keyAction` (Task 4); `kiln.over` (Task 2).
- Produces:

```ts
// Typing holds what's typed, outside Preact, so whatever switches or
// leaves can flush it: the debounced `input`, numbered.
export class Typing {
  constructor(act: (o: object) => void, wait = 300);
  edit(key: string, text: string): void;      // typed: send `input` after `wait`
  flush(): void;                              // send what's pending now
  send(key: string, o: { Do: string; Text?: string }): void; // flush, then a numbered action for key
  takes(key: string, ack: number): boolean;
  text(key: string): string | undefined;      // the page's own text, while dirty
}
export function switchTo(state: State, typing: Typing, Key: string): void; // flush, then switch
```

- [ ] **Step 1: Catalog:** `[web]` `input = "Send to {name}"` and `input_idle = "Command"`, the textarea's accessible names. Run `go generate ./internal/str`.
- [ ] **Step 2: Failing tests** (`typing.test.ts`, with a fake `act` and fake timers via `node:test`'s `mock.timers`):
  - Edits within `wait` send one `input` with the last text and the next `N`.
  - `flush()` sends now.
  - `send("k", {Do: "submit", Text: "x"})` sends a pending `input` first, then `submit` with a higher `N`.
  - `takes("k", n)` follows `input.ts` (dirty after `edit` until Go's `Ack` reaches the number that carried the text).
  - `switchTo` sends the pending `input` before `switch`.

  In `App.test.tsx`:
  - With Kit active and connected, the bottom pane has a `<textarea>` whose `aria-label` is `str.webInput("Kit")`, and the gutter `str.glyph.echo` (`aria-hidden`).
  - Disconnected and empty, the placeholder is `str.viewDisconnected()` and has the class `roleClass("input.hint")`.
  - `Active` `""` with nothing configured gives the label `str.webInputIdle()` and the placeholder `str.viewNothingOpen()`.
  - The textarea's value is `inputs[k].Text`.

  Run → FAIL.
- [ ] **Step 3: Implement `typing.ts`, then `Input.tsx`:**
  - A `<textarea rows=1>` that grows with its content (`field-sizing: content` where supported, else set the height from `scrollHeight` on input). The bottom pane's `max-height: 50dvh` caps it, and it scrolls past that.
  - The placeholder follows the TUI's `prompt`:
    - nothing open and nothing typed: `view.nothing_open`;
    - not connected and empty: `view.cert_changed` if the server's certificate changed (`Char.CertChanged`), else `view.connecting`, `view.connect_failed` or `view.disconnected` by state.
  - `onInput` calls `typing.edit(key, value)`.
  - `onKeyDown` follows `keyAction`:
    - **submit:** with nothing open and nothing typed, do nothing (Ruling 3); else `typing.send(key, {Do: "submit", Text: value})`.
    - **older / newer:** `typing.send(key, {Do, Text: value})` and `preventDefault`.
    - **cancel:** `act({Do: "cancel"})`.
    - **page_up / page_down:** scroll the scrollback by its height less one line, and `preventDefault`.
  - The value comes from `inputs[key].Text` when `typing.takes(key, inputs[key].Ack)`, else from the page's own text. A controlled textarea keeps the caret: only write `value` when it differs.
  - **Over limit:** call `kiln.over(value, char.MaxLineBytes, char.NewlineMode)` on each render (cheap, synchronous). When it gives ranges, turn on the overlay: an `aria-hidden` `<div>` behind the textarea, with the same font, padding, wrapping and scroll, drawing the text with each range in a `<span class={roleClass("input.over_limit")}>`. The textarea's text goes transparent (`color: transparent; caret-color: var(--term-fg)`). Without ranges, there's no overlay.
  - **Focus (Ruling 4):** focus the textarea on mount and when `active` changes. A document `keydown` whose target is `body`, the scrollback or a line, and that's a printable single character with no Ctrl/Meta, focuses the textarea first (the key then lands in it).
  - `Sidebar` and `presence.ts`'s switching keys go through `switchTo`. `step` and `next_unread` flush first too. Store the `Typing` on `State` the way `act` is (`typing?: Typing`), created in `boot` with `act`.
  - The gutter is `<span aria-hidden="true">{str.glyph.echo}</span>`, in `input` (the textarea's role).
- [ ] **Step 4: Smoke** (`smoke_test.go`, replacing the `kilnPage.act` submits after login):
  - Focus the textarea and type `hello` with DevTools `Input.insertText`, then press Enter with `Input.dispatchKeyEvent` (`keyDown`/`keyUp`, `key: "Enter"`, `code: "Enter"`, `windowsVirtualKeyCode: 13`).
  - Wait for the echo. Check that the textarea is empty again.
  - Press ↑ and wait for the textarea to read `hello`.
- [ ] **Step 5:** Run → PASS. **Commit** `web/ui: the input` with `Strings: web.input, web.input_idle (new)`.

### Task 6: the password form

**Files:** Create `web/ui/src/ui/PasswordForm.tsx`; modify `BottomPane.tsx`, `App.test.tsx`, `web/smoke/smoke_test.go`

- [ ] **Step 1: Failing tests:** with Kit's `Input` `NeedPW: true`, the bottom pane has:
  - a `<form>`;
  - an `<input type="text" name="username" autocomplete="username" value="Kit@fm" hidden>` (the character's name `@` the world's id), and `<input type="password" autocomplete="current-password">` labelled by a `<label>` with `str.viewPasswordLabel("Kit")` (the label's text is visible, as the TUI draws it);
  - the hint `str.viewPasswordKeys()` in `input.hint`;
  - no `<textarea>`.

  Run → FAIL.
- [ ] **Step 2: Implement.**
  - `onSubmit`: `preventDefault`, then `typing.send(key, {Do: "submit", Text: password})`, then clear the field. Never `edit` it: no `input` actions.
  - Esc sends `cancel`, which skips the login.
  - Focus the password field when the form appears.
  - The form replaces the input while `NeedPW`.
  - Password managers find the hidden username, since it's in the same form.
- [ ] **Step 3: Smoke:** replace the smoke's password `kilnPage.act` with typing `secret` into the password field and pressing Enter. Assert that `kilnPage.sent` has no action whose `Do` is `input` with `secret` in it, and that the only action containing `secret` is one `submit`. Then wait for the login echo, as now.
- [ ] **Step 4:** Run → PASS. **Commit** `web/ui: the password form`.

### Task 7: drafts when you leave, and typing before Kiln starts

**Files:** Modify `web/ui/src/boot.ts`, `web/ui/src/boot.test.ts` (or `typing.test.ts`), `web/ui/src/ui/Input.tsx`

- [ ] **Step 1: Failing tests:**
  - `leaving(typing, act)` (a function extracted from `boot`'s handler) flushes the pending `input` before `save_drafts`, checked by the fake `act`'s order.
  - Before Kiln runs (`phase` `loading`), the textarea is enabled and keeps what's typed. When Go's first `Input` comes with `Ack: 0` and `Text: ""`, the typed text stays, since it's dirty, and it's sent as `input` once `act` exists.

  Run → FAIL.
- [ ] **Step 2: Implement.**
  - `Typing` holds edits while `act` is undefined and sends them when it's set (`store.set({ act })` → `typing.ready(act)`).
  - The leaving handler is `leaving(typing, act)`.
  - In the `stopped` phase the textarea is `disabled`, as the spec's "the controls are disabled".

  Run → PASS. **Commit** `web/ui: drafts kept when you leave and before Kiln starts (#141)`.

### Task 8: `/backup` and `/restore` on the page

**Files:** Modify `web/ui/src/main.tsx` (effects), `web/smoke/smoke_test.go`

- [ ] **Step 1:** `SaveFile` already downloads. Add the `Do` effect with `Cmd === "/restore"`:
  - Make an `<input type="file" accept=".zip,application/zip">` and `click()` it.
  - On `change`, read the file (`arrayBuffer`) and `act({Do: "restore", Data: base64})`. `btoa` over the bytes in chunks, so large files don't overflow the argument list.

  Other `Do` commands are ignored for now (Ruling 6).
- [ ] **Step 2: Smoke:**
  - Type `/backup` + Enter. Wait for a `SaveFile` effect in `kilnPage.events` whose `Name` ends in `.zip`.
  - For restore, call `DOM.setFileInputFiles` on the file input the page creates. Find it with `document.querySelector('input[type=file]')`, and keep the element in the DOM until it's chosen; the file is the backup's bytes written to a temp file by the test. Wait for the status `str.StatusRestored(n)`.
- [ ] **Step 3:** Run → PASS. **Commit** `web/ui: back up and restore`.

### Task 9: typing across a switch, end to end

**Files:** Modify `web/smoke/scroll_test.go` or `smoke_test.go`

- [ ] **Step 1: Smoke:**
  - With Tester and Second open, type `half a pose` into the textarea on Tester (don't send it). Click Second; Second's textarea is empty. Click Tester; the textarea reads `half a pose`.
  - Then type on Tester, and in the same evaluation click Second before the debounce fires. Switch back: the text is still there. (This is Task 1's Go change plus `switchTo`'s flush.)
- [ ] **Step 2: Mutation check:**
  - Drop `switchTo`'s flush: the second case fails.
  - Undo that, then drop Task 1's `SetDraft`: it still passes, because the flush covers it. Note that in the ledger: Go's change is the safety net for an `input` that was already in flight.
- [ ] **Step 3:** Run → PASS. **Commit** `web/smoke: typing kept across a switch`.

### Task 10: wrap up

- [ ] Screenshots (dark and light):
  - the input with a typed line;
  - one over the limit (`max_line_bytes = 40` in the scratch preset);
  - the password form;
  - the disconnected hint.
- [ ] File the `Notify` effect issue (Ruling 6).
- [ ] Fresh whole-branch review using this plan's Review Focus. Fix Critical and Important findings test-first, triage the minors with Indi, then open a PR with the strings, the Rulings and the screenshots described.

**For you to check by hand (Safari):** `/restore` opens the file picker; a password manager offers and fills `Kit@<world>` on the password form.
