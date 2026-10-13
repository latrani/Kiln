import { test } from "node:test";
import assert from "node:assert/strict";
import fixture from "./testdata/fold.json" with { type: "json" };
import { color, fold, latte, lookCSS, macchiato, roleClass, themeCSS } from "./theme.ts";
import type { Span, TagLook, ThemeEvent } from "./types.ts";
import * as str from "./str.gen.ts";

interface FoldCase { Name: string; Spans: Span[]; Tags: Record<string, TagLook>; Want: Record<string, unknown>[] }

test("the fold matches Go's, case by case", () => {
  for (const c of fixture as FoldCase[]) {
    c.Spans.forEach((s, i) => {
      const got = fold(s, c.Tags) as Record<string, unknown>;
      for (const k of ["FG", "BG", "Bold", "Faint", "Italic", "Underline", "Reverse"]) {
        assert.equal(got[k] || undefined, c.Want[i][k] || undefined, `${c.Name}: span ${i} ${k}`);
      }
    });
  }
});

test("colors resolve against the palette", () => {
  assert.equal(color("ansi:1", macchiato), macchiato.ansi[1]);
  assert.equal(color("#123456", macchiato), "#123456");
  assert.equal(color("", macchiato), undefined);
  assert.equal(macchiato.ansi.length, 16);
  assert.equal(latte.ansi.length, 16);
});

test("reverse swaps, defaulting to the terminal's colors", () => {
  const css = lookCSS({ FG: "ansi:2", Reverse: true }, macchiato);
  assert.ok(css.includes(`color:${macchiato.bg}`), css);
  assert.ok(css.includes(`background:${macchiato.ansi[2]}`), css);
});

const ev = (appearance: ThemeEvent["Appearance"]): ThemeEvent => ({
  type: "Theme",
  Appearance: appearance,
  Dark: { Roles: { "sidebar.active": { FG: "#111111", Bold: true }, "rule.status": { FG: "#222222" } }, Tags: {} },
  Light: { Roles: { "sidebar.active": { FG: "#eeeeee" }, "rule.status": { FG: "#dddddd" } }, Tags: {} },
});

test("auto follows the system; a forced appearance doesn't", () => {
  const auto = themeCSS(ev("auto"));
  assert.ok(auto.includes("@media (prefers-color-scheme: light)"));
  assert.ok(auto.includes(`.${roleClass("sidebar.active")}{`));
  assert.ok(auto.includes("#111111") && auto.includes("#eeeeee"));
  const light = themeCSS(ev("light"));
  assert.ok(!light.includes("@media") && light.includes("#eeeeee") && !light.includes("#111111"));
  const dark = themeCSS(ev("dark"));
  assert.ok(!dark.includes("@media") && dark.includes("#111111") && !dark.includes("#eeeeee"));
});

test("the sixteen, the default colors and the rules are variables", () => {
  const css = themeCSS(ev("dark"));
  assert.ok(css.includes(`--ansi-1:${macchiato.ansi[1]}`));
  assert.ok(css.includes(`--term-fg:${macchiato.fg}`) && css.includes(`--term-bg:${macchiato.bg}`));
  assert.ok(css.includes("--role-rule-status-fg:#222222"));
  assert.ok(css.includes(`--role-divider-fg:${macchiato.fg}`), "a rule with no color falls back to the terminal's");
});

test("before Go's theme, the terminal's colors still draw the page", () => {
  const css = themeCSS(undefined);
  assert.ok(css.includes(`--term-bg:${macchiato.bg}`) && css.includes(`--term-bg:${latte.bg}`));
  assert.ok(css.includes(`--role-rule-fg:${macchiato.fg}`));
  assert.ok(css.includes("@media (prefers-color-scheme: light)"));
});

test("the browser's own parts follow the appearance", () => {
  assert.ok(themeCSS(ev("dark")).includes("color-scheme:dark"));
  assert.ok(themeCSS(ev("light")).includes("color-scheme:light"));
  const auto = themeCSS(ev("auto"));
  assert.ok(auto.includes("color-scheme:dark") && auto.includes("color-scheme:light"));
});

test("spans take the appearance's pair of colors", () => {
  assert.ok(themeCSS(ev("dark")).includes(".s{color:var(--d-fg);background:var(--d-bg)}"));
  assert.ok(themeCSS(ev("light")).includes(".s{color:var(--l-fg);background:var(--l-bg)}"));
  const auto = themeCSS(ev("auto"));
  const at = auto.indexOf("@media");
  assert.ok(auto.indexOf("--d-fg)") < at && auto.indexOf("--l-fg)") > at, auto);
});

test("the line glyphs are variables, in every sheet", () => {
  for (const a of ["dark", "light", "auto"] as const) {
    const css = themeCSS(ev(a));
    assert.ok(css.includes(`--g-echo:${JSON.stringify(str.glyph.echo)}`), a);
    assert.ok(css.includes(`--g-sys:${JSON.stringify(str.glyph.sys)}`) && css.includes(`--g-rule:${JSON.stringify(str.glyph.rule)}`), a);
  }
});

test("a link under the pointer takes link.hover", () => {
  const t = ev("dark");
  t.Dark.Roles["link.hover"] = { FG: "#abcdef", Underline: true };
  assert.ok(themeCSS(t).includes(`a.${roleClass("link")}:hover{${lookCSS({ FG: "#abcdef", Underline: true }, macchiato)}}`));
});
