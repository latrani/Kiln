# Running kiln-relay

Kiln's web build can't open TCP connections, so the page opens a
WebSocket to `kiln-relay`, which connects to the MUCK. It only connects to
worlds in its allowlist, and only passes TLS traffic unless a world says
`plaintext = true`. So whoever runs the relay never sees other worlds'
passwords.

## Allowlist

```toml
# Hosts (beyond the relay's own) whose pages may use this relay.
origins = ["muck.example.org"]
# Who may say who the client is (X-Forwarded-For): your nginx.
trusted_proxies = ["127.0.0.1"]

[[world]]
host = "muck.example.org"
ports = [8899]
plaintext = true   # only for your own MUCK

[[world]]
host = "friends.example.net"
ports = [8888]     # TLS only
```

The relay reloads the file when it changes. If the new file has an
error, it logs it and keeps the old list.

## nginx

```nginx
location /kiln/ {
    alias /srv/kiln/static/;
}
location = /kiln/relay {
    proxy_pass http://127.0.0.1:7801;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_read_timeout 1d;
    proxy_send_timeout 1d;
}
```

## systemd

```ini
[Unit]
Description=Kiln web relay
After=network-online.target

[Service]
ExecStart=/usr/local/bin/kiln-relay -allow /etc/kiln-relay/allow.toml
DynamicUser=yes
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

## Local testing

See `docs/web-dev.md`.
