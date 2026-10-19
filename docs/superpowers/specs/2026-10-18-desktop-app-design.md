# Desktop Kiln

Decided with Indi on 2026-10-18, from `docs/concepts/desktop-app.md`,
which this replaces where they differ.

## Why

Kiln already serves the people it's for: players coming from another
MUCK client, and newcomers drawn to PFMuck. Today they reach it through a
terminal or a browser tab. Desktop is the same app in a native window,
so nobody has to open a terminal first and nothing depends on the relay.

**Same functionality.** This is a port, not a new product: no new
features, no desktop-only UI. Where desktop differs from web, it's
because the native core can do what the browser can't, or because the
OS requires it.

## Goals

- The web page (`web/ui`) in a native window on macOS, Windows and
  Linux, driven by the native Go core (`internal/webapi`), not wasm.
- The TUI's config, data and themes: same directories, same files.
- Shipped from the same tag and the same GitHub Release as the TUI, and
  as a cask in `latrani/homebrew-tap`.

## Non-goals

- A tray, a background process, multiple windows.
- Kiln commands in a menu bar.
- An update check, signing or notarization (later, together).
- Mobile. The web build and its PWA cover phones.
- A per-webview test step.

## Anti-goals

- A wrap of the wasm page. Desktop carries none of the browser
  workarounds: no relay, `wsdial`, memfs, IndexedDB persist, Back up,
  `tablock`, resume, service worker or Web Push.
- Components that ask which host they're on.

## Gate

