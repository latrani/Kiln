import { test } from "node:test";
import assert from "node:assert/strict";
import { webHost } from "./webhost.ts";

test("the web host asks the wasm's kiln once it's there", (t) => {
  const g = globalThis as any;
  const was = g.kiln;
  t.after(() => { g.kiln = was; });
  delete g.kiln;
  const h = webHost();
  assert.equal(h.check("port", "x"), "");
  assert.deepEqual(h.over("abc", 1, "batch"), []);
  g.kiln = { check: (f: string, v: string) => `${f}=${v}`, over: () => [[1, 2]] };
  assert.equal(h.check("port", "x"), "port=x");
  assert.deepEqual(h.over("abc", 1, "batch"), [[1, 2]]);
});

test("the web host can back up", () => {
  assert.equal(typeof webHost().restore, "function");
});
