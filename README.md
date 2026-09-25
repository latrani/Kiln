# Kiln

A modern terminal MUCK client in the spirit of TinyFugue, built for social and roleplay play: poses, pages, whispers, and lots of characters across lots of worlds. It has full Unicode support, mouse support, truecolor, and a log browser that can turn any stretch of history into a clean scene export.

```
┌─────────────┬──────────────────────────────────┐
│ FurryMUCK   │ Rook says, "Evening!"            │
│   Kit       │ Sable waves a paw.               │
│   Rook      │ Mira pages: you around?          │
│ Tapestries  │                                  │
│   Ash    ● 2│                        ▼ 12 new  │
│ + Add conne…├──────────────────────────────────┤
│             │ > :grins, then leans on the      │
│             │   counter.                       │
│             ├──────────────────────────────────┤
│             │ FurryMUCK/Kit · connected · 21:14│
└─────────────┴──────────────────────────────────┘
```

- **Many worlds, many characters, all at once.** The sidebar shows the characters you have open, grouped under their worlds; everything else is a Ctrl+O away. Each one has its own scrollback, draft and history.
- **Knows what a page is.** Lines are tagged (page, whisper, say, and `self` when they mention you) by rules you can edit. Tags drive colors and the attention badge.
- **Everything is logged** to plain, greppable text, one file per session, in whatever folder you like.
- **Browse mode** pages back through all of a character's logs. You can filter by tag, search, mark a range, drop stray lines, and export the scene as plain text, ANSI or HTML.
- **Safe input.** The input box shows exactly where the server would cut an over-long line, so you can break it before sending.
- **Secure by default.** Passwords live in your OS keychain, never in config (or, if you opt in, a file only you can read). TLS certificates are pinned on first use, so self-signed MUCK certificates just work.

Kiln deliberately doesn't do combat-MUD features like GMCP or MSDP.

## Install

### Download a release

Grab the archive for your system from the [Releases page](https://github.com/latrani/Kiln/releases), unpack it, and put `kiln` somewhere on your `PATH`.

| System | File |
|---|---|
| macOS, Apple silicon | `kiln_<version>_darwin_arm64.tar.gz` |
| macOS, Intel | `kiln_<version>_darwin_amd64.tar.gz` |
| Linux, x86-64 | `kiln_<version>_linux_amd64.tar.gz` |
| Linux, ARM64 | `kiln_<version>_linux_arm64.tar.gz` |
| Windows | `kiln_<version>_windows_amd64.zip` |

Each is a single self-contained binary, with nothing else to install. `checksums.txt` lists SHA-256 sums if you want to verify your download.

**macOS:** the binaries aren't signed yet, so the first time you run a downloaded `kiln`, macOS may refuse to open it. Clear the quarantine flag once:

```sh
xattr -d com.apple.quarantine /path/to/kiln
```

**Linux:** saving passwords to the keychain needs a Secret Service keyring (GNOME Keyring or KWallet), which most desktop sessions already run. Without one, set `password_store = "file"` in `config.toml` to save them in a file only you can read, or `"none"` to just be asked each time you connect.

### Build from source

With Go 1.27.1 or newer:

```sh
go install github.com/latrani/Kiln/cmd/kiln@latest
```

## Getting started

1. **Run `kiln` once.** It creates the config directory, `~/.config/kiln/`, with a starter `config.toml` and a rule pack for Fuzzball MUCKs. With no worlds yet, it tells you where to add one. Quit with `Ctrl+C` twice.

2. **Add a world.** Either press `Ctrl+O` in Kiln and pick `+ World` then `+ Character` (which writes a file like the one below), or create `~/.config/kiln/worlds/furrymuck.toml` yourself. The file name is the world's id.

   ```toml
   host = "furrymuck.com"
   port = 8899
   tls = true
   login = "connect {name} {password}"
   use = ["fuzzball"]          # starter rules for pages, whispers and says

   [[characters]]              # one of these per character, in sidebar order
   name = "Kit"                # the in-game name
   aliases = ["Kitty"]         # other names that count as you
   autoconnect = true
   ```

   Worlds added from Kiln don't get a `login` line; to log in automatically, set `login` in the world file or in `config.toml`'s `[defaults]`.

   A character's id (used for its log folder, saved password and `kiln passwd`) is its name. If the name has anything besides letters, digits, `_` and `-`, give it one with `id = "…"`.

3. **Save the password** (optional). Run `kiln passwd furrymuck Kit`. If you skip this, Kiln asks for the password when it connects and offers to save it.

4. **Run `kiln`.** Characters with `autoconnect = true` open and log in. Press `Ctrl+O` (or click `+ Add connection`) to open another.

Config changes apply live while Kiln runs. If a file has a mistake, Kiln keeps the last working config and shows the error in the statusline.

## Using Kiln

### Keys

| Key | Does |
|---|---|
| `Enter` | Send (on an empty input, connect a disconnected character) |
| `Shift+Enter` (or `Alt+Enter`) | New line in the input box |
| `↑` / `↓` | Move between rows, or recall input history from the top or bottom row |
| `Ctrl+←` / `Ctrl+→` (or `Alt`/`Option`) | Move by word |
| `Home` / `End` (or `Ctrl+A` / `Ctrl+E`) | Start or end of the line |
| `Ctrl+W` (or `Alt+Backspace`) / `Alt+Delete` | Delete the word before or after the cursor |
| `Ctrl+U` / `Ctrl+K` | Delete to the start or end of the line |
| `Ctrl+↑` / `Ctrl+↓` | Switch between open characters |
| `Tab` / `Shift+Tab` | Jump to the next (or previous) character with unread lines |
| `Ctrl+O` | Add a connection: type to filter, `Enter` to connect, `Esc` to close |
| `PgUp` / `PgDn`, mouse wheel | Scroll back (click the `▼ new` pill to jump to live) |
| `Ctrl+B` | Open browse mode |
| `Esc` | Skip the login prompt |
| `Ctrl+C` | Clear the input; on an empty input, press twice to quit |
| `Ctrl+D` | Delete the character after the cursor; on an empty input, press twice to quit |

Links (`http://` or `https://`) in the scrollback are underlined and turn blue under the pointer; click one to open it in your browser. Drag across text in the scrollback or the input box to select it; it's copied to the clipboard when you let go, with line breaks only where the lines really break, not where they wrap. (Your terminal's own selection usually still works with `Shift` or `Option` held.) Click in the input box to move the cursor there. Each line that will be sent on its own starts with `>`. When Kiln is asking you something instead (a password, whether to save it, or to connect), the input box shows it in dim text with no `>`.

