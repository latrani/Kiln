# Trying Kiln's web build locally

Needs Go and node. Everything runs on your machine; the relay connects
to real MUCKs over the internet.

The page is being built (part 3 of #125): it's Kiln's main screen, the
picker, the password form, the editor and log mode so far. Open a connection with `+ Connection` in the sidebar, Ctrl+O, or
Enter with nothing open. Edit a world or character from the picker
(Enter on a world, `+ World`), with Ctrl+T (Mac) or Alt+T, or with
`/edit` and `/edit world`. In the editor Enter saves, Esc cancels, and
Ctrl+T or Alt+T hides it, keeping the edits.
Ctrl+L or the `Log` chip opens log mode for the character showing (and
hides it again, as it was). Select lines with the mouse: a click takes
a line, Shift+click extends the range, dragging selects a run. While
the log has focus, `/` finds, `n`/`N` step through matches, `g` goes
to a date, `t` shows a line's tags, `c` copies and `e` exports (a
download). `f` (or the `Filter` chip) opens the filter panel in the
sidebar: ↑/↓ move, `h` hides and `o` shows only an entry, ←/→ fold a
group, Enter on `+ Text` filters by a text, `x` removes one, Esc closes
it. Esc closes log mode.

```sh
web/build.sh
cp -R web/preset.example web/preset.local      # edit worlds/*.toml
cp web/dev-allow.example.toml web/dev-allow.toml  # list the same worlds
go run ./cmd/kiln-relay -dev -allow web/dev-allow.toml \
  -web web/static -preset web/preset.local
```

Open http://localhost:8080/kiln/. Rebuild with `web/build.sh` and reload
after changing Go or page code; the relay reloads `dev-allow.toml` by
itself. Add `?record` to the address to keep every event Go sends in
`kilnPage.events` (the smoke test does), and drive Kiln from the console
with `kilnPage.act({Do: "open", Key: "world/char"})`.

Tests:

```sh
go test ./...                                   # root: relay, conn, …
(cd web && go test ./...)                       # wsdial, backup (native)
(cd web && GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...)
node --test web/static/*.test.mjs
(cd web/ui && npm ci && npm run check && npm test)   # the page
(cd web && KILN_SMOKE=1 go test ./smoke -v)    # headless Chrome, end to end
```

The page lives in `web/ui` (Preact, TypeScript, esbuild; Node 22.18 or
later, which runs TypeScript directly): `npm ci` there once; `npm test` runs its tests (esbuild bundles each, Node runs them),
`npm run check` type-checks.

- `src/main.tsx`: the entry; `src/page.tsx` is the page around Kiln
  (the app, its keys, what Go asks of it), `src/boot.ts` starts Kiln
  (tab lock, memfs, IndexedDB, preset, the wasm) and `src/store.ts`
  holds what Go sends.
- `src/theme.ts`: the `Theme` event as a stylesheet, the terminal's
  colors (Catppuccin), and the fold that draws a span.
- `src/ui/`: one component per TUI part.
- `src/str.gen.ts`: the catalog and glyphs, made by
  `go generate ./internal/str`; don't edit it by hand.
- `fonts/`: JetBrains Mono 2.304 and its license (OFL).

`web/static` holds the boot modules (`memfs.mjs`, `persist.mjs`,
`tablock.mjs`, bundled into the page) and, once built, the page.

The smoke test builds the page, starts a fake MUCK and the relay, and
drives the page in headless Chrome: it opens a character, connects,
sends a line, waits for the echo, and checks every glyph Kiln draws is
one cell wide in the bundled font. It needs Google Chrome
(`$KILN_CHROME` names another binary) and is skipped without
`KILN_SMOKE=1`.

### Screenshot goldens

`web/smoke`'s `TestGolden…` tests draw the TUI's golden scenarios (main,
picker, editor, editor error, log, input, scrolled, password, log
filter) on the page, light and dark, and compare each with
`web/smoke/testdata/golden/<scenario>-<light|dark>.png`. They run with
the smoke tests:

```sh
(cd web && KILN_SMOKE=1 go test ./smoke -run TestGolden -v)
```

A shot that differs leaves what the page drew and a diff (the golden,
dimmed, with each differing pixel in magenta) in
`web/smoke/testdata/golden/failed/`. After a change to the look you
meant, rewrite the goldens, and look through them before committing:

```sh
(cd web && KILN_SMOKE=1 go test ./smoke -run TestGolden -args -update)
```

A scenario with no golden yet takes its shots, says so, and is skipped.
`TestGoldenStable` checks that the same screen shot twice is the same.
Go runs natively in these tests, over the same data as the TUI's
goldens (`internal/kilntest/golden.go`). The page is a test-only build
(`node build.mjs <dir> golden`, from `src/golden.tsx`) that talks to it
over a WebSocket instead of running the wasm. A Chrome update can change
how text is drawn and fail every golden: look at the diffs, then
`-update`.

Files are saved in the browser (IndexedDB) and come back on the next
visit: config edits, worlds, TLS pins and logs. That's best effort:
clearing site data, a private window, or a browser that evicts storage
loses them. Safari clears a site's storage after 7 days of Safari use
without visiting it. `/backup` downloads everything as a zip, and
`/restore` puts one back.

What you've typed but not sent is saved when you leave the tab (another
tab or app, or the phone locking), so it's still there if the browser
closes the tab in the background.

One tab runs Kiln at a time. A second tab offers **Use here**, which
moves Kiln over; the first tab saves, disconnects, and shows the same
offer.

Preset files are written only where nothing is saved, so your edits to a
preset world win over the server's copy. A preset world you delete comes
back on the next visit.
