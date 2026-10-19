import { test } from "node:test";
import assert from "node:assert/strict";
import { webHost } from "./webhost.ts";
import * as str from "./str.gen.ts";

test("the web host asks the wasm's kiln once it's there", async (t) => {
  const g = globalThis as any;
  const was = g.kiln;
  t.after(() => { g.kiln = was; });
  delete g.kiln;
  const h = webHost();
  assert.equal(await h.check("port", "x"), "");
  assert.deepEqual(await h.over("abc", 1, "batch"), []);
  g.kiln = { check: (f: string, v: string) => `${f}=${v}`, over: () => [[1, 2]] };
  assert.equal(await h.check("port", "x"), "port=x");
  assert.deepEqual(await h.over("abc", 1, "batch"), [[1, 2]]);
});

test("the web host can back up", () => {
  assert.equal(typeof webHost().restore, "function");
});

test("the web host has no reset or take-over until the page gives them", async () => {
  const h = webHost();
  assert.equal(h.reset, undefined);
  assert.equal(h.useHere, undefined);
  let n = 0;
  h.provide({ reset: async () => { n++; } });
  await h.reset!();
  assert.equal(n, 1);
  assert.equal(h.useHere, undefined);
});

test("the web host says why notifications can't show, in the browser's terms", () => {
  const h = webHost();
  assert.equal(h.whyNoNotify("denied"), str.webNotifyDenied());
  assert.equal(h.whyNoNotify("unsupported"), str.webNotifyUnsupported());
  assert.equal(h.whyNoNotify("granted"), "");
  assert.equal(h.whyNoNotify("default"), "");
});

test("the web host asks the browser, and subscribes to pushes once it's allowed", async (t) => {
  const g = globalThis as any;
  const was = g.Notification;
  t.after(() => { g.Notification = was; });
  for (const [answer, want] of [["granted", 1], ["denied", 0]] as const) {
    g.Notification = class { static permission = "default"; static async requestPermission() { return answer; } };
    const h = webHost();
    let subscribed = 0;
    h.provide({ subscribe: () => subscribed++ });
    assert.equal(await h.permission(), "default");
    assert.equal(await h.askPermission(), answer);
    assert.equal(subscribed, want, answer);
  }
  delete g.Notification;
  assert.equal(await webHost().permission(), "unsupported");
  assert.equal(await webHost().askPermission(), "unsupported");
});
