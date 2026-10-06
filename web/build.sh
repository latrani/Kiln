#!/bin/sh
# Builds the page into web/static: kiln.wasm and Go's matching wasm_exec.js.
set -eu
cd "$(dirname "$0")"
GOOS=js GOARCH=wasm go build -trimpath -o static/kiln.wasm ./cmd/kiln-web
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" static/wasm_exec.js
