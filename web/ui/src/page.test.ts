import { test } from "node:test";
import assert from "node:assert/strict";
import { effect } from "./page.tsx";
import { Store } from "./store.ts";
import * as str from "./str.gen.ts";

const withClipboard = async (clipboard: unknown, f: () => Promise<void>) => {
  const was = Object.getOwnPropertyDescriptor(globalThis, "navigator");
  Object.defineProperty(globalThis, "navigator", { value: { clipboard }, configurable: true });
  try {
    await f();
  } finally {
    if (was) Object.defineProperty(globalThis, "navigator", was);
  }
};

for (const [what, clipboard] of [["refused", { writeText: () => Promise.reject(new Error("no")) }], ["can't reach", undefined]] as const) {
  test(`a copy the browser ${what} says so on the status line`, async () => {
    const sent: any[] = [];
    const s = new Store();
    s.set({ act: (o: object) => sent.push(o) });
    await withClipboard(clipboard, async () => {
      effect(s)({ type: "Effect", effect: "Copy", Text: "x" });
      await new Promise((r) => setTimeout(r));
    });
    assert.deepEqual(sent.at(-1), { Do: "say", Text: str.webCopyRefused(), Err: true });
  });
}
