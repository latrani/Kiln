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
- **Knows what a page is.** Lines are tagged (page, whisper, and `self` when they mention you) by rules you can edit. The theme colors tags, and tags drive the attention badge.
- **Everything is logged** to plain, greppable text, one file per session, in whatever folder you like.
- **Log mode** pages back through all of a character's logs. You can filter by tag or text, search, mark a range, drop stray lines, and export the scene as plain text, ANSI or HTML.
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

4. **Run `kiln`.** Characters with `autoconnect = true` open and log in. Press `Ctrl+O` (or click `+ Open connection`, or type `/open`) to open another.

You can also edit worlds and characters from inside Kiln (`Ctrl+T` or `/edit` for the active character, or `Ctrl+T` on anything in the `Ctrl+O` list): a world's server and settings, a character's aliases and settings, and forgetting a saved password or deleting a character (logs are kept) or an empty world. Kiln changes only the lines it has to, so your comments and layout stay put.

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
| `Tab` / `Shift+Tab` | Jump to the next (or previous) character with unread lines; with none, back to the one you were on before |
| `Ctrl+O` | Open a connection (same as `/open`): type to filter, `Enter` to connect, `Esc` to close |
| `Ctrl+T` | Edit the active character (same as `/edit`); in the `Ctrl+O` list, the highlighted world or character (`Enter` on a world does it too) |
| `PgUp` / `PgDn`, mouse wheel | Scroll back, or page through held output (see [Paging](#paging)) |
| `Ctrl+L` | Open log mode |
| `Esc` | Skip the login prompt; while scrolled back, jump to live (so does clicking the `▼ new` pill) |
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
| `/open` | Open a connection (same as `Ctrl+O`) |
| `/log` | Open log mode |
| `/highlight <text>` | Highlight lines containing this text (saved to the world's file as a `highlight` tag) |
| `/away` | Count as away right now: output is held and notifications go out, until your next key or click |
| `/notify [level]` | Show or set (until Kiln quits) what notifies for this character: `all`, `first`, `attention`, `none`, or `default` to go back to the config |
| `/edit`, `/edit world` | Edit the active character, or its world |
| `/trust` | Accept a changed server certificate (see below) |
| `/quit` | Quit Kiln |

To send a line that starts with `/`, double it: `//me waves` sends `/me waves`.

### Paging

Kiln never scrolls text past you unread. When a burst of output (a long room description, a `WHO` list) is taller than the screen, the view stops with its first line at the top, and the rest waits below behind the `▼ new` pill. The same happens with everything that arrives while you're away (see [Notifications](#notifications), or say `/away`), and when you switch to a character that piled up more than a screen while you were elsewhere. `PgDn` shows the next screenful, and `Esc`, clicking the pill, or sending a line jumps to live. Lines that trickle in (less than a second apart counts as one burst) scroll normally while you're here.

### Log mode

`Ctrl+L` opens a browser over **all** of the active character's logs, with new lines still arriving at the bottom. `Esc` goes back.

| Key | Does |
|---|---|
| `↑`/`↓` (`k`/`j`), `PgUp`/`PgDn`, `Home`/`End` | Move; moving past the top loads older days |
| `g` | Go to a date (`2026-09-24`) |
| `/`, then `n` / `N` | Find, then next or previous match |
| `f`, or click ` Filter ` | Open the filter panel (see below) |
| `t` | Show the current line's tags, and whose style each one takes |
| `m` | Mark the start of a range, then its end |
| `Space` | Leave the current line out of the range |
| Click, shift-click | Select a line; shift-click outside the range to extend it, or inside it to leave a line out (or bring it back) |
| `e` | Export the range as plain text, ANSI or HTML |
| `c` | Copy the range as plain text |

Exports contain only received lines: no timestamps, your own commands (the server already echoes your poses) or lines hidden by filters. They're saved to `export_dir` (default `~/Documents/Kiln Scenes`) under a name from `export_name`, and Kiln never overwrites an existing file.

#### Filter

`f`, or the ` Filter ` chip at the top right, opens the filter panel in the sidebar. It lists **Untagged** (lines with no tags, including your own commands), every tag in the loaded logs or with a filter set, children indented under their parents (`page/in` under `page`), then any text you've added, each with **Hide** and **Only**:

- **Hide** hides lines with that tag (or text). Hiding a parent hides its children too; unhiding one child brings its parent back and leaves the other children hidden.
- **Only** shows just lines with that tag (or text), and lights Hide on every other tag (or text) so you can see what's hidden. Unhide any of them and Only lets go; press Only again to clear the tag (or text) filters; press Hide on it to flip it, showing everything but that. Tags and text are separate: Only on a text hides the other texts, not your tags.
- **+ Text** adds text to filter on, matched like `/` find. It starts on Only. `x` or `Delete` removes it.

`↑`/`↓` move, `h`/`o` press Hide/Only, `←`/`→` fold a parent (a folded parent with a filter inside shows `•`), and `f` or `Esc` closes the panel. The log updates as you go. Filters stay set until you quit Kiln.

### Themes

Kiln's colors come from a theme. Kiln makes `themes/default.toml` in its config folder on first run, saying `extends = "kiln"`: it starts as the built-in theme, `kiln`, and changes only what you set. Kiln picks up changes as you save. `kiln theme show kiln` prints everything the built-in sets, and `kiln theme show <name>` prints any theme with what it extends merged in. To keep several themes, give each its own file in `themes/` and pick one with `theme = "name"` in `config.toml`.

    extends = "kiln"

    [palette]
    panel = "#1f2029"

    [ui]
    "sidebar"        = { bg = "panel" }
    "sidebar.active" = { reverse = true }
    "status"         = { bg = "panel" }
    "status.error"   = { fg = "#ff6b6b", bold = true }

    [tags]
    "page/in"   = { fg = "#ff9f43", bold = true }
    "highlight" = { fg = "#ffd166", scope = "match" }

`[ui]` styles Kiln's own parts of the screen, by role; `[tags]` styles lines from the server, by their [tags](#rules). A style sets any of `fg`, `bg`, `bold`, `faint`, `italic`, `underline` and `reverse`. A color is `#rrggbb`, a name from `[palette]`, one of the terminal's own sixteen (`red`, `bright-blue`, and so on), or `default` for the terminal's own color, which clears one a role would inherit. A tag style can also say `scope = "match"` (see [Rules](#rules)). A role inherits from the one before its last dot (`link.hover` starts from `link`). Text from the server always sits on your terminal's background; the sidebar (the picker while it's open), input box (a form while one is up), statusline and log mode's header and action bar can have their own. If a theme has a mistake, Kiln says so in the statusline and keeps the colors it had.

A theme can give different colors for light and dark terminals: `[palette.light]` and `[palette.dark]` replace entries of `[palette]`, and everything that names them follows along.

    [palette]
    ember = "#ff9f43"

    [palette.light]
    ember = "#b35900"        # darker, to read on a light background

With `appearance = "auto"` (the default), Kiln asks the terminal what its background is when it starts and whenever its window comes back into focus, and assumes dark until it hears back. The built-in theme has a light palette of its own. Over mosh the terminal can't be asked (eternal terminal and tmux pass the question on), so on a light terminal set `appearance = "light"`.

The roles are: `sidebar` (`.world`, `.char`, `.active`, `.unread`, `.attention`, `.connecting`, `.disconnected`, `.add`, `.more`), `picker` (`.world`, `.world.selected`, `.selected`, `.add`), `divider`, `rule` (`.input`, `.form`, `.status`: each rule above its area), `scrollback` (`.day`, `.history_end`, `.loading`, `.echo`, `.sys`, `.pill`, `.selection`, `.inactive`, `.empty`), `link` (`.hover`), `input` (`.hint`, `.over_limit`, `.selection`), `status` (`.log`, `.log.on`, `.filter`, `.filter.on`, `.error`), `form` (`.label`, `.hint`, `.error`, `.focus`, `.title`, `.button`, `.button.secondary`), `log` (`.time`, `.cursor`, `.selected`, `.excluded`, `.find`, `.day`, `.loading`, `.bar`, `.bar.hints`), `filter` (`.item`, `.selected`, `.button`, `.button.on`, `.add`, `.more`), and `export` (the HTML export's page).

### Notifications

When you're away from Kiln, activity shows up as a desktop notification like `Kit: Rook pages: you around?` (`Kit@fm:` when two worlds have a Kit). You're away when you switch to another window or tab, after `notify_idle` (default 5 minutes) without typing or clicking, or from `/away` until your next key or click. Something that arrives while you still count as here is held, and sent (with how many more followed) if `notify_idle` passes without you coming back. The `notify` setting picks what notifies: `first` (the default) sends the first line since you left and then only lines that need attention (pages and whispers), `all` sends every line, `attention` only those, and `none` nothing. Quiet lines never notify, and for a few seconds after connecting only attention lines do, so the login banner stays quiet. A burst of lines (like a room description) notifies only its first line. `/notify` changes it for one character until Kiln quits.

Notifications work in iTerm2, kitty, Ghostty, WezTerm, foot and Blink, locally or over ssh. Inside tmux, add this to `~/.tmux.conf`:

    set -g allow-passthrough on
    set -g focus-events on

Then run `tmux source-file ~/.tmux.conf` and detach and reattach: tmux asks your terminal for focus events only when you attach.

Mosh drops notifications, but it passes on the bell: set `notify_method = "both"` and turn on Blink's "Notification on background shell" to get an alert (without the line).

## Where Kiln keeps things

Kiln splits its files in two: settings you edit, and data it writes.

```
~/.config/kiln/          # settings (see Configuration)
  config.toml
  worlds/<id>.toml
  packs/<id>.toml

~/.local/share/kiln/     # data
  logs/<world>/<char>/   # logs, unless log_dir moves them (see Logs)
  passwords.json         # saved passwords, only with password_store = "file"
  known_hosts            # pinned server certificates (see Certificates)
```

With the default `password_store = "keychain"`, passwords are in your OS keychain instead, under the service name `kiln` and the account `<world>/<char>`.

Those paths are the same on every system, macOS and Windows included. On macOS, `~/.local` is hidden in Finder: press `Cmd+Shift+G` and paste the path, or `Cmd+Shift+.` to show hidden files. To move either folder, set `XDG_CONFIG_HOME` or `XDG_DATA_HOME` (Kiln then uses `kiln/` inside it), but other programs that follow the same convention will move too.

## Configuration

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
| `export_name` | config.toml | Export file name, from `{date}`, `{time}`, `{world}`, `{name}` and [time codes](#name-templates) (default `"{date} {time} {world} {name}"`; may include `/` for subfolders) |
| `export_format` | config.toml | `"plain"`, `"ansi"` or `"html"`: what `Enter` picks when exporting (default: always ask) |
| `log_dir` | config.toml | Where logs go (default `~/.local/share/kiln/logs/{world}/{char}`), see [Logs](#logs) |
| `log_name` | config.toml | Log file names, without `.log` (default `"%Y-%m-%d %H%M%S {char}"`), see [Logs](#logs) |
| `password_store` | config.toml | `"keychain"` (default), `"file"` (`~/.local/share/kiln/passwords.json`, readable only by you) or `"none"` (never save) |
| `notify_idle` | config.toml | No input for this long counts as away (default `"5m"`; `"0"`: only switching away counts), see [Notifications](#notifications) |
| `notify_method` | config.toml | `"osc"` (default): a notification with the line; `"bell"`: a bell (works over mosh); `"both"` |
| `theme` | config.toml | Which `themes/<name>.toml` to draw with (default `"default"`, which Kiln makes on first run; `"kiln"` is the built-in), see [Themes](#themes) |
| `appearance` | config.toml | `"auto"` (default: ask the terminal), `"dark"` or `"light"`: which palette themes use, see [Themes](#themes) |
| `host`, `port`, `tls` | world | Where to connect |
| `tls_trust` | world | `"pin"` (default) or `"ca"`, see below |
| `login` | world, character | Login template; `{name}` and `{password}` are filled in |
| `use` | world | Rule packs to apply |
| `max_line_bytes` | any level | Longest line the server accepts (Fuzzball: 2047) |
| `newline_mode` | any level | `"batch"`: each line is its own command; `"flatten"`: lines are joined with spaces |
| `autoconnect` | any level | Connect when Kiln starts |
| `reconnect` | any level | Retry with backoff after a drop or failed connect (default `true`); `false` stays disconnected until `/connect` |
| `notify` | any level | What notifies while you're away: `"all"`, `"first"` (default: the first line, then attention lines), `"attention"` or `"none"` |
| `local_echo` | any level | Show what you send in the scrollback (default `false`; the server usually echoes poses, and sent lines are always logged) |
| `name`, `aliases` | character | Your in-game name and the others that count as you |

### Rules

Three things decide what happens to a line from the server: **classify rules** say what it is (they give it tags), the **theme** says how each tag looks, and the **`attention` and `quiet` lists** say how it behaves.

```toml
attention = ["page/in", "whisper/in", "self"]   # light up the ● badge
quiet = ["wiki"]                                 # never unread, never notifies

[[classify]]
tag = "ooc"
pattern = '^\[OOC\]'

[[classify]]
tag = "wiki"
pattern = '^\[Wiki\]'
```

A classify rule can give several tags at once with `tags = ["page", "page/in"]` (instead of, or as well as, `tag`). By convention a `/` nests a tag under a broader one: the starter pack tags pages you receive `page` and `page/in`, and the server's echo of your own (`You page, …`, `You page-pose, …`) `page` and `page/out`, so a `page` filter shows the whole conversation while only `page/in` asks for attention. Whispers work the same way (`whisper/in`, and `whisper/out` for `You whisper, …`). Kiln itself tags lines that mention your character's name or aliases `self`.

`attention` and `quiet` add up along the chain (defaults, packs, world, character), and an entry covers the tags under it: `attention = ["page"]` covers `page/in` too (but not `pages`). Quiet lines are still shown, but don't count as unread, don't bump the `▼ new` count, never light the badge and never notify, even if another tag (like `self`) asks for attention.

Tags are worked out when lines are shown, never saved. Fixing a rule fixes old logs too. A character can add rules of its own with `[[characters.classify]]` right after its `[[characters]]` entry, and `attention` and `quiet` lists inside it.

How tags look lives in the theme's `[tags]` (see [Themes](#themes)). A world or character can add to it with its own `[palette]` (with `[palette.light]` and `[palette.dark]` too) and `[tags]`, over the theme it uses:

```toml
# worlds/fm.toml
[palette]
beacon = "#ffd166"          # a color only this world uses

[tags]
"highlight" = { fg = "beacon", bold = true, scope = "match" }
"ooc"       = { fg = "#808080" }
```

A character's own looks go inside its `[[characters]]` entry, as `[characters.tags]` and `[characters.palette]` (or inline, `tags = { … }`). A bare `[tags]` header after a `[[characters]]` entry still means the world's, since TOML table headers are absolute:

```toml
[[characters]]
name = "Kit"

[characters.tags]
"self" = { fg = "#ffd166", bold = true }   # just Kit
```

A tag style takes the same settings as a theme role, plus `scope`. By default it styles the whole line; with `scope = "match"` it styles only the text the tag's classify rules matched, so a server that prefixes pages with `PAGE:` can color just the prefix. A tag inherits from the one up its slashes, the way a role does up its dots: `page/in` starts from `page` and changes only what it sets, and `fg = "default"` clears a color it would inherit. When a line has several tags, whole-line styles go first and match styles on top; later colors win and attributes add up.

`/highlight <text>` adds a classify rule to the world's file that gives every line containing *text* the tag `highlight`. The theme styles that tag once for all your highlights: by default, just the matching text, in bold yellow.

#### Core tags

Themes can count on these tags; packs emit them where their server makes that possible, and are free to add their own.

| Tag | Lines |
|---|---|
| `page`, `page/in`, `page/out` | pages to you, and your own |
| `whisper`, `whisper/in`, `whisper/out` | whispers to you, and your own |
| `watchfor`, `watchfor/connect`, `watchfor/disconnect` | watchfor notices of people connecting and disconnecting |
| `ooc` | out-of-character talk |
| `system` | messages from the MUCK itself (not Kiln's own `*` lines) |
| `self` | lines that mention you (Kiln adds these) |
| `highlight` | lines matching a `/highlight` |

`say` and `pose` aren't core: on a MUCK they're the bulk of ordinary scene text, not something to set apart.

### Certificates

With `tls_trust = "pin"` (the default), Kiln remembers a server's certificate the first time it connects, which suits the self-signed certificates most MUCKs use. If the certificate later changes, Kiln refuses to connect and says so. If you expected the change (the server renewed its certificate), accept it with `/trust`, or from a shell:

```sh
kiln trust <world> <fingerprint>     # the fingerprint is shown in the error
```

Use `tls_trust = "ca"` for servers with certificates from a real certificate authority.

## Logs

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
#kiln-char tapestries/Indi
2026-09-24T21:14:03.120-07:00 <	Rook says, "Evening!"
2026-09-24T21:14:15.002-07:00 >	:grins.
2026-09-24T21:20:00.000-07:00 *	disconnected (reset); retrying in 5s
```

`<` marks received lines, `>` sent lines, and `*` Kiln's own notes. Colors are kept, so `less -R` shows them. `grep` works directly, and `cut -f2-` gives a bare transcript. Passwords are never logged.

### Name templates

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

## Other commands

```
kiln                               the full-screen client
kiln tail <world> <char>           connect, print output, send lines typed on stdin
kiln passwd <world> <char>         save a character's password (see password_store)
kiln trust <world> <fingerprint>   accept a changed server certificate
kiln theme show <theme>            print a theme, merged into one file
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
