# Web-native Kiln, part 3: the web UI (desktop)

Epic #125 (web-native Kiln), sub-project 3 of 6. Part 1 moved everything
Kiln decides into `internal/app`; part 2 ran it in the browser behind a
JSON API and a bare test page. This part builds the real page: Kiln's
TUI, drawn with the DOM, for desktop browsers. The phone layout is
part 4.

## In short

- **It's the TUI, in a browser.** Same layout, same widths (counted in
  characters), same themes and colors, same dimming, same things are
  buttons. Not pixel-for-pixel, but anyone who knows one knows the
  other, and the two are kept in sync from here on. Where this spec
  doesn't say otherwise, the web does what the TUI does.
- **The browser does what browsers are good at.** Typing, selection,
  copy, scrolling, focus and form controls are the browser's own,
  styled to look like the terminal.
- **Go handles data; the page handles looks.** Go decides what every
  piece of text is (its server colors, its tags, whether it's a link,
  how a date reads). The page decides what that looks like, from the
  theme, in CSS.
- **Built for everyone.** Real controls, landmarks, a live region that
  reads incoming text aloud, and focus that goes where you'd expect.

## Done means

- The page opens in a desktop browser and shows the five areas: top
  bar, sidebar, scrollback, bottom pane, status line. A newcomer can
  connect a character (the password form works with a password
  manager), read and send, switch characters, open the picker, edit a
  world or character, and use log mode (select, find, filter, export).
- It looks like the TUI in the built-in theme, light and dark, with
  Catppuccin Macchiato (dark) and Latte (light) as the terminal's
  sixteen colors.
- It works with only a keyboard and with VoiceOver; incoming text in
  the active character is read aloud.
- The TUI's golden screens are unchanged. The web has screenshot
  goldens of its own, made from the same scenarios.
- The #141 items marked for the page are done.

## For you to check

These are the decisions that change what someone sees or does.
Everything else is plumbing, and you can skip it.

1. **Tab moves focus (web only).** In the TUI, Tab and Shift+Tab jump
   to the next and previous unread character. On the web, Tab has to
   move between controls, or the page fails keyboard and screen reader
   users. Alt+↓ and Alt+↑ take over "next/previous unread".
2. **Enter submits a form (web only).** In the TUI, Enter moves to the
   next field until it's on a button. On the web, Enter in a field
   saves, as browsers do, and Tab moves between fields. Screen reader
   users expect this, and password managers' "submit after fill" sends
   Enter. A too-early Enter is harmless: the save is checked and focus
   goes to the first field with an error. The hint line says so, in a
   web string of its own.
3. **Keys the browser won't give up (web only).**
   - Ctrl+T (the editor) is the browser's new tab on Windows and
     Linux, and a page can't take it. Mac keeps Ctrl+T; Alt+T works
     everywhere.
   - Ctrl+C and Ctrl+D don't go back or quit: Ctrl+C is copy on
     Windows and Linux, and quitting means little in a tab. Esc goes
     back.
   - The input's own editing keys (Ctrl+A/E/K/U/W, Alt+B/F) are the
     browser's, not Kiln's.
   - ↑/↓ recall history only from the input's first or last row, so
     they still move through a multi-line draft.
   - Log mode's single letters (`/ n N g e c t f`) work only while
     the log has focus, so they don't fight a screen reader's own
     letter keys.
   - Log mode's cursor keys (`j k m space`) aren't on the web: you
     select lines with the mouse or keyboard as on any page.
4. **The tab's title shows unread lines (web only):** `(3) Kiln`.
5. **Notification text comes from Go already cleaned (TUI and web).**
   This reverses a part 1 ruling (step 3), which left `Notify.Body` raw
   for each front end to clean. Nothing you see changes in the TUI.
6. **Live field hints come from Go (TUI and web).** This reverses part
   2's "live hints are each front end's". The web asks Go whether a
   value is fine as you type (`check(field, value)`), so the rules
   live in one place. The TUI keeps refusing keystrokes as it does now.
7. **The terminal's sixteen colors on the web are Catppuccin.** Dark is
   Macchiato, light is Latte. A theme can't change them yet (that would
   mean a new table in the theme format, which the TUI would have to
   ignore). See "Not in this part".
