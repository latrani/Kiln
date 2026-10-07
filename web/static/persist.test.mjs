import { test } from "node:test";
import assert from "node:assert/strict";
import { createFS } from "./memfs.mjs";
import { createPersist, idbStore } from "./persist.mjs";

const enc = (s) => new TextEncoder().encode(s);
const dec = (b) => new TextDecoder().decode(b);

// fakeStore keeps records in a Map; fail makes the next apply reject, and
// during runs while an apply is in flight.
function fakeStore(initial = []) {
  const recs = new Map(initial);
  const s = {
    recs, applies: 0, fail: false, during: null,
    async all() { return [...recs]; },
    async apply(puts, dels) {
      s.applies++;
      const d = s.during;
      s.during = null;
      d?.();
      await null;
      if (s.fail) { s.fail = false; throw new Error("quota"); }
      for (const [k, v] of puts) recs.set(k, v);
      for (const k of dels) recs.delete(k);
    },
  };
  return s;
}

// manual timers: fire() runs whatever is pending.
function timers() {
  let pending = null;
  return {
    setTimer: (fn) => { pending = fn; return 1; },
    clearTimer: () => { pending = null; },
    fire: async (p) => { const fn = pending; pending = null; fn?.(); await p.flush(); },
    get armed() { return pending !== null; },
  };
}

function setup(initial) {
  const store = fakeStore(initial);
  const t = timers();
  const errors = [];
  let p;
  const fs = createFS({ onChange: (path) => p.changed(path), onSync: () => p.flush() });
  p = createPersist({ store, fs, onError: (e) => errors.push(e), setTimer: t.setTimer, clearTimer: t.clearTimer });
  return { store, t, errors, fs, p };
}

function write(fs, path, s) {
  fs.mkdirAll(path.split("/").slice(0, -1).join("/"));
  let fd;
  fs.open(path, 0o1101, 0o644, (e, r) => { if (e) throw e; fd = r; }); // O_WRONLY|O_CREAT|O_TRUNC
  const b = enc(s);
  fs.write(fd, b, 0, b.length, null, (e) => { if (e) throw e; });
  fs.close(fd, () => {});
}

test("load puts saved records into memfs, in any order", async () => {
  const records = [
    ["/home/kiln/.config/kiln/worlds/fm.toml", { kind: "file", mode: 0o100600, mtimeMs: 7000, bytes: enc("host = 'x'") }],
    ["/home/kiln/.config/kiln", { kind: "dir", mode: 0o040700 }],
  ];
  for (const recs of [records, [...records].reverse()]) {
    const { fs, p } = setup(recs);
    await p.load();
    assert.equal(dec(fs.entry("/home/kiln/.config/kiln/worlds/fm.toml").bytes), "host = 'x'");
    assert.equal(fs.entry("/home/kiln/.config/kiln").mode, 0o040700);
  }
});

test("changes flush once, after the debounce, in one apply", async () => {
  const { store, t, fs, p } = setup();
  write(fs, "/home/kiln/a", "1");
  write(fs, "/home/kiln/b", "2");
  assert.equal(store.applies, 0);
  assert.ok(t.armed);
  await t.fire(p);
  assert.equal(store.applies, 1);
  assert.equal(dec(store.recs.get("/home/kiln/a").bytes), "1");
  assert.equal(dec(store.recs.get("/home/kiln/b").bytes), "2");
});

test("a removed path's record is deleted", async () => {
  const { store, t, fs, p } = setup();
  write(fs, "/home/kiln/a", "1");
  await t.fire(p);
  fs.unlink("/home/kiln/a", (e) => { if (e) throw e; });
  await t.fire(p);
  assert.equal(store.recs.has("/home/kiln/a"), false);
});

test("/tmp is never saved", async () => {
  const { store, t, fs, p } = setup();
  write(fs, "/tmp/x", "1");
  await p.flush();
  assert.equal(store.recs.size, 0);
  assert.equal(t.armed, false);
});

test("fsync flushes at once", async () => {
  const { store, fs, p } = setup();
  write(fs, "/home/kiln/a", "1");
  let fd;
  fs.open("/home/kiln/a", 1, 0, (e, r) => { fd = r; });
  fs.fsync(fd, () => {});
  await new Promise((r) => setTimeout(r)); // no flush() here: the fsync has to start it
  assert.equal(store.applies, 1);
  assert.equal(dec(store.recs.get("/home/kiln/a").bytes), "1");
});