The sidebar lists the characters you have open. Click one to switch to it, double-click a disconnected one to reconnect, or click its `×` to close it. Connected characters have no mark; `…` means connecting and `×` disconnected. On the right, a number counts unread lines, and `●` means one of them needs your attention (a page or whisper, by default).

### Commands

| Command | Does |
|---|---|
| `/connect`, `/reconnect` | Connect now (skips the reconnect wait) |
| `/disconnect` | Disconnect and stay disconnected |
| `/close` | Disconnect and remove the character from the sidebar |
| `/browse` | Open browse mode |
| `/highlight <text>` | Highlight lines containing this text (saved to the world's file) |
| `/trust` | Accept a changed server certificate (see below) |
| `/quit` | Quit Kiln |

To send a line that starts with `/`, double it: `//me waves` sends `/me waves`.

### Browse mode

`Ctrl+B` opens a browser over **all** of the active character's logs, with new lines still arriving at the bottom. `Esc` goes back.

| Key | Does |
|---|---|
| `↑`/`↓` (`k`/`j`), `PgUp`/`PgDn`, `Home`/`End` | Move; moving past the top loads older days |
| `g` | Go to a date (`2026-09-24`) |
| `/`, then `n` / `N` | Find, then next or previous match |
| `1`–`9`, or click a tag | Cycle a tag filter: neutral → `+tag` (only these) → `−tag` (hide these) |
| `m` | Mark the start of a range, then its end |
| `Space` | Leave the current line out of the range |
| Click, shift-click | Select a line; shift-click outside the range to extend it, or inside it to leave a line out (or bring it back) |
| `e` | Export the range as plain text, ANSI or HTML |
| `c` | Copy the range as plain text |

Exports contain only received lines: no timestamps, your own commands (the server already echoes your poses) or lines hidden by filters. They're saved to `export_dir` (default `~/Documents/Kiln Scenes`) under a name from `export_name`, and Kiln never overwrites an existing file.

## Configuration

```
~/.config/kiln/
  config.toml          # global settings and defaults
  worlds/<id>.toml     # one world and its characters
  packs/<id>.toml      # shareable rule sets, e.g. the starter fuzzball.toml
```

Settings are inherited in this order: **defaults → packs (in `use` order) → world → character**. Rule lists add up along the way, and single settings are replaced by the most specific level.

| Setting | Where | Meaning |
|---|---|---|
| `export_dir` | config.toml | Where browse exports are saved |
| `export_name` | config.toml | Export file name, from `{date}`, `{time}`, `{world}` and `{name}` (default `"{date} {time} {world} {name}"`; may include `/` for subfolders) |
| `export_format` | config.toml | `"plain"`, `"ansi"` or `"html"`: what `Enter` picks when exporting (default: always ask) |
| `log_dir` | config.toml | Where logs go, with `{world}` and `{char}` filled in (default `~/.local/share/kiln/logs/{world}/{char}`), see [Logs](#logs) |
| `password_store` | config.toml | `"keychain"` (default), `"file"` (`~/.local/share/kiln/passwords.json`, readable only by you) or `"none"` (never save) |
| `host`, `port`, `tls` | world | Where to connect |
| `tls_trust` | world | `"pin"` (default) or `"ca"`, see below |
| `login` | world, character | Login template; `{name}` and `{password}` are filled in |
| `use` | world | Rule packs to apply |
| `max_line_bytes` | any level | Longest line the server accepts (Fuzzball: 2047) |
| `newline_mode` | any level | `"batch"`: each line is its own command; `"flatten"`: lines are joined with spaces |
| `autoconnect` | any level | Connect when Kiln starts |
| `local_echo` | any level | Show what you send in the scrollback (default `false`; the server usually echoes poses, and sent lines are always logged) |
| `name`, `aliases` | character | Your in-game name and the others that count as you |

### Rules

Classify rules tag lines, and highlight rules style them:

```toml
[[classify]]
tag = "ooc"
pattern = '^\[OOC\]'

[[highlight]]
match = { tags = ["page"] }          # match by tag, by pattern = '…', or both
style = { fg = "#ff9f43", bold = true }
attention = true                     # light up the ● badge
```

Tags are worked out when lines are shown, never saved. Fixing a rule fixes old logs too. A character can add rules of its own with `[[characters.classify]]` and `[[characters.highlight]]` right after its `[[characters]]` entry.

By default a highlight styles the whole line. With `scope = "match"` it styles only the part that matched: its own `pattern`'s matches, or else the text its tags' classify rules matched. So a server that prefixes pages with `PAGE:` can color just the prefix, and `self` can bold just your name:

```toml
[[classify]]
tag = "page"
pattern = '^PAGE:'

[[highlight]]
match = { tags = ["page"] }
style = { fg = "#2053ff", bold = true }
scope = "match"                      # just "PAGE:"; attention still marks the line
attention = true

[[highlight]]
match = { tags = ["self"] }
style = { bold = true }
scope = "match"                      # just your name, wherever it appears
```

### Certificates

With `tls_trust = "pin"` (the default), Kiln remembers a server's certificate the first time it connects, which suits the self-signed certificates most MUCKs use. If the certificate later changes, Kiln refuses to connect and says so. If you expected the change (the server renewed its certificate), accept it with `/trust`, or from a shell:

```sh
kiln trust <world> <fingerprint>     # the fingerprint is shown in the error
```

Use `tls_trust = "ca"` for servers with certificates from a real certificate authority.

## Logs

Kiln starts a new log file each time a character connects, named after that moment: `~/.local/share/kiln/logs/<world>/<character>/2026-09-24 211403 Kit.log`. To keep them with your other logs, set `log_dir` in `config.toml`:

```toml
log_dir = "~/Documents/Logs/Mucks/{world}"   # {char} is the character's id
```

Characters can share a folder: each file name ends with the character's id, and Kiln only reads back its own files. Files that aren't Kiln logs are left alone and skipped, even if their names look like Kiln's. Changing `log_dir` doesn't move old logs; move them yourself if you want Kiln to show them. Older versions of Kiln wrote one file per day (`YYYY-MM-DD.log`), and those are still read.

Every file looks like this:

```
#kiln-log v1
2026-09-24T21:14:03.120-07:00 <	Rook says, "Evening!"
2026-09-24T21:14:15.002-07:00 >	:grins.
2026-09-24T21:20:00.000-07:00 *	disconnected (reset); retrying in 5s
```

`<` marks received lines, `>` sent lines, and `*` Kiln's own notes. Colors are kept, so `less -R` shows them. `grep` works directly, and `cut -f2-` gives a bare transcript. Passwords are never logged.

Kiln uses `~/.config/kiln` and `~/.local/share/kiln` on every system, including macOS and Windows. Set `XDG_CONFIG_HOME` or `XDG_DATA_HOME` to move them.

## Other commands

```
kiln                               the full-screen client
kiln tail <world> <char>           connect, print output, send lines typed on stdin
kiln passwd <world> <char>         save a character's password (see password_store)
kiln trust <world> <fingerprint>   accept a changed server certificate
```

## Releasing

Releases are built by GitHub Actions ([`.github/workflows/release.yml`](.github/workflows/release.yml)) with [GoReleaser](https://goreleaser.com) ([`.goreleaser.yaml`](.goreleaser.yaml)).

**From GitHub (no terminal needed):**

1. Open the repository's **Actions** tab and pick **release** in the list on the left.
2. Press **Run workflow**, leave the branch as `main`, type the version (like `v0.1.0`), and press the green **Run workflow** button.
3. Wait a few minutes. The workflow runs the tests, tags `main` with that version, builds every binary, and creates a **draft** release.
4. Open the **Releases** page, check the notes and files, and press **Publish release**. Until then, nothing is public.

If the run fails before tagging (tests failed, or the version is malformed or already used), nothing changes: fix it and run again. If it fails after tagging, delete the tag from the repository's **Tags** page (or `git push --delete origin v0.1.0`) before retrying.

**From a terminal:** pushing a version tag starts the same workflow.

```sh
git checkout main && git pull
git tag v0.1.0
git push origin v0.1.0
```

To try the build locally without publishing anything, run `goreleaser release --snapshot --clean` (the output goes to `dist/`).

## License

[MIT](LICENSE)
