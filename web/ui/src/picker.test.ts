import { test } from "node:test";
import assert from "node:assert/strict";
import { choose, fixPick, pickerKey, movePick, pagePick, pickable, pickerRows } from "./picker.ts";
import { Store } from "./store.ts";

const worlds = [
  { ID: "Bay", Characters: [{ ID: "ash", Name: "Ash", Aliases: ["cinder"] }] },
  { ID: "fm", Characters: [{ ID: "kit", Name: "Kit" }, { ID: "rook", Name: "Rook" }] },
];
const kinds = (rows: { kind: string }[]) => rows.map((r) => r.kind).join(" ");

test("characters not open, under their worlds, each world ending with + Character", () => {
  const rows = pickerRows(worlds, { "fm/kit": {} }, "");
  assert.equal(kinds(rows), "world char add_char world char add_char");
  assert.deepEqual(pickable(rows), ["Bay/ash", "+Bay", "fm/rook", "+fm"]);
});

test("the filter matches a name, an alias or the world, ignoring case and spaces around it", () => {
  assert.deepEqual(pickable(pickerRows(worlds, {}, " CIN ")), ["Bay/ash", "+Bay"]);
  assert.deepEqual(pickable(pickerRows(worlds, {}, "fm")), ["fm/kit", "fm/rook", "+fm"]);
});

test("a world whose name matches is listed with nothing to offer; one that doesn't, isn't", () => {
  const rows = pickerRows(worlds, { "fm/kit": {}, "fm/rook": {} }, "f");
  assert.equal(kinds(rows), "world add_char");
});

test("the highlight stays on its row, else goes to the first character, else the first row", () => {
  const rows = pickerRows(worlds, {}, "");
  assert.equal(fixPick(rows, "fm/kit"), "fm/kit");
  assert.equal(fixPick(rows, "gone/x"), "Bay/ash");
  assert.equal(fixPick(pickerRows(worlds, { "Bay/ash": {}, "fm/kit": {}, "fm/rook": {} }, ""), ""), "+Bay");
});

test("nothing matches: nothing to highlight", () => {
  const rows = pickerRows(worlds, {}, "zzz");
  assert.deepEqual(rows, []);
  assert.equal(fixPick(rows, "fm/kit"), "");
  assert.equal(movePick(rows, "", 1), "");
});

test("↑/↓ move a row, stopping at the ends; PgUp/PgDn a page of rows", () => {
  const rows = pickerRows(worlds, {}, "");
  assert.equal(movePick(rows, "Bay/ash", -1), "Bay/ash");
  assert.equal(movePick(rows, "Bay/ash", 1), "+Bay");
  assert.equal(movePick(rows, "+fm", 1), "+fm");
  assert.equal(pagePick(rows, "Bay/ash", 1, 3), "fm/kit", "three rows on: the world row, then Kit");
  assert.equal(pagePick(rows, "+fm", -1, 100), "Bay/ash");
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

test("Ctrl+O shows or hides the picker anywhere; Enter on a world's overview shows it", () => {
  assert.equal(pickerKey(key("o", { ctrlKey: true }), ""), "toggle");
  assert.equal(pickerKey(key("O", { ctrlKey: true, shiftKey: true }), ""), undefined);
  assert.equal(pickerKey(key("o", { ctrlKey: true, metaKey: true }), ""), undefined);
  assert.equal(pickerKey(key("Enter"), "=fm"), "open");
  assert.equal(pickerKey(key("Enter", {}, false), "=fm"), undefined, "Enter on a link or button is its own");
  assert.equal(pickerKey(key("Enter"), "fm/kit"), undefined);
});
