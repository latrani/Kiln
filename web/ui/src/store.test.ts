import { test } from "node:test";
import assert from "node:assert/strict";
import { Store } from "./store.ts";
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
  s.apply([{ type: "Saved", Op: "add_character", Key: "fm/Ash" }]);
  assert.deepEqual({ ...s.state.answer }, { n: 1, Op: "add_character", Key: "fm/Ash", Problems: undefined });
  s.apply([{ type: "Problems", Op: "add_character", Problems: [{ Field: "name", Msg: "no" }] }]);
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
  s.apply([{ type: "Problems", Op: "save_world", Problems: [{ Field: "port", Msg: "no" }] }]);
  assert.deepEqual(editor(s)?.problems, [{ Field: "port", Msg: "no" }]);
  assert.equal(editor(s)?.pending, undefined);
  s.sendEditor({ Do: "save_world", World: "fm" });
  s.apply([{ type: "Saved", Op: "save_world" }]);
  assert.equal(s.state.picker, undefined);
});

test("an answer to something else leaves the editor be", () => {
  const { s } = running();
  s.openEditor({ kind: "world", world: "fm" }, false);
  s.apply([fmSettings]);
  s.apply([{ type: "Saved", Op: "save_world" }]);
  assert.ok(editor(s), "nothing was sent: not its answer");
  s.sendEditor({ Do: "save_world", World: "fm" });
  s.apply([{ type: "Saved", Op: "add_character", Key: "fm/x" }]);
  assert.equal(editor(s)?.pending, "save_world");
});

test("a new world saved: back to the picker, on its + Character", () => {
  const { s } = running();
  s.openPicker();
  s.openEditor({ kind: "new_world" }, false);
  s.apply([{ ...fmSettings, World: "" }]);
  s.editField("id", "zz");
  s.sendEditor({ Do: "add_world" });
  s.apply([{ type: "Worlds", Worlds: [{ ID: "fm", Characters: [] }, { ID: "zz", Characters: [] }] }, { type: "Saved", Op: "add_world" }]);
  assert.equal(editor(s), undefined);
  assert.equal(s.state.picker?.sel, "+zz");
});

test("deleting the edited character closes the editor; nothing's parked or drafted", () => {
  const { s } = running();
  s.openEditor({ kind: "char", world: "fm", char: "kit" }, true);
  s.apply([kitSettings]);
  s.sendEditor({ Do: "delete_character", World: "fm", Char: "kit" });
  s.apply([{ type: "Closed", Key: "fm/kit" }, { type: "Active", Key: "" }, { type: "Saved", Op: "delete_character" }]);
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
  s.apply([{ type: "Saved", Op: "delete_world" }]);
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
