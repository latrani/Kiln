# Responsive mode: a sidebar that gets out of the way

Issue #204. It takes the phone part of #125 (part 4) and does it for
the TUI too: one behavior for both front ends, driven by width.

## Why

The sidebar costs a fifth of the screen whether you need it or not.
On a wide screen you sometimes want that room back. On a narrow one
(a phone, a split terminal) it can't fit beside the main view at all:
below about 64 columns it's squeezed to its minimum, and so is
everything else.

## Two modes, by width

- **Wide** (64 columns or more; on the web, 64ch): the sidebar sits in
  its column as now, and can be **collapsed**.
- **Narrow** (under 64): there's no sidebar column. The sidebar is a
  toggle: closed, the main view has the whole width; open, the sidebar
  takes the top bar and scrollback area. The input area and status
  line stay, so the `Ctrl+O` filter and the editor's forms still have
  a place.

The TUI counts columns; the web counts `ch`. At the browser's default
16px that's about 614px (if JetBrains Mono's `ch` is 0.6em; not yet
measured), so phones in portrait (402–440px) are narrow, and phones in
landscape (874–956px) and tablets are wide. Zooming text up can tip a
screen into narrow, which is what you'd want. Resizing across the
breakpoint switches modes.

## Collapsing, and the ≡

Every sidebar list has a title row (`Connections`, `All connections`,
`Filters`). At its right end sits `‹` (U+2039): a click collapses the
sidebar (wide) or closes it (narrow).

Collapsed or closed, `≡` (`glyph.Menu`) is the first thing in the top
bar, at its left. A click opens the sidebar again.

**The badge.** After the world sidebar's `≡` (not the filter panel's),
a space, then the sum of the character rows' activity: `●` if any row
shows one (a priority line unread), then the total of the rows' unread
counts, in the same roles as the rows (`sidebar.priority`,
`sidebar.unread`): `≡ ●12`, or `≡ 12` with nothing a priority, or just
`≡` with nothing unread. The name after it shifts right as the badge
grows.

**Ctrl+B** toggles the sidebar in both modes, in both front ends, as
`≡` and `‹` do.

## In log mode

The filter panel is the sidebar, so it collapses and opens the same
way, with no badge on its `≡`.

- **Wide, collapsed:** `f` still moves focus. While the panel has
  focus it shows, so you never type into a panel you can't see; when
  focus goes back to the log it collapses again. Collapsing is what you
  chose, so it stays the saved state.
- **Narrow:** `f` opens the panel with focus in it, or closes it with
  focus back on the log, so it does what it does in wide mode, plus
  the opening. `Ctrl+B`, `≡` and `‹` do the same as `f`.

## The picker and choosing

- `Ctrl+O` (and `/open`) opens the sidebar for the picker, in either
  mode. In wide mode, collapsed, it shows only while the picker is up
  and collapses again after: collapsing is what you chose.
- **Narrow:** picking a character closes the sidebar, as does picking a
  world's overview. Escaping the picker closes it too.

## What's saved

Only wide mode's collapsed-or-not is saved: one setting for both the
world sidebar and the filter panel. It lives in a state file,
`$XDG_DATA_HOME/kiln/state.json` (Kiln's UI state, not config: Kiln
never rewrites `config.toml`). On the web that's the same file in the
page's own storage (memfs, kept in IndexedDB), so it's per browser.
Narrow mode always starts closed and saves nothing.

## Core

`internal/app` holds the setting and the counting, so both front ends
agree:

- `App.SidebarCollapsed() bool` and `SetSidebarCollapsed(bool)`. The
  latter writes `state.json` through a new `Deps.SaveState` (the TUI
  and the web each give it their file system), and is read at start
  through `Deps.LoadState`. A missing or unreadable file means "not
  collapsed", and the statusline says why if it couldn't be read.
- `App.Activity() (unread int, priority bool)`: the open characters'
  unread counts summed, and whether any has a priority line among them,
  for the badge.
- Whether the screen is narrow, and whether the narrow sidebar is
  open, are each front end's. The core doesn't know the width.

## TUI

- `layout()` gives the sidebar width 0 while collapsed (wide) or
  always (narrow). The top bar starts with `≡` and the badge then.
- Narrow and open, the sidebar draws across the right pane's top bar
  and scrollback rows, full width. The input area and status line stay.
- Clicks: `‹` in the title row, and `≡` in the top bar.
- `Ctrl+B` in `model.go`'s global keys, beside `Ctrl+O`. It's free in
  the input box (word movement is `Alt+B`).
- New strings: none for the glyphs. The badge's count is a number, and
  its meaning goes in a screen-reader label on the web only.
- New roles: `status.menu` for `≡` and `sidebar.collapse` for `‹`. The
  badge uses the rows' `sidebar.priority` and `sidebar.unread`.
  They go in `roles.go`, `default.toml` and `docs/themes.md`.

## Web

- A `(min-width: 64ch)` container check on the page decides the mode:
  CSS for the layout, with the page reading the same query for the
  behavior.
- `≡` is a `<button aria-expanded>` labelled "Sidebar", with the badge
  inside its accessible name ("Sidebar, 12 unread, priority"). `‹` is a
  button labelled "Collapse sidebar". Both labels are new catalog
  strings.
- **Narrow and open:** the sidebar is a region that replaces the top bar
  and scrollback. Focus moves into it, and back to the input when it
  closes.
- `Ctrl+B` joins the page's global keys. Browsers don't reserve it.

## Docs

`docs/using.md` gets `Ctrl+B` and the two modes. `docs/log-mode.md`
gets the panel's collapse and narrow behavior.

## Not in this

- A setting for the breakpoint.
- Swipe gestures on phones.
- The rest of #125's part 4 (a phone-shaped input area, the on-screen
  keyboard's effect on height). Those come with phone testing.
