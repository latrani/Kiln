#!/bin/sh
# Copies pinned xterm.js builds into web/static/vendor. Rerun after
# changing a version; commit the result.
set -eu
cd "$(dirname "$0")"
out=static/vendor
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$out"
for pkg in @xterm/xterm@6.0.0 @xterm/addon-fit@0.11.0 @xterm/addon-webgl@0.19.0 @xterm/addon-clipboard@0.2.0; do
  (cd "$tmp" && npm pack -q "$pkg" >/dev/null)
done
for f in "$tmp"/*.tgz; do
  name=$(basename "$f" .tgz)
  mkdir -p "$tmp/$name"
  tar xzf "$f" -C "$tmp/$name"
  cp "$tmp/$name"/package/lib/*.mjs "$out/"
  cp "$tmp/$name"/package/LICENSE "$out/LICENSE-$name"
done
cp "$tmp"/xterm-xterm-*/package/css/xterm.css "$out/"
