# Running kiln-relay

Kiln's web build can't open TCP connections, so the page opens a
WebSocket to `kiln-relay`, which connects to the MUCK. It only connects to
worlds in its allowlist (the `[[world]]` list in `relay.toml`); that list
is the real boundary. For a TLS world, the relay opens the TLS
connection itself, trusting the world's certificate as `relay.toml` says.

The relay also serves the page itself, and a preset: the worlds a
player's browser starts with. So one process and one nginx block run
the whole web build.

## What the relay can see

The relay, not the player's browser, holds the TLS connection to each
world. Traffic is encrypted between the browser and nginx, and between
the relay and the world, but the relay itself handles it in the clear:
passwords, pages, whispers, everything a player sends or sees. While a
player is away, it keeps what arrives in memory until they come back or
the hold runs out.

So whoever runs the relay, and anyone who gets onto its host, can read
its players' traffic. That's no new exposure when the relay is run by
the world's own staff. Don't serve worlds you don't operate unless their
players know you can read what they send.

The relay never writes traffic to disk. Turn off core dumps for it
(`LimitCORE=0` in the systemd unit).

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

- `-config PATH`: the relay's config file, `relay.toml` (below). Required.
- `-web DIR`: serve the page at `/kiln/`.
- `-preset DIR`: serve a preset at `/kiln/preset/` (below). Without it
  the page starts with nothing preset.
- `-listen ADDR`: where to listen; `127.0.0.1:7801` by default.

## relay.toml

Say `/etc/kiln-relay/relay.toml`. Its `[[world]]` entries are the
allowlist.

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
# How long a connection is held after its page goes away (below), and
# after its tab closes; how much is kept for each one meanwhile, and for
# all of them, in MB.
hold = "24h"
short_hold = "10m"
max_buffer_mb = 4
max_total_buffer_mb = 256
# Push notifications while a player is away (below). Leave both out for
# none.
vapid_key = "/run/credentials/kiln-relay.service/vapid.pem"
push_contact = "https://muck.example.org"

[[world]]
host = "muck.example.org"
ports = [8899]
plaintext = true

[[world]]
host = "muck.example.org"
ports = [8888]
pin = "sha256:…"    # a self-signed certificate (below)

[[world]]
host = "friends.example.net"
ports = [7777]
tls_trust = "ca"    # a certificate from a real CA
```

`origins` are host patterns (`*.example.org`, `localhost:8080`), not
URLs. A host and port may appear in only one `[[world]]`.

The relay reloads the file when it changes, following a symlink to its
target. If the new file has an error, it logs it and keeps the old one.

The relay pings each open connection every 30 seconds and holds one
that doesn't answer (below), so a browser that vanished (a laptop gone
to sleep) is noticed before nginx's timeout.

## Held connections

When a page's WebSocket dies without Kiln saying it's done (a phone
switching apps or losing its network, a tab suspended in the
background), the relay keeps the world connection open and holds it
for `hold` (24 hours unless you set it). Kiln picks it back up when it
comes back: no reconnect, no log-in, and the lines that arrived
meanwhile come in with the times they reached the relay. That's so in
the same tab, and in a fresh one in the same browser: a reload, **Use
here** in another tab, or a tab the phone discarded. The browser saves
each character's place beside its logs, and the relay keeps everything
past the last place the browser has saved.

A player quitting or closing the character, or `/reset`, lets the world
go at once. A tab that really closes tells the relay as it goes, and the
relay lets its characters go after `short_hold` (10 minutes) unless the
page comes back first, as a reload does. A phone switching apps or
locking doesn't close the tab, so those keep the full hold.

- A held connection still counts against `max_per_ip` and `max_conns`
  until it ends. Picking it up from a new address (wifi to cellular)
  moves its count there.
- A page picking a connection up has 10 seconds to say which one. Up to
  `max_per_ip` such pages from one client, and `max_conns` in all, may
  be waiting at once; they're counted apart from connections, so a
  player at the cap can still pick up every held one.
- While held, what the world sends is kept in memory, up to
  `max_buffer_mb` per connection and `max_total_buffer_mb` in all. Past
  that the oldest lines go (from the fullest connection, for the total),
  and the player is told how many they missed.
- If the hold runs out, the relay closes the world connection; Kiln
  says the relay let go, and does what the character's `reconnect`
  setting says.
- Restarting the relay lets every held connection go.

## Push notifications

While a connection is held, the relay can tell the player's browser
that something happened: one push the first time the world sends
anything after the page went away, then no more until the player has
come back and gone away again. The push carries nothing from the world,
not even which character; the browser shows "activity while you were
away", and tapping it opens Kiln. Kiln only asks for it for characters
whose `notify` isn't `"none"`, once the player has allowed
notifications.

Desktop browsers and Android take it from a normal tab. On an iPhone or
iPad, Kiln must be added to the Home Screen first (below), and
notifications allowed from inside it.

To turn it on, make the relay a key (a VAPID key, which push services
use to check pushes come from the same sender each time), readable only
by root:

```sh
sudo sh -c 'umask 077 && openssl ecparam -name prime256v1 -genkey -noout \
  | openssl pkcs8 -topk8 -nocrypt -out /etc/kiln-relay/vapid.pem'
