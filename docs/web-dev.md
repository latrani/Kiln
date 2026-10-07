# Trying Kiln's web build locally

Needs Go and node. Everything runs on your machine; the relay connects
to real MUCKs over the internet.

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
(cd web && go test ./...)                       # bridge, wsdial (native)
(cd web && GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...)
node --test web/static/*.test.mjs
```

Files are saved in the browser (IndexedDB) and come back on the next
visit: config edits, worlds, TLS pins and logs. That's best effort:
clearing site data, a private window, or a browser that evicts storage
loses them. Safari clears a site's storage after 7 days of Safari use
without visiting it. **↓ Back up** at the bottom of the sidebar downloads
a zip of your config and pins; **↑ Restore** reads one back (it adds and
replaces files, never deletes). `/backup` and `/restore` do the same.
Saving a log in log mode downloads the file.

One tab runs Kiln at a time. A second tab offers **Use here**, which
moves Kiln over; the first tab saves, disconnects, and shows the same
offer.

Preset files are written only where nothing is saved, so your edits to a
preset world win over the server's copy.
