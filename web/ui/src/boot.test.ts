import { test } from "node:test";
import assert from "node:assert/strict";
import { loadingText, readWithProgress } from "./boot.ts";
import * as str from "./str.gen.ts";

const res = (chunks: number[], length?: number) =>
  new Response(new ReadableStream({
    start(c) {
      for (const n of chunks) c.enqueue(new Uint8Array(n));
      c.close();
    },
  }), { headers: length === undefined ? {} : { "content-length": String(length) } });

test("progress counts every byte against the length", async () => {
  const seen: [number, number | undefined][] = [];
  const buf = await readWithProgress(res([3, 4], 7), (g, t) => seen.push([g, t]));
  assert.equal(buf.byteLength, 7);
  assert.deepEqual(seen, [[3, 7], [7, 7]]);
});

test("no length: progress without a total", async () => {
  const seen: [number, number | undefined][] = [];
  await readWithProgress(res([5]), (g, t) => seen.push([g, t]));
  assert.deepEqual(seen.at(-1), [5, undefined]);
});

test("a bad length is no length", async () => {
  const seen: [number, number | undefined][] = [];
  await readWithProgress(res([5], 0), (g, t) => seen.push([g, t]));
  assert.deepEqual(seen.at(-1), [5, undefined]);
});

test("the words, in megabytes to one place", () => {
  assert.equal(loadingText(2_100_000, 6_400_000), str.webLoadingSize("2.1", "6.4"));
  assert.equal(loadingText(2_100_000), str.webLoading("2.1"));
  assert.ok(!loadingText(0, 0).includes("NaN"));
});
