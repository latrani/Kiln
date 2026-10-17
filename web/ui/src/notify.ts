// Desktop notifications (the Notify effect) through the browser's
// Notification API. A page may only ask for them in answer to a click,
// so the status line offers a button while the browser won't show them;
// after one click, allowed or not, it doesn't offer again.
import type { Event, Store } from "./store.ts";
import * as str from "./str.gen.ts";

// Permission is the browser's answer so far, or "unsupported" without
// the API (Safari on iOS outside a Home Screen app).
export type Permission = NotificationPermission | "unsupported";

// Shown is a notification on screen; API is the Notification
// constructor's parts used here (tests pass their own).
export interface Shown {
  onclick: ((e: globalThis.Event) => void) | null;
  close(): void;
}
export interface API {
  readonly permission: NotificationPermission;
  requestPermission(): Promise<NotificationPermission>;
  new (title: string, opts?: { body?: string }): Shown;
}

const browser = (): API | undefined => (globalThis as any).Notification;

export function permission(n: API | undefined = browser()): Permission {
  return n ? n.permission : "unsupported";
}

// asked is in localStorage: the button was clicked or dismissed. Best
// effort: without storage it offers again next time.
const askedKey = "kiln.notify.asked"; // str:ok
function wasAsked(): boolean {
  try {
    return globalThis.localStorage?.getItem(askedKey) === "1";
  } catch {
    return false;
  }
}
function setAsked(): void {
  try {
    globalThis.localStorage?.setItem(askedKey, "1");
  } catch {}
}
// forgetAsked offers again, as on a first visit: /reset.
export function forgetAsked(): void {
  try {
    globalThis.localStorage?.removeItem(askedKey);
  } catch {}
}

// offer is whether the status line offers to allow notifications: the
// browser won't show them yet and the offer hasn't been taken or turned
// down.
export function offer(p: Permission = permission(), asked: boolean = wasAsked()): boolean {
  return p !== "granted" && !asked;
}

// dismiss turns the offer down for good in this browser.
export function dismiss(store: Store): void {
  setAsked();
  store.set({ notifyOffer: false });
}

// ask is the offer's click: the browser's prompt if it hasn't answered,
// then pushes if it allowed them, or a status line saying why it won't
// show them.
export async function ask(store: Store, n: API | undefined = browser()): Promise<void> {
  dismiss(store);
  let p = permission(n);
  if (p === "default") p = await n!.requestPermission();
  if (p === "granted") store.state.subscribe?.();
  const why = p === "denied" ? str.webNotifyDenied() : p === "unsupported" ? str.webNotifyUnsupported() : "";
  if (why) store.state.act?.({ Do: "say", Text: why, Err: false });
}

// show shows Notify e if the browser allows it. Clicking it brings the
// tab forward and switches to the character it's about.
export function show(store: Store, e: Event, n: API | undefined = browser(), focus: () => void = () => globalThis.focus?.()): void {
  if (permission(n) !== "granted") return;
  let note: Shown;
  try {
    note = new n!(String(e.Title ?? ""), { body: String(e.Body ?? "") });
  } catch (err) {
    // Chrome on Android only shows them from a service worker.
    console.error("notify", err);
    return;
  }
  note.onclick = () => {
    focus();
    if (e.Key) store.state.act?.({ Do: "switch", Key: e.Key });
    note.close();
  };
}
