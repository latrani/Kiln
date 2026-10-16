# Log mode

> **AI-generated.** Claude wrote this page from Kiln's code and earlier README. If something here is wrong, please [open an issue](https://github.com/latrani/Kiln/issues).

`Ctrl+L` opens a browser over **all** of the active character's logs, with new lines still arriving at the bottom. `Ctrl+L` (or the `Log` chip in the top bar) goes back and keeps log mode as you left it: the filter panel, cursor, selection and find are all there when you open it again with `Ctrl+L`, the chip or `/log`, even if you've looked at other characters in between. `Esc` goes back and starts fresh next time; while selecting, `Esc` drops the range first. The filter itself is kept either way.

| Key | Does |
|---|---|
| `↑`/`↓` (`k`/`j`), `PgUp`/`PgDn`, `Home`/`End` | Move; moving past the top loads older days |
| Mouse wheel | Scroll a line at a time; the cursor stays put until it would scroll out of sight, then rides along at the edge. While selecting it stays put even out of sight, so scrolling never changes the range: a ▲ or ▼ by the time at the top or bottom says which way it went, and the next key that moves the cursor brings it back into view |
| `g` | Go to a day and time (`2026-09-24 21:04`): the last line in or before that minute. The prompt starts with the current line's, for you to edit. A day alone (`2026-09-24`) goes to its first line |
| `/`, then `n` / `N` | Find, newest first: the nearest match at or above the cursor, reading in older days until one turns up. Find is a mode: while it's on, `n` goes on to the next older match and `N` back to the next newer one (neither wraps around), and the statusline shows the count, starting from the newest match, with a `+` while older history isn't loaded yet. While it reads older days the statusline says how far back it's got; any key cancels it, and `Esc` only cancels it. `Esc` turns find off, staying in log mode; `/` starts with the last term, so `/` then `Enter` brings it back, and an empty `Enter` turns it off too |
| `f`, or click ` Filter ` | Open the filter panel (see below) |
| `s`, or click ` Select ` | Select mode: start selecting from the current line. The range's other end follows the cursor as you move (keys, paging) until `s` or the chip again locks it; `c` and `e` lock it too, and `Esc` drops it. The ` Select ` chip at the right of the statusline is lit while you're selecting |
| `Space` | Leave the current line out of the range |
| Click | Move the cursor. While selecting, a click outside the range grows the range to that line, and one inside leaves the line out (or brings it back) |
| `e` | Export the range as plain text, ANSI or HTML |
| `c` | Copy the range as plain text |

The statusline at the bottom says, left to right: find's count while find is on, the current line's tags (and whose style each one takes; nothing for a line with none), and how many lines are selected, while any are. The action bar above it lists the keys for where you are: nothing selected, selecting, or a range selected. A message covers them until it times out or you act again, and a narrow window cuts them from the right.

Exports contain only received lines: no timestamps, your own commands (the server already echoes your poses) or lines hidden by filters. They're saved to `export_dir` (default `~/Documents/Kiln Scenes`) under a name from `export_name`, and Kiln never overwrites an existing file.

## Filter

`f`, or the ` Filter ` chip at the top right, opens the filter panel in the sidebar. It lists **Untagged** (lines with no tags, including your own commands), every tag in the loaded logs or with a filter set, children indented under their parents (`page/in` under `page`), then any text you've added, each with **Hide** and **Only**:

- **Hide** hides lines with that tag (or text). Hiding a parent hides its children too; unhiding one child brings its parent back and leaves the other children hidden.
- **Only** shows just lines with that tag (or text), and lights Hide on every other tag (or text) so you can see what's hidden. Unhide any of them and Only lets go; press Only again to clear the tag (or text) filters; press Hide on it to flip it, showing everything but that. Tags and text are separate: Only on a text hides the other texts, not your tags.
- **+ Text** adds text to filter on, matched like `/` find. It starts on Only. The `×` in front of it, `x` or `Delete` removes it.

`↑`/`↓` move, `h`/`o` press Hide/Only, `←`/`→` fold a parent shut or open and `Space` or `Enter` toggles it, as does a click on its `>` or `▼` (every parent starts folded; a folded parent with a filter inside shows `•`), and `f` or `Esc` closes the panel. The log updates as you go. Filters stay set until you quit Kiln.
