# Log filter panel

## Goal

Log mode's tag filter is a row of numbered chips that each cycle through
three states: neutral, only, hide. The cycle hides its own state machine:
to hide a tag you pass through "only", and the `+`/`-` glyphs don't say
what the next press does. It also ignores nesting: hiding `page` leaves
lines tagged only `page/in` alone, although themes taught that `page/in`
inherits from `page`.

The chips give way to a **Filter panel** in the sidebar. Every tag, and
any text you want to filter on, gets a row with two spelled-out buttons,
**Hide** and **Only**. Tags nest under their parents and can be
collapsed.

## The model

Filters live in `internal/scene`, replacing `Chip`, `Next` and today's
`Visible`.

### Items

An **item** is a tag or a text row.

- **Tags** are every tag on a loaded line, plus every parent up their
  slashes, even when no line carries the parent itself: lines tagged
  `page/in` and `page/out` give a `page` item too.
- **Text rows** are terms you add. A line matches one when its text
  contains the term, literally and ignoring case, as `/` find matches
  (`findRE`).

### State

Each item has a **Hide** flag, on or off. At most one item has **Only**.

Nesting works the way Only does, through real flags rather than
inherited looks:

- **Hiding a parent** sets Hide on all its descendants too. Their
  buttons light, since they really are hidden.
- **Unhiding a child** of a hidden parent clears the parent's Hide (and
  any hidden ancestor's). The child's siblings keep theirs, so "hide
  `page`, then unhide `page/in`" leaves `page/out` hidden and `page/in`
  shown.
- A tag that first arrives under a hidden parent is hidden: a tag is
  hidden when it or any ancestor has Hide. This is the only place
  ancestry is looked up, and it never shows on a button.

### Visibility

A line is **hidden** when either holds:

1. Any of its tags, or an ancestor of one, has Hide, or it matches a
   hidden text row.
2. An item has Only, and the line doesn't carry that tag (or a tag under
   it) or match that text row.

Hide beats Only: a line that carries the Only tag and a hidden tag is
hidden. A line matching an Only text row is still hidden by a hidden tag.

Rule 2 is today's Only rule, so untagged lines, and tags that first
arrive while an Only is set, are hidden without any special case.

### Buttons

- **Hide** toggles the item, as above.
- **Only** on an item: clears any other item's Only, gives this one Only,
  clears its Hide, and sets Hide on every other item. The panel then
  shows exactly what is hidden.
  - On a nested tag, "every other item" leaves out its ancestors and
    descendants: hiding `page` would hide `page/in` with it. Only on
    `page/in` hides `page/out` and the rest; `page` stays shown.
- **Only on the item that has it** clears the Only and every Hide: a
  clean slate.
- **Hide on the item that has Only** flips it: clears the Only and every
  other Hide, then hides just that item (and, as for any hidden parent,
  its descendants). Everything else shows again.
- **Unhiding any item** while an Only is set clears the Only. The Hide
  flags it set stay, as plain hides.

Buttons show only the item's own state. There are no implicit or
inherited styles.

## The panel

### Opening it

Log mode's header shrinks to one row. A ` Filter ` chip sits at its
right end, styled `log.header.chip`, and `log.header.chip.on` whenever
any item is hidden or has Only. Clicking it, or `f`, opens the panel in
the sidebar, as `Ctrl+O` opens the picker; clicking or `f` again, or
`Esc`, closes it.

The log body stays in view and updates live as buttons change. The mouse
still scrolls and selects there.

### Layout

Each item takes three rows: its name, its buttons, and a blank row.

```
 Filter

 ▼ page
    Hide Only
   in
    Hide Only
   out
    Hide Only

 self
 Hide Only

 "lighthouse"
 Hide Only

 + Text
```

- Tags come first, alphabetically, children indented one step under
  their parent. Text rows follow in the order added, then `+ Text`.
- A lit button is its item's own state (`filter.button.on`); an unlit one
  is `filter.button`.
- A parent has `▼` when open and `►` when collapsed (both WGL4, one
  cell). A collapsed parent with Hide or Only set anywhere under it shows
  `•` after its name, so hidden overrides are never invisible.
- Everything starts expanded.
- Names too long for the sidebar are cut with `…`, as sidebar names are.
- When the rows overflow, the panel scrolls with `▲ N more` / `▼ N more`
  rows, as the sidebar does.

### Keys

While the panel is open it has the keyboard:

| Key | Does |
| --- | --- |
| `↑` / `↓` | Move between items |
| `h` | Hide |
| `o` | Only |
| `→` / `←` | Expand / collapse a parent |
| `x` / `Delete` | Remove a text row |
| `Enter` on `+ Text` | Ask for a term in the action bar |
| `Esc` / `f` | Close the panel |

Clicks: a button presses it, `►`/`▼` toggles, a name highlights the item,
`+ Text` asks for a term.

### Adding text

`+ Text` opens an action-bar prompt, like `/` find. Enter adds the term
as a text row with **Only** (with Only's usual effect on the others): a
term is usually typed to read just the lines about it. An empty term or
one already listed adds nothing. Removing a text row that had Only clears
the Only, leaving the Hide flags it set.

### Lifetime

Filter state (flags, Only, text rows, collapsed parents) lives on the
character, not the log-mode session. Leave log mode and come back and it
is all still there; quitting Kiln clears it. Nothing is saved to disk.

## What else changes

- Header row 2 (`tags: 1[…] 2[…]`) and the `1`–`9` keys go away; the log
  body gains a row (`browseBodyH` is `height-5`).
- The hints line says `f filter` in place of `1-9 filter`.
- `scene.Chip`, `Chip.Next`, `glyphChipOnly`, `glyphChipHide`,
  `chipSpans`/`chipSpan` and the `browse.tags` string go.
- Export, copy and the visible-line walk use the new visibility rule, so
  they follow the filters as they follow the chips today.
- New theme roles: `filter` (area, in place of `sidebar` while the panel
  is open), `filter.item`, `filter.selected`, `filter.button`,
  `filter.button.on`, `filter.add`. Each goes in `roles.go` (and
  `Roles`), `default.toml` and the README's role list.
- New catalog strings for the panel title, the buttons, `+ Text`, the
  prompt and the hints, per the CLAUDE.md string rules.

## Testing

- `scene`: table tests for visibility: a hidden ancestor hides a tag,
  Hide beating Only, Only on a parent and on a child, text
  rows hiding and Only, untagged lines under Only, synthesized parents.
- `ui`:
  - The panel opens and closes by `f`, the chip and `Esc`.
  - Hiding a parent lights its descendants; unhiding one clears the
    parent and leaves its siblings hidden.
  - Only lights Hide on the others (not ancestors or descendants);
    unhiding one clears the Only; Only on a second item moves it; Only on
    the item that has it clears everything; Hide on it clears the rest and
    hides just it.
  - Collapse and expand by key and click; `•` on a collapsed parent with
    state under it.
  - Adding text (starts with Only), duplicates and empty terms ignored,
    removing a text row.
  - Filters survive leaving log mode and coming back.
  - Clicks on buttons, disclosure glyphs and `+ Text`.
- Golden screens: log mode's one-row header, and a new one with the panel
  open. Updated on purpose.

## Later

- A search field at the top of the panel, to filter the tag list, if
  lists grow long.
