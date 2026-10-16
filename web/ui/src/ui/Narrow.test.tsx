import { test } from "node:test";
import assert from "node:assert/strict";
import { render } from "preact-render-to-string";
import { App } from "./App.tsx";
import { becomeNarrow, swallowsClick, toggleSidebar } from "../sidebar.ts";
import { toggleLog } from "../log.ts";
import { Store } from "../store.ts";
import * as str from "../str.gen.ts";

const page = (more: object[] = []) => {
  const s = new Store();
  const sent: any[] = [];
  s.set({ phase: "running", act: (a: object) => { sent.push(a); }, narrow: true });
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Char", Key: "fm/rook", World: "fm", ID: "rook", Name: "Rook", State: 0 },
    { type: "Order", Keys: ["fm/kit", "fm/rook"] }, { type: "Active", Key: "fm/kit" },
    { type: "Sidebar", Collapsed: false, Unread: 0, Priority: false }, ...more] as any);
  return { s, sent };
};
const html = (s: Store) => render(<App store={s} />);

test("narrow, closed: no sidebar, ≡ in the top bar, though not collapsed", () => {
  const h = html(page().s);
  assert.ok(!h.includes(`<nav class="side`) && h.includes(`class="kiln narrow"`) && h.includes(`class="menu `));
});

test("narrow, open: the sidebar over the top bar and scrollback, ‹ to close; no top bar", () => {
  const { s } = page();
  toggleSidebar(s);
  assert.equal(s.state.narrowOpen, true);
  const h = html(s);
  assert.ok(h.includes(`class="kiln narrow open"`) && h.includes(`<nav class="side`) && h.includes(`aria-label="${str.webCollapseSidebar()}"`));
  assert.ok(!h.includes("<header"));
});

test("narrow: picking closes it; so does closing the picker; nothing's sent to Go", () => {
  const { s, sent } = page();
  toggleSidebar(s);
  assert.ok(!sent.some((a) => a.Do === "sidebar"), "narrow isn't saved");
  s.pickFromSidebar("fm/rook");
  assert.equal(s.state.narrowOpen, false);
  assert.deepEqual(sent.at(-1), { Do: "switch", Key: "fm/rook" });
  toggleSidebar(s);
  s.openPicker();
  s.closePicker();
  assert.equal(s.state.narrowOpen, false);
});

test("narrow log mode: toggling is f, the panel open while it has focus", () => {
  const { s } = page([{ type: "Log", Key: "fm/kit", Open: true, Find: "", Selected: 0, HistDone: true }, { type: "Filter", Key: "fm/kit", Entries: [] }]);
  assert.ok(!html(s).includes("filter-panel"));
  toggleSidebar(s);
  assert.equal(s.state.logView["fm/kit"].focus, "panel");
  assert.ok(html(s).includes("filter-panel"));
  toggleSidebar(s);
  assert.equal(s.state.logView["fm/kit"].focus, undefined);
});

test("a double-click's second click, outside the sidebar, does nothing once the first picked from it", () => {
  const { s } = page();
  toggleSidebar(s);
  s.pickFromSidebar("fm/rook");
  const at = s.state.narrowPicked!;
  assert.equal(swallowsClick(s.state, at + 100, false), true);
  assert.equal(swallowsClick(s.state, at + 100, true), false, "in the sidebar: a click as ever");
  assert.equal(swallowsClick(s.state, at + 1000, false), false, "later: a click as ever");
});

test("opening log mode starts the narrow sidebar closed; becoming narrow closes log mode's panel", () => {
  const { s } = page([{ type: "Log", Key: "fm/kit", Open: false, Find: "", Selected: 0, HistDone: true }]);
  toggleSidebar(s);
  toggleLog(s);
  assert.equal(s.state.narrowOpen, false);
  s.apply([{ type: "Log", Key: "fm/kit", Open: true, Find: "", Selected: 0, HistDone: true }] as any); // Go's answer
  s.set({ narrow: false });
  s.setLogView("fm/kit", { focus: "panel" });
  becomeNarrow(s, true);
  assert.equal(s.state.logView["fm/kit"].focus, undefined);
});
