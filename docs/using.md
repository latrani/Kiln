# Using Kiln

> **AI-generated.** Claude wrote this page from Kiln's code and earlier README. If something here is wrong, please [open an issue](https://github.com/latrani/Kiln/issues).

## Keys

| Key | Does |
|---|---|
| `Enter` | Send (on an empty input, connect a disconnected character) |
| `Shift+Enter` (or `Alt+Enter`) | New line in the input box |
| `↑` / `↓` | Move between rows, or recall input history from the top or bottom row |
| `Ctrl+←` / `Ctrl+→` (or `Alt`/`Option`) | Move by word |
| `Home` / `End` (or `Ctrl+A` / `Ctrl+E`) | Start or end of the line |
| `Ctrl+W` (or `Alt+Backspace`) / `Alt+Delete` | Delete the word before or after the cursor |
| `Ctrl+U` / `Ctrl+K` | Delete to the start or end of the line |
| `Ctrl+↑` / `Ctrl+↓` | Switch between open characters and their worlds' overviews |
| `Tab` / `Shift+Tab` | Jump to the next (or previous) character with unread lines; with none, back to the one you were on before |
| `Ctrl+O` | Open a connection (same as `/open`): type to filter, `Enter` to connect, `Esc` to close |
| `Ctrl+T` | Edit the active character (same as `/edit`), or on a world's overview the world; in the `Ctrl+O` list, the highlighted world or character (`Enter` on a world does it too). In the editor, `Ctrl+T` again closes it without saving, like `Esc` |
| `PgUp` / `PgDn`, mouse wheel | Scroll back, or page through held output (see [Paging](#paging)) |
| `Ctrl+L` | Open log mode; in log mode, close it |
| `Esc` | Skip the login prompt; while scrolled back, jump to live (so does clicking the `▼ new` pill) |
| `Ctrl+C` | Clear the input; on an empty input, press twice to quit |
| `Ctrl+D` | Delete the character after the cursor; on an empty input, press twice to quit |

The editing keys (word and line movement, deletes) work the same in every text field: the input box, the `Ctrl+O` filter and forms, and log mode's prompts.

Links (`http://` or `https://`) in the scrollback are underlined and turn blue under the pointer; click one to open it in your browser (over ssh it copies the link instead; see [Over ssh](notifications.md#over-ssh)). Drag across text in the scrollback or the input box to select it; it's copied to the clipboard when you let go, with line breaks only where the lines really break, not where they wrap. (Your terminal's own selection usually still works with `Shift` or `Option` held.) Click in the input box to move the cursor there. Each line that will be sent on its own starts with `>`. When Kiln is asking you something instead (a password, whether to save it, or to connect), the input box shows it in dim text with no `>`.

The sidebar lists the characters you have open. Click one to switch to it, double-click a disconnected one to reconnect, or click its `×` to close it. Connected characters have no mark; `…` means connecting and `×` disconnected. On the right, a number counts unread lines, and `●` means one of them needs your attention (a page or whisper, by default).

Click a world's name (or reach it with `Ctrl+↑`/`Ctrl+↓`) for its overview: each of its open characters, when their last line came, and their last five lines.

## Commands

| Command | Does |
|---|---|
| `/connect`, `/reconnect` | Connect now (skips the reconnect wait) |
| `/disconnect` | Disconnect and stay disconnected |
| `/close` | Disconnect and remove the character from the sidebar |
| `/open` | Open a connection (same as `Ctrl+O`) |
| `/log` | Open log mode |
| `/highlight <text>` | Highlight lines containing this text (saved to the world's file as a `highlight` tag) |
| `/away` | Count as away right now: notifications go out, until your next key or click |
| `/notify [level]` | Show or set (until Kiln quits) what notifies for this character: `all`, `first`, `attention`, `none`, or `default` to go back to the config |
| `/edit`, `/edit world` | Edit the active character, or its world |
| `/trust` | Accept a changed server certificate (see [Certificates](configuration.md#certificates)) |
| `/quit` | Quit Kiln |

To send a line that starts with `/`, double it: `//me waves` sends `/me waves`.

## Paging

Kiln never scrolls text past you unread. It counts what you've read up to the last line you sent (and up to wherever you've paged to). Once more has arrived since then than fits on the screen, the view stops with the first unread line at the top, and everything after it waits below behind the `▼ new` pill. With `local_echo` on, that first line is your own last sent line, so you can see what the new output answers. The time between lines and whether you're away make no difference, so room chatter piles up the same way a long room description or a `WHO` list does. `PgDn` shows the next screenful, and `Esc` or clicking the pill jumps to live. Sending a line starts over from there. Switching to a character that piled up more than a screen while you were elsewhere opens it at the first line you missed.

## Other commands

```
kiln [--no-autoconnect]            the full-screen client (with the flag, nothing opens at start)
kiln tail <world> <char>           connect, print output, send lines typed on stdin
kiln passwd <world> <char>         save a character's password (see password_store)
kiln trust <world> <fingerprint>   accept a changed server certificate
kiln theme show <theme>            print a theme, merged into one file (swatches after each color on a terminal)
kiln version                       print Kiln's version (also --version)
```
