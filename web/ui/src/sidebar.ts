// The sidebar's rows, as the TUI lists them (internal/ui/sidebar.go's
// sidebarRows), less "+ Connection", which comes with the picker.

export type Row = { kind: "world"; world: string } | { kind: "char"; key: string; world: string };

// sidebarRows lists the open characters under their worlds, in order. A
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
  return rows;
}
