// The sidebar's rows, as the TUI lists them (internal/ui/sidebar.go's
// sidebarRows), and when it shows (internal/ui/collapse.go).
import { focusLog } from "./filter.ts";
import { listing, logShows } from "./store.ts";
import type { State, Store } from "./store.ts";

export type Row = { kind: "world"; world: string } | { kind: "char"; key: string; world: string } | { kind: "gap" } | { kind: "add" };

// sidebarRows lists the open characters under their worlds, in order,
// then a blank row and "+ Connection" (only that, with nothing open). A
// key whose Char event hasn't come yet is left out.
export function sidebarRows(order: string[], chars: Record<string, { World: string }>): Row[] {
  const rows: Row[] = [];
  let world: string | undefined;
  for (const key of order) {
    const c = chars[key];
    if (!c) continue;
    if (c.World !== world) {
      world = c.World;
      rows.push({ kind: "world", world });
    }
    rows.push({ kind: "char", key, world: c.World });
  }
  if (rows.length) rows.push({ kind: "gap" });
  rows.push({ kind: "add" });
  return rows;
}

// sideForced reports whether the sidebar shows only for now, though
// collapsed, as the TUI's: the picker's list is up, or log mode's filter
// panel has focus. Collapsing is what was chosen, so it's collapsed again
// after.
export function sideForced(s: State): boolean {
  if (!s.sidebar?.Collapsed) return false;
  return listing(s) || (logShows(s, s.active) && s.logView[s.active]?.focus === "panel");
}

// sideShown reports whether the sidebar has its column.
export function sideShown(s: State): boolean {
  return !s.sidebar?.Collapsed || sideForced(s);
}

// canToggle reports whether the sidebar can collapse or come back now,
// as the TUI's: not while the picker or an editor is up. ‹ and ≡ show
// only when it can.
export function canToggle(s: State): boolean {
  return !s.picker;
}

// toggleSidebar is Ctrl+B, ‹ and ≡, as the TUI's: collapses the sidebar
// or brings it back; not while the picker or an editor is up.
// Collapsing while log mode's panel has focus gives the log focus.
// Focus that was on what goes away goes to what replaces it, so the
// keyboard keeps its place: from the sidebar to ≡, from ≡ to ‹.
export function toggleSidebar(store: Store): void {
  const s = store.state;
  if (!canToggle(s)) return;
  const shut = !s.sidebar?.Collapsed;
  const doc = globalThis.document;
  const was = doc?.activeElement;
  const from = shut ? was?.closest?.(".side") : was?.closest?.("header .menu");
  if (shut && s.logView[s.active]?.focus === "panel") focusLog(store);
  s.act?.({ Do: "sidebar", Shut: shut });
  if (!from) return;
  const move = () => doc.querySelector<HTMLElement>(shut ? "header .menu" : ".stitle button")?.focus();
  if (!!store.state.sidebar?.Collapsed === shut) { // Go answered at once
    setTimeout(move);
    return;
  }
  const stop = store.subscribe(() => { // once Go says so, after the page draws it
    if (!!store.state.sidebar?.Collapsed !== shut) return;
    stop();
    setTimeout(move);
  });
}

// sidebarKey reports whether a keydown is Ctrl+B.
export function sidebarKey(e: { key: string; ctrlKey: boolean; altKey: boolean; metaKey: boolean; shiftKey: boolean }): boolean {
  return e.ctrlKey && !e.altKey && !e.metaKey && !e.shiftKey && e.key.toLowerCase() === "b";
}
