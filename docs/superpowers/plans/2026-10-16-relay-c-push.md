# Detachable relay C: Web Push — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** While a character's connection is held, the first MUCK output after the WebSocket drops sends one contentless Web Push, and the page's service worker shows "activity while you were away"; a tap opens or focuses Kiln.

**Architecture:** The relay gets a VAPID key (a P-256 private key file named in `relay.toml`) and serves its public half at `relay?push_key=1`. The page registers a push-only service worker (`sw.js`), subscribes with that key once notifications are granted, and hands Go the subscription's endpoint (`kiln.push`). Each link whose character's `notify` isn't `none` sends `{t:"push", endpoint}` after every attach. The relay keeps the endpoint on the hold and, once per absence, POSTs an empty body to it with a VAPID `Authorization` header. Contentless push needs no payload encryption, so only the endpoint travels.

**Tech Stack:** Go (`internal/relay`, new `internal/relay/push`, `internal/relay/wire`, `web/wsdial`, `web/resume`, `web/cmd/kiln-web`), TypeScript page and service worker (`web/ui`), headless-Chrome smoke tests over DevTools.

**Spec:** `docs/superpowers/specs/2026-10-16-detachable-relay-design.md`, "Groundwork for Web Push", plus "Relay logs" and "Docs" under "Left to the builder". A (#214), B1 (#220) and B2 (#221) are merged.

## Global Constraints

- Every displayed string, relay log lines included, lives in `internal/str/locales/en.toml`; run `go generate ./internal/str`. Commits touching locales end with a `Strings:` trailer.
- Tests build expected text from the catalog (`str.X()`, `mark`/`upTo`).
- Below 1.0, clean breaks; the page↔relay protocol changes freely (the relay serves the page).
- Desktop Kiln and the TUI don't change. Golden screens are never `-update`d.
- The relay never writes traffic to disk; its log lines carry no traffic content. A push carries no content.
- The service worker caches nothing (#176's job).
- Names in tests and docs: Kit, Rook, Ash, world `fm`. The README is not touched.
- Before pushing web changes, run `GOOS=js GOARCH=wasm go test -exec testdata/memfs_exec.sh ./...` in `web/`; tests needing a network are `//go:build !js`.

## Decisions this plan makes

- **Only the endpoint travels.** A contentless push is a POST with an empty body; the subscription's `keys` (for payload encryption) are never needed, so the page sends `endpoint` alone.
- **"Notifications allowed" for a character** = the browser has granted notifications (the page only subscribes then) **and** the character's `notify` isn't `none`. Go decides the second at dial time (`resume.Store.Dial`). A `notify` change takes effect at the link's next attach; a page subscribing later (the player clicks Allow) reaches every live link at once.
- **Once per absence** = a `pushed` flag on the hold, cleared at every attach. Output while attached never pushes.
- **VAPID key:** `vapid_key = "PATH"` in `relay.toml` (relative to the config file's directory), a PEM PKCS#8 P-256 private key, made with `openssl` (docs). `push_contact` (an `https:` URL, or `mailto:`) is required with it: push services want a `sub` claim. No key: no push, `?push_key=1` answers 404, the page never subscribes. Both reload live with the rest of the file.
- **JWT:** ES256, header `{"typ":"JWT","alg":"ES256"}`, claims `aud` (the endpoint's scheme+host), `exp` now+12h, `sub` the contact. The signature is raw `r‖s`, 32 bytes each, not ASN.1. Header: `Authorization: vapid t=<jwt>, k=<public key>`, plus `TTL: 86400`, `Urgency: high`, `Content-Length: 0`.
- **Where the relay will POST:** only `https` endpoints whose host is one of the known push services: `fcm.googleapis.com`, `updates.push.services.mozilla.com`, `web.push.apple.com`, or ends in `.notify.windows.com`. Anything else is refused at the `push` message (logged), so a page can't point the relay at an arbitrary URL. Endpoints over 2048 bytes are refused too.
- **Sending** runs on its own goroutine with a 30 s timeout, never under a lock and never inside `take`. 404 or 410 drops the hold's endpoint (logged); other failures are logged and the flag stays set (no retry storm).
- **Notification text:** title `Kiln`, body `activity while you were away` (the spec's "Kiln: activity while you were away" split the way browsers show it). `tag: "kiln-away"` (`//str:ok`), so a second push replaces the first instead of stacking.
- **Click:** focus a window client under the worker's scope if there is one, else `clients.openWindow(registration.scope)`.
- **Key rotation:** if `pushManager.subscribe` fails because an existing subscription has another key, the page unsubscribes and subscribes again. `/reset` unsubscribes.

## Review Focus

1. **A busy room while away** must push exactly once, even with output arriving in many chunks at once. Test in Task 2 (`TestPushOncePerAbsence`, 50 chunks from a goroutine, then counts POSTs after 500 ms).
2. **Resume then away again** must push again, and the resumed tab must have re-sent its endpoint (a fresh tab included). Task 2 (`TestPushAgainAfterResume`) and Task 3 (`TestLinkSendsPushAfterResume`).
3. **A slow or dead push service** must not stall buffering or the page's resume. Task 2 (`TestSlowPushDoesNotBlock`: the push server sleeps 5 s; `take` and a resume finish within 500 ms).
4. **A relay with no `vapid_key`, or a browser that refuses** (permission revoked, no `PushManager`, subscribe throws) must boot as before with nothing shown. Task 4 (`push.test.ts`: 404 key, missing API, throwing subscribe → `kiln.push` never called, no status line).
5. **A page naming a non-push URL** must not make the relay POST anywhere. Task 2 (`TestPushRefusesOtherHosts`: `http://` and `https://example.com/…` → no request, log line).

---

### Task 1: VAPID key and the push sender

**Files:**
- Create: `internal/relay/push/push.go`, `push_test.go`
- Modify: `internal/relay/config.go`, `config_test.go`, `internal/relay/server.go`, `server_test.go`, `internal/str/locales/en.toml`

**Interfaces:**
- Produces (package `push`): `ParseKey(pemBytes []byte) (*ecdsa.PrivateKey, error)`; `PublicKey(k *ecdsa.PrivateKey) string` (base64url, no padding, 65-byte uncompressed point); `Allowed(endpoint string) bool`; `Send(ctx context.Context, c *http.Client, k *ecdsa.PrivateKey, contact, endpoint string, now time.Time) (gone bool, err error)`; `var Hosts = []string{…}` (exact hosts; a leading `.` means a suffix), which `Allowed` reads so tests can swap it.
- Produces (package `relay`): `Config.VAPIDKey string` (`toml:"vapid_key"`), `Config.PushContact string` (`toml:"push_contact"`), `(*Config).Push() (*ecdsa.PrivateKey, string)` (nil when off); `GET relay?push_key=1` → 200 `text/plain` public key, or 404.

- [ ] **Step 1: Tests in `push_test.go`.** `TestParseKey` (a key from `ecdsa.GenerateKey(elliptic.P256())` marshalled with `x509.MarshalPKCS8PrivateKey` parses; a P-384 key, an RSA key and garbage don't). `TestPublicKey` (decodes to 65 bytes starting `0x04`). `TestAllowed` (`https://fcm.googleapis.com/fcm/send/x`, `https://updates.push.services.mozilla.com/wpush/v2/x`, `https://web.push.apple.com/x`, `https://db5p.notify.windows.com/w/?token=x` → true; `http://fcm.googleapis.com/x`, `https://example.com/x`, `https://fcm.googleapis.com.evil.net/x`, `https://notify.windows.com.evil.net/x`, a 2049-byte endpoint → false). `TestSendSignsAndIsEmpty` (httptest TLS server, `Hosts` swapped to its host: the request is POST, body empty, `TTL: 86400`, `Urgency: high`; parse `Authorization: vapid t=…, k=…`, `k` equals `PublicKey`, the JWT's header is ES256, `aud` is the server's origin, `sub` the contact, `exp` 12h after `now`, and `ecdsa.Verify` passes on the raw 64-byte signature). `TestSendGone` (404 and 410 → `gone == true, err == nil`; 500 → `gone == false`, err non-nil; 201 → neither).
- [ ] **Step 2:** `go test ./internal/relay/push` — FAIL.
- [ ] **Step 3: Implement `push.go`** with `crypto/ecdsa`, `crypto/x509`, `encoding/pem` only; no new module.
- [ ] **Step 4: Config tests in `config_test.go`.** `TestParseVAPID` (a temp dir with `relay.toml` and `vapid.pem`: `vapid_key = "vapid.pem"` + `push_contact = "https://fm.example"` → `Push()` returns the key and contact; unset → nil). Errors, each built from the catalog: a missing file → `str.RelayConfigVAPIDReading(path, err)`; not a P-256 PKCS#8 key → `str.RelayConfigBadVAPID(path, keyPath)`; key without contact → `str.RelayConfigNoContact(path)`; contact not starting `mailto:` or `https:` → `str.RelayConfigBadContact(path, contact)`; contact without key → `str.RelayConfigContactNoKey(path)`. In `server_test.go`: `TestPushKey` (with a key: 200, body `push.PublicKey(k)`; without: 404; `POST` with `push_key=1`: 405).
- [ ] **Step 5:** Run — FAIL.
- [ ] **Step 6: Implement.** `ParseConfig` resolves `vapid_key` against `filepath.Dir(path)` and reads it there. `ServeHTTP` answers `push_key=1` before upgrading, next to the `short=1` branch. Entries under `[relay]` (wording to settle with the catalog's style): `config_vapid_reading`, `config_bad_vapid`, `config_no_contact`, `config_bad_contact`, `config_contact_no_key`.
- [ ] **Step 7:** `go generate ./internal/str && go test -race ./internal/relay/...` — PASS.
- [ ] **Step 8:** Commit `relay: a VAPID key and a contentless push sender` with the `Strings:` trailer.

### Task 2: The relay pushes once per absence

**Files:**
- Modify: `internal/relay/wire/wire.go`, `internal/relay/hold.go`, `hold_test.go`, `internal/relay/server.go`, `internal/str/locales/en.toml`, `docs/web-relay.md`, `web/dev-relay.example.toml`

**Interfaces:**
- Consumes: `push.Allowed`, `push.Send`, `(*Config).Push()` (Task 1).
- Produces: `wire.Push = "push"` (page to relay: `Endpoint`; empty clears it), `wire.Msg.Endpoint string` (`json:"endpoint,omitempty"`); `held.endpoint string`, `held.pushed bool`; `Server.pushClient *http.Client` (nil: a default client), a test seam.

- [ ] **Step 1: Tests in `hold_test.go`**, each with a VAPID key in the config and an httptest TLS push server counting POSTs (`push.Hosts` and `pushClient` pointed at it). `TestPushOncePerAbsence` (send `push`, CloseNow so it's held, the world writes 50 chunks from a goroutine → exactly 1 POST after 500 ms; the log has `upTo(str.RelayPushed(local, world))`). `TestNoPushWhileAttached` (output while attached → 0 POSTs). `TestPushAgainAfterResume` (absence, push, resume, re-send `push`, CloseNow, output → 2 POSTs). `TestPushGoneDropsTheEndpoint` (server answers 410; a second absence → no second POST; log `str.RelayPushGone(...)`). `TestPushFailureLogs` (500 → log `str.RelayPushFailed(...)`, no retry within the absence). `TestPushRefusesOtherHosts` (endpoints `http://` + the server's host, and `https://example.com/x` → no POST, log `str.RelayPushRefused(local, host)`). `TestNoPushWithoutKey` (endpoint set, no `vapid_key` → no POST, nothing logged). `TestSlowPushDoesNotBlock` (server sleeps 5 s; while it hangs, more world output is buffered and a resume gets `resumed` within 500 ms).
- [ ] **Step 2:** `go test -race ./internal/relay` — FAIL.
- [ ] **Step 3: Implement.** `read` takes `wire.Push`: store the endpoint if `push.Allowed`, else log refused (the host, never the full URL: the endpoint path is a bearer secret). `attach` clears `pushed`. `take` decides under `h.mu` (`h.ws == nil && h.endpoint != "" && !h.pushed`, key configured), sets `pushed`, and starts `go s.push(h, endpoint)` after unlocking. `push` calls `push.Send` with a 30 s context; `gone` clears `h.endpoint` only if it's still the one sent. Entries: `pushed = "pushed {ip} → {world:%q}"`, `push_gone`, `push_failed` (with `{err}`), `push_refused` (with `{host:%q}`).
- [ ] **Step 4:** `go generate ./internal/str && go test -race ./internal/relay/...` — PASS.
- [ ] **Step 5: Docs.** `docs/web-relay.md`: a "Push notifications" section after "Held connections": what's pushed (one contentless push per absence; no content leaves the relay), making the key (`openssl ecparam -name prime256v1 -genkey -noout | openssl pkcs8 -topk8 -nocrypt -out /etc/kiln-relay/vapid.pem`, owned by the relay's user, mode 600), `vapid_key` and `push_contact` in the `relay.toml` example, that changing the key invalidates every subscription (pages resubscribe on their next load), the egress note (the relay host needs outbound HTTPS to the push services listed), and that iPhones and iPads ignore it until Kiln is a Home Screen app (#176). `web/dev-relay.example.toml`: the two keys, commented out.
- [ ] **Step 6:** Commit `relay: one push per absence` with the `Strings:` trailer.

### Task 3: Go hands the endpoint to each link

**Files:**
- Modify: `web/wsdial/link.go`, `link_test.go`, `web/resume/resume.go`, `resume_test.go`, `web/cmd/kiln-web/main.go`

**Interfaces:**
- Consumes: `wire.Push`, `wire.Msg.Endpoint` (Task 2).
- Produces: `wsdial.SetPush(endpoint string)` (package-wide; sends to every live link that wants it, now); `wsdial.Options.Push bool` (this character's notifications are on); the link sends `{t:"push", endpoint}` after each attach (open, B1 same-tab resume, fresh-tab resume) when `Push` and the endpoint is set; `kiln.push(endpoint: string)` on the page's `kiln` object.

- [ ] **Step 1: Tests in `link_test.go`** over the fake relay the file already uses. `TestLinkSendsPushAfterOpen` (`SetPush(e)` then `Dial` with `Push: true` → the relay reads a `push` message with `e` after the open). `TestLinkSendsPushAfterResume` (kill the socket; the resumed socket gets `push` again; same for `Dial` with `Resume`). `TestSetPushReachesLiveLinks` (dial with no endpoint set → no `push`; `SetPush(e)` → the live link sends it). `TestNoPushWhenOff` (`Push: false` → never sent). Reset the package endpoint in `t.Cleanup`. In `resume_test.go`: `TestDialPushFollowsNotify` (`ch.Notify = notify.None` → `Options.Push` false; `notify.First` → true).
- [ ] **Step 2:** `go test -race ./web/wsdial ./web/resume` (from `web/`) — FAIL.
- [ ] **Step 3: Implement.** The package endpoint sits beside `links` under `linksMu`. `resume.Store.Dial` sets `o.Push = ch.Notify != notify.None`. `kiln-web` registers `k.Set("push", …)` next to `synced`.
- [ ] **Step 4:** Run, plus the wasm command from Global Constraints — PASS.
- [ ] **Step 5:** Commit `web: links give the relay a push endpoint`.

### Task 4: The page subscribes; the service worker shows it

**Files:**
- Create: `web/ui/src/push.ts`, `push.test.ts`, `web/ui/src/sw.ts`, `sw.test.ts`
- Modify: `web/ui/build.mjs`, `build.test.ts`, `web/ui/src/boot.ts`, `notify.ts`, `notify.test.ts`, `reset.ts` (or wherever `/reset` forgets), `internal/str/locales/en.toml`, `web/ui/src/str.gen.ts` (generated)

**Interfaces:**
- Consumes: `kiln.push(endpoint)` (Task 3), `relay?push_key=1` (Task 1).
- Produces: `push.subscribe(o: { keyURL: URL; sw: ServiceWorkerContainer | undefined; send: (endpoint: string) => void; fetch?: typeof fetch }): Promise<void>` (never throws; logs to `console.error`); `push.unsubscribe(sw): Promise<void>`; `sw.ts` exports `onPush(self)` and `onClick(self, e)` for tests and wires them when it runs as a worker; `build.mjs` writes `sw.js` beside `app.js` (its own esbuild entry, `format: "iife"`, no sourcemap needed).

- [ ] **Step 1: `push.test.ts`** with fakes for `fetch`, `ServiceWorkerContainer`, `PushManager`: granted + key → `register("sw.js")`, `subscribe({userVisibleOnly: true, applicationServerKey})` with the decoded key bytes, `send(endpoint)`; an existing subscription with the same key is reused (no `subscribe`); one with another key → `unsubscribe` then `subscribe`; key fetch 404 → nothing registered, `send` not called; `sw` undefined → no throw; `subscribe` rejects → no throw, `send` not called. `unsubscribe` with a subscription calls its `unsubscribe`.
- [ ] **Step 2: `sw.test.ts`.** `onPush` calls `registration.showNotification(str.webPushTitle(), { body: str.webPushBody(), tag: "kiln-away" })`. `onClick` closes the notification; with a window client it calls `focus()`; with none it calls `clients.openWindow(registration.scope)`.
- [ ] **Step 3:** `npm test` (in `web/ui`) — FAIL.
- [ ] **Step 4: Implement.** Entries under `[web]`: `push_title = "Kiln"`, `push_body = "activity while you were away"`; `go generate ./internal/str`. `boot.ts` calls `push.subscribe` after `kiln.start` when `notify.permission() === "granted"`, with `keyURL = new URL("relay?push_key=1", location.href)`. `notify.ask` calls it after a grant (pass the subscriber in, so `notify.test.ts` checks it's called on grant and not on deny). `/reset` awaits `push.unsubscribe` before the wipe. `build.test.ts` checks `sw.js` is written.
- [ ] **Step 5:** `npm test && npm run build` (or the repo's build script) — PASS; `go test ./internal/str` — PASS (no stray literals).
- [ ] **Step 6:** Commit `web: subscribe to push; a push-only service worker` with `Strings: web.push_title, web.push_body (new)`.

### Task 5: Smoke test and the hand check

**Files:**
- Modify: `web/smoke/` (a new `push_test.go`, using the existing harness), the smoke relay config the harness writes

**Interfaces:**
- Consumes: everything above.

- [ ] **Step 1: `TestServiceWorkerShowsPush`.** The smoke relay gets a VAPID key. Over DevTools: `Browser.grantPermissions(["notifications"])` for the page's origin, load the page, wait for `navigator.serviceWorker.ready`; then `ServiceWorker.deliverPushMessage` to the registration with empty data, and wait for `registration.getNotifications()` to hold one with the catalog's title and body. Headless Chrome likely can't get a real subscription (no push service), so the test doesn't require `kiln.push` to have been called; it does check `relay?push_key=1` answered 200 (the fetch went through).
- [ ] **Step 2:** `KILN_SMOKE=1 go test ./web/smoke -run 'TestServiceWorkerShowsPush'` (from `web/`) — FAIL, then PASS after any harness fixes. Then the whole smoke suite once.
- [ ] **Step 3:** Commit `smoke: the service worker shows a push`.
- [ ] **Step 4: Hand check for Indi (in the PR description, not automated):** desktop Chrome and Firefox against a local relay with a key: allow notifications, connect Kit, kill the WebSocket (DevTools offline or close the tab without unload, e.g. kill the process), have Rook page Kit → one notification; more output → no second; click → Kiln opens or comes forward. iPhone: nothing until #176.

---

## Self-review notes

- Spec coverage: SW push-only and caching nothing (Task 4), subscription with the connection when notifications are allowed (Tasks 3–4), one contentless push per absence on any output (Task 2), catalog notification and tap-to-focus (Task 4), VAPID kept on the relay host and documented (Tasks 1–2), relay logs for push sent (Task 2), docs (Task 2).
- Left alone: subscription `expirationTime` (browsers rarely set it; a dead one comes back 404/410 and is dropped), and pushing while a desktop tab is merely hidden but attached (the relay only pushes while held; the tab's own Notify covers attached tabs).
