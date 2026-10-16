import { test } from "node:test";
import assert from "node:assert/strict";
import { render } from "preact-render-to-string";
import { App } from "./App.tsx";
import { focusPanel, panelAct } from "../filter.ts";
import { titled } from "../layout.ts";
import { logAct } from "../log.ts";
import { switchAction } from "../presence.ts";
import { Store } from "../store.ts";
import type { LogView } from "../store.ts";
import { roleClass } from "../theme.ts";
import * as str from "../str.gen.ts";

const E = (Name: string, more: object = {}) => ({ Item: { Name, Text: false, Untagged: false }, Depth: 0, Parent: false, FoldedAway: false, Hidden: false, Only: false, Folded: false, FilteredUnder: false, ...more });
const entries = [E("", { Item: { Name: "", Text: false, Untagged: true } }), E("page", { Parent: true })];
const store = (logMode: boolean, view: Partial<LogView> = {}) => {
  const s = new Store();
  const sent: any[] = [];
  s.set({ phase: "running", act: (a: object) => { sent.push(a); } });
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" },
    ...(logMode ? [{ type: "Log", Key: "fm/kit", Open: true, Find: "", Selected: 0, HistDone: true }, { type: "Filter", Key: "fm/kit", Entries: entries }] : [])]);
  if (logMode) s.setLogView("fm/kit", view);
  return { s, sent };
};
const html = (s: Store) => render(<App store={s} />);
const side = (h: string) => h.slice(h.indexOf("<nav"), h.indexOf("</nav>"));

test("each sidebar list is titled: Connections, All connections in the picker, Filters in log mode", () => {
  const { s } = store(false);
  assert.match(side(html(s)), new RegExp(`^<nav[^>]*><h2 class="stitle ${roleClass("sidebar.title")}">${titled(str.viewConnectionsTitle())}</h2>`));
  s.openPicker();
  assert.match(side(html(s)), new RegExp(`<h2 class="stitle ${roleClass("picker.title")}">${titled(str.pickerTitle())}</h2>`));
  const { s: l } = store(true);
  assert.match(side(html(l)), new RegExp(`^<nav class="side filter-panel[^>]*><h2 class="stitle ${roleClass("filter.title")}">${titled(str.filterTitle())}</h2>`));
});

test("in log mode the panel is always there, on its first entry, and there's no Filter chip", () => {
  const { s } = store(true);
  const h = html(s);
  assert.match(side(h), new RegExp(`class="fname fsel ${roleClass("filter.selected")}"[^>]*>${str.filterUntagged()}<`));
  const top = h.slice(h.indexOf("<header"), h.indexOf("</header>"));
  assert.ok(!top.includes("status.filter") && (top.match(/<button/g) ?? []).length === 2, "presence and Log only");
});

test("footers follow focus: the log's while it has it (or nothing has), the panel's while it does", () => {
  const foot = (h: string) => side(h).slice(side(h).indexOf(`class="ffoot`));
  const bar = (h: string) => h.slice(h.indexOf(`class="logbar`));
  const { s } = store(true);
  assert.ok(bar(html(s)).includes(str.browseHintsNone()) && foot(html(s)).includes(str.filterHintsAway().split(str.separator())[0]));
  assert.ok(!foot(html(s)).includes("enter"), "the away footer has no shared keys");
  s.setLogView("fm/kit", { focus: "panel" });
  assert.ok(bar(html(s)).includes(str.browseHintsNoneAway()));
  for (const k of str.filterHints().split(str.separator())) assert.ok(foot(html(s)).includes(k), k);
});

test("f in the log focuses the panel; Esc (close) in the panel the log; the panel stays", () => {
  const { s } = store(true);
  logAct(s, "filter");
  assert.equal(s.state.logView["fm/kit"].focus, "panel");
  assert.equal(s.state.logView["fm/kit"].panel?.follow, true);
  panelAct(s, "close");
  assert.equal(s.state.logView["fm/kit"].focus, undefined);
  assert.ok(html(s).includes("filter-panel"));
});

test("Esc in the log never closes the panel: marking, find, then log mode", () => {
  const { s, sent } = store(true);
  focusPanel(s);
  panelAct(s, "close");
  logAct(s, "back");
  assert.deepEqual(sent.at(-1), { Do: "log_close", Key: "fm/kit" });
});

test("switching keys do nothing in log mode", () => {
  const k = { key: "ArrowDown", altKey: true, ctrlKey: false, shiftKey: false, metaKey: false };
  assert.equal(switchAction(k, true), undefined);
  assert.deepEqual(switchAction(k, false), { Do: "next_unread", N: 1 });
});
