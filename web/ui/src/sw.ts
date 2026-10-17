// The service worker. It shows the relay's pushes (push.ts) and opens
// Kiln when one's clicked, and keeps the page's files for a launch with
// no network.
//
// Online, the network always answers (the relay serves the files
// no-cache, so an unchanged one is a cheap 304), so a new build is never
// kept waiting behind an old one. Once a launch is running, the page
// tells the worker which files it used, and the worker keeps them as one
// set (keepLaunch): a launch with no network gets every file from the
// same build, never a new app.js with an old kiln.wasm.
import { awayTag, ownsCache } from "./push.ts";
import * as str from "./str.gen.ts";

// Options are a notification's: TypeScript's DOM types lack renotify,
// which Chromium honors (others ignore it).
export type Options = NotificationOptions & { renotify?: boolean };

// Worker is the parts of the worker's global scope used here (tests pass
// their own).
export interface Worker {
  registration: { scope: string; showNotification(title: string, opts: Options): Promise<void> };
  clients: {
    matchAll(o: { type: "window"; includeUncontrolled: boolean }): Promise<readonly { focused?: boolean; focus(): Promise<unknown> }[]>;
    openWindow(url: string): Promise<unknown>;
  };
  caches: {
    open(name: string): Promise<{ match(key: string): Promise<Response | undefined>; put(key: string, r: Response): Promise<void> }>;
    keys(): Promise<string[]>;
    delete(name: string): Promise<boolean>;
  };
  fetch(r: Req | string): Promise<Response>;
}

// Req is the parts of a fetch event's request used here.
export interface Req {
  url: string;
  method: string;
  mode: string;
}

// The kept set: its cache's name and each file's stamp, under metaKey
// in the meta cache. Switching sets is one write there.
const metaCache = "kiln-meta"; // str:ok cache name
const metaKey = "current"; // str:ok cache key
interface Kept {
  name: string;
  stamps: Record<string, string>;
}

async function kept(w: Worker): Promise<Kept | undefined> {
  const r = await (await w.caches.open(metaCache)).match(metaKey);
  return r ? r.json() : undefined;
}

// fromKept is url from the kept set (for a navigation, also the page
// itself, whatever address it was launched at), if there is one.
async function fromKept(w: Worker, keys: string[]): Promise<Response | undefined> {
  const k = await kept(w);
  if (!k) return undefined;
  const cache = await w.caches.open(k.name);
  for (const key of keys) {
    const hit = await cache.match(key);
    if (hit) return hit;
  }
  return undefined;
}

// offline are the pages launched from the kept set (by client id): all
// their files come from it too, even once the network is back, so they
// run one build. A worker that restarts forgets them, and their next
// file then needs the network: a failed launch, never a mixed one.
const offline = new Set<string>();

// onFetch answers req, for the page (clientId) asking or the page a
// navigation will make (resultingClientId): from the network, or, when
// it fails for a navigation, from the kept set. undefined: not the
// page's own file (the relay, another site, not a GET), so the browser
// fetches it as if there were no worker.
export function onFetch(w: Worker, req: Req, clientId: string, resultingClientId: string): Promise<Response> | undefined {
  const scope = new URL(w.registration.scope);
  const url = new URL(req.url);
  if (req.method !== "GET" || url.origin !== scope.origin || !url.pathname.startsWith(scope.pathname)) return undefined;
  if (url.pathname.slice(scope.pathname.length) === "relay") return undefined; // str:ok path
  const key = url.origin + url.pathname; // ?record and the like don't make another file
  if (req.mode === "navigate") {
    return w.fetch(req).catch(async (e) => {
      const hit = await fromKept(w, [key, scope.href]);
      if (!hit) throw e;
      offline.add(resultingClientId);
      return hit;
    });
  }
  if (offline.has(clientId)) {
    return fromKept(w, [key]).then((hit) => hit ?? w.fetch(req));
  }
  return w.fetch(req);
}

