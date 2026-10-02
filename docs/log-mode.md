# Log mode

> **AI-generated.** Claude wrote this page from Kiln's code and earlier README. If something here is wrong, please [open an issue](https://github.com/latrani/Kiln/issues).

`Ctrl+L` opens a browser over **all** of the active character's logs, with new lines still arriving at the bottom. `Ctrl+L` (or the `Log` chip in the top bar) goes back and keeps log mode as you left it: the filter panel, cursor, marks and find are all there when you open it again with `Ctrl+L`, the chip or `/log`, even if you've looked at other characters in between. `Esc` goes back and starts fresh next time. The filter itself is kept either way.

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

## Filter

`f`, or the ` Filter ` chip at the top right, opens the filter panel in the sidebar. It lists **Untagged** (lines with no tags, including your own commands), every tag in the loaded logs or with a filter set, children indented under their parents (`page/in` under `page`), then any text you've added, each with **Hide** and **Only**:

- **Hide** hides lines with that tag (or text). Hiding a parent hides its children too; unhiding one child brings its parent back and leaves the other children hidden.
- **Only** shows just lines with that tag (or text), and lights Hide on every other tag (or text) so you can see what's hidden. Unhide any of them and Only lets go; press Only again to clear the tag (or text) filters; press Hide on it to flip it, showing everything but that. Tags and text are separate: Only on a text hides the other texts, not your tags.
- **+ Text** adds text to filter on, matched like `/` find. It starts on Only. The `×` in front of it, `x` or `Delete` removes it.

`↑`/`↓` move, `h`/`o` press Hide/Only, `←`/`→` fold a parent (every parent starts folded; a folded parent with a filter inside shows `•`), and `f` or `Esc` closes the panel. The log updates as you go. Filters stay set until you quit Kiln.
