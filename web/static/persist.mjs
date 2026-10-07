// persist: saves memfs to IndexedDB so Kiln's files outlast the tab, and
// loads them back before Go starts. memfs says which paths changed; a
// flush writes each one's current state (or deletes its record if it's
// gone) in one transaction. Saving is best effort: browsers can clear
// IndexedDB, so world defs also have Back up (see web/cmd/kiln-web).

const DB = "kiln";
const STORE = "files";

// idbStore adapts an IDBFactory (globalThis.indexedDB) to the three calls
// createPersist needs.
export function idbStore(idb, { setTimer = setTimeout, clearTimer = clearTimeout, openTimeout = 5000 } = {}) {
  const req = (r) => new Promise((ok, fail) => { r.onsuccess = () => ok(r.result); r.onerror = () => fail(r.error); });
  let db;
  return {
    // open rejects if the browser neither opens nor fails within
    // openTimeout (a blocked upgrade can hang forever). A db that opens
    // after open gave up is closed, so it can't block a later upgrade.
    async open() {
      const r = idb.open(DB, 1);
      r.onupgradeneeded = () => r.result.createObjectStore(STORE);
      let timer;
      try {
        db = await Promise.race([
          new Promise((ok, fail) => {
            r.onsuccess = () => ok(r.result);
            r.onerror = () => fail(r.error);
            r.onblocked = () => fail(new Error("indexedDB open blocked"));
          }),
          new Promise((_, fail) => { timer = setTimer(() => fail(new Error("indexedDB open timed out")), openTimeout); }),
        ]);
      } catch (e) {
        r.onsuccess = () => r.result.close();
        throw e;
      } finally {
        clearTimer(timer);
      }
    },
    async all() {
      const s = db.transaction(STORE, "readonly").objectStore(STORE);
      const [keys, vals] = await Promise.all([req(s.getAllKeys()), req(s.getAll())]);
      return keys.map((k, i) => [k, vals[i]]);
    },
    apply(puts, dels) {
      return new Promise((ok, fail) => {
        const tx = db.transaction(STORE, "readwrite");
        const s = tx.objectStore(STORE);
        for (const [k, v] of puts) s.put(v, k);
        for (const k of dels) s.delete(k);
        tx.oncomplete = () => ok();
        tx.onerror = tx.onabort = () => fail(tx.error);
      });
    },
  };
}

const skipped = (p) => p === "/tmp" || p.startsWith("/tmp/");

// createPersist saves fs's changed paths to store. onError hears about the
// first failed flush only; failed paths stay changed and go out next time.
export function createPersist({ store, fs, onError = () => {}, delay = 500, setTimer = setTimeout, clearTimer = clearTimeout }) {
  const dirty = new Set();
  let timer = null, chain = Promise.resolve(), failed = false, stopped = false, queued = false;

  async function run() {
    queued = false;
    if (stopped || dirty.size === 0) return;
    const paths = [...dirty];
    dirty.clear();
    const puts = [], dels = [];
    for (const p of paths) {
      const rec = fs.entry(p);
      if (rec) puts.push([p, rec]); else dels.push(p);
    }
    try {
      await store.apply(puts, dels);
    } catch (e) {
      for (const p of paths) dirty.add(p);
      if (!failed) { failed = true; onError(e); }
    }
  }

  const persist = {
    // load puts every record back. Order doesn't matter: put makes a
    // file's parents, and a dir's record then sets the mode on the dir.
    async load() {
      const recs = await store.all();
      for (const [p, rec] of recs) fs.put(p, rec);
    },
    changed(path) {
      if (stopped || skipped(path)) return;
      dirty.add(path);
      clearTimer(timer);
      timer = setTimer(persist.flush, delay);
    },
    // flush saves now; flushes run one at a time.
    flush() {
      clearTimer(timer);
      timer = null;
      if (!queued) {
        queued = true;
        chain = chain.then(run);
      }
      return chain;
    },
    // handOff saves everything, including changes that land while it
    // saves, then stops: this tab is giving Kiln to another one. It tries a
    // few times, so a store that keeps failing can't hold the handoff up.
    async handOff() {
      for (let i = 0; i < 3; i++) {
        await persist.flush(); // waits out a flush already running, too
        if (dirty.size === 0) break;
      }
      persist.stop();
    },
    // stop ends saving for good: another tab owns Kiln now.
    stop() { stopped = true; clearTimer(timer); },
  };
  return persist;
}
