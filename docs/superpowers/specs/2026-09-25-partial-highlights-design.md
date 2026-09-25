# Partial highlights

## Goal

A highlight rule can style just the part of a line that matched, not the
whole line. Example: a pack classifies `^PAGE:` as `page` and wants only
`PAGE:` colored, with the rest of the page in the server's own colors.
The same mechanism highlights just your name when a line mentions you.

Prior art: TinyFugue's `/def -P` applies attributes to part of a line.
It works on every match on the line, and overlapping partial attributes
combine.

## Config

Highlight rules get an optional `scope`:

```toml
[[highlight]]
match = { tags = ["page"] }
style = { fg = "#2053ff", bold = true }
scope = "match"        # "line" (default) or "match"
attention = true
```

- `scope = "line"` (or no `scope`): today's behavior; the whole line is
  styled.
- `scope = "match"`: only the rule's **spans** are styled (see below).
- Any other value is a config error:
  `highlight rule N: scope must be "line" or "match"`.
- `attention` is always per line, whatever the scope.
- Existing configs behave exactly as before. Capture groups in patterns
  mean nothing special.

## Spans

A span is a byte range of the line's plain (ANSI-stripped) text.

- **Classify records spans.** A tag's spans are every non-empty match of
  every classify rule for that tag (all matches on the line, not just
  the first). A tag is on the line if any of its rules matched, as
  today.
- **`self` spans** are each place a name or alias appears, and cover just
  the name, not the word-boundary characters around it. Back-to-back
  mentions (`Kit, Kit!`) are each found.
- **A highlight rule's spans**, for `scope = "match"`:
  - with a `pattern`: every non-empty match of that pattern (whether or
    not it also has `tags`; the pattern is the more specific of the two);
  - with only `tags`: the union of the spans of the rule's tags that are
    on the line.
- A rule applies to a line exactly as today: its tags (if any) are on
  the line and its pattern (if any) matches. A `scope = "match"` rule that
  applies but ends up with no non-empty spans styles nothing.

## How styles combine

For each character of the line, fold the rules that apply to it, in rule
order: whole-line rules apply to every character; match-scope rules to
the characters inside their spans. The fold is today's: later non-empty
colors win; bold, italic and underline accumulate. Overlapping spans
from several rules therefore combine, as in TinyFugue.

## Drawing

The styled result is drawn over the server's own colors, as today:

- Where our style applies, its SGR is emitted and re-emitted after any
  server reset inside it, so a server reset can't cut our style short.
- Where a styled range ends mid-line, the server's own state at that
  point is restored: a reset, then the server's SGR sequences since its
  last reset.
- Unstyled parts of the line are left exactly as the server sent them.

This applies wherever highlights are drawn: the scrollback, browse
mode, and `kiln tail`. Scene exports don't use highlights and are
unchanged.

## Code shape

- `classify.Classify` returns tags with their spans
  (`[]classify.Tag{Name string; Spans []Span}`, `Span{Start, End int}`),
  plus a helper for callers that want only names (browse mode's tag
  filters).
- `rules.Highlighter.Apply` takes those tags and returns the line style,
  the per-range styles for match-scope rules, and attention.
- One drawing function (in `style`) turns the server text and that result
  into the styled string. `renderLine` in the UI and `kiln tail`'s render
  both call it.

## Testing

- Config: `scope` accepted as `line`/`match`/absent; anything else is an
  error naming the rule.
- Classify: spans for every match of every rule of a tag; `self` spans
  cover only the name; back-to-back mentions; zero-width matches ignored.
- Highlight: line vs match scope; pattern spans vs tag spans; overlapping
  match-scope rules combine (color override, attributes accumulate);
  a match-scope rule over a line-scope one; attention stays per line.
- Drawing: a styled prefix (`PAGE:`) followed by server-colored text
  restores the server's color; a server reset inside a styled span doesn't
  end it; an unstyled line is returned untouched; multi-byte text
  (`Zoë`) spans land on the right characters.
- README: `scope` in the highlight docs, with the `PAGE:` example and a
  name-only `self` example.