test("a failed flush reports once and retries with the next one", async () => {
  const { store, t, errors, fs, p } = setup();
  store.fail = true;
  write(fs, "/home/kiln/a", "1");
  await t.fire(p);
  assert.equal(errors.length, 1);
  assert.equal(store.recs.size, 0);
  store.fail = true;
  write(fs, "/home/kiln/b", "2");
  await t.fire(p);
  assert.equal(errors.length, 1); // still once
  write(fs, "/home/kiln/c", "3");
  await t.fire(p);
  assert.deepEqual([...store.recs.keys()].sort(), ["/home/kiln/a", "/home/kiln/b", "/home/kiln/c"]);
});

test("after stop nothing is saved", async () => {
  const { store, fs, p } = setup();
  p.stop();
  write(fs, "/home/kiln/a", "1");
  await p.flush();
  assert.equal(store.applies, 0);
});

test("handOff saves changes that land while it saves, then stops", async () => {
  const { store, t, fs, p } = setup();
  write(fs, "/home/kiln/a", "1");
  store.during = () => write(fs, "/home/kiln/b", "2"); // Go writes mid-flush
  await p.handOff();
  assert.deepEqual([...store.recs.keys()].sort(), ["/home/kiln/a", "/home/kiln/b"]);
  write(fs, "/home/kiln/c", "3");
  assert.equal(t.armed, false);
  await p.flush();
  assert.equal(store.recs.has("/home/kiln/c"), false);
});

test("handOff gives up on a store that keeps failing", async () => {
  const { store, errors, fs, p } = setup();
  store.apply = async () => { store.applies++; throw new Error("quota"); };
  write(fs, "/home/kiln/a", "1");
  await p.handOff();
  assert.ok(store.applies >= 1 && store.applies <= 3, `applies = ${store.applies}`);
  assert.equal(errors.length, 1);
});

test("direct flush after failed timer flush saves failed paths", async () => {
  const { store, t, fs, p } = setup();
  store.fail = true;
  write(fs, "/home/kiln/a", "1");
  await t.fire(p);
  assert.equal(store.recs.size, 0); // failed flush saves nothing
  // no new changes, no timer armed, but direct flush() should save
  await p.flush();
  assert.equal(store.recs.size, 1);
  assert.equal(dec(store.recs.get("/home/kiln/a").bytes), "1");
});

// fakeIDB is an IDBFactory whose open request is driven by the test.
function fakeIDB() {
  const r = {};
  return { req: r, open: () => r };
}

test("idbStore.open rejects when the open never settles", async () => {
  let fire;
  const idb = fakeIDB();
  const store = idbStore(idb, { setTimer: (fn) => { fire = fn; return 1; }, clearTimer() {} });
  const opened = store.open();
  fire();
  await assert.rejects(opened, /timed out/);
});

test("idbStore.open rejects when the open is blocked", async () => {
  const idb = fakeIDB();
  const store = idbStore(idb, { setTimer: () => 1, clearTimer() {} });
  const opened = store.open();
  idb.req.onblocked();
  await assert.rejects(opened, /blocked/);
});

test("idbStore.open closes a db that opens after it gave up", async () => {
  for (const giveUp of ["timeout", "blocked"]) {
    let fire, closed = 0;
    const idb = fakeIDB();
    const store = idbStore(idb, { setTimer: (fn) => { fire = fn; return 1; }, clearTimer() {} });
    const opened = store.open();
    if (giveUp === "timeout") fire(); else idb.req.onblocked();
    await assert.rejects(opened);
    idb.req.result = { close: () => { closed++; } };
    idb.req.onsuccess();
    assert.equal(closed, 1, giveUp);
  }
});

test("idbStore.open succeeds when the open does, and stops the timer", async () => {
  let cleared = 0;
  const idb = fakeIDB();
  const store = idbStore(idb, { setTimer: () => 7, clearTimer: (t) => { if (t === 7) cleared++; } });
  const opened = store.open();
  idb.req.result = {};
  idb.req.onsuccess();
  await opened;
  assert.equal(cleared, 1);
});
