import { test } from "node:test";
import assert from "node:assert/strict";
import { glyphClick, panelAct, panelClick, togglePanel } from "./filter.ts";
import { cursorOf, logAct, logClick, selectChip, toggleLog } from "./log.ts";
import type { LogState } from "./log.ts";
import { listing, logShows, Store } from "./store.ts";
import * as str from "./str.gen.ts";
import type { LineData } from "./store.ts";

const line = (Seq: number, Plain = `l${Seq}`): LineData => ({ Seq, Kind: "server", Day: "", Out: false, Plain, Spans: [{ Text: Plain }], Priority: false, Quiet: false });
const quiet = <T>(f: () => T): T => {
  const was = console.error;
  console.error = () => {};
  try { return f(); } finally { console.error = was; }
};

test("a batch is one update", () => {
  const s = new Store();
  let n = 0;
  s.subscribe(() => n++);
  s.apply([{ type: "Order", Keys: ["fm/kit"] }, { type: "Char", Key: "fm/kit", Name: "Kit" }, { type: "Active", Key: "fm/kit" }]);
  assert.equal(n, 1);
  assert.deepEqual(s.state.order, ["fm/kit"]);
  assert.equal(s.state.chars["fm/kit"].Name, "Kit");
  assert.equal(s.state.active, "fm/kit");
});

test("Start drops everything the page held, but not the page's own state", () => {
  const s = new Store();
  s.set({ phase: "running", saving: false });
  s.apply([{ type: "Char", Key: "fm/kit" }, { type: "Lines", Key: "fm/kit", Epoch: 1, Op: "reset", Lines: [line(0)] },
    { type: "Log", Key: "fm/kit", Open: true }, { type: "LogLines", Key: "fm/kit", Epoch: 1, Op: "reset", Lines: [line(0)] },
    { type: "Status", Text: "x", Err: false, Gen: 1 }, { type: "Input", Key: "fm/kit", Text: "hi" }]);
  s.apply([{ type: "Start" }, { type: "Order", Keys: [] }]);
  assert.deepEqual(s.state.chars, {});
  assert.deepEqual(s.state.lines, {});
  assert.deepEqual(s.state.logs, {});
  assert.deepEqual(s.state.logLines, {});
  assert.deepEqual(s.state.inputs, {});
  assert.equal(s.state.status, undefined);
  assert.equal(s.state.phase, "running");
  assert.equal(s.state.saving, false);
});

test("lines append, prepend, replace and reset within an epoch", () => {
  const s = new Store();
  const L = (Op: string, Epoch: number, seqs: number[]) => ({ type: "Lines", Key: "k", Epoch, Op, Lines: seqs.map((q) => line(q)) });
  s.apply([L("reset", 1, [0, 1])]);
  s.apply([L("append", 1, [2])]);
  s.apply([L("prepend", 1, [-2, -1])]);
  assert.deepEqual(s.state.lines.k.lines.map((l) => l.Seq), [-2, -1, 0, 1, 2]);
  s.apply([{ type: "Lines", Key: "k", Epoch: 1, Op: "replace", Lines: ["a", "b", "c", "d", "e"].map((p, i) => line(i - 2, p)) }]);
  assert.deepEqual(s.state.lines.k.lines.map((l) => l.Plain), ["a", "b", "c", "d", "e"]);
  quiet(() => s.apply([L("append", 9, [3])])); // another epoch, not a reset: ignored
  assert.deepEqual(s.state.lines.k.lines.map((l) => l.Seq), [-2, -1, 0, 1, 2]);
  s.apply([L("reset", 2, [7])]);
  assert.deepEqual(s.state.lines.k, { epoch: 2, lines: [line(7)] });
});

test("log-mode lines keep their own numbering", () => {
  const s = new Store();
  s.apply([{ type: "LogLines", Key: "k", Epoch: 3, Op: "reset", Lines: [line(0)] }, { type: "LogLines", Key: "k", Epoch: 3, Op: "prepend", Lines: [line(-1)] }]);
  assert.deepEqual(s.state.logLines.k.lines.map((l) => l.Seq), [-1, 0]);
  assert.equal(s.state.lines.k, undefined);
});

test("Closed drops a character's things", () => {
  const s = new Store();
  s.apply([{ type: "Char", Key: "k" }, { type: "Lines", Key: "k", Epoch: 1, Op: "reset", Lines: [line(0)] }, { type: "Input", Key: "k", Text: "hi" },
    { type: "Log", Key: "k" }, { type: "Filter", Key: "k", Entries: [] }]);
  s.apply([{ type: "Closed", Key: "k" }]);
  for (const m of [s.state.chars, s.state.lines, s.state.inputs, s.state.logs, s.state.filters]) assert.equal(m.k, undefined);
});

test("a bad event doesn't stop the batch or reach the caller", () => {
  const s = new Store();
  quiet(() => assert.doesNotThrow(() => s.apply([{ type: "Lines", Key: "k", Epoch: 1, Op: "append" }, null as never, { type: "Active", Key: "k" }])));
  assert.equal(s.state.active, "k");
});

test("a subscriber that throws doesn't stop the others", () => {
  const s = new Store();
  let ran = false;
  s.subscribe(() => { throw new Error("x"); });
  s.subscribe(() => { ran = true; });
  quiet(() => s.apply([{ type: "Order", Keys: [] }]));
  assert.ok(ran);
});

