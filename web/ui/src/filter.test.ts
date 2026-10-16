import { test } from "node:test";
import assert from "node:assert/strict";
import { filterKey, fixSel, moveSel, panelRows } from "./filter.ts";
import type { FilterEntry } from "./filter.ts";
import * as str from "./str.gen.ts";

const E = (Name: string, more: Partial<FilterEntry> & { Text?: boolean; Untagged?: boolean } = {}): FilterEntry => {
  const { Text = false, Untagged = false, ...rest } = more;
  return { Item: { Name, Text, Untagged }, Depth: Name.split("/").length - 1, Parent: false, FoldedAway: false, Hidden: false, Only: false, Folded: false, FilteredUnder: false, ...rest };
};

test("the panel lists entries less those folded away, + Text last; names as the TUI's", () => {
  const rows = panelRows([E("", { Untagged: true, Depth: 0 }), E("page", { Parent: true }), E("page/ooc"), E("page/ic", { FoldedAway: true }), E("hello world", { Text: true, Depth: 0 })]);
  assert.deepEqual(rows.map((r) => r.key), ["u", "g:page", "g:page/ooc", "t:hello world", "+"]);
  const names = rows.flatMap((r) => (r.kind === "entry" ? [r.name] : []));
  assert.deepEqual(names, [str.filterUntagged(), "page", "ooc", str.filterTextItem("hello world")]);
});

test("glyphs: ▼ open, > folded (• when something under it is filtered), × on a text row", () => {
  const rows = panelRows([E("a", { Parent: true }), E("b", { Parent: true, Folded: true, FilteredUnder: true }), E("c", { Parent: true, Folded: true }), E("t", { Text: true, Depth: 0 })]);
  const g = rows.flatMap((r) => (r.kind === "entry" ? [[r.glyph, r.after]] : []));
  assert.deepEqual(g, [[str.glyph.open, ""], [str.glyph.collapsed, str.glyph.filtered], [str.glyph.collapsed, ""], [str.glyph.remove, ""]]);
});

test("a blank row ends each top-level group", () => {
  const rows = panelRows([E("a", { Parent: true }), E("a/b"), E("c"), E("t", { Text: true, Depth: 0 })]);
  assert.deepEqual(rows.flatMap((r) => (r.kind === "entry" ? [r.endsGroup] : [])), [false, true, true, true]);
});

test("the highlight: kept if it's there, else the first entry; moves stop at the ends", () => {
  const rows = panelRows([E("a"), E("b")]);
  assert.equal(fixSel(rows, "g:b"), "g:b");
  assert.equal(fixSel(rows, "t:gone"), "g:a", "gone: the first");
  assert.equal(moveSel(rows, "g:a", 1), "g:b");
  assert.equal(moveSel(rows, "g:b", 5), "+");
  assert.equal(moveSel(rows, "g:a", -1), "g:a");
  assert.equal(moveSel(rows, "t:gone", 1), "g:b", "from the first, when it's gone");
});

test("the panel's keys, as the TUI's; never with a modifier", () => {
  const k = (key: string, mods: { ctrlKey?: boolean; altKey?: boolean; metaKey?: boolean } = {}) => filterKey({ key, ctrlKey: false, altKey: false, metaKey: false, ...mods });
  assert.deepEqual(["ArrowUp", "ArrowDown", "h", "o", "ArrowLeft", "ArrowRight", " ", "Enter", "x", "Delete", "Escape", "f"].map((x) => k(x)),
    ["up", "down", "hide", "only", "shut", "open", "toggle", "enter", "remove", "remove", "close", "close"]);
  assert.equal(k("h", { ctrlKey: true }), undefined);
  assert.equal(k("o", { metaKey: true }), undefined);
  assert.equal(k("j"), undefined, "no j/k on the web: arrows");
});
