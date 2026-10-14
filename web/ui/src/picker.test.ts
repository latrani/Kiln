import { test } from "node:test";
import assert from "node:assert/strict";
import { answered, choose, editorKey, fixPick, logToggleKey, pickerKey, movePick, pagePick, pickable, pickerRows } from "./picker.ts";
import { Store } from "./store.ts";

const worlds = [
  { ID: "Bay", Characters: [{ ID: "ash", Name: "Ash", Aliases: ["cinder"] }] },
  { ID: "fm", Characters: [{ ID: "kit", Name: "Kit" }, { ID: "rook", Name: "Rook" }] },
];
const kinds = (rows: { kind: string }[]) => rows.map((r) => r.kind).join(" ");

test("characters not open, under their worlds, each world ending with + Character, then + World", () => {
  const rows = pickerRows(worlds, { "fm/kit": {} }, "");
  assert.equal(kinds(rows), "world char add_char world char add_char add_world");
  assert.deepEqual(pickable(rows), ["=Bay", "Bay/ash", "+Bay", "=fm", "fm/rook", "+fm", "+"]);
});

test("the filter matches a name, an alias or the world, ignoring case and spaces around it", () => {
  assert.deepEqual(pickable(pickerRows(worlds, {}, " CIN ")), ["=Bay", "Bay/ash", "+Bay", "+"]);
  assert.deepEqual(pickable(pickerRows(worlds, {}, "fm")), ["=fm", "fm/kit", "fm/rook", "+fm", "+"]);
});

test("a world whose name matches is listed with nothing to offer; one that doesn't, isn't", () => {
  const rows = pickerRows(worlds, { "fm/kit": {}, "fm/rook": {} }, "f");
  assert.equal(kinds(rows), "world add_char add_world");
});

test("the highlight stays on its row, else goes to the first character, else the first row", () => {
  const rows = pickerRows(worlds, {}, "");
  assert.equal(fixPick(rows, "fm/kit"), "fm/kit");
  assert.equal(fixPick(rows, "=fm"), "=fm", "a world's row can be highlighted");
  assert.equal(fixPick(rows, "gone/x"), "Bay/ash");
  assert.equal(fixPick(pickerRows(worlds, { "Bay/ash": {}, "fm/kit": {}, "fm/rook": {} }, ""), ""), "=Bay");
});

test("nothing matches: only + World", () => {
  const rows = pickerRows(worlds, {}, "zzz");
  assert.equal(kinds(rows), "add_world");
  assert.equal(fixPick(rows, "fm/kit"), "+");
  assert.equal(movePick(rows, "+", 1), "+");
});

test("↑/↓ move a row, stopping at the ends; PgUp/PgDn a page of rows", () => {
  const rows = pickerRows(worlds, {}, "");
  assert.equal(movePick(rows, "Bay/ash", -1), "=Bay");
  assert.equal(movePick(rows, "=Bay", -1), "=Bay");
  assert.equal(movePick(rows, "+fm", 1), "+");
  assert.equal(movePick(rows, "+", 1), "+");
  assert.equal(pagePick(rows, "Bay/ash", 1, 3), "fm/kit", "three rows on: + Character, fm, then Kit");
  assert.equal(pagePick(rows, "+", -1, 100), "=Bay");
});

test("choosing a character picks it and closes the picker; + Character opens the name form", () => {
  const sent: object[] = [];
  const s = new Store();
  s.set({ act: (o) => sent.push(o) });
  s.apply([{ type: "Worlds", Worlds: [{ ID: "fm", Characters: [{ ID: "kit", Name: "Kit" }] }] }]);
  s.openPicker();
  choose(s, "+fm");
  assert.equal(s.state.picker?.adding, "fm");
  choose(s, "fm/kit");
  assert.deepEqual(sent, [{ Do: "pick", Key: "fm/kit" }]);
  assert.equal(s.state.picker, undefined);
});

const key = (key: string, mods: { ctrlKey?: boolean; altKey?: boolean; metaKey?: boolean; shiftKey?: boolean } = {}, onBody = true) =>
  ({ key, ctrlKey: false, altKey: false, metaKey: false, shiftKey: false, ...mods, onBody });

const at = (active: string, more: { phase?: "loading" | "running" | "stopped"; picker?: { filter: string; sel: string; adding?: string } } = {}) =>
  ({ active, phase: "running" as const, ...more });

test("Ctrl+O shows or hides the picker anywhere; Enter on a world's overview shows it", () => {
  assert.equal(pickerKey(key("o", { ctrlKey: true }), at("")), "open");
  assert.equal(pickerKey(key("o", { ctrlKey: true }), at("", { picker: { filter: "", sel: "" } })), "close");
  assert.equal(pickerKey(key("O", { ctrlKey: true, shiftKey: true }), at("")), undefined);
  assert.equal(pickerKey(key("o", { ctrlKey: true, metaKey: true }), at("")), undefined);
  assert.equal(pickerKey(key("Enter"), at("=fm")), "open");
  assert.equal(pickerKey(key("Enter", {}, false), at("=fm")), undefined, "Enter on a link or button is its own");
  assert.equal(pickerKey(key("Enter"), at("fm/kit")), undefined);
});

test("Ctrl+O is kept from the browser where it does nothing: naming a character, or Kiln not running", () => {
  assert.equal(pickerKey(key("o", { ctrlKey: true }), at("", { picker: { filter: "", sel: "+fm", adding: "fm" } })), "eat", "the TUI's name form ignores Ctrl+O");
  assert.equal(pickerKey(key("o", { ctrlKey: true }), at("", { phase: "loading" })), "eat", "not the browser's Open File");
  assert.equal(pickerKey(key("o", { ctrlKey: true }), at("", { phase: "stopped" })), "eat");
  assert.equal(pickerKey(key("Enter"), at("=fm", { phase: "stopped" })), undefined);
});

