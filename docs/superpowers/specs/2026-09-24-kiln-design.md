# Kiln — Design Spec

**Date:** 2026-09-24
**Status:** Draft for review

## 1. Purpose

Kiln is a modern terminal MUCK client in the spirit of TinyFugue, with first-class unicode, mouse, and truecolor support.

**Audience:** first the author, then the broader MUCK community (FurryMUCK-style social/RP play: poses, pages, whispers, many characters across many worlds). Easy installation matters for the second audience.

**Explicitly not for:** combat-oriented MUD play. GMCP/MSDP and similar structured-data protocols are out of scope; this is a deliberate differentiator from Mudlet, Blightmud, etc.

### Success criteria (v1)

- Connect to multiple worlds, with multiple characters per world, simultaneously.
- Characters inherit their world's settings, and the UI groups them under that world.
- Semantic classification of lines (page, whisper, pose, …) drives highlighting and attention.
- Every session is logged to plain, greppable text, and any past range can be retroactively selected and exported as a scene.
- Ships as a single static binary per platform.

## 2. Platform

- **Language:** Go.
- **TUI:** Bubble Tea v2 + Lip Gloss (Charm). v2 is needed for the kitty keyboard protocol (Shift+Enter) and its improved mouse handling.
- **Keychain:** `zalando/go-keyring`.
- **Distribution:** static binaries per OS/arch.

## 3. Architecture

Packages, one responsibility each:

| Package | Responsibility |
|---|---|
| `conn` | TCP/TLS; telnet negotiation (NAWS, CHARSET → UTF-8, keepalive); swallow MCP `#$#` lines; reconnect with backoff. Emits raw decoded lines. |
| `session` | One per character. Owns a `conn`, performs auto-login, and tracks connection state. |
| `ansi` | Raw text → styled spans. Measures unicode width correctly (wide CJK, emoji, combining marks). |
| `classify` | Applies a character's effective classifier rules to a line → set of semantic tags. Pure. |
| `rules` | Tags + patterns → actions (style, attention). Pure. |
| `logstore` | Appends lines to per-character day files, and reads them back (forward and in reverse by day) for scrollback paging. |
| `config` | Loads the config directory, merges inheritance, hot-reloads, and writes back in-client edits. |
| `ui` | Bubble Tea app: sidebar, scrollback (incl. browse mode), input box, statusline. |

### Data flow (inbound)

```
conn goroutine → session → logstore.Append(raw) → program.Send(LineMsg) → ui
                                                                   ↓
                                             classify → rules → render (lazy, visible lines only)
```

- The line is logged **before** it reaches the UI, so a UI failure never loses log data.
- Everything downstream of `session` is a pure function over lines and can be tested without a network.

### Data flow (outbound)

Input → split per newline mode → per-line byte check → `session.Send` → `logstore.Append` (dir `>`, with login lines redacted).

## 4. Configuration

TOML has no include mechanism, so config is organized by directory convention:

```
~/.config/kiln/
  config.toml          # [defaults] + global UI prefs
  worlds/
    <world-id>.toml    # one world + its characters; filename is the world id
  packs/
    <pack-id>.toml     # reusable classify/highlight rule sets
```

### `config.toml`

```toml
[defaults]
max_line_bytes = 2047             # Fuzzball: MAX_COMMAND_LEN 2048 incl. NUL; excess is silently truncated
newline_mode = "batch"          # "batch" | "flatten"
```

### `worlds/furrymuck.toml`

```toml
host = "furrymuck.com"
port = 8899
tls = true
tls_trust = "pin"               # "pin" (default, trust-on-first-use) | "ca"
login = "connect {name} {password}"   # inherited by every character
use = ["fuzzball"]              # packs, applied in order

[[classify]]
tag = "page"
pattern = '^(\w+) pages?: (.*)$'

[[highlight]]
match = { tags = ["page"] }     # and/or pattern = '…'
style = { fg = "#ff9f43", bold = true }
attention = true

[characters.kit]                 # the key is only an id (used for paths and the keychain)
name = "Kit"                     # required: the canonical in-game name
aliases = ["Kitty"]              # optional: other names that count as "me"
# {name} and {password} (from the OS keychain) are the login template's variables
# characters may add their own [[characters.kit.classify]] / [[characters.kit.highlight]]
```

