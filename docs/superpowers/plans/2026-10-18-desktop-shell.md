# Desktop shell and packaging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Phases 4–5 of desktop Kiln: the page in a Wails v3 window driven by the native core, with the TUI's config and data, then packaged for macOS, Windows and Linux and shipped from the same release as the TUI.

**Architecture:** A new Go module `desktop/` (like `web/`, with `replace github.com/latrani/Kiln => ../`) holds the Wails app: `main.go` builds the core as the TUI does and wires a `Kiln` service (bindings) and a `kiln:batch` event to `internal/webapi`'s loop. The page gets a third entry, `web/ui/src/desktop.tsx`, with a `desktopHost()` (`web/ui/src/desktophost.ts`) that implements `Host` from PR B over the generated bindings. `build.mjs desktop <dir>` builds that entry into `desktop/frontend/dist`, which `main.go` embeds. Two TUI features the web build never had (adding worlds, editing a world's connection) come to the page behind a host capability.

**Tech Stack:** Go; Wails v3 (`github.com/wailsapp/wails/v3`, `@wailsio/runtime`); TypeScript + Preact page; GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-10-18-desktop-app-design.md` (phases 4–5). Builds on `docs/superpowers/plans/2026-10-18-desktop-groundwork.md` (`Host`, `noHost`, `webHost`, async `check`/`over`). Research notes: memory `desktop-wails-research`.

## Global Constraints

- Wails pinned to one exact tag, **v3.0.0-beta.32**, in `desktop/go.mod` and `web/ui/package.json` (`@wailsio/runtime` `3.0.0-beta.32`), and the `wails3` CLI used to build. Not below beta.31: it fixes clipboard writes from a click on macOS 27 (#6170). Bumping is its own commit.
- The `wails3` CLI is installed with `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.32` (Homebrew has only v2). If it's missing, ask Indi to run that; don't vendor or build it another way.
- Strings people read live in `internal/str/locales/en.toml` (desktop ones under `[desktop]`); commits that add them carry a `Strings:` trailer.
- Colors come from theme roles; new page UI (the add-world row and form) uses the roles the TUI's picker and editor already have.
- Same config and data as the TUI: `config.Dir()`, `config.DataDir()`. Try Kiln with `XDG_CONFIG_HOME`/`XDG_DATA_HOME` pointed at a temporary directory, never the real setup.
- No tray, no background process, no Kiln menus. Closing the window quits.
- Names in tests and docs: Kit, Rook, Ash, Mira, Sable; world `fm`.
- The web build doesn't change: no capability it lacks today appears there, and the goldens stay put.
- Each PR is opened and left for Indi.

## Review Focus

1. **Quitting while a character is connected:** Cmd+Q, Alt+F4 and the close button must all ask first; Cancel leaves everything connected; with nothing connected, quit at once. (Task 6 test.)
2. **A notification for a character you then close or delete:** clicking it must not crash or open a missing character. (Task 7 test.)
3. **Two notifications in a row:** each must show; a reused ID makes macOS update silently. (Task 7 test.)
4. **A link in server text:** opens in the system browser, never navigates the Kiln window away from the page. (Task 5 test.)
5. **The config edited outside Kiln (the TUI, a text editor) while desktop runs:** desktop reloads it, as the TUI does. (Task 1 test.)

---

## PR D: desktop Kiln (phase 4)

Branch `desktop`, from `async-check-over`.

### Task 0: one place for what a host can do

Before `Host`, the web build put its host-only hooks on the store's state (`boot.ts` sets `onUseHere`, `reset`, `subscribe`). Capabilities then live in two places. They move to `Host`, so a component checks one thing.

**Files:**
- Modify: `web/ui/src/host.ts`, `web/ui/src/webhost.ts`, `web/ui/src/store.ts` (`State`, `fresh`), `web/ui/src/boot.ts`, `web/ui/src/page.tsx` (`/reset`), `web/ui/src/ui/Splash.tsx`, `web/ui/src/main.tsx`
- Test: `web/ui/src/host.test.ts`, `web/ui/src/reset.test.ts`, `web/ui/src/ui/App.test.tsx`

**Interfaces:**
- `Host.reset?(): Promise<void>`: wipe what the host saved and start over (web: today's `resetter(...).reset`). `/reset` without it does nothing.
- `Host.useHere?(): Promise<void>`: take Kiln over from another tab (web: today's `onUseHere` body). The Splash's button shows only with it.
- `State.handingOff?: boolean`: the take-over was clicked; the button is disabled (today's clearing of `onUseHere`).
- `State.reset`, `State.onUseHere` go. `State.subscribe` goes in Task 3, which folds it into `webHost`'s `askPermission`.
- `webHost()` gets what these need from `boot` (the resetter, the tab lock) by arguments or a setter `boot` calls once they exist; it never imports `boot.ts` (the bundle guard from PR B still holds).

- [ ] **Step 1: Write the failing tests:** `/reset` reaches a fake host's `reset`, and a host without it does nothing; a reset that rejects still says `str.webResetFailed()` and stops saving, as today; the Splash's button appears with `useHere`, and after a click it's disabled (`handingOff`) and `useHere` was called once; with `noHost` there's no button.
- [ ] **Step 2: Run** `cd web/ui && npm test`. Expected: FAIL.
- [ ] **Step 3: Implement.** `grep -rn "onUseHere\|state.reset\|\.reset?\.(" web/ui/src` outside tests then hits only `webhost.ts`/`boot.ts` wiring.
- [ ] **Step 4: Run** `npm test && npm run check && cd .. && ./build.sh && KILN_SMOKE=1 go test ./smoke`. Expected: PASS, goldens unchanged (the smoke tests drive the tab handoff and `/reset`).
- [ ] **Step 5: Commit** `Web: what a host can do lives on the Host`.

### Task 1: the loop for a host with real files

**Files:**
- Modify: `internal/webapi/loop.go` (`Options`, `Run`), `internal/webapi/actions.go` (`backup`)
- Modify: `web/cmd/kiln-web/main.go` (sets the new option)
- Test: `internal/webapi/loop_test.go`

**Interfaces:**
- Produces in `webapi.Options`:
  - `PageCommands bool`: `/backup`, `/restore` and `/reset` are the page's. When false, each answers `str.StatusUnknownCommand(cmd)` on the status line, as the TUI's `model.do` does, and nothing goes to the page. The wasm main sets it to true.
  - `Changes <-chan struct{}`: each value reloads the config and theme, as the TUI's `reloadNow`. nil for none. A closed channel stops watching.

- [ ] **Step 1: Write the failing tests:**
  - `TestBrowserCommandsUnknownWithoutPage`: Run without `PageCommands`; submit `/backup`, `/restore`, `/reset` → status is `str.StatusUnknownCommand("/backup")` and so on; no `Do` effect in any batch.
  - `TestChangesReload`: Run with a `Changes` channel and a temp config dir holding world `fm`; write a second world file `ash.toml`, send on the channel → a later batch's `Worlds` lists `ash`. A theme file change applied the same way restyles (a `Theme` event follows).
- [ ] **Step 2: Run** `go test ./internal/webapi -run 'TestBrowserCommandsUnknownWithoutPage|TestChangesReload'`. Expected: FAIL.
- [ ] **Step 3: Implement.** The reload goes through `l.Call` so it runs on the loop. Make the theme reload unconditionally on a change, not only when the config changed. Set `PageCommands: true` in the wasm main.
- [ ] **Step 4: Run** `go test ./internal/webapi ./... && (cd web && go test ./...)`. Expected: PASS.
- [ ] **Step 5: Commit** `webapi: a host with real files: watched config, no page commands`.

### Task 2: adding worlds and editing their connection, behind a capability

The web build fixes a world's host, port, TLS and cert trust (the relay's) and has no way to add a world. The TUI has both (`picker.add_world` "+ World", `editor.new_world`, `internal/ui/editor.go:447`). Desktop gets them as on the TUI.

**Files:**
- Modify: `web/ui/src/host.ts` (`Host.worlds?: true`), `web/ui/src/editor.ts` (`worldFields`, a new-world target), `web/ui/src/picker.ts` (the "+ World" row), `web/ui/src/store.ts`, `web/ui/src/ui/PickerForm.tsx`, `web/ui/src/ui/EditorForm.tsx`
- Test: `web/ui/src/editor.test.ts`, `web/ui/src/picker.test.ts`, `web/ui/src/ui/EditorForm.test.tsx`

**Interfaces:**
- `Host.worlds?: true`: the host can add worlds and change their connection.
- `newEditor(e, closeAll, worlds: boolean)`: with `worlds`, host/port/TLS/cert trust are not `fixed`. A new-world editor has target `{ kind: "new-world" }`, an ID field first (checked live with `check("id", …)`), and saves with `{ Do: "add_world", ID, WorldSettings }` (the action `internal/webapi/actions.go:424` already handles).
- `pickerRows(worlds, chars, filter, addWorld: boolean)`: with `addWorld`, a final "+ World" row (`str.pickerAddWorld()`), placed and filtered as the TUI's picker places it (`internal/ui/picker.go`).

- [ ] **Step 1: Write the failing tests:** without the capability, rows and fields are exactly as today (pin with the existing tests); with it, the "+ World" row is last and choosing it opens the new-world editor; the world form's connection fields aren't fixed; saving a new world sends `add_world` with the typed ID and settings; Go's problems mark the ID field.
- [ ] **Step 2: Run** `cd web/ui && npm test`. Expected: FAIL.
- [ ] **Step 3: Implement**, reading the TUI's picker and editor for order, labels and keys. Copy the TUI; where the page already has a convention for the same thing (focus, Esc, Enter), use the page's.
- [ ] **Step 4: Run** `npm test && npm run check && cd .. && ./build.sh && KILN_SMOKE=1 go test ./smoke`. Expected: PASS, goldens unchanged.
- [ ] **Step 5: Commit** `Web: adding worlds and their connection, for a host that can`.

### Task 3: notification permission through the host

**Files:**
- Modify: `web/ui/src/host.ts`, `web/ui/src/webhost.ts`, `web/ui/src/notify.ts` (`offer`, `ask`), `web/ui/src/main.tsx`
- Test: `web/ui/src/notify.test.ts`, `web/ui/src/webhost.test.ts`

**Interfaces:**
- `Host.permission(): Promise<Permission>` and `Host.askPermission(): Promise<Permission>` (`Permission` from `notify.ts`). `noHost` answers `"unsupported"`. `webHost` wraps today's browser calls, and its `askPermission` subscribes to pushes after `"granted"` itself, so `State.subscribe` goes (the last of Task 0's moves).
- `Host.whyNoNotify(p: Permission): string`: what the status line says when notifications can't show, "" when they can. Web's are today's (`str.webNotifyDenied()`, `str.webNotifyUnsupported()`, which name the browser); desktop's come from `[desktop]` strings that point to System Settings → Notifications (Task 5 adds them).
- `offer(store)` and `ask(store)` go through `store.host`; the status-line offer is unchanged.

- [ ] **Step 1: Write the failing tests:** a fake host's answers drive `offer` and `ask` (granted → no offer; ask → the host is asked once; denied → the host's `whyNoNotify("denied")` is said, whatever it is); `webHost().whyNoNotify("denied") === str.webNotifyDenied()`.
- [ ] **Step 2: Run** `npm test`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm test && npm run check`. Expected: PASS.
- [ ] **Step 5: Commit** `Web: asking to notify goes through the host`.

