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
