// The service worker, for Web Push only (push.ts): it shows the relay's
// push and opens Kiln when it's clicked. It caches nothing.
import * as str from "./str.gen.ts";

// Worker is the parts of the worker's global scope used here (tests pass
// their own).
export interface Worker {
  registration: { scope: string; showNotification(title: string, opts: NotificationOptions): Promise<void> };
  clients: {
    matchAll(o: { type: "window"; includeUncontrolled: boolean }): Promise<readonly { focus(): Promise<unknown> }[]>;
    openWindow(url: string): Promise<unknown>;
  };
}

// onPush shows the push. One tag: a newer one replaces it, never stacks.
export function onPush(w: Worker): Promise<void> {
  return w.registration.showNotification(str.webPushTitle(), { body: str.webPushBody(), tag: "kiln-away" }); // str:ok notification tag
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
