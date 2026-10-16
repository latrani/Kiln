import { test } from "node:test";
import assert from "node:assert/strict";
import { dragCursor, exportFormats, findStatus, formatKey, logKey, logRows, placeOf, keepsPlace, openAt, segments, topFor, wantsOlder } from "./log.ts";
import type { LogState } from "./log.ts";
import type { LineData } from "./store.ts";
import * as str from "./str.gen.ts";

const L = (Seq: number, Day: string, Plain = `l${Seq}`): LineData =>
  ({ Seq, Kind: "server", Day, DayLabel: "label " + Day, Clock: "12:00", Out: false, Plain, Spans: [{ Text: Plain }], Priority: false, Quiet: false });
const lines = [L(1, "2026-10-12"), L(2, "2026-10-12"), L(3, "2026-10-13"), L(4, "2026-10-13")];
const g = (more: Partial<LogState> = {}): LogState => ({ Key: "fm/kit", Open: true, Find: "", Status: "", StatusErr: false, Searching: false,
  Loading: false, HistDone: true, Selected: 0, ExportFormat: "", ...more });

test("a day rule before each day's first shown line; hidden lines aren't shown", () => {
  const rows = logRows(lines, g({ Hidden: [3] }));
  assert.deepEqual(rows.map((r) => (r.kind === "day" ? "day:" + r.label : r.line.Seq)), ["day:label 2026-10-12", 1, 2, "day:label 2026-10-13", 4]);
});

test("the range: from start to end by place; with only a start, just it; exclusions inside", () => {
  const sel = (more: Partial<LogState>) => logRows(lines, g(more)).flatMap((r) => (r.kind === "line" ? [`${r.line.Seq}${r.selected ? "s" : ""}${r.excluded ? "x" : ""}`] : []));
  assert.deepEqual(sel({ Start: 2, End: 4, Excluded: [3] }), ["1", "2s", "3sx", "4s"]);
  assert.deepEqual(sel({ Start: 2 }), ["1", "2s", "3", "4"]);
  assert.deepEqual(sel({ Start: 99, End: 4 }), ["1", "2", "3", "4"], "a start not loaded marks nothing");
});

test("matches are flagged; a match not loaded is nothing", () => {
  const m = logRows(lines, g({ Find: "l", Matches: [2, 4, 99] })).flatMap((r) => (r.kind === "line" && r.match ? [r.line.Seq] : []));
  assert.deepEqual(m, [2, 4]);
});

test("find status counts from the newest, as the TUI's; + while older history may hold more", () => {
  assert.equal(findStatus(g(), undefined, lines), "");
  assert.equal(findStatus(g({ Find: "l", Matches: [2, 4] }), 2, lines), str.browseFindStatus("l", 2, 2));
  assert.equal(findStatus(g({ Find: "l", Matches: [2, 4], HistDone: false }), 4, lines), str.browseFindStatusMore("l", 1, 2));
  assert.equal(findStatus(g({ Find: "l", Matches: [2, 4] }), 3, lines), str.browseFindStatus("l", 0, 2), "not on a match: 0");
  assert.equal(findStatus(g({ Find: "l", Searching: true }), undefined, lines), str.browseSearching("l", "2026-10-12"), "searching back to the oldest day read");
});

test("segments mark every case-insensitive, literal match", () => {
  assert.deepEqual(segments("Apple and apple.", "APPLE"), [{ text: "Apple", match: true }, { text: " and ", match: false }, { text: "apple", match: true }, { text: ".", match: false }]);
  assert.deepEqual(segments("a.b", "."), [{ text: "a", match: false }, { text: ".", match: true }, { text: "b", match: false }], "literal, not a pattern");
  assert.deepEqual(segments("abc", ""), [{ text: "abc", match: false }]);
});

test("log mode's letters, as the TUI's; modifiers aren't letters", () => {
  const k = (key: string, mods: { ctrlKey?: boolean; altKey?: boolean; metaKey?: boolean } = {}) => logKey({ key, ctrlKey: false, altKey: false, metaKey: false, ...mods });
  assert.deepEqual(["ArrowUp", "k", "ArrowDown", "j", "/", "n", "N", "g", "f", "e", "c", "t", "Escape"].map((x) => k(x)),
    ["up", "up", "down", "down", "find", "older", "newer", "date", "filter", "export", "copy", "tags", "back"]);
  assert.deepEqual(["PageUp", "PageDown", "Home", "End"].map((x) => k(x)), [undefined, undefined, undefined, undefined], "the browser's scrolling");
  assert.equal(k("a", { ctrlKey: true }), undefined, "Ctrl+A stays the browser's");
  assert.equal(k("a", { metaKey: true }), undefined, "and Cmd+A");
  assert.equal(k("n", { ctrlKey: true }), undefined);
  assert.equal(k("m"), undefined, "no cursor keys on the web (spec)");
  assert.deepEqual(exportFormats, { p: "plain", a: "ansi", h: "html" });
});

