import { test } from "node:test";
import assert from "node:assert/strict";
import { below, pause } from "./pager.ts";

const rows = (hs: number[]) => {
  const tops: number[] = [];
  let y = 0;
  for (const h of hs) { tops.push(y); y += h; }
  return { tops, heights: hs };
};

test("unseen lines that fit: stay live", () => {
  const { tops, heights } = rows([20, 20, 20, 20]);
  assert.equal(pause(tops, heights, 2, 100), undefined);
});

test("unseen lines that overflow: stop at the first of them", () => {
  const { tops, heights } = rows([20, 20, 20, 20, 20, 20, 20, 20]);
  assert.equal(pause(tops, heights, 2, 100), 40);
});

test("one line taller than the view: stop at its top", () => {
  const { tops, heights } = rows([20, 300, 20]);
  assert.equal(pause(tops, heights, 1, 100), 20);
});

test("nothing unseen: stay live", () => {
  const { tops, heights } = rows([20, 20]);
  assert.equal(pause(tops, heights, 2, 10), undefined);
});

test("the pill counts lines wholly below the view", () => {
  const { tops, heights } = rows([20, 20, 20, 20, 20, 20]);
  assert.equal(below(tops, 0, 50), 3); // lines 3, 4, 5 start at 60+
  assert.equal(below(tops, 20, 100), 0);
});
