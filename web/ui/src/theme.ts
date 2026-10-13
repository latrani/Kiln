// The theme on the page: a Theme event as a stylesheet, and the fold that
// draws a span (see internal/webapi's Span).
import type { Color, Look, Looks, Span, TagLook, ThemeEvent } from "./types.ts";

// Palette is the terminal's own colors: its foreground and background,
// and the sixteen.
export interface Palette {
  fg: string;
  bg: string;
  ansi: string[];
}

// The terminal's colors on the web: Catppuccin Macchiato (dark) and Latte
// (light), from Catppuccin's own terminal ports. Normal and bright differ
// only in black and white.
export const macchiato: Palette = {
  fg: "#cad3f5",
  bg: "#24273a",
  ansi: ["#494d64", "#ed8796", "#a6da95", "#eed49f", "#8aadf4", "#f5bde6", "#8bd5ca", "#b8c0e0",
    "#5b6078", "#ed8796", "#a6da95", "#eed49f", "#8aadf4", "#f5bde6", "#8bd5ca", "#a5adcb"],
};
export const latte: Palette = {
  fg: "#4c4f69",
  bg: "#eff1f5",
  ansi: ["#bcc0cc", "#d20f39", "#40a02b", "#df8e1d", "#1e66f5", "#ea76cb", "#179299", "#5c5f77",
    "#acb0be", "#d20f39", "#40a02b", "#df8e1d", "#1e66f5", "#ea76cb", "#179299", "#6c6f85"],
};

// color is c as a CSS color under p; undefined for none.
export function color(c: Color | undefined, p: Palette): string | undefined {
  if (!c) return undefined;
  const m = /^ansi:(\d+)$/.exec(c);
  return m ? p.ansi[Number(m[1])] : c;
}

// lookCSS is l as CSS declarations. Reverse swaps foreground and
// background, each falling back to the terminal's own. Faint is drawn at
// 60%: the web's nearest to a terminal's dim.
export function lookCSS(l: Look, p: Palette): string {
  let fg = color(l.FG, p);
  let bg = color(l.BG, p);
  if (l.Reverse) [fg, bg] = [bg ?? p.bg, fg ?? p.fg];
  const d: string[] = [];
  if (fg) d.push(`color:${fg}`);
  if (bg) d.push(`background:${bg}`);
  if (l.Bold) d.push("font-weight:700");
  if (l.Faint) d.push("opacity:.6");
  if (l.Italic) d.push("font-style:italic");
  if (l.Underline) d.push("text-decoration:underline");
  return d.join(";");
}

// roleClass is the class that draws a theme role.
export const roleClass = (role: string): string => "r-" + role.replaceAll(".", "-").replaceAll("_", "-");

// The roles drawn as lines rather than text: kiln.css colors the borders
// and the divider with their variables.
const lineRoles = ["rule", "rule.input", "rule.form", "rule.status", "divider"];

function sheet(l: Looks, p: Palette): string {
  const vars = [`--term-fg:${p.fg}`, `--term-bg:${p.bg}`, ...p.ansi.map((c, i) => `--ansi-${i}:${c}`)];
  for (const r of lineRoles) vars.push(`--role-${r.replace(".", "-")}-fg:${color(l.Roles[r]?.FG, p) ?? p.fg}`);
  let css = `:root{${vars.join(";")}}body{color:var(--term-fg);background:var(--term-bg)}`;
  for (const [role, look] of Object.entries(l.Roles)) css += `.${roleClass(role)}{${lookCSS(look, p)}}`;
  return css;
}

// themeCSS is t as a stylesheet: dark unless the setting, or with auto
// the system, says light.
export function themeCSS(t: ThemeEvent): string {
  if (t.Appearance === "light") return sheet(t.Light, latte);
  if (t.Appearance === "dark") return sheet(t.Dark, macchiato);
  return sheet(t.Dark, macchiato) + `@media (prefers-color-scheme: light){${sheet(t.Light, latte)}}`;
}

// fold is a span's look: the server's style, then each named tag's style
// in order; a set color replaces, flags add up (theme.Style.Over).
export function fold(s: Span, tags: Record<string, TagLook>): Look {
  const out: Look = { FG: s.FG, BG: s.BG, Bold: s.Bold, Faint: s.Faint, Italic: s.Italic, Underline: s.Underline, Reverse: s.Reverse };
  for (const n of s.Tags ?? []) {
    const l = tags[n];
    if (!l) continue;
    if (l.FG) out.FG = l.FG;
    if (l.BG) out.BG = l.BG;
    for (const k of ["Bold", "Faint", "Italic", "Underline", "Reverse"] as const) out[k] = out[k] || l[k];
  }
  return out;
}
