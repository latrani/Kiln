# Kiln

## User-facing strings

Every string a person reads (UI text, status and sys lines, errors, CLI
output) lives in `internal/str/locales/en.toml`, not in Go source.

- Add or edit the entry in `en.toml`, run `go generate ./internal/str`, and
  call the generated function: `[browse] saved = "saved {path}"` becomes
  `str.BrowseSaved(path)`.
- Placeholders are `{name}` or `{name:%q}`. `{n}` is always an `int` and
  `{err}` is always an `error`; everything else is `any`. Arguments go in
  the order the generated doc comment shows. For an existing entry that
  order is pinned, so rewording a template never reorders parameters.
- Counts use plural tables: `copied = { one = "copied 1 line", other = "copied {n} lines" }`.
- Wrap an error you're describing with `str.Wrap(msg, err)` so `errors.Is`
  still works.
- `go test ./internal/str` fails on stray literals. Mark a literal
  `//str:ok` only when it isn't for people to read: protocol tokens, file
  formats, regexps, config vocabulary.
- The lint can't tell a displayed single lowercase word ("on", "none") from
  a config value. Put displayed ones in the catalog anyway.
- Config values (`batch`, `pin`, `keychain`, …) and slash commands are
  vocabulary. They stay verbatim and are never translated.
- A commit that adds or changes entries in `internal/str/locales/` ends with
  a `Strings:` trailer listing the keys, e.g.
  `Strings: browse.saved, browse.copied (new)`. The PR description repeats
  the list and says briefly why each string exists.
- Tests build expected text from the catalog too (`str.StatusCopied()`,
  `str.Separator()`, labels like `maxBytesLabel`), never a copy of it, so
  rewording a string never breaks a test. For part of a message, fill a
  placeholder with `mark` and cut with `upTo`/`after` (in each package's
  `catalog_test.go`). `TestTestsReadTheCatalog` flags copied phrases.

## Colors and styles

Every color and attribute Kiln draws comes from a theme role
(`internal/theme/roles.go`), never an escape code in the UI:
`theme.Paint(theme.StatusError, msg)`. A new kind of thing on screen gets
a new role, added to `roles.go` (`Roles` too), the built-in
`internal/theme/default.toml`, and the role list in `docs/themes.md`.
`TestNoHardCodedStyles` fails on escape codes in `internal/ui` and
`cmd/kiln`. Painted areas (sidebar, input, statusline, log header and
action bar) go through `theme.Fill`. The scrollback and log body never do:
server text sits on the terminal's background. The golden screens in
`internal/ui/testdata/golden` pin the built-in look; update them with
`-update` only when a change to the look is intended, and say so in the
commit.

## The web build

The web page (`web/ui`) copies the TUI: its layout, its look in the
built-in theme, and its themes. Differ from the TUI only where the
browser forces it, and say why. Where the browser has its own way of
doing something (editing text, selecting, copying), use the browser's
rather than mimicking the TUI's.

## Breaking changes

Kiln is below 1.0. A change to config, a command, a key or an API breaks
cleanly: no aliases for old names, no migrations, no "renamed to" hints.

## Names

The repo never uses real people's or real characters' names. In tests,
docs and examples, the characters are Kit, Rook, Ash, Mira and Sable,
and the world is `fm`.

## Trying Kiln

Run Kiln against a scratch config (`XDG_CONFIG_HOME` and
`XDG_DATA_HOME` pointed at a temporary directory), never your own setup.

If a tool you need isn't installed, ask for it to be installed with
Homebrew rather than building a workaround.

## The README

The README is written in the maintainer's voice. Fix typos only: no
rewording, and no tightening for precision.
