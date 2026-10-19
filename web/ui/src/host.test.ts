import { test } from "node:test";
import assert from "node:assert/strict";
import { noHost } from "./host.ts";
import type { Host } from "./host.ts";
import { effect } from "./page.tsx";
import { Store } from "./store.ts";
import type { Event } from "./store.ts";

// fakeHost records what the page asks of it; restore only when given.
function fakeHost(restore?: Host["restore"]) {
  const saved: [string, Uint8Array][] = [];
  const notified: Event[] = [];
  const host: Host = {
    ...noHost,
    saveFile: (name, bytes) => saved.push([name, bytes]),
    notify: (_store, e) => notified.push(e),
    ...(restore ? { restore } : {}),
  };
  return { host, saved, notified };
}

// acting is a store on host whose actions are kept.
function acting(host: Host) {
  const sent: object[] = [];
  const s = new Store(undefined, host);
  s.set({ act: (o: object) => sent.push(o) });
  return { s, sent };
}

test("a Store without a host asks the no-op one", () => {
  assert.equal(new Store().host, noHost);
  assert.equal(noHost.check("port", "x"), "");
  assert.deepEqual(noHost.over("abc", 1, "batch"), []);
});

test("SaveFile goes to the host", () => {
  const { host, saved } = fakeHost();
  const { s } = acting(host);
  effect(s)({ type: "Effect", effect: "SaveFile", Name: "logs/fm/Kit.txt", Data: btoa("hi") });
  assert.equal(saved.length, 1);
  assert.equal(saved[0][0], "Kit.txt");
  assert.equal(new TextDecoder().decode(saved[0][1]), "hi");
});

test("/restore without the backup capability does nothing", () => {
  const { host } = fakeHost();
  const { s, sent } = acting(host);
  effect(s)({ type: "Effect", effect: "Do", Cmd: "/restore" });
  assert.deepEqual(sent, []);
});

test("/restore with it hands the picked bytes to Go", () => {
  const { host } = fakeHost((done) => done(new Uint8Array([1, 2])));
  const { s, sent } = acting(host);
  effect(s)({ type: "Effect", effect: "Do", Cmd: "/restore" });
  assert.deepEqual(sent, [{ Do: "restore", Data: "AQI=" }]);
});

test("Notify goes to the host", () => {
  const { host, notified } = fakeHost();
  const { s } = acting(host);
  const e = { type: "Effect", effect: "Notify", Title: "Kit", Body: "Rook pages: hi" };
  effect(s)(e);
  assert.deepEqual(notified, [e]);
});
