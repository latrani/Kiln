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

1. **What a line is: classify.** Classify rules turn patterns into tags,
   as they do now. A rule can also say `span = true`: its tags then style
   only the matched text, not the whole line. That takes over from
   highlight's `scope = "match"`.
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
  palette name or `#rrggbb`. The default theme uses free (24-bit) colors.
  Terminals with fewer colors get them downsampled by `colorprofile`,
  already a dependency through Bubble Tea.
- Roles fall back up their dots (`sidebar.active` → `sidebar`), and tags
  fall back up their slashes (`page/in` → `page`). A role with no style
  anywhere draws plainly.
- Several tags on one line fold in theme order, as highlight rules do now:
  later colors win, attributes add up.
- Themes are live. Editing one reloads it like the rest of the config, and
  the scrollback restyles (the #79 plumbing).

## Choosing a theme

`theme = "name"` is an inheritable setting: config.toml's `[defaults]`,
then the world, then the character. Different worlds can look different,
and a character can differ from its world.

Worlds and characters can also carry a theme's own tables (`[palette]`,
`[tags]`, even `[ui]`), layered over the theme they use: a character's
over its world's, over the theme. A world can name its own colors and
style its own special tags, in the same format as a theme file.

## Light and dark

The painted areas bring their own backgrounds, so they look the same
whatever the terminal's background is. Everything else sits on the
terminal's background (tag styles, links, the scrollback's roles, log
mode's body), and a color picked for dark can wash out on light.

- A theme can give a palette for each appearance: `[palette.dark]` and
  `[palette.light]` on top of `[palette]`. Tags and roles refer to palette
  names, so they follow along.
- `appearance = "auto"` (the default), `"dark"` or `"light"`. Auto asks
  the terminal for its background (Bubble Tea's `RequestBackgroundColor`,
  OSC 11) and goes by `IsDark`. Some setups may not answer (mosh or tmux,
  to be checked), so the setting can pin it, and with no answer Kiln
  assumes dark.

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

- Old `[[highlight]]` rules keep working. At load time each becomes its
  parts: a classify rule with a generated tag, a `[tags]` entry in its
  world or character, and attention or quiet for that tag. Nobody's
  config breaks.
- The fuzzball pack moves to the new form: classify tags plus
  `attention = ["page/in", "whisper/in", "self"]`. Its colors move to the
  default theme's `[tags]`.
- `/highlight <text>` adds a classify rule with tag `highlight` to the
  world. The theme styles `highlight` once, so every highlight follows the
  theme. (The sidebar's attention dot borrows that color today; it gets a
  role of its own.)
- `config.HighlightStyle` goes away.

## Phases (child issues)

1. **Theme engine and chrome roles.** Theme files, palette, fallback,
   `extends`, live reload. Every hard-coded style becomes a role, drawn by
   the area helpers. The modal backdrop comes with it. The default theme reproduces today's look closely,
   with tests pinning it, so the plumbing lands without a visual change.
2. **The new default look.** Indi's redesign, on top of phase 1.
3. **Tags, behavior and migration.** `[tags]`, world and character theme
   tables, `span`,
   the attention/quiet lists, the `[[highlight]]` shim, the fuzzball pack,
   and `/highlight`.
4. **Choosing themes.** The inheritable `theme` setting, light and dark
   (`appearance`, palettes per appearance), and restyling on change.
5. **Later:** importing palettes from other tools (base16/base24 YAML,
   Ghostty and kitty theme files), and maybe ANSI remapping.

## Open questions

- **A shared tag vocabulary.** A theme that styles `page/in` assumes the
  pack emits it. Packs should agree on standard tags (say, `pose`, `say`,
  `page`, `whisper`, `self`, `sys`), so themes carry between worlds. A
  world's own special tags are styled in the world (see Choosing a theme).

## Decided against

- **A default style for plain server text.** Server text keeps the
  terminal's foreground; a fixed one would fight light and dark.
- **A scrollback background.** The rule stands: server text sits on the
  terminal's background.