test("effects go to the handler, in order, after the state", () => {
  const got: string[] = [];
  const s = new Store((e) => got.push(`${e.effect}:${s.state.active}`));
  s.apply([{ type: "Effect", effect: "Copy", Text: "x" }, { type: "Active", Key: "k" }, { type: "Effect", effect: "Notify", Title: "Kit", Body: "hi" }]);
  assert.deepEqual(got, ["Copy:k", "Notify:k"]);
});

test("the small things are kept", () => {
  const s = new Store();
  s.apply([{ type: "Theme", Appearance: "auto", Dark: { Roles: {}, Tags: {} }, Light: { Roles: {}, Tags: {} } },
    { type: "Worlds", Worlds: [{ ID: "fm", Characters: [] }] }, { type: "Presence", State: 1 },
    { type: "Confirm", On: true }, { type: "SaveQuestion", Asking: true }, { type: "Status", Text: "hi", Err: true, Gen: 4 }]);
  assert.equal(s.state.theme?.Appearance, "auto");
  assert.equal(s.state.worlds.length, 1);
  assert.equal(s.state.presence, 1);
  assert.equal(s.state.confirm, true);
  assert.equal(s.state.saveQuestion?.Asking, true);
  assert.deepEqual(s.state.status, { Text: "hi", Err: true, Gen: 4 });
});

test("the picker: opened with its highlight on the first character; Start drops it", () => {
  const s = new Store();
  s.apply([{ type: "Worlds", Worlds: [{ ID: "fm", Characters: [{ ID: "kit", Name: "Kit" }] }] }]);
  s.openPicker();
  assert.deepEqual(s.state.picker, { filter: "", sel: "fm/kit" });
  s.setPicker({ filter: "zzz" });
  assert.equal(s.state.picker?.sel, "+", "nothing matches: + World");
  s.apply([{ type: "Start" }]);
  assert.equal(s.state.picker, undefined);
});

test("Saved and Problems are the latest answer, numbered", () => {
  const s = new Store();
  s.apply([{ type: "Saved", Op: "add_character", World: "fm", Char: "", Key: "fm/Ash" }]);
  assert.deepEqual({ ...s.state.answer }, { n: 1, Op: "add_character", target: "=fm", Key: "fm/Ash", Problems: undefined });
  s.apply([{ type: "Problems", Op: "add_character", World: "fm", Char: "", Problems: [{ Field: "name", Msg: "no" }] }]);
  assert.equal(s.state.answer?.n, 2);
  assert.equal(s.state.answer?.Problems?.[0].Msg, "no");
});

test("the picker's highlight follows what Go sends: worlds coming, the highlighted character opening", () => {
  const s = new Store();
  s.openPicker(); // before the worlds came: nothing to highlight
  assert.equal(s.state.picker?.sel, "+", "no worlds yet: + World");
  s.apply([{ type: "Worlds", Worlds: [{ ID: "fm", Characters: [{ ID: "kit", Name: "Kit" }, { ID: "rook", Name: "Rook" }] }] }]);
  assert.equal(s.state.picker?.sel, "fm/kit");
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", Name: "Kit" }]);
  assert.equal(s.state.picker?.sel, "fm/rook", "Kit's open now: not offered");
});

// editor reads the open editor afresh (a plain s.state.picker read stays
// narrowed after an assert).
const editor = (s: Store) => s.state.picker?.edit;
const inh = { Login: "", MaxLineBytes: 4096, NewlineMode: "batch", Autoconnect: false, Reconnect: true, Notify: "first", LocalEcho: true };
const fmSettings = { type: "Settings", World: "fm", Char: "", Name: "", Inherited: inh,
  WorldSettings: { Host: "muck.test", Port: 8888, TLS: true, TLSTrust: "", Use: [], Login: null, MaxLineBytes: null, NewlineMode: null, Autoconnect: null, Reconnect: null, Notify: null, LocalEcho: null } };
const kitSettings = { type: "Settings", World: "fm", Char: "kit", Name: "Kit", Inherited: inh,
  CharSettings: { Aliases: [], Autoconnect: null, Reconnect: null, Notify: null, LocalEcho: null } };
