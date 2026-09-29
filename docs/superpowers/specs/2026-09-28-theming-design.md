# Theming (#76)

## Goal

Kiln's look becomes a theme: a file of named colors and styles for every
part of the screen and every kind of line, chosen per world or character.
Today's styling is tangled. `[[highlight]]` rules match lines, color them
(raw hex) and set behavior (attention, quiet) all at once, and the chrome
is hard-coded (dim text, reverse video, a red for errors, a blue for
links). Theming untangles those three jobs, and gives the default look a
redesign along the way.

## The rule

**Anything the server wrote sits on the terminal's background. Everything
Kiln draws can be any color.**

- The scrollback, and log mode's body, keep the terminal background. Server
  colors (ANSI) are left to the terminal. Its palette was chosen for its
  own background, so server text stays readable, and an `ESC[0m` from the
  server never punches a hole in a painted area.
- The sidebar, input, statusline, rules, log mode's header and action bar,
  forms and the picker are **areas**. Each can have its own background and
  colors.
- Kiln's own styling of server lines (tag styles, below) may still use any
  color, background included. `style.Highlight` already re-applies our
  style after every server reset.
- Remapping server ANSI colors to a theme palette is out of scope. It may
  come later.

## Three layers, joined by tags

1. **What a line is: classify.** Classify rules tag lines, as they do
   now. A tag also remembers where in the line its pattern matched, which
   a tag style can use (`scope`, below).
2. **How it looks: the theme.** A palette, styles for the chrome's roles,
   and styles for tags.
3. **How it behaves: attention and quiet.** These become tag lists,
   inheritable like other settings (packs, `[defaults]`, world,
   character):

   ```toml
   attention = ["page/in", "whisper/in", "self"]
   quiet = ["spam"]
   ```

   Anything pattern-based gets a classify tag first.

With these three, `[[highlight]]` has no job left of its own (see
Migration).

## Theme files

Themes live in `themes/<name>.toml` in the config directory. The built-in
`default` theme is embedded, like the string catalog. It's always there,
and a user's `themes/default.toml` overrides it. A theme can say
`extends = "default"` and set only what it changes.

```toml
extends = "default"

[palette]            # names for colors; values are "#rrggbb"
ember  = "#ff9f43"
ash    = "#6b6f7a"
panel  = "#1f2029"

[ui]                 # the chrome's roles
"sidebar"           = { fg = "ash", bg = "panel" }
"sidebar.active"    = { fg = "#ffffff", bg = "panel", bold = true }
"status.error"      = { fg = "ember", bold = true }
"link.hover"        = { fg = "#5fafff", underline = true }

[tags]               # styles for classify tags on server lines
"page/in"   = { fg = "ember", bold = true }
"whisper"   = { fg = "#c39bd3", italic = true }
"highlight" = { fg = "#ffd166", bold = true }
```

- A style has `fg`, `bg`, `bold`, `italic` and `underline`. A color is a
  palette name or `#rrggbb`.
- A tag style can also say `scope = "match"` to style only the text its
  tag's pattern matched, instead of the whole line (`scope = "line"`, the
  default). That's a look decision, so it lives with the style, not the
  classify rule, and it suits some tags and not others: `highlight` wants
  just the word, while `page/in`'s pattern only matches the `Mira pages:`
  prefix, so it wants the whole line. It takes over from `[[highlight]]`'s
  `scope`. The default theme uses free (24-bit) colors.
  Terminals with fewer colors get them downsampled by `colorprofile`,
  already a dependency through Bubble Tea.
- Roles fall back up their dots (`sidebar.active` → `sidebar`), and tags
  fall back up their slashes (`page/in` → `page`). A role with no style
  anywhere draws plainly.
- Several tags on one line fold together: whole-line styles first, then
  match-scope styles on top of the text they cover. Within each, a
  world's or character's `[tags]` come after the theme's, and later colors
  win while attributes add up, as highlight rules do now.
- Themes are live. Editing one reloads it like the rest of the config, and
  the scrollback restyles (the #79 plumbing).

## Choosing a theme

`theme = "name"` is a global setting in config.toml (default `"default"`).
The chrome is one frame around every world, so the theme is global too: it
never changes when you switch characters.

Worlds and characters shape how their own lines look with a theme's line
tables, `[palette]` and `[tags]`, layered over the theme: a character's over
its world's, over the theme. A world can name its own colors and style its
own special tags, in the same format as a theme file. They never carry
`[ui]`.

## Light and dark

The painted parts (chips, the over-limit mark) bring their own backgrounds,
but everything else sits on the terminal's background (the chrome's text and
lines, tag styles, links, the scrollback's roles, log mode's body), and a
color picked for dark can wash out on light.

