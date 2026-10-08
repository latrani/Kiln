# Web-native Kiln, part 1: a headless app core

Epic #125 (web-native Kiln), sub-project 1 of 6. The epic replaces #74's
"Kiln's TUI on xterm.js" with a real web UI over Kiln's core. Before
there can be a second front end, what Kiln *decides* has to live
somewhere other than the Bubble Tea model.

## In short

- A new package, `internal/app`, holds everything Kiln decides: which
  characters are open and in what order, unread and attention, the input
  text and history, the password prompt, scrollback lines, notifications,
  slash commands, the log viewer's selection and filters, and edits to
  worlds and characters.
- `internal/ui` keeps what is about the terminal: turning keys and clicks
  into `app` calls, wrapping and painting lines, scroll offsets, mouse
  selection, the picker and editor forms as drawn, and the theme.
- Nothing changes for anyone using Kiln. The golden screens don't change,
  and the existing `ui` tests keep passing.
- It lands as a series of PRs, each one moving one area and leaving Kiln
  working.

## Done means

- `internal/app` doesn't depend on Bubble Tea or lipgloss, never paints
  (no `theme.Paint` or `theme.Fill`), and has its own tests. It may read
  `theme` as data: `config` already carries per-world tag looks, and the
  web will turn the theme into CSS.
- `internal/ui` holds no state that a web UI would also need: everything
  in the list above lives in `app`, and `ui` reads it from there.
- The golden screens in `internal/ui/testdata/golden` are unchanged and
  every existing test passes. Tests that only reached into `Model` fields
  to set up state may change how they set it up; none change what they
  expect.
- `go test ./internal/str` passes: strings move with their code and stay
  in the catalog.

## Shape: a plain struct, driven by its front end

`app.App` is an ordinary struct, not safe for concurrent use. It runs on
its front end's loop: Bubble Tea's `Update` in the TUI, and later a queue
goroutine in wasm.

- **Actions are methods.** `Open`, `Close`, `Switch`, `SwitchBy`,
  `NextUnread`, `SetInput`, `Submit`, `Here`, `Focus`, `Command`, and so
  on. A front end calls them in response to its own input.
- **Async results come back through `Handle(msg)`.** Session events, older
  log days, a config reload, timers.
- **Side effects go out as `[]app.Effect`.** Every method that can cause
  one returns them. The front end performs each its own way:

  | Effect | TUI | Web (later) |
  |---|---|---|
  | `Notify{Title, Body}` | `notify.Encode`, OSC 9 or bell | Notification API |
  | `Copy{Text}` | OSC 52 | Clipboard API |
  | `OpenURL{URL}` | `Deps.OpenURL` | `window.open` |
  | `SaveFile{Name, Data}` | write under `export_dir` | download |
  | `Run{Func func() Msg}` | a `tea.Cmd` | a goroutine |
  | `After{D, Msg}` | `tea.Tick` | `time.AfterFunc` |
  | `Quit` | `tea.Quit` | close the page's session |

  `Run` is how blocking work (waiting on a session's next event, reading
  an older log day, a backup) stays off the loop. Its result comes back
  through `Handle`, as `tea.Cmd` results do today.
- **State is read through accessors** after any call: `Sidebar()`,
  `Active()`, `Lines(key)`, `Input(key)`, `Status()`, `Presence()`,
  `Log(key)`, and so on. The front end redraws from them. There's no
  change feed in part 1; the TUI redraws every update, as now. Part 2
  (the web API) adds whatever change signal the page needs.
- **Dependencies** move from `ui.Deps` to `app.Deps`: `Dial`, `NewLog`,
  `Load`, `Password`/`SavePassword`/`DeletePassword`, `Backup`, `Restore`,
  `Now`, the config and log paths. `ui.Deps` keeps what's about the
  terminal: `Raw`, `Tmux`, `Remote`, and the `OpenURL`/`SaveFile`
  implementations used to perform effects.

## Lines

Today a line becomes a painted ANSI string early: `renderLine` runs the
highlight rules, which look up each tag's style in the theme and produce
SGR. The core can't do that, since a web page paints with CSS.

- **`app.Line`** is a log entry plus what Kiln made of it: the kind
  (server, echo, sys, day divider), its tags, attention and quiet, and
  highlight runs as `{Start, End, Tag}`. A run names the tag whose style
  wins, not an SGR. The server's own colors stay in the entry's text.
- **`rules` stops using the theme.** `rules.New` takes the attention and
  quiet lists only, and `Result.Runs` carries tags. Choosing which tag
  wins a span (the existing precedence) stays in `rules`.
- **The TUI paints.** A tag's run becomes the theme's SGR for that tag,
  and `style.Highlight` draws it over the server text, as today. Echo and
  sys lines get their roles (`ScrollbackEcho`, `ScrollbackSys`), and day
  dividers `ScrollbackDay`. The painted text is cached per line in `ui`.
- **A theme change** clears the TUI's paint cache. `Scrollback.Rerender`
  and `browse.restyle` go away; the core has nothing to redo.
