// Web Push: while the relay holds a character's connection (the page is
// gone or asleep), it can push once when the world says something, and
// the service worker (sw.ts) shows it. The page subscribes with the
// relay's key once notifications are allowed, and hands Go the
// subscription's endpoint (kiln.push), which goes to the relay with each
// character whose notify isn't "none". The push carries nothing, so the
// subscription's encryption keys never leave the browser.

export interface Subscriber {
  keyURL: URL; // the relay's ?push_key=1
  sw: ServiceWorkerContainer | undefined;
  send: (endpoint: string) => void;
  fetch?: typeof fetch;
  log?: (...a: unknown[]) => void;
}

// subscribe subscribes this browser to the relay's pushes, if the relay
// sends them and the browser can, and sends the endpoint on. It never
// throws: without push the page works as before.
export async function subscribe(o: Subscriber): Promise<void> {
  const log = o.log ?? console.error;
  try {
    if (!o.sw) return;
    const res = await (o.fetch ?? fetch)(o.keyURL);
    if (!res.ok) return; // the relay sends no pushes
    const key = fromBase64URL((await res.text()).trim());
    await o.sw.register("sw.js"); // str:ok file name
    const pm = (await o.sw.ready).pushManager;
    if (!pm) return;
    let sub = await pm.getSubscription();
    if (sub && !same(sub.options.applicationServerKey, key)) {
      await sub.unsubscribe(); // the relay's key changed: that subscription is dead
      sub = null;
    }
    sub ??= await pm.subscribe({ userVisibleOnly: true, applicationServerKey: key });
    o.send(sub.endpoint);
  } catch (e) {
    log("push", e); // str:ok log tag
  }
}

// unsubscribe ends this browser's subscription: /reset.
export async function unsubscribe(sw: ServiceWorkerContainer | undefined): Promise<void> {
  try {
    const sub = await (await sw?.getRegistration())?.pushManager?.getSubscription();
    await sub?.unsubscribe();
  } catch (e) {
    console.error("push", e); // str:ok log tag
  }
}

function fromBase64URL(s: string): Uint8Array<ArrayBuffer> {
  const b = atob(s.replace(/-/g, "+").replace(/_/g, "/"));
  return Uint8Array.from(b, (c) => c.charCodeAt(0));
}

function same(a: ArrayBuffer | null | undefined, b: Uint8Array): boolean {
  if (!a || a.byteLength !== b.byteLength) return false;
  const x = new Uint8Array(a);
  return x.every((v, i) => v === b[i]);
}
