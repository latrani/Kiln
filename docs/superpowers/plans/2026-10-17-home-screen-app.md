# Home Screen web app (#176) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Kiln added to a Home Screen opens as its own app named "Kiln" with Indi's icon, and the page and wasm come from a cache when there's no network, while a connected launch always gets the newest build.

**Architecture:** The icon's SVG lives in `web/ui/icons/`, with PNGs rendered from it by a script (`rsvg-convert`) and committed. `build.mjs` copies them into the build and writes `manifest.json` (name from the catalog, colors from the built-in terminal palette). `index.html` links the manifest and icons. The existing service worker (`sw.ts`, push only so far) gains a network-first fetch handler: every GET for the page's own files goes to the network (the relay serves them `no-cache`, so an unchanged file is a cheap 304), the answer is kept in a cache, and the cache answers only when the network fails. The page registers the worker on every boot, not only when subscribing to pushes.

**Tech Stack:** TypeScript (`web/ui`: `build.mjs`, `sw.ts`, `boot.ts`), `rsvg-convert` (librsvg, already installed via Homebrew) for the PNGs, headless-Chrome smoke tests.

**Spec:** issue #176 ("What it needs"), plus Indi's calls on 2026-10-17: always check online (active development), and the Home Screen name is "Kiln". The icon is `KilnIcon.svg` in the repo root (Indi's).

## Global Constraints

- Every displayed string lives in `internal/str/locales/en.toml` (`go generate ./internal/str`); the Home Screen name and the tab title are displayed. Commits touching locales end with a `Strings:` trailer.
- Tests build expected text from the catalog.
- Web copies the TUI's look in the built-in theme: the manifest's colors are the built-in terminal background (`terminal.gen.ts`), not new ones.
- Never cache the relay (`relay`, `relay?…`, the WebSocket) or the preset (`preset/…`). A preset is seeded only on a first visit, and the relay is live.
- Below 1.0: clean breaks.
- Names in tests and docs: Kit, Rook, Ash, world `fm`. The README is not touched.
- Headless smoke tests run with the worker live now; the full smoke suite must stay green.

## Decisions this plan makes