- **Scrollback splits.** `app` holds each character's `[]Line`, whether
  older days remain, whether a page is loading, and the unseen count.
  `ui.Scrollback` keeps the width, wrapping, the scroll offset, mouse
  selection and hover, reading lines from `app`. Pausing ("seen" up to a
  line) is reported to `app` so unread stays right.

## What moves, area by area

**Characters and the sidebar.** `charState` splits. The `app` half: the
character, session and cancel, state, classifier and highlighter, lines,
input, unread, attention, pin mismatch, password prompt, orphan flag,
history reader, notification bookkeeping. The `ui` half: scrollback view
state and log-mode view state. `app` owns open/close, the sidebar order
(`order`, `sortOrder`, `stops`), the active item (character or world
overview), `recent`, switching (by delta, to unread, to a key), and the
idle input used when nothing is open. Sidebar *rows* (indent, marks,
truncation) stay in `ui`; `app.Sidebar()` returns the items in order with
their state, unread and attention.

**Input.** `app` owns each character's text, caret, history and draft, so
history, per-character drafts, `Commit` and `CommitSecret` work the same
for any front end. `SetInput(text, caret)` replaces the text (the web's
path). The TUI keeps its editing keys: `ui.Input` becomes the editing and
drawing layer over `app`'s buffer (cluster-aware movement, wrapping,
`Render`, mouse selection), and history Up/Down calls into `app`. The
over-limit check and its "Enter again to send" confirmation move to `app`.

**Submit, commands and the password prompt.** `submit`, `command` and
every slash command move to `app`, with the status messages they set.
The password prompt (`startPassword`/`endPassword`, the stashed draft,
Esc to skip) moves too. The save-password question is a state in `app`
(`PendingSave()`); the TUI draws it and maps y/n to `app.AnswerSave`.

**Status.** The status line's message, error flag, log-mode ownership and
expiry move to `app`. Expiry is an `After` effect. The quit arming
(Ctrl+C or Ctrl+D twice) stays in `ui`: it's about keys. `app.Quit()`
cancels the sessions and returns `Quit`.

**Presence and notifications.** `here`, focus, `/away`, `notify_idle`,
`presence`, `/notify` overrides, and the notification decision (grace
after connecting, bursts, levels) move to `app`. The front end reports
focus and activity with `Focus(bool)` and `Here()`. A notification is a
`Notify` effect with the title (the character's name, with `@world` when
needed) and body; encoding it is the TUI's job.

**Log mode.** The log viewer's behavior moves to `app.Log`: its lines
(as `app.Line`), cursor, range marks, exclusions, find and its progress
through older days, the filter (already `scene.Filter`) and the panel's
entries, folds and selection, export and copy (as `SaveFile` and `Copy`
effects), and its status. `ui` keeps what's about drawing: the top line
of the body, `rowLines`, the panel's scroll window, click hit-testing,
and the key map, which turns keys into `app.Log` calls. The prompt inputs
(find, date) use `app` input buffers like the main one.

**Worlds and characters.** Saving a new world, saving a world's or
character's settings, deleting either, forgetting a password and the
delete warnings move to `app` as methods that take
`config.WorldSettings`/`config.CharacterSettings` and return field
errors by field. Field validation (`onlyMatching` and the rest) moves
with them. The forms themselves, the picker's filter and selection, and
editor drafts and parking stay in `ui`: a web page will have its own
forms.

**Config.** Reloading (`applyConfig`), recompiling classifiers and
highlighters, the logstore layout and preloading move to `app`. Watching
the config dir stays with the front end, which calls `app.Reload()`.
Loading the theme stays in `ui`.

## Order of work

Each step is one PR that leaves Kiln working, the golden screens
unchanged and every test passing.

1. **Lines.** `rules` without the theme, `app.Line`, and the TUI painting
   lines from it. `app` starts as just this.
2. **The app skeleton.** `app.App` with `Deps`, effects, and characters,
   sessions, the sidebar order, switching, unread and attention, status,
   input and history, submit, commands and the password prompt. `ui.Model`
   holds an `*app.App`. The biggest step.
3. **Presence and notifications.**
4. **Log mode.**
5. **Worlds and characters, and config reload.**

Steps 3 to 5 don't depend on each other, only on 2.

## Testing

- **The existing `ui` tests are the net.** They drive `Model` with key,
  mouse and session messages and check the screen, so they show that
  behavior didn't change. They stay; tests that poked `Model` fields to
  set things up go through `app` instead.
- **The golden screens are pinned.** No step runs `-update`.
- **`app` gets its own tests** for what moves, written against `app`
  directly with a fake `Dial` and `NewLog`. The fake connections the `ui`
  tests use move to a small shared test package so both can use them.
- **No terminal in `app`:** a test fails if `internal/app` depends on
  `charm.land/bubbletea` or lipgloss, or calls `theme.Paint` or
  `theme.Fill`.

## Not in this part

The wasm API and the web UI (parts 2 to 4), the detachable relay (5),
and the PFMuck launch (6). Changes to how anything looks or behaves.
Splitting `internal/ui` into more packages; it gets smaller, not
reorganized.
