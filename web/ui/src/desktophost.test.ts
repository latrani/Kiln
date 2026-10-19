import { test } from "node:test";
import assert from "node:assert/strict";
import { batchEvent, desktopHost, openLinks, run } from "./desktophost.ts";
import type { Kiln } from "./desktophost.ts";
import { Store } from "./store.ts";
import * as str from "./str.gen.ts";

// fake is the Kiln bindings, recording what the page sends.
function fake(over: Partial<Kiln> = {}) {
  const calls: { name: string; args: unknown[] }[] = [];
  const rec = <T>(name: string, answer: T) => async (...args: unknown[]) => { calls.push({ name, args }); return answer; };
  const kiln: Kiln = {
    Act: rec("Act", undefined),
    Check: rec("Check", "not a port"),
    Over: rec("Over", [[1, 2]]),
    SaveAs: rec("SaveAs", undefined),
    OpenURL: rec("OpenURL", undefined),
    Start: rec("Start", undefined),
    ...over,
  };
  return { kiln, calls };
}

test("the desktop host asks the Kiln bindings", async () => {
  const { kiln, calls } = fake();
  const h = desktopHost(kiln);
  assert.equal(await h.check("port", "x"), "not a port");
  assert.deepEqual(await h.over("abc", 1, "batch"), [[1, 2]]);
  assert.deepEqual(calls, [{ name: "Check", args: ["port", "x"] }, { name: "Over", args: ["abc", 1, "batch"] }]);
});

test("the desktop host saves through SaveAs, the bytes as base64", () => {
  const { kiln, calls } = fake();
  desktopHost(kiln).saveFile("kit.txt", new TextEncoder().encode("hi Kit"));
  assert.deepEqual(calls, [{ name: "SaveAs", args: ["kit.txt", btoa("hi Kit")] }]);
});

test("a failed save isn't an unhandled rejection", async () => {
  const { kiln } = fake({ SaveAs: async () => { throw new Error("disk full"); } });
  const errs: unknown[] = [];
  const was = console.error;
  console.error = (...a: unknown[]) => { errs.push(a); };
  try {
    desktopHost(kiln).saveFile("kit.txt", new Uint8Array(1));
    await new Promise((r) => setTimeout(r, 0));
  } finally {
    console.error = was;
  }
  assert.equal(errs.length, 1);
});

test("the desktop host adds worlds, and has no backups, reset or take-over", () => {
  const h = desktopHost(fake().kiln);
  assert.equal(h.worlds, true);
  assert.equal(h.restore, undefined);
  assert.equal(h.storageFailed, undefined);
  assert.equal(h.reset, undefined);
  assert.equal(h.useHere, undefined);
});

test("the desktop host can't show notifications yet, and says why in the app's terms", async () => {
  const h = desktopHost(fake().kiln);
  assert.equal(await h.permission(), "unsupported");
  assert.equal(await h.askPermission(), "unsupported");
  assert.equal(h.whyNoNotify("denied"), str.desktopNotifyDenied());
  assert.equal(h.whyNoNotify("unsupported"), str.desktopNotifyUnsupported());
  assert.equal(h.whyNoNotify("granted"), "");
  assert.equal(h.whyNoNotify("default"), "");
});

// clicks is a document's capture-phase click listener, from openLinks.
function clicks(open: (url: string) => Promise<void>) {
  let listener: ((e: any) => void) | undefined;
  let capture = false;
  const doc = {
    addEventListener: (type: string, f: (e: any) => void, opts: AddEventListenerOptions) => {
      assert.equal(type, "click");
      listener = f;
      capture = opts.capture === true;
    },
  } as unknown as EventTarget;
  openLinks(doc, open);
  assert.ok(capture, "the listener captures, so the page's own handlers can't open a window first");
  // click is a click on an element inside a, or on nothing that's a link.
  return (a?: { href: string }) => {
    let prevented = false;
    const target = { closest: (sel: string) => (sel === "a[target=_blank]" ? a ?? null : null) };
    listener!({ target, preventDefault: () => { prevented = true; } });
    return prevented;
  };
}

test("a link opens in the system browser", () => {
  const opened: string[] = [];
  const click = clicks(async (url) => { opened.push(url); });
  assert.ok(click({ href: "https://fm.example/" }), "the webview doesn't open it too");
  assert.deepEqual(opened, ["https://fm.example/"]);
  assert.ok(!click(), "a click on anything else is the page's");
  assert.deepEqual(opened, ["https://fm.example/"]);
});

test("a link the app won't open isn't an unhandled rejection", async () => {
  const errs: unknown[] = [];
  const was = console.error;
  console.error = (...a: unknown[]) => { errs.push(a); };
  try {
    clicks(async () => { throw new Error("not a web link"); })({ href: "file:///etc/passwd" });
    await new Promise((r) => setTimeout(r, 0));
  } finally {
    console.error = was;
  }
  assert.equal(errs.length, 1);
});

// events is the Wails events the page listens to, which the test fires.
function events() {
  const on = new Map<string, (data: string) => void>();
  return { on: (name: string, f: (data: string) => void) => { on.set(name, f); }, fire: (name: string, data: string) => on.get(name)!(data) };
}

test("the page runs on the desktop as the wasm: batches into the store, actions held until the first", async () => {
  const { kiln, calls } = fake();
  const s = new Store(undefined, desktopHost(kiln));
  const ev = events();
  await run(s, kiln, ev.on);
  assert.deepEqual(calls.map((c) => c.name), ["Start"], "listening before Go starts");
  assert.equal(s.state.phase, "running");
  s.state.act!({ Do: "here" });
  assert.equal(calls.length, 1, "held until Go's first batch");
  ev.fire(batchEvent, JSON.stringify([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" }]));
  assert.equal(s.state.active, "fm/kit");
  assert.deepEqual(calls.at(-1), { name: "Act", args: [JSON.stringify({ Do: "here" })] });
});

test("Go not starting stops the page", async () => {
  const { kiln } = fake({ Start: async () => { throw new Error("no config"); } });
  const s = new Store(undefined, desktopHost(kiln));
  const was = console.error;
  console.error = () => {};
  try {
    await run(s, kiln, events().on);
  } finally {
    console.error = was;
  }
  assert.equal(s.state.phase, "stopped");
});
