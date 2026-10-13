# Trying Kiln's web build locally

Needs Go and node. Everything runs on your machine; the relay connects
to real MUCKs over the internet.

The page is a development page for Kiln's web API (`internal/webapi`):
a list of open characters, a line to type in, and a log of every event
the core sends. The real web UI comes next (part 3 of #125).

```sh
web/build.sh
cp -R web/preset.example web/preset.local      # edit worlds/*.toml
cp web/dev-allow.example.toml web/dev-allow.toml  # list the same worlds
go run ./cmd/kiln-relay -dev -allow web/dev-allow.toml \
  -web web/static -preset web/preset.local
```

Open http://localhost:8080/kiln/. Rebuild with `web/build.sh` and reload
after changing Go code; the relay reloads `dev-allow.toml` by itself.

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
`npm run check` type-checks. `web/ui/src/str.gen.ts` (the catalog and
glyphs for the page) is made by `go generate ./internal/str`; don't edit
it by hand.

The smoke test builds the page, starts a fake MUCK and the relay, and
drives the page in headless Chrome: it opens a character, connects,
sends a line and waits for the echo. It needs Google Chrome
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
