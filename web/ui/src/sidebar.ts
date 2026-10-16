// The sidebar's rows, as the TUI lists them (internal/ui/sidebar.go's
// sidebarRows), and when it shows (internal/ui/collapse.go).
import { focusLog, focusPanel } from "./filter.ts";
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
  if (s.narrow || !s.sidebar?.Collapsed) return false; // narrow: ‹ always closes it
  return listing(s) || (logShows(s, s.active) && s.logView[s.active]?.focus === "panel");
}

// narrowShown reports whether the narrow sidebar is open, as the TUI's:
// opened (≡, Ctrl+B), the picker's list up, or in log mode the panel
// with focus (it's open exactly while it has the keys).
export function narrowShown(s: State): boolean {
  if (!s.narrow) return false;
  if (logShows(s, s.active)) return s.logView[s.active]?.focus === "panel";
  return !!s.narrowOpen || listing(s);
}

// sideShown reports whether the sidebar shows: its column, or narrow,
// open over the top bar and scrollback.
export function sideShown(s: State): boolean {
  if (s.narrow) return narrowShown(s);
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
  if (s.narrow) { // a toggle, not saved; in log mode, the panel with the keys, as f
    if (logShows(s, s.active)) {
      if (s.logView[s.active]?.focus === "panel") focusLog(store);
      else focusPanel(store);
      return;
    }
    const open = !s.narrowOpen;
    store.set({ narrowOpen: open });
    // Focus goes in when it opens, and back to the input when it closes.
    setTimeout(() => globalThis.document?.querySelector<HTMLElement>(open ? ".side .stitle button" : ".bottom textarea")?.focus());
    return;
  }
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

// doubleClick is the longest gap between a double-click's clicks, as the
// TUI's.
const doubleClick = 400;

// swallowsClick reports whether a click outside the sidebar is a
// double-click's second, landing on what was under the narrow sidebar
// its first click picked from and closed, as the TUI's: it does nothing.
export function swallowsClick(s: State, now: number, inSidebar: boolean): boolean {
  return !inSidebar && s.narrowPicked !== undefined && now - s.narrowPicked <= doubleClick;
}

// becomeNarrow records the page going narrow or wide. Narrow starts
// closed: the narrow sidebar, and in log mode the panel (which would
// otherwise open over the log), as the TUI's.
export function becomeNarrow(store: Store, narrow: boolean): void {
  const s = store.state;
  if (narrow === !!s.narrow) return;
  store.set({ narrow, narrowOpen: false });
  if (narrow && logShows(s, s.active) && s.logView[s.active]?.focus === "panel") focusLog(store);
}
