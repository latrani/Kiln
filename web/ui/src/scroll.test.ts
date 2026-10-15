import { test } from "node:test";
import assert from "node:assert/strict";
import { sent, settle } from "./scroll.ts";
import type { View } from "./scroll.ts";
import type { LineData } from "./store.ts";

const L = (Seq: number, Quiet = false): LineData => ({ Seq, Kind: "server", Day: "", Out: false, Plain: "", Spans: [], Priority: false, Quiet });
const upTo = (n: number, from = 1) => Array.from({ length: n - from + 1 }, (_, i) => L(from + i));

test("a character shown for the first time starts live, its history seen", () => {
  const [v, move] = settle(undefined, 1, upTo(10), true);
  assert.deepEqual(move, { to: "end" });
  assert.equal(v.live, true);
  assert.equal(v.seen, 10);
});

test("switching back to a live character stops at the first line you haven't seen", () => {
  const was: View = { epoch: 1, live: true, seen: 10, last: 10, unseen: 0 };
  const [, move] = settle(was, 1, upTo(70), true);
  assert.deepEqual(move, { to: "end", pauseFrom: 10 }, "line 11 is index 10");
});

test("lines that came while a scrolled-up character was away keep your place and count", () => {
  const was: View = { epoch: 1, live: false, seen: 5, last: 10, unseen: 2, anchor: { seq: 4, off: 3 } };
  const [v, move] = settle(was, 1, upTo(40), true);
  assert.deepEqual(move, { to: "anchor" });
  assert.equal(v.unseen, 32);
  assert.deepEqual(v.anchor, { seq: 4, off: 3 });
});

test("quiet lines don't count as new", () => {
  const was: View = { epoch: 1, live: false, seen: 5, last: 10, unseen: 0 };
  const [v] = settle(was, 1, [...upTo(10), L(11, true), L(12)], false);
  assert.equal(v.unseen, 1);
});

test("a view from an earlier numbering starts over (the character reopened)", () => {
  const old: View = { epoch: 1, live: true, seen: 3, last: 3, unseen: 0 };
  const [v, move] = settle(old, 2, upTo(200), true);
  assert.deepEqual(move, { to: "end" });
  assert.equal(v.seen, 200);
  assert.equal(v.epoch, 2);
});

test("older history above doesn't move a live view off the end, nor count", () => {
  const was: View = { epoch: 1, live: true, seen: 20, last: 20, unseen: 0 };
  const [v, move] = settle(was, 1, upTo(20, -30), false);
  assert.deepEqual(move, { to: "end" });
  assert.equal(v.unseen, 0);
});

test("a live view takes new lines at the end, unless they overflow (the pager decides)", () => {
  const was: View = { epoch: 1, live: true, seen: 20, last: 20, unseen: 0 };
  assert.deepEqual(settle(was, 1, upTo(22), false)[1], { to: "end", pauseFrom: 20 });
});

test("sending goes to the end, everything seen: then what comes is live", () => {
  const was: View = { epoch: 1, live: false, seen: 5, last: 40, unseen: 35, anchor: { seq: 4, off: 3 } };
  const v = sent(was, upTo(41), false);
  assert.equal(v.live, true);
  assert.equal(v.unseen, 0);
  assert.equal(v.seen, 41);
  assert.deepEqual(settle(v, 1, upTo(41), false)[1], { to: "end" });
});

test("sending a line that's echoed leaves the echo unseen, so the pager can stop with it at the top", () => {
  const was: View = { epoch: 1, live: false, seen: 5, last: 41, unseen: 36 };
  const v = sent(was, upTo(41), true);
  assert.equal(v.seen, 40, "line 41 is the echo");
  // Lines after it that more than fill the pane stop the view at the echo.
  assert.deepEqual(settle(v, 1, upTo(120), false)[1], { to: "end", pauseFrom: 40 });
});