### `packs/fuzzball.toml`

Contains only `[[classify]]` and `[[highlight]]` arrays. A pack is the unit people share with each other.

### Character identity

- `name` and `aliases` feed a built-in `self` tag. A line that mentions the character's own name is tagged `self` without the user writing a rule, so highlight rules can match on `tags = ["self"]`.
- Name matching is case-insensitive and respects word boundaries.

### Inheritance

`defaults → packs (in `use` order) → world → character`

- **Rule lists** (`classify`, `highlight`) are appended along the chain.
- **Scalars** are overridden by later levels.

### Hot reload and write-back

- The config directory is watched, and changes apply live.
- If a file fails to parse, Kiln keeps the last good config and shows the error in the statusline.
- In-client conveniences (e.g. "highlight selected text") write to the relevant world file.

### TLS trust

- **`pin` (default):** trust-on-first-use. On the first TLS connect, the server certificate's SHA-256 fingerprint is saved to `~/.local/share/kiln/known_hosts`, silently. After that, a matching cert connects normally and a mismatch refuses to connect. Self-signed certs work with no configuration and no warnings.
- **`ca`:** standard CA chain verification, for servers with real certificates.

### Secrets

Passwords are never stored in config. They live in the OS keychain, keyed by world + character. If an entry is missing, Kiln prompts for it with masked input and then offers to save it.

## 5. Logs

**Path:** `~/.local/share/kiln/logs/<world>/<char>/YYYY-MM-DD.log`

**Format:**

```
#kiln-log v1
2026-09-24T21:14:03.120-07:00 <	Rook says, "Evening!"
2026-09-24T21:14:09.844-07:00 <	Mira pages: you around?
2026-09-24T21:14:15.002-07:00 >	:grins.
2026-09-24T21:20:00.000-07:00 *	disconnected (reset); retrying in 5s
```

- Every line has a fixed-width prefix: an RFC 3339 timestamp with milliseconds and a UTC offset, a space, and a direction character, followed by a TAB. The raw text runs from after the first TAB to the end of the line.
- Direction is `<` for received, `>` for sent, `*` for client/system events.
- Raw text is stored UTF-8-decoded with ANSI bytes intact. **Nothing is escaped.** Readers split only on the first TAB, so tabs inside the raw text are safe.
- `less -R` renders the colors, `grep` works directly, and `cut -f2-` yields a pure transcript.
- **Tags are not stored.** Raw text is canonical, and classification always runs on read using the current rules. This means fixing a bad classifier fixes history too.
- Lines sent by auto-login are logged with the password replaced, e.g. `connect Kit ***`, never with the password.
- The `#kiln-log v1` header allows the format to evolve.

## 6. UI

### Layout

```
┌─────────────┬──────────────────────────────────┐
│ ▾ FurryMUCK │ Rook says, "Evening!"            │
│   ● Kit   2 │ Sable waves a paw.               │
│   ○ Rook    │ Mira pages: you around?          │
│ ▾ Tapestries│                                  │
│   ○ Ash     │                        ▼ 12 new  │
│             ├──────────────────────────────────┤
│             │ > :grins, then leans on the      │
│             │   counter.                       │
│             ├──────────────────────────────────┤
│             │ FurryMUCK/Kit 🔒 · connected·21:14│
└─────────────┴──────────────────────────────────┘
```

The sidebar spans the full height of the window and sits at the top of the information hierarchy. It selects *which* character the right pane is showing. Everything else (scrollback, input box, statusline) lives in the right pane and belongs to the selected character.

### Sidebar

