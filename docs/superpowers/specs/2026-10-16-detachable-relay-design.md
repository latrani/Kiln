# Detachable relay (#187)

Part of #125 (sub-project 5). Decided with Indi on 2026-10-16.

## Why

On a phone, the web build loses its MUCK connections whenever the
WebSocket to `kiln-relay` dies: switching apps, locking the phone, iOS
discarding a background tab, or "Use Kiln here" handing Kiln to another
tab (#184). Each drop means a reconnect and log-in, lines missed while
away, and the character visibly leaving and coming back on the MUCK. A
newcomer on PFMuck shouldn't have to keep a tab in the foreground just to
stay in a scene.

The relay already sits between the tab and the MUCK. When the WebSocket
drops, it keeps the MUCK connection open, holds what arrives, and hands it
over when Kiln comes back.

## What it has to survive

All of these, the same way:

- A short app switch (the tab is suspended, and its wasm survives).
- An hour or more with the phone locked. iOS may or may not discard the
  tab in that time; there's no documented limit, so assume it might.
- A tab handoff ("Use Kiln here"): the old tab reloads, and the new tab
  boots fresh wasm.
- A tab iOS discarded and reloads when the player comes back.

Indi's rule: **a non-TLS world never resumes better than a TLS one.**

## The big decision: the relay terminates TLS

Today TLS runs in the tab (Go's `crypto/tls` over `wsdial`), and the relay
only sees ciphertext. Resume can't survive a fresh wasm instance that way:
the TLS state lives only in the old tab's memory, and `crypto/tls` can't
export it. That would let plaintext worlds survive reloads while TLS worlds
couldn't, which breaks the rule above.

So, starting now, **the relay opens the TLS connection to each TLS world
itself.** The browser → relay leg stays encrypted (`wss://` through
nginx), and so does the relay → MUCK leg, but the relay process handles
traffic in the clear. One mode for every world, with no setting to choose
between them:

- In-tab TLS for the web build goes away: the web build's `conn.Dial`
  runs with `TLS: false` over `wsdial`, and the relay does TLS. Desktop
  Kiln is unchanged.
- Browser-side TLS pins go away from the web build: the preset's
  `known_hosts`, its merge in `boot.ts`, and the "certificate changed"
  path for web characters. Trust moves into the relay's allowlist (below).
- The relay's first-frame TLS check (`CodeNeedsTLS`, 4426) goes, because
  the client no longer starts TLS.

Why this is acceptable (Indi): for the current deployment the relay runs
on the same box as PFMuck, whose staff can already read everything on
PFMuck, and the other likely destination (Tapestries) is plaintext anyway.
It's a two-way door: if a more professional deployment later needs "the
relay never sees plaintext," that's a new project (a `crypto/tls` fork
that can carry connection state between tabs) that brings in-tab TLS back
with full resume. Never a weaker mode behind a setting.

### Documenting it

`docs/web-relay.md` gets a section near the top, before Installing,
replacing the "relay never sees a TLS world's plaintext" text in "TLS trust
in the browser". The wording Indi saw:

> ## What the relay can see
>
> The relay, not the player's browser, holds the TLS connection to each
> world. Traffic is encrypted between the browser and nginx, and between
> the relay and the world, but the relay itself handles it in the clear:
> passwords, pages, whispers, everything a player sends or sees. While a
> player is away, it keeps what arrives in memory until they come back or
> the hold runs out.
>
> So whoever runs the relay, and anyone who gets onto its host, can read
> its players' traffic. That's no new exposure when the relay is run by
> the world's own staff. Don't serve worlds you don't operate unless their
> players know you can read what they send.
>
> The relay never writes traffic to disk. Turn off core dumps for it
> (`LimitCORE=0` in the systemd unit).

The page doesn't tell players (Indi: operator caveat only, for now).

## How it behaves

**Hold.** When a WebSocket drops without the page saying "close", the
relay keeps the MUCK connection open and holds it. The MUCK decides
whether an idle character should stay: the relay never idles anyone out
before the hold limit (Indi). An explicit close from the page (the player
quits or closes the character, `/reset`) ends the MUCK connection right
away. A page unloading is *not* an explicit close.

**Short hold on a real unload** (Indi). When a page actually unloads
(`pagehide` with `persisted` false) and it isn't handing Kiln to another
tab (the old tab knows when it is), it tells the relay with `sendBeacon`
to let go soon, and the relay shortens that connection's hold to **10
minutes**. A refresh resumes within seconds; closing the tab lets the
character go shortly after. A phone switching apps or locking doesn't
unload the page, and iOS discards a tab without running any code, so those
keep the full hold, as do a crash or a dead battery. To check on a real
iPhone (Indi): an app switch or lock never fires a non-persisted
`pagehide`. If it does, the plan comes back to Indi.

**Buffer.** While held, and while the page hasn't confirmed receipt, the
relay keeps MUCK output in memory. When a connection's buffer is full, the
relay **drops its oldest lines** and keeps going (Indi). The player sees
one line saying lines were missed, with how many. The buffer is never
written to disk.

**Resume.** Any tab in the same browser can pick a held connection back
up: the suspended tab, the new tab after a handoff, or the reloaded tab
after a discard. Kiln puts the buffered lines in scrollback and the log
as though they'd arrived live, with the times they reached the relay, and
the character carries on: no reconnect, no log-in, no leave-and-return on
the MUCK. Scrollback shows one line saying it resumed and how long the
player was away (Indi; catalog text), after any missed-lines line.

My call: resuming isn't reconnecting, so a character with
`reconnect = off` still resumes.

**Same browser only.** No picking up a phone's held character from a
laptop (Indi: "they should just get a shell account").

**Hold runs out.** The relay closes the MUCK connection. The next time
Kiln tries to resume, the relay says the hold is gone, and Kiln falls back
to what it does today for a dropped connection (its `reconnect` setting),
after a line saying the relay let go.

## Settings (the relay's config file)

The allowlist file becomes the relay's config file (Indi): `relay.toml`,
named by `-config PATH`, which replaces `-allow`. Clean break, no alias.
Its `[[world]]` list is still "the allowlist" in the docs, and it still
reloads live. The examples, systemd unit, tap formula and smoke setup
follow the new name.

New keys go in it next to `max_per_ip` and `max_conns`, so they reload
live without dropping held connections (Indi: file, not flags). Names are the builder's, following the file's style; defaults are
my calls:

- How long a detached connection is held: **24 hours** (the figure Indi
  asked about).
- Hold after a real unload (above): **10 minutes** (Indi).
- Buffer cap per connection: **4 MB**. A busy room is roughly 3–4 MB a
  day by my rough estimate (unmeasured).
- Buffer cap for the whole relay: **256 MB**. When it's reached, the
  relay drops oldest lines from the connection holding the most.

Per-world TLS trust moves into `[[world]]`:

- `plaintext = true` stays as is.
- Otherwise the world is TLS, and its entry says how the relay trusts it:
  a real CA, or a pinned `sha256:` fingerprint in the format
  `conn.Fingerprint` makes. My call: no trust-on-first-use on the relay.
  The operator writes the pin, and a world without either is an
  allowlist error. Desktop Kiln's `known_hosts` lines are the source for
  pins.
- A pin mismatch or a failed CA check refuses the connection with its own
  close code, and Kiln says the world's certificate didn't match what the
  relay expects.

My call: if the player's world file says `tls = true` but the allowlist
says `plaintext`, the relay refuses rather than quietly sending in the
clear. The page tells the relay what the world file asks for.

## Limits (#113)

A held connection still holds a MUCK socket, so it counts against
`max_per_ip` (for the address that opened it) and `max_conns` until the
hold ends. Resuming from a new address (wifi → cellular) moves its count
to the new address. My call: if that address is already at its cap, the
resume still succeeds; the cap gates new connections, not existing ones.

## Groundwork for Web Push

Indi: build it, and accept that iPhone ignores it until #176 (a Home
Screen web app) lands. Desktop browsers and Android Chrome take Web Push
from a normal tab.

