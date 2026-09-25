# Sidebar redesign (#40), with room for #41

## Goal

The sidebar scales to many worlds and characters by showing only what
you're actually using: characters that are connected, or disconnected but
not yet cleared. Everything else is one step away in an "add connection"
picker. #41 (adding worlds and characters from inside Kiln) builds on the
picker and on the form in the input area, so both are shaped for it now.

## What the sidebar holds

- **Open characters only.** `m.chars` and `m.order` hold *open* characters.
  A `charState` (scrollback preload, input, session) is created when a
  character is opened and dropped when it's closed, not made for every
  character in the config.
- The model keeps the latest `*config.Config` (`m.cfg`), which the picker
  lists from.
- **Startup:** characters with `autoconnect = true` are opened and
  connected. Nothing else is remembered between runs.
- **Order:** alphabetical, case-insensitive: worlds by id, then characters
  by name within each world. The picker uses the same order. (The config
  keeps file order; the UI sorts.) Because the order is always sorted, an
  orphan (open character removed from the config) needs no special
  placement, and `insertInWorld` goes away.
- **Rows:** a header per world that has open characters, the open
  characters under it, then `+ Add connection` as the last row.
- Collapsing a world by clicking its header works as today.

### Row layout

```
▾ fm
    Ash
  ✕ Kit        ● 3
  … Rook         12
+ Add connection
```

- **Left slot: connection state only.** Blank when connected, `…` while
  connecting, `✕` when disconnected or failed.
- **Right side: activity.** The unread count, prefixed with `●` in the
  highlight color when a line needed attention (a highlight rule with
  `attention = true`, e.g. pages and whispers). Attention and unread clear
  when you switch to the character, as today.
- **Truncation:** names (characters, worlds, picker rows, and
  `+ Add connection` itself) are cut with `…` to fit. The right-side
  activity is never truncated; the name gives way to it.
- The active character's row is shown in reverse video, as today.

## Closing

- **Clicking the `✕` cell** closes that character: its session is
  cancelled, its `charState` dropped, and it leaves the sidebar. Its logs
  stay, so reopening it preloads history as usual.
- The `✕` hit zone is only the badge cell. Clicking the name switches;
  double-clicking the name reconnects (as in #35). So a double-click on
  the name never closes.
- **`/close`** closes the active character, disconnecting it first if it's
  connected.
- When the active character closes, the next one down becomes active, or
  the one above if it was last. With nothing open, there is no active
  character (see the empty state).
- Ctrl+↑/↓ cycles through open characters only.

## The picker

**Opened by** Ctrl+O, clicking `+ Add connection`, or Enter on the empty
state.

- **The sidebar swaps** to every configured world and character, minus the
  ones already open, in the same alphabetical order. No connection badges,
  no activity. Worlds left with no characters to offer are hidden (in #41
  they stay, since they'll have `+ Add character`).
- **The right pane is unchanged**, showing whoever is active.
- **The input area becomes a filter**, drawn as a prompt-state row:
  `Filter: ma▏  · Enter to connect · Esc to close`. It is the one field of
  a `form` (see below).
- **Filter** matches the character's name, aliases, or world id,
  case-insensitive substring. A world header shows when any of its
  characters match.
- **Keys:** ↑/↓ move the highlight (skipping world headers), PgUp/PgDn move
  by a page, the wheel scrolls. Typing edits the filter; the highlight
  stays on the same character if it still matches, else it goes to the first match.
  Enter or a click on a character opens it, connects it, makes it active,
  and closes the picker. Esc closes the picker without changes. Clicking a
  world header in the picker does nothing in #40.
- A config reload while the picker is open refreshes its list and keeps the
  filter.

### Empty state

With nothing open, the right pane is blank and the input area shows the
prompt `Nothing open · Enter or Ctrl+O to add a connection`. Enter opens
the picker. `/commands` still type and run as usual (typing replaces the
prompt, like the connect hint).

## Room for #41

- **Picker rows have a kind:** `world`, `character`, `addCharacter`,
  `addWorld`. #40 only produces the first two. #41 adds
  `+ Add character` at the end of each world and `+ Add world` at the end
  of the list.
- **The filter is a `form`:** a small type holding labeled fields, each
  with its own `Input`, drawn as the prompt-state row in the input area
  and owning the keys while open. #40's form has one field. #41's world
  editor adds fields (server, port, a TLS toggle) with Tab/Shift+Tab
  between them, and its character editor has a name field; Enter saves,
  Esc cancels. Designing multi-field layout and toggles is #41's job; #40
  only has to avoid assuming one field in how the form is stored and drawn.
- **Config writes:** `[[characters]]` entries can be appended safely to a
  world file (headers are absolute, as `/highlight` already relies on),
  and a new world is a new file. #41 adds those writers next to
  `AppendHighlight`; the config watcher picks the result up.

## Testing

- Open, close by `✕` click, `/close`, and active-character hand-off.
- Startup opens only autoconnect characters; the sidebar always ends with
  `+ Add connection`.
- Badge and activity rendering: blank when connected, `…`, `✕`, `● 3`, and
  truncation with `…` that never cuts the activity.
- Alphabetical, case-insensitive ordering in the sidebar and picker.
- `✕` click vs name click vs double-click on the name.
- Picker: Ctrl+O and `+ Add connection` open it; filter by name, alias and
  world; ↑/↓ skip headers; Enter and click open and connect; Esc closes;
  already-open characters are hidden; reload keeps the filter.
- Empty state prompt, and Enter opening the picker.
- README: sidebar, picker, `/close`, Ctrl+O.
