import { test } from "node:test";
import assert from "node:assert/strict";
import { cleanCaches, keepLaunch, onClick, onFetch, onPush } from "./sw.ts";
import type { Options, Worker } from "./sw.ts";
import * as str from "./str.gen.ts";

function worker(windows: number, focused = false) {
  const shown: { title: string; opts: Options }[] = [];
  const done: string[] = [];
  const w: Worker = {
    registration: {
      scope: "https://fm.example/kiln/",
      async showNotification(title, opts) { shown.push({ title, opts }); },
    },
    clients: {
      async matchAll() { return Array.from({ length: windows }, () => ({ focused, async focus() { done.push("focus"); } })); },
      async openWindow(url) { done.push(`open ${url}`); },
    },
    caches: fakeCaches().caches,
    fetch: async () => { throw new TypeError("offline"); },
  };
  return { w, shown, done };
}

test("a push shows the away notification, replacing any before it and alerting again", async () => {
  const { w, shown } = worker(0);
  await onPush(w);
  assert.deepEqual(shown, [{ title: str.webPushTitle(), opts: { body: str.webPushBody(), tag: "kiln-away", renotify: true } }]);
});

test("a click closes it and focuses Kiln, or opens it", async () => {
  for (const [windows, want] of [[1, ["closed", "focus"]], [0, ["closed", "open https://fm.example/kiln/"]]] as const) {
    const { w, done } = worker(windows);
    await onClick(w, { notification: { close: () => done.push("closed") } });
    assert.deepEqual(done, want);
  }
});

test("with Kiln focused, a push shows nothing: the player is already looking", async () => {
  const { w, shown } = worker(1, true);
  await onPush(w);
  assert.deepEqual(shown, []);
  const unfocused = worker(1, false);
  await onPush(unfocused.w);
  assert.equal(unfocused.shown.length, 1);
});

// fakeCaches is CacheStorage over Maps; puts counts what's stored.
function fakeCaches(names: string[] = []) {
  const all = new Map<string, Map<string, Response>>(names.map((n) => [n, new Map()]));
  let puts = 0;
  const caches = {
    async open(name: string) {
      if (!all.has(name)) all.set(name, new Map());
      const m = all.get(name)!;
      return {
        async match(key: string) { return m.get(key)?.clone(); },
        async put(key: string, r: Response) { puts++; m.set(key, r); },
      };
    },
    async keys() { return [...all.keys()]; },
    async delete(name: string) { return all.delete(name); },
  };
  return { caches, all, puts: () => puts };
}

const scope = "https://fm.example/kiln/";
const req = (path: string, mode = "cors") => ({ url: scope + path, method: "GET", mode });
const files = ["", "app.js", "app.css", "wasm_exec.js", "kiln.wasm", "preset/manifest.json"].map((f) => scope + f);

// site is a worker whose network serves build: each file's text is
// `${path} ${build}`, stamped with the build; undefined is offline.
function site(build: string | undefined) {
  const c = fakeCaches();
  const { w } = worker(0);
  w.caches = c.caches;
  const fetched: string[] = [];
  let fail = "";
  w.fetch = async (r) => {
    const url = typeof r === "string" ? r : r.url;
    fetched.push(url);
    if (build === undefined || url === fail) throw new TypeError("offline");
    const path = url.slice(scope.length).replace(/\?.*/, "");
    return new Response(`${path} ${build}`, { headers: { "last-modified": build } });
  };
  return { w, c, fetched, go: (b: string | undefined) => { build = b; }, failOn: (u: string) => { fail = u; } };
}

const text = async (p: Promise<Response> | undefined) => (await p!).text();

test("online, every file comes from the network, and nothing is kept as it passes", async () => {
  const s = site("v1");
  assert.equal(await text(onFetch(s.w, req("app.js"), "page", "")), "app.js v1");
  assert.equal(s.c.puts(), 0);
});

test("a launch's files are kept as one set, and an offline launch gets that set", async () => {
  const s = site("v1");
  assert.equal(await keepLaunch(s.w, files), true);
  s.go(undefined);
  assert.equal(await text(onFetch(s.w, req("?record", "navigate"), "", "kit")), " v1");
  assert.equal(await text(onFetch(s.w, req("kiln.wasm"), "kit", "")), "kiln.wasm v1");
  assert.equal(await text(onFetch(s.w, req("preset/manifest.json"), "kit", "")), "preset/manifest.json v1");
});

