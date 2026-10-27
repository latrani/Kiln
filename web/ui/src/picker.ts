// The picker's rows and highlight, as the TUI's (internal/ui/picker.go):
// every configured world and character, open or not, those the filter
// matches, each world ending with a row to add a character. Go sends the
// worlds in this order (feed.worlds). A world's own row shows its
// overview. A final row adds a world, as the TUI's, but only for a host
// that can (worlds): on the web the worlds are the relay's allowlist,
// which it serves as the preset.

import type { Target } from "./editor.ts";
import { branch } from "./sidebar.ts";
import type { Answer, State, Store } from "./store.ts";

// WorldInfo is a world of the Worlds event.
export interface WorldInfo {
  ID: string;
  Characters?: { ID: string; Name: string; Aliases?: string[] }[];
}

// PickRow is one row of the picker. marks are the letters of its name
// the filter matched, and score how well; match, whether a world's own
// name did.
export type PickRow =
  | { kind: "world"; world: string; match: boolean; marks: number[]; score: number }
  | { kind: "char"; world: string; key: string; name: string; open: boolean; marks: number[]; score: number; last?: boolean }
  | { kind: "add_char"; world: string; last?: boolean }
  | { kind: "add_world" };

// The filter's scoring, as the TUI's (internal/ui/picker.go), after
// fzf's: each letter matched scores, more on a word's first letter (the
// filter's first letter most), more in a run, less after a gap.
const scoreMatch = 16, bonusWordStart = 8, bonusCamel = 7, bonusConsecutive = 4, scoreGapStart = -3, scoreGapMore = -1;

const wordy = /[\p{L}\p{N}]/u;
const lower = (c: string) => c !== c.toUpperCase() && c === c.toLowerCase();
const upper = (c: string) => c !== c.toLowerCase() && c === c.toUpperCase();

// startBonus is the bonus for a match at rs[j]: on a word's first
// letter (the start, after anything but a letter or digit, or an
// uppercase letter after a lowercase one), or none.
function startBonus(rs: string[], j: number): number {
  if (j === 0 || !wordy.test(rs[j - 1])) return bonusWordStart;
  return lower(rs[j - 1]) && upper(rs[j]) ? bonusCamel : 0;
}

// match reports whether f's letters are in s in order, ignoring case,
// as the TUI's: which of s's letters they are, placed where they score
// best, and that score (ties to the placement earliest in s); undefined
// when they aren't all there.
export function match(s: string, f: string): { at: number[]; score: number } | undefined {
  const rs = [...s], want = [...f];
  const m = want.length, n = rs.length;
  if (!m) return { at: [], score: 0 };
  const none = -Infinity;
  // best[i][j] is the best score with the filter's letter i at rs[j],
  // from[i][j] where letter i-1 was then.
  const best = want.map(() => new Array<number>(n).fill(none)), from = want.map(() => new Array<number>(n).fill(0));
  for (let i = 0; i < m; i++) {
    for (let j = 0; j < n; j++) {
      if (rs[j].toLowerCase() !== want[i].toLowerCase()) continue;
      if (i === 0) {
        best[0][j] = scoreMatch + 2 * startBonus(rs, j);
        continue;
      }
      for (let k = i - 1; k < j; k++) {
        if (best[i - 1][k] === none) continue;
        const sc = best[i - 1][k] + scoreMatch + startBonus(rs, j) + (k === j - 1 ? bonusConsecutive : scoreGapStart + scoreGapMore * (j - k - 2));
        if (sc > best[i][j]) [best[i][j], from[i][j]] = [sc, k];
      }
    }
  }
  let end = -1;
  for (let j = 0; j < n; j++) if (best[m - 1][j] !== none && (end < 0 || best[m - 1][j] > best[m - 1][end])) end = j;
  if (end < 0) return undefined;
  const at = new Array<number>(m);
  for (let i = m - 1, j = end; i >= 0; i--) {
    at[i] = j;
    j = from[i][j];
  }
  return { at, score: best[m - 1][end] };
}

// pickerRows lists every world and character, open or not (open says
// which are), those the filter matches, as the TUI's: a world when its
// name does or one of its characters' does, a character when
// "world/name" does. Each world ends with "+ Character". A world listed
// only for its characters marks the letters of its name the best of
// them matched. With addWorld the list ends with "+ World", whatever the
// filter, as the TUI's.
export function pickerRows(worlds: WorldInfo[], open: Record<string, unknown>, filter: string, addWorld: boolean): PickRow[] {
  const f = filter.trim();
  const rows: PickRow[] = [];
  for (const w of worlds) {
    const wm = match(w.ID, f);
    const head: PickRow & { kind: "world" } = { kind: "world", world: w.ID, match: !!wm, marks: wm?.at ?? [], score: wm?.score ?? 0 };
    const wn = [...w.ID].length + 1; // past the "/"
    const chars: PickRow[] = [];
    let bestChar = -Infinity;
    for (const c of w.Characters ?? []) {
      const cm = match(`${w.ID}/${c.Name}`, f);
      if (!cm) continue;
      if (!wm && cm.score > bestChar) [head.marks, bestChar] = [cm.at.filter((i) => i < wn), cm.score];
      const key = `${w.ID}/${c.ID}`;
      chars.push({ kind: "char", world: w.ID, key, name: c.Name, open: !!open[key], marks: cm.at.filter((i) => i >= wn).map((i) => i - wn), score: cm.score });
    }
    if (chars.length || wm) rows.push(head, ...chars, { kind: "add_char", world: w.ID });
  }
  if (addWorld) rows.push({ kind: "add_world" });
  return branch(rows);
}

// firstMatch is the selection key of the row the filter matched best, a
// world or a character, the first of those that tie, as the TUI's;
// undefined when none did.
export function firstMatch(rows: PickRow[]): string | undefined {
  let top: PickRow | undefined;
  for (const r of rows) {
    if (((r.kind === "world" && r.match) || r.kind === "char") && (!top || r.score > (top as { score: number }).score)) top = r;
  }
  return top && selKey(top);
}

// selKey is what the highlight holds when r is highlighted: a
// character's key, "=world" for a world's own row (its overview), "+world"
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

// choose acts on the picker's row sel, as Enter or a click does, as the
// TUI's: a character shows (pick: one not open opens and connects), a
// world its overview, closing the picker; a world with nothing open has
// none, so its editor opens instead. "+ Character" asks for the new
// character's name, and "+ World" opens a new world's editor. An editor
// open over the picker is left first, its edits kept. Nothing for "".
export function choose(store: Store, sel: string): void {
  store.leaveEditor();
  const s = store.state;
  if (sel.startsWith("=") && s.order.some((k) => s.chars[k]?.World === sel.slice(1))) {
    s.act?.({ Do: "switch", Key: sel });
    store.closePicker();
  } else if (sel.startsWith("=")) store.openEditor({ kind: "world", world: sel.slice(1) }, false);
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
