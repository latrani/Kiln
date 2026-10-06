// Boots Kiln in the page: memfs seeded with the preset config, Go's wasm
// runtime, and xterm.js as Kiln's terminal. memfs must be in place before
// wasm_exec.js loads: wasm_exec only stubs globalThis.fs if it's missing.
import { Terminal } from "./vendor/xterm.mjs";
import { FitAddon } from "./vendor/addon-fit.mjs";
import { WebglAddon } from "./vendor/addon-webgl.mjs";
import { ClipboardAddon } from "./vendor/addon-clipboard.mjs";
import { createFS } from "./memfs.mjs";

const HOME = "/home/kiln";
const CONFIG = `${HOME}/.config/kiln`;

function loadScript(src) {
  return new Promise((ok, fail) => {
    const s = document.createElement("script");
    s.src = src;
    s.onload = ok;
    s.onerror = () => fail(new Error(`loading ${src}`));
    document.head.append(s);
  });
}

async function seedPreset(fs) {
  const files = await (await fetch("preset/manifest.json")).json();
  for (const f of files) {
    const res = await fetch(`preset/${f}`);
    if (!res.ok) throw new Error(`preset/${f}: ${res.status}`);
    fs.writeFileAll(`${CONFIG}/${f}`, new Uint8Array(await res.arrayBuffer()));
  }
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

const fs = createFS();
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
await loadScript("wasm_exec.js");

const relay = new URL("relay", location.href);
relay.protocol = location.protocol === "https:" ? "wss:" : "ws:";

globalThis.kilnReady = () => {
  kiln.start(term.cols, term.rows, relay.href, (bytes) => term.write(bytes));
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
