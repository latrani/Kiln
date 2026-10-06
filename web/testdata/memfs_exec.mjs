// Runs a GOOS=js test binary like Go's go_js_wasm_exec, but on memfs, so
// tests see the filesystem the browser page gives Kiln.
// Use (in web/): GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...
import { createRequire } from "node:module";
import { readFileSync } from "node:fs";
import { createFS } from "../static/memfs.mjs";

const require = createRequire(import.meta.url);
const [, , wasmPath, ...args] = process.argv;
const wasm = readFileSync(wasmPath); // with the real fs, before the swap

const fs = createFS({ stdout: (b) => process.stdout.write(b), stderr: (b) => process.stderr.write(b) });
fs.mkdirAll(process.cwd()); // go test runs the binary in the package dir
fs.mkdirAll("/tmp");
fs.mkdirAll("/home/kiln");
globalThis.fs = fs;
globalThis.path = require("node:path");
require(`${process.env.GOROOT}/lib/wasm/wasm_exec.js`);

const go = new Go();
go.argv = [wasmPath, ...args];
go.env = { ...process.env, HOME: "/home/kiln", TMPDIR: "/tmp" };
go.exit = process.exit;
const { instance } = await WebAssembly.instantiate(wasm, go.importObject);
process.on("exit", (code) => { // as in wasm_exec_node.js: report a deadlock
  if (code === 0 && !go.exited) {
    go._pendingEvent = { id: 0 };
    go._resume();
  }
});
await go.run(instance);
