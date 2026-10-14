import { test } from "node:test";
import assert from "node:assert/strict";
import { render } from "preact-render-to-string";
import { App } from "./App.tsx";
import { filterPromptEnter, logAct } from "../log.ts";
import { Store } from "../store.ts";
import type { Event } from "../store.ts";
import { roleClass } from "../theme.ts";
import * as str from "../str.gen.ts";

const L = (Seq: number, Day: string, Plain: string) =>
  ({ Seq, Kind: "server", Day, DayLabel: "Mon " + Day, Clock: "12:0" + Seq, Out: false, Plain, Spans: [{ Text: Plain }], Priority: false, Quiet: false });
const page = (log: object, more: Event[] = []) => {
  const s = new Store();
  s.set({ phase: "running", act: () => {} });
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Order", Keys: ["fm/kit"] }, { type: "Active", Key: "fm/kit" },
    { type: "LogLines", Key: "fm/kit", Epoch: 1, Op: "reset", Lines: [L(1, "2026-10-12", "an apple"), L(2, "2026-10-13", "a pear"), L(3, "2026-10-13", "Apple pie")] },
    { type: "Log", Key: "fm/kit", Open: true, Find: "", Status: "", StatusErr: false, Searching: false, Loading: false, HistDone: true, Selected: 0, ExportFormat: "", ...log }, ...more]);
  return render(<App store={s} />);
};
const main = (html: string) => html.slice(html.indexOf("<main"), html.indexOf("</main>"));

test("log mode has the main area: a focusable log, no live region", () => {
  const m = main(page({}));
  assert.match(m, /<div class="log"[^>]*tabindex="0"/);
  assert.match(m, new RegExp(`<div class="log"[^>]*role="region"[^>]*aria-label="${str.webLog()}"`), "named where a screen reader reads the name: a region, not a live one");
  assert.ok(!m.includes("aria-live"), "history: nothing read aloud (spec)");
  assert.ok(!m.includes('role="log"'));
});

test("rows: a rule per day, then each line's time, gutter and text", () => {
  const m = main(page({ Start: 2, End: 3, Excluded: [3] }));
  assert.match(m, new RegExp(`class="${roleClass("log.day")}"[^>]*>${str.glyph.rule}${str.glyph.rule} Mon 2026-10-12 ${str.glyph.rule}${str.glyph.rule}<`));
  assert.match(m, new RegExp(`data-seq="1"[^>]*><span class="${roleClass("log.time")}">12:01</span> <span class="gutter"> </span> `));
  assert.match(m, new RegExp(`data-seq="2"[^>]*>.*?<span class="gutter ${roleClass("log.selected")}">${str.glyph.selected}</span>`));
  assert.match(m, new RegExp(`data-seq="3"[^>]*>.*?<span class="gutter ${roleClass("log.excluded")}">${str.glyph.excluded}</span>`));
});

test("find: matches marked in log.find, the line landed on has its time in log.cursor", () => {
  const m = main(page({ Find: "apple", Matches: [1, 3] }, [{ type: "LogAt", Key: "fm/kit", Seq: 3 }]));
  assert.match(m, new RegExp(`<mark class="${roleClass("log.find")}">Apple</mark> pie`));
  assert.match(m, new RegExp(`an <mark class="${roleClass("log.find")}">apple</mark>`));
  assert.match(m, new RegExp(`data-seq="3"[^>]*><span class="${roleClass("log.cursor")}">`));
});

test("loading older history, and no logs yet, at the top", () => {
  assert.ok(main(page({ Loading: true, HistDone: false })).includes(str.browseLoading()));
  const s = new Store();
  s.set({ phase: "running" });
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" },
    { type: "Log", Key: "fm/kit", Open: true, HistDone: true, Find: "", Selected: 0 }]);
  assert.ok(main(render(<App store={s} />)).includes(str.browseNoLogs()));
});

test("hidden by Ctrl+L: the scrollback shows again", () => {
  const s = new Store();
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" }, { type: "Log", Key: "fm/kit", Open: true }]);
  s.setLogView("fm/kit", { hidden: true });
  assert.match(render(<App store={s} />), /role="log"/);
});

const pageWith = (log: object, view?: object) => {
  const s = new Store();
  s.set({ phase: "running", act: () => {} });
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" },
    { type: "LogLines", Key: "fm/kit", Epoch: 1, Op: "reset", Lines: [L(1, "2026-10-12", "an apple")] },
    { type: "Log", Key: "fm/kit", Open: true, Find: "", Status: "", StatusErr: false, Searching: false, Loading: false, HistDone: true, Selected: 0, ExportFormat: "", ...log }]);
  if (view) s.setLogView("fm/kit", view);
  return render(<App store={s} />);
};
const bottom = (html: string) => html.slice(html.indexOf('class="bottom'), html.indexOf('class="status'));