### Task 4: the desktop module and its service

On the Go side desktop is the TUI: same dialing, keychain, known hosts, watcher, log writers and state. Both build their core from one constructor, so a fix to one can't skip the other. Only the dial's window size and the link opener differ.

**Files:**
- Create: `internal/native/native.go`, `internal/native/native_test.go`
- Modify: `cmd/kiln/main.go` (builds its `ui.Deps` from `native.Core`)
- Create: `desktop/go.mod` (`module github.com/latrani/Kiln/desktop`, `replace github.com/latrani/Kiln => ../`, Wails `v3.0.0-beta.32`), `desktop/main.go`, `desktop/kiln.go`, `desktop/kiln_test.go`, `desktop/frontend/dist/.gitkeep`
- Modify: `.gitignore` (`desktop/frontend/dist/*`, `!desktop/frontend/dist/.gitkeep`, `desktop/bin/`)

**Interfaces:**
- In `internal/native`, Kiln's core as a front end on this machine runs it:
  ```go
  type Options struct {
      ConfigDir, DataDir string
      Size func() (w, h int) // the dial's starting window size; nil: none, the front end reports it once known
  }
  // Core is the core's deps as the TUI's: conn.Dial with known hosts,
  // secrets for passwords, a log writer per layout, state.json, and the
  // config watcher. Close closes the writers and the watcher.
  type Core struct {
      Deps    app.Deps
      Changes <-chan struct{}
      Close   func()
  }
  func New(o Options) (*Core, error)
  ```
  `cmd/kiln` fills `ui.Deps` from it (keeping its terminal size, tmux and `openURL`); `desktop` passes `Size: nil`. The TUI's behavior doesn't change.