- **Always check online = network-first, no timeout.** Each GET under the worker's scope (except the relay and preset) is fetched from the network; the cache answers only when that fails. A slow network waits for the network, as now.
- **What's kept:** each successful (`200`) same-origin GET under scope, keyed by URL without its query string, in one cache, `kiln`. To avoid rewriting the 15 MB wasm into Cache Storage on every launch, a response is stored only when its `ETag` or `Last-Modified` differs from the stored one's (or nothing is stored yet).
- **No build stamps.** Network-first can't run a stale build while online; offline, the cache holds what the last online launch got, so the files match each other. The cache is named once (`kiln`), never versioned.
- **`index.html` / `./`:** a navigation request is fetched and kept like any file, and offline it falls back to the cached `./` (or `index.html`), whichever URL it was kept under. The cache key strips `?record` and other queries.
- **The worker registers at boot** (`boot.ts`, before the wasm download). A worker controls a page only from its next load, so a first visit's files are kept starting with the second launch. Accepted: offline only matters after a launch online anyway.
- **Icons:** `icon.svg` (the favicon and a manifest icon, `purpose: "any"`), `icon-192.png` and `icon-512.png` (`purpose: "any maskable"`: the art is full-bleed stripes, so a mask's crop only trims stripes), `apple-touch-icon.png` (180 px). PNGs are committed, rendered by `web/ui/icons/render.sh`; the build doesn't need `rsvg-convert`.
- **Manifest:** `manifest.json` (Go's file server knows `.json`; `.webmanifest` would need a MIME type the relay may not have). `name` and `short_name` from a new catalog entry `web.app_name = "Kiln"`, `start_url: "./"`, `scope: "./"`, `display: "standalone"`, `background_color` and `theme_color` = `terminalDark.bg`. `index.html` also gets two `<meta name="theme-color">` tags, `terminalDark.bg` for `(prefers-color-scheme: dark)` and `terminalLight.bg` for light.
- **`<title>`** comes from the catalog too (`web.title`, already there), filled by the build like `needsJs`.
- **`/reset`** clears the `kiln` cache and unregisters the worker, after unsubscribing pushes: a reset wipes what the browser saved. (This also settles C's deferred minor "reset leaves sw.js registered".)

## Review Focus

1. **A new build reaches an online launch** even though a worker is in charge: nothing serves a cached `app.js` with a new `kiln.wasm` or the reverse. Task 2 (`onFetch` tests: online always answers from the network) and Task 3 (smoke: change a file on the relay's disk, reload, see the new one).
2. **The relay, the push-key fetch, the beacon and the preset are never cached or answered from cache.** Task 2 (`onFetch` passes them through).
3. **Offline launch after one online launch** boots Kiln to its first batch from the cache. Task 3 (smoke, DevTools offline).
4. **A failed or partial network answer** (a 404, a 500, an opaque response) is never stored over a good cached copy. Task 2.
5. **The worker update path:** a new `sw.js` takes over at once (it already calls `skipWaiting` and `clients.claim`), and an old cache shape doesn't break it. Task 2 (cache name unchanged; `activate` deletes caches not named `kiln`).

---

### Task 1: Icons, manifest and the page's head

**Files:**
- Create: `web/ui/icons/kiln.svg` (moved from `KilnIcon.svg` in the root), `web/ui/icons/render.sh`, `web/ui/icons/icon-192.png`, `icon-512.png`, `apple-touch-icon.png`
- Modify: `web/ui/build.mjs`, `web/ui/index.html`, `web/ui/src/build.test.ts`, `internal/str/locales/en.toml`, `web/ui/src/str.gen.ts` (generated), `.gitignore` (the built copies in `web/static/`), `docs/web-dev.md` (re-rendering the icons)

**Interfaces:**
- Produces: built files `manifest.json`, `icon.svg`, `icon-192.png`, `icon-512.png`, `apple-touch-icon.png` beside `index.html`; `str.webAppName()`.

- [ ] **Step 1: Tests in `build.test.ts`.** Extend "build writes the page…": the five new files exist. New test "the manifest names Kiln and its icons": parse `manifest.json`; `name` and `short_name` = `str.webAppName()`, `display` = `"standalone"`, `start_url` and `scope` = `"./"`, `theme_color` and `background_color` = `terminalDark.bg`, icons list `icon.svg` (`image/svg+xml`, `any`), `icon-192.png` (`192x192`) and `icon-512.png` (`512x512`), each icon file exists. New test "the PNGs are the sizes they say": read each PNG's IHDR width and height (bytes 16–23, big-endian) and compare with 192, 512, 180. New test "index.html links the manifest and icons": it has `<link rel="manifest" href="manifest.json">`, `rel="icon"` → `icon.svg`, `rel="apple-touch-icon"` → `apple-touch-icon.png`, both `theme-color` metas with their media queries, and `<title>` = `str.webTitle()`; no `data:,` favicon.
- [ ] **Step 2:** `npm test` (in `web/ui`) — FAIL.
- [ ] **Step 3: Implement.** `render.sh`: `rsvg-convert -w N -h N kiln.svg -o …` for each size (192, 512, 180); run it and commit the PNGs. `build.mjs` copies the four icons, writes `manifest.json` (importing `terminalDark` from `src/terminal.gen.ts`), and fills `{{title}}`, `{{themeDark}}`, `{{themeLight}}` in `index.html`. Catalog: `[web] app_name = "Kiln"` with a comment (the name under the Home Screen icon). `docs/web-dev.md`: a line on `icons/render.sh` (needs `brew install librsvg`).
- [ ] **Step 4:** `go generate ./internal/str && npm test && npx tsc` — PASS; `go test ./internal/str` — PASS.
- [ ] **Step 5:** Commit `web: icons and a manifest, so Kiln installs as an app` with `Strings: web.app_name (new)`.

### Task 2: The worker keeps the page for offline

**Files:**
- Modify: `web/ui/src/sw.ts`, `sw.test.ts`, `web/ui/src/boot.ts`, `web/ui/src/push.ts`, `push.test.ts`

**Interfaces:**
- Produces (sw.ts): `onFetch(w: Worker, req: Request): Promise<Response> | undefined` (undefined: not handled, the browser fetches it itself); `cacheName = "kiln"`; `cleanCaches(w)` for `activate`. `Worker` gains `caches: CacheStorage`-shaped parts and `fetch`. (push.ts): `forget(sw)` replaces `unsubscribe(sw)` for `/reset`: unsubscribe, delete the `kiln` cache, unregister.

- [ ] **Step 1: `sw.test.ts`**, with a fake cache (a `Map`) and a fake `fetch`. `onFetch` online: returns the network's response; the first 200 is stored; a second 200 with the same `ETag` isn't stored again (count puts); one with another `ETag` replaces it. Offline (fetch rejects): returns the stored response; a navigation to `./?record` when offline returns what's stored for `./`; nothing stored → rejects (the browser's offline page). A 404, a 500 or an opaque response is returned but not stored. Not handled (returns `undefined`): POST, a URL outside the scope, `relay`, `relay?push_key=1`, anything under `preset/`. `cleanCaches` deletes every cache but `kiln`.
- [ ] **Step 2: `push.test.ts`:** `forget` unsubscribes, deletes the `kiln` cache and unregisters; with no worker or registration it does nothing and doesn't throw.
- [ ] **Step 3:** `npm test` — FAIL.
- [ ] **Step 4: Implement.** `sw.ts` wires `fetch` (`e.respondWith` only when `onFetch` returns a promise) and `activate` (claim + `cleanCaches`). `boot.ts` registers `sw.js` right after the tab lock, before the wasm download, when `navigator.serviceWorker` exists, catching failures (an `http:` page off localhost has none). `/reset` calls `push.forget`.
- [ ] **Step 5:** `npx tsc && npm test` — PASS.
- [ ] **Step 6:** Commit `web: the service worker keeps the page for offline, always checking online first`.

### Task 3: Smoke tests and docs

**Files:**
- Create: `web/smoke/offline_test.go`
- Modify: `docs/web-relay.md` (a short "On a phone's Home Screen" note), `docs/web-dev.md` if needed

- [ ] **Step 1: `TestOfflineLaunch`.** Launch, wait for the first batch, reload (now the worker controls the page and keeps each file), wait for the first batch again; then `Network.enable` + `Network.emulateNetworkConditions` offline, reload, and wait for the first batch: Kiln booted from the cache.
- [ ] **Step 2: `TestNewBuildReachesAnOnlineLaunch`.** Launch, reload once (worker in charge), then write a marker into the served `app.css` on disk (append `/* kiln-smoke-new */`), reload, and check `fetch("app.css").then(r => r.text())` from the page (which goes through the worker) contains the marker. The harness needs the static dir from `buildPage`: return it from a `launchStatic` variant, don't change `launch`'s signature.
- [ ] **Step 3:** `KILN_SMOKE=1 go test ./smoke -run 'Offline|NewBuild' -count=1` (from `web/`), then the whole smoke suite — PASS.
- [ ] **Step 4: Docs.** `docs/web-relay.md`: Kiln can be added to a Home Screen (iOS 26 opens it as an app by default); it keeps the page for offline use and checks for a new one on every online launch; on iPhone, notifications and pushes need it (and must be allowed from inside the installed app).
- [ ] **Step 5:** Commit `smoke: offline launch and a new build reaching an online one`.

Hand checks for Indi go in the PR description, not issues: add Kiln to an iPhone Home Screen (name, icon), launch it, allow notifications from inside it.
