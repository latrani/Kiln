# Web UI step 5b: the filter panel — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** In log mode, `f` or the top bar's `Filter` chip opens the TUI's filter panel in the sidebar. From it you can hide lines by tag or by text, show only one tag or text, fold tag groups, and add or remove text filters.

**Architecture:** Go already sends everything the panel needs and takes every action it sends. The `Filter` event lists each entry with its Hidden, Only, Folded and FilteredUnder state; the `Log` event's `Hidden` lists the lines it hides. The actions are `hide`, `only`, `fold`, `add_text` and `remove_text`. So this step is page-only:
- `filter.ts`: the TUI's rules (what the panel lists, how each entry reads, its keys) as pure functions, plus `panelAct` and `togglePanel`, which take the store.
- The store's per-character log view gains `panel` (which entry is highlighted).
- `FilterPanel.tsx`: the panel itself, in the sidebar.
- `LogBar`: the filter text prompt and the panel's hints.
- `TopBar`: the `Filter` chip.
- `LogView`: keeps your place when the filter hides lines.

**Tech Stack:** Preact + TypeScript (web/ui), node:test with preact-render-to-string, headless Chrome smoke (web/smoke).

**Spec:** `docs/superpowers/specs/2026-10-12-web-ui-design.md`. This plan covers step 5, "Log mode", second half. The relevant parts:
- **Layout:** "In log mode it's the filter panel".
- **Keys:** `f` among log mode's letters, which work only while the log has focus.
- **Accessibility:** native elements first; the top bar has real buttons.
- **Order of work:** 5.

## For you to check

These calls change what someone sees or does. Everything else copies the TUI (`internal/ui/filter.go`, `browse.go`'s prompt and hints, `view.go`'s top bar).

1. **The panel's keys are the TUI's,** while the panel has focus:
   - ↑/↓ move the highlight, and focus moves with it;
   - `h` hides and `o` shows only the highlighted entry;
   - ←/→ fold a tag group shut or open;
   - Enter on `+ Text` asks for a text to filter by;
   - `x` or Delete removes a text row;
   - Esc or `f` closes the panel, and focus goes back to the log.
2. **Hide and Only are real toggle buttons** (`aria-pressed`). Tab reaches the highlighted entry's name, Hide and Only, then leaves the panel. Other entries' buttons are reached with ↑/↓, or clicked. The ▼ ► × glyphs are for the mouse; the keyboard has ←/→ and `x` for them.
3. **Screen readers:** each entry is a group named by its entry, so they read "page, Hide, toggle button". The panel is named "Filter" (new string `web.filter`).
4. **With the panel open and the log focused** (you clicked a line), log mode's letters work as usual, and `f` closes the panel. Esc closes the panel first, then log mode on the next Esc. The TUI has no such state, since its panel takes every key.
5. **The hint row shows the panel's keys** (the TUI's `filter.hints`) whenever the panel is open, as the TUI does, even while the log has focus.
6. **No `▲ N more` rows.** The panel scrolls natively, as the web's sidebar does.
7. **`web.log_hints` gains `f filter`,** after `g date` as in the TUI's hints.
8. **Hiding the line you're reading keeps your place** at the next line still shown.

## Global Constraints

- **The TUI is the reference** where the spec doesn't say otherwise: `internal/ui/filter.go` (rows, keys, clicks), `browse.go` (`promptFilterText`, hints), `view.go` (the `Filter` chip between presence and `Log`).
- **Go decides data; the page decides looks,** from theme roles (`filter`, `filter.item`, `filter.selected`, `filter.button`, `filter.button.on`, `filter.add`, `status.filter`, `status.filter.on`), never a hard-coded color.
- **Every string a person reads comes from the catalog,** and a commit that changes it carries a `Strings:` trailer. Tests build expected text from `str.*`.
- **Log mode's letters work only while the log has focus** (spec). The panel's letters work only while the panel has focus.
- **No flex rows for indented text.** A flex box drops a text run that's only a space, which is how #161's picker lost its indent. The panel's rows are blocks with `white-space: pre`.
- **Pre-1.0:** no compatibility shims. The TUI's goldens never move.

## Review Focus

