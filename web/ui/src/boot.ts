// Boots Kiln in the page: takes the tab lock (one tab owns Kiln), loads
// saved files from IndexedDB into memfs, seeds the preset where nothing's
// saved, downloads and starts Go's wasm, and feeds its batches to the
// store. memfs must be in place before wasm_exec.js loads: wasm_exec only
// stubs globalThis.fs if it's missing.
import { createFS } from "../../static/memfs.mjs";
import { createPersist, idbStore } from "../../static/persist.mjs";
import { createTabLock } from "../../static/tablock.mjs";
import { forgetAsked } from "./notify.ts";
import { resetter } from "./reset.ts";
import type { Event, Store } from "./store.ts";
import * as str from "./str.gen.ts";
import { forgetTheme } from "./theme.ts";

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
// untouched preset world follows the server. The preset is laid out like
// a backup: config/… is the config dir. Anything else is ignored (TLS
// pins live in the relay's config).
async function seedPreset(fs: any): Promise<void> {
  const files: string[] = await (await fetch("preset/manifest.json")).json();
  const get = async (f: string) => {
    const res = await fetch(`preset/${f}`);
    if (!res.ok) throw new Error(`preset/${f}: ${res.status}`);
    return new Uint8Array(await res.arrayBuffer());
  };
  for (const f of files) {
    const rel = f.startsWith("config/") ? f.slice("config/".length) : "";
    if (!rel || fs.exists(`${CONFIG}/${rel}`)) continue;
    fs.writeFileAll(`${CONFIG}/${rel}`, await get(f));
  }
}

// toBase64 is bytes as base64, a chunk at a time: one call with every
// byte as an argument would overflow the stack for a large file.
export function toBase64(bytes: Uint8Array): string {
  let s = "";
  for (let i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(s);
}

// pickFile asks for a file (a backup to restore) and gives its bytes to
// done, no more than max and one past it, so Go can say a file is too
// big without the page reading all of one; nothing if none is chosen.
export function pickFile(accept: string, max: number, done: (bytes: Uint8Array) => void): void {
  const f = Object.assign(document.createElement("input"), { type: "file", accept, hidden: true });
  document.body.append(f);
  f.addEventListener("cancel", () => f.remove());
  f.addEventListener("change", async () => {
    const file = f.files?.[0];
    f.remove();
    if (file) done(new Uint8Array(await file.slice(0, max + 1).arrayBuffer()));
  });
  f.click();
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

  // kilnPage is the page's handle for tests: act, and with ?record in the
  // address every event Go sent and every action the page sent (a
  // password's text left out). Every action goes after what's typed
  // (typing.ts), which starts going to Go once Kiln runs.
  const record = new URLSearchParams(location.search).has("record");
  const events: Event[] = [], sent: object[] = [];
  const send = (o: object, secret?: boolean) => {
    if (record) sent.push(secret ? { ...o, Text: undefined } : o);
    g.kiln?.act?.(JSON.stringify(o));
  };
  const typing = store.state.typing!;
  const act = (o: object) => typing.act(o);
  store.set({ act });
  // /reset: wipe what this browser saved, then reload, where the preset
  // seeds a first visit's files. Saving stops first, so nothing written on
  // the way out (drafts, at pagehide) comes back. A handoff to another
  // tab waits out the wipe (reset.ts).
  const resets = resetter({ wipe: async () => { await persist?.wipe(); }, forget: () => { forgetAsked(); forgetTheme(); }, reload: () => location.reload() });
  store.set({ reset: resets.reset });
  // dropLinks kills the WebSockets to the relay without a word, as a dying
  // network does; Kiln resumes them.
  g.kilnPage = record ? { act, events, sent, dropLinks: () => g.kiln?.dropLinks?.() } : { act };

  const saveDrafts = () => act({ Do: "save_drafts" });
  // Another tab taking over: save drafts first (Go writes and syncs the
  // file before kiln.act returns), then hand the files over.
  lock?.serve(async () => { await resets.leave(); saveDrafts(); await persist?.handOff(); }, () => location.reload());

  // Leaving the tab (another tab or app, the phone locking): save drafts,
  // so iOS killing the tab in the background loses nothing typed. Like
  // every action, save_drafts goes after what's typed and not yet sent.
  const leaving = () => { saveDrafts(); persist?.flush(); };
  addEventListener("visibilitychange", () => { if (document.visibilityState === "hidden") leaving(); });
  addEventListener("pagehide", leaving);
  // Back in view: a socket a suspended tab left can look open but be dead,
  // so Go checks each and resumes any that doesn't answer.
  addEventListener("visibilitychange", () => { if (document.visibilityState === "visible") g.kiln?.wake?.(); });
  addEventListener("pageshow", () => g.kiln?.wake?.());

  await loadScript("wasm_exec.js");
  const relay = new URL("relay", location.href);
  relay.protocol = location.protocol === "https:" ? "wss:" : "ws:";
  // onBatch must never throw: it runs inside a call from Go. It may call
  // kiln.act: Go queues it, never waits.
  // The first batch means Go's loop runs and takes actions: what was done
  // and typed while it loaded goes now, in order, once the batch says
  // which input is showing.
  let running = false;
  const onBatch = (json: string) => {
    try {
      const batch: Event[] = JSON.parse(json);
      if (record) events.push(...batch);
      store.apply(batch);
    } catch (e) {
      console.error(e);
    }
    if (!running) {
      running = true;
      const s = store.state;
      typing.ready(send, s.chars[s.active] ? s.active : ""); // after the batch: it says what Kiln opened with
      store.set({}); // draw the typing where it went
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
