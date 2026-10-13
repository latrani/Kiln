import { test } from "node:test";
import assert from "node:assert/strict";
import { switchAction, throttle } from "./presence.ts";

test("throttle runs the first call, then at most one a period", () => {
  let now = 0;
  const ran: number[] = [];
  const f = throttle(() => ran.push(now), 1000, () => now);
  for (const t of [0, 100, 900, 1000, 1500]) {
    now = t;
    f();
  }
  assert.deepEqual(ran, [0, 1000]);
});

const key = (k: string, mods: { ctrlKey?: boolean; altKey?: boolean; shiftKey?: boolean; metaKey?: boolean } = {}) =>
  ({ key: k, ctrlKey: false, altKey: false, shiftKey: false, metaKey: false, ...mods });

test("the switching keys ask Kiln's core where to go", () => {
  assert.deepEqual(switchAction(key("ArrowUp", { ctrlKey: true })), { Do: "step", N: -1 });
  assert.deepEqual(switchAction(key("ArrowDown", { ctrlKey: true })), { Do: "step", N: 1 });
  assert.deepEqual(switchAction(key("ArrowDown", { altKey: true })), { Do: "next_unread", N: 1 });
  assert.deepEqual(switchAction(key("ArrowUp", { altKey: true })), { Do: "next_unread", N: -1 });
});

test("other keys, and extra modifiers, aren't switching keys", () => {
  assert.equal(switchAction(key("ArrowUp")), undefined);
  assert.equal(switchAction(key("ArrowUp", { ctrlKey: true, shiftKey: true })), undefined);
  assert.equal(switchAction(key("ArrowDown", { altKey: true, metaKey: true })), undefined);
  assert.equal(switchAction(key("a", { ctrlKey: true })), undefined);
});
