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
node --test web/static/memfs.test.mjs
```

Files live in memory: config edits, pins and logs last until the tab
closes.
