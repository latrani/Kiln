# Setting up Kiln

> **AI-generated.** Claude wrote this page from Kiln's code and earlier README. If something here is wrong, please [open an issue](https://github.com/latrani/Kiln/issues).

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

4. **Run `kiln`.** Characters with `autoconnect = true` open and log in (`kiln --no-autoconnect` skips that for one run). Press `Ctrl+O` (or click `+ Open connection`, or type `/open`) to open another.

You can also edit worlds and characters from inside Kiln (`Ctrl+T` or `/edit` for the active character, or `Ctrl+T` on anything in the `Ctrl+O` list): a world's server and settings, a character's aliases and settings, and forgetting a saved password or deleting a character (logs are kept) or an empty world. Kiln changes only the lines it has to, so your comments and layout stay put.

Config changes apply live while Kiln runs. If a file has a mistake, Kiln keeps the last working config and shows the error in the statusline.

## Where Kiln keeps things

Kiln splits its files in two: settings you edit, and data it writes.

```
~/.config/kiln/          # settings (see configuration.md)
  config.toml
  worlds/<id>.toml
  packs/<id>.toml

~/.local/share/kiln/     # data
  logs/<world>/<char>/   # logs, unless log_dir moves them (see logs.md)
  passwords.json         # saved passwords, only with password_store = "file"
  known_hosts            # pinned server certificates (see configuration.md)
  kiln.log               # why Kiln last quit with an error, and crash reports
```

With the default `password_store = "keychain"`, passwords are in your OS keychain instead, under the service name `kiln` and the account `<world>/<char>`.

If Kiln quits with an error, the message is printed and also added to `kiln.log`, with the stack if it crashed. That's the place to look when Kiln closes before you can read why. On Windows, when Kiln has a window to itself (you double-clicked it), it waits for Enter before closing it.

Those paths are the same on every system, macOS and Windows included. On macOS, `~/.local` is hidden in Finder: press `Cmd+Shift+G` and paste the path, or `Cmd+Shift+.` to show hidden files. To move either folder, set `XDG_CONFIG_HOME` or `XDG_DATA_HOME` (Kiln then uses `kiln/` inside it), but other programs that follow the same convention will move too.