8. **The page has an npm build.** `web/ui/package.json` pins Preact,
   esbuild and TypeScript; `npm ci` installs them. No global installs.

## The page

### Layout

Five areas, as in the TUI:

- **Top bar** and **status line**: one row each, fixed.
- **Sidebar**: 16 characters wide (`16ch`), as in the TUI, not
  resizable. In log mode it's the filter panel; with the picker open,
  the picker.
- **Scrollback**: the rest of the width.
- **Bottom pane**: under the scrollback, the same width. It holds the
  input, or the picker's filter, an editor form, the password form, or
  log mode's hint row. It's as tall as what it holds, pushing the
  scrollback up, and the scrollback dims while a form is open (the
  TUI's dim roles).

The sidebar, scrollback and bottom pane each scroll on their own. Rules
between areas are single lines in the theme's divider roles, drawn
with borders. A CSS grid holds the five, with named areas so part 4
can rearrange them.

Sizes are in `ch` and `rem`, so the browser's zoom (Cmd±) scales
everything. There's no font size setting.

### Font

JetBrains Mono, bundled (WOFF2, self-hosted, its OFL license file next
to it). Every glyph Kiln draws must be in it and one cell wide (the
UI characters rubric).

### Terminal colors

The terminal's sixteen and its default colors, from Catppuccin's own
terminal ports
([alacritty](https://github.com/catppuccin/alacritty)). Normal and
bright are the same except black and white.

| | Macchiato (dark) | Latte (light) |
|---|---|---|
| default fg / bg | `#cad3f5` / `#24273a` | `#4c4f69` / `#eff1f5` |
| black / bright | `#494d64` / `#5b6078` | `#bcc0cc` / `#acb0be` |
| red | `#ed8796` | `#d20f39` |
| green | `#a6da95` | `#40a02b` |
| yellow | `#eed49f` | `#df8e1d` |
| blue | `#8aadf4` | `#1e66f5` |
| magenta | `#f5bde6` | `#ea76cb` |
| cyan | `#8bd5ca` | `#179299` |
| white / bright | `#b8c0e0` / `#a5adcb` | `#5c5f77` / `#6c6f85` |

### Scrollback lines

- Each line is a block with `white-space: pre-wrap`,
  `overflow-wrap: anywhere` and `tab-size: 8`. Wrapped rows hang one
  character in (`padding-left: 1ch; text-indent: -1ch`), like the
  TUI's `wrapIndent`.
- A line is a run of `<span>`s rendered from Go's spans (below), with
  nothing between them. Spaces at span edges survive, because
  `pre-wrap` never collapses them. A test checks that every rendered
  line's `textContent` equals the line's plain text.
- Kiln's glyphs (`›` for an echo, `* ` for a sys line, `── ` around a
  day) are drawn by CSS `::before`/`::after`, so copying a line copies
  only its text.
- Links are `<a>`s. Clicking one opens it in a new tab.
- Every line stays in the DOM, up to the scrollback's existing limit.
  No virtual scrolling: it would break the browser's find and
  selection past the screen.
- Lines are keyed by their number (`Seq`) and epoch, so appends and
  prepends only add elements. Loading older history keeps your place
  (`overflow-anchor`), and new lines keep you at the bottom only if
  you were there.
- PgUp/PgDn in the input scroll the scrollback, as in the TUI.

### Controls

Each TUI field becomes a real control, restyled:

| TUI | Web | How |
|---|---|---|
| Text field | `<input>` | no border, role colors, `caret-color` |
| Toggle `[x]` | checkbox | `appearance: none`; `[x]`/`[ ]` in `::before` |
| Choice `◄ v ►` | radio group | only the checked one shows; `◄ ►` while focused; arrows cycle natively |
| Button chip | `<button>` | the `FormButton` roles |
| `► More settings` | `<details>`/`<summary>` | the TUI's `►`/`▼` |

No `<select>`: its open list is drawn by the OS. Focus is drawn by
`:focus-visible` in the TUI's focused-field style. Scrollbars are thin
and themed (`scrollbar-color`; Safari before 26.2 gets
`::-webkit-scrollbar`). Chrome's autofill background is covered with
an inset `box-shadow`.

Errors show next to their field (`FieldError`) and are tied to it with
`aria-describedby`; an error about no one field shows at the top of
the form.

### Keys

| Key | Does |
|---|---|
| Ctrl+↑/↓ | switch character |
| Alt+↓/↑ | next/previous unread |
| Ctrl+O | the picker |
| Ctrl+L | log mode (opens and closes it) |
| Ctrl+T, Alt+T | the editor |
| Esc | back: close what's open, return focus |
| Enter / Shift+Enter | send / new line |
| ↑/↓ (first/last row of the input) | history |
| PgUp/PgDn | scroll the scrollback |
| `/ n N g e c t f` (log focused) | log mode's keys |

Ctrl+O and Ctrl+L belong to the browser on Windows and Linux too, but
a page can take them. Whatever the key, the mouse does the same thing
it does in the TUI.

### Accessibility

- Sidebar is a `<nav>` with a list; each character's name carries its
  unread count ("Rook, 3 unread").
- Scrollback is `<main>` with `role="log"`: new lines in the active
  character are read politely. Readers that want less use their screen
  reader's own controls.
- A character in the background says nothing, except a line Kiln would
  notify about, which goes to a polite status region ("Rook: Mira pages:
  you around?").
- Log mode has no live region: it's history.
- The status line is `role="status"`; errors in it are `role="alert"`.
- The top bar is a `<header>` with real buttons (`Log`; later, `≡`).
- Opening a form or the picker moves focus into it; Esc puts focus back.
  The dimmed scrollback stays readable (not `inert`).
- Native elements first; ARIA only where none fits.

### States around the main screen

- **Loading.** The five areas draw at once, from the page's own
  strings; the status line counts the download (`loading Kiln… 2.1/6.4
  MB`). No spinner.
- **Another tab has Kiln.** Today's message and `Use here`, in the
  scrollback.
- **Kiln stopped** (after `/quit` or a start-up error, #141). The
  status line says so in its error role, the controls are disabled, and
  a `Reload` button shows.
- **Not saving** (no IndexedDB). A warning that stays in the status
  line.
- **Password.** Where the TUI's prompt is, as a real `<form>`: a hidden
  username `char@world` (e.g. `Kit@PFMuck`) and a password field with
  `autocomplete="current-password"`, so password managers tell worlds
  apart. Kiln never stores the password.
- **Backup and restore.** `/backup` and `/restore`, as today: a
  download, and a file picker.

### Window size

The page reports the scrollback's width in columns (its width over
`1ch`) and its height in rows to the MUCK, 200ms after the last resize,
as the TUI does (#141).

## What changes in the API

Part 2's API sends some things raw that the page would have to finish,
and one thing styled that the page should style. Each change keeps Go
on data and the page on looks.

**Marked spans.** A scrollback line, a log-mode line and the server's
prompt (`Input.Prompt`) go out as spans instead of text with the
server's escape codes (SGR, the ANSI codes for color and bold) in it.
A span is its text and its marks: the server's colors (an ANSI index
or a hex color), its flags (bold, faint, italic, underline, reverse),
the tags covering it, and `link` with the URL. Go splits lines with the
same code the TUI paints with: the escape-code parser the HTML export
uses (`ansi.SpanStyle`) and the tag runs (`hl.Runs`). Each span also
carries the plain text, and each line its plain text, for the
`textContent` test.

**Links move into the core.** Finding links (the pattern and
`trimURL`, now in `internal/ui/sbmouse.go`) moves into `internal/app`,
and the TUI uses it from there.

**A `Theme` event.** The theme as data: every role's style, every
tag's style (with `scope`) in the order that decides which wins, and
the palette, for light and dark. Sent at start and when the theme or
its file changes. No lines are resent.

**Display text for dates and times.** Lines, log mode and characters
get the text the TUI shows (`Thu Sep 24`, `21:06`, `Connected since
21:14`) beside the raw values, formatted with the catalog's layouts.
The raw values stay for logic.

**Clean notification text.** `Notify.Body` goes out with escape codes
and control characters removed.

**`check(field, value)`.** A second synchronous call beside
`overLimit`: whether a settings value would be accepted, and the
message if not. It's the core's save-time check for one field.

**The page's strings and glyphs.** `internal/str/gen` writes a typed
TypeScript module (`web/ui/src/str.gen.ts`) instead of `strings.json`:
a function per entry in the `[web]` table, with plurals through
`Intl.PluralRules`. Glyphs both front ends draw (`›`, `* `, `──`,
`►`, `▼`, `•`, `×`, `≡`) come from Go constants into the same module.
The staleness test covers it.

## The page's code

- **Where:** `web/ui/` (TypeScript and TSX, `package.json`, a lockfile).
  esbuild bundles it into `web/static/`; `tsc --noEmit` type-checks.
  `web/build.sh` runs both after the wasm build.
- **Preact.** Keyed lists come built in, and there's no shadow DOM to
  get between CSS variables or `aria-labelledby`.
- **One component per TUI part:** `TopBar`, `Sidebar`, `Picker`,
  `FilterPanel`, `Scrollback`, `BottomPane` (`Input`, `Form`,
  `PasswordForm`, `LogHints`), `StatusLine`. A change in one of
  `internal/ui`'s files has an obvious twin.
- **One store.** It applies each turn's batch of events as one update
  (#141), and drops everything on `Start`. Components read from it;
  actions go to Go through the one queue.
- **Styles.** `theme.css` turns the `Theme` event into CSS variables on
  `:root` (`--role-scrollback-echo-fg`, …), one rule per tag
  (`.tag-page`), and the sixteen colors (`--ansi-0`…`--ansi-15`, plus
  default fg and bg). Light and dark sit under `prefers-color-scheme`,
  so switching needs no round trip. Tag rules come after server-color
  rules, in the theme's order, so the cascade layers them per property
  as the TUI does. `reverse` swaps through variables.
- **#141 items** live in the store and `Input`: flush pending input
  before `switch`; Enter with nothing open opens the picker; keep
  typing across start-up and a switch in flight (don't overwrite the
  line on `Active` while newer input is unsent); say when Kiln stops;
  debounce resize. The JSON items (bare ints for `Kind`/`Op`, unknown
  fields, `omitzero` on `ConnectedAt`) are settled in step 1.

## Order of work

Each step is one PR.

1. **API data.** Spans, links into the core, the `Theme` event,
   display text, clean `Notify`, `check`, the TS strings and glyphs
   module, and #141's JSON items. Native Go; the TUI's goldens don't
   move.
2. **Page scaffold.** `web/ui`, the build, the store, the five areas,
   the font, theme CSS, and the states around the main screen. The
   test page goes.
3. **The main screen.** Sidebar, scrollback, input and password form,
   top bar and status line, the keys, live regions.
4. **The bottom pane's forms.** The picker, the editor, dimming.
5. **Log mode.** The filter panel, selection by line, find, export,
   the hint row.
6. **Screenshot goldens.** The shared scenarios, and the web's
   goldens.

## Testing

- **Go:** spans for lines with server colors, tags, overlapping tags
  and links, against what the TUI paints; the `Theme` event's JSON;
  `check` for each field rule; the TS module is fresh. The TUI's goldens
  are never `-update`d.
- **Page:** `node --test` for the store (batches, `Start`, epochs,
  appends and prepends, typing kept across a switch) and for
  components, rendered to a DOM. Every rendered line's `textContent`
  equals its plain text.
- **Screenshots:** the TUI's golden scenarios (main, input, picker,
  editor, editor error, password, log, log filter, scrolled) are built once
  and drive both front ends. The TUI checks its text goldens; headless
  Chrome renders the page and checks a screenshot per scenario, light
  and dark. `-update` only when a change to the look is intended.
- **Smoke:** the headless Chrome smoke test drives the real page
  through the relay to the fake MUCK.
- **By hand:** VoiceOver in Safari, keyboard only.

## Not in this part

- The phone layout and the `≡` menu (part 4), the detachable relay (5),
  the launch (6).
- A theme setting for the terminal's sixteen colors.
- Editing themes in the browser: a custom theme gets in by `/restore`.
- Virtual scrolling.
- Any change to the TUI's look or keys.
