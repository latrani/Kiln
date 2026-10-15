# Color names every theme has

## Why

Color names are a contract: packs, worlds and themes name colors
(`page` is `ember`), and a name has to mean something wherever it's
used. Today a name exists only in the theme that defines it, so a
theme with no `extends` loses Kiln's names and every pack color that
used them. And Kiln's names are its own: nobody outside Kiln knows what
`ember` looks like.

So: a person can write `catppuccin.mauve` or `catppuccin.sky` in any
theme, world or pack and get Catppuccin's color, Macchiato on a dark
background and Latte on a light one. Kiln's chrome keeps its own
colors, the glazes, and those names (`glaze.celadon-ink`) are always
there too.

## Names

Shipped palettes are namespaced by their file: `catppuccin.toml` writes
`mauve` and it loads as `catppuccin.mauve`; `glaze.toml`'s
`celadon-ink` is `glaze.celadon-ink`. Bare names are the terminal's
sixteen (`red`, `bright-blue`) and whatever a theme, pack, world or
character defines itself. Namespaces keep Catppuccin's `red`, `green`,
`yellow` and `blue` from colliding with the terminal's, and leave room
for more shipped palettes. A file can still redefine a namespaced name
(`catppuccin.mauve = "#…"`: TOML reads the dotted key as a table, and
any `[palette]` table but `light` and `dark` is a namespace), but
restyling a tag is the usual way.

## Layers

Lowest first:

1. `catppuccin.toml` (embedded): Catppuccin's fourteen accents.
2. `glaze.toml` (embedded): Kiln's glazes and `alarm`.
3. Packs, in `use` order.
4. The theme: the built-in, or the person's (with what it extends).
5. The world, then the character.

Each layer can redefine any name a lower one gave, as palettes already
merge. The two base files sit under **every** theme, including one with
no `extends`. So the palette is vocabulary, always there, and `extends`
is about styling: a theme with no `extends` starts with no roles or tag
styles but keeps every color name.

## `catppuccin.toml`

- `[palette]` is Macchiato's accents and `[palette.light]` Latte's,
  from catppuccin/palette's `palette.json`: `rosewater`, `flamingo`,
  `pink`, `mauve`, `red`, `maroon`, `peach`, `yellow`, `green`, `teal`,
  `sky`, `sapphire`, `blue`, `lavender`, each `catppuccin.` once loaded. Only the accents: Catppuccin's
  text, surface and base shades assume its own background, and in the
  terminal server text sits on the terminal's.
- `[terminal]` (and `[terminal.light]`) is the web's stand-in for a
  terminal's colors, which the browser doesn't have. It holds only what
  isn't an accent, as hex, and names accents for the rest:

  ```toml
  [terminal]
  fg = "#cad3f5"            # text
  bg = "#24273a"            # base
  black = "#494d64"         # surface1
  white = "#b8c0e0"         # subtext1
  bright-black = "#5b6078"  # surface2
  bright-white = "#a5adcb"  # subtext0
  red = "catppuccin.red"
  green = "catppuccin.green"
  yellow = "catppuccin.yellow"
  blue = "catppuccin.blue"
  magenta = "catppuccin.pink"
  cyan = "catppuccin.teal"
  ```

  The brights 9–14 are the normals again, as in Catppuccin's own
  terminal ports. `[terminal]` names aren't palette names: nothing
  else can use them.
- A header comment names the source and points at
  `catppuccin.LICENSE`, Catppuccin's MIT license verbatim, which sits
  beside it and goes into the release archives with `LICENSE`
  (`.goreleaser.yaml`).

## `glaze.toml`

The glazes (`celadon-*`, `cobalt-*`, `gold-*`, `kaki-*`) and `alarm`,
dark and light, moved out of `internal/theme/default.toml` unchanged.

## The built-in theme

`internal/theme/default.toml` keeps only `[ui]` and `[tags]`:

- `ember`, `lavender` and `beacon` are gone (Catppuccin's `lavender` is
  a periwinkle; ours was a purple).
- `highlight`, `sidebar.priority` and `status.presence.away`: `beacon`
  → `catppuccin.yellow`.
- The fuzzball pack: `page` → `catppuccin.peach`, `whisper` →
  `catppuccin.mauve`.
- Every glaze and `alarm` it names gains `glaze.`.

The look changes a little on purpose (Indi recolors the defaults soon
anyway); the TUI goldens are updated, with the change named in the
commit.

## The web

`go generate` writes `web/ui/src/terminal.gen.ts` from `[terminal]`
with the accents resolved, beside the catalog's `str.gen.ts`, and
`theme.ts`'s hand copy of Macchiato and Latte goes. The page needs
these before Go runs (loading, another tab has Kiln), which is why
they're generated rather than sent in the `Theme` event. Tag and role
colors already reach the page resolved, so the new names need nothing
there.

## Finding the names

- `docs/themes.md` lists the fourteen with their dark and light values,
  and the glaze names, and says every theme has them.
- `kiln theme show <name>` prints the base palettes as part of every
  theme.

## Breaking

Pre-1.0, a clean break:

- A file naming `ember`, `lavender`, `beacon`, or a glaze or `alarm`
  without `glaze.`, gets the usual "isn't a color" error; `Best` keeps
  everything else that resolves.
- An existing `packs/fuzzball.toml` names `ember` and `lavender`: its
  pages and whispers lose their color until it's
  deleted (Kiln writes the new one) or, on the web, `/reset`.
- A theme with no `extends` now has every color name; nothing it
  defined itself changes.

## Testing

- Every theme, including one with no `extends`, resolves all fourteen
  `catppuccin.` names and the `glaze.` names, dark and light, to the
  files' values; bare `red` is still the terminal's.
- A theme, pack, world or character can redefine `catppuccin.mauve`;
  the later layer wins.
- `[terminal]`: the generated file matches the TOML (accents resolved,
  brights equal to normals), and `go generate` leaves no diff.
- `kiln theme show` lists the base names.
- The fuzzball pack's lines are `peach` and `mauve`; the built-in
  theme has no `ember`, `lavender` or `beacon`.
- TUI goldens: `-update`d once, the diff looked over.
