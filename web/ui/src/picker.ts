// The picker's rows and highlight, as the TUI's (internal/ui/picker.go):
// every configured character that isn't open, under its world, each
// world ending with a row to add a character. Go sends the worlds in
// this order (feed.worlds). A world's own row opens its editor. A final
// row adds a world, as the TUI's, but only for a host that can (worlds):
// on the web the worlds are the relay's allowlist, which it serves as
// the preset.

import type { Target } from "./editor.ts";
import type { Answer, State, Store } from "./store.ts";

// WorldInfo is a world of the Worlds event.
export interface WorldInfo {
  ID: string;
  Characters?: { ID: string; Name: string; Aliases?: string[] }[];
}

// PickRow is one row of the picker.
export type PickRow =
  | { kind: "world"; world: string }
  | { kind: "char"; world: string; key: string; name: string }
  | { kind: "add_char"; world: string }
  | { kind: "add_world" };

// matches reports whether a character fits the lowercased filter f: its
// name, an alias or its world contains it.
function matches(world: string, c: { Name: string; Aliases?: string[] }, f: string): boolean {
  return !f || [c.Name, world, ...(c.Aliases ?? [])].some((s) => s.toLowerCase().includes(f));
}

// pickerRows lists the characters the picker offers, those not in open,
// under their worlds, each world ending with "+ Character". A world is
// listed when the filter matches it or one of its characters, even with
// no characters to offer. With addWorld the list ends with "+ World",
// whatever the filter, as the TUI's.
export function pickerRows(worlds: WorldInfo[], open: Record<string, unknown>, filter: string, addWorld: boolean): PickRow[] {
  const f = filter.trim().toLowerCase();
  const rows: PickRow[] = [];
  for (const w of worlds) {
    const chars: PickRow[] = (w.Characters ?? [])
      .filter((c) => !open[`${w.ID}/${c.ID}`] && matches(w.ID, c, f))
      .map((c) => ({ kind: "char", world: w.ID, key: `${w.ID}/${c.ID}`, name: c.Name }));
    if (chars.length || w.ID.toLowerCase().includes(f)) rows.push({ kind: "world", world: w.ID }, ...chars, { kind: "add_char", world: w.ID });
  }
  if (addWorld) rows.push({ kind: "add_world" });
  return rows;
}

// selKey is what the highlight holds when r is highlighted: a
// character's key, "=world" for a world's own row (its editor), "+world"
// for adding a character to world, "+" for adding a world.
export function selKey(r: PickRow): string {
  switch (r.kind) {
    case "char": return r.key;
    case "world": return "=" + r.world;
    case "add_char": return "+" + r.world;
    case "add_world": return "+";
  }
}

// pickable is the selection keys of the rows that can be highlighted, in
// order.
export const pickable = (rows: PickRow[]): string[] => rows.map(selKey).filter((k) => k);

// fixPick keeps the highlight on its row if it's still offered, and
// otherwise moves it to the first character, or the first row there is,
// or nothing.
export function fixPick(rows: PickRow[], sel: string): string {
  const keys = pickable(rows);
  if (keys.includes(sel)) return sel;
  return keys.find((k) => k.includes("/")) ?? keys[0] ?? "";
}

// movePick moves the highlight by delta rows, stopping at the ends.
export function movePick(rows: PickRow[], sel: string, delta: number): string {
  const keys = pickable(rows);
  const i = keys.indexOf(sel);
  return i < 0 ? sel : keys[Math.min(Math.max(0, i + delta), keys.length - 1)];
}

// pagePick moves the highlight about page rows (world headers included)
// in direction dir, landing on the last row that can be highlighted
// within that page, or on the next one when the page holds none.
export function pagePick(rows: PickRow[], sel: string, dir: number, page: number): string {
  const cur = rows.findIndex((r) => selKey(r) === sel);
  if (cur < 0) return sel;
  for (let i = Math.min(Math.max(0, cur + dir * page), rows.length - 1); i !== cur; i -= dir) {
    const k = selKey(rows[i]);
    if (k) return k;
  }
  return movePick(rows, sel, dir);
}

// choose acts on the picker's row sel, as Enter or a click does: a
// character opens, connects and shows (pick), closing the picker; "+
// Character" asks for the new character's name; a world's row opens its
// editor, and "+ World" a new world's. An editor open over the picker is left first, its edits kept,
// as the TUI's. Nothing for "".
export function choose(store: Store, sel: string): void {
  store.leaveEditor();
  if (sel.startsWith("=")) store.openEditor({ kind: "world", world: sel.slice(1) }, false);
  else if (sel === "+") store.openEditor({ kind: "new-world" }, false);
  else if (sel.startsWith("+")) store.setPicker({ adding: sel.slice(1), name: "", error: undefined });
  else if (sel) {
    store.state.act?.({ Do: "pick", Key: sel });
    store.closePicker();
  }
}

