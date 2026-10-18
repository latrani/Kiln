import { test } from "node:test";
import assert from "node:assert/strict";
import { deleteAction, edit, errorOf, firstError, formError, newEditor, refuse, saveAction, targetKey, unsaved } from "./editor.ts";
import * as str from "./str.gen.ts";

const inh = { Login: "", MaxLineBytes: 4096, NewlineMode: "batch", Autoconnect: false, Reconnect: true, Notify: "first", LocalEcho: true };
const fmSettings = { Host: "muck.test", Port: 8888, TLS: true, TLSTrust: "", Use: ["fuzzball"], Login: null, MaxLineBytes: 2048,
  NewlineMode: null, Autoconnect: true, Reconnect: null, Notify: null, LocalEcho: null };
const fm = () => newEditor({ World: "fm", Char: "", Name: "", WorldSettings: fmSettings, Inherited: inh }, false);
const kit = () => newEditor({ World: "fm", Char: "kit", Name: "Kit", CharSettings: { Aliases: ["kitty"], Autoconnect: null, Reconnect: false, Notify: "all", LocalEcho: null }, Inherited: inh }, true);

test("a world's form: the TUI's fields, in order, the rest behind More settings", () => {
  const ed = fm();
  assert.equal(ed.title, str.editorEditing("fm"));
  assert.deepEqual(ed.fields.filter((f) => !f.extra).map((f) => f.key), ["host", "port", "tls"]);
  assert.deepEqual(ed.fields.filter((f) => f.extra).map((f) => f.key),
    ["tls_trust", "use", "login", "max_line_bytes", "newline_mode", "autoconnect", "reconnect", "notify", "local_echo"]);
});

test("a world's host, port, TLS and cert trust are the relay's: shown, not edited", () => {
  assert.deepEqual(fm().fields.filter((f) => f.fixed).map((f) => f.key), ["host", "port", "tls", "tls_trust"], "and how it's trusted: the relay decides");
});

test("a choice offers what it inherits first, as the TUI's", () => {
  const nl = fm().fields.find((f) => f.key === "newline_mode")!;
  assert.ok(nl.kind === "choice");
  assert.deepEqual(nl.options.map((o) => o.shown), [str.editorDefault("batch"), "batch", "flatten"]);
  const rc = fm().fields.find((f) => f.key === "reconnect")!;
  assert.ok(rc.kind === "choice");
  assert.deepEqual(rc.options.map((o) => o.shown), [str.editorDefault(str.editorOn()), str.editorOn(), str.editorOff()]);
  const mb = fm().fields.find((f) => f.key === "max_line_bytes")!;
  assert.ok(mb.kind === "text" && mb.hint === str.editorDefaultHint(4096));
});

test("a character's form: aliases and what it can set, nothing hidden", () => {
  const ed = kit();
  assert.equal(ed.title, str.editorEditing("fm/Kit"));
  assert.deepEqual(ed.fields.map((f) => f.key), ["aliases", "autoconnect", "reconnect", "notify", "local_echo"]);
  assert.ok(ed.fields.every((f) => !f.extra));
});

test("saving untouched writes back what was read: unset stays unset", () => {
  assert.deepEqual(saveAction(fm()), { Do: "save_world", World: "fm", WorldSettings: fmSettings });
  assert.deepEqual(saveAction(kit()), { Do: "save_character", World: "fm", Char: "kit",
    CharSettings: { Aliases: ["kitty"], Autoconnect: null, Reconnect: false, Notify: "all", LocalEcho: null } });
});

test("edits read as the TUI reads them: lists split on commas and spaces, numbers or 0", () => {
  let ed = fm();
  for (const [k, v] of [["use", "a, b c"], ["max_line_bytes", "12x"], ["reconnect", "off"]] as const) ed = edit(ed, k, v);
  const a = saveAction(ed) as any;
  assert.equal(a.Do, "save_world");
  assert.deepEqual([a.WorldSettings.Host, a.WorldSettings.Port, a.WorldSettings.TLS], ["muck.test", 8888, true], "the relay's, as read");
  assert.deepEqual(a.WorldSettings.Use, ["a", "b", "c"]);
  assert.equal(a.WorldSettings.MaxLineBytes, 0, "an unreadable number is 0, which the save refuses");
  ed = edit(ed, "max_line_bytes", "");
  assert.equal((saveAction(ed) as any).WorldSettings.MaxLineBytes, null);
  assert.equal(a.WorldSettings.Reconnect, false);
});

test("unsaved: the title says so once a value differs from what was read", () => {
  const ed = fm();
  assert.equal(unsaved(ed), false);
  assert.equal(unsaved(edit(ed, "host", "x")), true);
  assert.equal(unsaved(edit(edit(ed, "host", "x"), "host", "muck.test")), false);
});

test("a live check shows beside its field until fixed; an edit clears that field's save problem and disarms Delete", () => {
  const check = (f: string, v: string) => (f === "port" && /\D/.test(v) ? str.editorPortNumber() : "");
  let ed = edit(fm(), "port", "88a", check);
  assert.equal(errorOf(ed, "port"), str.editorPortNumber());
  ed = edit(ed, "port", "88", check);
  assert.equal(errorOf(ed, "port"), undefined);
  ed = { ...refuse(ed, [{ Field: "host", Msg: "bad host" }, { Field: "", Msg: "disk full" }]), armed: true };
  assert.equal(errorOf(ed, "host"), "bad host");
  assert.equal(formError(ed), "disk full");
  ed = edit(ed, "host", "h");
  assert.equal(errorOf(ed, "host"), undefined);
  assert.equal(ed.armed, false);
});

test("a refusal counts, opens More settings for a field inside it, and names the first field to fix", () => {
  const ed = refuse(fm(), [{ Field: "notify", Msg: "x" }, { Field: "port", Msg: "y" }]);
  assert.equal(ed.refused, 1);
  assert.equal(ed.more, true);
  assert.equal(firstError(ed), "port", "in the form's order, not Go's");
  assert.equal(refuse(fm(), [{ Field: "", Msg: "z" }]).more, false);
});

test("delete: a world or a character", () => {
  assert.deepEqual(deleteAction(fm()), { Do: "delete_world", World: "fm" });
  assert.deepEqual(deleteAction(kit()), { Do: "delete_character", World: "fm", Char: "kit" });
});

test("targets key as the picker's rows do", () => {
  assert.equal(targetKey(fm().target), "=fm");
  assert.equal(targetKey(kit().target), "fm/kit");
});
