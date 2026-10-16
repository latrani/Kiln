import { test } from "node:test";
import assert from "node:assert/strict";
import { render } from "preact-render-to-string";
import { App } from "./App.tsx";
import { Store } from "../store.ts";
import { roleClass } from "../theme.ts";
import * as str from "../str.gen.ts";

const E = (Name: string, more: object = {}) => ({ Item: { Name, Text: false, Untagged: false }, Depth: Name.split("/").length - 1, Parent: false, FoldedAway: false, Hidden: false, Only: false, Folded: false, FilteredUnder: false, ...more });
const page = (entries: object[], sel = "u") => {
  const s = new Store();
  s.set({ phase: "running", act: () => {} });
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" },
    { type: "Log", Key: "fm/kit", Open: true, Find: "", Selected: 0, HistDone: true }, { type: "Filter", Key: "fm/kit", Entries: entries }]);
  s.setLogView("fm/kit", { panel: { sel } });
  const html = render(<App store={s} />);
  return html.slice(html.indexOf("<nav"), html.indexOf("</nav>"));
};
const entries = [E("", { Item: { Name: "", Text: false, Untagged: true } }), E("page", { Parent: true, Hidden: true }), E("page/ooc", { Only: true }), E("x", { Item: { Name: "x", Text: true, Untagged: false }, Depth: 0 })];

test("the panel takes the sidebar's place, named Filter, in the filter role", () => {
  const nav = page(entries);
  assert.match(nav, new RegExp(`<nav class="side filter-panel ${roleClass("filter")}" aria-label="${str.webFilter()}"`));
});

test("each entry: a list item holding a group named by its name, Hide and Only as toggle buttons; indented by depth", () => {
  const nav = page(entries, "g:page");
  assert.match(nav, /<li class="fentry"><div role="group" aria-labelledby="flt-1">/);
  assert.match(nav, new RegExp(`<button id="flt-1" class="fname fsel ${roleClass("filter.selected")}" tabindex="0">page</button>`), "the name doesn't fold, so it says nothing of folding");
  assert.match(nav, new RegExp(`<button class="${roleClass("filter.button.on")}" tabindex="0" aria-pressed="true">${str.filterHide()}</button>`), "page is hidden: Hide lit, pressed");
  assert.match(nav, new RegExp(`<button id="flt-2" class="fname ${roleClass("filter.item")}" tabindex="-1">ooc</button>`), "not highlighted: out of the tab order");
  assert.ok(nav.includes(`<div class="frow">   <button id="flt-2"`), "page/ooc: 1 + 2 spaces in");
  assert.match(nav, new RegExp(`tabindex="-1" aria-pressed="true">${str.filterOnly()}</button>`), "ooc is Only");
});

test("glyphs and + Text: ▼ on an open parent, a fold button named by it; × on a text row; + Text last, in filter.add", () => {
  const nav = page(entries);
  assert.ok(nav.includes(`<button class="fglyph ${roleClass("filter.item")}" tabindex="-1" aria-expanded="true" aria-labelledby="flt-1">${str.glyph.open} </button>`));
  assert.ok(nav.includes(`<span aria-hidden="true" class="fglyph ${roleClass("filter.item")}">${str.glyph.remove} </span><button id="flt-3"`));
  assert.ok(nav.includes(str.filterTextItem("x").replaceAll(`"`, "&quot;")), "the text row's name, its quotes escaped in HTML");
  assert.match(nav, new RegExp(`<button class="fname ${roleClass("filter.add")}" tabindex="-1">${str.filterAddText().replace("+", "\\+")}</button></li></ul><div class="ffoot`), "last in the list, the footer after");
});

test("a highlight Go no longer lists falls back to the first entry", () => {
  const nav = page(entries, "t:gone");
  assert.match(nav, /<button id="flt-0" class="fname fsel/);
});

test("the highlight lights the glyph and • with the name, as the TUI's", () => {
  const nav = page([E("page", { Parent: true, Folded: true, FilteredUnder: true })], "g:page");
  assert.ok(nav.includes(`<button class="fglyph ${roleClass("filter.selected")}" tabindex="0" aria-expanded="false" aria-labelledby="flt-0">${str.glyph.collapsed} </button><button id="flt-0"`), "folded: ►, not expanded, in the tab order with its entry");
  assert.ok(nav.includes(`</button><span aria-hidden="true" class="${roleClass("filter.selected")}"> ${str.glyph.filtered}</span></div>`));
});