// PickerKeyish is the part of a keydown pickerKey reads; onBody is
// whether focus was on no control (the page's body).
interface PickerKeyish {
  key: string;
  ctrlKey: boolean;
  altKey: boolean;
  metaKey: boolean;
  shiftKey: boolean;
  onBody: boolean;
}

// pickerKey is what a key does to the picker from anywhere on the page:
// Ctrl+O opens or closes it, as the TUI's; Enter on a world's overview
// (active "=world", which has no input) opens it, as the TUI's, unless
// the picker or an editor is up, which has the keys. Ctrl+O
// where it does nothing (naming a character or in an editor, as the
// TUI's forms, or Kiln not running) is still eaten, so the browser doesn't open a file
// or a line.
export function pickerKey(e: PickerKeyish, s: Pick<State, "active" | "phase" | "picker">): "open" | "close" | "eat" | undefined {
  const plain = !e.ctrlKey && !e.altKey && !e.metaKey && !e.shiftKey;
  if (e.key.toLowerCase() === "o" && e.ctrlKey && !e.altKey && !e.metaKey && !e.shiftKey) {
    if (s.phase !== "running" || s.picker?.adding !== undefined || s.picker?.edit) return "eat";
    return s.picker ? "close" : "open";
  }
  if (e.key === "Enter" && plain && e.onBody && s.active.startsWith("=") && s.phase === "running" && !s.picker) return "open";
  return undefined;
}

// EditorKeyish is a keydown as editorKey reads it: code too, since Alt+T
// on a Mac types a symbol, not "t".
interface EditorKeyish extends PickerKeyish {
  code: string;
}

// splitKey is a character's key as its world and id.
export const splitKey = (k: string): [string, string] => {
  const i = k.indexOf("/");
  return [k.slice(0, i), k.slice(i + 1)];
};

// editorKey is what Ctrl+T or Alt+T does, as the TUI's Ctrl+T: in an
// editor, hide it (its edits kept); in the picker, open the highlighted
// world's or character's editor; elsewhere the active character's, or
// on an overview its world's, closing with the picker. Where it does
// nothing it's still eaten, so the browser doesn't take it (Windows and
// Linux browsers keep Ctrl+T anyway; Alt+T works everywhere).
export function editorKey(e: EditorKeyish, s: Pick<State, "active" | "phase" | "picker" | "chars">): { open: Target; closeAll: boolean } | "hide" | "eat" | undefined {
  const ctrlT = e.ctrlKey && !e.altKey && e.key.toLowerCase() === "t";
  // By the letter where Alt+T types one (any layout); a Mac's Option
  // types a symbol, so there by the key's place.
  const altT = e.altKey && !e.ctrlKey && (/^[a-z]$/i.test(e.key) ? e.key.toLowerCase() === "t" : e.code === "KeyT");
  if (!(ctrlT || altT) || e.metaKey || e.shiftKey) return undefined;
  if (s.phase !== "running" || s.picker?.adding !== undefined) return "eat";
  if (s.picker?.edit) return "hide";
  if (s.picker) {
    const sel = s.picker.sel;
    if (sel.startsWith("=")) return { open: { kind: "world", world: sel.slice(1) }, closeAll: false };
    if (sel.includes("/")) {
      const [world, char] = splitKey(sel);
      return { open: { kind: "char", world, char }, closeAll: false };
    }
    return "eat";
  }
  if (s.active.startsWith("=")) return { open: { kind: "world", world: s.active.slice(1) }, closeAll: true };
  if (s.chars[s.active]) {
    const [world, char] = splitKey(s.active);
    return { open: { kind: "char", world, char }, closeAll: true };
  }
  return "eat";
}

// answered takes Go's answer to adding a character, while the picker
// asks for one: saved, it's picked (opened, connected and shown) and the
// picker closes, as the TUI's; refused, the form says why (the first
// problem, as the TUI's). An answer to anything else is left be.
export function answered(store: Store, a: Answer): void {
  if (a.Op !== "add_character" || !store.state.picker?.adding) return;
  if (a.Problems?.length) {
    store.setPicker({ error: a.Problems[0].Msg });
  } else if (a.Key) {
    store.state.act?.({ Do: "pick", Key: a.Key });
    store.closePicker();
  }
}

// logToggleKey reports whether a keydown is Ctrl+L, which shows and hides
// log mode, as the TUI's. Cmd+L stays the browser's (the address bar).
export function logToggleKey(e: { key: string; ctrlKey: boolean; altKey: boolean; metaKey: boolean; shiftKey: boolean }): boolean {
  return e.ctrlKey && !e.altKey && !e.metaKey && !e.shiftKey && e.key.toLowerCase() === "l";
}
