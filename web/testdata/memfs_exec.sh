#!/bin/sh
# go test -exec wrapper: runs a js/wasm test binary on memfs under node.
GOROOT="$(go env GOROOT)" exec node --stack-size=8192 "$(dirname "$0")/memfs_exec.mjs" "$@"
