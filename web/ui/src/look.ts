// How a span looks on the page, in both appearances at once: the theme's
// .s rule picks the pair for the appearance showing, so a line never
// needs drawing again when the system flips between light and dark.
import { fold } from "./theme.ts";
import type { Color, Look, Span, TagLook, ThemeEvent } from "./types.ts";

// ref is c as CSS that follows the appearance: the terminal's sixteen
// through their variables, hex as is.
const ref = (c: Color | undefined): string | undefined => {
  if (!c) return undefined;
  const m = /^ansi:(\d+)$/.exec(c);
  return m ? `var(--ansi-${m[1]})` : c;
};

// pair is a folded look's foreground and background, reverse applied,
// each then falling back to the terminal's own.
function pair(l: Look): [string | undefined, string | undefined] {
  const fg = ref(l.FG), bg = ref(l.BG);
  return l.Reverse ? [bg ?? "var(--term-bg)", fg ?? "var(--term-fg)"] : [fg, bg];
}

// spanStyle is s's inline style: its look folded under the dark and the
// light tag tables, as the custom properties .s picks between, and the
// flags. Flags come from the dark fold: a tag's flags are the same in
// both appearances, since only its colors come from a palette.
export function spanStyle(s: Span, dark: Record<string, TagLook>, light: Record<string, TagLook>): string {
  const d = fold(s, dark);
  const [dfg, dbg] = pair(d), [lfg, lbg] = pair(fold(s, light));
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
export function tagsFor(t: ThemeEvent | undefined, key: string): { dark: Record<string, TagLook>; light: Record<string, TagLook> } {
  return {
    dark: t?.Dark.Chars?.[key] ?? t?.Dark.Tags ?? {},
    light: t?.Light.Chars?.[key] ?? t?.Light.Tags ?? {},
  };
}
