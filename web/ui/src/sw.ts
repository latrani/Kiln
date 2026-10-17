// The service worker, for Web Push only (push.ts): it shows the relay's
// push and opens Kiln when it's clicked. It caches nothing.
import { awayTag } from "./push.ts";
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
  g.addEventListener("activate", (e: Waiting) => e.waitUntil(g.clients.claim()));
  g.addEventListener("push", (e: Waiting) => e.waitUntil(onPush(g)));
  g.addEventListener("notificationclick", (e: Waiting & { notification: Notification }) => e.waitUntil(onClick(g, e)));
}