const running = () => {
  const sent: any[] = [];
  const s = new Store();
  s.set({ phase: "running", act: (o: object) => sent.push(o) });
  s.apply([{ type: "Worlds", Worlds: [{ ID: "fm", Characters: [{ ID: "kit", Name: "Kit" }, { ID: "rook", Name: "Rook" }] }] },
    { type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Order", Keys: ["fm/kit"] }, { type: "Active", Key: "fm/kit" }]);
  return { s, sent };
};

test("opening an editor asks Go for its settings, and opens it, over the picker, when they come", () => {
  const { s, sent } = running();
  s.openEditor({ kind: "world", world: "fm" }, true);
  assert.deepEqual(sent.at(-1), { Do: "settings", World: "fm", Char: "" });
  assert.equal(s.state.picker, undefined, "nothing until the settings come");
  s.apply([fmSettings]);
  assert.equal(editor(s)?.title, str.editorEditing("fm"));
  assert.equal(editor(s)?.closeAll, true);
  s.apply([kitSettings]);
  assert.equal(editor(s)?.title, str.editorEditing("fm"), "settings nobody asked for open nothing");
});

test("Esc drops the edits; Ctrl+T hides them for next time; closeAll takes the picker too", () => {
  const { s } = running();
  s.openEditor({ kind: "world", world: "fm" }, false);
  s.apply([fmSettings]);
  s.editField("host", "changed");
  s.hideEditor();
  assert.ok(s.state.picker && !editor(s), "back to the picker");
  s.openEditor({ kind: "world", world: "fm" }, false);
  assert.equal(editor(s)?.values.host, "changed", "the draft, with no trip to Go");
  s.closeEditor();
  s.openEditor({ kind: "world", world: "fm" }, true);
  assert.equal(editor(s), undefined, "Esc dropped it: Go is asked again");
  s.apply([fmSettings]);
  s.closeEditor();
  assert.equal(s.state.picker, undefined, "opened from the main view: closing it closes the picker");
});

test("a save sends the form and waits; Go's refusal shows; Saved closes it", () => {
  const { s, sent } = running();
  s.openEditor({ kind: "world", world: "fm" }, true);
  s.apply([fmSettings]);
  s.sendEditor({ Do: "save_world", World: "fm" });
  assert.equal(editor(s)?.pending, "save_world");
  assert.deepEqual(sent.at(-1), { Do: "save_world", World: "fm" });
  s.apply([{ type: "Problems", Op: "save_world", World: "fm", Char: "", Problems: [{ Field: "port", Msg: "no" }] }]);
  assert.deepEqual(editor(s)?.problems, [{ Field: "port", Msg: "no" }]);
  assert.equal(editor(s)?.pending, undefined);
  s.sendEditor({ Do: "save_world", World: "fm" });
  s.apply([{ type: "Saved", Op: "save_world", World: "fm", Char: "" }]);
  assert.equal(s.state.picker, undefined);
});

test("an answer to something else leaves the editor be", () => {
  const { s } = running();
  s.openEditor({ kind: "world", world: "fm" }, false);
  s.apply([fmSettings]);
  s.apply([{ type: "Saved", Op: "save_world", World: "fm", Char: "" }]);
  assert.ok(editor(s), "nothing was sent: not its answer");
  s.sendEditor({ Do: "save_world", World: "fm" });
  s.apply([{ type: "Saved", Op: "add_character", World: "fm", Char: "", Key: "fm/x" }]);
  assert.equal(editor(s)?.pending, "save_world");
});

test("a new world saved: back to the picker, on its + Character", () => {
  const { s } = running();
  s.openPicker();
  s.openEditor({ kind: "new_world" }, false);
  s.apply([{ ...fmSettings, World: "" }]);
  s.editField("id", "zz");
  s.sendEditor({ Do: "add_world" });
  s.apply([{ type: "Worlds", Worlds: [{ ID: "fm", Characters: [] }, { ID: "zz", Characters: [] }] }, { type: "Saved", Op: "add_world", World: "", Char: "" }]);
  assert.equal(editor(s), undefined);
  assert.equal(s.state.picker?.sel, "+zz");
});

test("deleting the edited character closes the editor; nothing's parked or drafted", () => {
  const { s } = running();
  s.openEditor({ kind: "char", world: "fm", char: "kit" }, true);
  s.apply([kitSettings]);
  s.sendEditor({ Do: "delete_character", World: "fm", Char: "kit" });
  s.apply([{ type: "Closed", Key: "fm/kit" }, { type: "Active", Key: "" }, { type: "Saved", Op: "delete_character", World: "fm", Char: "kit" }]);
  assert.equal(s.state.picker, undefined);
  assert.deepEqual(s.state.parked, {});
  assert.deepEqual(s.state.drafts, {});
});

test("a deleted world's drafts go, so a new world of its name starts fresh", () => {
  const { s } = running();
  s.openEditor({ kind: "char", world: "fm", char: "kit" }, false);
  s.apply([kitSettings]);
  s.editField("aliases", "x");
  s.hideEditor();
  s.openEditor({ kind: "world", world: "fm" }, false);
  s.apply([fmSettings]);
  s.sendEditor({ Do: "delete_world", World: "fm" });
  s.apply([{ type: "Saved", Op: "delete_world", World: "fm", Char: "" }]);
  assert.deepEqual(s.state.drafts, {});
});

test("an editor opened from the main view stays with its character when you switch, and comes back with it", () => {
  const { s } = running();
  s.apply([{ type: "Char", Key: "fm/rook", World: "fm", ID: "rook", Name: "Rook", State: 2 }, { type: "Order", Keys: ["fm/kit", "fm/rook"] }]);
  s.openEditor({ kind: "char", world: "fm", char: "kit" }, true);
  s.apply([kitSettings]);
  s.editField("aliases", "x");
  s.apply([{ type: "Active", Key: "fm/rook" }]);
  assert.equal(s.state.picker, undefined, "parked: the picker goes with it");
  assert.equal(s.state.parked["fm/kit"]?.values.aliases, "x");
  s.apply([{ type: "Active", Key: "fm/kit" }]);
  assert.equal(editor(s)?.values.aliases, "x", "back, as it was");
  assert.deepEqual(s.state.parked, {});
});

test("a character closed with an editor parked on it keeps the edits as a draft", () => {
  const { s } = running();
  s.apply([{ type: "Char", Key: "fm/rook", World: "fm", ID: "rook", Name: "Rook", State: 2 }]);
  s.openEditor({ kind: "char", world: "fm", char: "kit" }, true);
  s.apply([kitSettings]);
  s.editField("aliases", "x");
  s.apply([{ type: "Active", Key: "fm/rook" }]);
  s.apply([{ type: "Closed", Key: "fm/kit" }]);
  assert.deepEqual(s.state.parked, {});
  assert.equal(s.state.drafts["fm/kit"]?.values.aliases, "x");
});

test("an editor opened in the picker stays open when the active character changes", () => {
  const { s } = running();
  s.apply([{ type: "Char", Key: "fm/rook", World: "fm", ID: "rook", Name: "Rook", State: 2 }]);
  s.openPicker();
  s.openEditor({ kind: "world", world: "fm" }, false);
  s.apply([fmSettings]);
  s.apply([{ type: "Active", Key: "fm/rook" }]);
  assert.ok(editor(s));
});

test("a first world saved lands on its + Character, though the worlds came in the same batch", () => {
  const s = new Store();
  s.set({ phase: "running", act: () => {} });
  s.openPicker();
  s.openEditor({ kind: "new_world" }, false);
  s.apply([{ ...fmSettings, World: "" }]);
  s.editField("id", "fm");
  s.sendEditor({ Do: "add_world" });
  s.apply([{ type: "Worlds", Worlds: [{ ID: "fm", Characters: [] }] }, { type: "Saved", Op: "add_world", World: "", Char: "" }]);
  assert.equal(s.state.picker?.sel, "+fm");
});

test("+ Connection with a main-view editor up parks it on its character, as the TUI's", () => {
  const { s } = running();
  s.openEditor({ kind: "char", world: "fm", char: "kit" }, true);
  s.apply([kitSettings]);
  s.editField("aliases", "x");
  s.openPicker();
  assert.equal(editor(s), undefined, "the picker, fresh");
  assert.equal(s.state.parked["fm/kit"]?.values.aliases, "x", "the edits kept with Kit");
});

test("the sidebar lists the picker only when no main-view editor is up, as the TUI's listing", () => {
  const { s } = running();
  assert.equal(listing(s.state), false);
  s.openPicker();
  assert.equal(listing(s.state), true);
  s.closePicker();
  s.openEditor({ kind: "char", world: "fm", char: "kit" }, true);
  s.apply([kitSettings]);
  assert.equal(listing(s.state), false, "Ctrl+T's editor keeps the characters showing");
});

test("log view: where find landed moves the view each time it's said; ExportName asks for the file name", () => {
  const s = new Store();
  s.apply([{ type: "Log", Key: "fm/kit", Open: true }]);
  assert.equal(logShows(s.state, "fm/kit"), true);
  s.apply([{ type: "LogAt", Key: "fm/kit", Seq: 4 }]);
  assert.deepEqual([s.state.logView["fm/kit"].at, s.state.logView["fm/kit"].goN], [4, 1]);
  s.apply([{ type: "LogAt", Key: "fm/kit", Seq: 4 }]);
  assert.equal(s.state.logView["fm/kit"].goN, 2, "the same line again still moves the view");
  s.apply([{ type: "LogLines", Key: "fm/kit", Epoch: 1, Op: "reset", Lines: [] }]);
  assert.equal(s.state.logView["fm/kit"].goN, 2, "lines arriving don't");
  s.apply([{ type: "ExportName", Key: "fm/kit", Format: "html", Name: "kit.html" }]);
  assert.deepEqual(s.state.logView["fm/kit"].prompt, { kind: "filename", value: "kit.html", format: "html" });
});

test("log view: hidden by Ctrl+L isn't shown; closing drops it; each character's is its own", () => {
  const s = new Store();
  s.apply([{ type: "Log", Key: "fm/kit", Open: true }, { type: "Log", Key: "fm/rook", Open: true }]);
  s.setLogView("fm/kit", { hidden: true, prompt: { kind: "find", value: "x" } });
  assert.equal(logShows(s.state, "fm/kit"), false);
  assert.equal(logShows(s.state, "fm/rook"), true);
  assert.equal(s.state.logView["fm/rook"]?.prompt, undefined);
  s.apply([{ type: "Log", Key: "fm/kit", Open: false }]);
  assert.equal(s.state.logView["fm/kit"], undefined);
  s.apply([{ type: "Closed", Key: "fm/rook" }]);
  assert.equal(s.state.logView["fm/rook"], undefined);
});

test("in log mode the picker and the editor are refused, as the TUI's picker is", () => {
  const sent: any[] = [];
  const s = new Store();
  s.set({ phase: "running", act: (o: object) => sent.push(o) });
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" }, { type: "Log", Key: "fm/kit", Open: true }]);
  s.openPicker();
  assert.equal(s.state.picker, undefined);
  assert.deepEqual(sent.at(-1), { Do: "say", Text: str.pickerBlockedByLog(), Err: true });
  s.openEditor({ kind: "char", world: "fm", char: "kit" }, true);
  assert.equal(sent.filter((a) => a.Do === "settings").length, 0);
  s.setLogView("fm/kit", { hidden: true });
  s.openPicker();
  assert.ok(s.state.picker, "hidden by Ctrl+L: the picker opens");
});

test("toggleLog: open, hide, show again; nothing open, nothing", () => {
  const sent: any[] = [];
  const s = new Store();
  s.set({ phase: "running", act: (o: object) => sent.push(o) });
  toggleLog(s);
  assert.equal(sent.length, 0);
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" }]);
  toggleLog(s);
  assert.deepEqual(sent.at(-1), { Do: "log", Key: "fm/kit" });
  s.apply([{ type: "Log", Key: "fm/kit", Open: true }]);
  toggleLog(s);
  assert.equal(logShows(s.state, "fm/kit"), false);
  toggleLog(s);
  assert.equal(logShows(s.state, "fm/kit"), true);
  assert.equal(sent.filter((a) => a.Do === "log").length, 2, "shown again, Go told (it clears the status, as the TUI's)");
});

// logged is a store with Kit's log mode open over lines 1-3, and what it
// sends.
const logged = (log: object = {}) => {
  const sent: any[] = [];
  const s = new Store();
  s.set({ phase: "running", act: (o: object) => sent.push(o) });
  const L = (Seq: number) => ({ Seq, Kind: "server", Day: "2026-10-13", Out: false, Plain: `l${Seq}`, Spans: null, Priority: false, Quiet: false });
  s.apply([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" },
    { type: "LogLines", Key: "fm/kit", Epoch: 1, Op: "reset", Lines: [L(1), L(2), L(3)] }, { type: "Log", Key: "fm/kit", Open: true, Find: "", Selected: 0, ...log }]);
  return { s, sent };
};

test("the cursor: the newest line at first; a click, find landing or ↑/↓ moves it; n goes from it", () => {
  const { s, sent } = logged({ Find: "l" });
  const cur = () => cursorOf(s.state.logs["fm/kit"] as LogState, s.state.logView["fm/kit"], s.state.logLines["fm/kit"].lines);
  assert.equal(cur(), 3, "the newest line, as the TUI's");
  s.apply([{ type: "LogAt", Key: "fm/kit", Seq: 1 }]);
  assert.equal(cur(), 1, "where find landed");
  const before = sent.length;
  logClick(s, 3, {});
  assert.deepEqual(sent.slice(before), [], "a click only moves the cursor");
  assert.equal(cur(), 3, "the line clicked");
  logAct(s, "older");
  assert.deepEqual(sent.at(-1), { Do: "find_older", Key: "fm/kit", Seq: 3 });
  logAct(s, "up");
  assert.deepEqual([cur(), s.state.logView["fm/kit"].follow], [2, true], "↑ moves it a line, into view");
  logAct(s, "down");
  logAct(s, "down");
  assert.equal(cur(), 3, "↓ stops at the newest");
  const n = sent.length;
  logAct(s, "up");
  logAct(s, "up");
  logAct(s, "up");
  assert.equal(cur(), 1, "↑ stops at the oldest loaded");
  assert.deepEqual(sent.slice(n), [{ Do: "log_older", Key: "fm/kit" }], "and asks for older history");
});

test("LineTags is kept for the status line; closing log mode drops it", () => {
  const { s } = logged();
  s.apply([{ type: "LineTags", Key: "fm/kit", Seq: 2, Text: str.browseLineTags("x") }]);
  assert.deepEqual(s.state.logView["fm/kit"].tags, { seq: 2, text: str.browseLineTags("x") });
  s.apply([{ type: "Log", Key: "fm/kit", Open: false }]);
  assert.equal(s.state.logView["fm/kit"], undefined);
});

test("g starts go-to at the cursor's day and time, to edit", () => {
  const { s } = logged();
  s.apply([{ type: "LogLines", Key: "fm/kit", Epoch: 1, Op: "reset", Lines: [1, 2].map((Seq) => ({ Seq, Kind: "server", Day: "2026-10-13", Clock: `21:0${Seq}`, Out: false, Plain: "x", Spans: null, Priority: false, Quiet: false }))}]);
  logAct(s, "date");
  assert.deepEqual(s.state.logView["fm/kit"].prompt, { kind: "date", value: "2026-10-13 21:02" });
  s.setLogView("fm/kit", { prompt: undefined, cursor: 1 });
  logAct(s, "date");
  assert.deepEqual(s.state.logView["fm/kit"].prompt, { kind: "date", value: "2026-10-13 21:01" });
});

test("the cursor skips lines the filter hides; on one it hides, it goes to the next shown", () => {
  const { s } = logged({ Hidden: [2] });
  const cur = () => cursorOf(s.state.logs["fm/kit"] as LogState, s.state.logView["fm/kit"], s.state.logLines["fm/kit"].lines);
  logAct(s, "up");
  assert.equal(cur(), 1);
  s.setLogView("fm/kit", { cursor: 2 });
  assert.equal(cur(), 3);
  s.apply([{ type: "Log", Key: "fm/kit", Open: true, Find: "", Selected: 0, Hidden: [2, 3] }]);
  assert.equal(cur(), 1, "nothing shown after it: the last shown before");
});

test("a click, or hiding log mode, stops a search reading older days, as the TUI's", () => {
  const { s, sent } = logged({ Find: "x", Searching: true });
  logClick(s, 2, {});
  assert.deepEqual(sent.at(-1), { Do: "stop_search", Key: "fm/kit" });
  toggleLog(s);
  assert.deepEqual(sent.at(-1), { Do: "stop_search", Key: "fm/kit" });
  assert.equal(logShows(s.state, "fm/kit"), false);
});

test("Ctrl+L does nothing over the picker or an editor: they keep the keys, as the TUI's", () => {
  const { s, sent } = logged();
  s.setLogView("fm/kit", { hidden: true });
  s.openPicker();
  toggleLog(s);
  assert.equal(logShows(s.state, "fm/kit"), false);
  assert.ok(s.state.picker);
  assert.equal(sent.filter((a) => a.Do === "log").length, 0);
});

test("e with nothing selected asks Go, which says to mark a range; no format prompt", () => {
  const { s, sent } = logged({ Selected: 0 });
  logAct(s, "export");
  assert.deepEqual(sent.at(-1), { Do: "export_name", Key: "fm/kit", Format: "" });
  assert.equal(s.state.logView["fm/kit"].prompt, undefined);
  const b = logged({ Start: 1, End: 2, Selected: 2 });
  logAct(b.s, "export");
  assert.deepEqual(b.s.state.logView["fm/kit"].prompt, { kind: "format" });
});

test("a click while a prompt is open changes nothing, as the TUI's: the prompt keeps it", () => {
  const { s, sent } = logged({ Start: 1, End: 1, Selected: 1 });
  s.setLogView("fm/kit", { prompt: { kind: "find", value: "x" } });
  logClick(s, 3, {});
  assert.equal(sent.length, 0);
  assert.equal(s.state.logView["fm/kit"].cursor, undefined);
});

test("select mode: s starts at the cursor; moving it while selecting sends mark_to; Esc drops it before find", () => {
  const { s, sent } = logged({ Find: "l" });
  logAct(s, "mark");
  assert.deepEqual(sent.at(-1), { Do: "mark", Key: "fm/kit", Seq: 3 });
  s.apply([{ type: "Log", Key: "fm/kit", Open: true, Find: "l", Selected: 1, Marking: true, Start: 3, End: 3 }]);
  logAct(s, "up");
  assert.deepEqual(sent.at(-1), { Do: "mark_to", Key: "fm/kit", Seq: 2 });
  logAct(s, "exclude");
  assert.deepEqual(sent.at(-1), { Do: "exclude", Key: "fm/kit", Seq: 2 });
  logAct(s, "back");
  assert.deepEqual(sent.at(-1), { Do: "end_mark", Key: "fm/kit" });
});

test("a click while selecting is a shift-click: inside, it takes the line out and leaves the cursor; outside, it grows to it", () => {
  const { s, sent } = logged({ Marking: true, Start: 2, End: 3, Selected: 2 });
  s.setLogView("fm/kit", { cursor: 3 });
  const n = sent.length;
  logClick(s, 2, { inRange: true });
  assert.deepEqual(sent.slice(n), [{ Do: "exclude", Key: "fm/kit", Seq: 2 }]);
  assert.equal(s.state.logView["fm/kit"].cursor, 3, "the cursor, the range's end, stays");
  logClick(s, 1, { shift: true, gutter: true });
  assert.deepEqual(sent.slice(n + 1), [{ Do: "grow", Key: "fm/kit", Seq: 1 }, { Do: "mark_to", Key: "fm/kit", Seq: 1 }], "shift and the gutter change nothing");
  assert.equal(s.state.logView["fm/kit"].cursor, 1);
});

test("the Select chip is s, as the TUI's: it stops a search, and does nothing under a prompt", () => {
  const { s, sent } = logged({ Find: "x", Searching: true });
  s.setLogView("fm/kit", { prompt: { kind: "find", value: "x" } });
  selectChip(s);
  assert.deepEqual(sent, [{ Do: "stop_search", Key: "fm/kit" }]);
  s.setLogView("fm/kit", { prompt: undefined });
  s.apply([{ type: "Log", Key: "fm/kit", Open: true, Find: "x", Selected: 0 }]);
  selectChip(s);
  assert.deepEqual(sent.at(-1), { Do: "mark", Key: "fm/kit", Seq: 3 });
});

// filtered is logged with a Filter: Untagged, a tag page with a child
// page/ooc, and a text row "x".
const filtered = (log: object = {}) => {
  const r = logged(log);
  const E = (Name: string, more: object = {}) => ({ Item: { Name, Text: false, Untagged: false }, Depth: 0, Parent: false, FoldedAway: false, Hidden: false, Only: false, Folded: false, FilteredUnder: false, ...more });
  r.s.apply([{ type: "Filter", Key: "fm/kit", Entries: [E("", { Item: { Name: "", Text: false, Untagged: true } }), E("page", { Parent: true }), E("page/ooc", { Depth: 1 }), E("x", { Item: { Name: "x", Text: true, Untagged: false } })] }]);
  return r;
};

test("f opens the panel on its first entry, focus following; f again closes it", () => {
  const { s } = filtered();
  logAct(s, "filter");
  assert.deepEqual(s.state.logView["fm/kit"].panel, { sel: "u", follow: true });
  logAct(s, "filter");
  assert.equal(s.state.logView["fm/kit"].panel, undefined);
});

test("the panel's keys act on the highlighted entry, as the TUI's", () => {
  const { s, sent } = filtered();
  togglePanel(s);
  panelAct(s, "down");
  assert.deepEqual(s.state.logView["fm/kit"].panel, { sel: "g:page", follow: true });
  panelAct(s, "hide");
  assert.deepEqual(sent.at(-1), { Do: "hide", Key: "fm/kit", Item: { Name: "page", Text: false, Untagged: false } });
  panelAct(s, "only");
  assert.deepEqual(sent.at(-1), { Do: "only", Key: "fm/kit", Item: { Name: "page", Text: false, Untagged: false } });
  panelAct(s, "shut");
  assert.deepEqual(sent.at(-1), { Do: "fold", Key: "fm/kit", Tag: "page", Shut: true });
  panelAct(s, "open");
  assert.deepEqual(sent.at(-1), { Do: "fold", Key: "fm/kit", Tag: "page", Shut: false });
  const n = sent.length;
  panelAct(s, "remove");
  assert.equal(sent.length, n, "a tag isn't removed");
  panelAct(s, "enter");
  assert.deepEqual(sent.at(-1), { Do: "fold", Key: "fm/kit", Tag: "page", Shut: true }, "Enter on a tag toggles its fold");
  panelAct(s, "toggle");
  assert.deepEqual(sent.at(-1), { Do: "fold", Key: "fm/kit", Tag: "page", Shut: true }, "Space too (Go hasn't said it's folded yet)");
  assert.equal(s.state.logView["fm/kit"].prompt, undefined, "Enter only asks for a text on + Text");
  panelAct(s, "down");
  const m = sent.length;
  panelAct(s, "toggle");
  panelAct(s, "enter");
  assert.equal(sent.length, m, "a tag with no children doesn't fold");
});

test("x on a text row removes it, the highlight moving down first; Enter on + Text asks for a text", () => {
  const { s, sent } = filtered();
  s.setLogView("fm/kit", { panel: { sel: "t:x" } });
  panelAct(s, "remove");
  assert.deepEqual(sent.at(-1), { Do: "remove_text", Key: "fm/kit", Term: "x" });
  assert.equal(s.state.logView["fm/kit"].panel?.sel, "+");
  panelAct(s, "hide");
  assert.equal(sent.at(-1).Do, "remove_text", "+ Text has no Hide");
  panelAct(s, "enter");
  assert.deepEqual(s.state.logView["fm/kit"].prompt, { kind: "filter", value: "" });
});

test("Esc in the log closes the panel first, then log mode", () => {
  const { s, sent } = filtered();
  togglePanel(s);
  logAct(s, "back");
  assert.equal(s.state.logView["fm/kit"].panel, undefined);
  assert.equal(sent.filter((a) => a.Do === "log_close").length, 0);
  logAct(s, "back");
  assert.deepEqual(sent.at(-1), { Do: "log_close", Key: "fm/kit" });
});

test("each character's panel is its own; Ctrl+L keeps it as it was", () => {
  const { s } = filtered();
  s.apply([{ type: "Char", Key: "fm/rook", World: "fm", ID: "rook", Name: "Rook", State: 2 }, { type: "Log", Key: "fm/rook", Open: true, Find: "", Selected: 0 }]);
  s.setLogView("fm/kit", { panel: { sel: "g:page" } });
  assert.equal(s.state.logView["fm/rook"]?.panel, undefined);
  toggleLog(s);
  toggleLog(s);
  assert.deepEqual(s.state.logView["fm/kit"].panel, { sel: "g:page" }, "hidden and shown again: as it was, focus left with the log");
});

test("not in log mode, the panel doesn't open", () => {
  const { s } = filtered();
  s.setLogView("fm/kit", { hidden: true });
  togglePanel(s);
  assert.equal(s.state.logView["fm/kit"].panel, undefined);
});

test("a click on a glyph highlights its entry and folds or removes it; focus follows only a text row's (a span takes none)", () => {
  const { s, sent } = filtered();
  togglePanel(s);
  glyphClick(s, "g:page");
  assert.deepEqual(s.state.logView["fm/kit"].panel, { sel: "g:page" }, "a group's glyph is a button and keeps focus");
  assert.deepEqual(sent.at(-1), { Do: "fold", Key: "fm/kit", Tag: "page", Shut: true });
  glyphClick(s, "t:x");
  assert.deepEqual(sent.at(-1), { Do: "remove_text", Key: "fm/kit", Term: "x" });
  assert.deepEqual(s.state.logView["fm/kit"].panel, { sel: "+", follow: true });
});

test("with a prompt open, clicks in the panel and the Filter chip do nothing, as the TUI's", () => {
  const { s, sent } = filtered();
  s.setLogView("fm/kit", { panel: { sel: "u" }, prompt: { kind: "filter", value: "" } });
  const n = sent.length;
  panelClick(s, "g:page", "hide");
  panelClick(s, "g:page");
  glyphClick(s, "g:page");
  togglePanel(s);
  assert.equal(sent.length, n);
  assert.deepEqual(s.state.logView["fm/kit"].panel, { sel: "u" });
});

test("clicks: a name highlights its entry; Hide and Only press too; + Text asks for a text", () => {
  const { s, sent } = filtered();
  togglePanel(s);
  panelClick(s, "g:page");
  assert.deepEqual(s.state.logView["fm/kit"].panel, { sel: "g:page" });
  panelClick(s, "g:page/ooc", "only");
  assert.deepEqual(sent.at(-1), { Do: "only", Key: "fm/kit", Item: { Name: "page/ooc", Text: false, Untagged: false } });
  assert.equal(s.state.logView["fm/kit"].panel?.sel, "g:page/ooc");
  panelClick(s, "+", "add");
  assert.deepEqual(s.state.logView["fm/kit"].prompt, { kind: "filter", value: "" });
});

test("a save answered while its editor's parked settles it there: saved, it's gone; refused, it comes back with why", () => {
  const { s } = running();
  s.apply([{ type: "Char", Key: "fm/rook", World: "fm", ID: "rook", Name: "Rook", State: 2 }, { type: "Order", Keys: ["fm/kit", "fm/rook"] }]);
  s.openEditor({ kind: "world", world: "fm" }, true);
  s.apply([fmSettings]);
  s.sendEditor({ Do: "save_world", World: "fm" });
  s.apply([{ type: "Active", Key: "fm/rook" }]);
  assert.ok(s.state.parked["fm/kit"], "parked mid-save");
  s.apply([{ type: "Problems", Op: "save_world", World: "fm", Char: "", Problems: [{ Field: "port", Msg: "no" }] }]);
  s.apply([{ type: "Active", Key: "fm/kit" }]);
  assert.deepEqual(editor(s)?.problems, [{ Field: "port", Msg: "no" }]);
  assert.equal(editor(s)?.pending, undefined, "Save works again");
  s.sendEditor({ Do: "save_world", World: "fm" });
  s.apply([{ type: "Active", Key: "fm/rook" }]);
  s.apply([{ type: "Saved", Op: "save_world", World: "fm", Char: "" }]);
  assert.equal(s.state.parked["fm/kit"], undefined, "saved: nothing left to come back");
});

test("a world deleted while its editor's hidden takes its drafts with it", () => {
  const { s } = running();
  s.openEditor({ kind: "world", world: "fm" }, false);
  s.apply([fmSettings]);
  s.sendEditor({ Do: "delete_world", World: "fm" });
  s.hideEditor();
  s.apply([{ type: "Saved", Op: "delete_world", World: "fm", Char: "" }]);
  assert.deepEqual(s.state.drafts, {});
});

test("an answer for no editor changes nothing; one for another target leaves the open editor be", () => {
  const { s } = running();
  s.openEditor({ kind: "world", world: "fm" }, false);
  s.apply([fmSettings]);
  s.sendEditor({ Do: "save_world", World: "fm" });
  s.apply([{ type: "Saved", Op: "save_world", World: "zz", Char: "" }]);
  assert.equal(editor(s)?.pending, "save_world", "zz's answer isn't fm's");
});

test("a settings read that fails stops the wait, so the editor never opens later", () => {
  const { s } = running();
  s.openEditor({ kind: "world", world: "fm" }, true);
  s.apply([{ type: "Problems", Op: "settings", World: "fm", Char: "" }]);
  assert.equal(s.state.opening, undefined);
  s.apply([fmSettings]);
  assert.equal(editor(s), undefined);
});

test("log mode's place is kept per character until log mode closes", () => {
  const { s } = logged();
  s.logPlaces.set("fm/kit", { seq: 2, off: 0 });
  s.apply([{ type: "Log", Key: "fm/kit", Open: true, Find: "", Selected: 0 }]);
  assert.deepEqual(s.logPlaces.get("fm/kit"), { seq: 2, off: 0 }, "still open: kept");
  s.logSeen.set("fm/kit", 3);
  s.apply([{ type: "Log", Key: "fm/kit", Open: false }]);
  assert.equal(s.logPlaces.get("fm/kit"), undefined, "Esc closed it: the newest line next time");
  assert.equal(s.logSeen.get("fm/kit"), undefined, "and no landing from before shows");
  s.logPlaces.set("fm/kit", { seq: 2, off: 0 });
  s.apply([{ type: "Closed", Key: "fm/kit" }]);
  assert.equal(s.logPlaces.has("fm/kit"), false);
});

test("an editor hidden and reopened mid-save still gets Go's answer", () => {
  const { s } = running();
  s.openEditor({ kind: "world", world: "fm" }, false);
  s.apply([fmSettings]);
  s.sendEditor({ Do: "save_world", World: "fm" });
  s.hideEditor();
  s.openEditor({ kind: "world", world: "fm" }, false);
  s.apply([{ type: "Problems", Op: "save_world", World: "fm", Char: "", Problems: [{ Field: "port", Msg: "no" }] }]);
  assert.deepEqual(editor(s)?.problems, [{ Field: "port", Msg: "no" }]);
});

test("an answer for an editor Esc'd away changes nothing, but a world deleted still takes its drafts", () => {
  const { s } = running();
  s.openEditor({ kind: "char", world: "fm", char: "kit" }, false);
  s.apply([kitSettings]);
  s.editField("aliases", "x");
  s.hideEditor();
  s.openEditor({ kind: "world", world: "fm" }, false);
  s.apply([fmSettings]);
  s.sendEditor({ Do: "save_world", World: "fm" });
  s.closeEditor();
  s.apply([{ type: "Saved", Op: "save_world", World: "fm", Char: "" }]);
  assert.ok(s.state.drafts["fm/kit"], "nothing changed");
  s.openEditor({ kind: "world", world: "fm" }, false);
  s.apply([fmSettings]);
  s.sendEditor({ Do: "delete_world", World: "fm" });
  s.closeEditor();
  s.apply([{ type: "Saved", Op: "delete_world", World: "fm", Char: "" }]);
  assert.deepEqual(s.state.drafts, {});
});

test("a Sent event is counted, so the scrollback hears each one, across a fresh start too", () => {
  const s = new Store();
  s.apply([{ type: "Sent", Key: "fm/kit", Seen: 40 }]);
  s.apply([{ type: "Start" }]);
  s.apply([{ type: "Sent", Key: "fm/kit", Seen: 41 }]);
  assert.deepEqual(s.state.sent, { key: "fm/kit", seen: 41, n: 2 });
});
