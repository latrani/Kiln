import { test } from "node:test";
import assert from "node:assert/strict";
import * as str from "./str.gen.ts";

test("plurals pick their form", () => {
  // browse.lines_in_range has one and other forms: 1 and 2 must differ,
  // and only "other" carries the count.
  assert.notEqual(str.browseLinesInRange(1), str.browseLinesInRange(2).replace("2", "1"));
  assert.match(str.browseLinesInRange(7), /7/);
});

test("placeholders fill in", () => {
  assert.match(str.viewConnectedSince("21:14"), /21:14/);
});

test("errors read as their message", () => {
  assert.match(str.statusThemeNotLoaded(new Error("no such theme")), /no such theme/);
  assert.doesNotMatch(str.statusThemeNotLoaded(new Error("no such theme")), /Error:/);
});

test("glyphs are one character each", () => {
  for (const g of Object.values(str.glyph)) assert.equal([...g].length, 1);
});
