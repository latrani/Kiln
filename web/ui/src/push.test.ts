import { test } from "node:test";
import assert from "node:assert/strict";
import { clearAway, forget, subscribe } from "./push.ts";

const key = "BPtHpXmY3r9rWzQh2a6vN0KQe2o0a0d5rFv7yJkVZ3c1aJm2b8u0p0xT9vF3m4wq7XG6YfQ6M3nZ5kL1sD2hR0c";
const keyBytes = () => Uint8Array.from(Buffer.from(key, "base64url"));

// fakes is a service worker container whose push manager may hold a
// subscription for existing (a key), and a fetch answering status.
function fakes(o: { status?: number; existing?: Uint8Array; push?: boolean; failSubscribe?: boolean } = {}) {
  const calls: string[] = [];
  const sent: string[] = [];
  let sub: any = o.existing ? mkSub(o.existing) : null;
  function mkSub(k: Uint8Array) {
    return {
      endpoint: `https://fcm.googleapis.com/fcm/send/${calls.length}`,
      options: { applicationServerKey: k.buffer },
      async unsubscribe() { calls.push("unsubscribe"); sub = null; return true; },
    };
  }
  const pushManager = {
    async getSubscription() { return sub; },
    async subscribe(opts: { userVisibleOnly: boolean; applicationServerKey: Uint8Array }) {
      calls.push("subscribe");
      if (o.failSubscribe) throw new Error("denied");
      assert.equal(opts.userVisibleOnly, true);
      assert.deepEqual(new Uint8Array(opts.applicationServerKey), keyBytes());
      return (sub = mkSub(new Uint8Array(opts.applicationServerKey)));
    },
  };
  const reg = o.push === false ? {} : { pushManager };
  const sw = {
    async register(url: string) { calls.push(`register ${url}`); return reg; },
    ready: Promise.resolve(reg),
    async getRegistration() { return reg; },
  } as unknown as ServiceWorkerContainer;
  const fetch = (async (u: string | URL) => {
    calls.push(`fetch ${u}`);
    return new Response(key, { status: o.status ?? 200 });
  }) as typeof globalThis.fetch;
  const opts = { keyURL: new URL("https://fm.example/kiln/relay?push_key=1"), sw, send: (e: string) => sent.push(e), fetch, log: () => {} };
  return { calls, sent, opts, sub: () => sub };
}

test("subscribing registers the worker, subscribes with the relay's key, and hands Go the endpoint", async () => {
  const f = fakes();
  await subscribe(f.opts);
  assert.deepEqual(f.calls, ["fetch https://fm.example/kiln/relay?push_key=1", "register sw.js", "subscribe"]);
  assert.deepEqual(f.sent, [f.sub().endpoint]);
});

test("a subscription with the same key is reused", async () => {
  const f = fakes({ existing: keyBytes() });
  await subscribe(f.opts);
  assert.ok(!f.calls.includes("subscribe"));
  assert.deepEqual(f.sent, [f.sub().endpoint]);
});

test("a subscription with another key (the relay's changed) is replaced", async () => {
  const f = fakes({ existing: new Uint8Array(65).fill(4) });
  await subscribe(f.opts);
  assert.deepEqual(f.calls.slice(-2), ["unsubscribe", "subscribe"]);
  assert.deepEqual(f.sent, [f.sub().endpoint]);
});

test("a relay without a key, a browser without push, or a refused subscribe: nothing, quietly", async () => {
  for (const o of [{ status: 404 }, { push: false }, { failSubscribe: true }]) {
    const f = fakes(o);
    await subscribe(f.opts);
    assert.deepEqual(f.sent, [], JSON.stringify(o));
    if (o.status) assert.ok(!f.calls.some((c) => c.startsWith("register")), "registered with no key");
  }
  const f = fakes();
  await subscribe({ ...f.opts, sw: undefined });
  assert.deepEqual(f.sent, []);
});

test("forgetting drops the subscription, the kept page and the worker", async () => {
  const f = fakes({ existing: keyBytes() });
  const done: string[] = [];
  (await f.opts.sw!.getRegistration() as any).unregister = async () => { done.push("unregister"); return true; };
  const caches = { async keys() { return ["kiln-meta", "kiln-1", "someone-elses"]; }, async delete(n: string) { done.push(`delete ${n}`); return true; } } as unknown as CacheStorage;
  await forget(f.opts.sw, caches);
  assert.deepEqual(f.calls, ["unsubscribe"]);
  assert.deepEqual(done, ["delete kiln-meta", "delete kiln-1", "unregister"]);
  await forget(undefined, undefined);
});

test("a step of forgetting that fails doesn't stop the rest", async () => {
  const f = fakes({ existing: keyBytes() });
  const done: string[] = [];
  const reg = await f.opts.sw!.getRegistration() as any;
  reg.unregister = async () => { done.push("unregister"); return true; };
  (await reg.pushManager.getSubscription()).unsubscribe = async () => { throw new Error("push service out of reach"); };
  const caches = { async keys() { return ["kiln-1"]; }, async delete(n: string) { done.push(`delete ${n}`); return true; } } as unknown as CacheStorage;
  await forget(f.opts.sw, caches, () => {});
  assert.deepEqual(done, ["delete kiln-1", "unregister"]);
});

test("coming back clears the away notification, and only that one", async () => {
  const closed: string[] = [];
  const note = (tag: string) => ({ tag, close: () => closed.push(tag) });
  const reg = { async getNotifications(o?: { tag?: string }) { return [note("kiln-away"), note("other")].filter((n) => !o?.tag || n.tag === o.tag); } };
  await clearAway({ async getRegistration() { return reg; } } as unknown as ServiceWorkerContainer);
  assert.deepEqual(closed, ["kiln-away"]);
  await clearAway(undefined);
  await clearAway({ async getRegistration() { return undefined; } } as unknown as ServiceWorkerContainer);
});
