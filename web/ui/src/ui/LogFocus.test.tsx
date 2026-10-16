import { test } from "node:test";
import assert from "node:assert/strict";
import { render } from "preact-render-to-string";
import { App } from "./App.tsx";
import { focusPanel, panelAct } from "../filter.ts";
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
  const title = (role: string, rule: string, label: string) => `<div class="stitle ${roleClass(role)}" style="--rule:var(--role-${rule.replaceAll(".", "-")}-fg)"><h2 class="stext">${label}</h2>`;
  assert.ok(side(html(s)).includes(title("sidebar.title", "rule.sidebar", str.viewConnectionsTitle())), "over a rule in the sidebar's color");
  s.openPicker();
  assert.ok(side(html(s)).includes(title("picker.title", "rule.picker", str.pickerTitle())));
  const { s: l } = store(true);
  assert.ok(side(html(l)).includes(title("filter.title", "rule.filter", str.filterTitle())));
});

test("in log mode the panel is always there, on its first entry, and there's no Filter chip", () => {
  const { s } = store(true);
  const h = html(s);
  assert.match(side(h), new RegExp(`class="fname fsel ${roleClass("filter.selected")}"[^>]*>${str.filterUntagged()}<`));
  const top = h.slice(h.indexOf("<header"), h.indexOf("</header>"));
  assert.ok(!top.includes("status.filter") && (top.match(/<button/g) ?? []).length === 2, "presence and Log only");
});

test("footers follow focus, the unfocused side's faded; the panel's has the keys for the highlighted row", () => {
  const text = (h: string) => h.replace(/<[^>]*>/g, "");
  const foot = (h: string) => side(h).slice(side(h).indexOf(`<div class="ffoot`));
  const bar = (h: string) => h.slice(h.indexOf(`class="logbar`));
  const keys = (...ks: string[]) => ks.join(str.separator());
  const { s } = store(true); // the highlight on Untagged
  assert.ok(bar(html(s)).includes(`class="${roleClass("log.bar.hints")}">${str.browseHintsNone()}<`));
  assert.ok(foot(html(s)).startsWith(`<div class="ffoot ${roleClass("filter.hints.away")}"`), "the panel's faded");
  assert.equal(text(foot(html(s))), keys(str.filterKeyHide(), str.filterKeyOnly()));
  s.setLogView("fm/kit", { focus: "panel" });
  assert.ok(bar(html(s)).includes(`class="${roleClass("log.bar.hints.away")}">${str.browseHintsNoneAway()}<`), "the log's faded");
  assert.ok(foot(html(s)).startsWith(`<div class="ffoot ${roleClass("filter.hints")}"`));
  assert.equal(text(foot(html(s))), keys(str.filterKeyHide(), str.filterKeyOnly(), str.filterKeyLog()));
  s.setLogView("fm/kit", { panel: { sel: "g:page" } }); // has tags under it
  assert.equal(text(foot(html(s))), keys(str.filterKeyHide(), str.filterKeyOnly(), str.filterKeyFold(), str.filterKeyLog()));
  s.setLogView("fm/kit", { panel: { sel: "+" } });
  assert.equal(text(foot(html(s))), keys(str.filterKeyAdd(), str.filterKeyLog()));
  s.setLogView("fm/kit", { focus: undefined });
  assert.equal(text(foot(html(s))), "", "nothing on + Text reaches it from the log");
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