- A tree of worlds → characters.
- Clicking a world header collapses it. Clicking a character, or pressing `Ctrl+↑/↓`, switches to that character.
- Badges: an unread count; `●` means an attention-rule hit, `○` means connected with nothing flagged, and `✕` means disconnected.

### Scrollback

- One buffer per character, holding raw strings in memory with **no cap**. ANSI parsing and styling are done lazily, only for visible lines.
- On open, the buffer preloads recent history from disk. Scrolling up past the start of memory keeps paging back through older day files, in reverse date order.
- Wrapping is unicode-width-aware.
- The mouse wheel and PgUp/PgDn scroll. When scrolled up, new output does not move the view; a `▼ N new` pill appears, and clicking it returns to live.

### Browse mode (log browser)

The log browser is a toggle on the scrollback itself, not a separate screen.

- Filter by date range, by tags (classified on read), and by text search.
- Select a range by click-drag, or with `m` to mark the start and end.
- Export the selection as plaintext, ANSI, or HTML, either to a file or to the clipboard via OSC 52.

### Input box

- It grows with its content up to ⅓ of the screen height, then scrolls internally. Long lines soft-wrap visually but are sent as one line.
- Newline is Shift+Enter (via the kitty keyboard protocol), with Alt+Enter as a fallback. Enter sends.
- **Newline mode** (configurable per world or character):
  - `batch` (default): each hard line is sent as a separate command.
  - `flatten`: newlines are replaced with spaces and sent as one line.
- **Buffer-bust guard:** each line's UTF-8 **byte** length is checked against `max_line_bytes`. There is no counter. Text past the limit is highlighted red *from the exact cut point*, so you can see where the server would truncate and split the line there with a newline (which `batch` mode sends as separate commands). Sending while any line is over the limit requires confirmation.
- Drafts and history are kept per character. Switching characters preserves the half-written draft with its character.

### Statusline

Shows the active world/character, a TLS indicator (🔒), the connection state, a browse-mode/selection indicator, and a clock.

## 7. Protocol scope (v1)

- **In:** TLS with cert pinning; telnet NAWS, CHARSET (UTF-8), and keepalive; auto-login; auto-reconnect with backoff (1s → 60s cap) plus a manual `/reconnect` that skips the wait; MCP minimal support, meaning `#$#` lines are swallowed and not displayed.
- **Out:** GMCP, MSDP, MXP, full MCP packages (e.g. `simpleedit`).

## 8. Error handling

The principle is: never crash, always surface the problem.

| Condition | Behavior |
|---|---|
| Connection drop | A `*` sys line, a `✕` badge, and backoff reconnect |
| Pinned cert mismatch | Connection refused with the old and new fingerprints shown; `/trust` accepts the new cert and re-pins it |
| Config parse error | Keep the last good config and show the error in the statusline |
| Log write failure | A persistent statusline warning; the session continues |
| Missing keychain entry | Masked password prompt, then an offer to save it |

## 9. Testing

- **Pure packages** (`ansi`, `classify`, `rules`, `logstore` format round-trip, `config` merge/inheritance): Go table-driven tests.
- **`conn`:** an in-process fake telnet server covering negotiation, MCP swallowing, charset, and reconnect.
- **`conn`, TLS:** first-use pinning, a matching reconnect, a mismatch refusal, and `/trust`.
- **`ui`:** `teatest` golden snapshots of the layout, input growth, the over-limit highlight, and the new-output pill.

## 10. Deferred (not v1)

- Shared-scene awareness (deduplicating or merging output when multiple alts are in the same room)
- A `kiln export` headless CLI (it would share code with browse-mode export)
- Full MCP (`simpleedit`, etc.)
- Gags (hide actions)
- A curated library of packs for other MUCK engines. v1 ships exactly one starter pack (`fuzzball`), which is written to `packs/` on first run.
- A "split long line here" helper that carries the pose prefix onto the continuation
- Scripting