test("a saved character is picked; a refused name says why", () => {
  const sent: object[] = [];
  const s = new Store();
  s.set({ act: (o) => sent.push(o) });
  s.openPicker();
  s.setPicker({ adding: "fm", name: "Ash" });
  answered(s, { n: 1, Op: "add_character", Problems: [{ Field: "name", Msg: "taken" }] });
  assert.equal(s.state.picker?.error, "taken");
  answered(s, { n: 2, Op: "add_character", Key: "fm/Ash" });
  assert.deepEqual(sent, [{ Do: "pick", Key: "fm/Ash" }]);
  assert.equal(s.state.picker, undefined);
});

test("an answer to something else leaves the picker be", () => {
  const s = new Store();
  s.openPicker();
  s.setPicker({ adding: "fm" });
  answered(s, { n: 1, Op: "save_world", Key: "" });
  assert.equal(s.state.picker?.adding, "fm");
});

test("choosing a world's row or + World opens its editor (Go is asked); leaving one keeps its edits", () => {
  const sent: any[] = [];
  const s = new Store();
  s.set({ phase: "running", act: (o: object) => sent.push(o) });
  s.apply([{ type: "Worlds", Worlds: [{ ID: "fm", Characters: [{ ID: "kit", Name: "Kit" }] }] }]);
  s.openPicker();
  choose(s, "=fm");
  assert.deepEqual(sent.at(-1), { Do: "settings", World: "fm", Char: "" });
  choose(s, "+");
  assert.deepEqual(sent.at(-1), { Do: "settings", World: "", Char: "" });
});

const keyT = (mods: { ctrlKey?: boolean; altKey?: boolean }) => ({ ...key("t", mods), code: "KeyT" });
const altT = () => ({ ...key("†", { altKey: true }), code: "KeyT" });
const chars = { "fm/kit": { World: "fm", ID: "kit" } };
const on = (active: string, picker?: any) => ({ active, phase: "running" as const, picker, chars });

test("Ctrl+T or Alt+T: the active character's editor, or its world's from the overview, as the TUI's", () => {
  assert.deepEqual(editorKey(keyT({ ctrlKey: true }), on("fm/kit")), { open: { kind: "char", world: "fm", char: "kit" }, closeAll: true });
  assert.deepEqual(editorKey(altT(), on("=fm")), { open: { kind: "world", world: "fm" }, closeAll: true });
  assert.equal(editorKey(keyT({ ctrlKey: true }), on("")), "eat", "nothing open: nothing, and not the browser's");
});

test("in the picker: the highlighted world or character's editor; in an editor, Ctrl+T hides it", () => {
  assert.deepEqual(editorKey(keyT({ ctrlKey: true }), on("", { filter: "", sel: "=fm" })), { open: { kind: "world", world: "fm" }, closeAll: false });
  assert.deepEqual(editorKey(keyT({ ctrlKey: true }), on("", { filter: "", sel: "fm/rook" })), { open: { kind: "char", world: "fm", char: "rook" }, closeAll: false });
  assert.equal(editorKey(keyT({ ctrlKey: true }), on("", { filter: "", sel: "+fm" })), "eat");
  assert.equal(editorKey(keyT({ ctrlKey: true }), on("", { filter: "", sel: "", edit: {} })), "hide");
  assert.equal(editorKey(keyT({ ctrlKey: true }), on("", { filter: "", sel: "+fm", adding: "fm" })), "eat");
});

test("not Ctrl+T: Shift, Meta, or Kiln not running", () => {
  assert.equal(editorKey(keyT({}), on("fm/kit")), undefined);
  assert.equal(editorKey({ ...keyT({ ctrlKey: true }), shiftKey: true }, on("fm/kit")), undefined);
  assert.equal(editorKey(keyT({ ctrlKey: true }), { ...on("fm/kit"), phase: "stopped" as const }), "eat");
});

test("Ctrl+O while an editor is open does nothing, as the TUI's", () => {
  assert.equal(pickerKey(key("o", { ctrlKey: true }), at("", { picker: { filter: "", sel: "", edit: {} } as any })), "eat");
});

test("Enter on an overview with the picker or an editor up isn't the overview's", () => {
  assert.equal(pickerKey(key("Enter"), at("=fm", { picker: { filter: "", sel: "=fm" } })), undefined);
});

test("Alt+T goes by the letter where there is one, by the key's place where the Mac types a symbol", () => {
  const alt = (key: string, code: string) => ({ ...key_(key), code, altKey: true });
  assert.deepEqual(editorKey(alt("t", "KeyK"), on("fm/kit")), { open: { kind: "char", world: "fm", char: "kit" }, closeAll: true }, "Dvorak's T");
  assert.equal(editorKey(alt("y", "KeyT"), on("fm/kit")), undefined, "Dvorak's Y, where QWERTY has T");
  assert.notEqual(editorKey(alt("†", "KeyT"), on("fm/kit")), undefined, "a Mac's Option+T");
});
const key_ = (k: string) => key(k);

test("Ctrl+L, and only Ctrl+L, toggles log mode", () => {
  assert.equal(logToggleKey(key("l", { ctrlKey: true })), true);
  assert.equal(logToggleKey(key("l", { metaKey: true })), false, "Cmd+L is the address bar");
  assert.equal(logToggleKey(key("l", { ctrlKey: true, shiftKey: true })), false);
});
