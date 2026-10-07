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
