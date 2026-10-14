import { test } from "node:test";
import assert from "node:assert/strict";
import { keyAction, strayKey, takes } from "./input.ts";

test("the page takes Go's text only when Go has its newest action and nothing's typed since", () => {
  assert.equal(takes(undefined, 0), true, "nothing typed yet");
  assert.equal(takes({ n: 3, dirty: false }, 3), true);
  assert.equal(takes({ n: 3, dirty: false }, 2), false, "a reply to an older action");
  assert.equal(takes({ n: 3, dirty: true }, 3), false, "typed since");
});

const k = (key: string, mods: { shiftKey?: boolean; altKey?: boolean; ctrlKey?: boolean; metaKey?: boolean; isComposing?: boolean; keyCode?: number } = {}) =>
  ({ key, shiftKey: false, altKey: false, ctrlKey: false, metaKey: false, isComposing: false, keyCode: 0, ...mods });
const at = (value: string, start: number, end = start) => ({ value, start, end });

test("Enter sends; Shift+Enter is a new line; Enter while composing is the IME's", () => {
  assert.equal(keyAction(k("Enter"), at("hi", 2)), "submit");
  assert.equal(keyAction(k("Enter", { shiftKey: true }), at("hi", 2)), undefined);
  assert.equal(keyAction(k("Enter", { isComposing: true }), at("hi", 2)), undefined);
  assert.equal(keyAction(k("Enter", { keyCode: 229 }), at("hi", 2)), undefined, "Safari's Enter confirming a conversion");
});

test("up and down recall history only from the first and last rows", () => {
  const v = "one\ntwo\nthree";
  assert.equal(keyAction(k("ArrowUp"), at(v, 2)), "older");
  assert.equal(keyAction(k("ArrowUp"), at(v, 6)), undefined, "the second row moves the caret");
  assert.equal(keyAction(k("ArrowDown"), at(v, 10)), "newer");
  assert.equal(keyAction(k("ArrowDown"), at(v, 2)), undefined, "the first row moves the caret");
  assert.equal(keyAction(k("ArrowUp"), at("", 0)), "older");
  assert.equal(keyAction(k("ArrowDown"), at("", 0)), "newer");
});

test("up with a selection, or with a modifier, isn't history", () => {
  assert.equal(keyAction(k("ArrowUp"), at("abc", 0, 2)), undefined);
  for (const m of [{ ctrlKey: true }, { altKey: true }, { metaKey: true }, { shiftKey: true }]) {
    assert.equal(keyAction(k("ArrowUp", m), at("abc", 1)), undefined, JSON.stringify(m));
  }
});

test("Esc cancels; PgUp and PgDn scroll the scrollback", () => {
  assert.equal(keyAction(k("Escape"), at("", 0)), "cancel");
  assert.equal(keyAction(k("PageUp"), at("", 0)), "page_up");
  assert.equal(keyAction(k("PageDown"), at("", 0)), "page_down");
  assert.equal(keyAction(k("a"), at("", 0)), undefined);
});

test("rows are screen rows when the page measured them: a wrapped line has several", () => {
  const long = "x".repeat(300); // one line, wrapped
  assert.equal(keyAction(k("ArrowUp"), { ...at(long, 300), first: false, last: true }), undefined, "the last row of three: the caret moves up");
  assert.equal(keyAction(k("ArrowUp"), { ...at(long, 5), first: true, last: false }), "older");
  assert.equal(keyAction(k("ArrowDown"), { ...at(long, 5), first: true, last: false }), undefined);
  assert.equal(keyAction(k("ArrowDown"), { ...at(long, 300), first: false, last: true }), "newer");
});

test("typed outside a control, these go to the input: letters, dead keys, Option and AltGr letters, IME; never a shortcut", () => {
  const k = (key: string, more: object = {}) => strayKey({ key, ctrlKey: false, altKey: false, metaKey: false, ...more });
  assert.equal(k("a"), true);
  assert.equal(k("Dead"), true, "a dead key: the accent, then its letter");
  assert.equal(k("Dead", { altKey: true }), true, "Option+E on a Mac");
  assert.equal(k("å", { altKey: true }), true, "Option+A");
  assert.equal(k("@", { ctrlKey: true, altKey: true }), true, "AltGr, as some browsers report it");
  assert.equal(k("Process", { keyCode: 229 }), true, "an IME starting");
  assert.equal(k("x", { isComposing: true }), true);
  assert.equal(k("t", { ctrlKey: true }), false);
  assert.equal(k("v", { metaKey: true }), false);
  assert.equal(k("†", { altKey: true, defaultPrevented: true }), false, "Alt+T: the page took it");
  assert.equal(k("ArrowDown", { altKey: true }), false);
  assert.equal(k("Enter"), false);
  assert.equal(k("Tab"), false);
  assert.equal(k("d", { altKey: true }), false, "Alt+D on Windows or Linux: the browser's, not a letter");
});