// keepLaunch keeps urls, the files a launch that's running used, as the
// set an offline launch gets. If they're unchanged since the kept set
// (each one's Last-Modified or ETag), nothing is written: the wasm is
// 15 MB. Otherwise a new set is written whole and then made current; if
// any file can't be fetched, the last whole set stays. It reports
// whether the kept set now matches urls.
export async function keepLaunch(w: Worker, urls: string[]): Promise<boolean> {
  try {
    const got: [string, Response, string][] = [];
    for (const u of urls) {
      const res = await w.fetch(u); // mostly a 304 from the browser's cache
      if (res.status !== 200 || res.redirected) return false;
      got.push([u, res, await stamp(res)]);
    }
    const now = await kept(w);
    const stamps = Object.fromEntries(got.map(([u, , st]) => [u, st]));
    if (now && sameStamps(now.stamps, stamps)) {
      for (const [, res] of got) await res.body?.cancel();
      return true;
    }
    const name = `kiln-${Date.now()}`; // str:ok cache name
    const cache = await w.caches.open(name);
    for (const [u, res] of got) await cache.put(u, res);
    await (await w.caches.open(metaCache)).put(metaKey, new Response(JSON.stringify({ name, stamps } satisfies Kept)));
    await cleanCaches(w);
    return true;
  } catch (e) {
    console.error("keep", e); // str:ok log tag
    return false;
  }
}

// stamp names res's version: its Last-Modified or ETag, or, for a file
// made per request (the preset's manifest), its contents.
async function stamp(res: Response): Promise<string> {
  const h = res.headers.get("last-modified") ?? res.headers.get("etag"); // str:ok header names
  if (h !== null) return h;
  const d = await crypto.subtle.digest("SHA-256", await res.clone().arrayBuffer()); // str:ok algorithm
  return Array.from(new Uint8Array(d), (b) => b.toString(16).padStart(2, "0")).join("");
}

function sameStamps(a: Record<string, string>, b: Record<string, string>): boolean {
  const ka = Object.keys(a), kb = Object.keys(b);
  return ka.length === kb.length && kb.every((k) => a[k] === b[k]);
}

// cleanCaches drops Kiln's caches but the meta cache and the kept set.
// Other apps on the same site keep theirs.
export async function cleanCaches(w: Worker): Promise<void> {
  const k = await kept(w);
  for (const n of await w.caches.keys()) if (ownsCache(n) && n !== metaCache && n !== k?.name) await w.caches.delete(n);
}

// onPush shows the push, unless a Kiln window is focused (a network blip
// on a tab the player is looking at): browsers let a push show nothing
// then. One tag: a newer one replaces it, never stacks, and still
// alerts, since it's a new absence.
export async function onPush(w: Worker): Promise<void> {
  const wins = await w.clients.matchAll({ type: "window", includeUncontrolled: true });
  if (wins.some((c) => c.focused)) return;
  return w.registration.showNotification(str.webPushTitle(), { body: str.webPushBody(), tag: awayTag, renotify: true });
}

// onClick brings Kiln forward, opening it if no window has it.
export async function onClick(w: Worker, e: { notification: { close(): void } }): Promise<void> {
  e.notification.close();
  const [win] = await w.clients.matchAll({ type: "window", includeUncontrolled: true });
  if (win) await win.focus();
  else await w.clients.openWindow(w.registration.scope);
}

interface Waiting {
  waitUntil(p: Promise<unknown>): void;
}

const g = globalThis as any;
if (typeof g.ServiceWorkerGlobalScope === "function" && g instanceof g.ServiceWorkerGlobalScope) {
  g.addEventListener("install", () => g.skipWaiting());
  g.addEventListener("activate", (e: Waiting) => e.waitUntil(Promise.all([g.clients.claim(), cleanCaches(g)])));
  g.addEventListener("fetch", (e: { request: Req; clientId: string; resultingClientId: string; respondWith(r: Promise<Response>): void }) => {
    const r = onFetch(g, e.request, e.clientId, e.resultingClientId);
    if (r) e.respondWith(r);
  });
  // The page says it's running, with the files it used (offline.ts).
  g.addEventListener("message", (e: Waiting & { data: { t?: string; files?: string[] } }) => {
    if (e.data?.t === "launched" && Array.isArray(e.data.files)) e.waitUntil(keepLaunch(g, e.data.files)); // str:ok message type
  });
  g.addEventListener("push", (e: Waiting) => e.waitUntil(onPush(g)));
  g.addEventListener("notificationclick", (e: Waiting & { notification: Notification }) => e.waitUntil(onClick(g, e)));
}
