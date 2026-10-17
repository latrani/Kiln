import { test } from "node:test";
import assert from "node:assert/strict";
import { cacheName, cleanCaches, onClick, onFetch, onPush } from "./sw.ts";
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
const get = (path: string, mode = "cors") => ({ url: scope + path, method: "GET", mode });

// net is a worker whose network answers with answer(url), or throws
// when it's undefined (offline).
function net(answer: ((url: string) => Response) | undefined) {
  const c = fakeCaches();
  const { w } = worker(0);
  w.caches = c.caches;
  w.fetch = async (r) => {
    if (!answer) throw new TypeError("offline");
    return answer(r.url);
  };
  const flush = () => new Promise((ok) => setTimeout(ok, 10)); // stores go on after the answer
  return { w, c, flush, go: (a: typeof answer) => { answer = a; } };
}

const ok = (body: string, etag: string) => () => new Response(body, { headers: { etag } });

test("online, the network answers, and a changed file is kept", async () => {
  const { w, c, flush, go } = net(ok("v1", "a"));
  assert.equal(await (await onFetch(w, get("app.js"))!).text(), "v1");
  await flush();
  assert.equal(c.puts(), 1);
  await onFetch(w, get("app.js"));
  await flush();
  assert.equal(c.puts(), 1, "an unchanged file is stored again");
  go(ok("v2", "b"));
  assert.equal(await (await onFetch(w, get("app.js"))!).text(), "v2", "online, a new build always wins");
  await flush();
  assert.equal(c.puts(), 2);
  go(undefined);
  assert.equal(await (await onFetch(w, get("app.js"))!).text(), "v2", "offline, the last one kept");
});

test("offline, a launch at any address of the page gets the kept page", async () => {
  const { w, flush, go } = net(ok("<html>", "p"));
  await onFetch(w, get("", "navigate"));
  await flush();
  go(undefined);
  for (const path of ["", "?record", "index.html"]) {
    const r = await onFetch(w, get(path, "navigate"));
    assert.equal(await (await r!).text(), "<html>", path);
  }
  await assert.rejects(onFetch(w, get("kiln.wasm"))!, "nothing kept: the browser's own offline answer");
});

test("a failed or opaque answer is passed on but never kept over a good one", async () => {
  const { w, c, flush, go } = net(ok("good", "g"));
  await onFetch(w, get("app.css"));
  await flush();
  for (const bad of [() => new Response("nope", { status: 404 }), () => new Response("oops", { status: 500 })]) {
    go(bad);
    await onFetch(w, get("app.css"));
    await flush();
  }
  assert.equal(c.puts(), 1);
  go(undefined);
  assert.equal(await (await onFetch(w, get("app.css"))!).text(), "good");
});

test("the relay, the preset, other sites and anything but GET are left to the browser", () => {
  const { w } = net(ok("x", "x"));
  for (const r of [get("relay"), get("relay?push_key=1"), get("relay?short=1"), get("preset/manifest.json"), get("preset/config/worlds/fm.toml"),
    { url: "https://elsewhere.example/kiln/app.js", method: "GET", mode: "cors" }, { url: "https://fm.example/other/app.js", method: "GET", mode: "cors" },
    { ...get("app.js"), method: "POST" }]) {
    assert.equal(onFetch(w, r), undefined, `${r.method} ${r.url}`);
  }
});

test("activating drops caches by other names", async () => {
  const c = fakeCaches([cacheName, "old"]);
  const { w } = worker(0);
  w.caches = c.caches;
  await cleanCaches(w);
  assert.deepEqual([...c.all.keys()], [cacheName]);
});
