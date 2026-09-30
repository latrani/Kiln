# Relayout: a top status bar

## Goal

Tidy the screen around two status bars, one on top and one at the
bottom, the same in both modes.

- **No clock.** A clock that ticks every minute trips terminals'
  activity monitors for nothing, and everyone already has one in view.
- **A top bar** holds the character and the mode toggles, ` Log `
  always, ` Filter ` in log mode. It replaces log mode's header.
- **The bottom bar** says one thing: how the connection stands, or in
  log mode how many lines are selected, or a status message.
- **Log mode's hint row** looks like the input box, with a rule above
  and below.

Nothing on screen changes unless something happened, so the activity
monitor only lights for real events.

## Layout

Right pane only; the sidebar keeps its full height.

```
NORMAL                                  LOG
fm       │fm/Kit              [ Log ]   Untagged │fm/Kit  find: Mira 1/1 [ Filter ][▓Log▓]
 Kit     │───────────────────────────    Hide Only│──────────────────────────────────────
         │Rook says, "Evening!"                  │── Thu Sep 24 ──
         │…                             ▼ page   │21:00   Rook says, "Evening!"
         │───────────────────────────    Hide Only│…
         │› typing here                          │──────────────────────────────────────
         │───────────────────────────            │m mark · space exclude · / find · …
         │Connected since 19:41                  │──────────────────────────────────────
                                                 │3 selected
```

From the top of the right pane down:

| Row | Normal mode | Log mode |
| --- | --- | --- |
| Top bar | name · ` Log ` | name · find status · ` Filter ` · ` Log ` (lit) |
| Rule | `rule.status` | `rule.status` |
| Body | scrollback | log lines |
| Rule | `rule.input` | `rule` (as now) |
| Input area | input box, or a form | hint row, or a prompt |
| Rule | `rule.status` | `rule.status` |
| Bottom bar | connection, or a message | `N selected`, or a message |

Normal mode's scrollback loses two rows to the top bar and its rule.
Log mode's body loses one, to the hint row's new bottom rule.

## The top bar

- Painted with `status`, like the bottom bar.
- Left: `world/Name` (as the statusline shows it today). In log mode,
  the find status follows (`find: Mira 1/1`, as the log header shows it
  now), in `status`.
- Right, pinned: in log mode ` Filter `, then always ` Log ` at the far
  right, so Log never moves when toggling.
- ` Log ` is `status.log`, lit (`status.log.on`) in log mode. Click it to
  open or close log mode.
- ` Filter ` is the new `status.filter`, lit (`status.filter.on`)
  **while the filter panel is open**. Click it (or `f`) to open or close
  the panel. It does not light for filters that are set while the panel
  is closed.
- A chip is clickable only when it's drawn whole. When space runs out,
  the name and find status are cut first, with `…`.
- With no active character the bar is painted and empty.
- Gone from log mode: the `LOG Kit` title and the date range
  (`Thu Sep 24 → today`). The empty log body says "no logs yet"
  (`browse.no_logs`, styled `log.loading` like the body's other notice)
  instead.

## The bottom bar

Left-aligned, nothing on the right: no clock, no version.

- **Normal mode:** the connection state.
  - Connected: `Connected since 19:41`, local 24-hour time of when the
    current connection came up (`charState.connectedAt`). For a
    connection that came up before today, the day goes first in the
    log's day format (`str.DateDay`): `Connected since Mon Sep 28 19:41`.
    The text changes only when the connection changes, and once at
    midnight.
  - Otherwise the state as now: connecting, disconnected, failed.
- **Log mode:** `N selected`.
- A **status message** takes the bar's place while it shows, in either
  mode, as normal mode's messages do now. Log mode's own messages
  ("copied 3 lines", "2 lines in range", errors), which today replace the
  hints in the action bar, move here; errors paint in `status.error` like
  any other.
- With no active character, the bar shows messages only.

## Log mode's hint row

It becomes the input-area equivalent: a rule above (as now) and a
`rule.status` below it, before the bottom bar.

It shows the key hints, or a prompt while one is open (`find:`,
`export as …`, `filter text:`, go to date, save as). It never shows
status messages any more.

## The filter panel

No "Filter" title or the blank row under it. The list starts at the top
of the sidebar with Untagged.

## The version

Gone from the screen. `kiln version` and `kiln --version` print
`version.String()`. The README mentions them.

## Theme roles

| Role | Change |
| --- | --- |
| `status` | now paints the top bar too |
| `rule.status` | now also under the top bar and under log mode's hint row |
| `status.log`, `status.log.on` | the Log chip, now in the top bar |
| `status.filter`, `status.filter.on` | **new**, the Filter chip; the built-in styles them like `status.log` and `.on` |
| `status.clock` | **removed** |
| `log.header`, `log.header.title`, `log.header.chip`, `log.header.chip.on` | **removed** |
| `log.bar.error` | **removed** (log mode's errors now show in the bottom bar as `status.error`) |

A theme naming a removed role fails to load like any unknown role
(`theme.unknown_role`); no alias, no warning.

`roles.go` (and `Roles`), `default.toml` and the README's role list
change to match.

## Strings

- New: `view.connected_since` (`Connected since {time}`),
  `view.connected_since_day` (`Connected since {day} {time}`). `kiln
  version` prints the bare version string, like `version.String()`
  returns it, so it needs no catalog entry beyond a `//str:ok`.
- Removed: `browse.to_today`, `browse.log` (the `LOG` title), `filter.title` (the panel title; the chip
  label moves to a new `view.filter_button`, like `view.log_button`).
- `browse.no_logs` moves from the header to the empty body.

Commits list the keys in a `Strings:` trailer.

## Testing

- The layout in both modes: row order, the top bar's content, chips
  pinned right with Log at the far right, and the bottom bar's text in
  each state.
- `Connected since` today and on an earlier day, driven by the test
  clock.
- Clicks on ` Log ` and ` Filter ` toggle; a chip cut off on a narrow
  screen isn't clickable.
- Log-mode messages land in the bottom bar, and the hint row keeps its
  hints.
- Nothing in the view depends on the minute: the same state drawn at two
  times a minute apart is the same screen (with a connection from today).
- A theme naming `status.clock`, `log.header` or `log.bar.error` fails with
  `unknown_role`.
- `kiln version` / `--version`.
- Golden screens: every one changes, on purpose.
