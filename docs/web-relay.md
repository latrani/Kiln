# Running kiln-relay

Kiln's web build can't open TCP connections, so the page opens a
WebSocket to `kiln-relay`, which connects to the MUCK. It only connects to
worlds in its allowlist; that list is the real boundary. For a world
without `plaintext = true`, the relay also checks that the client's first
frame starts a TLS handshake and refuses it otherwise, so an honest
client can't send a password in the clear by accident. It doesn't inspect
anything after that first frame.

The relay also serves the page itself, and a preset: the worlds a
player's browser starts with. So one process and one nginx block run
the whole web build.

## Installing

```sh
brew install latrani/tap/kiln
```

installs `kiln`, `kiln-relay` and the page, at
`$(brew --prefix)/share/kiln/web`, from a release whose formula builds
the relay and the page. To deploy a new release:

```sh
brew upgrade kiln
sudo systemctl restart kiln-relay
```

The relay's flags:

- `-allow PATH`: the allowlist (below). Required.
- `-web DIR`: serve the page at `/kiln/`.
- `-preset DIR`: serve a preset at `/kiln/preset/` (below). Without it
  the page starts with nothing preset.
- `-listen ADDR`: where to listen; `127.0.0.1:7801` by default.

## Allowlist

```toml
# Hosts (beyond the relay's own) whose pages may use this relay.
origins = ["muck.example.org"]
# Who may say who the client is (X-Forwarded-For): your nginx.
trusted_proxies = ["127.0.0.1"]
# Connections from one client: an IPv4 address or an IPv6 /64. Raise it
# if players share an address (campus or carrier NAT).
max_per_ip = 8
# Connections in all.
max_conns = 256

[[world]]
host = "muck.example.org"
ports = [8899]
plaintext = true   # only for your own MUCK

[[world]]
host = "friends.example.net"
ports = [8888]     # TLS only
```

`origins` are host patterns (`*.example.org`, `localhost:8080`), not
URLs. A host and port may appear in only one `[[world]]`.

The relay reloads the file when it changes, following a symlink to its
target. If the new file has an error, it logs it and keeps the old list.

The relay pings each open connection every 30 seconds and closes one
that doesn't answer, so a browser that vanished (a laptop gone to sleep)
doesn't hold a slot until nginx's timeout.

## TLS trust in the browser

The relay never sees a TLS world's plaintext, but it, or anyone between
it and the MUCK, could show the browser its own certificate. A `pin`
world trusts the first certificate it sees and remembers it, and the
browser keeps that pin only as long as it keeps the site's data. So
a player's first visit, a private window, or a visit after clearing site
data trusts whatever it's shown. Close that gap for the worlds you serve:

- A world with a certificate from a real CA: `tls_trust = "ca"` in its
  world file. The web build carries its own root certificates.
- A self-signed world: put its pin in the preset's `known_hosts`, one
  `host:port sha256:…` line each, as in a desktop Kiln's `known_hosts`
  once it has connected. The page adds each to the browser's pins unless that host:port already
  has one.

## Preset

A preset is laid out like a `/backup` zip, in a folder of its own, say
`/etc/kiln-relay/preset/`:

```
/etc/kiln-relay/preset/
  config/worlds/PFMuck.toml
  known_hosts            # pins for self-signed worlds (above)
```

`config/` is Kiln's config folder: worlds, and any other config file you
want players to start with. The relay lists the folder for the page on
each request, so adding or editing a file needs no restart. A preset
file is written into a player's browser only where nothing is saved
yet, so a player's own edits win (see `docs/web-dev.md`).

## nginx

Over https: the page's tab lock and copying to the clipboard need a
secure context. Everything under `/kiln/` goes to the relay, the page,
the preset and the WebSocket alike:

```nginx
# In http {}: a WebSocket asks to upgrade; the page's files don't.
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

# In the https server {}:
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

`$http_host` keeps the port the page was opened on. `$host` drops it,
and on a port other than 80 or 443 the relay would then refuse the page
as a foreign origin.

## systemd

```ini
[Unit]
Description=Kiln web relay
After=network-online.target

[Service]
ExecStart=/home/linuxbrew/.linuxbrew/bin/kiln-relay -allow /etc/kiln-relay/allow.toml -web /home/linuxbrew/.linuxbrew/share/kiln/web -preset /etc/kiln-relay/preset
DynamicUser=yes
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Write your own `brew --prefix` in place of `/home/linuxbrew/.linuxbrew`:
systemd doesn't expand it. With `DynamicUser`, the relay runs as a user
made up for it, so `/etc/kiln-relay` and what's in it must be readable
by anyone (the usual 755 and 644).

## Local testing

See `docs/web-dev.md`.
