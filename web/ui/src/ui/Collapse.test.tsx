import { test } from "node:test";
import assert from "node:assert/strict";
import { render } from "preact-render-to-string";
import { App } from "./App.tsx";
import { canToggle, toggleSidebar } from "../sidebar.ts";
import { Store } from "../store.ts";
import { roleClass } from "../theme.ts";
import * as str from "../str.gen.ts";

const page = (sidebar: object, more: object[] = []) => {
  const s = new Store();
  const sent: any[] = [];
  s.set({ phase: "running", act: (a: object) => { sent.push(a); } });
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" },
    { type: "Sidebar", Collapsed: false, Unread: 0, Priority: false, ...sidebar }, ...more] as any);
  return { s, sent };
};
const html = (s: Store) => render(<App store={s} />);
const top = (h: string) => h.slice(h.indexOf("<header"), h.indexOf("</header>"));

test("expanded: ‹ at the title's right, no ≡", () => {
  const h = html(page({}).s);
  assert.ok(h.includes(`aria-label="${str.webCollapseSidebar()}"`) && h.includes(`>${str.glyph.collapse}</button>`));
  assert.ok(!top(h).includes(str.glyph.menu));
});

test("collapsed: no sidebar, ≡ first in the top bar, named with the rows' activity summed", () => {
  const h = html(page({ Collapsed: true, Unread: 5, Priority: true }).s);
  assert.ok(!h.includes(`<nav class="side`) && h.includes(`class="kiln collapsed"`));
  const t = top(h);
  assert.match(t, new RegExp(`^<header[^>]*><button class="menu ${roleClass("status.menu")}" aria-expanded="false" aria-label="${[str.webSidebarMenu(), str.webSidebarUnread(5), str.webSidebarPriority()].join(str.separator())}">`));
  assert.ok(t.includes(`${str.glyph.menu} <span class="${roleClass("sidebar.priority")}">${str.glyph.priority}</span><span class="${roleClass("sidebar.unread")}">5</span>`));
  const plain = top(html(page({ Collapsed: true, Unread: 5 }).s));
  assert.ok(plain.includes(`${str.glyph.menu} <span class="${roleClass("sidebar.unread")}">5</span>`) && !plain.includes(str.glyph.priority));
  const none = top(html(page({ Collapsed: true }).s));
  assert.ok(none.includes(`aria-label="${str.webSidebarMenu()}">${str.glyph.menu}</button>`));
});

test("collapsed, the picker shows the sidebar with no ‹; log mode's panel with focus shows, its ≡ without the badge", () => {
  const { s } = page({ Collapsed: true, Unread: 2 });
  s.openPicker();
  const h = html(s);
  assert.ok(h.includes(`<nav class="side`) && !h.includes(`aria-label="${str.webCollapseSidebar()}"`));
  s.closePicker();
  s.apply([{ type: "Log", Key: "fm/kit", Open: true, Find: "", Selected: 0, HistDone: true }, { type: "Filter", Key: "fm/kit", Entries: [] }] as any);
  assert.ok(top(html(s)).includes(`aria-label="${str.webSidebarMenu()}">${str.glyph.menu}</button>`), "the filters' ≡: no badge");
  s.setLogView("fm/kit", { focus: "panel" });
  assert.ok(html(s).includes("filter-panel"));
});

test("toggleSidebar sends the new state; collapsing with the panel focused gives the log focus first", () => {
  const { s, sent } = page({}, [{ type: "Log", Key: "fm/kit", Open: true, Find: "", Selected: 0, HistDone: true }]);
  s.setLogView("fm/kit", { focus: "panel" });
  toggleSidebar(s);
  assert.deepEqual(sent.at(-1), { Do: "sidebar", Shut: true });
  assert.equal(s.state.logView["fm/kit"].focus, undefined);
});

test("‹ and ≡ show only where they'd do something: not while the picker or an editor is up", () => {
  const { s } = page({});
  s.openPicker();
  assert.ok(!html(s).includes(`aria-label="${str.webCollapseSidebar()}"`), "no ‹ with the picker up");
  s.closePicker();
  const c = page({ Collapsed: true }).s;
  c.set({ picker: { filter: "", sel: "", edit: { closeAll: true } as any } }); // an editor from the main view: the sidebar stays collapsed
  assert.ok(!canToggle(c.state), "an editor up: no toggling, so TopBar draws no ≡");
});
