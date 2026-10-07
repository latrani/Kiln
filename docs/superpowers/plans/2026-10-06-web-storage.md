# Web Storage Across Visits Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The web build keeps its files (config, world defs, pins, logs) across visits in IndexedDB, lets one tab own Kiln at a time, turns browse-mode save into a download, and adds Back up / Restore rows to the sidebar.

**Architecture:** memfs reports which paths change. `persist.mjs` writes those paths to IndexedDB after a short debounce and loads them back before Go starts. `tablock.mjs` uses Web Locks plus a BroadcastChannel so one tab holds Kiln. On the Go side, web-only behavior is a set of nil-on-desktop hooks on `ui.Deps` (`SaveFile`, `Backup`, `Restore`), wired by `kiln-web`, with zip packing in a new `web/backup` package.

**Tech Stack:** Go (Bubble Tea v2, `archive/zip`, `syscall/js`), plain ES modules, `node --test`, IndexedDB, Web Locks, BroadcastChannel.

**Spec:** `docs/superpowers/specs/2026-10-06-web-storage-design.md`

## Global Constraints

- Every string a person reads lives in `internal/str/locales/en.toml`; run `go generate ./internal/str` after editing it. Commits touching `internal/str/locales/` end with a `Strings:` trailer listing keys.
- Tests build expected text from the catalog (`str.X()`), never a copy.
- Colors come from theme roles. The new role is `sidebar.action`: add it to `internal/theme/roles.go` (const + `Roles`), `internal/theme/default.toml`, and the role list in `docs/themes.md`.
- No emoji in the UI; glyphs are single-cell. `↓` and `↑` are fine.
- No npm dependencies. JS tests are `node --test` with hand-written fakes.
- Desktop Kiln is unchanged: no existing golden screen in `internal/ui/testdata/golden` changes.
- Paths under `/tmp` are never saved.
- Preset files are seeded only when the path doesn't exist yet.
- Restore never deletes files and checks the whole zip before writing: only `config/…` and `known_hosts`, no `..`, no absolute paths, at most 10 MB unpacked.
- Commits end with:
  ```
  Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_015FYvSfmNHiKj5FDkQYMBGX
  ```
- Work on branch `web-storage`. Open a PR at the end; never merge it.

## Review Focus

