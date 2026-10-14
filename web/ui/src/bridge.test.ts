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
  bridge(s, sock);
  ws.onopen();
  assert.equal(s.state.phase, "running");
  ws.onmessage({ data: JSON.stringify([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" }]) });
  assert.equal(s.state.active, "fm/kit");
});

test("what the page did before the first batch goes after it, in order", () => {
  const s = new Store();
  const { ws, sock, sent } = fake();
  bridge(s, sock);
  ws.onopen();
  s.state.act!({ Do: "here" });
  assert.equal(sent.length, 0, "held until Go's first batch");
  ws.onmessage({ data: "[]" });
  assert.deepEqual(sent.at(-1), { Do: "here" });
});
