// Boots Kiln in the page: takes the tab lock (one tab owns Kiln), loads
// saved files from IndexedDB into memfs, seeds the preset where nothing's
// saved, starts Go's wasm runtime, and drives the web API from a
// development page (index.html). memfs must be in place before
// wasm_exec.js loads: wasm_exec only stubs globalThis.fs if it's missing.
import { createFS } from "./memfs.mjs";
import { createPersist, idbStore } from "./persist.mjs";
import { createTabLock } from "./tablock.mjs";

const HOME = "/home/kiln";
const CONFIG = `${HOME}/.config/kiln`;

// pageStrings fetches the page's own strings (the catalog's [web] table),
// and returns t, which fills one in. Only the lock screen needs them, so
// a tab that owns Kiln never fetches them.
async function pageStrings() {
  const res = await fetch("strings.json");
  if (!res.ok) throw new Error(`strings.json: ${res.status}`);
  const strings = await res.json();
  return (key, args = {}) => strings[key].replace(/\{([a-z][a-z0-9_]*)\}/g, (_, n) => String(args[n]));
}

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
  const t = await pageStrings();
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
lock?.serve(async () => { await persist?.handOff(); }, () => location.reload());

globalThis.kilnDownload = (name, bytes) => {
  const url = URL.createObjectURL(new Blob([bytes]));
  const a = Object.assign(document.createElement("a"), { href: url, download: name });
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
};
// kilnPickFile resolves with the zip's bytes, null if the user cancels (or a
// newer pick replaces this one), and rejects if the file can't be read.
let cancelPick = null;
globalThis.kilnPickFile = () => new Promise((ok, fail) => {
  cancelPick?.();
  const input = Object.assign(document.createElement("input"), { type: "file", accept: ".zip" });
  input.hidden = true;
  document.body.append(input);
  const done = (settle, v) => { input.remove(); if (cancelPick === replace) cancelPick = null; settle(v); };
  const replace = () => done(ok, null);
  cancelPick = replace;
  input.onchange = async () => {
    const f = input.files[0];
    if (!f) return done(ok, null);
    let bytes;
    try { bytes = new Uint8Array(await f.arrayBuffer()); } catch (e) { return done(fail, e); }
    done(ok, bytes);
  };
  input.oncancel = () => done(ok, null);
  input.click();
});

await loadScript("wasm_exec.js");

const relay = new URL("relay", location.href);
relay.protocol = location.protocol === "https:" ? "wss:" : "ws:";

// The development page. events keeps every event, for the smoke test.
const events = [];
const act = (o) => globalThis.kiln?.act?.(JSON.stringify(o));
globalThis.kilnPage = { events, act };

const list = document.getElementById("chars");
const log = document.getElementById("log");
const line = document.getElementById("line");
let order = [], active = "", chars = {}, inputs = {};
let n = 0; // numbers what the line sends, so a reply to an older one never overwrites newer typing
const inputKey = () => (chars[active] ? active : "");
const send = (Do) => act({ Do, Key: inputKey(), Text: line.value, N: ++n });

function render() {
  list.replaceChildren(...order.map((k) => {
    const c = chars[k] ?? {};
    const b = document.createElement("button");
    b.textContent = `${c.Name ?? k} [${c.State ?? 0}]${c.Unread ? ` ${c.Unread}` : ""}`;
    b.className = k === active ? "active" : "";
    b.onclick = () => { send("input"); act({ Do: "switch", Key: k }); };
    return b;
  }));
}

// onBatch must never throw: it runs inside a call from Go, and an error
// here would stop Kiln.
function onBatch(json) {
  try { apply(JSON.parse(json)); } catch (e) { console.error(e); }
}

function apply(batch) {
  for (const e of batch) {
    events.push(e);
    switch (e.type) {
      case "Start": order = []; active = ""; chars = {}; inputs = {}; break;
      case "Order": order = e.Keys ?? []; break;
      case "Char": chars[e.Key] = e; break;
      case "Closed": delete chars[e.Key]; break;
      case "Active": active = e.Key; line.value = inputs[inputKey()] ?? ""; break;
      case "Input":
        inputs[e.Key] = e.Text;
        if (e.Key === inputKey() && e.Ack === n) line.value = e.Text;
        break;
      case "Effect":
        if (e.effect === "SaveFile") {
          globalThis.kilnDownload(e.Name.split("/").pop(), Uint8Array.from(atob(e.Data ?? ""), (ch) => ch.charCodeAt(0)));
        }
        break;
    }
    log.append(JSON.stringify(e) + "\n");
  }
  while (log.childNodes.length > 2000) log.firstChild.remove();
  log.scrollTop = log.scrollHeight;
  render();
}

line.addEventListener("input", () => send("input"));
line.addEventListener("keydown", (ev) => {
  const Do = { Enter: "submit", ArrowUp: "older", ArrowDown: "newer" }[ev.key];
  if (!Do || ev.shiftKey) return;
  ev.preventDefault();
  send(Do);
});
// Leaving the tab (another tab or app, the phone locking): save drafts, so
// iOS killing the tab in the background loses nothing typed. Go syncs the
// drafts file, which flushes it to IndexedDB.
const leaving = () => { send("input"); act({ Do: "save_drafts" }); persist?.flush(); };
addEventListener("visibilitychange", () => { if (document.visibilityState === "hidden") leaving(); });
addEventListener("pagehide", leaving);

globalThis.kilnReady = () => {
  kiln.start(relay.href, onBatch, saving);
  line.focus();
};

const go = new Go();
go.env = { HOME, TMPDIR: "/tmp" };
const wasm = await (await fetch("kiln.wasm")).arrayBuffer();
const { instance } = await WebAssembly.instantiate(wasm, go.importObject);
await go.run(instance);