- **A file written through an fd after its path was renamed** (Go's atomic write: write a temp file, rename over the real one). The saved record must land at the new path, and the temp path's record must be deleted. Test in Task 2.
- **A flush that fails, then a later change.** The failed paths must go out with the next flush, and the error is reported once, not on every retry. Test in Task 3.
- **The holder tab is frozen and never answers `release`.** Use here must still get the lock (steal after 3 s), and the tab that lost the lock must stop saving. Test in Task 5.
- **Restore with a zip that has `config/../../etc/x`, an absolute path, or a directory entry.** Reject the first two before writing anything; skip directory entries. Test in Task 8.
- **Browse save with a name like `~/scenes/x.txt` on the web.** It downloads `x.txt` and never touches `export_dir`. Test in Task 7.

---

## File Structure

| File | Does |
|---|---|
| `web/static/memfs.mjs` (modify) | `onChange`/`onSync` hooks; fds remember their path; page helpers `entry`, `put`, `exists` |
| `web/static/persist.mjs` (new) | `idbStore(idb)`: thin IndexedDB adapter. `createPersist({store, fs, …})`: dirty set, debounce, flush, load |
| `web/static/tablock.mjs` (new) | `createTabLock({locks, openChannel, …})`: try/serve/takeOver |
| `web/static/kiln.js` (modify) | boot order, lock screen, preset seed-if-missing, `kilnDownload`, `kilnPickFile` |
| `web/static/index.html` (modify) | lock screen markup |
| `web/static/strings.json` (new, generated) | the catalog's `[web]` table for the page |
| `internal/str/catalog/generate.go` (modify) | `WebJSON` |
| `internal/str/gen/main.go` (modify) | writes `web/static/strings.json` |
| `internal/ui/model.go` (modify) | `Deps.SaveFile/Backup/Restore`, `StatusMsg`, `/backup` `/restore`, footer clicks |
| `internal/ui/sidebar.go` (modify) | footer rows |
| `internal/ui/view.go` (modify) | draw footer rows |
| `internal/ui/browse.go` (modify) | save → `saveFile` hook |
| `internal/theme/roles.go`, `default.toml`, `docs/themes.md` | `sidebar.action` |
| `web/backup/backup.go` (new) | `Zip`, `Unzip` |
| `web/cmd/kiln-web/main.go` (modify), `web/cmd/kiln-web/page.go` (new) | wiring: hooks, storage status, JS calls |
| `docs/web-dev.md` (modify) | what's saved now, Back up/Restore, the lock |

---

### Task 1: Check that a file picker opens after the Go round trip

This is a throwaway check that decides how Restore asks for a file. Nothing from it gets committed.

**Files:**
- Create (scratch, not committed): `$SCRATCH/picker-check.html`, where `$SCRATCH` is the session scratchpad.

- [ ] **Step 1: Write the check page**

The real path is xterm.js `mousedown` → `term.onData` → `kiln.input` → Go goroutine → `tea.Cmd` goroutine → `kilnPickFile()`. That's asynchronous but well under 100 ms. The page mimics it with a 150 ms delay and a promise hop:

```html
<!doctype html>
<meta charset="utf-8">
<p>Press the mouse on the box. A file picker should open about 150 ms later.</p>
<div id="box" style="width:200px;height:100px;background:#8cf"></div>
<pre id="out"></pre>
<script>
const out = document.getElementById("out");
document.getElementById("box").addEventListener("mousedown", () => {
  setTimeout(() => Promise.resolve().then(() => {
    const input = Object.assign(document.createElement("input"), { type: "file", accept: ".zip" });
    input.style.display = "none";
    document.body.append(input);
    input.onchange = () => { out.textContent = "picked " + input.files[0]?.name; input.remove(); };
    input.oncancel = () => { out.textContent = "cancelled"; input.remove(); };
    input.click();
    out.textContent = "click() called; activation " + navigator.userActivation?.isActive;
  }), 150);
});
</script>
```

- [ ] **Step 2: Run it in Chrome and ask Indi to run it in Safari**

Serve it with `python3 -m http.server -d "$SCRATCH" 8099` (run in the background), open `http://localhost:8099/picker-check.html` in Chrome, and press on the box. Ask Indi to do the same in Safari and report whether the picker opened and whether cancelling printed "cancelled".

- [ ] **Step 3: Record the result**

- **Picker opens in both:** Task 9's `kilnPickFile` stays as written.
- **It fails in Safari:** add drop-to-restore in Task 9. The `kilnPickFile` promise also resolves when a `.zip` is dropped on the page: `addEventListener("drop", …)` with `preventDefault` on `dragover`. And the restore hint status tells the user to drop the file: a new `status.restore_drop = "drop a Kiln backup onto the page"` entry, shown before waiting.

Write the result as one line in the PR description. Stop the http server.

---

### Task 2: memfs reports changes

**Files:**
- Modify: `web/static/memfs.mjs`
- Test: `web/static/memfs.test.mjs`

**Interfaces:**
- Produces: `createFS({ stdout, stderr, onChange, onSync })`. `onChange(path)` and `onSync(path)` take normalized absolute paths (`"/a/b"`). Page helpers on the returned fs:
  - `fs.entry(path)` → `null` | `{kind: "dir", mode}` | `{kind: "file", mode, mtimeMs, bytes: Uint8Array}`, where `bytes` is a copy.
  - `fs.put(path, rec)`: create or replace from such a record; creates parents; fires no `onChange`.
  - `fs.exists(path)` → boolean.
  - `mkdirAll` and `writeFileAll` stay silent: they fire no `onChange`.

- [ ] **Step 1: Write the failing tests**

Append to `web/static/memfs.test.mjs`:

```js
function watched() {
  const changed = [], synced = [];
  const fs = createFS({ onChange: (p) => changed.push(p), onSync: (p) => synced.push(p) });
  return { fs, changed, synced };
}

test("changing calls report their path", () => {
  const { fs, changed } = watched();
  fs.mkdirAll("/d");
  call(fs, "mkdir", "/d/sub", 0o755);
  writeFile(fs, "/d/f", "hi");
  call(fs, "chmod", "/d/f", 0o600);
  call(fs, "utimes", "/d/f", 1, 2);
  call(fs, "unlink", "/d/f");
  call(fs, "rmdir", "/d/sub");
  assert.deepEqual([...new Set(changed)], ["/d/sub", "/d/f"]);
  assert.ok(changed.filter((p) => p === "/d/f").length >= 4); // create, write, chmod, utimes, unlink
});

test("an fd reports the path it was opened with, normalized", () => {
  const { fs, changed } = watched();
  fs.mkdirAll("/a/x");
  const fd = call(fs, "open", "/a/./x//f", C.O_WRONLY | C.O_CREAT, 0o644);
  changed.length = 0;
  call(fs, "write", fd, enc("x"), 0, 1, null);
  call(fs, "ftruncate", fd, 0);
  call(fs, "fchmod", fd, 0o600);
  call(fs, "close", fd);
  assert.deepEqual(changed, ["/a/x/f", "/a/x/f", "/a/x/f"]);
});

test("fsync reports through onSync", () => {
  const { fs, synced } = watched();
  const fd = call(fs, "open", "/f", C.O_WRONLY | C.O_CREAT, 0o644);
  call(fs, "fsync", fd);
  assert.deepEqual(synced, ["/f"]);
});

test("rename reports both paths, everything under a dir, and moves open fds", () => {
  const { fs, changed } = watched();
  fs.mkdirAll("/old/sub");
  writeFile(fs, "/old/sub/f", "x");
  const fd = call(fs, "open", "/old/sub/f", C.O_WRONLY, 0);
  changed.length = 0;
  call(fs, "rename", "/old", "/new");
  assert.deepEqual(new Set(changed), new Set(["/old", "/old/sub", "/old/sub/f", "/new", "/new/sub", "/new/sub/f"]));
  changed.length = 0;
  call(fs, "write", fd, enc("y"), 0, 1, null);
  assert.deepEqual(changed, ["/new/sub/f"]);
});

test("atomic write: temp file renamed over the real one", () => {
  const { fs, changed } = watched();
  writeFile(fs, "/f", "old");
  const fd = call(fs, "open", "/f.tmp", C.O_WRONLY | C.O_CREAT, 0o600);
  call(fs, "write", fd, enc("new"), 0, 3, null);
  call(fs, "close", fd);
  changed.length = 0;
  call(fs, "rename", "/f.tmp", "/f");
  assert.deepEqual(new Set(changed), new Set(["/f.tmp", "/f"]));
  assert.equal(fs.entry("/f.tmp"), null);
  assert.equal(dec(fs.entry("/f").bytes), "new");
});

test("entry, put and exists round-trip without reporting", () => {
  const { fs, changed } = watched();
  fs.put("/x/y/f", { kind: "file", mode: 0o100600, mtimeMs: 5000, bytes: enc("abc") });
  fs.put("/x/d", { kind: "dir", mode: 0o040700 });
  fs.writeFileAll("/x/p", enc("preset"));
  assert.deepEqual(changed, []);
  assert.ok(fs.exists("/x/y/f") && fs.exists("/x/d") && !fs.exists("/nope"));
  const e = fs.entry("/x/y/f");
  assert.equal(e.kind, "file");
  assert.equal(e.mode, 0o100600);
  assert.equal(e.mtimeMs, 5000);
  assert.equal(dec(e.bytes), "abc");
  e.bytes[0] = 0; // a copy
  assert.equal(readFile(fs, "/x/y/f"), "abc");
  assert.deepEqual(fs.entry("/x/d"), { kind: "dir", mode: 0o040700 });
  assert.equal(fs.entry("/nope"), null);
});
```

- [ ] **Step 2: Run them and watch them fail**

Run: `node --test web/static/memfs.test.mjs`
Expected: the new tests FAIL (`fs.entry is not a function`; `changed` is empty).

- [ ] **Step 3: Implement**

In `web/static/memfs.mjs`:

1. Update the header comment's last paragraph to say memfs reports changed paths through `onChange`/`onSync` so the page can save them (`persist.mjs`).
2. Change the signature:
   ```js
   export function createFS({ stdout = lineLogger(console.log), stderr = lineLogger(console.error),
     onChange = () => {}, onSync = () => {} } = {}) {
   ```
3. Add after `parts`:
   ```js
   const norm = (path) => "/" + parts(path).join("/");
   // under lists n's path and every path below it, joined onto base.
   function under(n, base) {
     const out = [base];
     if (n.kind === "dir") for (const [name, c] of n.entries) out.push(...under(c, base === "/" ? `/${name}` : `${base}/${name}`));
     return out;
   }
   ```
4. In `open`: track whether a file was created and remember the path on the fd:
   ```js
        let n, created = false;
        ...
          n = file(perm);
          created = true;
        ...
        if ((flags & constants.O_TRUNC) && n.kind === "file") { resize(n, 0, "open"); created = true; }
        const fd = nextFd++;
        fds.set(fd, { node: n, pos: 0, append: !!(flags & constants.O_APPEND), path: norm(path) });
        if (created) onChange(norm(path));
        return fd;
   ```
5. `fsync`: `run(cb, () => { onSync(open(fd, "fsync").path); });`
6. `write` (after `n.mtimeMs = now();`): `onChange(f.path);`
7. `ftruncate`: `run(cb, () => { const f = open(fd, "ftruncate"); resize(f.node, len, "ftruncate"); onChange(f.path); });`
   `truncate`: `run(cb, () => { resize(lookup(path, "truncate"), len, "truncate"); onChange(norm(path)); });`
8. `mkdir`, `unlink`, `rmdir`: call `onChange(norm(path))` as their last statement.
9. `rename`: before `fd.entries.delete(fname)`, compute `const moved = under(n, "/" + parts(from).join("/"));`; after `td.entries.set(...)`:
   ```js
        const from_ = norm(from), to_ = norm(to);
        for (const f of fds.values()) {
          if (f.path === from_ || f.path.startsWith(from_ + "/")) f.path = to_ + f.path.slice(from_.length);
        }
        for (const p of moved) { onChange(p); onChange(to_ + p.slice(from_.length)); }
   ```
   Rename the local `fd` variable in `rename` to `fdir` so it doesn't shadow the loop's `f`/`fds` naming. (`from_` and `to_` are fine; `moved` is computed from `from_` too, so compute `from_` first.)
10. `chmod`, `utimes`: add `onChange(norm(path))`. `fchmod`: `const f = open(fd, "fchmod"); const n = f.node; …; onChange(f.path);`
11. Page helpers, beside `mkdirAll`:
    ```js
    exists(path) { try { lookup(path, "exists"); return true; } catch { return false; } },
    entry(path) {
      let n;
      try { n = lookup(path, "entry"); } catch { return null; }
      if (n.kind === "dir") return { kind: "dir", mode: n.mode };
      return { kind: "file", mode: n.mode, mtimeMs: n.mtimeMs, bytes: n.data.slice(0, n.size) };
    },
    put(path, rec) {
      const ps = parts(path);
      fs.mkdirAll(ps.slice(0, -1).join("/"));
      const [d, name] = parentOf(path, "put");
      if (rec.kind === "dir") {
        const old = d.entries.get(name);
        if (old?.kind === "dir") old.mode = rec.mode; else { const n = dir(rec.mode); n.mode = rec.mode; d.entries.set(name, n); }
        return;
      }
      const n = file(rec.mode);
      n.mode = rec.mode;
      ensure(n, rec.bytes.length);
      n.data.set(rec.bytes);
      n.size = rec.bytes.length;
      n.mtimeMs = rec.mtimeMs;
      d.entries.set(name, n);
    },
    ```
    `dir(perm)`/`file(perm)` mask to `0o7777` and add the type bit; setting `n.mode = rec.mode` keeps the saved full mode as is.

- [ ] **Step 4: Run all memfs tests**

Run: `node --test web/static/memfs.test.mjs && (cd web && GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...)`
Expected: PASS, with the old tests still green.

- [ ] **Step 5: Commit**

```bash
git add web/static/memfs.mjs web/static/memfs.test.mjs
git commit -m "web: memfs reports changed paths for saving"   # + trailers
```

---

### Task 3: Save memfs to IndexedDB (`persist.mjs`)

**Files:**
- Create: `web/static/persist.mjs`
- Test: `web/static/persist.test.mjs`

**Interfaces:**
- Consumes: Task 2's `fs.entry`, `fs.put`.
- Produces:
  - `idbStore(idb)` → `{ open(): Promise, all(): Promise<[path, rec][]>, apply(puts: [path, rec][], dels: path[]): Promise }`
  - `createPersist({ store, fs, onError, delay = 500, setTimer = setTimeout, clearTimer = clearTimeout })` → `{ load(): Promise, changed(path), flush(): Promise, stop() }`
  - `store` is anything shaped like `idbStore`'s result. The spec says "takes the IndexedDB factory"; injecting this adapter instead keeps the tested logic free of IDB's request/transaction plumbing. `idbStore` is exercised by the smoke test in Task 10.

- [ ] **Step 1: Write the failing tests**

`web/static/persist.test.mjs`:

```js
import { test } from "node:test";
import assert from "node:assert/strict";
import { createFS } from "./memfs.mjs";
import { createPersist } from "./persist.mjs";

const enc = (s) => new TextEncoder().encode(s);
const dec = (b) => new TextDecoder().decode(b);

// fakeStore keeps records in a Map; fail makes the next apply reject.
function fakeStore(initial = []) {
  const recs = new Map(initial);
  const s = {
    recs, applies: 0, fail: false,
    async all() { return [...recs]; },
    async apply(puts, dels) {
      s.applies++;
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

test("load puts saved records into memfs, parents first", async () => {
  const { fs, p } = setup([
    ["/home/kiln/.config/kiln/worlds/fm.toml", { kind: "file", mode: 0o100600, mtimeMs: 7000, bytes: enc("host = 'x'") }],
    ["/home/kiln/.config/kiln", { kind: "dir", mode: 0o040700 }],
  ]);
  await p.load();
  assert.equal(dec(fs.entry("/home/kiln/.config/kiln/worlds/fm.toml").bytes), "host = 'x'");
  assert.equal(fs.entry("/home/kiln/.config/kiln").mode, 0o040700);
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
  await p.flush(); // wait for the chain the fsync started
  assert.equal(store.applies, 1);
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
```

- [ ] **Step 2: Run them and watch them fail**

Run: `node --test web/static/persist.test.mjs`
Expected: FAIL with `Cannot find module … persist.mjs`.

- [ ] **Step 3: Implement**

`web/static/persist.mjs`:

```js
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
      clearTimer(timer);
      timer = null;
      chain = chain.then(run);
      return chain;
    },
    // stop ends saving for good: another tab owns Kiln now.
    stop() { stopped = true; clearTimer(timer); },
  };
  return persist;
}
```

- [ ] **Step 4: Run the tests**

Run: `node --test web/static/persist.test.mjs`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/static/persist.mjs web/static/persist.test.mjs
git commit -m "web: save memfs to IndexedDB"   # + trailers
```

---

### Task 4: Page strings from the catalog (`[web]` → `strings.json`)

**Files:**
- Modify: `internal/str/locales/en.toml`, `internal/str/catalog/generate.go`, `internal/str/gen/main.go`
- Create (generated): `web/static/strings.json`
- Test: `internal/str/catalog/generate_test.go` (create it if it doesn't exist; otherwise append)

**Interfaces:**
- Produces: `catalog.WebJSON(en []byte) ([]byte, error)`: a JSON object of the `[web]` entries, with the `web.` prefix dropped, as `{"other_tab": "…", …}`. It errors on a `[web]` entry that's plural, uses a fmt verb, or has a literal brace, since the page's filler can't handle those.
- New catalog entries (Go functions come from `go generate`):
  - `web.other_tab`: page lock screen
  - `web.use_here`: lock screen button
  - `web.not_saving`: Go's status when IndexedDB is off or a flush fails

- [ ] **Step 1: Add the entries**

Append to `internal/str/locales/en.toml`:

```toml
# The web build. Entries here also go to web/static/strings.json for the
# page, which fills {name} placeholders only: no plurals, verbs or braces.
[web]
other_tab = "Kiln's open in another tab."
use_here = "Use here"
not_saving = "this browser isn't saving Kiln's files · they last until the tab closes"
```

- [ ] **Step 2: Write the failing test**

```go
func TestWebJSON(t *testing.T) {
	got, err := WebJSON([]byte(`
[web]
a = "hello {name}"
[other]
b = "x"
`))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 || m["a"] != "hello {name}" {
		t.Errorf("WebJSON = %v", m)
	}
	for _, bad := range []string{
		"[web]\na = { one = \"1\", other = \"{n}\" }",
		"[web]\na = \"{x:%q}\"",
		"[web]\na = \"{{literal}}\"",
	} {
		if _, err := WebJSON([]byte(bad)); err == nil {
			t.Errorf("WebJSON(%q): no error", bad)
		}
	}
}
```

Imports: `encoding/json`, `testing`. The package is `catalog`.

- [ ] **Step 3: Run it and watch it fail**

Run: `go test ./internal/str/catalog -run TestWebJSON`
Expected: FAIL, `undefined: WebJSON`.

- [ ] **Step 4: Implement**

Append to `internal/str/catalog/generate.go`:

```go
// WebJSON is the catalog's [web] table as JSON, keys without "web.", for
// the web page (web/static/strings.json). The page fills {name}
// placeholders and nothing else, so plural entries, fmt verbs and literal
// braces are errors here.
func WebJSON(en []byte) ([]byte, error) {
	c, err := Parse(en)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for k, e := range c {
		name, ok := strings.CutPrefix(k, "web.")
		if !ok {
			continue
		}
		if e.Plural() {
			return nil, fmt.Errorf("%s: the page can't show plural entries", k) //str:ok
		}
		for _, s := range e.Forms[""] {
			if s.Name != "" && s.Verb != "%v" {
				return nil, fmt.Errorf("%s: the page can't use fmt verbs", k) //str:ok
			}
			if s.Name == "" && strings.ContainsAny(s.Text, "{}") {
				return nil, fmt.Errorf("%s: the page can't show braces", k) //str:ok
			}
		}
		out[name] = e.Forms[""].String()
	}
	b, err := json.MarshalIndent(out, "", "  ") // map keys come out sorted
	return append(b, '\n'), err
}
```

Add `encoding/json` (and `strings`/`fmt` if they're missing) to the imports. Check `ParseTemplate`: if a placeholder without a verb gets `Verb == ""` rather than `"%v"`, compare against whichever it uses. The `Segment` doc says `"%v"`.

Then in `internal/str/gen/main.go`, after writing `keys_gen.go`:

```go
		if err == nil {
			var js []byte
			if js, err = catalog.WebJSON(data); err == nil {
				err = os.WriteFile("../../web/static/strings.json", js, 0o644)
			}
		}
```

Update gen's doc comment: "…and web/static/strings.json, the [web] table for the page."

- [ ] **Step 5: Generate and run the string tests**

Run: `go generate ./internal/str && go test ./internal/str/... && cat web/static/strings.json`
Expected: PASS. The JSON has the three keys.

- [ ] **Step 6: Commit**

```bash
git add internal/str web/static/strings.json
git commit -m "strings: a [web] table the page reads from strings.json

Strings: web.other_tab, web.use_here, web.not_saving (new)"   # + trailers
```

---

### Task 5: One tab owns Kiln (`tablock.mjs`)

**Files:**
- Create: `web/static/tablock.mjs`
- Test: `web/static/tablock.test.mjs`

**Interfaces:**
- Produces: `createTabLock({ locks, openChannel, onLost, setTimer = setTimeout, clearTimer = clearTimeout, stealAfter = 3000 })` → `{ tryTake(): Promise<boolean>, serve(beforeRelease: () => Promise, afterRelease: () => void), takeOver(): Promise<void> }`.
  - `locks` is shaped like `navigator.locks`; `openChannel()` returns a `BroadcastChannel("kiln")`-like object.
  - `onLost()` runs when another tab stole the lock.

- [ ] **Step 1: Write the failing tests**

`web/static/tablock.test.mjs`:

```js
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
```

- [ ] **Step 2: Run them and watch them fail**

Run: `node --test web/static/tablock.test.mjs`
Expected: FAIL, module not found.

- [ ] **Step 3: Implement**

`web/static/tablock.mjs`:

```js
// tablock: one tab owns Kiln. Two tabs saving their own memfs would
// overwrite each other's files, and two live Kilns would connect the same
// characters twice. The owner holds the Web Lock "kiln" for its lifetime;
// another tab can ask for it over BroadcastChannel ("release"), and takes
// it by force if the owner doesn't answer (a frozen background tab).

const NAME = "kiln";

export function createTabLock({ locks, openChannel, onLost, setTimer = setTimeout, clearTimer = clearTimeout, stealAfter = 3000 }) {
  let release = null;
  // hold is the lock callback: it reports whether the lock was granted,
  // and keeps it until release() is called.
  const hold = (got) => (lock) => {
    got(!!lock);
    if (!lock) return undefined;
    return new Promise((r) => { release = r; });
  };
  const lost = (e) => { if (e?.name === "AbortError") onLost(); };

  return {
    // tryTake resolves true when this tab now owns Kiln.
    tryTake() {
      return new Promise((ok) => { locks.request(NAME, { ifAvailable: true }, hold(ok)).catch(lost); });
    },
    // serve answers other tabs' requests: beforeRelease (save everything),
    // then let go of the lock, then afterRelease (reload, which shows this
    // tab the lock screen).
    serve(beforeRelease, afterRelease) {
      const ch = openChannel();
      ch.onmessage = async (e) => {
        if (e.data !== "release" || !release) return;
        await beforeRelease();
        const r = release;
        release = null;
        r();
        afterRelease();
      };
    },
    // takeOver asks the owner to let go, and steals the lock if it hasn't
    // after stealAfter ms. It resolves once this tab owns Kiln.
    takeOver() {
      return new Promise((ok) => {
        const ac = new AbortController();
        const timer = setTimer(() => {
          ac.abort();
          locks.request(NAME, { steal: true }, hold(() => ok())).catch(lost);
        }, stealAfter);
        locks.request(NAME, { signal: ac.signal }, (lock) => {
          clearTimer(timer);
          return hold(() => ok())(lock);
        }).catch(() => {}); // aborted: the steal took over
        openChannel().postMessage("release");
      });
    },
  };
}
```

- [ ] **Step 4: Run the tests**

Run: `node --test web/static/tablock.test.mjs`
Expected: PASS. If the frozen-holder test fails because `fakeLocks` rejects the holder's promise before `hold` returns, fix the fake, not `tablock.mjs`: real Web Locks reject the stolen request's returned promise with `AbortError`, and that's what the fake models.

- [ ] **Step 5: Commit**

```bash
git add web/static/tablock.mjs web/static/tablock.test.mjs
git commit -m "web: one tab owns Kiln, with use-here takeover"   # + trailers
```

---

### Task 6: `sidebar.action` role and the hooks on `ui.Deps`

**Files:**
- Modify: `internal/theme/roles.go`, `internal/theme/default.toml`, `docs/themes.md`, `internal/ui/model.go`, `internal/ui/sidebar.go`, `internal/ui/view.go`, `internal/str/locales/en.toml`
- Test: `internal/ui/web_test.go` (new), `internal/ui/golden_test.go`

**Interfaces:**
- Produces, in `ui`:
  ```go
  SaveFile func(name string, data []byte) error // offers a file to download (web); nil: browse save writes under export_dir
  Backup   func() (name string, err error)      // downloads a backup of config and pins (web); nil: no Back up
  Restore  func() (n int, err error)            // asks for a backup and writes its n files back; 0, nil when cancelled; nil: no Restore
  ```
  ```go
  // StatusMsg puts Text on the statusline (as an error when Err is set):
  // how a front end, like the web build, tells the user something.
  type StatusMsg struct {
  	Text string
  	Err  bool
  }
  ```
- New catalog entries: `sidebar.back_up = "↓ Back up"`, `sidebar.restore = "↑ Restore"`, `status.downloaded = "downloaded {name}"`, `status.backup_failed = "couldn't back up: {err}"`, `status.restore_failed = "couldn't restore: {err}"`, `status.restored = { one = "restored 1 file", other = "restored {n} files" }`.

- [ ] **Step 1: Add the role and strings**

`internal/theme/roles.go`: add `SidebarAction Role = "sidebar.action" // Back up and Restore, the web build's sidebar footer` after `SidebarAdd`, and add `SidebarAction` after `SidebarAdd` in `Roles`.
`internal/theme/default.toml`: after `"sidebar.add"`, add
`"sidebar.action"       = { fg = "celadon-bright", bg = "celadon-deep", bold = true }`.
`docs/themes.md`: in the role list, change `.add, .more` under `sidebar` to `` `.add`, `.action` (the web build's Back up and Restore), `.more` ``.
`internal/str/locales/en.toml`: under `[sidebar]` add `back_up` and `restore`; under `[status]` add the four status entries listed above. Run `go generate ./internal/str`.

- [ ] **Step 2: Write the failing tests**

`internal/ui/web_test.go`:

```go
package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/str"
)

// webCalls records what the web build's hooks were given.
type webCalls struct {
	saved      map[string]string
	backups    int
	restores   int
	restoreN   int
	restoreErr error
}

// webHarness is a harness with the web build's hooks set.
func webHarness(t *testing.T) (*harness, *webCalls) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	c := &webCalls{saved: map[string]string{}}
	h.deps.SaveFile = func(name string, data []byte) error { c.saved[name] = string(data); return nil }
	h.deps.Backup = func() (string, error) { c.backups++; return "kiln-backup.zip", nil }
	h.deps.Restore = func() (int, error) { c.restores++; return c.restoreN, c.restoreErr }
	h.m = New(h.deps, h.cfg)
	h.m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return h, c
}

// run runs cmd and feeds its message back, as Bubble Tea would.
func (h *harness) run(cmd tea.Cmd) {
	if cmd != nil {
		h.m.Update(cmd())
	}
}

func TestSidebarFooterOnlyWithHooks(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	if strings.Contains(h.screen(), str.SidebarBackUp()) {
		t.Error("desktop sidebar shows Back up")
	}
	w, _ := webHarness(t)
	lines := strings.Split(w.screen(), "\n")
	if !strings.Contains(lines[22], str.SidebarBackUp()) || !strings.Contains(lines[23], str.SidebarRestore()) {
		t.Errorf("footer rows = %q / %q", lines[22], lines[23])
	}
}

func TestBackupClickAndCommand(t *testing.T) {
	h, c := webHarness(t)
	_, cmd := h.m.Update(tea.MouseClickMsg{X: 2, Y: 22, Button: tea.MouseLeft})
	h.run(cmd)
	if c.backups != 1 || h.m.status != str.StatusDownloaded("kiln-backup.zip") {
		t.Errorf("click: backups = %d, status %q", c.backups, h.m.status)
	}
	h.typeText("/backup")
	h.run(h.enter())
	if c.backups != 2 {
		t.Errorf("/backup: backups = %d", c.backups)
	}
}

func TestRestoreReloadsAndCounts(t *testing.T) {
	h, c := webHarness(t)
	c.restoreN = 3
	_, cmd := h.m.Update(tea.MouseClickMsg{X: 2, Y: 23, Button: tea.MouseLeft})
	h.run(cmd)
	if c.restores != 1 || h.m.status != str.StatusRestored(3) {
		t.Errorf("restores = %d, status %q", c.restores, h.m.status)
	}
	c.restoreN, c.restoreErr = 0, nil // cancelled: says nothing
	h.m.status = ""
	h.typeText("/restore")
	h.run(h.enter())
	if h.m.status != "" {
		t.Errorf("cancelled restore said %q", h.m.status)
	}
	c.restoreErr = errors.New("bad zip")
	h.typeText("/restore")
	h.run(h.enter())
	if h.m.status != str.StatusRestoreFailed(c.restoreErr) {
		t.Errorf("failed restore said %q", h.m.status)
	}
}

func TestBackupCommandsUnknownOnDesktop(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("/backup")
	h.enter()
	if h.m.status != str.StatusUnknownCommand("/backup") {
		t.Errorf("status = %q", h.m.status)
	}
}

func TestStatusMsg(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.Update(StatusMsg{Text: str.WebNotSaving(), Err: true})
	if h.m.status != str.WebNotSaving() {
		t.Errorf("status = %q", h.m.status)
	}
}
```

Check how `harness` exposes the statusline (`h.m.status` is the field shown in `model.go`); if `setStatus` stores a prefix or role with it, compare with whatever the existing tests use, e.g. `grep -n "m.status !=" internal/ui/*_test.go`. Check also that the default harness with nothing open shows the idle input, not a character, so the screen's rows 22–23 are the bottom of the sidebar. If the harness autoconnects Kit, the footer is still the last two sidebar rows.

Golden, in `internal/ui/golden_test.go`:

```go
func TestGoldenWebSidebar(t *testing.T) {
	h, _ := webHarness(t)
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.advance(time.Minute)
	assertGolden(t, "web-sidebar", h.drawn())
}
```

`webHarness` uses `fmWorld`; switch it to `goldenWorld` if the golden needs local echo to match `main.txt`'s look. It doesn't matter for this screen.

- [ ] **Step 3: Run them and watch them fail**

Run: `go test ./internal/ui -run 'Sidebar|Backup|Restore|StatusMsg|GoldenWeb'`
Expected: compile FAIL (`h.deps.SaveFile undefined`).

- [ ] **Step 4: Implement**

`internal/ui/model.go`:

1. Add the three fields to `Deps` (after `OpenURL`), with the comments from Interfaces.
2. Add `StatusMsg` (from Interfaces) and the two private messages near `resizeMsg`:
   ```go
   type backupDoneMsg struct {
   	name string
   	err  error
   }
   type restoreDoneMsg struct {
   	n   int
   	err error
   }
   ```
3. In `update`'s message switch, add:
   ```go
   case StatusMsg:
   	m.setStatus(msg.Err, msg.Text)
   case backupDoneMsg:
   	if msg.err != nil {
   		m.setStatus(true, str.StatusBackupFailed(msg.err))
   	} else {
   		m.setStatus(false, str.StatusDownloaded(msg.name))
   	}
   case restoreDoneMsg:
   	switch {
   	case msg.err != nil:
   		m.setStatus(true, str.StatusRestoreFailed(msg.err))
   	case msg.n > 0 && m.reloadNow():
   		m.setStatus(false, str.StatusRestored(msg.n))
   	}
   ```
   (`reloadNow` sets its own error status when the restored config doesn't load.)
4. The commands, near `command`:
   ```go
   // backupCmd runs the Backup hook off the UI goroutine.
   func (m *Model) backupCmd() tea.Cmd {
   	backup := m.d.Backup
   	return func() tea.Msg { name, err := backup(); return backupDoneMsg{name, err} }
   }

   // restoreCmd runs the Restore hook, which waits for the user to pick a
   // file, off the UI goroutine.
   func (m *Model) restoreCmd() tea.Cmd {
   	restore := m.d.Restore
   	return func() tea.Msg { n, err := restore(); return restoreDoneMsg{n, err} }
   }
   ```
5. In `command`: put this first, so a web-only command on desktop is just unknown (not "needs a character"):
   ```go
   	if (args[0] == "/backup" && m.d.Backup == nil) || (args[0] == "/restore" && m.d.Restore == nil) {
   		m.setStatus(true, str.StatusUnknownCommand(args[0]))
   		return nil
   	}
   ```
   Let them through the needs-a-character guard:
   ```go
   	if cs == nil && args[0] != "/quit" && args[0] != "/open" && args[0] != "/backup" && args[0] != "/restore" {
   ```
   and add cases before `default`:
   ```go
   	case "/backup":
   		return m.backupCmd()
   	case "/restore":
   		return m.restoreCmd()
   ```
6. In `handleClick`, right after the browse-panel check inside `if msg.X < l.sw {`:
   ```go
   		if f := m.footerH(); f > 0 && msg.Y >= m.height-f {
   			if msg.Y == m.height-f {
   				return m.backupCmd()
   			}
   			return m.restoreCmd()
   		}
   ```

`internal/ui/sidebar.go`:

```go
// footerH is how many rows the web build's Back up and Restore take at the
// bottom of the sidebar: none on desktop, in the picker, or on a short screen.
func (m *Model) footerH() int {
	if m.d.Backup == nil || m.d.Restore == nil || m.listing() || m.height < 8 {
		return 0
	}
	return 2
}

// footerLabels are the footer's rows, top to bottom.
var footerLabels = []string{str.SidebarBackUp(), str.SidebarRestore()}
```

In `sidebarView`, change `h := max(1, m.height)` to `h := max(1, m.height-m.footerH())`.

`internal/ui/view.go`, in the sidebar loop: compute `foot := m.footerH()` before the loop, and add a case right after `case panel != nil:`:

```go
		case y >= m.height-foot:
			b.WriteString(theme.Fill(side, fit(chip(theme.SidebarAction, footerLabels[y-(m.height-foot)]), l.sw), l.sw))
```

- [ ] **Step 5: Run the tests, generate the new golden, and check no other golden moved**

Run: `go test ./internal/ui -run TestGoldenWebSidebar -update && go test ./internal/ui ./internal/theme ./internal/str/... && git status --short internal/ui/testdata/golden`
Expected: PASS. `git status` shows only `web-sidebar.txt` as new. Read it: the last two sidebar rows are ` ↓ Back up ` and ` ↑ Restore ` with the `sidebar.action` colors.

- [ ] **Step 6: Commit**

```bash
git add internal/theme internal/ui internal/str docs/themes.md
git commit -m "ui: Back up and Restore rows, and hooks for the web build

New golden web-sidebar.txt: the sidebar with the web build's footer.
Existing goldens are unchanged.

Strings: sidebar.back_up, sidebar.restore, status.downloaded, status.backup_failed, status.restore_failed, status.restored (new)"   # + trailers
```

---

### Task 7: Browse save downloads on the web

**Files:**
- Modify: `internal/ui/browse.go`, `internal/ui/model.go` (`openBrowse`)
- Test: `internal/ui/web_test.go`

**Interfaces:**
- Consumes: `Deps.SaveFile` and `str.StatusDownloaded` from Task 6.

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/web_test.go`. Model it on the existing browse export test: find it with `grep -n "BrowseSaved" internal/ui/browse_test.go` and copy its key sequence, which marks a range, presses the export key, picks a format, and gets to the filename prompt. Then:

```go
func TestBrowseSaveDownloads(t *testing.T) {
	h, c := webHarness(t)
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	// …the same keys the existing export test uses to reach the filename prompt…
	if v := h.br().pin.Value(); strings.Contains(v, "/") {
		t.Errorf("web filename prompt has a folder: %q", v)
	}
	h.br().pin.SetValue("~/scenes/x.txt")
	h.key("enter")
	if _, ok := c.saved["x.txt"]; !ok || len(c.saved) != 1 {
		t.Errorf("saved = %v", c.saved)
	}
	if h.br().status != str.StatusDownloaded("x.txt") { // or however browse exposes its status; see the existing test
		t.Errorf("status = %q", h.br().status)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/ui -run TestBrowseSaveDownloads`
Expected: FAIL: the prompt holds the export dir, and `saved` is empty.

- [ ] **Step 3: Implement**

`internal/ui/browse.go`:

1. Add a field beside `copy`: `saveFile func(name string, data []byte) error // Deps.SaveFile; set, saving downloads instead`.
2. In the format prompt (`scene.FileName(b.exportDir, …)`), use the folder only on desktop:
   ```go
   			dir := b.exportDir
   			if b.saveFile != nil {
   				dir = "" // a download: just a name
   			}
   			b.pin.SetValue(scene.FileName(dir, b.exportName, sel[0].Time.Local(), b.cs.ch.World, b.cs.ch.Name, f))
   ```
3. In `save`, after the empty-name check:
   ```go
   	if b.saveFile != nil {
   		sel := b.selection()
   		if len(sel) == 0 {
   			b.setStatus(true, str.BrowseNothingToExport())
   			return
   		}
   		name := filepath.Base(path)
   		if err := b.saveFile(name, []byte(scene.Render(b.format, sel, b.title()))); err != nil {
   			b.setStatus(true, err.Error())
   			return
   		}
   		b.setStatus(false, str.StatusDownloaded(name))
   		return
   	}
   ```
   Update `save`'s doc comment: "…With saveFile set (the web build) it offers a download named after path's last element instead."

`internal/ui/model.go`, `openBrowse`: after `cs.browse.copy = m.copyCmd` add `cs.browse.saveFile = m.d.SaveFile`.

- [ ] **Step 4: Run the ui tests**

Run: `go test ./internal/ui`
Expected: PASS, and no golden changes.

- [ ] **Step 5: Commit**

```bash
git add internal/ui
git commit -m "ui: browse save offers a download when the front end can"   # + trailers
```

---

### Task 8: Pack and unpack backups (`web/backup`)

**Files:**
- Create: `web/backup/backup.go`, `web/backup/backup_test.go`
- Modify: `internal/str/locales/en.toml`

**Interfaces:**
- Produces:
  ```go
  const MaxUnpacked = 10 << 20
  func Zip(configDir, knownHosts string) ([]byte, error)
  func Unzip(data []byte, configDir, knownHosts string) (int, error)
  ```
- New catalog entries, in a `[backup]` table:
  - `backup.not_zip = "not a zip file"`
  - `backup.stray_file = "not a Kiln backup: it has {name:%q}"`
  - `backup.too_big = "backup unpacks to more than {mb} MB"`
  - `backup.empty = "no Kiln files in this backup"`
  - `backup.file_name = "kiln-backup-{date}.zip"`

- [ ] **Step 1: Add the entries, then write the failing tests**

Add the `[backup]` table to `en.toml` and run `go generate ./internal/str`.

`web/backup/backup_test.go`:

```go
package backup

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, s string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o700)
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// zipOf builds a zip with the given names and contents.
func zipOf(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(body))
	}
	w.Close()
	return buf.Bytes()
}

func TestRoundTrip(t *testing.T) {
	src := t.TempDir()
	write(t, filepath.Join(src, "config", "config.toml"), "theme = 'x'")
	write(t, filepath.Join(src, "config", "worlds", "fm.toml"), "host = 'fm'")
	write(t, filepath.Join(src, "known_hosts"), "fm:5555 sha256")
	data, err := Zip(filepath.Join(src, "config"), filepath.Join(src, "known_hosts"))
	if err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	write(t, filepath.Join(dst, "config", "worlds", "mine.toml"), "host = 'mine'") // added since: survives
	n, err := Unzip(data, filepath.Join(dst, "config"), filepath.Join(dst, "known_hosts"))
	if err != nil || n != 3 {
		t.Fatalf("Unzip = %d, %v", n, err)
	}
	if read(t, filepath.Join(dst, "config", "worlds", "fm.toml")) != "host = 'fm'" ||
		read(t, filepath.Join(dst, "known_hosts")) != "fm:5555 sha256" ||
		read(t, filepath.Join(dst, "config", "worlds", "mine.toml")) != "host = 'mine'" {
		t.Error("files after restore don't match")
	}
}

func TestZipWithoutKnownHosts(t *testing.T) {
	src := t.TempDir()
	write(t, filepath.Join(src, "config", "config.toml"), "x")
	if _, err := Zip(filepath.Join(src, "config"), filepath.Join(src, "known_hosts")); err != nil {
		t.Fatal(err)
	}
}

func TestUnzipRejectsBeforeWriting(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"dotdot":   {"config/ok.toml": "x", "config/../../evil": "x"},
		"absolute": {"config/ok.toml": "x", "/etc/evil": "x"},
		"stray":    {"config/ok.toml": "x", "notes.txt": "x"},
	} {
		dst := t.TempDir()
		if _, err := Unzip(zipOf(t, files), filepath.Join(dst, "config"), filepath.Join(dst, "known_hosts")); err == nil {
			t.Errorf("%s: no error", name)
		}
		if _, err := os.Stat(filepath.Join(dst, "config", "ok.toml")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: wrote ok.toml before rejecting", name)
		}
	}
}

func TestUnzipSkipsDirsAndRejectsEmptyAndJunk(t *testing.T) {
	dst := t.TempDir()
	n, err := Unzip(zipOf(t, map[string]string{"config/": "", "config/worlds/": "", "config/a.toml": "x"}), filepath.Join(dst, "config"), filepath.Join(dst, "known_hosts"))
	if err != nil || n != 1 {
		t.Errorf("dirs: %d, %v", n, err)
	}
	if _, err := Unzip(zipOf(t, map[string]string{"config/": ""}), filepath.Join(dst, "config"), filepath.Join(dst, "known_hosts")); err == nil {
		t.Error("empty backup: no error")
	}
	if _, err := Unzip([]byte("not a zip"), filepath.Join(dst, "config"), filepath.Join(dst, "known_hosts")); err == nil {
		t.Error("junk: no error")
	}
}

func TestUnzipTooBig(t *testing.T) {
	dst := t.TempDir()
	big := string(bytes.Repeat([]byte("a"), MaxUnpacked+1))
	if _, err := Unzip(zipOf(t, map[string]string{"config/big": big}), filepath.Join(dst, "config"), filepath.Join(dst, "known_hosts")); err == nil {
		t.Error("too big: no error")
	}
}
```

Error text isn't compared, so nothing here copies catalog text.

- [ ] **Step 2: Run them and watch them fail**

Run: `(cd web && go test ./backup)`
Expected: FAIL: `undefined: Zip`.

- [ ] **Step 3: Implement**

`web/backup/backup.go`:

```go
// Package backup packs Kiln's config and TLS pins into a zip and unpacks
// one back: the web build's Back up and Restore. Browsers can clear what
// a page saves, so this is the copy the user keeps.
package backup

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/latrani/Kiln/internal/str"
)

// MaxUnpacked caps a restore's total size.
const MaxUnpacked = 10 << 20

const (
	configPrefix = "config/"     //str:ok
	knownName    = "known_hosts" //str:ok
)

// Zip packs every file under configDir as config/… and knownHosts as
// known_hosts, if it exists.
func Zip(configDir, knownHosts string) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	add := func(name, file string) error {
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		f, err := w.Create(name)
		if err != nil {
			return err
		}
		_, err = f.Write(b)
		return err
	}
	err := filepath.WalkDir(configDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(configDir, p)
		if err != nil {
			return err
		}
		return add(configPrefix+filepath.ToSlash(rel), p)
	})
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(knownHosts); err == nil {
		if err := add(knownName, knownHosts); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Unzip writes a backup's files over configDir and knownHosts. It checks
// every entry before writing any, never deletes a file, and returns how
// many files it wrote.
func Unzip(data []byte, configDir, knownHosts string) (int, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, str.Wrap(str.BackupNotZip(), err)
	}
	type item struct {
		f    *zip.File
		dest string
	}
	var items []item
	var total uint64
	for _, f := range r.File {
		if strings.HasSuffix(f.Name, "/") {
			continue // a folder: made as files need it
		}
		clean := path.Clean(f.Name)
		var dest string
		switch {
		case clean != f.Name || path.IsAbs(clean):
		case clean == knownName:
			dest = knownHosts
		case strings.HasPrefix(clean, configPrefix):
			dest = filepath.Join(configDir, filepath.FromSlash(strings.TrimPrefix(clean, configPrefix)))
		}
		if dest == "" {
			return 0, errors.New(str.BackupStrayFile(f.Name))
		}
		total += f.UncompressedSize64
		if total > MaxUnpacked {
			return 0, errors.New(str.BackupTooBig(MaxUnpacked >> 20))
		}
		items = append(items, item{f, dest})
	}
	if len(items) == 0 {
		return 0, errors.New(str.BackupEmpty())
	}
	for _, it := range items {
		rc, err := it.f.Open()
		if err != nil {
			return 0, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, MaxUnpacked+1))
		rc.Close()
		if err != nil {
			return 0, err
		}
		if len(b) > MaxUnpacked {
			return 0, errors.New(str.BackupTooBig(MaxUnpacked >> 20))
		}
		if err := os.MkdirAll(filepath.Dir(it.dest), 0o700); err != nil {
			return 0, err
		}
		if err := os.WriteFile(it.dest, b, 0o600); err != nil {
			return 0, err
		}
	}
	return len(items), nil
}
```

`path.Clean("config/../../evil")` is `"../evil"`, which differs from the name, so the first case rejects it. `"/etc/evil"` is absolute. Both leave `dest` empty, and `notes.txt` matches nothing.

- [ ] **Step 4: Run the tests**

Run: `(cd web && go test ./backup) && go test ./internal/str`
Expected: PASS. If the string lint scans `web/` and flags `configPrefix`/`knownName`, they're already marked `//str:ok` (file-format vocabulary).

- [ ] **Step 5: Commit**

```bash
git add web/backup internal/str
git commit -m "web: pack and unpack backups of config and pins

Strings: backup.not_zip, backup.stray_file, backup.too_big, backup.empty, backup.file_name (new)"   # + trailers
```

---

### Task 9: Wire it up in the page and `kiln-web`

**Files:**
- Modify: `web/static/kiln.js`, `web/static/index.html`, `web/cmd/kiln-web/main.go`
- Create: `web/cmd/kiln-web/page.go`

**Interfaces:**
- Consumes: everything above.
- Page ↔ Go contract:
  - `kiln.start(cols, rows, relayURL, write, saving: boolean)`
  - `kiln.storageFailed()`
  - `globalThis.kilnDownload(name: string, bytes: Uint8Array)`
  - `globalThis.kilnPickFile(): Promise<Uint8Array | null>`

- [ ] **Step 1: `kiln-web`'s page calls**

`web/cmd/kiln-web/page.go`:

```go
//go:build js

package main

import (
	"errors"
	"syscall/js"
)

// download hands bytes to the page, which offers them as a file.
func download(name string, data []byte) error {
	u := js.Global().Get("Uint8Array").New(len(data)) //str:ok
	js.CopyBytesToJS(u, data)
	js.Global().Call("kilnDownload", name, u) //str:ok
	return nil
}

// pickFile asks the page for a file and waits; nil, nil when the user
// cancels.
func pickFile() ([]byte, error) {
	type result struct {
		b   []byte
		err error
	}
	ch := make(chan result, 1)
	ok := js.FuncOf(func(_ js.Value, a []js.Value) any {
		if a[0].IsNull() {
			ch <- result{}
			return nil
		}
		b := make([]byte, a[0].Length())
		js.CopyBytesToGo(b, a[0])
		ch <- result{b: b}
		return nil
	})
	fail := js.FuncOf(func(_ js.Value, a []js.Value) any {
		ch <- result{err: errors.New(a[0].Call("toString").String())} //str:ok
		return nil
	})
	defer ok.Release()
	defer fail.Release()
	js.Global().Call("kilnPickFile").Call("then", ok, fail) //str:ok
	r := <-ch
	return r.b, r.err
}
```

- [ ] **Step 2: `kiln-web` main**

In `web/cmd/kiln-web/main.go`:

1. `page` gains `saving bool` and `storageFailed chan struct{}`.
2. In `start`: `saving := len(a) > 4 && a[4].Bool()`. Make `storageFailed := make(chan struct{}, 1)` and set
   ```go
   		k.Set("storageFailed", js.FuncOf(func(js.Value, []js.Value) any { //str:ok
   			select {
   			case storageFailed <- struct{}{}:
   			default:
   			}
   			return nil
   		}))
   ```
   Pass both into `page{…}`.
3. In `run`, compute `knownHosts := filepath.Join(dataDir, "known_hosts")` (reuse it for `kh`), and add to `ui.Deps`:
   ```go
   		SaveFile: download,
   		Backup: func() (string, error) {
   			data, err := backup.Zip(cfgDir, knownHosts)
   			if err != nil {
   				return "", err
   			}
   			name := str.BackupFileName(time.Now().Format("2006-01-02")) //str:ok
   			return name, download(name, data)
   		},
   		Restore: func() (int, error) {
   			data, err := pickFile()
   			if err != nil || data == nil {
   				return 0, err
   			}
   			return backup.Unzip(data, cfgDir, knownHosts)
   		},
   ```
   Imports: `time`, `github.com/latrani/Kiln/web/backup`.
4. After `prog := tea.NewProgram(…)` and `p.b.Attach(prog)`:
   ```go
   	notSaving := ui.StatusMsg{Text: str.WebNotSaving(), Err: true}
   	go func() {
   		if !p.saving {
   			prog.Send(notSaving)
   		}
   		for range p.storageFailed {
   			prog.Send(notSaving)
   		}
   	}()
   ```
   `prog.Send` blocks until the program runs, which is fine in a goroutine. The channel is never closed; the goroutine lives as long as the page, like the bridge's (#115 tracks that).
5. Update the doc comment at the top: files are saved to IndexedDB by the page (`persist.mjs`).

- [ ] **Step 3: The page**

`web/static/index.html`: add, after `#term`:

```html
<div id="lock" hidden>
  <p id="lock-text"></p>
  <button id="lock-use"></button>
</div>
```

and to the `<style>`:

```css
  #lock { position: fixed; inset: 0; display: flex; flex-direction: column; align-items: center;
          justify-content: center; gap: 1em; color: #ccc; font: 16px ui-monospace, Menlo, monospace; }
  #lock[hidden] { display: none; }
  #lock button { font: inherit; padding: .4em 1.2em; }
```

`web/static/kiln.js`: restructure the boot. The full new file:

```js
// Boots Kiln in the page: takes the tab lock (one tab owns Kiln), loads
// saved files from IndexedDB into memfs, seeds the preset where nothing's
// saved, starts Go's wasm runtime, and gives Kiln xterm.js as its
// terminal. memfs must be in place before wasm_exec.js loads: wasm_exec
// only stubs globalThis.fs if it's missing.
import { Terminal } from "./vendor/xterm.mjs";
import { FitAddon } from "./vendor/addon-fit.mjs";
import { WebglAddon } from "./vendor/addon-webgl.mjs";
import { ClipboardAddon } from "./vendor/addon-clipboard.mjs";
import { createFS } from "./memfs.mjs";
import { createPersist, idbStore } from "./persist.mjs";
import { createTabLock } from "./tablock.mjs";

const HOME = "/home/kiln";
const CONFIG = `${HOME}/.config/kiln`;

const strings = await (await fetch("strings.json")).json();
const t = (key, args = {}) => strings[key].replace(/\{([a-z][a-z0-9_]*)\}/g, (_, n) => String(args[n]));

function loadScript(src) {
  return new Promise((ok, fail) => {
    const s = document.createElement("script");
    s.src = src;
    s.onload = ok;
    s.onerror = () => fail(new Error(`loading ${src}`));
    document.head.append(s);
  });
}

// seedPreset writes the preset's files where nothing is saved yet: a
// saved copy (the user's edits) wins. Seeded files aren't saved, so an
// untouched preset world follows the server.
async function seedPreset(fs) {
  const files = await (await fetch("preset/manifest.json")).json();
  for (const f of files) {
    if (fs.exists(`${CONFIG}/${f}`)) continue;
    const res = await fetch(`preset/${f}`);
    if (!res.ok) throw new Error(`preset/${f}: ${res.status}`);
    fs.writeFileAll(`${CONFIG}/${f}`, new Uint8Array(await res.arrayBuffer()));
  }
}

// The tab lock. Without Web Locks (not a secure context), every tab runs.
const lock = navigator.locks && createTabLock({
  locks: navigator.locks,
  openChannel: () => new BroadcastChannel("kiln"),
  onLost: () => { persist?.stop(); location.reload(); },
});
let persist = null;

if (lock && !(await lock.tryTake())) {
  const screen = document.getElementById("lock");
  const use = document.getElementById("lock-use");
  document.getElementById("lock-text").textContent = t("other_tab");
  use.textContent = t("use_here");
  screen.hidden = false;
  await new Promise((ok) => {
    use.onclick = async () => { use.disabled = true; await lock.takeOver(); ok(); };
  });
  screen.hidden = true;
}

// convertEol: with no TTY to put in raw mode, Bubble Tea draws as if the
// terminal turns "\n" into "\r\n", so xterm.js has to do that too.
const term = new Terminal({
  fontFamily: "ui-monospace, Menlo, Consolas, monospace",
  fontSize: 14,
  convertEol: true,
});
const fit = new FitAddon();
term.loadAddon(fit);
term.loadAddon(new ClipboardAddon()); // OSC 52: Kiln's copy
term.open(document.getElementById("term"));
try { term.loadAddon(new WebglAddon()); } catch { /* the DOM renderer is fine */ }
fit.fit();
addEventListener("resize", () => fit.fit());

let saving = true;
const fs = createFS({ onChange: (p) => persist?.changed(p), onSync: () => persist?.flush() });
try {
  const store = idbStore(indexedDB);
  await store.open();
  persist = createPersist({
    store, fs,
    onError: (e) => { console.error(e); saving = false; globalThis.kiln?.storageFailed?.(); },
  });
  await persist.load();
} catch (e) {
  console.error(e);
  persist = null;
  saving = false;
}
fs.mkdirAll("/tmp");
fs.mkdirAll(HOME);
globalThis.fs = fs;
const enosys = () => Object.assign(new Error("ENOSYS"), { code: "ENOSYS" });
globalThis.process = {
  getuid: () => -1, getgid: () => -1, geteuid: () => -1, getegid: () => -1,
  getgroups() { throw enosys(); }, pid: -1, ppid: -1,
  umask: () => 0o022, cwd: () => "/", chdir() { throw enosys(); },
};
await seedPreset(fs);
navigator.storage?.persist?.().catch(() => {}); // best effort; Safari often says no
addEventListener("visibilitychange", () => { if (document.visibilityState === "hidden") persist?.flush(); });
lock?.serve(async () => { await persist?.flush(); persist?.stop(); }, () => location.reload());

globalThis.kilnDownload = (name, bytes) => {
  const url = URL.createObjectURL(new Blob([bytes]));
  const a = Object.assign(document.createElement("a"), { href: url, download: name });
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
};
globalThis.kilnPickFile = () => new Promise((ok) => {
  const input = Object.assign(document.createElement("input"), { type: "file", accept: ".zip" });
  input.hidden = true;
  document.body.append(input);
  const done = (v) => { input.remove(); ok(v); };
  input.onchange = async () => { const f = input.files[0]; done(f ? new Uint8Array(await f.arrayBuffer()) : null); };
  input.oncancel = () => done(null);
  input.click();
});

await loadScript("wasm_exec.js");

const relay = new URL("relay", location.href);
relay.protocol = location.protocol === "https:" ? "wss:" : "ws:";

globalThis.kilnReady = () => {
  kiln.start(term.cols, term.rows, relay.href, (bytes) => term.write(bytes), saving);
  term.onData((d) => kiln.input(d));
  term.onBinary((d) => kiln.input(d));
  term.onResize(({ cols, rows }) => kiln.resize(cols, rows));
  term.focus();
};

const go = new Go();
go.env = { HOME, TMPDIR: "/tmp" };
const wasm = await (await fetch("kiln.wasm")).arrayBuffer();
const { instance } = await WebAssembly.instantiate(wasm, go.importObject);
await go.run(instance);
```

If Task 1 found Safari blocks the delayed picker, add the drop fallback it describes to `kilnPickFile`, in the same function.

- [ ] **Step 4: Build and run every test suite**

```bash
web/build.sh
go test ./...
(cd web && go test ./...)
(cd web && GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...)
node --test web/static/*.test.mjs
go vet ./... && (cd web && GOOS=js GOARCH=wasm go vet ./...)
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add web/static web/cmd/kiln-web
git commit -m "web: save files across visits, one tab at a time, with Back up and Restore"   # + trailers
```

---

### Task 10: Smoke test in a real browser, docs, PR

**Files:**
- Modify: `docs/web-dev.md`
- Scratch (not committed): `$SCRATCH/smoke.mjs`

- [ ] **Step 1: Run the dev loop with a scratch config**

Use the preset in `web/preset.local` and `web/dev-allow.toml` (they exist in the main checkout). Start the relay in the background:

```bash
go run ./cmd/kiln-relay -dev -allow web/dev-allow.toml -web web/static -preset web/preset.local
```

- [ ] **Step 2: Drive headless Chrome over DevTools**

Write `$SCRATCH/smoke.mjs`: a small node script with no npm dependencies, using node's built-in `WebSocket`. It launches `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome --headless=new --remote-debugging-port=9333 --user-data-dir=$SCRATCH/chrome` and talks CDP. Check each of these, printing PASS/FAIL per item:

1. Load `http://localhost:8080/kiln/`, wait for the screen text to show the preset world in the sidebar (read `term.buffer` via `Runtime.evaluate`), and type `/edit world` + Enter, change a field, save.
2. Reload the page; run `Runtime.evaluate` on `fs.entry("/home/kiln/.config/kiln/worlds/<file>.toml")` and check the edit is there.
3. Open a second target (`Target.createTarget`) at the same URL; check `#lock` is visible and `#lock-text` reads `strings.other_tab`. Click `#lock-use`; check the first target reloads and shows `#lock`, and the second's `#lock` is hidden.
4. `Browser.setDownloadBehavior` to `$SCRATCH/dl`. Dispatch a mouse press on the `↓ Back up` row (`Input.dispatchMouseEvent` at the sidebar's second-to-last row). Check a `kiln-backup-*.zip` lands in `$SCRATCH/dl`.
5. Restore: `Page.setInterceptFileChooserDialog({enabled: true})`, press `↑ Restore`, answer `Page.fileChooserOpened` with `DOM.setFileInputFiles` pointing at the zip, and check the statusline shows `restored N files`.

Report results to Indi, along with any item you couldn't automate.

- [ ] **Step 3: Update `docs/web-dev.md`**

Replace the last paragraph ("Files live in memory…") with:

```markdown
Files are saved in the browser (IndexedDB) and come back on the next
visit: config edits, worlds, TLS pins and logs. That's best effort:
clearing site data, a private window, or a browser that evicts storage
loses them. Safari clears a site's storage after 7 days of Safari use
without visiting it. **↓ Back up** at the bottom of the sidebar downloads
a zip of your config and pins; **↑ Restore** reads one back (it adds and
replaces files, never deletes). `/backup` and `/restore` do the same.
Saving a log in log mode downloads the file.

One tab runs Kiln at a time. A second tab offers **Use here**, which
moves Kiln over; the first tab saves, disconnects, and shows the same
offer.

Preset files are written only where nothing is saved, so your edits to a
preset world win over the server's copy.
```

Add `node --test web/static/*.test.mjs` in place of the single memfs line in the Tests block.

- [ ] **Step 4: Commit and open the PR**

```bash
git add docs/web-dev.md
git commit -m "docs: what the web build saves, Back up and Restore, and the tab lock"   # + trailers
git push -u origin web-storage
gh pr create --title "Web: storage across visits (#74 part 3)" --body "…"
```

The PR body covers:
- what it does, in one paragraph;
- the Task 1 result;
- "Fixes the per-visit half of #114: pins now last per browser. Seeded pins / `ca` trust for the PFMuck preset is still part 5";
- the new golden `web-sidebar.txt` (intended; existing goldens unchanged);
- **Strings**, each key with one line on why it exists: `web.other_tab`, `web.use_here`, `web.not_saving`, `sidebar.back_up`, `sidebar.restore`, `status.downloaded`, `status.backup_failed`, `status.restore_failed`, `status.restored`, `backup.not_zip`, `backup.stray_file`, `backup.too_big`, `backup.empty`, `backup.file_name`;
- the PR attribution footer.

Don't merge. Stop and tell Indi it's ready.