1. **The highlighted entry goes away** (a text row removed, a group folded over it, a new `Filter`). The highlight falls back to the first entry; nothing throws and focus isn't lost. → Task 1's `fixSel` test; Task 3's test of a `Filter` without the highlighted entry.
2. **Typing in the filter text prompt** never reaches the panel's keys or log mode's letters. "fake muck" has an `f`, which closes the panel if it leaks. → Task 6's smoke step.
3. **Hiding the line you're reading:** the view stays at the next line still shown, rather than jumping. → Task 5's `topFor` fallback test and `keepsPlace` test.
4. **Switching characters with the panel open:** each character's panel (open or not, and its highlight) is its own. → Task 2's store test.
5. **Ctrl+L with the panel open,** then Ctrl+L again: the panel is back as it was, and the log has focus (log mode's own focus wins on the way back). → Task 2's store test; Task 6's smoke step.

---

## File Structure

- `web/ui/src/filter.ts` (new): `FilterEntry`, `PanelRow`, `entryKey`, `panelRows`, `fixSel`, `moveSel`, `filterKey`, `panelAct`, `togglePanel`, `closePanel`. Test `filter.test.ts`.
- `web/ui/src/store.ts`: `LogView.panel`; the `Prompt` kind `filter`. Test `store.test.ts`.
- `web/ui/src/log.ts`: `f` (`LogKey` `filter`); Esc closes the panel first; `topFor`'s fallback; `keepsPlace`. Test `log.test.ts`, `store.test.ts`.
- `web/ui/src/ui/FilterPanel.tsx` (new): the panel. `Sidebar.tsx` shows it. Test `ui/FilterPanel.test.tsx`.
- `web/ui/src/ui/LogBar.tsx`: the filter prompt; the panel's hints. `TopBar.tsx`: the `Filter` chip. Tests in `ui/LogView.test.tsx`.
- `web/ui/src/ui/LogView.tsx`: your place kept when the filter changes.
- `web/ui/src/kiln.css`: the panel's rows.
- `internal/str/locales/en.toml`: `web.filter` (new), `web.log_hints` (changed).
- `web/smoke/log_test.go`: `TestFilter`.
- `docs/web-dev.md`: the filter panel.

---

### Task 1: The panel's rules (`filter.ts`, pure)

**Files:**
- Create: `web/ui/src/filter.ts`
- Test: `web/ui/src/filter.test.ts`

**Interfaces:**
- Consumes: Go's `Filter` event (`feed.Filter`): `{ Key, Entries: FilterEntry[] }`. Each `FilterEntry` is `app.PanelEntry` (`Item {Name, Text, Untagged}`, `Depth`, `Parent`, `FoldedAway`) plus `Hidden`, `Only`, `Folded`, `FilteredUnder`.
- Produces:

```ts
export interface FilterItem { Name: string; Text: boolean; Untagged: boolean }
export interface FilterEntry { Item: FilterItem; Depth: number; Parent: boolean; FoldedAway: boolean; Hidden: boolean; Only: boolean; Folded: boolean; FilteredUnder: boolean }
export type PanelRow =
  | { kind: "entry"; key: string; e: FilterEntry; glyph: string; name: string; after: string; endsGroup: boolean }
  | { kind: "add"; key: "+" };
export const entryKey: (i: FilterItem) => string; // "u", "t:<term>", "g:<tag>"
export function panelRows(entries: FilterEntry[]): PanelRow[];
export function fixSel(rows: PanelRow[], sel: string): string;
export function moveSel(rows: PanelRow[], sel: string, delta: number): string;
export type PanelKey = "up" | "down" | "hide" | "only" | "shut" | "open" | "add" | "remove" | "close";
export function filterKey(e: { key: string; ctrlKey: boolean; altKey: boolean; metaKey: boolean }): PanelKey | undefined;
```

What the TUI lists (`panelRows`):
- **The entries,** less those folded away, then `+ Text` last.
- **An entry's name:**
  - `Untagged` (`str.filterUntagged()`) for lines with no tags;
  - a text row's term in quotes (`str.filterTextItem(term)`), after `×`;
  - a tag's last path segment (`page/ooc` reads `ooc`).
- **A parent's glyph:** `▼` open, `►` folded. A folded parent with something under it hidden or Only gets ` •` after its name.
- **The indent** is `1 + 2 × Depth` spaces. The Hide/Only row is one space further in.
- **A blank row** follows each top-level group: after an entry whose next entry has depth 0 (`+ Text` counts as depth 0).

- [ ] **Step 1: Write the failing tests** (`filter.test.ts`)

```ts
import { test } from "node:test";
import assert from "node:assert/strict";
import { filterKey, fixSel, moveSel, panelRows } from "./filter.ts";
import type { FilterEntry } from "./filter.ts";
import * as str from "./str.gen.ts";

const E = (Name: string, more: Partial<FilterEntry> & { Text?: boolean; Untagged?: boolean } = {}): FilterEntry => {
  const { Text = false, Untagged = false, ...rest } = more;
  return { Item: { Name, Text, Untagged }, Depth: Name.split("/").length - 1, Parent: false, FoldedAway: false, Hidden: false, Only: false, Folded: false, FilteredUnder: false, ...rest };
};

test("the panel lists entries less those folded away, + Text last; names as the TUI's", () => {
  const rows = panelRows([E("", { Untagged: true, Depth: 0 }), E("page", { Parent: true }), E("page/ooc"), E("page/ic", { FoldedAway: true }), E("hello world", { Text: true, Depth: 0 })]);
  assert.deepEqual(rows.map((r) => r.key), ["u", "g:page", "g:page/ooc", "t:hello world", "+"]);
  const names = rows.flatMap((r) => (r.kind === "entry" ? [r.name] : []));
  assert.deepEqual(names, [str.filterUntagged(), "page", "ooc", str.filterTextItem("hello world")]);
});

test("glyphs: ▼ open, ► folded (• when something under it is filtered), × on a text row", () => {
  const rows = panelRows([E("a", { Parent: true }), E("b", { Parent: true, Folded: true, FilteredUnder: true }), E("c", { Parent: true, Folded: true }), E("t", { Text: true, Depth: 0 })]);
  const g = rows.flatMap((r) => (r.kind === "entry" ? [[r.glyph, r.after]] : []));
  assert.deepEqual(g, [[str.glyph.open, ""], [str.glyph.collapsed, str.glyph.filtered], [str.glyph.collapsed, ""], [str.glyph.remove, ""]]);
});

test("a blank row ends each top-level group", () => {
  const rows = panelRows([E("a", { Parent: true }), E("a/b"), E("c"), E("t", { Text: true, Depth: 0 })]);
  assert.deepEqual(rows.flatMap((r) => (r.kind === "entry" ? [r.endsGroup] : [])), [false, true, true, true]);
});

test("the highlight: kept if it's there, else the first entry; moves stop at the ends", () => {
  const rows = panelRows([E("a"), E("b")]);
  assert.equal(fixSel(rows, "g:b"), "g:b");
  assert.equal(fixSel(rows, "t:gone"), "g:a", "gone: the first");
  assert.equal(moveSel(rows, "g:a", 1), "g:b");
  assert.equal(moveSel(rows, "g:b", 5), "+");
  assert.equal(moveSel(rows, "g:a", -1), "g:a");
  assert.equal(moveSel(rows, "t:gone", 1), "g:b", "from the first, when it's gone");
});

test("the panel's keys, as the TUI's; never with a modifier", () => {
  const k = (key: string, mods: { ctrlKey?: boolean; altKey?: boolean; metaKey?: boolean } = {}) => filterKey({ key, ctrlKey: false, altKey: false, metaKey: false, ...mods });
  assert.deepEqual(["ArrowUp", "ArrowDown", "h", "o", "ArrowLeft", "ArrowRight", "Enter", "x", "Delete", "Escape", "f"].map((x) => k(x)),
    ["up", "down", "hide", "only", "shut", "open", "add", "remove", "remove", "close", "close"]);
  assert.equal(k("h", { ctrlKey: true }), undefined);
  assert.equal(k("o", { metaKey: true }), undefined);
  assert.equal(k("j"), undefined, "no j/k on the web: arrows");
});
```

- [ ] **Step 2: Run to see them fail**

Run: `cd web/ui && npm test`
Expected: FAIL: `./filter.ts` can't be resolved.

- [ ] **Step 3: Implement** (`filter.ts`; Task 2 adds the store-taking functions to the same file)

```ts
// The filter panel's rules, as the TUI's (internal/ui/filter.go): what it
// lists, how each entry reads, and its keys. Go holds the filter and the
// folds (the Filter event); the page holds which entry is highlighted.
import * as str from "./str.gen.ts";

// FilterItem is a thing the filter can hide or show only: a tag, a text,
// or Untagged (scene.Item).
export interface FilterItem { Name: string; Text: boolean; Untagged: boolean }

// FilterEntry is one entry of Go's Filter event (feed.FilterEntry).
export interface FilterEntry {
  Item: FilterItem; Depth: number; Parent: boolean; FoldedAway: boolean;
  Hidden: boolean; Only: boolean; Folded: boolean; FilteredUnder: boolean;
}

// PanelRow is an entry the panel lists, as it reads (its glyph, name and
// what follows it; a blank row after it when it ends a top-level group),
// or + Text.
export type PanelRow =
  | { kind: "entry"; key: string; e: FilterEntry; glyph: string; name: string; after: string; endsGroup: boolean }
  | { kind: "add"; key: "+" };

// entryKey names an item for the highlight: "u" for Untagged, "t:" a
// text, "g:" a tag.
export const entryKey = (i: FilterItem): string => (i.Untagged ? "u" : i.Text ? "t:" + i.Name : "g:" + i.Name);

// panelRows is what the panel lists, as the TUI's panelRows: the entries
// less those folded away, then + Text.
export function panelRows(entries: FilterEntry[]): PanelRow[] {
  const shown = entries.filter((e) => !e.FoldedAway);
  const rows: PanelRow[] = shown.map((e, i) => {
    const it = e.Item;
    const name = it.Untagged ? str.filterUntagged() : it.Text ? str.filterTextItem(it.Name) : it.Name.slice(it.Name.lastIndexOf("/") + 1);
    const glyph = it.Text ? str.glyph.remove : e.Parent ? (e.Folded ? str.glyph.collapsed : str.glyph.open) : "";
    const after = e.Parent && e.Folded && e.FilteredUnder ? str.glyph.filtered : "";
    return { kind: "entry", key: entryKey(it), e, glyph, name, after, endsGroup: (shown[i + 1]?.Depth ?? 0) === 0 };
  });
  return [...rows, { kind: "add", key: "+" }];
}

// fixSel is sel if the panel still lists it, else the first entry.
export function fixSel(rows: PanelRow[], sel: string): string {
  return rows.some((r) => r.key === sel) ? sel : rows[0].key;
}

// moveSel is the entry delta from sel, stopping at the ends.
export function moveSel(rows: PanelRow[], sel: string, delta: number): string {
  const i = Math.max(0, rows.findIndex((r) => r.key === sel));
  return rows[Math.min(Math.max(0, i + delta), rows.length - 1)].key;
}

// PanelKey is what a key does in the panel.
export type PanelKey = "up" | "down" | "hide" | "only" | "shut" | "open" | "add" | "remove" | "close";

const panelKeys: Record<string, PanelKey> = {
  ArrowUp: "up", ArrowDown: "down", h: "hide", o: "only", ArrowLeft: "shut", ArrowRight: "open",
  Enter: "add", x: "remove", Delete: "remove", Escape: "close", f: "close",
};

// filterKey is a key in the panel, as the TUI's panelKey: arrows, not
// j/k (spec: the web has no cursor keys). A key with a modifier is the
// browser's.
export function filterKey(e: { key: string; ctrlKey: boolean; altKey: boolean; metaKey: boolean }): PanelKey | undefined {
  if (e.ctrlKey || e.altKey || e.metaKey) return undefined;
  return panelKeys[e.key];
}
```

- [ ] **Step 4: Run the tests and type check**

Run: `cd web/ui && npm run check && npm test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/ui/src/filter.ts web/ui/src/filter.test.ts
git commit -m "web/ui: the filter panel's rules, as the TUI's"
```

---

### Task 2: The store keeps each character's panel; `f`, Esc and the panel's keys act

**Files:**
- Modify: `web/ui/src/store.ts` (`LogView.panel`, `Prompt`), `web/ui/src/filter.ts` (`panelAct`, `togglePanel`, `closePanel`), `web/ui/src/log.ts` (`f`, Esc)
- Test: `web/ui/src/store.test.ts`

**Interfaces:**
- Consumes: Task 1's `panelRows`, `fixSel`, `moveSel`, `PanelKey`; the store's `logShows`, `setLogView`.
- Produces:
  - `LogView.panel?: { sel: string; follow?: boolean }`: the panel is open, with its highlight. `follow` asks the panel to move focus to the highlight on its next draw, as the TUI's `filterPanel.follow` scrolls to it; FilterPanel clears it.
  - `Prompt` gains `kind: "filter"`, the filter text prompt.
  - `togglePanel(store)`: opens the panel on its first entry (`follow`), or closes it. It does nothing unless log mode shows.
  - `closePanel(store)`: closes it, and focuses the log.
  - `panelAct(store, does: PanelKey)`: does what a panel key asks, on the highlighted entry.
  - `LogKey` gains `"filter"` (`f`). `logAct(store, "filter")` toggles the panel. `logAct(store, "back")` closes the panel if it's open (after stopping a search), else log mode.

- [ ] **Step 1: Write the failing tests** (append to `store.test.ts`; `logged` is the helper already there. Add `panelAct, togglePanel` from `./filter.ts` to the imports.)

```ts
// filtered is logged with a Filter: Untagged, a tag page with a child
// page/ooc, and a text row "x".
const filtered = (log: object = {}) => {
  const r = logged(log);
  const E = (Name: string, more: object = {}) => ({ Item: { Name, Text: false, Untagged: false }, Depth: 0, Parent: false, FoldedAway: false, Hidden: false, Only: false, Folded: false, FilteredUnder: false, ...more });
  r.s.apply([{ type: "Filter", Key: "fm/kit", Entries: [E("", { Item: { Name: "", Text: false, Untagged: true } }), E("page", { Parent: true }), E("page/ooc", { Depth: 1 }), E("x", { Item: { Name: "x", Text: true, Untagged: false } })] }]);
  return r;
};

test("f opens the panel on its first entry, focus following; f again closes it", () => {
  const { s } = filtered();
  logAct(s, "filter");
  assert.deepEqual(s.state.logView["fm/kit"].panel, { sel: "u", follow: true });
  logAct(s, "filter");
  assert.equal(s.state.logView["fm/kit"].panel, undefined);
});

test("the panel's keys act on the highlighted entry, as the TUI's", () => {
  const { s, sent } = filtered();
  togglePanel(s);
  panelAct(s, "down");
  assert.deepEqual(s.state.logView["fm/kit"].panel, { sel: "g:page", follow: true });
  panelAct(s, "hide");
  assert.deepEqual(sent.at(-1), { Do: "hide", Key: "fm/kit", Item: { Name: "page", Text: false, Untagged: false } });
  panelAct(s, "only");
  assert.deepEqual(sent.at(-1), { Do: "only", Key: "fm/kit", Item: { Name: "page", Text: false, Untagged: false } });
  panelAct(s, "shut");
  assert.deepEqual(sent.at(-1), { Do: "fold", Key: "fm/kit", Tag: "page", Shut: true });
  panelAct(s, "open");
  assert.deepEqual(sent.at(-1), { Do: "fold", Key: "fm/kit", Tag: "page", Shut: false });
  const n = sent.length;
  panelAct(s, "remove");
  panelAct(s, "add");
  assert.equal(sent.length, n, "a tag isn't removed; Enter only adds on + Text");
  assert.equal(s.state.logView["fm/kit"].prompt, undefined);
});

test("x on a text row removes it, the highlight moving down first; Enter on + Text asks for a text", () => {
  const { s, sent } = filtered();
  s.setLogView("fm/kit", { panel: { sel: "t:x" } });
  panelAct(s, "remove");
  assert.deepEqual(sent.at(-1), { Do: "remove_text", Key: "fm/kit", Term: "x" });
  assert.equal(s.state.logView["fm/kit"].panel?.sel, "+");
  panelAct(s, "hide");
  assert.equal(sent.at(-1).Do, "remove_text", "+ Text has no Hide");
  panelAct(s, "add");
  assert.deepEqual(s.state.logView["fm/kit"].prompt, { kind: "filter", value: "" });
});

test("Esc in the log closes the panel first, then log mode", () => {
  const { s, sent } = filtered();
  togglePanel(s);
  logAct(s, "back");
  assert.equal(s.state.logView["fm/kit"].panel, undefined);
  assert.equal(sent.filter((a) => a.Do === "log_close").length, 0);
  logAct(s, "back");
  assert.deepEqual(sent.at(-1), { Do: "log_close", Key: "fm/kit" });
});

test("each character's panel is its own; Ctrl+L keeps it as it was", () => {
  const { s } = filtered();
  s.apply([{ type: "Char", Key: "fm/rook", World: "fm", ID: "rook", Name: "Rook", State: 2 }, { type: "Log", Key: "fm/rook", Open: true, Find: "", Selected: 0 }]);
  s.setLogView("fm/kit", { panel: { sel: "g:page" } });
  assert.equal(s.state.logView["fm/rook"]?.panel, undefined);
  toggleLog(s);
  toggleLog(s);
  assert.deepEqual(s.state.logView["fm/kit"].panel, { sel: "g:page" }, "hidden and shown again: as it was, focus left with the log");
});

test("not in log mode, the panel doesn't open", () => {
  const { s } = filtered();
  s.setLogView("fm/kit", { hidden: true });
  togglePanel(s);
  assert.equal(s.state.logView["fm/kit"].panel, undefined);
});
```

- [ ] **Step 2: Run to see them fail**

Run: `cd web/ui && npm test`
Expected: FAIL (bundle error: no `panelAct` export).

- [ ] **Step 3: Implement**

`store.ts`:
- `Prompt` becomes `{ kind: "find" | "date" | "filename" | "filter"; value: string; format?: string } | { kind: "format" }`.
- `LogView` gains:

```ts
  panel?: { sel: string; follow?: boolean }; // the filter panel, open, and its highlight (filter.ts)
```

and its doc comment adds "…, and the filter panel, open with its highlight".

`filter.ts` gains (add `import { logShows } from "./store.ts"; import type { Store } from "./store.ts";`):

```ts
// togglePanel is f, or the Filter chip, as the TUI's togglePanel: the
// panel opens on its first entry, focus going to it, or closes. Only in
// log mode.
export function togglePanel(store: Store): void {
  const s = store.state, k = s.active;
  if (!logShows(s, k)) return;
  if (s.logView[k]?.panel) {
    closePanel(store);
    return;
  }
  store.setLogView(k, { panel: { sel: panelRows(s.filters[k]?.Entries ?? [])[0].key, follow: true } });
}

// closePanel closes the panel, focus going back to the log.
export function closePanel(store: Store): void {
  store.setLogView(store.state.active, { panel: undefined });
  globalThis.document?.querySelector<HTMLElement>(".log")?.focus();
}

// panelAct does what a panel key asks, as the TUI's panelKey: ↑/↓ move
// the highlight (focus following); h and o hide or show only its entry;
// ←/→ fold a tag shut or open; Enter on + Text asks for a text; x
// removes a text row, the highlight moving down first so one stays; Esc
// and f close the panel.
export function panelAct(store: Store, does: PanelKey): void {
  const s = store.state, k = s.active, p = s.logView[k]?.panel;
  if (!p) return;
  const rows = panelRows(s.filters[k]?.Entries ?? []);
  const sel = fixSel(rows, p.sel);
  const r = rows.find((r) => r.key === sel)!;
  const act = s.act ?? (() => {});
  const move = (to: string) => store.setLogView(k, { panel: { sel: to, follow: true } });
  switch (does) {
    case "up": move(moveSel(rows, sel, -1)); break;
    case "down": move(moveSel(rows, sel, 1)); break;
    case "hide":
    case "only":
      if (r.kind === "entry") act({ Do: does, Key: k, Item: r.e.Item });
      break;
    case "shut":
    case "open":
      if (r.kind === "entry" && !r.e.Item.Text) act({ Do: "fold", Key: k, Tag: r.e.Item.Name, Shut: does === "shut" });
      break;
    case "add":
      if (r.kind === "add") store.setLogView(k, { prompt: { kind: "filter", value: "" } });
      break;
    case "remove":
      if (r.kind === "entry" && r.e.Item.Text) {
        move(moveSel(rows, sel, 1));
        act({ Do: "remove_text", Key: k, Term: r.e.Item.Name });
      }
      break;
    case "close": closePanel(store); break;
  }
}
```

`log.ts`:
- `LogKey` gains `"filter"`; `letters` gains `f: "filter"`.
- `import { closePanel, togglePanel } from "./filter.ts";`
- `logAct` gains `case "filter": togglePanel(store); break;` and `back` becomes:

```ts
    case "back":
      if (g.Searching) act({ Do: "stop_search", Key: k });
      else if (v.panel) closePanel(store); // the panel first, as Esc backs out of one thing at a time
      else act({ Do: "log_close", Key: k });
      break;
```

- Update `logAct`'s comment: "…; f opens or closes the filter panel; Esc stops a search, else closes the panel, else log mode."
- `log.test.ts`'s letters test gains `"f"` → `"filter"` in both lists.

- [ ] **Step 4: Run the tests and type check**

Run: `cd web/ui && npm run check && npm test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/ui/src
git commit -m "web/ui: f opens the filter panel; its keys act, as the TUI's"
```

---

### Task 3: The panel in the sidebar (`FilterPanel.tsx`)

**Files:**
- Create: `web/ui/src/ui/FilterPanel.tsx`
- Modify: `web/ui/src/ui/Sidebar.tsx`, `web/ui/src/kiln.css`, `internal/str/locales/en.toml` (`web.filter`)
- Test: `web/ui/src/ui/FilterPanel.test.tsx`

**Interfaces:**
- Consumes: Task 1's `panelRows`, `fixSel`, `filterKey`; Task 2's `panelAct`, `LogView.panel`.
- Produces: `FilterPanel({ state, store })`, which `Sidebar` shows in place of the characters while log mode shows with its panel open. It's built from these elements:
  - **The panel:** `<nav class="side filter-panel …filter role" aria-label={str.webFilter()}>`, holding a `<ul>`.
  - **An entry:** `<li class="fentry" role="group" aria-labelledby="flt-<i>">`, with two rows:
    - **the name row:** the indent, the glyph (an `aria-hidden` span, clickable), and the name, a `<button id="flt-<i>">` with `aria-expanded` on a parent;
    - **the buttons row:** the indent plus one space, then Hide and Only, each a `<button aria-pressed>`.
  - **A blank `<li aria-hidden="true">`** after an entry that ends a group.
  - **`+ Text`:** `<li class="fentry"><button>…</button></li>`, last.
- **Roving focus:** the highlighted entry's name, Hide and Only (or `+ Text`) have `tabindex=0`; everything else has `-1`. The highlighted entry's name carries the class `fsel`.
- **Clicks, as the TUI's `panelClick`:**
  - a name highlights its entry;
  - a parent's glyph folds it;
  - a text row's × removes it;
  - Hide and Only press, and highlight their entry;
  - `+ Text` asks for a text.

- [ ] **Step 1: The string** (`[web]`, after `log`)

```toml
# The filter panel's name for screen readers: the list of tags and texts
# log mode can hide or show only.
filter = "Filter"
```

Run `go generate ./internal/str && go test ./internal/str`. (`view.filter_button` reads the same; it's the chip's text, this is a region's name, and either may be reworded on its own.)

- [ ] **Step 2: Write the failing tests** (`ui/FilterPanel.test.tsx`)

```tsx
import { test } from "node:test";
import assert from "node:assert/strict";
import { render } from "preact-render-to-string";
import { App } from "./App.tsx";
import { Store } from "../store.ts";
import { roleClass } from "../theme.ts";
import * as str from "../str.gen.ts";

const E = (Name: string, more: object = {}) => ({ Item: { Name, Text: false, Untagged: false }, Depth: Name.split("/").length - 1, Parent: false, FoldedAway: false, Hidden: false, Only: false, Folded: false, FilteredUnder: false, ...more });
const page = (entries: object[], sel = "u") => {
  const s = new Store();
  s.set({ phase: "running", act: () => {} });
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" },
    { type: "Log", Key: "fm/kit", Open: true, Find: "", Selected: 0, HistDone: true }, { type: "Filter", Key: "fm/kit", Entries: entries }]);
  s.setLogView("fm/kit", { panel: { sel } });
  const html = render(<App store={s} />);
  return html.slice(html.indexOf("<nav"), html.indexOf("</nav>"));
};
const entries = [E("", { Item: { Name: "", Text: false, Untagged: true } }), E("page", { Parent: true, Hidden: true }), E("page/ooc", { Only: true }), E("x", { Item: { Name: "x", Text: true, Untagged: false }, Depth: 0 })];

test("the panel takes the sidebar's place, named Filter, in the filter role", () => {
  const nav = page(entries);
  assert.match(nav, new RegExp(`<nav class="side filter-panel ${roleClass("filter")}" aria-label="${str.webFilter()}"`));
});

test("each entry: a group named by its name, Hide and Only as toggle buttons; indented by depth", () => {
  const nav = page(entries, "g:page");
  assert.match(nav, /<li class="fentry" role="group" aria-labelledby="flt-1">/);
  assert.match(nav, new RegExp(`<button id="flt-1" class="fname fsel ${roleClass("filter.selected")}" tabindex="0" aria-expanded="true">page</button>`));
  assert.match(nav, new RegExp(`<button class="${roleClass("filter.button.on")}" tabindex="0" aria-pressed="true">${str.filterHide()}</button>`), "page is hidden: Hide lit, pressed");
  assert.match(nav, new RegExp(`<button id="flt-2" class="fname ${roleClass("filter.item")}" tabindex="-1">ooc</button>`), "not highlighted: out of the tab order");
  assert.ok(nav.includes(`<div class="frow">   <button id="flt-2"`), "page/ooc: 1 + 2 spaces in");
  assert.match(nav, new RegExp(`tabindex="-1" aria-pressed="true">${str.filterOnly()}</button>`), "ooc is Only");
});

test("glyphs and + Text: ▼ on an open parent, × on a text row; + Text last, in filter.add", () => {
  const nav = page(entries);
  assert.ok(nav.includes(`<span aria-hidden="true" class="fglyph">${str.glyph.open}</span>`));
  assert.ok(nav.includes(`<span aria-hidden="true" class="fglyph">${str.glyph.remove}</span> <button id="flt-3"`));
  assert.ok(nav.includes(str.filterTextItem("x")));
  assert.match(nav, new RegExp(`<button class="fname ${roleClass("filter.add")}" tabindex="-1">${str.filterAddText().replace("+", "\\+")}</button></li></ul>$`));
});

test("a highlight Go no longer lists falls back to the first entry", () => {
  const nav = page(entries, "t:gone");
  assert.match(nav, /<button id="flt-0" class="fname fsel/);
});
```

- [ ] **Step 3: Run to see them fail**

Run: `cd web/ui && npm test`
Expected: FAIL: the sidebar is still the characters.

- [ ] **Step 4: Implement**

`ui/FilterPanel.tsx`:

```tsx
import { useLayoutEffect, useRef } from "preact/hooks";
import { filterKey, fixSel, panelAct, panelRows } from "../filter.ts";
import type { State, Store } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// FilterPanel is log mode's filter panel in the sidebar, as the TUI's:
// each entry its name (indented by depth; ▼ ► on a group, × on a text
// row) and its Hide and Only, a blank row after each top-level group, +
// Text last. Hide and Only are toggle buttons; each entry is a group
// named by its entry, so they're read with it. The highlight is the
// roving focus: ↑/↓ move it, and only its entry's controls are in the
// tab order. Its keys work while it has focus (filter.ts).
export function FilterPanel({ state, store }: { state: State; store: Store }) {
  const k = state.active;
  const p = state.logView[k]!.panel!;
  const rows = panelRows(state.filters[k]?.Entries ?? []);
  const sel = fixSel(rows, p.sel);
  const nav = useRef<HTMLElement>(null);
  const act = state.act ?? (() => {});

  // Focus goes to the highlight when asked (opening, a key's move), as
  // the TUI's follow scrolls to it; never on a draw Go caused.
  useLayoutEffect(() => {
    if (!p.follow) return;
    nav.current?.querySelector<HTMLElement>(".fsel")?.focus();
    store.setLogView(k, { panel: { sel, follow: false } });
  });

  const pick = (key: string) => store.setLogView(k, { panel: { sel: key } });
  const onKeyDown = (e: KeyboardEvent) => {
    const does = filterKey(e);
    if (!does) return;
    e.preventDefault();
    panelAct(store, does);
  };

  return (
    <nav ref={nav} class={`side filter-panel ${roleClass("filter")}`} aria-label={str.webFilter()} onKeyDown={onKeyDown}>
      <ul>
        {rows.map((r, i) => {
          const on = r.key === sel;
          const tab = on ? 0 : -1;
          if (r.kind === "add") {
            return (
              <li key="+" class="fentry">
                {" "}
                <button class={`fname ${on ? "fsel " : ""}${roleClass(on ? "filter.selected" : "filter.add")}`} tabindex={tab}
                  onClick={() => { pick("+"); store.setLogView(k, { prompt: { kind: "filter", value: "" } }); }}>{str.filterAddText()}</button>
              </li>
            );
          }
          const { e } = r;
          const pad = " ".repeat(1 + 2 * e.Depth);
          const id = `flt-${i}`;
          // The glyph, as the TUI's panelClick: a group's folds it, a text
          // row's removes it.
          const onGlyph = () => {
            pick(r.key);
            if (e.Item.Text) panelAct(store, "remove");
            else act({ Do: "fold", Key: k, Tag: e.Item.Name, Shut: !e.Folded });
          };
          return [
            <li key={r.key} class="fentry" role="group" aria-labelledby={id}>
              <div class="frow">
                {pad}
                {r.glyph && <><span aria-hidden="true" class="fglyph" onClick={onGlyph}>{r.glyph}</span>{" "}</>}
                <button id={id} class={`fname ${on ? "fsel " : ""}${roleClass(on ? "filter.selected" : "filter.item")}`} tabindex={tab}
                  aria-expanded={e.Parent ? !e.Folded : undefined} onClick={() => pick(r.key)}>{r.name}</button>
                {r.after && <span aria-hidden="true">{" " + r.after}</span>}
              </div>
              <div class="frow">
                {pad + " "}
                <button class={roleClass(e.Hidden ? "filter.button.on" : "filter.button")} tabindex={tab} aria-pressed={e.Hidden}
                  onClick={() => { pick(r.key); act({ Do: "hide", Key: k, Item: e.Item }); }}>{str.filterHide()}</button>
                {" "}
                <button class={roleClass(e.Only ? "filter.button.on" : "filter.button")} tabindex={tab} aria-pressed={e.Only}
                  onClick={() => { pick(r.key); act({ Do: "only", Key: k, Item: e.Item }); }}>{str.filterOnly()}</button>
              </div>
            </li>,
            r.endsGroup && <li key={r.key + "\n"} aria-hidden="true"> </li>,
          ];
        })}
      </ul>
    </nav>
  );
}
```

`Sidebar.tsx`: first line of `Sidebar`:

```tsx
  if (logShows(state, state.active) && state.logView[state.active]?.panel) return <FilterPanel state={state} store={store} />;
```

Import `logShows` from `../store.ts` and `FilterPanel`. Update the comment's "later, in log mode, the filter panel" to "in log mode with the filter panel open (f), the panel".

`kiln.css`, after the picker's rows:

```css
/* The filter panel, as the TUI's: rows of text, each a block (a flex row
   would drop the indent, a run of spaces alone), cut with … when long. */
.side li.fentry, .filter-panel li { display: block; }
.side .frow { white-space: pre; overflow: hidden; text-overflow: ellipsis; }
.side .fglyph { cursor: pointer; }
```

The assertion attribute order follows the JSX (`id`, `class`, `tabindex`, `aria-expanded`), as preact-render-to-string writes them.

- [ ] **Step 5: Run the tests and type check**

Run: `cd web/ui && npm run check && npm test`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/str web/ui/src
git commit -m "web/ui: the filter panel in the sidebar, as the TUI's

Strings: web.filter (new)"
```

---

### Task 4: The filter prompt, the panel's hints, and the `Filter` chip

**Files:**
- Modify: `web/ui/src/ui/LogBar.tsx`, `web/ui/src/ui/TopBar.tsx`, `internal/str/locales/en.toml` (`web.log_hints`)
- Test: `web/ui/src/ui/LogView.test.tsx` (append)

**Interfaces:**
- Consumes: Task 2's `Prompt` kind `filter`, `togglePanel`, `LogView.panel`.
- Produces:
  - **The filter prompt** (`str.filterPrompt()` and a field). Enter sends `add_text` with the term. A term that isn't empty becomes the highlight (`t:<term>`, following), as the TUI's. Either way, focus goes back to the panel.
  - **Esc in the filter prompt** goes back to the panel.
  - **The hint row** shows `str.filterHints()` while the panel is open, else `str.webLogHints()`.
  - **The `Filter` chip** sits between presence and `Log` while log mode shows: `status.filter`, or `status.filter.on` while the panel is open. A click calls `togglePanel`.

- [ ] **Step 1: The string** (`[web]` `log_hints`, changed: `f filter` after `g date`, as the TUI's `browse.hints`)

```toml
log_hints = "click to select · shift-click extends · / find · n/N · g date · f filter · t tags · e export · c copy · esc back"
```

Run `go generate ./internal/str && go test ./internal/str`.

- [ ] **Step 2: Write the failing tests** (append to `ui/LogView.test.tsx`; `pageWith` and `bottom` are there)

```tsx
test("with the panel open: the panel's hints; the filter prompt's label and field", () => {
  assert.ok(bottom(pageWith({}, { panel: { sel: "+" } })).includes(str.filterHints()));
  assert.match(bottom(pageWith({}, { panel: { sel: "+" }, prompt: { kind: "filter", value: "y" } })), new RegExp(`<label[^>]*>${str.filterPrompt()}</label><input[^>]*value="y"`));
});

test("the Filter chip, between presence and Log in log mode, lit while the panel's open", () => {
  const top = (html: string) => html.slice(html.indexOf("<header"), html.indexOf("</header>"));
  assert.match(top(pageWith({})), new RegExp(`<button class="${roleClass("status.filter")}"[^>]*>${str.viewFilterButton()}</button><span>${str.separator()}</span><button class="${roleClass("status.log.on")}"`));
  assert.match(top(pageWith({}, { panel: { sel: "+" } })), new RegExp(`<button class="${roleClass("status.filter.on")}"`));
});

test("Enter in the filter prompt adds the text, which becomes the highlight; Esc goes back to the panel", () => {
  const sent: any[] = [];
  const s = new Store();
  s.set({ phase: "running", act: (o: object) => sent.push(o) });
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" }, { type: "Log", Key: "fm/kit", Open: true, Find: "", Selected: 0 }]);
  s.setLogView("fm/kit", { panel: { sel: "+" }, prompt: { kind: "filter", value: " pie " } });
  filterPromptEnter(s);
  assert.deepEqual(sent.at(-1), { Do: "add_text", Key: "fm/kit", Term: "pie" });
  assert.deepEqual(s.state.logView["fm/kit"].panel, { sel: "t:pie", follow: true });
  assert.equal(s.state.logView["fm/kit"].prompt, undefined);
});
```

Add `filterPromptEnter` to the `../log.ts` import in this file. (It's the prompt's Enter for the filter kind, pulled out of LogBar so it's testable without a DOM. See Step 4.)

- [ ] **Step 3: Run to see them fail**

Run: `cd web/ui && npm test`
Expected: FAIL (bundle error: no `filterPromptEnter`).

- [ ] **Step 4: Implement**

`log.ts`:

```ts
// filterPromptEnter is Enter in the filter text prompt, as the TUI's:
// the text is added, a new row becoming the highlight, and focus goes
// back to the panel.
export function filterPromptEnter(store: Store): void {
  const s = store.state, k = s.active, v = s.logView[k];
  if (v?.prompt?.kind !== "filter") return;
  const term = v.prompt.value.trim();
  s.act?.({ Do: "add_text", Key: k, Term: term });
  store.setLogView(k, { prompt: undefined, panel: { sel: term ? "t:" + term : v.panel?.sel ?? "+", follow: true } });
}
```

`LogBar.tsx`:
- **The hints:** `{v.panel ? str.filterHints() : str.webLogHints()}`.
- **The label:** `p.kind === "filter" ? str.filterPrompt() : …`.
- **`back`** sends the filter prompt back to the panel, not the log:

```ts
  const back = () => {
    if (p?.kind === "filter") {
      store.setLogView(k, { prompt: undefined, panel: { sel: v.panel?.sel ?? "+", follow: true } });
      return;
    }
    store.setLogView(k, { prompt: undefined });
    document.querySelector<HTMLElement>(".log")?.focus();
  };
```

- **Enter** for the filter kind: `if (p.kind === "filter") { filterPromptEnter(store); return; }` before the other kinds, so the prompt's own `back()` doesn't run after it.

`TopBar.tsx`: between the presence chip's separator and the `Log` chip:

```tsx
      {shows && <button class={roleClass(state.logView[state.active]?.panel ? "status.filter.on" : "status.filter")} onClick={() => togglePanel(store)}>{str.viewFilterButton()}</button>}
      {shows && <span>{str.separator()}</span>}
```

Import `togglePanel` from `../filter.ts`. Update the comment: "…then, in log mode, the Filter chip, lit while the filter panel's open, which opens and closes it as f does; then the Log chip…".

- [ ] **Step 5: Run the tests and type check**

Run: `cd web/ui && npm run check && npm test`
Expected: PASS. The 5a test that checks the hint row reads `str.webLogHints()`, so it follows the new text.

- [ ] **Step 6: Commit**

```bash
git add internal/str web/ui/src
git commit -m "web/ui: the filter prompt, the panel's hints, and the Filter chip

Strings: web.log_hints (changed: f filter)"
```

---

### Task 5: Your place kept when the filter changes

**Files:**
- Modify: `web/ui/src/log.ts` (`topFor`, `keepsPlace`), `web/ui/src/ui/LogView.tsx`
- Test: `web/ui/src/log.test.ts`

**Interfaces:**
- Produces:
  - `topFor(p, rows)`: when p's line is gone (hidden), it now puts the next line still shown at the view's top. Rows are in line order, and line numbers rise down the log.
  - `keepsPlace(was: { first?: number; hidden: string }, now: { first?: number; hidden: string }): boolean`: whether a draw changed what's above your place: older lines arrived, or the filter changed what's hidden. `hidden` is the `Log` event's `Hidden` joined with commas.

- [ ] **Step 1: Write the failing tests** (append to `log.test.ts`; import `keepsPlace`)

```ts
test("place: a line hidden by the filter gives way to the next one shown", () => {
  const rows = [{ seq: 5, top: 0, height: 20 }, { seq: 7, top: 20, height: 20 }];
  assert.equal(topFor({ seq: 6, off: -10 }, rows), 20, "6 hidden: 7 at the top");
  assert.equal(topFor({ seq: 9, off: 0 }, rows), undefined, "nothing after it: nothing to keep");
});

test("the place is put back when older lines arrive or the filter changes what's hidden; not otherwise", () => {
  assert.equal(keepsPlace({ first: 1, hidden: "" }, { first: 0, hidden: "" }), true);
  assert.equal(keepsPlace({ first: 1, hidden: "" }, { first: 1, hidden: "3,4" }), true);
  assert.equal(keepsPlace({ first: 1, hidden: "3" }, { first: 1, hidden: "3" }), false);
  assert.equal(keepsPlace({ first: undefined, hidden: "" }, { first: 1, hidden: "" }), false, "the first draw: nowhere to keep");
});
```

The 5a test `"gone: nothing to keep"` (`topFor({ seq: 99, off: 0 }, after)`) still holds: no line comes after 99.

- [ ] **Step 2: Run to see them fail**

Run: `cd web/ui && npm test`
Expected: FAIL (bundle error: no `keepsPlace`).

- [ ] **Step 3: Implement**

`log.ts`:

```ts
export function topFor(p: Place, rows: { seq: number; top: number }[]): number | undefined {
  const r = rows.find((r) => r.seq === p.seq);
  if (r) return r.top - p.off;
  return rows.find((r) => r.seq > p.seq)?.top; // hidden: the next line shown, at the top
}

// keepsPlace reports whether a draw changed what's above your place, so
// it must be put back: older lines arrived, or the filter changed what's
// hidden. Not on the first draw.
export function keepsPlace(was: { first?: number; hidden: string }, now: { first?: number; hidden: string }): boolean {
  return was.first !== undefined && (was.first !== now.first || was.hidden !== now.hidden);
}
```

Update `topFor`'s comment: "…; undefined if its line is gone and nothing follows it. A hidden line gives way to the next one shown."

`LogView.tsx`, the place effect:

```tsx
  // Older history arriving above, or the filter hiding lines, keeps your
  // place, as the scrollback's.
  useLayoutEffect(() => {
    const e = el.current, p = place.current, now = { first: lines[0]?.Seq, hidden: (g.Hidden ?? []).join(",") };
    if (e && p.k === k && p.at && keepsPlace(p, now)) {
      const top = topFor(p.at, rowsOf(e));
      if (top !== undefined) e.scrollTop = top;
    }
    place.current = { k, ...now, at: p.k === k ? p.at : undefined };
  });
```

The `place` ref's type becomes `{ k: string; first?: number; hidden: string; at?: Place }`, initialised with `{ k, hidden: "" }`.

- [ ] **Step 4: Run the tests and type check**

Run: `cd web/ui && npm run check && npm test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/ui/src
git commit -m "web/ui: log mode keeps your place when the filter hides lines"
```

---

### Task 6: End to end, and the docs

**Files:**
- Modify: `web/smoke/log_test.go` (`TestFilter`), `docs/web-dev.md`

**Interfaces:**
- Consumes: the smoke helpers. `TestLog` (in `log_test.go`) shows how to reach log mode with Second: `ctrlO`, the filter "sec", Enter, then its `ctrlL`. Lift `ctrlL` to a file-level helper `ctrlL(t, ws)` and use it in both tests.

The fake MUCK's lines in Second's log are two sys lines (connected; no saved password) and "Welcome to the fake MUCK". None has a tag, so the panel lists `Untagged`, then `+ Text`.

- [ ] **Step 1: Write the smoke test** (append to `log_test.go`)

```go
// TestFilter uses the filter panel the way a person does: f, + Text,
// a text typed (its f and o never reaching the panel), Only on it, x,
// Esc; the Filter chip; Ctrl+L keeping it.
func TestFilter(t *testing.T) {
	ws, _ := launch(t)
	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Worlds")`, "the worlds")
	ctrlO(t, ws)
	waitFor(t, ev, `document.activeElement === document.querySelector('.bottom input[role="combobox"]')`, "the filter")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "sec"})
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Lines" && e.Key === "fake/second" && e.Lines.some(l => l.Kind === "server"))`, "Second's lines")
	ctrlL(t, ws)
	logEl := `document.querySelector(".log")`
	waitFor(t, ev, `document.activeElement === `+logEl+` && document.querySelectorAll(".lrow").length === 3`, "log mode, three lines")
	letter := func(k string) {
		devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": k, "text": k})
		devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": k})
	}

	// f: the panel, focus on its first entry, its hints below.
	letter("f")
	sel := `document.querySelector(".filter-panel .fsel")`
	waitFor(t, ev, `document.activeElement === `+sel+` && `+sel+`.textContent === `+jsString(str.FilterUntagged()), "the panel, Untagged focused")
	waitFor(t, ev, `document.querySelector(".bottom").textContent === `+jsString(str.FilterHints()), "the panel's hints")

	// ↓ to + Text, Enter: the prompt. "fake muck" typed there has an f and
	// no panel key reaches the panel; Enter adds it, the new row focused.
	pressKey(t, ws, "ArrowDown", 40)
	waitFor(t, ev, `document.activeElement === `+sel+` && `+sel+`.textContent === `+jsString(str.FilterAddText()), "+ Text")
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `document.activeElement === document.querySelector("#kiln-logprompt")`, "the filter prompt")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "fake muck"})
	if got := ev(`!!document.querySelector(".filter-panel") && !kilnPage.sent.some(a => a.Do === "only" || a.Do === "hide")`); got != "true" {
		t.Error("typing in the prompt reached the panel")
	}
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `document.activeElement === `+sel+` && `+sel+`.textContent === `+jsString(str.FilterTextItem("fake muck")), "the new text row, focused")

	// o: only lines with the text; o again: all back.
	letter("o")
	waitFor(t, ev, `document.querySelectorAll(".lrow").length === 1`, "only the Welcome line")
	letter("o")
	waitFor(t, ev, `document.querySelectorAll(".lrow").length === 3`, "all three back")

	// x removes the text row, the highlight moving to + Text.
	letter("x")
	waitFor(t, ev, `!kilnPage.events.filter(e => e.type === "Filter").at(-1).Entries.some(e => e.Item.Text) && `+sel+`.textContent === `+jsString(str.FilterAddText()), "removed, + Text highlighted")

	// Esc closes the panel, the log focused; the chip opens it again, lit.
	pressKey(t, ws, "Escape", 27)
	waitFor(t, ev, `!document.querySelector(".filter-panel") && document.activeElement === `+logEl, "closed, the log focused")
	click(t, ws, ev, `[...document.querySelectorAll("header button")].find(b => b.textContent === `+jsString(str.ViewFilterButton())+`)`)
	waitFor(t, ev, `document.querySelector(".filter-panel")`, "the chip opened it")

	// Ctrl+L hides log mode, panel and all; Ctrl+L again: both back.
	ctrlL(t, ws)
	waitFor(t, ev, `!document.querySelector(".log") && !document.querySelector(".filter-panel")`, "hidden, the characters back")
	ctrlL(t, ws)
	waitFor(t, ev, `document.querySelector(".log") && document.querySelector(".filter-panel")`, "back as it was")

	// Esc in the log closes the panel, not log mode.
	ev(logEl + `.focus()`)
	pressKey(t, ws, "Escape", 27)
	waitFor(t, ev, `!document.querySelector(".filter-panel") && document.querySelector(".log")`, "the panel closed, log mode stays")
}
```

Notes for the executor:
- `kilnPage` in `?record` mode keeps `events` and `sent`, as `TestLog` uses.
- Wait for focus before keys: focus is a mount or layout effect.
- If the two sys lines aren't logged (so log mode has one line, not three), take the counts from the first `LogLines`, through the same waits.

- [ ] **Step 2: Run it**

Run: `web/build.sh >/dev/null && cd web && KILN_SMOKE=1 go test -count=1 -run 'TestLog|TestFilter' ./smoke`
Expected: PASS. A failure is a finding: debug before changing the test.

- [ ] **Step 3: Mutation-check two steps**
  1. In `FilterPanel.tsx`, drop `onKeyDown` from the `<nav>`. Expect "+ Text" (the ↓) to fail.
  2. In `log.ts` `logAct`, make `back` skip the panel (`else if (false)`). Expect "the panel closed, log mode stays" to fail.

  Undo each with a reverse edit or a backup copy, never `git checkout`.

- [ ] **Step 4: The docs** (`docs/web-dev.md`): in the log mode paragraph, after the letters, add: "`f` (or the `Filter` chip) opens the filter panel in the sidebar: ↑/↓ move, `h` hides and `o` shows only an entry, ←/→ fold a group, Enter on `+ Text` filters by a text, `x` removes one, Esc closes it." Change "log mode's filter panel is still to come" so it no longer says anything is still to come in log mode.

- [ ] **Step 5: The whole suite**

Run: `go vet ./... && go test ./... && (cd web && go vet ./... && go test ./... && GOOS=js GOARCH=wasm go vet ./...) && (cd web/ui && npm run check && npm test) && web/build.sh >/dev/null && (cd web && KILN_SMOKE=1 go test -count=1 ./smoke)`
Expected: PASS, every package and every smoke test.

- [ ] **Step 6: Commit**

```bash
git add web/smoke docs/web-dev.md
git commit -m "web: the filter panel end to end; web-dev.md says how to use it"
```