- `type Kiln struct` (the bound service), in `kiln.go`:
  - `Start() error`: the page's listeners are set; build the core and start `webapi.Loop.Run` (once; a second call does nothing).
  - `Act(action string)`, `Check(field, value string) string`, `Over(text string, maxLineBytes int, newlineMode string) [][2]int`: as the wasm main's `act`, `check`, `over`.
  - `SaveAs(name string, data []byte) error`: the OS Save dialog, named `name`, starting in the config's `export_dir` when set; writes the file; a cancelled dialog is not an error.
  - `OpenURL(url string) error`: `http`/`https` only, through Wails' `Browser.OpenURL`.
- The desktop's core is `native.New` (after `config.EnsureDefaults` and `config.Load`, as the TUI), its `Changes` into `webapi.Options.Changes`, its `Close` on quit.
- The loop runs with `webapi.Options{Autoconnect: true, ConfigDir: cfgDir, Drafts: <data>/drafts.json, Changes: core.Changes}`: no `PageCommands`, `Backup`, `Restore` or `Resume`.
- Batches go to the page as the Wails event `kiln:batch`, data the batch JSON as a string.
- When `Run` returns (a `/quit`), the app quits.

- [ ] **Step 0: The shared core first.** Failing tests in `native_test.go`, on temp dirs with `password_store = "file"`: `SavePassword` then `Password` round-trips for `fm`/`kit`; `DeletePassword` forgets it; `KnownHosts.Path` is `<data>/known_hosts`; `NewLog` returns the same writer for the same layout and a new one for a changed `log_dir`; writing a world `.toml` sends on `Changes`; `Close` closes the channel. Run `go test ./internal/native` (FAIL), implement by moving the code out of `cmd/kiln/main.go`, switch `cmd/kiln` to it, then `go test ./...` (PASS, TUI tests and goldens unchanged). Commit `native: the core as the TUI runs it, for any front end on this machine`.
- [ ] **Step 1: Write the failing tests** in `kiln_test.go`, with the window and dialogs behind small interfaces the test fakes: `Start` twice starts one loop; `Act` of `{"Do":"here"}` reaches the loop; `Check("port","x")` equals `webapi.CheckField("port","x")`; `SaveAs` with a fake dialog that cancels writes nothing and returns nil; `OpenURL("file:///etc/passwd")` is refused.
- [ ] **Step 2: Run** `cd desktop && go test ./...`. Expected: FAIL.
- [ ] **Step 3: Implement** `kiln.go` and `main.go`: one window titled from the catalog (`str.WebAppName()`), the page embedded from `frontend/dist`. `ApplicationShouldTerminateAfterLastWindowClosed: true`.
- [ ] **Step 4: Run** `go test ./... && go vet ./...` in `desktop/`. Expected: PASS.
- [ ] **Step 5: Commit** `Desktop: the core in a Wails app`.

