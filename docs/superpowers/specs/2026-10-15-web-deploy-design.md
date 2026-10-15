# Deploying the web build with Homebrew

## Why

Putting Kiln's web build on a server takes a cross-built relay, a page
built with node on another machine, nginx serving files, and a
`preset/manifest.json` written by hand, which nothing documents; without
it the page doesn't start. The server Indi deploys to already has Kiln
from Homebrew. A release should carry everything, so a deploy is
`brew upgrade` and a restart.

## The relay serves the page and the preset

- `-web DIR` and `-preset DIR` work without `-dev`: the relay serves
  the page at `/kiln/` and the preset at `/kiln/preset/`, with
  `/kiln/preset/manifest.json` made from the preset folder on each
  request (as `-dev` does now), so it never drifts.
- Without `-preset`, `manifest.json` is `[]`: the page starts with
  nothing preset rather than failing.
- `-dev` goes: all it would still do is listen on `localhost:8080` by
  default. `-listen localhost:8080` says that. Pre-1.0, a clean break:
  `-dev` is an unknown flag.
- The flags' help and the relay's "serving" log line say what they do
  without "dev".

## nginx

One location, everything to the relay:

```nginx
location /kiln/ {
    proxy_pass http://127.0.0.1:7801;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection $connection_upgrade;
    proxy_set_header Host $http_host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_read_timeout 1d;
    proxy_send_timeout 1d;
}
```

(`$connection_upgrade` from the usual `map $http_upgrade` block, so
plain page requests aren't sent `Connection: upgrade`; the doc gives
it.) Over https: the tab lock and the clipboard need a secure context.

## Homebrew

`latrani/homebrew-tap`'s `kiln.rb` (a one-time change there, landed
with `brew pr-pull`):

- `depends_on "node" => :build` beside `go`.
- Builds `kiln` and `kiln-relay` into `bin`.
- Runs `web/build.sh`, which builds the wasm and the page into
  `web/static`, and installs that to `share/kiln/web`.
- Its test runs `kiln version`, checks `kiln-relay` starts far enough
  to complain it needs `-allow`, and checks `share/kiln/web` has
  `index.html` and `kiln.wasm`.

Releases after that carry the relay and the page with no more tap
changes: the bump workflow only updates the tag.

## On the server

- `/etc/kiln-relay/allow.toml`: the allowlist (unchanged).
- `/etc/kiln-relay/preset/`: `config/worlds/*.toml` and, for a
  self-signed world, `known_hosts`.
- systemd runs `kiln-relay -allow /etc/kiln-relay/allow.toml -web
  <prefix>/share/kiln/web -preset /etc/kiln-relay/preset`, with
  Homebrew's prefix written out (`brew --prefix`; systemd doesn't
  expand it).
- Deploying a release: `brew upgrade kiln`, `systemctl restart
  kiln-relay`.

## Docs

- `docs/web-relay.md` is rewritten around this: install, allowlist,
  preset layout, nginx, systemd, upgrading. It says the install needs a
  release whose formula builds `kiln-relay` and the page.
- `docs/web-dev.md`: the run command drops `-dev` for `-listen
  localhost:8080`.
- The README is the maintainer's: unchanged.

## Testing

- The relay with `-web` and `-preset` and no `-dev` serves the page,
  the preset's files and its manifest; without `-preset` the manifest
  is `[]`; `-dev` is an unknown flag.
- `web/smoke` starts the relay the new way and passes.
- The formula's test, run by the tap's test-bot on the bump PR.

## Not in this

- Theme discovery, the phone layout, password managers: separate.
- Cutting the release itself: Indi's call, after this merges and the
  tap change lands.
