# Configuration

> **AI-generated.** Claude wrote this page from Kiln's code and earlier README. If something here is wrong, please [open an issue](https://github.com/latrani/Kiln/issues).

```
~/.config/kiln/
  config.toml          # global settings and defaults
  worlds/<id>.toml     # one world and its characters
  packs/<id>.toml      # shareable rules and tag lists, e.g. the starter fuzzball.toml
```

Settings are inherited in this order: **defaults → packs (in `use` order) → world → character**. Rule lists add up along the way, and single settings are replaced by the most specific level.

| Setting | Where | Meaning |
|---|---|---|
| `export_dir` | config.toml | Where log mode's exports are saved |
| `export_name` | config.toml | Export file name, from `{date}`, `{time}`, `{world}`, `{name}` and [time codes](logs.md#name-templates) (default `"{date} {time} {world} {name}"`; may include `/` for subfolders) |
| `export_format` | config.toml | `"plain"`, `"ansi"` or `"html"`: what `Enter` picks when exporting (default: always ask) |
| `log_dir` | config.toml | Where logs go (default `~/.local/share/kiln/logs/{world}/{char}`), see [Logs](logs.md) |
| `log_name` | config.toml | Log file names, without `.log` (default `"%Y-%m-%d %H%M%S {char}"`), see [Logs](logs.md) |
| `password_store` | config.toml | `"keychain"` (default), `"file"` (`~/.local/share/kiln/passwords.json`, readable only by you) or `"none"` (never save) |
| `notify_idle` | config.toml | No input for this long counts as away (default `"5m"`; `"0"`: only switching away counts), see [Notifications](notifications.md) |
| `notify_method` | config.toml | `"osc"` (default): a notification with the line; `"bell"`: a bell (works over mosh); `"both"` |
| `theme` | config.toml | Which `themes/<name>.toml` to draw with (default `"default"`, which Kiln makes on first run; `"kiln"` is the built-in), see [Themes](themes.md) |
| `scroll_lines` | config.toml | How far one notch of the mouse wheel scrolls: rows in the scrollback, lines in log mode (default `1`; anything but a whole number of at least 1 means `1`) |
| `appearance` | config.toml | `"auto"` (default: ask the terminal), `"dark"` or `"light"`: which palette themes use, see [Themes](themes.md) |
| `host`, `port`, `tls` | world | Where to connect |
| `tls_trust` | world | `"pin"` (default) or `"ca"`, see below |
| `login` | world, character | Login template; `{name}` and `{password}` are filled in |
| `use` | world | Rule packs to apply |
| `max_line_bytes` | any level | Longest line the server accepts (Fuzzball: 2047) |
| `newline_mode` | any level | `"batch"`: each line is its own command; `"flatten"`: lines are joined with spaces |
| `autoconnect` | any level | Connect when Kiln starts |
| `reconnect` | any level | Retry with backoff after a drop or failed connect (default `true`); `false` stays disconnected until `/connect` |
| `notify` | any level | What notifies while you're away: `"all"`, `"first"` (default: the first line, then attention lines), `"attention"` or `"none"` |
| `local_echo` | any level | Show what you send in the scrollback and the log view (default `false`; the server usually echoes poses, and sent lines are always logged) |
| `name`, `aliases` | character | Your in-game name and the others that count as you |

## Certificates

With `tls_trust = "pin"` (the default), Kiln remembers a server's certificate the first time it connects, which suits the self-signed certificates most MUCKs use. If the certificate later changes, Kiln refuses to connect and says so. If you expected the change (the server renewed its certificate), accept it with `/trust`, or from a shell:

```sh
kiln trust <world> <fingerprint>     # the fingerprint is shown in the error
```

Use `tls_trust = "ca"` for servers with certificates from a real certificate authority.
