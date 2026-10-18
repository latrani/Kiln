# Desktop Kiln: concept outline

A desktop app for Kiln: the web page (`web/ui`) in a native window,
driven by the native Go core instead of the wasm build. The goal is
something that feels like the TUI without anyone having to open a
terminal first.

This is an outline of decisions made so far, not a spec.

## Gate

Desktop waits until the web build reaches parity with the TUI. Parity
is the hard part and desktop inherits it unchanged; the desktop shell
itself is small once web is solid.

## Shape

- **Wails v3**, not Electron. Wails is Go-native and uses the system
  webview, so there's no bundled Chromium. v3 over v2: v2 is
  single-window, has no tray API and would need a migration later.
- **Not a wrap of the web build.** Wrapping the wasm page as-is would
  keep every browser workaround and the dependency on kiln-relay. On
  desktop the native core does what the browser can't:
  - dials worlds directly (no relay, no `web/wsdial`)
  - real files, the TUI's own (no memfs, IndexedDB persist or Back up)
  - one process (no `tablock`)
  - no resume and no Web Push: there's no tab to lose
  - the OS keychain via `internal/secrets`

## Code layout

- **One core.** `internal/webapi` stays the only brain: JSON actions
  in, JSON batches out. It's already plain Go, tested natively.
- **A thin main per host.** `web/cmd/kiln-web` (wasm) stays; a desktop
  main sits beside it and wires the same loop to Wails bindings and
  events. Browser-only packages (`wsdial`, `resume`, `backup`) are
  imported only by the wasm main.
- **A boot entry per host.** `boot.ts` is the wasm host; a desktop boot
  sits beside it. `build.mjs` builds one entry per host, so the desktop
  bundle never carries memfs, persist, tablock, the service worker or
  push.
- **Capabilities, not "if desktop".** The host tells the store what it
  has (`backup`, `push`, `tabLock`, …) and the UI checks those. No
  component asks which host it's on.
- **Precedent:** `bridge.ts` already runs the page against native Go
  over a WebSocket for the golden screens. Keep those tests running
  through it.

## Behavior

- **Settings and data are the TUI's.** Same config and data
  directories, same themes.
- **Desktop and TUI at once** behaves like two TUIs at once does today.
  Not a new problem, but worth knowing: config, state and secrets
  writes are atomic (temp file + rename) but last-writer-wins, and two
  Kilns on one character append each line to its log twice.
- **Quitting closes connections**, like the TUI. No tray, no
  background process.

## Platforms

- **macOS, Windows and Linux at once.** Wails v3 builds Windows from
  any host; macOS and Linux cross-builds go through its Docker image.
  `wails3 package` makes a `.app`, an NSIS installer, and
  AppImage/deb/rpm. GitHub Actions has runners for all three.
- **No per-webview test step.** WebView2 is Chromium and WKWebView is
  Safari's engine, which web users already hit. WebKitGTK (Linux) is
  the likeliest source of bugs; fix them as testers report them.
- **Recommended:** a macOS and Windows matrix for the Go tests. Every
  workflow runs on `ubuntu-latest` today, though `.goreleaser.yaml`
  already ships the TUI for Windows, so keychain, paths and renames
  have never been tested off Linux in CI.

## Distribution

- **Unsigned for now.** No Apple Developer ID, no Windows signing.
- **Ad-hoc sign the `.app`** (`codesign --force --deep -s - Kiln.app`):
  free, and without it Apple Silicon may report the app as "damaged",
  which has no Open Anyway, only `xattr -cr`. Verify on the first
  build that this holds for Wails' bundle.
- **macOS testers** do the Gatekeeper steps: System Settings → Privacy
  & Security → Open Anyway. (Sequoia removed the Control-click
  shortcut.)
- **Windows testers** click More info → Run anyway on SmartScreen.
- **No official Homebrew cask.** Since Homebrew 5.0 those must be
  notarized.
- **No update check.** Add it once desktop is mature, at the same time
  as Apple notarization.

## Open question: `check` and `over`

The page calls two core functions synchronously as you type:
`check(field, value)` (field validation in `editor.ts` and
`PickerForm.tsx`) and `over(text, maxLineBytes, newlineMode)` (the
over-limit ranges `Input.tsx` draws during render). Wails bindings
return promises, so something has to give. Options:

1. **Port to TS** (`web/ui`, shared by both hosts).
   - No round trip; drops `kiln.over` from the wasm bridge and the
     synchronous stand-in from `bridge.ts`.
   - Two copies of the logic (TS for the page, Go for the TUI). Keep
     them together with Go-generated cases in `testdata` that both
     test suites check.
   - Offsets: the limit counts UTF-8 bytes but the page needs UTF-16
     offsets. Cases must cover emoji and combining marks.
2. **Make them async.**
   - One Go implementation, parity for free.
   - Answers arrive a frame or so late (a few ms, unmeasured). For
     `check` nobody notices. For `over`, the red part can lag or
     briefly land on the wrong characters while typing fast or
     pasting; callers hold the last answer and drop stale ones.
3. **Keep the wasm on desktop just for these two.**
   - No rewrite.
   - Desktop ships a second Go build.

A plausible split is async `check` and a TS `over`, but this is
undecided.

## Out of scope

- **Mobile.** Neither Wails nor Electron does it. iOS suspends
  background apps like it discards tabs, so a mobile Kiln would still
  need the relay. It's closer to the web build than to desktop; the
  PWA may already cover it.
