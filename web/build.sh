#!/bin/sh
# Builds the page into web/static: kiln.wasm and Go's matching
# wasm_exec.js, then the page itself from web/ui (app.js, app.css, the
# fonts and index.html).
set -eu
cd "$(dirname "$0")"
GOOS=js GOARCH=wasm go build -trimpath -o static/kiln.wasm ./cmd/kiln-web
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" static/wasm_exec.js
cd ui
[ -d node_modules ] || npm ci
npm run check
node build.mjs ../static
