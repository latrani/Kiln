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

// glyphClick is a click on an entry's glyph, as the TUI's panelClick: a
// group's folds it, a text row's removes it. The entry is highlighted
// and focus follows, since the glyph (a span) takes none.
export function glyphClick(store: Store, key: string): void {
  const s = store.state, k = s.active;
  const r = panelRows(s.filters[k]?.Entries ?? []).find((r) => r.key === key);
  if (r?.kind !== "entry") return;
  store.setLogView(k, { panel: { sel: key, follow: true } });
  if (r.e.Item.Text) panelAct(store, "remove");
  else s.act?.({ Do: "fold", Key: k, Tag: r.e.Item.Name, Shut: !r.e.Folded });
}