```

systemd hands it to the relay (`LoadCredential` in the unit below), at
`/run/credentials/kiln-relay.service/vapid.pem`. Name that in
`relay.toml` with `vapid_key` (a relative path is taken from the file's
directory), along with `push_contact`: a web page or `mailto:` address where a push
service can reach you about your relay's pushes. Apple refuses pushes
whose contact isn't a real `https:` or `mailto:` address.

- The relay posts to the browsers' push services (Google's, Mozilla's,
  Apple's and Microsoft's), so its host needs outbound HTTPS to them.
  It posts nowhere else, whatever a page asks.
- Changing the key ends every player's subscription; each browser
  subscribes again the next time Kiln loads in it.

## On a phone's Home Screen

Players can add Kiln to a Home Screen (Share, then Add to Home Screen),
and it opens as its own app, named Kiln, with its icon. Newer iPhones
open every site added that way as an app.

The page keeps its files (and the preset) in the browser, so Kiln
launches with no network too, though it can't reach a world then. With
a network it always asks the relay for the page first, so a new release
reaches players on their next launch, as it does in a browser tab.

## TLS trust

Every world without `plaintext = true` is TLS, and says how the relay
trusts it. Nothing is trusted on first use.

- A world with a certificate from a real CA: `tls_trust = "ca"`. The
  relay checks it against the system's root certificates.
- A self-signed world: `pin = "sha256:…"`. Copy the pin from a desktop
  Kiln's `known_hosts` once it has connected to the world (the part
  after `host:port`).

If the certificate doesn't match, the relay refuses the connection and
the player sees that it refused the world's certificate. A player's
world file can't turn TLS off for a TLS world here; one that asks for TLS
on a `plaintext` world is refused rather than sent in the clear.

## Preset

A preset is laid out like a `/backup` zip, in a folder of its own, say
`/etc/kiln-relay/preset/`:

```
/etc/kiln-relay/preset/
  config/worlds/fm.toml
```

`config/` is Kiln's config folder: worlds, and any other config file you
want players to start with. The relay lists the folder for the page on
each request, so adding or editing a file needs no restart. It leaves
out dotfiles (`.DS_Store`, `.git`) and won't serve them. A preset
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
location = /kiln {
    return 301 /kiln/;
}
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
ExecStart=/home/linuxbrew/.linuxbrew/bin/kiln-relay -config /etc/kiln-relay/relay.toml -web /home/linuxbrew/.linuxbrew/share/kiln/web -preset /etc/kiln-relay/preset
DynamicUser=yes
LoadCredential=vapid.pem:/etc/kiln-relay/vapid.pem
LimitCORE=0
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Write your own `brew --prefix` in place of `/home/linuxbrew/.linuxbrew`:
systemd doesn't expand it. With `DynamicUser`, the relay runs as a user
made up for it, so `/etc/kiln-relay` and what's in it must be readable
by anyone (the usual 755 and 644), apart from the push key, which
`LoadCredential` passes along (leave that line out without one), and so
must the page under brew's
prefix. If the prefix is in a home folder that others can't enter (newer
Ubuntu makes them 750), let them through it: `sudo chmod o+x
/home/linuxbrew`.

## Local testing

See `docs/web-dev.md`.
