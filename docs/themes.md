# Themes

> **AI-generated.** Claude wrote this page from Kiln's code and earlier README. If something here is wrong, please [open an issue](https://github.com/latrani/Kiln/issues).

Kiln's colors come from a theme. Kiln makes `themes/default.toml` in its config folder on first run, saying `extends = "kiln"`: it starts as the built-in theme, `kiln`, and changes only what you set. Kiln picks up changes as you save. `kiln theme show kiln` prints everything the built-in sets, and `kiln theme show <name>` prints any theme with what it extends merged in, along with every [color name](#color-names) it can use. To keep several themes, give each its own file in `themes/` and pick one with `theme = "name"` in `config.toml`.

    extends = "kiln"

    [palette]
    panel = "#1f2029"

    [ui]
    "sidebar"        = { bg = "panel" }
    "sidebar.active" = { reverse = true }
    "status"         = { bg = "panel" }
    "status.error"   = { fg = "#ff6b6b", bold = true }

    [tags]
    "page/in"   = { fg = "#ff9f43", bold = true }
    "highlight" = { fg = "#ffd166", scope = "match" }

`[ui]` styles Kiln's own parts of the screen, by role; `[tags]` styles the lines in the scrollback and log mode (the server's, yours and Kiln's own `*` lines) by their [tags](rules.md). The built-in theme styles only Kiln's own tags and `/highlight`'s (`self`, `sent`, `kiln`, `highlight`); packs style theirs, and your theme can restyle any of them. A style sets any of `fg`, `bg`, `bold`, `faint`, `italic`, `underline` and `reverse`. A color is `#rrggbb`, a name from `[palette]`, one of the terminal's own sixteen (`red`, `bright-blue`, and so on), or `default` for the terminal's own color, which clears one a role would inherit. A tag style can also say `scope = "match"` (see [Rules](rules.md)). A role inherits from the one before its last dot (`link.hover` starts from `link`). Text from the server always sits on your terminal's background; the sidebar (the picker while it's open, the filter panel in log mode), input box (a form while one is up), log mode's hint row, and the two status bars (top and bottom, both `status`) can have their own. If a theme has a mistake, Kiln says so in the statusline and keeps the colors it had.

A theme can give different colors for light and dark terminals: `[palette.light]` and `[palette.dark]` replace entries of `[palette]`, and everything that names them follows along.

    [palette]
    orange = "#ff9f43"

    [palette.light]
    orange = "#b35900"       # darker, to read on a light background

## Color names

Every theme can name these, whatever it extends (even nothing), and so can packs, worlds and characters. Each switches with the background: the first value on a dark one, the second on a light one.

Catppuccin's accents, from its Macchiato (dark) and Latte (light) flavors:

| Name | Dark | Light |
|---|---|---|
| `catppuccin.rosewater` | `#f4dbd6` | `#dc8a78` |
| `catppuccin.flamingo` | `#f0c6c6` | `#dd7878` |
| `catppuccin.pink` | `#f5bde6` | `#ea76cb` |
| `catppuccin.mauve` | `#c6a0f6` | `#8839ef` |
| `catppuccin.red` | `#ed8796` | `#d20f39` |
| `catppuccin.maroon` | `#ee99a0` | `#e64553` |
| `catppuccin.peach` | `#f5a97f` | `#fe640b` |
| `catppuccin.yellow` | `#eed49f` | `#df8e1d` |
| `catppuccin.green` | `#a6da95` | `#40a02b` |
| `catppuccin.teal` | `#8bd5ca` | `#179299` |
| `catppuccin.sky` | `#91d7e3` | `#04a5e5` |
| `catppuccin.sapphire` | `#7dc4e4` | `#209fb5` |
| `catppuccin.blue` | `#8aadf4` | `#1e66f5` |
| `catppuccin.lavender` | `#b7bdf8` | `#7287fd` |

Kiln's glazes, which its own parts of the screen use: `glaze.celadon-*` (the sidebar), `glaze.cobalt-*` (the status bars), `glaze.gold-*` (the input) and `glaze.kaki-*` (forms), each in `-deep`, `-mid`, `-line`, `-ink` and `-bright` (gold has `-mid`, `-line` and `-ink`), and `glaze.alarm` and `glaze.alarm-deep` for errors. `kiln theme show kiln` prints their values.

Names without a dot are the terminal's own sixteen (`red`, `bright-blue`) and whatever you define in a `[palette]`. You can redefine a shipped name too: `catppuccin.mauve = "#b48ead"` in a `[palette]`.

Catppuccin's colors are MIT licensed ([catppuccin/palette](https://github.com/catppuccin/palette)).

With `appearance = "auto"` (the default), Kiln asks the terminal what its background is when it starts and whenever its window comes back into focus, and assumes dark until it hears back. The built-in theme has a light palette of its own. Over mosh the terminal can't be asked (eternal terminal and tmux pass the question on), so on a light terminal set `appearance = "light"`.

The roles are: `sidebar` (`.world`, `.char`, `.active`, `.unread`, `.priority`, `.connecting`, `.disconnected`, `.add`, `.action` (unused now; kept so themes that set it still load), `.more`, `.title`: its first row, `.collapse`: the `‹` at its right), `picker` (`.world`, `.world.selected`, `.selected`, `.add`: also a character's name while you add one, `.error`: under that name, why it won't do, `.title`: its list's first row, `.collapse`), `divider`, `rule` (`.input`, `.form`: the rule above that area; `.filter`: under the filter panel's title and above its footer; `.sidebar`, `.picker`: under that list's title; `.status`: the rules that border the status bars, under the top one and above the bottom one), `scrollback` (`.day`, `.history_end`, `.loading`, `.sys`: the world overview's time since a character's last line, and the command-line client's notices, `.pill`, `.selection`, `.inactive`, `.empty`, `.overview`, `.rule`), `link` (`.hover`), `input` (`.hint`, `.over_limit`, `.selection`, and the history panel's `.history` with `.selected`, `.time`, `.cursor`: the selected line's time, and `.more`), `status` (`.log`, `.log.on`, `.menu`: the collapsed sidebar's `≡` at the top bar's left, `.history`, `.history.on`, `.scope`, `.scope.on`: the Session/Character switch beside History while its panel is up, `.select`, `.select.on`: log mode's Select chip, `.error`, `.presence` with `.here`, `.away` and `.unknown`), `form` (`.label`, `.hint`, `.error`, `.focus`, `.title`, `.button`, `.button.secondary`), `log` (`.time`, `.cursor` with `.away`: the ▲ or ▼ by the time while the cursor is scrolled out of view, `.selected`, `.excluded`, `.find`, `.day`, `.loading`, `.bar`, `.bar.hints` with `.away`: while the filter panel has focus), `filter` (`.item`, `.selected`, `.button`, `.button.on`, `.add`, `.more`, `.tree`: the lines from a parent to its children, `.title`, `.collapse`, `.hints`: its footer's keys, with `.away`: while the log has focus), and `export` (the HTML export's page).
