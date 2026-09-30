# OS notifications

## Goal

When you've tabbed away from Kiln, activity on your characters reaches you
as a desktop notification, through whatever terminal you're using: iTerm2
locally, over ssh, inside tmux on a shell server, or from Blink on iOS
(including over mosh, as a bell). While you're looking at Kiln, nothing
pops up; the sidebar already shows unread and attention.

## When you count as away

- Kiln turns on focus reporting (DECSET 1004) through Bubble Tea's
  `View.ReportFocus`. `tea.BlurMsg` means away; `tea.FocusMsg` means here.
  Mosh 1.4 relays focus events; tmux needs `focus-events on`.
- **Idle fallback:** you're also away when there's been no key or mouse
  input for `notify_idle` (default 5 minutes). This covers terminals that
  never send blur, or miss it (Blink sends blur for Cmd+Tab but not for
  swiping the app away).
- At startup Kiln assumes focus. A terminal without focus reporting just
  relies on the idle fallback.
- `lastHere` is the time of the latest focus-in, key press, paste, click,
  or wheel while focused (not mouse motion, and not wheel while blurred:
  terminals report hover, and macOS scrolls, over unfocused windows). It
  starts at launch time. Each of these also bumps `hereGen`.
- Away is worked out when a line arrives: `!focused || now − lastHere >
  idle` (idle 0 disables the fallback). There's no ticker.

## What notifies

Each character has a level, from the inheritable `notify` setting,
overridden by `/notify` for the life of the app. While away, an incoming
(`Dir == In`), non-quiet line notifies when:

| Level | Notifies |
|---|---|
| `all` | every such line |
| `first` (default) | the first such line since `lastHere`, then only attention lines |
| `attention` | only lines an attention rule matched |
| `none` | never |

`first` keeps a per-character `sentGen`: the `hereGen` when its last
notification went out (-1 before any). A line counts as first when
`sentGen != hereGen`; sending sets `sentGen = hereGen`. Coming back
therefore re-arms every character without any transition bookkeeping, and
without comparing clock readings.

For `connectGrace` (5 seconds) after a connection comes up, only attention
lines notify, so the login banner and MOTD don't use up `first` or flood
`all`.

Likewise, for `burstGap` (250ms) after a character's last notification,
only attention lines notify: a multi-line description arrives as one
burst, and only its first line notifies.

Quiet lines never notify, at any level, even `all`. (Quiet already beats
attention in the rules engine, so a quiet line is never an attention line
either.)

Away covers every character, including the active one.

**No holding.** Only a line that arrives while you're away notifies. An
earlier version held lines that arrived while you still counted as here and
sent them if `notify_idle` passed without you coming back, so that
something sent just after you looked away wasn't missed. With blur
detection that only produced notifications for lines you'd already read, so
it's gone: what arrived while you were here, you saw. Where focus events
don't reach Kiln (mosh, tmux without `focus-events`), idle is the only
signal, so the first `notify_idle` after you leave isn't covered.

System lines (connect, disconnect) never notify.

## The message

- `Name: line`, where Name is the character's name. When another
  configured character in a different world has the same name
  (case-insensitive), it's `Name@world: line`.
- The line is stripped of ANSI, then of every remaining C0 and C1 control
  character (so MUD text can't end the OSC early or inject sequences), and
  cut to 200 characters with a trailing `…` if longer.

## On the wire

- `notify_method` (global): `"osc"` (default), `"bell"`, or `"both"`.
  - `osc`: `ESC ] 9 ; message BEL` (iTerm2, kitty, Ghostty, WezTerm,
    foot, Blink).
  - `bell`: a lone `BEL`. Mosh strips OSC 9 but relays the bell, and Blink's
    "Notification on background shell" turns it into an alert without
    text. iTerm2 can also notify on bell.
  - `both`: the OSC, then a BEL.
- **tmux:** when `$TMUX` is set (read once at startup), each sequence is
  wrapped as `ESC P tmux ; <payload with every ESC doubled> ESC \`. This
  needs `allow-passthrough on` (tmux 3.3+). A lone BEL isn't wrapped;
  tmux forwards bells on its own.
- Written with `tea.Raw`, batched with the event's next `waitEvent`.
- Only OSC 9. OSC 777 and OSC 99 are out of scope.

## Config

- `notify = "all" | "first" | "attention" | "none"` joins the inheritable
  settings (`[defaults]`, world, character), validated like
  `newline_mode`. It goes through `edit.go` and gets an inherit-choice
  field ("Notify") in both editor forms.
- `notify_idle` (top-level in `config.toml`): a Go duration string,
  default `"5m"`; `"0"` turns the idle fallback off. Invalid or negative
  values are config errors.
- `notify_method` (top-level): see above; anything else is a config error.
- The bundled default `config.toml` gets commented lines for all three.

## `/notify`

- `/notify <level>`: override the current character's level until Kiln
  quits (it survives reconnects and config reloads).
- `/notify default`: drop the override.
- `/notify`: show the level and where it comes from, e.g.
  `notify: attention (override; config says first)`.
- A bad level is a status error listing the choices.
- Listed in the README's command table.

## Code layout

- **`internal/notify`** (new, pure):
  - `Level` with `ParseLevel`, and `Method` with `ParseMethod`.
  - `Message(name, line string) string`: sanitize and truncate.
  - `Encode(msg string, m Method, tmux bool) string`.
- **`internal/config`**: `Character.Notify`, `Config.NotifyIdle`
  (`time.Duration`), `Config.NotifyMethod`; the `settings`, `Inherited`,
  `WorldSettings` and `CharacterSettings` plumbing.
- **`internal/ui`**:
  - `Deps.Tmux bool` (main sets it from `$TMUX`), so tests don't depend on
    the environment, and `Deps.Raw func(string) tea.Cmd` (default
    `tea.Raw`), so tests can see what's written.
  - Model fields `focused`, `lastHere`, and `notifyOverrides` (by
    character key, so `/close` and reopening keep it); `charState` fields
    `sentGen`, `connectedAt`, `lastSent`. Key, paste and click also set `focused`, since only a
    focused window gets input (a lost focus-in mustn't leave Kiln "away").
  - The view sets `ReportFocus`. Update handles `FocusMsg`/`BlurMsg`, and
    key, paste, click and wheel messages stamp `lastHere` from `Deps.Now`.
  - `handleEvent` decides next to the unread/attention logic and batches
    the raw write.
  - `/notify` in the command switch.

## README

A short "Notifications" section: what notifies and when, the `notify`
levels, and terminal setup: tmux needs `allow-passthrough on` and
`focus-events on`; over mosh use `notify_method = "both"` with Blink's
"Notification on background shell".

## Testing

- `notify`: sanitizing (ANSI, BEL, ESC, C1), truncation at 200, each
  method, and the tmux wrap with doubled ESCs.
- `config`: defaults, inheritance of `notify`, validation of all three
  keys, editor round-trip.
- `ui` harness with a fake clock:
  - each level while away (blurred) and silence while here;
  - the idle fallback, and `notify_idle = "0"`;
  - `first` re-arming after focus-in and after a key press;
  - quiet lines and sent lines never notify;
  - `Name@world` when names collide;
  - `/notify` set, show and default, surviving a config reload;
  - tmux wrapping driven by `Deps.Tmux`.
- Manual: iTerm2 locally, ssh+tmux, and Blink over mosh with the bell (this
  confirms mosh relays the bell).
