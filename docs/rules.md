# Rules

> **AI-generated.** Claude wrote this page from Kiln's code and earlier README. If something here is wrong, please [open an issue](https://github.com/latrani/Kiln/issues).

Three things decide what happens to a line from the server: **classify rules** say what it is (they give it tags), the **theme** says how each tag looks, and the **`attention` and `quiet` lists** say how it behaves.

```toml
attention = ["page/in", "whisper/in", "self"]   # light up the ● badge
quiet = ["wiki"]                                 # never unread, never notifies

[[classify]]
tag = "ooc"
pattern = '^\[OOC\]'

[[classify]]
tag = "wiki"
pattern = '^\[Wiki\]'
```

A classify rule can give several tags at once with `tags = ["page", "page/in"]` (instead of, or as well as, `tag`). By convention a `/` nests a tag under a broader one: the starter pack tags pages you receive `page` and `page/in`, and the server's echo of your own (`You page, …`, `You page-pose, …`) `page` and `page/out`, so a `page` filter shows the whole conversation while only `page/in` asks for attention. Whispers work the same way (`whisper/in`, and `whisper/out` for `You whisper, …`). Kiln itself tags lines that mention your character's name or aliases `self`.

`attention` and `quiet` add up along the chain (defaults, packs, world, character), and an entry covers the tags under it: `attention = ["page"]` covers `page/in` too (but not `pages`). Quiet lines are still shown, but don't count as unread, don't bump the `▼ new` count, never light the badge and never notify, even if another tag (like `self`) asks for attention.

Tags are worked out when lines are shown, never saved. Fixing a rule fixes old logs too. A character can add rules of its own with `[[characters.classify]]` right after its `[[characters]]` entry, and `attention` and `quiet` lists inside it.

How tags look lives in the theme's `[tags]` (see [Themes](themes.md)). A world or character can add to it with its own `[palette]` (with `[palette.light]` and `[palette.dark]` too) and `[tags]`, over the theme it uses:

```toml
# worlds/fm.toml
[palette]
beacon = "#ffd166"          # a color only this world uses

[tags]
"highlight" = { fg = "beacon", bold = true, scope = "match" }
"ooc"       = { fg = "#808080" }
```

A character's own looks go inside its `[[characters]]` entry, as `[characters.tags]` and `[characters.palette]` (or inline, `tags = { … }`). A bare `[tags]` header after a `[[characters]]` entry still means the world's, since TOML table headers are absolute:

```toml
[[characters]]
name = "Kit"

[characters.tags]
"self" = { fg = "#ffd166", bold = true }   # just Kit
```

A tag style takes the same settings as a theme role, plus `scope`. By default it styles the whole line; with `scope = "match"` it styles only the text the tag's classify rules matched, so a server that prefixes pages with `PAGE:` can color just the prefix. A tag inherits from the one up its slashes, the way a role does up its dots: `page/in` starts from `page` and changes only what it sets, and `fg = "default"` clears a color it would inherit. When a line has several tags, whole-line styles go first and match styles on top; later colors win and attributes add up.

`/highlight <text>` adds a classify rule to the world's file that gives every line containing *text* the tag `highlight`. The theme styles that tag once for all your highlights: by default, just the matching text, in bold yellow.

## Core tags

Themes can count on these tags; packs emit them where their server makes that possible, and are free to add their own.

| Tag | Lines |
|---|---|
| `page`, `page/in`, `page/out` | pages to you, and your own |
| `whisper`, `whisper/in`, `whisper/out` | whispers to you, and your own |
| `watchfor`, `watchfor/connect`, `watchfor/disconnect` | watchfor notices of people connecting and disconnecting |
| `ooc` | out-of-character talk |
| `system` | messages from the MUCK itself (not Kiln's own `*` lines) |
| `self` | lines that mention you (Kiln adds these) |
| `highlight` | lines matching a `/highlight` |

`say` and `pose` aren't core: on a MUCK they're the bulk of ordinary scene text, not something to set apart.
