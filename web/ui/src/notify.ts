// Desktop notifications (the Notify effect). The browser's Notification
// API is here; the host (host.ts) says what it allows and asks for it. A
// page may only ask in answer to a click, so the status line offers a
// button while notifications won't show;
// after one click, allowed or not, it doesn't offer again.
import type { Event, Store } from "./store.ts";

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

// request asks the browser for permission if it hasn't answered, and
// gives its answer.
export async function request(n: API | undefined = browser()): Promise<Permission> {
  const p = permission(n);
  return p === "default" ? await n!.requestPermission() : p;
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
// host won't show them yet and the offer hasn't been taken or turned
// down.
export async function offer(store: Store, asked: boolean = wasAsked()): Promise<boolean> {
  return !asked && (await store.host.permission()) !== "granted";
}

// dismiss turns the offer down for good in this browser.
export function dismiss(store: Store): void {
  setAsked();
  store.set({ notifyOffer: false });
}

// ask is the offer's click: the host's prompt if it hasn't answered,
// then a status line saying why it won't show them if it won't.
export async function ask(store: Store): Promise<void> {
  dismiss(store);
  const why = store.host.whyNoNotify(await store.host.askPermission());
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
