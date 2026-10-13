import { test } from "node:test";
import assert from "node:assert/strict";
import { loadingText, readWithProgress, toBase64 } from "./boot.ts";
import * as str from "./str.gen.ts";

const res = (chunks: number[], length?: number, encoding?: string) =>
  new Response(new ReadableStream({
    start(c) {
      for (const n of chunks) c.enqueue(new Uint8Array(n));
      c.close();
    },
  }), { headers: { ...(length === undefined ? {} : { "content-length": String(length) }), ...(encoding ? { "content-encoding": encoding } : {}) } });

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

test("a compressed download's length isn't the size", async () => {
  const seen: [number, number | undefined][] = [];
  await readWithProgress(res([5, 5], 4, "gzip"), (g, t) => seen.push([g, t]));
  assert.deepEqual(seen, [[5, undefined], [10, undefined]]);
});

test("a length the download outgrows is dropped", async () => {
  const seen: [number, number | undefined][] = [];
  await readWithProgress(res([3, 3], 4), (g, t) => seen.push([g, t]));
  assert.deepEqual(seen, [[3, 4], [6, undefined]]);
});

test("a file's bytes go to Go as base64, however large", () => {
  const big = new Uint8Array(300_000).map((_, i) => i * 7);
  assert.equal(toBase64(big), Buffer.from(big).toString("base64"));
  assert.equal(toBase64(new Uint8Array()), "");
});