test("place: the first row showing and how far above the view's top it starts; put back after older rows arrive", () => {
  const rows = [{ seq: 5, top: 0, height: 20 }, { seq: 6, top: 20, height: 40 }, { seq: 7, top: 60, height: 20 }];
  assert.deepEqual(placeOf(rows, 30), { seq: 6, off: -10 });
  assert.equal(placeOf([], 0), undefined);
  const after = [{ seq: 1, top: 0, height: 100 }, ...rows.map((r) => ({ ...r, top: r.top + 100 }))];
  assert.equal(topFor({ seq: 6, off: -10 }, after), 130, "the same line, the same distance from the top");
  assert.equal(topFor({ seq: 99, off: 0 }, after), undefined, "gone: nothing to keep");
});

test("export's format prompt: p a h, Enter for a default, Esc back; never with a modifier", () => {
  const k = (key: string, mods: { ctrlKey?: boolean; altKey?: boolean; metaKey?: boolean } = {}) => ({ key, ctrlKey: false, altKey: false, metaKey: false, ...mods });
  assert.equal(formatKey(k("p"), ""), "plain");
  assert.equal(formatKey(k("h"), ""), "html");
  assert.equal(formatKey(k("Enter"), "ansi"), "ansi");
  assert.equal(formatKey(k("Enter"), ""), undefined);
  assert.equal(formatKey(k("Escape"), ""), "back");
  assert.equal(formatKey(k("p", { ctrlKey: true }), ""), undefined, "Ctrl+P is the browser's");
  assert.equal(formatKey(k("a", { metaKey: true }), ""), undefined);
  assert.equal(formatKey(k("h", { altKey: true }), ""), undefined);
});

test("older history is asked for near the top, or when what's loaded doesn't fill the view; not while it loads or once it's all there", () => {
  const g = { Loading: false, HistDone: false };
  assert.equal(wantsOlder(g, { scrollTop: 0, scrollHeight: 300, clientHeight: 500 }), true, "too short to scroll");
  assert.equal(wantsOlder(g, { scrollTop: 100, scrollHeight: 2000, clientHeight: 500 }), true, "near the top");
  assert.equal(wantsOlder(g, { scrollTop: 1500, scrollHeight: 2000, clientHeight: 500 }), false);
  assert.equal(wantsOlder({ ...g, Loading: true }, { scrollTop: 0, scrollHeight: 300, clientHeight: 500 }), false);
  assert.equal(wantsOlder({ ...g, HistDone: true }, { scrollTop: 0, scrollHeight: 300, clientHeight: 500 }), false);
});

test("place: a line hidden by the filter gives way to the next one shown", () => {
  const rows = [{ seq: 5, top: 0, height: 20 }, { seq: 7, top: 20, height: 20 }];
  assert.equal(topFor({ seq: 6, off: -10 }, rows), 20, "6 hidden: 7 at the top");
  assert.equal(topFor({ seq: 9, off: 0 }, rows), undefined, "nothing after it: nothing to keep");
});

test("the place is put back when older lines arrive or the filter changes what's hidden; not otherwise", () => {
  assert.equal(keepsPlace({ first: 1, hidden: "" }, { first: 0, hidden: "" }), true);
  assert.equal(keepsPlace({ first: 1, hidden: "" }, { first: 1, hidden: "3,4" }), true);
  assert.equal(keepsPlace({ first: 1, hidden: "3" }, { first: 1, hidden: "3" }), false);
  assert.equal(keepsPlace({ first: undefined, hidden: "" }, { first: 1, hidden: "" }), false, "the first draw: nowhere to keep");
});

test("showing a character's log: a landing made while away, else where you were, else the newest line", () => {
  const p = { seq: 3, off: 0 };
  assert.deepEqual(openAt(p, 1, 2, 9), { to: "line", seq: 9 }, "g or a find landed while it wasn't showing");
  assert.deepEqual(openAt(p, 2, 2, 9), { to: "place", place: p }, "nothing new: where you were");
  assert.deepEqual(openAt(undefined, 2, 2, 9), { to: "newest" });
  assert.deepEqual(openAt(undefined, undefined, 0, undefined), { to: "newest" }, "first shown");
});

test("dragCursor: the cursor stays put in view, and rides the edge it would leave by", () => {
  const rows = [1, 2, 3, 4, 5].map((seq, i) => ({ seq, top: i * 10, height: 10 }));
  assert.equal(dragCursor(rows, 10, 30, 3), undefined, "in view: put");
  assert.equal(dragCursor(rows, 15, 30, 2), 3, "scrolled off the top: the first line wholly in view");
  assert.equal(dragCursor(rows, 0, 25, 4), 2, "off the bottom: the last line wholly in view");
  assert.equal(dragCursor(rows, 0, 30, 9), undefined, "not drawn: put");
});