### Task 5: the desktop page

**Files:**
- Create: `web/ui/src/desktop.tsx` (entry), `web/ui/src/desktophost.ts`, `web/ui/src/desktophost.test.ts`, `web/ui/src/bindings/` (generated, committed)
- Modify: `web/ui/build.mjs` (a `desktop` mode), `web/ui/package.json` (`@wailsio/runtime` `3.0.0-beta.32`), `web/ui/src/build.test.ts`, `web/ui/test.mjs` if bindings need excluding from the lint

**Interfaces:**
- `desktopHost(): Host`: `check`/`over` call the `Kiln` bindings; `saveFile` → `SaveAs`; `notify` → Task 7's binding; `permission`/`askPermission` → the notifications service; `whyNoNotify` → new `[desktop]` strings (`desktop.notify_denied`, `desktop.notify_unsupported`; the commit's `Strings:` trailer lists them); `worlds: true`; no `restore`, `storageFailed`, `reset` or `useHere`.
- `desktop.tsx` mounts the page as `golden.tsx` does: a `Store` on `desktopHost()`, `mount`, a `kiln:batch` listener that applies batches and starts the typing queue on the first (as `bridge.ts`), then `Start()`. A capture-phase click listener sends any `a[target=_blank]` to `OpenURL` and prevents the default.
- `node build.mjs <dir> desktop` writes `app.js`, `app.css`, `index.html` and fonts, no service worker, manifest or icons.
- Bindings: `wails3 generate bindings` from `desktop/`, output under `web/ui/src/bindings/`, committed like `str.gen.ts`; a test or CI step regenerates them and fails on a diff.

