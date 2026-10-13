// Boots Kiln in the page: takes the tab lock (one tab owns Kiln), loads
// saved files from IndexedDB into memfs, seeds the preset where nothing's
// saved, downloads and starts Go's wasm, and feeds its batches to the
// store. memfs must be in place before wasm_exec.js loads: wasm_exec only
// stubs globalThis.fs if it's missing.
import { createFS } from "../../static/memfs.mjs";
import { createPersist, idbStore } from "../../static/persist.mjs";
import { createTabLock } from "../../static/tablock.mjs";
import type { Event, Store } from "./store.ts";
import * as str from "./str.gen.ts";

const HOME = "/home/kiln";
const CONFIG = `${HOME}/.config/kiln`;

// readWithProgress reads res's body, saying after each chunk how much has
// come and, when the server said, how much there is.
export async function readWithProgress(res: Response, onProgress: (got: number, total?: number) => void): Promise<ArrayBuffer> {
  // A compressed response's length is what came over the wire, not the
  // file; and a length the download outgrows was wrong anyway.
  const n = Number(res.headers.get("content-length"));
  let total = Number.isFinite(n) && n > 0 && !res.headers.get("content-encoding") ? n : undefined;
  const chunks: Uint8Array[] = [];
  let got = 0;
  const reader = res.body!.getReader();
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    chunks.push(value);
    got += value.byteLength;
    if (total !== undefined && got > total) total = undefined;
    onProgress(got, total);
  }
  const out = new Uint8Array(got);
  let at = 0;
  for (const c of chunks) {
    out.set(c, at);
    at += c.byteLength;
  }
  return out.buffer;
}

const mb = (n: number) => (n / 1e6).toFixed(1);

// loadingText is the status line's words while the wasm downloads.
export function loadingText(got: number, total?: number): string {
  return total ? str.webLoadingSize(mb(got), mb(total)) : str.webLoading(mb(got));
}

function loadScript(src: string): Promise<void> {
  return new Promise((ok, fail) => {
    const s = document.createElement("script");
    s.src = src;
    s.onload = () => ok();
    s.onerror = () => fail(new Error(`loading ${src}`)); // a developer's message, in the console
    document.head.append(s);
  });
}

// seedPreset writes the preset's files where nothing is saved yet: a
// saved copy (the user's edits) wins. Seeded files aren't saved, so an
// untouched preset world follows the server.
async function seedPreset(fs: any): Promise<void> {
  const files: string[] = await (await fetch("preset/manifest.json")).json();
  for (const f of files) {
    if (fs.exists(`${CONFIG}/${f}`)) continue;
    const res = await fetch(`preset/${f}`);
    if (!res.ok) throw new Error(`preset/${f}: ${res.status}`);
    fs.writeFileAll(`${CONFIG}/${f}`, new Uint8Array(await res.arrayBuffer()));
  }
}

// download saves bytes as a file named name.
export function download(name: string, bytes: Uint8Array): void {
  const url = URL.createObjectURL(new Blob([bytes as BlobPart]));
  const a = Object.assign(document.createElement("a"), { href: url, download: name });
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

// boot starts Kiln, telling the store how far it's got.
export async function boot(store: Store): Promise<void> {
  const g = globalThis as any;
  let persist: any = null;
  // The tab lock. Without Web Locks (not a secure context), every tab runs.
  const lock = navigator.locks && createTabLock({
    locks: navigator.locks,
    openChannel: () => new BroadcastChannel("kiln"),
    onLost: () => { persist?.stop(); location.reload(); },
  });
  if (lock && !(await lock.tryTake())) {
    await new Promise<void>((ok) => {
      store.set({
        phase: "locked",
        onUseHere: async () => {
          store.set({ onUseHere: undefined }); // one click is enough
          await lock.takeOver();
          ok();
        },
      });
    });
  }
  store.set({ phase: "loading", progress: { got: 0 } });

  let saving = true;
  const fs = createFS({ onChange: (p: string) => persist?.changed(p), onSync: () => persist?.flush() });
  try {
    const db = idbStore(indexedDB);
    await db.open();
    persist = createPersist({
      store: db, fs,
      onError: (e: unknown) => { console.error(e); store.set({ saving: false }); g.kiln?.storageFailed?.(); },
    });
    await persist.load();
  } catch (e) {
    console.error(e);
    persist = null;
    saving = false;
  }
  store.set({ saving });
  fs.mkdirAll("/tmp");
  fs.mkdirAll(HOME);
  g.fs = fs;
  const enosys = () => Object.assign(new Error("ENOSYS"), { code: "ENOSYS" });
  g.process = {
    getuid: () => -1, getgid: () => -1, geteuid: () => -1, getegid: () => -1,
    getgroups() { throw enosys(); }, pid: -1, ppid: -1,
    umask: () => 0o022, cwd: () => "/", chdir() { throw enosys(); },
  };
  await seedPreset(fs);
  navigator.storage?.persist?.().catch(() => {}); // best effort; Safari often says no

  const act = (o: object) => g.kiln?.act?.(JSON.stringify(o));
  store.set({ act });
  const saveDrafts = () => act({ Do: "save_drafts" });
  // Another tab taking over: save drafts first (Go writes and syncs the
  // file before kiln.act returns), then hand the files over.
  lock?.serve(async () => { saveDrafts(); await persist?.handOff(); }, () => location.reload());

  // kilnPage is the page's handle for tests: act, and with ?record in the
  // address every event Go sent.
  const record = new URLSearchParams(location.search).has("record");
  const events: Event[] = [];
  g.kilnPage = record ? { act, events } : { act };

  // Leaving the tab (another tab or app, the phone locking): save drafts,
  // so iOS killing the tab in the background loses nothing typed.
  const leaving = () => { saveDrafts(); persist?.flush(); };
  addEventListener("visibilitychange", () => { if (document.visibilityState === "hidden") leaving(); });
  addEventListener("pagehide", leaving);

  await loadScript("wasm_exec.js");
  const relay = new URL("relay", location.href);
  relay.protocol = location.protocol === "https:" ? "wss:" : "ws:";
  // onBatch must never throw: it runs inside a call from Go. It may call
  // kiln.act: Go queues it, never waits.
  const onBatch = (json: string) => {
    try {
      const batch: Event[] = JSON.parse(json);
      if (record) events.push(...batch);
      store.apply(batch);
    } catch (e) {
      console.error(e);
    }
  };
  g.kilnReady = () => {
    g.kiln.start(relay.href, onBatch, store.state.saving); // storage may have failed since it loaded
    store.set({ phase: "running", progress: undefined });
  };

  const go = new g.Go();
  go.env = { HOME, TMPDIR: "/tmp" };
  const res = await fetch("kiln.wasm");
  const wasm = await readWithProgress(res, (got, total) => store.set({ progress: { got, total } }));
  const { instance } = await WebAssembly.instantiate(wasm, go.importObject);
  await go.run(instance); // returns when Go exits: /quit, or it couldn't start
  store.set({ phase: "stopped" });
}
