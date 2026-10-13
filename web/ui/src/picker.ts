// The picker's rows and highlight, as the TUI's (internal/ui/picker.go):
// every configured character that isn't open, under its world, each
// world ending with a row to add a character. Go sends the worlds in
// this order (feed.worlds). A world's own row can be highlighted once its
// editor comes (step 4b), and "+ World" comes with it.

// WorldInfo is a world of the Worlds event.
export interface WorldInfo {
  ID: string;
  Characters?: { ID: string; Name: string; Aliases?: string[] }[];
}

// PickRow is one row of the picker.
export type PickRow =
  | { kind: "world"; world: string }
  | { kind: "char"; world: string; key: string; name: string }
  | { kind: "add_char"; world: string };

// matches reports whether a character fits the lowercased filter f: its
// name, an alias or its world contains it.
function matches(world: string, c: { Name: string; Aliases?: string[] }, f: string): boolean {
  return !f || [c.Name, world, ...(c.Aliases ?? [])].some((s) => s.toLowerCase().includes(f));
}

// pickerRows lists the characters the picker offers, those not in open,
// under their worlds, each world ending with "+ Character". A world is
// listed when the filter matches it or one of its characters, even with
// no characters to offer.
export function pickerRows(worlds: WorldInfo[], open: Record<string, unknown>, filter: string): PickRow[] {
  const f = filter.trim().toLowerCase();
  const rows: PickRow[] = [];
  for (const w of worlds) {
    const chars: PickRow[] = (w.Characters ?? [])
      .filter((c) => !open[`${w.ID}/${c.ID}`] && matches(w.ID, c, f))
      .map((c) => ({ kind: "char", world: w.ID, key: `${w.ID}/${c.ID}`, name: c.Name }));
    if (chars.length || w.ID.toLowerCase().includes(f)) rows.push({ kind: "world", world: w.ID }, ...chars, { kind: "add_char", world: w.ID });
  }
  return rows;
}

// selKey is what the highlight holds when r is highlighted: a
// character's key, or "+world" for adding a character to world; "" for a
// row that can't be.
export function selKey(r: PickRow): string {
  return r.kind === "char" ? r.key : r.kind === "add_char" ? "+" + r.world : "";
}

// pickable is the selection keys of the rows that can be highlighted, in
// order.
export const pickable = (rows: PickRow[]): string[] => rows.map(selKey).filter((k) => k);

// fixPick keeps the highlight on its row if it's still offered, and
// otherwise moves it to the first character, or the first row there is;
// "" when there's none.
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
