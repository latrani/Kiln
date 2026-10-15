// The theme on the page: a Theme event as a stylesheet, and the fold that
// draws a span (see internal/webapi's Span).
import * as str from "./str.gen.ts";
import { terminalDark, terminalLight } from "./terminal.gen.ts";
import type { Color, Look, Looks, Span, TagLook, ThemeEvent } from "./types.ts";

// Palette is the terminal's own colors: its foreground and background,
// and the sixteen.
export interface Palette {
  fg: string;
  bg: string;
  ansi: string[];
}

// The terminal's colors on the web: Catppuccin Macchiato (dark) and Latte
// (light), generated from internal/theme/catppuccin.toml.
export const macchiato: Palette = terminalDark;
export const latte: Palette = terminalLight;

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

// The roles a class can't reach, drawn through variables in kiln.css:
// the rules and the divider (borders), and the input's and forms' hints
// (placeholders).
const lineRoles = ["rule", "rule.input", "rule.form", "rule.status", "divider", "input.hint", "form.hint"];

// The glyphs kiln.css draws: around Kiln's own lines, and a focused
// choice's ◄ ►.
const glyphs = `--g-echo:${JSON.stringify(str.glyph.echo)};--g-sys:${JSON.stringify(str.glyph.sys)};--g-rule:${JSON.stringify(str.glyph.rule)}` +
  `;--g-prev:${JSON.stringify(str.glyph.choicePrev)};--g-next:${JSON.stringify(str.glyph.choiceNext)}`;

function sheet(l: Looks, p: Palette, scheme: "dark" | "light"): string {
  const vars = [`color-scheme:${scheme}`, `--term-fg:${p.fg}`, `--term-bg:${p.bg}`, ...p.ansi.map((c, i) => `--ansi-${i}:${c}`), glyphs];
  for (const r of lineRoles) vars.push(`--role-${r.replace(".", "-")}-fg:${color(l.Roles[r]?.FG, p) ?? p.fg}`);
  // A form's fill, for kiln.css to cover the browser's autofill with.
  vars.push(`--role-form-bg:${color(l.Roles["form"]?.BG, p) ?? p.bg}`);
  // A span's colors for this appearance (look.ts); unset, they're the
  // terminal's own.
  const a = scheme === "light" ? "l" : "d";
  let css = `:root{${vars.join(";")}}body{color:var(--term-fg);background:var(--term-bg)}.s{color:var(--${a}-fg);background:var(--${a}-bg)}`;
  for (const [role, look] of Object.entries(l.Roles)) css += `.${roleClass(role)}{${lookCSS(look, p)}}`;
  const hover = l.Roles["link.hover"];
  if (hover) css += `a.${roleClass("link")}:hover{${lookCSS(hover, p)}}`;
  return css;
}

const none: Looks = { Roles: {}, Tags: {} };

// themeCSS is t as a stylesheet: dark unless the setting, or with auto
// the system, says light. With no theme at all (a first visit, before
// Go's first) it's the terminal's colors alone, following the system.
export function themeCSS(t: ThemeEvent | undefined): string {
  if (t?.Appearance === "light") return sheet(t.Light, latte, "light");
  if (t?.Appearance === "dark") return sheet(t.Dark, macchiato, "dark");
  return sheet(t?.Dark ?? none, macchiato, "dark") + `@media (prefers-color-scheme: light){${sheet(t?.Light ?? none, latte, "light")}}`;
}

// The last theme Go sent, kept in localStorage so the page looks like
// Kiln before Go sends one: while it loads, and while another tab has it
// (that tab keeps it up to date). Best effort: without storage, those
// follow the system.
const themeKey = "kiln.theme"; // str:ok
export function rememberTheme(t: ThemeEvent): void {
  try {
    globalThis.localStorage?.setItem(themeKey, JSON.stringify(t));
  } catch {}
}
export function recallTheme(): ThemeEvent | undefined {
  try {
    const t = JSON.parse(globalThis.localStorage?.getItem(themeKey) ?? "null");
    return t?.Dark?.Roles && t?.Light?.Roles ? t : undefined;
  } catch {
    return undefined;
  }
}
// forgetTheme is /reset's: the next load follows the system again until
// Go says otherwise.
export function forgetTheme(): void {
  try {
    globalThis.localStorage?.removeItem(themeKey);
  } catch {}
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
