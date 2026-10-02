# Logs

> **AI-generated.** Claude wrote this page from Kiln's code and earlier README. If something here is wrong, please [open an issue](https://github.com/latrani/Kiln/issues).

Kiln starts a new log file each time a character connects, named after that moment: `~/.local/share/kiln/logs/<world>/<character>/2026-09-24 211403 Kit.log`. To keep them with your other logs, or name and sort them your own way, set `log_dir` and `log_name` in `config.toml`:

```toml
log_dir  = "~/Documents/Logs/Mucks/{world}/{name}/%Y/%m"   # a folder per month
log_name = "%Y-%m-%d.%H.%M.%S"                             # 2026-09-16.20.28.58.log
```

Both are [name templates](#name-templates), filled in with the time the session started, so a session that runs past midnight at the end of a month stays in the month it began. Kiln adds `.log` to the name. If a name leaves out the time (`log_name = "%Y-%m-%d"`), sessions that start the same day share one file.

Kiln finds its logs by what's in them, not their names: it searches `log_dir` and every folder under it for files that start with Kiln's header and name the character. So characters can share folders, other programs' files are left alone even if their names look like Kiln's, and a name Kiln would use that's taken by someone else's file gets ` (2)` added. Changing `log_dir` doesn't move old logs; move them yourself if you want Kiln to show them. Logs from older versions of Kiln (one file per day, `YYYY-MM-DD.log`, then one per session) are still read.

Every file looks like this:

```
#kiln-log v1
#kiln-char tapestries/Ash
2026-09-24T21:14:03.120-07:00 <	Rook says, "Evening!"
2026-09-24T21:14:15.002-07:00 >	:grins.
2026-09-24T21:20:00.000-07:00 *	disconnected (reset); retrying in 5s
```

`<` marks received lines, `>` sent lines, and `*` Kiln's own notes. Colors are kept, so `less -R` shows them. `grep` works directly, and `cut -f2-` gives a bare transcript. Passwords are never logged.

## Name templates

`log_dir`, `log_name` and `export_name` are filled in from time codes (as in `strftime`) and names in braces:

| Code | Gives | | Code | Gives |
|---|---|---|---|---|
| `%Y` | `2026` | | `%H` | `20` (hour, 00–23) |
| `%y` | `26` | | `%I` `%p` | `08` `PM` (12-hour) |
| `%m` | `09` (month) | | `%M` | `28` (minute) |
| `%b` `%B` | `Sep` `September` | | `%S` | `58` (second) |
| `%d` | `16` (day) | | `%a` `%A` | `Wed` `Wednesday` |
| `%%` | `%` | | | |

| Name | Gives |
|---|---|
| `{world}` | the world's id |
| `{char}` | the character's id (logs) |
| `{name}` | the character's name |
| `{date}`, `{time}` | `2026-09-16`, `2028` (exports) |
