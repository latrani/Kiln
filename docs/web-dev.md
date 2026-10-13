# Trying Kiln's web build locally

Needs Go and node. Everything runs on your machine; the relay connects
to real MUCKs over the internet.

The page is being built (part 3 of #125): for now it draws the TUI's
five areas, empty, with the theme, the font and the status line.

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

The page lives in `web/ui` (Preact, TypeScript, esbuild): `npm ci` there
once; `npm test` runs its tests (esbuild bundles each, Node runs them),
`npm run check` type-checks.

- `src/main.tsx`: the entry; `src/boot.ts` starts Kiln (tab lock, memfs,
  IndexedDB, preset, the wasm) and `src/store.ts` holds what Go sends.
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

Files are saved in the browser (IndexedDB) and come back on the next
visit: config edits, worlds, TLS pins and logs. That's best effort:
clearing site data, a private window, or a browser that evicts storage
loses them. Safari clears a site's storage after 7 days of Safari use
without visiting it. Back up and restore aren't on the development page
yet (`/backup` and `/restore` only show up in its event log); the real
page brings them back.

What you've typed but not sent is saved when you leave the tab (another
tab or app, or the phone locking), so it's still there if the browser
closes the tab in the background.

One tab runs Kiln at a time. A second tab offers **Use here**, which
moves Kiln over; the first tab saves, disconnects, and shows the same
offer.

Preset files are written only where nothing is saved, so your edits to a
preset world win over the server's copy. A preset world you delete comes
back on the next visit.
