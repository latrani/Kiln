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

test("/reset runs the page's reset", async () => {
  let resets = 0;
  const s = new Store();
  s.set({ reset: async () => { resets++; } });
  effect(s)({ type: "Effect", effect: "Do", Cmd: "/reset" });
  assert.equal(resets, 1);
});

test("a reset the browser refuses says so on the status line", async () => {
  const sent: any[] = [];
  const s = new Store();
  s.set({ act: (o: object) => sent.push(o), reset: () => Promise.reject(new Error("no")) });
  const was = console.error;
  console.error = () => {};
  try {
    effect(s)({ type: "Effect", effect: "Do", Cmd: "/reset" });
    await new Promise((r) => setTimeout(r));
  } finally {
    console.error = was;
  }
  assert.deepEqual(sent.at(-1), { Do: "say", Text: str.webResetFailed(), Err: true });
  assert.equal(s.state.saving, false, "saving has stopped, so the page says it isn't");
});
