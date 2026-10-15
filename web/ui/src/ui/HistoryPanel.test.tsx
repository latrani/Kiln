import { test } from "node:test";
import assert from "node:assert/strict";
import { render } from "preact-render-to-string";
import { App } from "./App.tsx";
import { firstLine } from "./HistoryPanel.tsx";
import { Store } from "../store.ts";
import { roleClass } from "../theme.ts";
import * as str from "../str.gen.ts";

const rows = [{ Text: "look", Clock: "Wed Sep 23 22:03" }, { Text: "say hi\nand more", Clock: "21:15" }];
const page = (more: object = {}) => {
  const s = new Store();
  s.set({ phase: "running", act: () => {} });
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" },
    { type: "History", Key: "fm/kit", Open: true, Query: "", Rows: rows, Sel: 1, ...more }]);
  return render(<App store={s} />);
};
const bottom = (html: string) => html.slice(html.indexOf(`<div class="bottom`), html.indexOf(`<div class="status`));

test("the panel sits over the input, the bottom pane at its most: a line a row, the selected one bold with › and its time reversed", () => {
  const b = bottom(page());
  assert.match(b, /^<div class="bottom [^"]* histories">/);
  assert.ok(b.indexOf(`class="history"`) < b.indexOf("<textarea"), "the panel, then the input");
  assert.ok(b.includes(`<div role="option" aria-selected="false" class="hrow ${roleClass("input.history")}"><span class="htext">look</span><span class="${roleClass("input.history.time")}">Wed Sep 23 22:03</span></div>`));
  assert.ok(b.includes(`<div role="option" aria-selected="true" class="hrow ${roleClass("input.history.selected")}" data-mark="${str.glyph.echo}"><span class="htext">${firstLine("say hi\nand more")}</span><span class="${roleClass("input.history.cursor")}">21:15</span></div>`));
});

test("a sent line shows its first line, … when there's more", () => {
  assert.equal(firstLine("look"), "look");
  assert.equal(firstLine("one\ntwo"), "one…");
});

test("nothing to list: the panel says why", () => {
  assert.ok(bottom(page({ Rows: [], Sel: -1 })).includes(str.viewHistoryNone()));
  assert.ok(bottom(page({ Rows: null, Sel: -1, Query: "zz" })).includes(str.viewHistoryNoMatch()));
});

test("the History chip is in the status line, lit while the panel's up; the panel is only for the input showing", () => {
  const lit = page();
  assert.ok(lit.includes(`class="chip ${roleClass("status.history.on")}" aria-pressed="true">${str.viewHistoryButton()}</button>`));
  const other = page({ Key: "" }); // the idle input's panel: not this one's
  assert.ok(!bottom(other).includes(`class="history"`));
  assert.ok(other.includes(`class="chip ${roleClass("status.history")}" aria-pressed="false">`));
});