The concept doc held desktop until web reached parity. Indi: web is at
parity (#125 closed 2026-10-17). The gate is open.

## Decided

### Framework

**Wails v3, pinned to an exact beta.** v3 is beta (beta.30 on
2026-10-15, no date for stable), but its desktop API is called stable,
and the release pipeline below depends on its tooling: `wails3 package`
for the `.app`, NSIS installer and AppImage/deb/rpm, and its Docker image
for cross-builds. Moving off a beta later touches only the desktop main
and desktop boot. Pin per the repo's pinning habit; bump deliberately.

### Code layout

- **One core.** `internal/webapi` stays the only brain: JSON actions in,
  JSON batches out.
- **A main per host.** `web/cmd/kiln-web` (wasm) stays. A desktop main
  sits beside it and wires the same loop to Wails bindings and events.
  `wsdial`, `resume` and `backup` are imported only by the wasm main.
  Desktop doesn't provide `Backup`/`Restore` to the loop.
- **A boot per host.** `boot.ts` stays the wasm host; a desktop boot sits
  beside it. `build.mjs` builds one bundle per host, and the desktop
  bundle carries none of memfs, persist, `tablock`, the service worker or
  push.
- **Capabilities, not "if desktop".** The host tells the store what it
  has (`backup`, `push`, `tabLock`, …) and the UI checks those. The store
  already takes host hooks (`onUseHere`, `reset`, `subscribe`,
  `notifyOffer`); capabilities extend that pattern.
- **Golden screens** keep running through `bridge.ts`.

### `check` and `over`: async

The page calls `check(field, value)` (`editor.ts`, `PickerForm.tsx`) and
`over(text, maxLineBytes, newlineMode)` (`Input.tsx`) synchronously.
Wails bindings return promises, so both become async on every host:

- One Go implementation each (`app.CheckField`, `webapi.OverRanges`).
  No TS port: `check`'s messages come from the catalog, and `over`
  segments graphemes with `uniseg`, which a webview's `Intl.Segmenter`
  could disagree with.
- The wasm host answers at once behind the same promise. `bridge.ts`'s
  synchronous stand-in goes away; golden.tsx answers over the socket.
- Callers keep the last answer and drop stale ones. The `over` highlight
  may trail a keystroke by about a frame; text never moves.

### Behavior

- **Settings and data are the TUI's.** Running desktop and TUI at once
  behaves like two TUIs at once does today (atomic writes,
  last-writer-wins, a line logged twice if both have one character).
- **Dials worlds directly**, no relay. Passwords go through
  `internal/secrets` (the OS keychain), honoring `password_store`.
- **No worlds on first launch**, like the TUI. Desktop ships no preset
  and seeds nothing; the web build's preset is a relay deployment's, not
  Kiln's.
- **Closing the window quits**, and quitting closes connections, like
  the TUI. Cmd+Q and Alt+F4 quit.
- **Quitting asks first while worlds are connected**, the desktop form
  of the TUI's press-again guard: closing the window or Cmd+Q / Alt+F4
  shows a confirm. With nothing connected, it quits at once.
- **Exports open the OS Save dialog**, the OS's own way, as web uses the
  browser's download. `export_dir`, when set, is where the dialog starts.
- **`/reset`, `/backup` and `/restore` are unknown commands**, as in the
  TUI (my call): reset on desktop could only reach the real config.
- **Notification permission** goes through the page's existing offer
  (`notifyOffer`), as a capability that asks the OS (my call). Wails'
  macOS notifications may need a real signature: verify on the first
  build whether an ad-hoc one delivers, and bring it back to Indi if
  not rather than working around it.
- **XDG variables set only in a shell** aren't seen by an app launched
  from Finder or Start, so desktop may read different directories from a
  TUI that has them. Accepted; say so in the docs (my call).
- **Notifications are OS notifications** through Wails v3's notifications
  service, triggered by the same `Notify` effect. `level` works as it
  does today; `method` (osc/bell) doesn't apply on desktop, as on web.
- **Menus:** none of Kiln's own. macOS gets the system App, Edit and
  Window menus, because without an Edit menu Cmd+C/V/X/A don't work in
  WKWebView. Windows and Linux get no menu bar. Kiln's keys stay in the
  page.
- **Links open in the system browser**, never in the app window.

### Platforms and CI

- macOS, Windows and Linux at once, via `wails3 package`.
- **A macOS and Windows matrix for the Go tests.** Every workflow runs on
  `ubuntu-latest` today, though goreleaser already ships the TUI for
  Windows, so keychain, paths and renames have never been tested off
  Linux in CI.
- No per-webview test step. WebKitGTK bugs get fixed as testers report
  them.

### Distribution

- **Same tag, same GitHub Release** as the TUI, through `release.yml`.
- **Unsigned.** Ad-hoc sign the `.app` (`codesign --force --deep -s -`)
  so Apple Silicon doesn't call it "damaged"; verify that holds for
  Wails' bundle on the first build.
- **A cask in `latrani/homebrew-tap`**, installing `Kiln.app` into
  `/Applications`, with a `postflight` that clears quarantine
  (`xattr -dr com.apple.quarantine`). Homebrew only disables
  Gatekeeper-failing casks in `homebrew/cask`; Homebrew's project leader
  points third-party taps at exactly this. The tap's `bump.yml` updates
  the cask from the same `kiln-release` dispatch as the formula, and the
  formula stays as it is.
- **Direct downloads** get the Gatekeeper steps: macOS testers use System
  Settings → Privacy & Security → Open Anyway (Sequoia removed the
  Control-click shortcut); Windows testers click More info → Run anyway.
- **Known gotcha, accepted:** keychain approval is tied to the code
  signature, and an ad-hoc signature changes every build, so macOS asks
  for keychain access again after each update. Say so in the tester
  notes; it goes away with a Developer ID.

## Order

One spec, built in phases. The first three are useful on their own and
each lands as its own PR:

1. macOS and Windows matrix for the Go tests.
2. Capabilities in the store, replacing any host checks in the UI.
3. Async `check` and `over` on the wasm host and the golden bridge.
4. The desktop main and desktop boot.
5. Packaging, release and the tap cask.

## Left to the builder

- Window size and position, and whether to remember them.
- The quit confirm's wording (in `en.toml`) and whether it's a native
  dialog or the page's.
- Whether the `Notify` effect reaches the OS from the Go side or through
  the page.
- How the desktop boot receives batches (Wails events or a binding).
- Which exact v3 beta to pin.
- Where the desktop main lives (beside `web/cmd/kiln-web` or under
  `cmd/`).

The repo's rules hold throughout: user-facing strings in `en.toml`, colors
from theme roles, the web build copying the TUI, no aliases for renamed
things, and the stock names (Kit, Rook, Ash, Mira, Sable; world `fm`).
