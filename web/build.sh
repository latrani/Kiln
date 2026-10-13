#!/bin/sh
# Builds the page into web/static: kiln.wasm and Go's matching
# wasm_exec.js, then the page itself from web/ui (app.js, app.css, the
# fonts and index.html).
set -eu
cd "$(dirname "$0")"
GOOS=js GOARCH=wasm go build -trimpath -o static/kiln.wasm ./cmd/kiln-web
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" static/wasm_exec.js
cd ui
# Install the page's packages, again whenever the lockfile is newer than
# what's installed (npm writes node_modules/.package-lock.json).
if [ ! -f node_modules/.package-lock.json ] || [ package-lock.json -nt node_modules/.package-lock.json ]; then
	npm ci
fi
npm run check
node build.mjs ../static
