# Themes

> **AI-generated.** Claude wrote this page from Kiln's code and earlier README. If something here is wrong, please [open an issue](https://github.com/latrani/Kiln/issues).

Kiln's colors come from a theme. Kiln makes `themes/default.toml` in its config folder on first run, saying `extends = "kiln"`: it starts as the built-in theme, `kiln`, and changes only what you set. Kiln picks up changes as you save. `kiln theme show kiln` prints everything the built-in sets, and `kiln theme show <name>` prints any theme with what it extends merged in. To keep several themes, give each its own file in `themes/` and pick one with `theme = "name"` in `config.toml`.

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

`[ui]` styles Kiln's own parts of the screen, by role; `[tags]` styles lines from the server, by their [tags](rules.md). A style sets any of `fg`, `bg`, `bold`, `faint`, `italic`, `underline` and `reverse`. A color is `#rrggbb`, a name from `[palette]`, one of the terminal's own sixteen (`red`, `bright-blue`, and so on), or `default` for the terminal's own color, which clears one a role would inherit. A tag style can also say `scope = "match"` (see [Rules](rules.md)). A role inherits from the one before its last dot (`link.hover` starts from `link`). Text from the server always sits on your terminal's background; the sidebar (the picker or filter panel while one is open), input box (a form while one is up), log mode's hint row, and the two status bars (top and bottom, both `status`) can have their own. If a theme has a mistake, Kiln says so in the statusline and keeps the colors it had.

A theme can give different colors for light and dark terminals: `[palette.light]` and `[palette.dark]` replace entries of `[palette]`, and everything that names them follows along.

    [palette]
    ember = "#ff9f43"

    [palette.light]
    ember = "#b35900"        # darker, to read on a light background

With `appearance = "auto"` (the default), Kiln asks the terminal what its background is when it starts and whenever its window comes back into focus, and assumes dark until it hears back. The built-in theme has a light palette of its own. Over mosh the terminal can't be asked (eternal terminal and tmux pass the question on), so on a light terminal set `appearance = "light"`.

The roles are: `sidebar` (`.world`, `.char`, `.active`, `.unread`, `.attention`, `.connecting`, `.disconnected`, `.add`, `.more`), `picker` (`.world`, `.world.selected`, `.selected`, `.add`), `divider`, `rule` (`.input`, `.form`: the rule above that area; `.status`: the rules that border the status bars, under the top one and above the bottom one), `scrollback` (`.day`, `.history_end`, `.loading`, `.echo`, `.sys`, `.pill`, `.selection`, `.inactive`, `.empty`, `.overview`), `link` (`.hover`), `input` (`.hint`, `.over_limit`, `.selection`), `status` (`.log`, `.log.on`, `.filter`, `.filter.on`, `.error`, `.presence` with `.here`, `.away` and `.unknown`), `form` (`.label`, `.hint`, `.error`, `.focus`, `.title`, `.button`, `.button.secondary`), `log` (`.time`, `.cursor`, `.selected`, `.excluded`, `.find`, `.day`, `.loading`, `.bar`, `.bar.hints`), `filter` (`.item`, `.selected`, `.button`, `.button.on`, `.add`, `.more`), and `export` (the HTML export's page).
