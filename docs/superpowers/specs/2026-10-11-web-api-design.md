# Web-native Kiln, part 2: the web API

Epic #125 (web-native Kiln), sub-project 2 of 6. Part 1 moved everything
Kiln decides into `internal/app`. This part runs that core in the
browser (as wasm: Go compiled to run inside a web page) and gives the
page a way to talk to it: send it what you did, and hear back what
changed. There's no real web UI yet; that's part 3.

## In short

- **The page tells Kiln what you did; Kiln tells the page what
  changed.** Everything you do (send a line, switch characters, hide a
  tag) goes into one queue, handled in order. After each one, Kiln
  works out which parts of the screen changed and sends the page just
  those.
- **The browser does what browsers are good at.** Typing, the caret,
  selecting text, scrolling and live form hints are the page's job.
  Kiln keeps the decisions: what gets sent, what's unread, what a
  filter hides, whether a setting is valid.
- **The core gets a little cleanup first.** A few pieces that are
  really about driving a terminal with a keyboard (the log viewer's
  cursor, the filter panel's highlight) move back into the TUI. Settings
  errors name the field they're about.
- **The old xterm web page goes away.** Nobody uses it, and nothing
  in part 2 needs it. A bare test page replaces it, enough to smoke-test
  the API.
- **The TUI looks the same**, and it works the same apart from one
  small change to "Enter again to send" (see "For you to check").

## Done means

- A page can open Kiln in a tab, connect a character through the relay,
  see lines arrive, send a line, switch characters, open log mode, find,
  filter and export, and edit a world or character. All of it goes
  through the API, with no terminal anywhere.
- The test page shows the raw traffic both ways, and a headless Chrome
  smoke test drives it.
- Every settings error that's about one field says which field.
  Settings are checked when they're saved, not only as you type.
- The golden screens are unchanged and every existing test passes,
  except tests of the over-limit confirmation, which change as described
  below.
- The xterm shim, `web/bridge` and the Bubble Tea pin are gone.

## For you to check

These are the decisions that change what someone sees or does.
Everything else in this spec is plumbing, and you can skip it.

1. **"Enter again to send" (TUI and web).** Today, an over-limit line
   needs a second Enter, and *any* keystroke in between makes you start
   over, even typing a letter and deleting it. New rule: the second
   Enter sends if the text is **exactly what it refused**. Change
   anything and it asks again; switching characters still resets it.
   The difference is only visible if you edit and then undo the edit.
   (Why: the web page doesn't tell Kiln about every keystroke, so Kiln
   compares the text instead.)
2. **Drafts survive a closed tab (web only).** When you leave the tab
   (another app, another tab, locking the phone), the page hands Kiln
   your half-typed lines, and Kiln saves them with your other files.
   If iOS kills the tab while it's in the background, your drafts are
   there when you come back. Closing the tab yourself isn't guaranteed
   to save them, since Safari doesn't always say it's closing.
   The TUI doesn't keep drafts across quitting: quitting is rarer there,
   and it isn't worth it (your call, 2026-10-11). Input history isn't
   saved either, in the web or the TUI.
3. **Settings errors (TUI and web).** Each error about one field
   carries that field's name. On the web it shows next to the field,
   and a screen reader announces it as part of that field. A few errors
   aren't about any one field ("couldn't write the file", "this world
   still has characters"), and those show at the top of the form. The
   TUI keeps showing every error where it does today.
4. **Settings are checked on save.** Today some rules (no spaces in a
   host, which characters a world ID can use, the port range) are only
   enforced by the TUI refusing keystrokes. Kiln now checks them when
   anything is saved too. The TUI still refuses the keystrokes, so
   nothing changes there. The web lets you type and then flags the
   field, which is friendlier to screen readers than input that silently
   vanishes.
5. **Copy and notifications on the web are best effort.** Copy may not
   work from some keyboard commands in Safari. On iPhone, notifications
   only work once Kiln is added to the Home Screen
   ([OneSignal](https://documentation.onesignal.com/docs/ios-web-push)).
   Part 2 doesn't work around either.

## How the page and Kiln talk

For readers who want the shape. The rest of the spec fills this in.

- **One queue in.** Everything that can change Kiln's state arrives as a
  message on one queue: what the page sends, what the MUCK sends, older
  log days read from storage, timers. Kiln handles one at a time, in
  order, so a keystroke and a history recall can't cross wires.
- **Changes out, per thing.** After each message, Kiln compares each
  thing the page shows with what it last sent, and sends the ones that
  changed. Those things are: the sidebar, the active item, the status
  line, each character's input and prompt, each character's
  scrollback, log mode, the filter panel, and the list of worlds and
  characters. Small things go out whole each time they change.
  Scrollback goes out as just the new lines.
- **Why compare instead of announcing.** If every bit of code that
  changes, say, an unread count had to remember to announce it,
  someone would eventually forget, and the web would quietly show a
  stale count while the TUI (which redraws everything) looked fine.
  Comparing in one place after each message can't forget.
- **Lines are numbered.** Every scrollback and log-mode line gets a
  number, counting up as lines arrive and down as older history loads.
  The page can always ask for "everything after line N", and a log
  selection is "lines N to M".
- **A few plain lookups.** The page can ask the over-limit question
  directly ("is this text too long for this character?") without going
  through the queue. It's the same code the TUI uses, so the two can't
  disagree.

## What changes in the core

Each change here shrinks the core or makes it front-end neutral. The
TUI keeps its behavior.

**The filter panel (#135).** The panel's calls take a tag or a text
term, not "the highlighted row": `ToggleHide(item)`, `PressOnly(item)`,
`Fold(tag, shut)`, `AddText(term)`, `RemoveText(term)`. Whether the
panel is open, the highlight and moving it (`Panel`, `Panel.Sel`,
`TogglePanel`, `MovePanel`, `Select`) go back to `internal/ui/filter.go`,
along with keeping a highlight after a text row is removed. The filter,
the folds (`Collapsed`) and "a newly seen parent starts folded"
(`FoldSeen`) stay in the core: they outlast a log-mode session.
`Entries()` also returns the children of folded parents, marked as
folded away, so a web panel can open a fold without asking again. This
amends part 1's spec, which put the panel's selection in `app.Log`.

**Log mode's cursor.** `Log.Cursor`, `SetCursor`, `MoveCursor`,
`ToTop` and `ToBottom` move to the TUI. The marked range and exclusions
stay in the core, addressed by line (see "Line numbers"). Find stays in
the core: the matches, and the next match older or newer than a line the
front end names, reading older days until one turns up. Where you are is
the front end's: the TUI passes its cursor and moves it to what's found,
and the web passes what's on screen and scrolls. `Log.Shown` stays
("find matches what you see").

**The over-limit confirmation.** `App.confirm` becomes the text that was
refused. `Submit` sends an over-limit line when the text equals it, and
switching or closing clears it as today. `Unconfirm` and its calls in
`internal/ui/model.go` go away. `overLimit` is exported as a plain
function of (text, character) for front ends to mark the overflow live.

**Settings errors.** A new `app.FieldError{Field, Msg}`. Field names are
the config keys (`host`, `port`, `max_line_bytes`, …), which are
vocabulary and never translated. `AddWorld`, `SaveWorld`,
`SaveCharacter` and `AddCharacter` check every field before writing:
everything the `Accept*` keystroke filters enforce, plus the port range
(moved from `ui/editor.go`'s `ParsePort` call into the core). Each one
that fails returns a `FieldError`; an error about no one field stays a
plain `error`. Several errors at once come back joined (`errors.Join`),
so a form can mark every bad field in one go. The `Accept*` filters stay
for the TUI's keystroke filtering. The TUI shows the message as it does
now.

**Line numbers.** `app.Line` and `app.LogLine` get a `Seq int`. Each
character numbers its scrollback lines: live lines count up from 0, and
older pages count down from -1. Log mode numbers its own lines the same
way. A rules change that remakes the scrollback keeps each line's
number.

**Drafts (web only).** `Deps` gets an optional drafts store, which the
TUI leaves nil. The web saves drafts to a file in Kiln's data directory
(in memfs, so `persist.mjs` saves it to IndexedDB), and a character's
saved draft becomes its input text when it opens.

## The change feed

A new package, `internal/feed`, plain Go and tested natively.

- `feed.New(a *app.App)` keeps what it last sent. `feed.Turn()` runs
  after every message, compares, and returns the events for what
  changed. The TUI doesn't use it.
- Small things are compared whole and sent whole when they differ.
- Scrollback and log-mode lines are tracked by the highest and lowest
  numbers sent, and new lines at either end go out as `Appended` or
  `Prepended`. A rules change that remakes lines sends `Replaced` (all
  lines, same numbers, new tags), because the tags may have changed.
- `feed.Since(key, seq)` gives everything after a line, for a page that
  wants to catch up, and `feed.All()` gives everything, for a page just
  starting.
- Effects (`Notify`, `Copy`, `SaveFile`, `Do`, `Quit`) go out as events
  too, in order with the changes.

## The wasm side

`web/cmd/kiln-web` is rewritten. It keeps `roots.go`, the dialer
(`web/wsdial`), backup and restore (`web/backup`), memfs and
`persist.mjs`, and the tab lock (`tablock.mjs`).

- **The loop.** One goroutine owns the `app.App` and drains one
  channel. Page actions, `Run` results and timers all arrive on it.
  After each message it calls `feed.Turn()` and hands the events to the
  page.
- **Across the boundary.** The page calls one Go function with an action
  as JSON, which puts it on the channel and returns right away. Go calls
  one page function with a batch of events as JSON. The over-limit check
  is a second, synchronous Go function.
- **Actions.** One per thing a person can do: open, close, switch,
  submit (with the text), history older/newer (with the text), set
  input, here, focus, the filter calls, log-mode calls (by line number),
  find, export, the settings calls, answer the save-password question,
  quit. Each one maps to an `app` method; the list is fixed in the
  plans.
- **Presence.** The page sends `here` on a keystroke, click or tap (at
  most once a second) and `focus` on the window's focus and blur.
- **Leaving the tab.** On `visibilitychange` to hidden, and on
  `pagehide` as Safari's backup, the page sends the drafts and asks
  `persist.mjs` to save right away
  ([WebKit #205942](https://bugs.webkit.org/show_bug.cgi?id=205942)).
- **Effects.** `Run` is a goroutine. `SaveFile` is a download.
  `Notify` uses the Notification API when permitted. `Copy` uses the
  Clipboard API and may fail in Safari. `Do` goes to the page, which
  ignores what it can't do yet.
- **The test page.** `web/static/index.html` becomes a bare page: a
  character list, a log of events, and a box to send actions. It's for
  development only.

## What goes away

- `web/bridge` and its tests.
- The xterm.js page code and `web/static/vendor`'s xterm files.
- The Bubble Tea replace in `web/go.mod`, `web/bubbletea_test.go` and
  `.github/workflows/bubbletea-watch.yml`. They existed so Bubble Tea
  could run in wasm, and the web build no longer runs Bubble Tea.
- `App.Unconfirm`, and the panel and cursor calls that move to the TUI.

## Order of work

Each step is one PR. Steps 1 to 3 are native Go and don't touch the
web build.

1. **Core cleanups.** #135, log mode's cursor, the over-limit
   confirmation and settings errors. These could be two PRs: the panel
   and cursor, then confirmation and errors.
2. **Line numbers.**
3. **The change feed.**
4. **The wasm loop and API.** The new `kiln-web`, the test page and
   drafts, and deleting the shim.

## Testing

- **TUI:** the golden screens are never `-update`d. Existing tests keep
  passing, except over-limit tests that relied on an edit clearing the
  confirmation. A new test pins the edit-and-undo case.
- **Core:** tests for the item-addressed panel calls, `FieldError`s from
  each save call (one test per field rule), line numbers through
  appends, older pages and a rules change.
- **Feed:** tests drive an `app.App` with the shared fakes in
  `internal/kilntest` and check the events: one change, one event; no
  change, no event; lines in order with no gaps across appends and
  prepends; `Since` matches what was sent.
- **Wasm:** the action and event JSON round-trips in native tests.
  A headless Chrome smoke test loads the test page against a scratch
  config, connects through the relay to a fake MUCK, sends a line and
  sees it echoed.

## Not in this part

- The real web UI (part 3), the phone layout (4), the detachable relay
  (5) and the PFMuck launch (6).
- Showing the TUI's settings errors next to their fields.
- Work-arounds for Safari's clipboard or iPhone notifications.
- What size the web reports to the MUCK (NAWS: how many columns wide
  the screen is). Part 3 decides that with the layout; the API takes a
  size like the TUI does.
