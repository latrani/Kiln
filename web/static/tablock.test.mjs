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
// channel, asynchronously.
function bus() {
  const chans = [];
  return () => {
    const ch = { onmessage: null, postMessage(data) { for (const c of chans) if (c !== ch) setTimeout(() => c.onmessage?.({ data })); } };
    chans.push(ch);
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