- [ ] **Step 1: Write the failing tests:**
  - `build.test.ts`, `"the desktop bundle carries no browser host"`: bundling `src/desktop.tsx` pulls in none of `boot.ts`, `memfs.mjs`, `persist.mjs`, `tablock.mjs`, `push.ts`, `sw.ts`, `webhost.ts`.
  - `build.test.ts`, `"the desktop build has no service worker"`: `build.mjs <dir> desktop` writes `app.js` and `index.html` and no `sw.js` or `manifest.json`.
  - `desktophost.test.ts`: with the bindings module faked, `check` and `over` resolve the binding's answer; `saveFile` passes name and bytes; `worlds === true`; `restore` is undefined.
  - `desktophost.test.ts`, `"a link opens in the system browser"`: the click listener on a fake `<a target=_blank href="https://fm.example/">` calls `OpenURL` with that URL and prevents the default.
- [ ] **Step 2: Run** `npm test`. Expected: FAIL.
- [ ] **Step 3: Implement**, and add a `desktop` target to the desktop Taskfile (Task 8) or a script that builds into `desktop/frontend/dist`.
- [ ] **Step 4: Run** `npm test && npm run check`, then `cd desktop && wails3 dev` against a scratch `XDG_CONFIG_HOME`/`XDG_DATA_HOME`: add world `fm` on a local fake MUCK (`web/smoke`'s or `internal/kilntest`'s), connect Kit, type past `max_line_bytes`, edit a setting, `/log`, export, quit. Expected: each works as in the TUI.
- [ ] **Step 5: Commit** `Desktop: the page, on the desktop host`.

### Task 6: quitting asks first

**Files:**
- Modify: `desktop/main.go`, `desktop/kiln.go`, `internal/str/locales/en.toml` (`[desktop]` quit confirm: title, message, Quit, Cancel)
- Test: `desktop/kiln_test.go`

**Interfaces:**
- `func (k *Kiln) shouldQuit() bool`: true when no character is connected; otherwise a native question dialog (Wails `Dialog.Question`) with the catalog's strings, true on Quit. Wired to `Options.ShouldQuit` (Cmd+Q, the app menu) and to a `RegisterHook(events.Common.WindowClosing, …)` that cancels the close when it returns false (the close button, Alt+F4).

