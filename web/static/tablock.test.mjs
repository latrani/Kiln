import { test } from "node:test";
import assert from "node:assert/strict";
import { createTabLock } from "./tablock.mjs";

// fakeLocks implements the part of the Web Locks API tablock uses:
// one exclusive lock name, ifAvailable, steal and signal.
function fakeLocks() {
  let holder = null;
  const queue = [];
  function grant(entry) {
    holder = entry;
    const done = Promise.resolve(entry.cb({ name: "kiln" }));
    done.then((v) => { if (holder === entry) { holder = null; next(); } entry.ok(v); }, entry.fail);
  }
  function next() { const e = queue.shift(); if (e) grant(e); }
  return {
    request(name, opts, cb) {
      return new Promise((ok, fail) => {
        const entry = { cb, ok, fail };
        entry.cb = cb;
        if (opts.steal) {
          if (holder) { const h = holder; holder = null; h.fail(Object.assign(new Error("stolen"), { name: "AbortError" })); }
          return grant(entry);
        }
        if (!holder) return grant(entry);
        if (opts.ifAvailable) return Promise.resolve(cb(null)).then(ok, fail);
        queue.push(entry);
        opts.signal?.addEventListener("abort", () => {
          const i = queue.indexOf(entry);
          if (i >= 0) { queue.splice(i, 1); fail(Object.assign(new Error("aborted"), { name: "AbortError" })); }
        });
      });
    },
  };
}

// bus is a BroadcastChannel stand-in: a message reaches every other open
// channel, asynchronously. As with BroadcastChannel, closing a channel
// after posting doesn't take back what it posted.
function bus() {
  const chans = new Set();
  return () => {
    const ch = {
      onmessage: null, closed: false,
      postMessage(data) { for (const c of chans) if (c !== ch) setTimeout(() => c.onmessage?.({ data })); },
      close() { ch.closed = true; chans.delete(ch); },
    };
    chans.add(ch);
    return ch;
  };
}

function manualTimer() {
  const t = { fn: null, setTimer: (fn) => { t.fn = fn; return 1; }, clearTimer: () => { t.fn = null; } };
  return t;
}

const tick = () => new Promise((r) => setTimeout(r, 5));

test("the first tab gets the lock, the second doesn't", async () => {
  const locks = fakeLocks(), open = bus();
  const a = createTabLock({ locks, openChannel: open, onLost() {} });
  const b = createTabLock({ locks, openChannel: open, onLost() {} });
  assert.equal(await a.tryTake(), true);
  assert.equal(await b.tryTake(), false);
});

test("use here: the holder flushes, then lets go, and the new tab holds it", async () => {
  const locks = fakeLocks(), open = bus();
  const steps = [];
  const a = createTabLock({ locks, openChannel: open, onLost() {} });
  const b = createTabLock({ locks, openChannel: open, onLost() {} });
  await a.tryTake();
  a.serve(async () => { steps.push("flush"); }, () => steps.push("reload"));
  await b.tryTake();
  await b.takeOver();
  assert.deepEqual(steps, ["flush", "reload"]);
  const c = createTabLock({ locks, openChannel: open, onLost() {} });
  assert.equal(await c.tryTake(), false); // b holds it now
});

test("a frozen holder gets the lock stolen after the timeout, and hears it lost", async () => {
  const locks = fakeLocks(), open = bus();
  let lost = false;
  const a = createTabLock({ locks, openChannel: open, onLost: () => { lost = true; } });
  await a.tryTake(); // never serves: frozen
  const t = manualTimer();
  const b = createTabLock({ locks, openChannel: open, onLost() {}, setTimer: t.setTimer, clearTimer: t.clearTimer });
  const took = b.takeOver();
  await tick();
  assert.ok(t.fn, "steal timer armed");
  t.fn();
  await took;
  await tick();
  assert.equal(lost, true);
});

test("a tab that was stolen from doesn't flush when a late release arrives", async () => {
  const locks = fakeLocks();
  let ch = null, flushed = 0, lost = false;
  const a = createTabLock({ locks, openChannel: () => (ch = { onmessage: null, postMessage() {} }), onLost: () => { lost = true; } });
  await a.tryTake();
  a.serve(async () => { flushed++; }, () => {});
  const t = manualTimer();
  const b = createTabLock({ locks, openChannel: bus(), onLost() {}, setTimer: t.setTimer, clearTimer: t.clearTimer });
  const took = b.takeOver();
  await tick();
  t.fn();
  await took;
  await tick();
  assert.equal(lost, true);
  await ch.onmessage({ data: "release" });
  assert.equal(flushed, 0);
});

test("two release messages flush once", async () => {
  const locks = fakeLocks();
  let ch = null, flushed = 0, reloaded = 0;
  const a = createTabLock({ locks, openChannel: () => (ch = { onmessage: null, postMessage() {} }), onLost() {} });
  await a.tryTake();
  a.serve(async () => { flushed++; }, () => { reloaded++; });
  await Promise.all([ch.onmessage({ data: "release" }), ch.onmessage({ data: "release" })]);
  assert.equal(flushed, 1);
  assert.equal(reloaded, 1);
});

test("a tab that got Kiln through use here hears it lost when someone steals it", async () => {
  const locks = fakeLocks(), open = bus();
  let lost = false;
  const a = createTabLock({ locks, openChannel: open, onLost() {} });
  const b = createTabLock({ locks, openChannel: open, onLost: () => { lost = true; } });
  await a.tryTake();
  a.serve(async () => {}, () => {});
  await b.takeOver(); // granted the normal way, no steal
  const t = manualTimer();
  const c = createTabLock({ locks, openChannel: open, onLost() {}, setTimer: t.setTimer, clearTimer: t.clearTimer });
  const took = c.takeOver();
  await tick();
  t.fn();
  await took;
  await tick();
  assert.equal(lost, true);
});

test("a holder whose save throws still lets go, without the steal", async () => {
  const locks = fakeLocks(), open = bus();
  const errors = [];
  let reloaded = 0;
  const a = createTabLock({ locks, openChannel: open, onLost() {}, onError: (e) => errors.push(e) });
  await a.tryTake();
  a.serve(async () => { throw new Error("quota"); }, () => { reloaded++; });
  const t = manualTimer();
  const b = createTabLock({ locks, openChannel: open, onLost() {}, setTimer: t.setTimer, clearTimer: t.clearTimer });
  await b.takeOver(); // t.fn never runs: no steal
  assert.equal(reloaded, 1);
  assert.equal(errors.length, 1);
});

test("takeOver closes the channel it asks on", async () => {
  const locks = fakeLocks(), open = bus();
  const opened = [];
  const a = createTabLock({ locks, openChannel: open, onLost() {} });
  await a.tryTake();
  a.serve(async () => {}, () => {});
  const b = createTabLock({ locks, openChannel: () => { const ch = open(); opened.push(ch); return ch; }, onLost() {} });
  await b.takeOver();
  assert.equal(opened.length, 1);
  assert.equal(opened[0].closed, true);
});
