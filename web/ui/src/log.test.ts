import { test } from "node:test";
import assert from "node:assert/strict";
import { exportFormats, findStatus, logKey, logRows, segments } from "./log.ts";
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
  assert.deepEqual(["/", "n", "N", "g", "e", "c", "t", "Escape"].map((x) => k(x)), ["find", "older", "newer", "date", "export", "copy", "tags", "back"]);
  assert.equal(k("a", { ctrlKey: true }), undefined, "Ctrl+A stays the browser's");
  assert.equal(k("a", { metaKey: true }), undefined, "and Cmd+A");
  assert.equal(k("n", { ctrlKey: true }), undefined);
  assert.equal(k("m"), undefined, "no cursor keys on the web (spec)");
  assert.deepEqual(exportFormats, { p: "plain", a: "ansi", h: "html" });
});
