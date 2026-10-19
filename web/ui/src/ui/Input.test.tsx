import { test } from "node:test";
import assert from "node:assert/strict";
import { noHost } from "../host.ts";
import type { Host, Ranges } from "../host.ts";
import { latest, over } from "./Input.tsx";

// later is a promise and what settles it.
function later<T>() {
  let ok!: (v: T) => void, fail!: (e: unknown) => void;
  const p = new Promise<T>((a, b) => { ok = a; fail = b; });
  return { p, ok, fail };
}
const settle = () => new Promise((r) => setTimeout(r, 0));

test("only the latest over answer is drawn", async () => {
  for (const order of [[1, 0], [0, 1]]) {
    const ask = latest<Ranges>();
    const drawn: Ranges[] = [];
    const answers = [later<Ranges>(), later<Ranges>()];
    const values: Ranges[] = [[[0, 1]], [[2, 3]]];
    for (const a of answers) ask(a.p, (r) => drawn.push(r));
    for (const i of order) answers[i].ok(values[i]);
    await settle();
    assert.deepEqual(drawn, [values[1]], `answered in order ${order}`);
  }
});

test("a failed over answer is logged, not thrown", async (t) => {
  const logged: unknown[] = [];
  t.mock.method(console, "error", (...a: unknown[]) => logged.push(a));
  const ask = latest<Ranges>();
  const drawn: Ranges[] = [];
  const a = later<Ranges>();
  ask(a.p, (r) => drawn.push(r));
  a.fail(new Error("gone"));
  await settle();
  assert.deepEqual(drawn, []);
  assert.equal(logged.length, 1);
});

test("no highlight before Kiln runs", async () => {
  let asked = 0;
  const host: Host = { ...noHost, over: async () => { asked++; return [[0, 1]]; } };
  assert.deepEqual(await over(host, "abc", undefined), []);
  assert.deepEqual(await over(host, "", { MaxLineBytes: 1 }), []);
  assert.equal(asked, 0);
});
