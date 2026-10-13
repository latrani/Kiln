import { test } from "node:test";
import assert from "node:assert/strict";
import { spanStyle, tagsFor } from "./look.ts";
import type { ThemeEvent } from "./types.ts";

test("server colors are the terminal's, either appearance", () => {
  const css = spanStyle({ Text: "x", FG: "ansi:1", Bold: true }, {}, {});
  assert.ok(css.includes("--d-fg:var(--ansi-1)") && css.includes("--l-fg:var(--ansi-1)"), css);
  assert.ok(css.includes("font-weight:700"), css);
});

test("a tag's color can differ by appearance", () => {
  const css = spanStyle({ Text: "x", Tags: ["page"] }, { page: { FG: "#111111" } }, { page: { FG: "#eeeeee" } });
  assert.ok(css.includes("--d-fg:#111111") && css.includes("--l-fg:#eeeeee"), css);
});

test("reverse swaps, defaulting to the terminal's colors", () => {
  const css = spanStyle({ Text: "x", FG: "ansi:2", Reverse: true }, {}, {});
  assert.ok(css.includes("--d-fg:var(--term-bg)") && css.includes("--d-bg:var(--ansi-2)"), css);
});

test("a plain span has no style", () => {
  assert.equal(spanStyle({ Text: "x" }, {}, {}), "");
});

test("a character's own tags win over the theme's", () => {
  const t: ThemeEvent = { type: "Theme", Appearance: "auto",
    Dark: { Roles: {}, Tags: { page: { FG: "#1" } }, Chars: { "fm/kit": { page: { FG: "#2" } } } },
    Light: { Roles: {}, Tags: { page: { FG: "#3" } }, Chars: { "fm/kit": { page: { FG: "#4" } } } } };
  assert.equal(tagsFor(t, "fm/kit").dark.page.FG, "#2");
  assert.equal(tagsFor(t, "fm/kit").light.page.FG, "#4");
  assert.equal(tagsFor(t, "fm/rook").dark.page.FG, "#1");
  assert.deepEqual(tagsFor(undefined, "x"), { dark: {}, light: {} });
});

test("a character's tag tables are the same objects until the theme changes", () => {
  const t: ThemeEvent = { type: "Theme", Appearance: "auto", Dark: { Roles: {}, Tags: {} }, Light: { Roles: {}, Tags: {} } };
  assert.equal(tagsFor(t, "fm/kit"), tagsFor(t, "fm/kit"));
  assert.notEqual(tagsFor({ ...t }, "fm/kit"), tagsFor(t, "fm/kit"));
});