test("the bottom pane: log mode's hints, in the bar's roles", () => {
  const b = bottom(pageWith({}));
  assert.ok(b.includes(roleClass("log.bar")) && b.includes(roleClass("log.bar.hints")));
  assert.ok(b.includes(str.webLogHints()));
  assert.ok(!b.includes("<textarea"), "no chat input in log mode");
});

test("a prompt: find's label and field; the format prompt names the default", () => {
  assert.match(bottom(pageWith({}, { prompt: { kind: "find", value: "x" } })), new RegExp(`<label[^>]*>${str.browseFindPrompt()}</label><input[^>]*value="x"`));
  assert.ok(bottom(pageWith({ ExportFormat: "html" }, { prompt: { kind: "format" } })).includes(str.browseFormatPromptDefault("html")));
  assert.ok(bottom(pageWith({}, { prompt: { kind: "format" } })).includes(str.browseFormatPrompt()));
  const fmt = bottom(pageWith({}, { prompt: { kind: "format" } }));
  assert.ok(fmt.includes('role="group" aria-labelledby="kiln-format"') && fmt.includes(`<span id="kiln-format">${str.browseFormatPrompt()}</span>`), "named for a screen reader as it takes focus");
});

test("top bar: the find status and the Log chip, lit; status line: the log's status, else how many selected", () => {
  const html = pageWith({ Find: "apple", Matches: [1], Selected: 3 });
  const top = html.slice(html.indexOf("<header"), html.indexOf("</header>"));
  assert.ok(top.includes(str.browseFindStatus("apple", 0, 1)));
  assert.match(top, new RegExp(`<button class="${roleClass("status.log.on")}"[^>]*>${str.viewLogButton()}</button>`));
  assert.ok(html.slice(html.indexOf('class="status')).includes(str.viewSelected(3)));
  // Log mode's messages come as the status (Go moves them there, as the
  // TUI's takeLogStatus); the log's own field isn't drawn.
  const stale = pageWith({ Status: "stale", Selected: 2 });
  assert.ok(!stale.includes("stale") && stale.slice(stale.indexOf('class="status')).includes(str.viewSelected(2)));
});

test("not in log mode, the Log chip is unlit", () => {
  const s = new Store();
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" }]);
  assert.match(render(<App store={s} />), new RegExp(`<button class="${roleClass("status.log")}"[^>]*>${str.viewLogButton()}</button>`));
});

test("Esc stops a search reading older days, not log mode; otherwise it closes log mode", () => {
  const sent: any[] = [];
  const s = new Store();
  s.set({ phase: "running", act: (o: object) => sent.push(o) });
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" },
    { type: "Log", Key: "fm/kit", Open: true, Searching: true, Find: "x" }]);
  logAct(s, "back");
  assert.deepEqual(sent.at(-1), { Do: "stop_search", Key: "fm/kit" });
  s.apply([{ type: "Log", Key: "fm/kit", Open: true, Searching: false, Find: "x" }]);
  logAct(s, "back");
  assert.deepEqual(sent.at(-1), { Do: "log_close", Key: "fm/kit" });
});

test("with the panel open: the panel's hints; the filter prompt's label and field", () => {
  assert.ok(bottom(pageWith({}, { panel: { sel: "+" } })).includes(str.filterHints()));
  assert.match(bottom(pageWith({}, { panel: { sel: "+" }, prompt: { kind: "filter", value: "y" } })), new RegExp(`<label[^>]*>${str.filterPrompt()}</label><input[^>]*value="y"`));
});

test("the Filter chip, between presence and Log in log mode, lit while the panel's open", () => {
  const top = (html: string) => html.slice(html.indexOf("<header"), html.indexOf("</header>"));
  assert.match(top(pageWith({})), new RegExp(`<button class="${roleClass("status.filter")}"[^>]*>${str.viewFilterButton()}</button><span>${str.separator()}</span><button class="${roleClass("status.log.on")}"`));
  assert.match(top(pageWith({}, { panel: { sel: "+" } })), new RegExp(`<button class="${roleClass("status.filter.on")}"`));
});

test("Enter in the filter prompt adds the text, which becomes the highlight; Esc goes back to the panel", () => {
  const sent: any[] = [];
  const s = new Store();
  s.set({ phase: "running", act: (o: object) => sent.push(o) });
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" }, { type: "Log", Key: "fm/kit", Open: true, Find: "", Selected: 0 }]);
  s.setLogView("fm/kit", { panel: { sel: "+" }, prompt: { kind: "filter", value: " pie " } });
  filterPromptEnter(s);
  assert.deepEqual(sent.at(-1), { Do: "add_text", Key: "fm/kit", Term: "pie" });
  assert.deepEqual(s.state.logView["fm/kit"].panel, { sel: "t:pie", follow: true });
  assert.equal(s.state.logView["fm/kit"].prompt, undefined);
});
