// persist: saves memfs to IndexedDB so Kiln's files outlast the tab, and
// loads them back before Go starts. memfs says which paths changed; a
// flush writes each one's current state (or deletes its record if it's
// gone) in one transaction. Saving is best effort: browsers can clear
// IndexedDB, so world defs also have Back up (see kiln-web).

const DB = "kiln";
const STORE = "files";

// idbStore adapts an IDBFactory (globalThis.indexedDB) to the three calls
// createPersist needs.
export function idbStore(idb) {
  const req = (r) => new Promise((ok, fail) => { r.onsuccess = () => ok(r.result); r.onerror = () => fail(r.error); });
  let db;
  return {
    async open() {
      const r = idb.open(DB, 1);
      r.onupgradeneeded = () => r.result.createObjectStore(STORE);
      db = await req(r);
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

const depth = (p) => p.split("/").length;
const skipped = (p) => p === "/tmp" || p.startsWith("/tmp/");

// createPersist saves fs's changed paths to store. onError hears about the
// first failed flush only; failed paths stay changed and go out next time.
export function createPersist({ store, fs, onError = () => {}, delay = 500, setTimer = setTimeout, clearTimer = clearTimeout }) {
  const dirty = new Set();
  let timer = null, chain = Promise.resolve(), failed = false, stopped = false;

  async function run() {
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
    async load() {
      const recs = await store.all();
      recs.sort(([a], [b]) => depth(a) - depth(b));
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
      if (timer !== null) {
        clearTimer(timer);
        timer = null;
        chain = chain.then(run);
      }
      return chain;
    },
    // stop ends saving for good: another tab owns Kiln now.
    stop() { stopped = true; clearTimer(timer); },
  };
  return persist;
}