test("a launch from the kept page gets only kept files, even once the network's back", async () => {
  const s = site("v1");
  await keepLaunch(s.w, files);
  s.go(undefined);
  await onFetch(s.w, req("", "navigate"), "", "kit");
  s.go("v2"); // a new build, and the network back, mid-launch
  assert.equal(await text(onFetch(s.w, req("kiln.wasm"), "kit", "")), "kiln.wasm v1", "a v1 page must run the v1 wasm");
});

test("online, a file that fails isn't answered from the kept set: no mixing builds", async () => {
  const s = site("v1");
  await keepLaunch(s.w, files);
  s.go("v2");
  // (Rook: the worker remembers pages by id, and Kit launched offline above.)
  assert.equal(await text(onFetch(s.w, req("", "navigate"), "", "rook")), " v2");
  s.failOn(scope + "kiln.wasm");
  await assert.rejects(onFetch(s.w, req("kiln.wasm"), "rook", "")!);
});

test("an unchanged launch keeps nothing new; a new build replaces the whole set", async () => {
  const s = site("v1");
  await keepLaunch(s.w, files);
  const puts = s.c.puts();
  assert.equal(await keepLaunch(s.w, files), true);
  assert.equal(s.c.puts(), puts, "an unchanged launch rewrote the set (the wasm is 15 MB)");
  s.go("v2");
  assert.equal(await keepLaunch(s.w, files), true);
  assert.equal([...s.c.all.keys()].filter((k) => k.startsWith("kiln-") && k !== "kiln-meta").length, 1, "the old set wasn't dropped");
  s.go(undefined);
  assert.equal(await text(onFetch(s.w, req("", "navigate"), "", "kit")), " v2");
  assert.equal(await text(onFetch(s.w, req("app.js"), "kit", "")), "app.js v2");
});

test("a set that can't all be fetched isn't kept: the last whole set stays", async () => {
  const s = site("v1");
  await keepLaunch(s.w, files);
  s.go("v2");
  s.failOn(scope + "kiln.wasm");
  assert.equal(await keepLaunch(s.w, files), false);
  s.go(undefined);
  assert.equal(await text(onFetch(s.w, req("", "navigate"), "", "kit")), " v1");
  assert.equal(await text(onFetch(s.w, req("kiln.wasm"), "kit", "")), "kiln.wasm v1");
});

test("a redirected or failed answer spoils the set", async () => {
  for (const bad of [
    () => Object.defineProperty(new Response("login"), "redirected", { value: true }),
    () => new Response("nope", { status: 404 }),
  ]) {
    const s = site("v1");
    s.w.fetch = async () => bad();
    assert.equal(await keepLaunch(s.w, files), false);
  }
});

test("offline with nothing kept, the browser gives its own answer", async () => {
  const s = site(undefined);
  await assert.rejects(onFetch(s.w, req("", "navigate"), "", "kit")!);
});

test("the relay, other sites and anything but GET are left to the browser", () => {
  const s = site("v1");
  for (const r of [req("relay"), req("relay?push_key=1"), req("relay?short=1"),
    { url: "https://elsewhere.example/kiln/app.js", method: "GET", mode: "cors" }, { url: "https://fm.example/other/app.js", method: "GET", mode: "cors" },
    { ...req("app.js"), method: "POST" }]) {
    assert.equal(onFetch(s.w, r, "kit", ""), undefined, `${r.method} ${r.url}`);
  }
});

test("cleaning drops Kiln's old sets, never another app's caches", async () => {
  const s = site("v1");
  await keepLaunch(s.w, files);
  await s.c.caches.open("kiln-0");
  await s.c.caches.open("kiln"); // the old single cache
  await s.c.caches.open("someone-elses");
  await cleanCaches(s.w);
  const left = [...s.c.all.keys()].sort();
  assert.equal(left.length, 3, JSON.stringify(left));
  assert.ok(left.includes("someone-elses") && left.includes("kiln-meta"));
});
