// The service worker. It shows the relay's pushes (push.ts) and opens
// Kiln when one's clicked, and keeps the page's files for a launch with
// no network. Online, the network always answers (the relay serves the
// files no-cache, so an unchanged one is a cheap 304), so a new build
// is never kept waiting behind an old one.
import { awayTag, cacheName } from "./push.ts";

export { cacheName };
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
  fetch(r: Req): Promise<Response>;
}

// Req is the parts of a fetch event's request used here.
export interface Req {
  url: string;
  method: string;
  mode: string;
}

// onFetch answers req from the network, keeping what it gets, or from
// what's kept when the network fails. undefined: not the page's own file
// (the relay, the preset, another site, not a GET), so the browser
// fetches it as if there were no worker.
export function onFetch(w: Worker, req: Req): Promise<Response> | undefined {
  const scope = new URL(w.registration.scope);
  const url = new URL(req.url);
  if (req.method !== "GET" || url.origin !== scope.origin || !url.pathname.startsWith(scope.pathname)) return undefined;
  const rel = url.pathname.slice(scope.pathname.length);
  if (rel === "relay" || rel.startsWith("preset/")) return undefined; // str:ok paths
  const key = url.origin + url.pathname; // ?record and the like don't make another file
  return (async () => {
    try {
      const res = await w.fetch(req);
      if (res.status === 200 && res.type !== "opaque") keep(w, key, res.clone()).catch((e) => console.error("cache", e)); // str:ok log tag
      return res;
    } catch (e) {
      const cache = await w.caches.open(cacheName);
      // Offline, a launch at any of the page's addresses gets the page.
      const keys = req.mode === "navigate" ? [key, scope.href, scope.href + "index.html"] : [key]; // str:ok file name
      for (const k of keys) {
        const hit = await cache.match(k);
        if (hit) return hit;
      }
      throw e;
    }
  })();
}

// keep stores res as key unless what's kept is the same file (its ETag
// or Last-Modified): the wasm is 15 MB, too much to rewrite every launch.
async function keep(w: Worker, key: string, res: Response): Promise<void> {
  const cache = await w.caches.open(cacheName);
  const stamp = (r: Response) => r.headers.get("etag") ?? r.headers.get("last-modified"); // str:ok header names
  const old = await cache.match(key);
  if (old && stamp(res) !== null && stamp(old) === stamp(res)) return;
  await cache.put(key, res);
}

// cleanCaches drops caches by any other name.
export async function cleanCaches(w: Worker): Promise<void> {
  for (const k of await w.caches.keys()) if (k !== cacheName) await w.caches.delete(k);
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
  g.addEventListener("fetch", (e: { request: Req; respondWith(r: Promise<Response>): void }) => {
    const r = onFetch(g, e.request);
    if (r) e.respondWith(r);
  });
  g.addEventListener("push", (e: Waiting) => e.waitUntil(onPush(g)));
  g.addEventListener("notificationclick", (e: Waiting & { notification: Notification }) => e.waitUntil(onClick(g, e)));
}
