# Live range marking in log mode

Issue #191.

## Why

Marking a range in log mode works from the keyboard (`m`, page, `m`)
but not with the mouse: a plain click starts a new one-line range, so
click, `m`, click throws the start away. Shift-click extends a range,
but many terminals keep Shift+mouse for their own text selection, and
Blink on iOS has no way to send it. Marking has to work with plain
clicks and keys alone.

## Behavior

`m` starts a **live range** on the cursor's line: that line is the
anchor, and the range's other end follows the cursor however it moves
(`↑`/`↓`, `j`/`k`, `PgUp`/`PgDn`, `Home`/`End`, `g`, `n`/`N`, a click,
the wheel dragging the cursor along at the edge). Moving above the
anchor puts the range above it; the anchor never moves.

While the range is live:

- The statusline shows it as it shows find: a `marking` part first,
  then the cursor line's tags, then the selected count, which changes
  as the cursor moves.
- `m` locks the range where it is.
- `c` and `e` lock it and act on it.
- `Esc` drops the range and stays in log mode, as `Esc` turns find off
  without leaving. With both on, `Esc` ends marking first.
- `Space` leaves the cursor's line out, as now. When the range shrinks
  past an excluded line, that exclusion is forgotten, so growing back
  over the line doesn't bring it back excluded.
- A click is a sticky shift-click (changed after trying it; see #191).
  Inside the range it takes its line out, or puts it back, and leaves
  the cursor (the range's end) where it is. Outside the range it grows
  the range to take the line in: the end away from the click becomes
  the anchor, and the cursor goes to the clicked line, so the keys move
  on from there. Shift and the gutter change nothing while marking.
  With the mouse alone a live range only grows; the keys or `Esc`
  shrink it.

Once locked, nothing changes from today: a click starts a new one-line
range, shift-click extends the range or toggles a line in it, a gutter
click toggles its line, and `m` starts a new live range on the cursor.

The live range looks like a locked one: the same selected and excluded
styles. Only the statusline tells them apart.

## Core

`app.Log` gains the anchor and a `Marking` flag. The cursor stays the
front end's (the TUI's `browse`, the web's `logView`), so the core
doesn't follow it; the front end tells it:

- `Mark(l)` with no live range: anchor, `Start` and `End` are `l`,
  exclusions cleared, `Marking` on. With a live range: `MarkTo(l)`,
  then `Marking` off. Front ends pass the cursor, so the range locks
  where it is.
- `MarkTo(l)`, while marking: `Start`/`End` become anchor and `l` in
  order, and exclusions outside them are dropped.
- `GrowTo(l)`, while marking: the anchor becomes the end away from
  `l`, then `MarkTo(l)`.
- `EndMark()`: drops the range (`Start`, `End` nil, exclusions cleared,
  `Marking` off).
- `Copy` and `ExportReady` turn `Marking` off before they act.
- `NewRange` and `ExtendTo` turn it off too, so no other path leaves a
  stale live range.

A pending start with no end (`End == nil`) no longer exists: `Mark`
always makes a whole range. `InRange`, `Selection` and `RangeStatus`
lose their `End == nil` cases.

## TUI

`browse.SetCursor` and `MoveCursor`, and every other place the cursor
moves, call `MarkTo` while marking. One hook in `SetCursor` covers them
if every move goes through it; the plan checks that. `click` checks
`Marking` first: inside the range `ToggleExclude`, outside `GrowTo`
then `SetCursor`. `Esc` in `browseKey` ends marking before it ends find
or leaves log mode.

## Web

The web gets `m`, which it doesn't have today (log.ts left marking to
the mouse). The TUI's keys copy over:

- `m` sends `mark` with the cursor's seq.
- While `Marking` (in the log state the page gets), each cursor move,
  from keys, a click or `dragCursor` as the browser scrolls, sends a new
  `mark_to` action with the seq. `Esc` sends a new `end_mark`.
- `logClick` checks `Marking` first, as the TUI's `click`: inside the
  range it sends `exclude`; outside, a new `grow` action, then moves
  the cursor.
- The statusline's marking part is the TUI's string.

## Strings

- `browse.marking`, the statusline part: `marking`. New.
- `browse.range_start` (`BrowseRangeStart`, the message `m` gives now)
  is reworded to say how to finish: `marking: move to the other end, m
  to finish, Esc to drop`.

## Docs

`docs/log-mode.md`: the `m` and click rows, and `Esc`.

## Testing

- Core: `Mark`, `MarkTo` above and below the anchor, exclusions dropped
  on shrink, `Esc`/`EndMark`, `c`/`e` locking.
- TUI: `m`, PgDn twice, `m` covers the lines between; `m`, click, `m`
  covers anchor to the clicked line; shift-click and gutter click while
  marking only move the end; `Esc` with find on ends marking only.
- Web: `logKey`'s `m`, `logClick` while marking, and the smoke test's
  click, `m`, click.
