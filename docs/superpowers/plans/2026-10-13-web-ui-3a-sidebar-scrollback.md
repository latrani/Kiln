# Web UI, step 3a: sidebar, scrollback and top bar — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The page shows Kiln: the sidebar of open characters (switching, closing, unread and attention), the active character's scrollback drawn from Go's spans (colors, tags, links, hanging indent, the TUI's pager, older history, the "▼ N new" pill), a world's overview, the top bar with the presence chip, the status line's connection text, the tab title, presence reporting, and the window size reported to the MUCK. Typing and the password form are 3b.

**Architecture:**
- **Go:** one new action, `{"Do":"away"}` → `App.SetAway` (the presence chip). Nothing else changes in Go.
- **`src/look.ts`:** a span's inline style for both appearances at once: the fold (`theme.fold`) under the dark and the light tag tables, written as `--d-fg/--d-bg/--l-fg/--l-bg` custom properties plus the flags; `themeCSS` maps `.s` to the dark or light pair (and the media query, under `auto`). So a span never needs redrawing when the system flips light/dark.
- **`src/layout.ts`:** the sidebar's width, as the TUI works it out (`SidebarWidth`: a fifth of the columns, at least the add chips, at most 22), as a CSS expression in `ch`; and the scrollback's size in columns and rows for `resize`.
- **`src/pager.ts`:** the TUI's pager and pill as pure functions over line heights: where to stop when unseen lines overflow the view, and how many lines are below it.
- **Components:** `Sidebar` (rows, badges, counts), `Scrollback` (lines, prompt, pill, older history, overview, empty), `Line` (spans), `TopBar` (where you are, presence chip), `StatusLine` (connection text), `Announcer` (a visually hidden polite region for background attention lines), and `presence.ts` (here/focus/scrolled, throttled) and the title.
- **Lint (#147):** the page-text lint parses the TSX with the TypeScript compiler API instead of matching lines.

**Tech Stack:** as step 2; no new packages.

**Spec:** `docs/superpowers/specs/2026-10-12-web-ui-design.md` — "Layout", "Scrollback lines", "Keys" (the switching keys), "Accessibility", "Window size". The TUI is the reference for everything the spec doesn't say: `internal/ui/sidebar.go`, `view.go` (`topBarFor`, `statusLine`, `overview`), `scrollback.go` (`Pause`, `Unseen`, pill).

**Rulings (for review):**
1. **The sidebar's width follows the TUI's formula, not a fixed 16ch.** The TUI's sidebar is a fifth of the terminal, between the add chips' width and 22; 16 is what an 80-column terminal gets (the goldens). The spec's "16ch" came from those goldens. Same rule here, in whole `ch`, from the window's width.
2. **The sidebar scrolls natively**, without the TUI's "▲ N more" rows: a terminal can't scroll a region; a browser can.
3. **`+ Connection` and the Log chip come with what they open**: the picker (step 4) and log mode (step 5). Until then they'd be buttons that do nothing.
4. **Presence reporting lands here** (`here` on a key or click, at most once a second; `focus` on window focus and blur; `scrolled` on a scroll): part 2 decided it, but no page sent it yet, and the presence chip shows it.
5. **New strings:** `web.title`, `web.title_unread` (the tab), `web.char_unread`, `web.char_attention` (a character's name for screen readers, with its count), `web.close_char` (the × button's name), `web.announce` (a background character's attention line, read aloud).

## Global Constraints

- Every person-facing string from the catalog (`[web]` for new ones; reuse `view.*`, `state.*`, `scrollback.*`, `sidebar.*` where the TUI has the text). Commits that change `en.toml` end with `Strings:` listing the keys.
- TS tests build expected text from `str.gen.ts`; Go tests from `str`.
- Colors only from the theme (`themeCSS`, `roleClass`, the `--ansi-N`/`--term-*` variables); glyphs only from `str.glyph`.
- The TUI is unchanged; golden screens NOT updated.
- Before each commit: `go vet ./... && go test ./...`; `(cd web && go vet ./... && go test ./... && GOOS=js GOARCH=wasm go vet ./...)`; `(cd web/ui && npm run check && npm test)`; `web/build.sh`; and for Tasks 5–9 `(cd web && KILN_SMOKE=1 go test ./smoke)`. No `-race`.
- Never `git checkout` a file to undo a mutation test; undo with the same edit in reverse.

## Review Focus

1. **Light/dark switching with tag colors that differ by appearance** (a theme whose `[palette.light]` changes a tag's color): spans must take the light color under light, without a redraw. → Task 2's two-appearance test.
2. **The pager with one line taller than the view** (a long wrapped line): the view stops at that line's top, never below it, and the pill counts the lines wholly below. → Task 4's tall-line case.
3. **Older history arriving while scrolled up mid-history**: the line you're reading stays put (Safari has no `overflow-anchor`, so the page keeps the place itself). → Task 6's smoke check and the anchor code.
4. **A world's overview for a world whose characters have no lines yet**: names with "quiet", no crash on empty line lists. → Task 7's test.
5. **Background attention announcements for a character that's then closed, or a `replace` of its lines**: only newly appended attention lines are announced, once. → Task 8's tests.

---

### Task 1: the `away` action

**Files:** Modify `internal/webapi/actions.go`, `internal/webapi/loop_test.go`

- [ ] **Step 1: Failing test:**

```go
func TestAwaySaysSo(t *testing.T) {
	h := start(t)
	b := h.act(`{"Do":"away"}`)
	if st := event(b, "Status"); st == nil || st["Text"] != str.StatusAway() {
		t.Errorf("status %v", st)
	}
	if p := event(b, "Presence"); p == nil || p["State"] != float64(app.PresenceAway) {
		t.Errorf("presence %v", p)
	}
}
```

Run `go test ./internal/webapi -run Away` → FAIL (bad action).

- [ ] **Step 2:** In `do`, beside `here`: `case "away": //str:ok` → `a.SetAway()`. Run → PASS (if `Presence` doesn't report away without focus having been seen, send `{"Do":"focus","In":true}` first in the test and note it). **Commit** `webapi: away, for the page's presence chip`.

### Task 2: spans styled for both appearances

**Files:** Create `web/ui/src/look.ts`, `web/ui/src/look.test.ts`; modify `web/ui/src/theme.ts`, `web/ui/src/theme.test.ts`

**Interfaces:**
- Produces: `spanStyle(s: Span, dark: Record<string, TagLook>, light: Record<string, TagLook>): string` (inline CSS); `tagsFor(t: ThemeEvent | undefined, key: string): { dark: Record<string, TagLook>; light: Record<string, TagLook> }` (a character's own tag table, else the theme's); `themeCSS` gains the `.s` rules and the glyph variables (`--g-echo`, `--g-sys`, `--g-rule`).

- [ ] **Step 1: Failing tests** (`look.test.ts`):

```ts
import { test } from "node:test";
import assert from "node:assert/strict";
import { spanStyle, tagsFor } from "./look.ts";
import type { ThemeEvent } from "./types.ts";

test("server colors are the terminal's, either appearance", () => {
  const css = spanStyle({ Text: "x", FG: "ansi:1", Bold: true }, {}, {});
  assert.ok(css.includes("--d-fg:var(--ansi-1)") && css.includes("--l-fg:var(--ansi-1)"));
  assert.ok(css.includes("font-weight:700"));
});

test("a tag's color can differ by appearance", () => {
  const css = spanStyle({ Text: "x", Tags: ["page"] }, { page: { FG: "#111111" } }, { page: { FG: "#eeeeee" } });
  assert.ok(css.includes("--d-fg:#111111") && css.includes("--l-fg:#eeeeee"));
});

test("reverse swaps, defaulting to the terminal's colors", () => {
  const css = spanStyle({ Text: "x", FG: "ansi:2", Reverse: true }, {}, {});
  assert.ok(css.includes("--d-fg:var(--term-bg)") && css.includes("--d-bg:var(--ansi-2)"));
});

test("a plain span has no style", () => {
  assert.equal(spanStyle({ Text: "x" }, {}, {}), "");
});

test("a character's own tags win over the theme's", () => {
  const t: ThemeEvent = { type: "Theme", Appearance: "auto",
    Dark: { Roles: {}, Tags: { page: { FG: "#1" } }, Chars: { "fm/kit": { page: { FG: "#2" } } } },
    Light: { Roles: {}, Tags: { page: { FG: "#3" } }, Chars: { "fm/kit": { page: { FG: "#4" } } } } };
  assert.equal(tagsFor(t, "fm/kit").dark.page.FG, "#2");
  assert.equal(tagsFor(t, "fm/kit").light.page.FG, "#4");
  assert.equal(tagsFor(t, "fm/rook").dark.page.FG, "#1");
  assert.deepEqual(tagsFor(undefined, "x"), { dark: {}, light: {} });
});
```

And in `theme.test.ts`: `themeCSS(ev("dark"))` includes `.s{color:var(--d-fg);background:var(--d-bg)}`; `themeCSS(ev("light"))` the `--l-` pair; `auto` has the dark pair outside and the light pair inside the media query; and `--g-echo:"${str.glyph.echo}"` is in every sheet.

Run → FAIL.

- [ ] **Step 2: Implement `look.ts`:**

```ts
import { fold } from "./theme.ts";
import type { Color, Span, TagLook, ThemeEvent } from "./types.ts";

// ref is a color as CSS that follows the appearance: the terminal's
// sixteen through their variables, hex as is.
const ref = (c: Color | undefined): string | undefined => {
  if (!c) return undefined;
  const m = /^ansi:(\d+)$/.exec(c);
  return m ? `var(--ansi-${m[1]})` : c;
};

// pair is a folded look's foreground and background, reverse applied.
function pair(l: { FG?: Color; BG?: Color; Reverse?: boolean }): [string | undefined, string | undefined] {
  let fg = ref(l.FG), bg = ref(l.BG);
  if (l.Reverse) [fg, bg] = [bg ?? "var(--term-bg)", fg ?? "var(--term-fg)"];
  return [fg, bg];
}

// spanStyle is a span's inline style: its look folded under the dark and
// the light tag tables, as custom properties the theme's .s rule picks
// between, and the flags (which don't depend on the appearance).
export function spanStyle(s: Span, dark: Record<string, TagLook>, light: Record<string, TagLook>): string {
  const d = fold(s, dark), l = fold(s, light);
  const [dfg, dbg] = pair(d), [lfg, lbg] = pair(l);
  const out: string[] = [];
  if (dfg) out.push(`--d-fg:${dfg}`);
  if (dbg) out.push(`--d-bg:${dbg}`);
  if (lfg) out.push(`--l-fg:${lfg}`);
  if (lbg) out.push(`--l-bg:${lbg}`);
  if (d.Bold) out.push("font-weight:700");
  if (d.Faint) out.push("opacity:.6");
  if (d.Italic) out.push("font-style:italic");
  if (d.Underline) out.push("text-decoration:underline");
  return out.join(";");
}

// tagsFor is character key's tag tables: its own, else the theme's.
export function tagsFor(t: ThemeEvent | undefined, key: string) {
  return {
    dark: t?.Dark.Chars?.[key] ?? t?.Dark.Tags ?? {},
    light: t?.Light.Chars?.[key] ?? t?.Light.Tags ?? {},
  };
}
```

(Flags come from the dark fold: a tag's flags are the same in both appearances, since only palettes change with it.) In `theme.ts`'s `sheet`, append `.s{color:var(--${p === latte ? "l" : "d"}-fg);background:var(--${…}-bg)}` and add the glyph variables to `:root` from `str.glyph` (`--g-echo`, `--g-sys`, `--g-rule`), each `JSON.stringify`d. An unset `--d-fg` leaves `color` inherited and `background` transparent, which is the terminal's own.

- [ ] **Step 3:** Run → PASS. **Commit** `web/ui: spans styled for dark and light at once`.

### Task 3: layout

**Files:** Create `web/ui/src/layout.ts`, `web/ui/src/layout.test.ts`; modify `web/ui/src/kiln.css`, `web/ui/src/ui/App.tsx`

**Interfaces:**
- Produces: `chipsWidth(): number` (the TUI's `2 + max(len("+ Connection"), 1 + len(addChar), len(addWorld))`, in characters, from the catalog); `sidebarCSS(): string` (`min(22ch, max(<chips>ch, round(down, 20vw, 1ch)))`); `paneSize(el: HTMLElement): { cols: number; rows: number }` (the element's width and height over one `ch` and one `lh`, measured with a hidden `0` span).

- [ ] **Step 1: Failing test:**

```ts
import { test } from "node:test";
import assert from "node:assert/strict";
import { chipsWidth, sidebarCSS } from "./layout.ts";
import * as str from "./str.gen.ts";

test("the sidebar is the TUI's width rule", () => {
  const len = (s: string) => [...s].length;
  assert.equal(chipsWidth(), 2 + Math.max(len(str.sidebarOpenConnection()), 1 + len(str.pickerAddCharacter()), len(str.pickerAddWorld())));
  assert.equal(sidebarCSS(), `min(22ch, max(${chipsWidth()}ch, round(down, 20vw, 1ch)))`);
});
```

Run → FAIL.

- [ ] **Step 2: Implement**, and in `App.tsx` set `style={{ "--side": sidebarCSS() }}` on `.kiln`; in `kiln.css`, `grid-template-columns: var(--side) 1px 1fr`. `paneSize` isn't unit-testable (no layout in Node); the smoke test covers `resize` (Task 9). Run → PASS. **Commit** `web/ui: the sidebar is as wide as the TUI's`.

### Task 4: the pager, as arithmetic

**Files:** Create `web/ui/src/pager.ts`, `web/ui/src/pager.test.ts`

**Interfaces:**
- Produces:

```ts
// tops/heights are each line's offset and height in px, in order.
export function pause(tops: number[], heights: number[], firstUnseen: number, view: number): number | undefined;
// scrollTop that puts line firstUnseen at the top, if the unseen lines overflow view; else undefined (stay live).
export function below(tops: number[], heights: number[], scrollTop: number, view: number): number;
// how many lines lie wholly below the view (the pill's N).
```

- [ ] **Step 1: Failing tests:**

```ts
import { test } from "node:test";
import assert from "node:assert/strict";
import { below, pause } from "./pager.ts";

const rows = (hs: number[]) => {
  const tops: number[] = [];
  let y = 0;
  for (const h of hs) { tops.push(y); y += h; }
  return { tops, heights: hs };
};

test("unseen lines that fit: stay live", () => {
  const { tops, heights } = rows([20, 20, 20, 20]);
  assert.equal(pause(tops, heights, 2, 100), undefined);
});

test("unseen lines that overflow: stop at the first of them", () => {
  const { tops, heights } = rows([20, 20, 20, 20, 20, 20, 20, 20]);
  assert.equal(pause(tops, heights, 2, 100), 40);
});

test("one line taller than the view: stop at its top", () => {
  const { tops, heights } = rows([20, 300, 20]);
  assert.equal(pause(tops, heights, 1, 100), 20);
});

test("nothing unseen: stay live", () => {
  const { tops, heights } = rows([20, 20]);
  assert.equal(pause(tops, heights, 2, 10), undefined);
});

test("the pill counts lines wholly below the view", () => {
  const { tops, heights } = rows([20, 20, 20, 20, 20, 20]);
  assert.equal(below(tops, heights, 0, 50), 3); // lines 3, 4, 5 start at 60+
  assert.equal(below(tops, heights, 20, 100), 0);
});
```

Run → FAIL.

- [ ] **Step 2: Implement:** `pause`: if `firstUnseen >= tops.length` → undefined; `const end = tops.at(-1)! + heights.at(-1)!`; if `end - tops[firstUnseen] <= view` → undefined; else `tops[firstUnseen]`. `below`: count `i` with `tops[i] >= scrollTop + view`. Run → PASS. **Commit** `web/ui: the pager and the pill, as arithmetic`.

### Task 5: lines and the scrollback

**Files:** Create `web/ui/src/ui/Line.tsx`, `web/ui/src/ui/Line.test.tsx`; rewrite `web/ui/src/ui/Scrollback.tsx`; modify `web/ui/src/ui/App.test.tsx`, `web/ui/src/kiln.css`, `internal/str/locales/en.toml` (none yet), `web/smoke/smoke_test.go`

**Interfaces:**
- Consumes: `spanStyle`, `tagsFor` (Task 2); `pause`, `below` (Task 4); `State.lines`, `inputs[key].Prompt` (spans), `chars[key].More/Loading`.
- Produces: `Line({ line, tags })`; the scrollback's behavior below.

- [ ] **Step 1: Failing tests** (`Line.test.tsx`):

```tsx
import { test } from "node:test";
import assert from "node:assert/strict";
import { render } from "preact-render-to-string";
import { Line } from "./Line.tsx";
import { roleClass } from "../theme.ts";
import type { LineData } from "../store.ts";

const L = (over: Partial<LineData>): LineData => ({ Seq: 1, Kind: "server", Day: "", Out: false, Plain: "", Spans: [], Attention: false, Quiet: false, ...over });
const text = (html: string) => html.replace(/<[^>]*>/g, "").replace(/&amp;/g, "&").replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&quot;/g, '"');
const tags = { dark: {}, light: {} };

test("a line's text is exactly its plain text, spaces at span edges and all", () => {
  const l = L({ Plain: "a  b\tc ", Spans: [{ Text: "a " }, { Text: " b", Bold: true }, { Text: "\tc " }] });
  assert.equal(text(render(<Line line={l} tags={tags} />)), l.Plain);
});

test("links are links, opening a new tab", () => {
  const html = render(<Line line={L({ Plain: "see https://x.test", Spans: [{ Text: "see " }, { Text: "https://x.test", Link: "https://x.test" }] })} tags={tags} />);
  assert.match(html, /<a [^>]*href="https:\/\/x.test"[^>]*target="_blank"[^>]*rel="noopener noreferrer"/);
  assert.ok(html.includes(roleClass("link")));
});

test("Kiln's lines wear their roles", () => {
  for (const [Kind, role] of [["echo", "scrollback.echo"], ["sys", "scrollback.sys"], ["day", "scrollback.day"], ["history_end", "scrollback.history_end"]] as const) {
    assert.ok(render(<Line line={L({ Kind, Plain: "x", Spans: [{ Text: "x" }] })} tags={tags} />).includes(roleClass(role)), Kind);
  }
});

test("a link that isn't http(s) isn't a link", () => {
  const html = render(<Line line={L({ Plain: "x", Spans: [{ Text: "x", Link: "javascript:alert(1)" }] })} tags={tags} />);
  assert.ok(!html.includes("<a "));
});
```

`App.test.tsx`: with `Order`, `Char` (`State: 2`), `Active` and a `Lines` reset of two lines for `fm/kit`, the page's text includes both lines' `Plain`; a `Char` with `Loading: true` shows `str.scrollbackLoading()`; an `Input` with `Prompt` spans for the active character shows the prompt text after the lines.

Run → FAIL.

- [ ] **Step 2: Implement `Line.tsx`:** `<div class={"line " + kindClass}>` (kind classes: `l-server`, `l-echo` + `roleClass("scrollback.echo")`, `l-sys` + sys role, `l-day` + day role, `l-history_end` + its role), each span a `<span class="s" style={spanStyle(...)}>` (no `class`/`style` when the style is empty: a bare text node), a span with a `Link` starting `http://` or `https://` an `<a class={"s " + roleClass("link")} href target="_blank" rel="noopener noreferrer">`. `kiln.css`:

```css
.line { white-space: pre-wrap; overflow-wrap: anywhere; tab-size: 8; padding-left: 1ch; text-indent: -1ch; }
.l-echo::before { content: var(--g-echo); }
.l-sys::before { content: var(--g-sys) " "; }
.l-day::before { content: var(--g-rule) var(--g-rule) " "; }
.l-day::after { content: " " var(--g-rule) var(--g-rule); }
```

For a link under the pointer, `themeCSS` also emits `a.r-link:hover{…}` from the `link.hover` role's look (with a `theme.test.ts` case).

- [ ] **Step 3: Implement `Scrollback.tsx`** (the active character; the overview is Task 7):
  - Lines keyed `${epoch}:${Seq}`, the prompt (`inputs[active].Prompt` spans, as a `Line` of kind `server`) last, only while live (at the bottom), as the TUI shows it.
  - **Stick to the bottom:** a `useLayoutEffect` after each render: if the view was at the bottom before the update (`scrollHeight - scrollTop - clientHeight < 2`, measured in an `onScroll` handler and kept in a ref), scroll to the bottom — unless the pager stops it (below).
  - **Pager:** per character, the page keeps `seen` (the count of lines seen); a reset of a character's lines (history loading) marks them all seen; switching away marks the old character's lines seen; scrolling to the bottom marks them seen. After an append while live, `pause(tops, heights, firstUnseenIndex, clientHeight)` — if it gives a top, scroll there instead of the bottom. (Sending a line marks them seen too: 3b.)
  - **Pill:** while not at the bottom, a `<button class={roleClass("scrollback.pill")}>` at the bottom right with `str.viewPillNew(n)` (`n = below(...)` when the pager stopped the view, else the lines appended since scrolling up) or `str.viewPillMore()`; clicking scrolls to the bottom.
  - **Older history:** when `scrollTop < clientHeight` and `chars[active].More && !Loading`, `act({Do: "page", Key: active})` (once until `Loading` changes). While `Loading`, a first row `str.scrollbackLoading()` in `roleClass("scrollback.loading")`. Keeping your place on a prepend: before the update record `scrollHeight - scrollTop`, after it set `scrollTop = scrollHeight - recorded` (Safari has no `overflow-anchor`).
  - **`scrolled`:** user scrolls report `act({Do:"scrolled"})`, at most once a second (Task 9's `presence.ts` throttle).
  - Inside `<main class="body"><div role="log" aria-live="polite">` (the log role's own politeness; only the active character's lines are in it).
  - `act` reaches the component through the store: add `act?: (o: object) => void` to `State`, set by `boot` with `kilnPage.act`.

- [ ] **Step 4: Smoke** (`smoke_test.go`, after the echo): the echoed line is on the page: `[...document.querySelectorAll("main .line")].some(l => l.textContent.includes('You say, "hello"'))`.

- [ ] **Step 5:** Run → PASS. **Commit** `web/ui: the scrollback draws Go's lines`.

### Task 6: the sidebar

**Files:** Rewrite `web/ui/src/ui/Sidebar.tsx`; create `web/ui/src/sidebar.ts` (rows), `web/ui/src/sidebar.test.ts`; modify `web/ui/src/ui/App.test.tsx`, `kiln.css`, `internal/str/locales/en.toml`

**Interfaces:**
- Produces: `sidebarRows(order: string[], chars: Record<string, any>): Row[]` with `type Row = { kind: "world"; world: string } | { kind: "char"; key: string; world: string }` — the TUI's `sidebarRows` without the gap and add rows (Ruling 3).

- [ ] **Step 1: Catalog:** `[web]` `char_unread = { one = "{name}, 1 unread", other = "{name}, {n} unread" }`, `char_attention = { one = "{name}, 1 unread, wants attention", other = "{name}, {n} unread, wants attention" }`, `close_char = "Close {name}"`. `go generate ./internal/str`.
- [ ] **Step 2: Failing tests:** `sidebarRows(["fm/kit", "fm/rook", "pf/ash"], …)` gives world fm, kit, rook, world pf, ash. In `App.test.tsx`, with Kit active and connected, Rook disconnected with 3 unread and attention, and Ash connecting:
  - Kit's row has `aria-current="true"` and `roleClass("sidebar.active")`;
  - Rook's button is named `str.webCharAttention(3, "Rook")` (check the generated parameter order) and shows `str.glyph.attention` and `3`, and has a button named `str.webCloseChar("Rook")` showing `str.glyph.remove`;
  - Ash shows `…` in `roleClass("sidebar.connecting")`. The TUI draws `" … "` literally (`sidebar.go`); add `Connecting = "…"` to `internal/glyph` (and `All`), regenerate, and use `str.glyph.connecting` — the TUI then uses `glyph.Connecting` too.
  Run → FAIL.
- [ ] **Step 3: Implement:** a `<ul>` of rows; a world row is a `<button class={roleClass(active === "=" + world ? "sidebar.active" : "sidebar.world")}>` switching to `"=" + world` (the world's overview, `app.WorldSel`); a character row is a `<li>` with: the badge (` … ` connecting; a `×` `<button aria-label={str.webCloseChar(name)}>` disconnected or failed, which sends `close`; else one space), the name button (switches; `aria-current` when active; `aria-label` from `char_unread`/`char_attention` when there's unread), and the activity (`glyph.attention` in `sidebar.attention` when `Attention && Unread`, then the count in `sidebar.unread`). The name is cut with `…` by CSS (`text-overflow: ellipsis; overflow: hidden; white-space: nowrap`), the activity never. The active row is `sidebar.active` across the row. Switching sends `act({Do: "switch", Key})`.
- [ ] **Step 4: Smoke:** after opening the character, a sidebar button with its name exists; clicking another character's row isn't possible with one character — instead, `act({Do:"open", Key:"fake/second"})` needs a second preset character: add one to the smoke preset (`config()`), open it, click the first character's button, and wait for `Active` to be the first.
- [ ] **Step 5:** Run → PASS. **Commit** `web/ui: the sidebar` with `Strings: web.char_unread, web.char_attention, web.close_char (new)`.

### Task 7: a world's overview, and an empty pane

**Files:** Create `web/ui/src/ui/Overview.tsx`; modify `Scrollback.tsx`, `App.test.tsx`

- [ ] **Step 1: Failing tests:** with `Active` `=fm` and Kit (two lines, `LastSeen: "21:06"`) and Rook (no lines, no `LastSeen`): the page shows Kit's name, `str.separator() + "21:06"`, its last lines (at most 5: give it 7 and check the first two are absent), a rule, Rook's name with `str.viewOverviewQuiet()`. With no worlds configured (`Worlds: []`) and nothing open: `str.viewNoCharacters()` when it's non-empty (it's `""` today, so assert nothing breaks and nothing shows).
- [ ] **Step 2: Implement** as the TUI's `overview`: for each open character in the world, a `<button>` row (name in `scrollback.overview`, the separator and when in `scrollback.sys`) that switches to it, then its last five lines as `Line`s cut to one row each (`white-space: nowrap; overflow: hidden; text-overflow: ellipsis` on an `.ov` wrapper), a `<hr class={roleClass("scrollback.rule")}>`-styled rule between characters (a `div` with a top border in `--role-…`; add `scrollback.rule` to `themeCSS`'s line roles).
- [ ] **Step 3:** Run → PASS. **Commit** `web/ui: a world's overview`.

### Task 8: top bar, status line, title, announcements

**Files:** Rewrite `TopBar.tsx`; modify `StatusLine.tsx`; create `web/ui/src/ui/Announcer.tsx`; modify `App.tsx`, `App.test.tsx`, `store.ts` (append notifications), `internal/str/locales/en.toml`

- [ ] **Step 1: Catalog:** `[web]` `title = "Kiln"`, `title_unread = "({n}) Kiln"`, `announce = "{name}: {line}"`. Generate.
- [ ] **Step 2: Failing tests:**
  - Top bar: with Kit active, the text `fm` + `str.separator()` + `Kit`, and a `<button>` showing `str.viewPresenceUnknown()` in `roleClass("status.presence.unknown")` (with `Presence: 1` → `viewPresenceHere`/`…here`; `2` → away); with `=fm` active, just `fm`.
  - Status line: Kit connected with `ConnectedSince: "Connected since 21:14"` and no status → that text; disconnected → `str.stateDisconnected()`; a status wins over both (existing tests).
  - Title: a pure `titleFor(state)` in `src/title.ts`: total unread 0 → `str.webTitle()`, 4 → `str.webTitleUnread(4)`.
  - Announcements: a pure `announcements(prev: State, next: State): string[]` in `src/announce.ts`: lines **appended** (`append` op) to a character that isn't active, with `Attention`, give `str.webAnnounce(name, plain)`; a `reset`/`replace`/`prepend`, the active character's lines, and lines without attention give nothing; a closed character gives nothing.
  Run → FAIL.
- [ ] **Step 3: Implement:**
  - `TopBar`: left text in `status`; right, the presence chip as a `<button>` (padded one space either side, as `chip()` does) that sends `act({Do: "away"})` unless already away.
  - `StatusLine`: after the status and before not-saving: with an active character, `ConnectedSince` when connected, else the state's name (`str.stateDisconnected/Connecting/Connected/Failed` by `State` 0–3).
  - `title.ts` + an effect in `App` setting `document.title`.
  - `announce.ts`: compare the two states' `lines` per key: new lines are those whose `Seq` is above the last `Seq` in `prev` for the same `epoch`. `Announcer` keeps the last message in a visually hidden `<div role="status" class="sr-only">` (keyed so a repeat is read again); `App` computes announcements in a store subscription (keep the previous state in a ref). `.sr-only` in `kiln.css`: the standard clip pattern.
- [ ] **Step 4:** Run → PASS. **Commit** `web/ui: top bar, connection status, tab title, background announcements` with `Strings: web.title, web.title_unread, web.announce (new)`.

### Task 9: presence, keys for switching, window size

**Files:** Create `web/ui/src/presence.ts`, `web/ui/src/presence.test.ts`; modify `main.tsx`, `boot.ts` (or `App.tsx`) for listeners; `web/smoke/smoke_test.go`

**Interfaces:**
- Produces: `throttle(fn: () => void, ms: number, now = Date.now): () => void` (leading call, then at most one per `ms`); `nextUnread(order, chars, active, dir: 1 | -1): string | undefined` (the TUI's `switchToUnread`: the next character with unread lines after `active` in `order`, wrapping); `nextChar(order, active, dir)` (Ctrl+↑/↓: the next open character, wrapping, from the TUI's `switchBy`).

- [ ] **Step 1: Failing tests:** `throttle` with a fake clock: calls at 0, 100, 900, 1000, 1500 ms with 1000ms → runs at 0 and 1000 only. `nextUnread(["a","b","c"], {a:{},b:{Unread:2},c:{}}, "c", 1) === "b"` (wraps), `-1` from `a` → `b`, none unread → undefined. `nextChar(["a","b"], "b", 1) === "a"`. Read `internal/ui/model.go`'s `switchBy` and `switchToUnread` first and mirror them exactly (world rows? whether overviews count) — if they consult the core (`app`), note it and mirror the core's rule.
- [ ] **Step 2: Implement**, and wire in `main.tsx`: `keydown`/`pointerdown` → throttled `here`; window `focus`/`blur` → `focus` with `In`; Ctrl+↑/↓ → `switch` to `nextChar`; Alt+↓/↑ → `switch` to `nextUnread` (`preventDefault` on each). A `ResizeObserver` on the scrollback → `act({Do: "resize", Cols, Rows})` from `paneSize`, 200ms after the last change, and once at start.
- [ ] **Step 3: Smoke:** after connecting, dispatch a `keydown` on `document` and a `focus` event on `window` through `Runtime.evaluate`, and wait for a `Presence` event with `State === 1` (here). The window size isn't checked end to end: the fake MUCK doesn't speak NAWS; `resize` reaching Go is covered by `webapi`'s existing resize test.
- [ ] **Step 4:** Run → PASS. **Commit** `web/ui: presence, switching keys, and the window size for the MUCK`.

### Task 10: the page-text lint, properly (#147)

**Files:** Rewrite `web/ui/src/lint.test.ts`

- [ ] **Step 1:** Use `typescript` (already a dev dependency): `ts.createSourceFile(file, text, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)`, walk it, and flag: a `JsxText` node with a letter; a `JsxAttribute` named `aria-label`, `aria-description`, `aria-placeholder`, `title`, `placeholder`, `alt` or `value` whose initializer is a string literal, or a `JsxExpression` containing only a string literal or a no-substitution template literal. Exempt a node on a line ending in `// str:ok`.
- [ ] **Step 2: Prove it bites:** the three shapes #147 names (multi-line text in `TopBar`, `aria-label={"x"}` in `Sidebar`, `title="x"` in `Line`) each fail it; undo each with the reverse edit.
- [ ] **Step 3:** Run → PASS. **Commit** `web/ui: the page-text lint parses the TSX (closes #147)`.

### Task 11: wrap up

- [ ] Screenshots (dark and light) of the page with two characters, one with unread and attention, a few colored lines, a link, and a world's overview — with the scratch relay and the DevTools script from step 2. Fresh whole-branch review with this plan's Review Focus; fix Critical/Important test-first; triage minors with Indi; PR with the strings, the Rulings and the screenshots described.
