import { test } from "node:test";
import assert from "node:assert/strict";
import { onClick, onPush } from "./sw.ts";
import type { Worker } from "./sw.ts";
import * as str from "./str.gen.ts";

function worker(windows: number) {
  const shown: { title: string; opts: NotificationOptions }[] = [];
  const done: string[] = [];
  const w: Worker = {
    registration: {
      scope: "https://fm.example/kiln/",
      async showNotification(title, opts) { shown.push({ title, opts }); },
    },
    clients: {
      async matchAll() { return Array.from({ length: windows }, () => ({ async focus() { done.push("focus"); } })); },
      async openWindow(url) { done.push(`open ${url}`); },
    },
  };
  return { w, shown, done };
}

test("a push shows the away notification, replacing any before it", async () => {
  const { w, shown } = worker(0);
  await onPush(w);
  assert.deepEqual(shown, [{ title: str.webPushTitle(), opts: { body: str.webPushBody(), tag: "kiln-away" } }]);
});

test("a click closes it and focuses Kiln, or opens it", async () => {
  for (const [windows, want] of [[1, ["closed", "focus"]], [0, ["closed", "open https://fm.example/kiln/"]]] as const) {
    const { w, done } = worker(windows);
    await onClick(w, { notification: { close: () => done.push("closed") } });
    assert.deepEqual(done, want);
  }
});
