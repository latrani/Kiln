import { test } from "node:test";
import assert from "node:assert/strict";
import { throttle } from "./presence.ts";

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
