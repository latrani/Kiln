# Kiln

Kiln is a modern MUCK client that runs in the terminal. It's built for social and roleplay worlds, focusing on tracking multiple characters and multiple worlds at the same time.

## Features
- A sidebar that shows your connected worlds and characters inside them, with their unread activity
- The ability to define patterns that set text highlights (in any color you want) and tag what the line meant (page, whisper, etc)
- A log export view that lets you easily choose what parts you want to save out, and can hide away those tagged lines
- A hint when you're about to bust the buffer with a pose, and the ability to split into a new line to send multiples at once.
- The ability to store passwords to your OS keychain (or a flat file if one isn't available)
- Mouse support and desktop notifications that even work over ssh and tmux

## Non-features
- No combat-MUD features like GMCP or MSDP are implemented or planned
- Currently no built-in scripting support, though that may be added at some point

## Installing

You can get the latest release for your OS on the [Releases page](https://github.com/latrani/Kiln/releases).

On macOS, the binaries aren't signed, so the first time you run it from download, you'll have to clear the quarantine flag in System Settings > Privacy & Security, or run this on the command line:

```sh
xattr -d com.apple.quarantine [path-to-kiln]
```

## Getting started

1. **Run `kiln` on the command line.** It will create itself a config directory, `~/.config/kiln/`, with some default configurations.

2. **Add a world and a character.** When you have worlds defined, they'll show up in the left sidebar. For now, choose `+ Connection` there, then `+ World`. Fill out the world info and save, then you can use `+ Character` to add a character name to it. All these are stored in TOML files in `~/.config/kiln/`

3. **Connect to your character.** You'll see the login message for your world, and get a password prompt. Type in your password and hit enter, and you'll get to choose whether or not to save it for the future.

## Using Kiln

When you start up, you'll be in regular interact mode. Worlds you connect to will show up in the sidebar, and you can use the input area to interact in the current one. Text editing should work mostly how you'd expect with a desktop text editor. Use `Tab` to jump between active worlds, and `Ctrl+↑` / `Ctrl+↓` to switch between all your open characters. `Ctrl+T` lets you edit settings for the current character, and `Ctrl+O` opens the full list of worlds and characters, where you can edit any of them and add new ones.

### Log mode

Hit `Ctrl+L` to enter Log mode, which shows a timestamp for each line Kiln has received in the current world. Here, you can filter in and out different types of lines, and select out text to save to a file. Kiln logs are stored in timestamped plain text. By default they're stored in `~/.local/share/kiln/logs/<world>/<char>/`.

## More

The [docs](docs/README.md) cover every key, command and setting, plus themes, rules, logs and notifications.

## Feedback and contributions

These are very welcome! Feel free to open issues and pull-requests, and happy MUCKing!
## License

[MIT](LICENSE)