- [ ] **Step 1: Write the failing tests** with the dialog faked: nothing connected → true, dialog not shown; Kit connected and Cancel → false, Kit still connected; Quit → true.
- [ ] **Step 2: Run** `cd desktop && go test ./...`. Expected: FAIL.
- [ ] **Step 3: Implement.** On macOS only, set the application menu to the roles `AppMenu`, `EditMenu`, `WindowMenu` (without an Edit menu Cmd+C/V/X/A don't work in WKWebView); no menu on Windows and Linux.
- [ ] **Step 4: Run** `go test ./...` and, in `wails3 dev` with a scratch config, Cmd+Q and the close button with Kit connected. Expected: asked both times; Cancel keeps Kit connected.
- [ ] **Step 5: Commit** with `Strings: desktop.quit_title, desktop.quit_message, desktop.quit, desktop.cancel (new)`.

### Task 7: OS notifications

**Files:**
- Modify: `desktop/kiln.go`, `desktop/main.go`
- Test: `desktop/kiln_test.go`

**Interfaces:**
- `func (k *Kiln) Notify(title, body, key string) error`: sends through Wails' notifications service with a fresh ID each time (`key` plus a counter or time): macOS silently updates a reused ID instead of showing it. `key` rides in the notification's `Data`.
- On a notification click (`OnNotificationResponse`): bring the window forward and `Act` `{"Do":"switch","Key":key}` only if that character still exists (`l.Call` checking `a.Char(key)`).
- `desktopHost().notify(store, e)` calls `Notify(e.Title, e.Body, e.Key)`.

- [ ] **Step 1: Write the failing tests** with the service faked: two `Notify` calls send two different IDs; a click for `fm/kit` switches to it; a click for a deleted character does nothing and doesn't panic.
- [ ] **Step 2: Run** `go test ./...`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS. The hand check comes with packaging (Task 8): notifications only work from an Applications folder.
- [ ] **Step 5: Commit** `Desktop: notifications are the OS's`. Then open PR D.

---

## PR E: packaging and release (phase 5)

Branch `desktop-release`, from `desktop`.

### Task 8: the app bundle

**Files:**
- Create: `desktop/Taskfile.yml`, `desktop/build/` (from `wails3 init`, trimmed to darwin, windows, linux), `desktop/build/config.yml`
- Modify: `web/ui/icons/render.sh` (a 1024px `appicon.png` from `kiln.svg` into `desktop/build/`)

**Interfaces:**
- `build/config.yml`: product name `Kiln`, identifier `io.github.latrani.kiln` (it names the keychain and notification identity, so it never changes once shipped), version from the tag.
- `wails3 package` on macOS makes a universal `Kiln.app`, ad-hoc signed (Wails' `codesign:adhoc` task, already in its Taskfile); Windows an NSIS installer; Linux AppImage, deb and rpm. The frontend step runs `node web/ui/build.mjs desktop/frontend/dist desktop`.

- [ ] **Step 1: Package on this Mac.** Run: `cd desktop && wails3 package`. Expected: `bin/Kiln.app`; `codesign -dv bin/Kiln.app` says `Signature=adhoc`, `Identifier=io.github.latrani.kiln`.
- [ ] **Step 2: Hand check, with Indi** (copy to `~/Applications`, scratch config via `launchctl setenv XDG_CONFIG_HOME …` or a test account): it opens, the "may send notifications" banner appears, a page from Rook while another app is in front shows a banner, clicking it switches to Kit's view of Rook's page. Then remove the copy.
- [ ] **Step 3: Commit** `Desktop: the Kiln app bundle`.

### Task 9: desktop builds in CI and in the release

**Files:**
- Modify: `.github/workflows/go.yml` (a `desktop` job), `.github/workflows/release.yml` (desktop jobs that upload to the same draft release)

**Interfaces:**
- `go.yml` `desktop` job: `macos-latest`, `windows-latest`, `ubuntu-latest`; installs the pinned `wails3`; runs `go test ./...` in `desktop/` and `wails3 package`. Native builds per OS, no Docker.
- `release.yml`: the same three builds after the TUI's GoReleaser job, uploading `Kiln-<version>-macos-universal.zip`, `Kiln-<version>-windows-amd64-installer.exe`, `Kiln-<version>-linux-amd64.AppImage`, `.deb`, `.rpm` to the release GoReleaser drafted, for the same tag.

- [ ] **Step 1: Verify first.** On `ubuntu-latest`, the GTK4 and WebKitGTK 6.0 dev packages install (`libgtk-4-dev`, `libwebkitgtk-6.0-dev`; fall back to Wails' GTK3 tag only if they don't, and say so in the PR). On `windows-latest`, NSIS is present or installs. Record what each needed in the workflow's comments.
- [ ] **Step 2: Write the jobs**, push, `gh pr checks --watch`. Expected: all three package.
- [ ] **Step 3: Commit** `CI: build and release desktop Kiln`. 

### Task 10: the cask and the tester notes

**Files:**
- Create: `docs/desktop.md` (installing, Gatekeeper's Open Anyway on Sequoia and later, SmartScreen's Run anyway, the notifications banner, keychain asking again after each macOS update until there's a Developer ID, shell-only XDG variables not reaching a launched app)
- Modify: `docs/README.md` (link it)
- In `latrani/homebrew-tap` (its own PR): `Casks/kiln.rb` installing `Kiln.app` to `/Applications` with a `postflight` running `xattr -dr com.apple.quarantine` on it; `bump.yml` updates it from the same `kiln-release` dispatch as the formula.

- [ ] **Step 1: Write the doc and the cask.** The cask's `sha256` and URL come from the release's macOS zip.
- [ ] **Step 2: Verify** against a test release (a prerelease tag Indi chooses; `homebrew.yml` skips prereleases, so point a local copy of the cask at its zip): `brew install --cask ./kiln.rb` installs to `/Applications`, `xattr -l /Applications/Kiln.app` shows no `com.apple.quarantine`, and it opens with no Gatekeeper prompt. Uninstall after.
- [ ] **Step 3: Commit** `Docs: desktop Kiln for testers`; open PR E and the tap PR. The tap PR changes no formula, so it doesn't need `brew pr-pull`, but say so in it.