- The page registers a service worker for push only. It caches nothing:
  caching the page and wasm is #176's job.
- When a character's notifications are allowed, the page gives the relay
  a push subscription along with that connection.
- While a connection is held, the relay sends one contentless push when
  the first MUCK output arrives after the WebSocket drops, then no more
  until the page has resumed and detached again. My call: once per
  absence, which matches the `notify = "first"` default. Indi: any output
  triggers it. Kiln's notify rules live in the tab, and shipping them to
  the relay and keeping them in sync isn't clean enough for this project.
- The service worker shows a plain "Kiln: activity while you were away"
  notification (catalog text), and a tap opens or focuses the page.
- The relay needs a VAPID key pair (the keys a push service uses to
  check who's sending), generated and kept on the relay host. How it's
  made and configured is the builder's call; document it in
  `docs/web-relay.md`.

## Constraints

- Desktop Kiln and the TUI don't change, apart from shared code moving.
- Strings go through `internal/str/locales/en.toml`, relay log lines
  included; commits carry the `Strings:` trailer.
- Below 1.0: the allowlist and preset break cleanly. No compatibility for
  old pin files and no migration hints. `known_hosts` in a preset simply
  stops meaning anything; the docs drop it.
- The page and relay ship together (the relay serves the page, #182), so
  the WebSocket protocol between them can change without versioning.
- Names in tests and docs: Kit, Rook, Ash, world `fm`.

## Non-goals

- Resume across browsers or devices.
- A setting to keep TLS in the tab.
- Telling players on the page who can read their traffic.
- Caching the page or wasm for offline use (#176).
- Push content (who paged, what they said).

## Anti-goals

- A TLS world resumes worse than a plaintext one.
- A held connection drops because the WebSocket died, during the hold.
- Resuming re-sends the log-in, or the character visibly leaves and
  returns on the MUCK.
- Traffic or the buffer touching disk on the relay.
- Duplicated or silently missing lines after a resume. Lost lines are
  only ever the counted drop from a full buffer.

## Left to the builder

These are design work, but not Indi's calls. Settle them in the plan:

- **The wire protocol.** The connection is no longer a bare byte pipe.
  It needs, at least: a resume token, sequence numbers both ways with
  acknowledgements (bytes the browser "sent" can die with a WebSocket, in
  either direction), arrival times for buffered lines, the "N lines
  dropped" notice, an explicit close, and the TLS-expectation and push
  subscription from the page. My guess: binary frames for data, small
  JSON text frames for control.
- **Where the seam sits.** My guess: a resumable `net.Conn` in `wsdial`
  that redials and resumes underneath, so `conn` and `session` see one
  unbroken connection while the tab survives. For a fresh tab, `session`
  needs to know a dial resumed a held connection, so it skips log-in and
  `Connected`-side effects meant for a new connection.
- **Telnet state.** Telnet negotiation (NAWS and the rest) runs in the
  tab's `telnet.Parser`. A fresh tab doesn't know what was agreed, so the
  agreed options need saving with the token, or the relay needs to track
  them. While held, nobody answers the MUCK's telnet requests; check what
  Fuzzball sends after connect, and have the relay answer or refuse if it
  must.
- **Dropping at line boundaries** in a raw telnet stream without cutting
  an IAC sequence.
- **The token.** 256 random bits, issued by the relay when a connection
  opens, kept in the browser's storage (IndexedDB, through `persist`)
  next to the character, and good for the life of the hold. A newer
  attachment with the same token wins and the older WebSocket is closed.
  Not tied to an IP (phones change networks).
- **What a fresh tab resumes.** On boot, every character with a saved
  token resumes, whether or not it's `autoconnect`. A fresh tab's saved
  scrollback can lag what it had received (`persist` writes after a
  delay), so the acknowledged position must be saved with the log, or
  resume will duplicate or skip lines.
- **Noticing a dead WebSocket.** Close and error events can't be trusted
  after a long iOS suspension. On `visibilitychange` (visible) and
  `pageshow`, the page checks the socket with a ping of its own, and
  resumes if there's no answer within a few seconds. The relay's
  existing server pings switch from closing the MUCK connection to
  detaching it.
- **The handoff.** "Use Kiln here" no longer drops characters: the old tab
  leaves without an explicit close, and the new tab resumes. #184's
  warning about dropped connections isn't needed. Check `tablock`/`boot.ts`
  ordering so the new tab doesn't resume before the old tab's tokens and
  logs are saved.
- **Relay logs.** Held, resumed, hold expired, buffer dropping, push sent,
  with no traffic content. `RelayClosed`'s byte counts cover the whole
  connection, not one WebSocket.
- **Tests.** Native tests over `wsdial_other.go` and the relay (drop
  mid-stream in either direction, resume, a fresh-client resume, expiry,
  the buffer cap, both caps, pin mismatch, CA trust, plaintext mismatch);
  smoke tests for a handoff and a reload resuming in headless Chrome.
  The fake TLS MUCK in `web/smoke` now gets TLS from the relay.
- **Docs.** `docs/web-relay.md`: the caveat, the new settings, per-world
  trust replacing "TLS trust in the browser", the systemd `LimitCORE=0`
  line, and VAPID setup. The preset section loses `known_hosts`.
