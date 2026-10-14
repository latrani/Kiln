import { test } from "node:test";
import assert from "node:assert/strict";
import { bridge } from "./bridge.ts";
import { Store } from "./store.ts";

const fake = () => {
  const sent: any[] = [];
  const ws = { send: (s: string) => sent.push(JSON.parse(s)), onmessage: null as any, onopen: null as any };
  return { ws, sock: ws as unknown as WebSocket, sent };
};

test("the bridge runs the page as the wasm does: running once open, batches into the store", () => {
  const s = new Store();
  const { ws, sock } = fake();
  bridge(s, sock, { check: () => "", over: () => [] });
  ws.onopen();
  assert.equal(s.state.phase, "running");
  ws.onmessage({ data: JSON.stringify([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" }]) });
  assert.equal(s.state.active, "fm/kit");
});

test("what the page did before the first batch goes after it, in order", () => {
  const s = new Store();
  const { ws, sock, sent } = fake();
  bridge(s, sock, { check: () => "", over: () => [] });
  ws.onopen();
  s.state.act!({ Do: "here" });
  assert.equal(sent.length, 0, "held until Go's first batch");
  ws.onmessage({ data: "[]" });
  assert.deepEqual(sent.at(-1), { Do: "here" });
});

test("Go's field check, which the page calls as it's typed, is the one the bridge is given", () => {
  const s = new Store();
  const { sock } = fake();
  bridge(s, sock, { check: (f: string, v: string) => `${f}=${v}`, over: () => [] });
  assert.equal((globalThis as any).kiln.check("port", "x"), "port=x");
});

test("Go's over-limit ranges, which the input draws as it's typed, are the bridge's too", () => {
  const s = new Store();
  const { sock } = fake();
  bridge(s, sock, { check: () => "", over: () => [[1, 2]] });
  assert.deepEqual((globalThis as any).kiln.over("abc", 1, "batch"), [[1, 2]]);
});
