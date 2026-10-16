// The filter panel's rules, as the TUI's (internal/ui/filter.go): what it
// lists, how each entry reads, and its keys. Go holds the filter and the
// folds (the Filter event); the page holds which entry is highlighted.
import { logShows } from "./store.ts";
import type { Store } from "./store.ts";
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
export type PanelKey = "up" | "down" | "hide" | "only" | "shut" | "open" | "toggle" | "enter" | "remove" | "close" | "log";

const panelKeys: Record<string, PanelKey> = {
  ArrowUp: "up", ArrowDown: "down", k: "up", j: "down", h: "hide", o: "only", ArrowLeft: "shut", ArrowRight: "open",
  " ": "toggle", Enter: "enter", x: "remove", Delete: "remove", Escape: "close", f: "log",
};

// filterKey is a key in the panel, as the TUI's panelKey: arrows or j/k
// move. A key with a modifier is the browser's.
export function filterKey(e: { key: string; ctrlKey: boolean; altKey: boolean; metaKey: boolean }): PanelKey | undefined {
  if (e.ctrlKey || e.altKey || e.metaKey) return undefined;
  return panelKeys[e.key];
}

// focusPanel is f in the log, as the TUI's: focus goes to the panel's
// highlight. Only in log mode.
export function focusPanel(store: Store): void {
  const s = store.state, k = s.active;
  if (!logShows(s, k)) return;
  const sel = fixSel(panelRows(s.filters[k]?.Entries ?? []), s.logView[k]?.panel?.sel ?? "");
  store.setLogView(k, { panel: { sel, follow: true }, focus: "panel" });
}

// focusLog is f or Esc in the panel, as the TUI's: focus goes back to
// the log, the panel staying.
export function focusLog(store: Store): void {
  store.setLogView(store.state.active, { focus: undefined });
  const doc = globalThis.document;
  const log = () => doc?.querySelector<HTMLElement>(".log");
  log()?.focus();
  // Narrow, the log was hidden under the panel until the page draws
  // again, and a hidden element takes no focus: try again then.
  if (doc && doc.activeElement !== log()) setTimeout(() => log()?.focus());
}

// panelAct does what a panel key asks, as the TUI's panelKey: ↑/↓ move
// the highlight (focus following); h and o hide or show only its entry;
// ←/→ fold a tag shut or open, and Space or Enter toggle it; Enter on +
// Text asks for a text; x
// removes a text row, the highlight moving down first so one stays; Esc
// and f give the log focus. Any key stops a search reading older days
// first, as the TUI's: Esc only that.
export function panelAct(store: Store, does: PanelKey): void {
  const s = store.state, k = s.active, p = s.logView[k]?.panel ?? { sel: "" };
  const rows = panelRows(s.filters[k]?.Entries ?? []);
  const sel = fixSel(rows, p.sel);
  const r = rows.find((r) => r.key === sel)!;
  const act = s.act ?? (() => {});
  if (s.logs[k]?.Searching) {
    act({ Do: "stop_search", Key: k });
    if (does === "close") return;
  }
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
    case "enter":
      if (r.kind === "add") {
        store.setLogView(k, { prompt: { kind: "filter", value: "" } });
        break;
      }
    // falls through: Enter on a tag folds it, as Space does
    case "toggle":
      if (r.kind === "entry" && r.e.Parent) act({ Do: "fold", Key: k, Tag: r.e.Item.Name, Shut: !r.e.Folded });
      break;
    case "remove":
      if (r.kind === "entry" && r.e.Item.Text) {
        move(moveSel(rows, sel, 1));
        act({ Do: "remove_text", Key: k, Term: r.e.Item.Name });
      }
      break;
    case "close":
    case "log": focusLog(store); break;
  }
}

// glyphClick is a click on an entry's glyph, as the TUI's panelClick: a
// group's folds it, a text row's removes it. The entry is highlighted.
// A text row's focus follows, since its glyph (a span) takes none; a
// group's glyph is a button and keeps it. With a prompt open it does
// nothing, as the TUI's.
export function glyphClick(store: Store, key: string): void {
  const s = store.state, k = s.active;
  const r = panelRows(s.filters[k]?.Entries ?? []).find((r) => r.key === key);
  if (r?.kind !== "entry" || s.logView[k]?.prompt) return;
  store.setLogView(k, { panel: r.e.Item.Text ? { sel: key, follow: true } : { sel: key } });
  if (r.e.Item.Text) panelAct(store, "remove");
  else s.act?.({ Do: "fold", Key: k, Tag: r.e.Item.Name, Shut: !r.e.Folded });
}

// panelClick is a click on an entry's name (does undefined), its Hide or
// Only, or + Text, as the TUI's panelClick: the entry is highlighted,
// and the button pressed. With a prompt open it does nothing, as the
// TUI's.
export function panelClick(store: Store, key: string, does?: "hide" | "only" | "add"): void {
  const s = store.state, k = s.active;
  if (s.logView[k]?.prompt) return;
  const r = panelRows(s.filters[k]?.Entries ?? []).find((r) => r.key === key);
  if (!r) return;
  store.setLogView(k, { panel: { sel: key } });
  if (does === "add") store.setLogView(k, { prompt: { kind: "filter", value: "" } });
  else if (does && r.kind === "entry") s.act?.({ Do: does, Key: k, Item: r.e.Item });
}
