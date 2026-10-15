# Packs carry their tags' looks

## Why

A pack says what a server's lines are; until now only the theme could
say how they look. So the built-in theme styles `page` and `whisper`,
tags that exist only because the fuzzball pack emits them, and a pack
for a server that calls its pages something else (`tell`) can't give
them a look at all. Tags are a pack's vocabulary, not Kiln's: there are
no server tags every theme can count on.

## What changes

- **A pack file takes looks**: `[palette]`, `[palette.light]`,
  `[palette.dark]` and `[tags]`, with the same syntax and checks as a
  world file's (`docs/rules.md`). Colors are `#rrggbb`, a palette name
  (the pack's own or the theme's), a terminal color, or `default`.
- **Order**, lowest first: packs (in `use` order) → the theme (the
  built-in, then the person's, which extends it) → the world → the
  character. Each layer overrides the one before, field by field, as
  world and character looks already do. So a person's theme can still
  restyle `page` once for every world, and a world or character file
  can still restyle it for itself.
- **Palette names merge in the same order.** A pack's `ember` is used
  unless the theme, world or character defines `ember` too; then
  theirs wins. A theme that redefines a name a pack uses is restyling
  that pack, on purpose or not.
- **The fuzzball pack gets the looks** the built-in theme has now:
  `page` (`ember`, bold), `whisper` (`lavender`, italic), with `ember`
  and `lavender` in its `[palette]` and their light variants in
  `[palette.light]`. Lines look the same as before, so the TUI
  goldens don't change.
- **`watchfor` gets a look in the fuzzball pack.** Indi picks it.
- **The built-in theme keeps only Kiln's own tags**: `self` (bold) and
  `highlight` (`beacon`, bold, match). Kiln emits these itself, so the
  theme is the right home. `page`, `whisper`, `ember` and `lavender`
  leave it.

## Where it lands

- `internal/config`: `Rules` (or a sibling) gets the pack's palette and
  tags; `loadPack` parses them as a `theme.Layer`. A character's looks
  become two lists, below and above the theme, or one list with the
  theme's place marked.
- `internal/theme`: putting layers *under* a theme is new; today
  `With` puts them on top. It needs a way to resolve pack layers, then
  the theme, then the world and character layers, with palette names
  resolving across all of them.
- `internal/webapi/theme.go`: a character whose pack has looks has its
  own tag table (`Looks.Chars`), as one with world looks does now. Most
  characters will have one; the base table is the theme's alone.
- `internal/ui`: the TUI's highlighter per character, the same way.
- Docs: `docs/rules.md` ("Core tags" becomes Kiln's own tags, `self`
  and `highlight`; a pack section on looks), `docs/themes.md` (the
  built-in theme styles only Kiln's own tags; themes can still style
  any tag), the starter `themes/default.toml` comments.

## Breaking

Pre-1.0, a clean break. Starter files are written only where none
exist, so anyone with an old `packs/fuzzball.toml` keeps a pack with no
looks, and with the built-in theme no longer styling them, their pages
and whispers lose their color. They get it back by deleting the old
pack (Kiln writes the new one) or, on the web, with `/reset`. The
release notes say so.

A person's theme that names `ember` or `lavender` without defining them
no longer loads: those names left the built-in palette, and a theme is
loaded before any pack's looks go under it. The starter theme's
examples name only the built-in's colors. The release notes say this
too.

## Not changing

- Classify rules, `priority` and `quiet`: as they are.
- Tag inheritance up slashes (`page/in` starts from `page`) and
  `scope = "match"`: as they are, at every layer.
- `self` and `highlight` stay tags.

## Testing

- A pack's `[tags]` and `[palette]` style its lines; light variants
  apply on a light background.
- Order: a theme's, world's and character's `[tags]` each beat a
  pack's; a theme's palette name beats a pack's of the same name.
- A bad color or style in a pack fails loading with the pack's file
  named, as a world file's does.
- The fuzzball pack's lines look as before (TUI goldens unchanged; the
  web's `Theme` event carries the same looks per character).
- The built-in theme has no `[tags]` but `self` and `highlight`.