- A theme can give a palette for each appearance: `[palette.dark]` and
  `[palette.light]` on top of `[palette]`. Tags and roles refer to palette
  names, so they follow along. Along `extends`, each file adds its
  `[palette]` and then its entry for the current appearance; later files
  win. A world's or character's `[palette]` can do the same.
- `appearance = "auto"` (the default), `"dark"` or `"light"`, global in
  config.toml. Auto asks the terminal for its background (Bubble Tea's
  `RequestBackgroundColor`, OSC 11) at startup and each time the window
  gets focus back (Bubble Tea has no color-scheme-change event), and goes
  by `IsDark`. Until an answer comes, or if none does, Kiln assumes dark.
- Checked: eternal terminal (through tmux) answers; mosh doesn't, so over
  mosh a light terminal needs `appearance = "light"`. `kiln tail` doesn't
  ask; it uses `appearance`, with auto meaning dark.
- The default theme's light palette keeps each glaze's hue: pale tints for
  chips, dark inks for text, and darker tag colors for paper.

## A full example

One world, `fm`, using a theme called `ember`. Here's every file involved,
then what three incoming lines look like.

**The pack** (`packs/fuzzball.toml`) says what lines are and how they
behave. No colors:

```toml
[[classify]]
tags = ["page", "page/in"]
pattern = '^\S+ pages( from [^:]+)?: '

attention = ["page/in", "whisper/in", "self"]
```

**The theme** (`themes/ember.toml`) says how things look, with a palette
for each appearance:

```toml
extends = "default"

[palette]
ember = "#ff9f43"
panel = "#1f2029"

[palette.light]
ember = "#b35900"           # darker, to read on a light background

[ui]
"sidebar" = { fg = "#9aa0ad", bg = "panel" }

[tags]
"page/in" = { fg = "ember", bold = true }       # scope "line": the whole line
"self"    = { bold = true }
```

**config.toml** picks the theme: `theme = "ember"`.

**The world** (`worlds/fm.toml`) adds its own bits.
The classify rule is what `/highlight lighthouse` writes; the `[tags]`
entry could just as well live in the theme:

```toml
host = "furrymuck.com"
port = 8899
use = ["fuzzball"]

[[classify]]                # added by /highlight
tags = ["highlight"]
pattern = '(?i)lighthouse'

[palette]
beacon = "#ffd166"          # a color only this world uses

[tags]
"highlight" = { fg = "beacon", bold = true, scope = "match" }
```

**What arrives, and how it's drawn:**

```
Rook pages: the lighthouse is dark
└─────────── ember, bold ─────────┘     page/in, scope "line": the whole line
                ^^^^^^^^^^              highlight, scope "match": just this, beacon on top

Sable waves a paw.                       no tags: the server's text, as sent

Ash says, "Kit, over here."
└──────────── bold ─────────┘           self (Kiln tags lines that mention Kit)
```

- The page gets three tags: `page` and `page/in` from the pack, and
  `highlight` from the world. `page` has no style, so it falls back to
  nothing; `page/in` styles the line; `highlight` styles only the word.
  The whole line is orange and bold, and "lighthouse" is yellow and bold.
- It's an attention line, because `page/in` is in `attention`. So is the
  last line, through `self`. Neither has anything to do with color: the
  theme can change without touching behavior, and the other way round.
- On a light terminal, `ember` becomes `#b35900` and nothing else changes.

**The same thing today**, for comparison: one `[[highlight]]` rule does
all three jobs at once, colors in raw hex:

```toml
[[highlight]]
match = { tags = ["page/in"] }
style = { fg = "#ff9f43", bold = true }
attention = true

[[highlight]]
match = { pattern = '(?i)lighthouse' }
style = { fg = "#ffd166", bold = true }
scope = "match"
```

## Roles

Every hard-coded style becomes a role. A first cut, from the code:

| Area | Roles |
|---|---|
| Sidebar | `sidebar`, `.world`, `.char`, `.active`, `.unread`, `.attention` (the ●), `.connecting`, `.disconnected`, `.add`, `.more` (▲/▼ hints) |
| Picker | `picker.world`, `.char`, `.selected`, `.add` |
| Divider | `divider` (the `│`), `rule` (the `─` lines) |
| Scrollback | `scrollback.day` (day dividers), `.history_end`, `.loading`, `.echo` (your sent lines), `.sys` (`*` lines), `.pill`, `.selection`, `.inactive` (the backdrop, below), `link`, `link.hover` |
| Input | `input`, `.gutter`, `.hint` (dim prompts), `.over_limit`, `.selection` |
| Statusline | `status`, `.log` (the LOG label), `.error`, `.clock` |
| Forms | `form.label`, `.hint`, `.error`, `.focus`, `.button`, `.section`, `.title` |
| Log mode | `log.header`, `.chip`, `.chip.on`, `.time`, `.cursor`, `.selected` (▌), `.excluded` (░), `.find`, `.hints` |
| Export | `export` (the HTML page's background and text) |

The list gets settled while building phase 1. Role names are API: themes
refer to them, so they're named for what they are, not how they look.

## Areas

Painting an area takes three things, and one helper per area does them:

- **fill:** every row padded to the area's full width in its background,
  empty rows included;
- **re-assert:** anything drawn inside (a role's style, a reset) ends by
  restoring the area's colors, never the terminal's;
- **no dim:** SGR 2 renders differently in every terminal and turns muddy
  on colored backgrounds, so each current dim becomes a real role color.

The scrollback and log body use the same helpers with no background, which
is where they already are.

## The backdrop

While a modal owns the input area (the Ctrl+O picker, the world and
character editors, and maybe the save-password question), the scrollback
turns into a backdrop. Its rows lose their styling (server colors, tag
styles, links) and are drawn in `scrollback.inactive`, so the eye goes to
the modal.

- The rows are stripped and repainted, not dimmed with SGR 2. Dim would
  need re-applying after every server reset, and terminals render it
  inconsistently (some barely fade truecolor text). A repaint looks the
  same everywhere.
- Fading the real colors instead would mean querying the terminal's
  palette and background (OSC 4 / OSC 11) and blending toward them. That's
  out of scope.
- Log mode replaces the pane, so it doesn't apply there.

## Migration

- `[[highlight]]` goes away, with no shim: Kiln has one user so far, and
  their config is rewritten by hand to the new form. A file that still has
  `[[highlight]]` fails to load with the usual unknown-key error.
- The fuzzball pack moves to the new form: classify tags plus
  `attention = ["page/in", "whisper/in", "self"]`. Its colors move to the
  default theme's `[tags]`.
- `/highlight <text>` adds a classify rule with tag `highlight` to the
  world. The theme styles `highlight` once, so every highlight follows the
  theme. (The sidebar's attention dot has its own role, `sidebar.attention`.)
- `config.HighlightStyle` goes away.

## Phases (child issues)

1. **Theme engine and chrome roles.** Theme files, palette, fallback,
   `extends`, live reload. Every hard-coded style becomes a role, drawn by
   the area helpers. The modal backdrop comes with it. The default theme reproduces today's look closely,
   with tests pinning it, so the plumbing lands without a visual change.
2. **The new default look.** Indi's redesign, on top of phase 1.
3. **Tags, behavior and migration.** `[tags]`, world and character theme
   tables, `scope`,
   the attention/quiet lists, dropping `[[highlight]]`, the fuzzball pack,
   and `/highlight`.
4. **Choosing themes.** The global `theme` setting, light and dark
   (`appearance`, palettes per appearance, detection), the default theme's
   light palette, and restyling on change.
5. **Later:** importing palettes from other tools (base16/base24 YAML,
   Ghostty and kitty theme files), and maybe ANSI remapping.

## Core tags

Themes can count on these. Packs emit them where their server makes that
possible, and are free to add their own (a world's special tags are
styled in the world; see Choosing a theme).

| Tag | Lines |
|---|---|
| `page`, `page/in`, `page/out` | pages to you, and your own |
| `whisper`, `whisper/in`, `whisper/out` | whispers to you, and your own |
| `watchfor`, `watchfor/connect`, `watchfor/disconnect` | watchfor notices of people connecting and disconnecting |
| `ooc` | out-of-character talk |
| `system` | messages from the MUCK itself (not Kiln's own `*` lines, which are the `scrollback.sys` role) |
| `self` | lines that mention you (Kiln tags these from the character's name and aliases) |

`say` and `pose` aren't core: on a MUCK they're the bulk of ordinary
scene text, not something to set apart. The fuzzball pack's `say` tag is
gone for the same reason.

## Decided against

- **A default style for plain server text.** Server text keeps the
  terminal's foreground; a fixed one would fight light and dark.
- **A scrollback background.** The rule stands: server text sits on the
  terminal's background.
