import { test } from "node:test";
import assert from "node:assert/strict";
import { createFS, constants as C } from "./memfs.mjs";

// call runs an fs method and returns its result, or throws its error.
function call(fs, name, ...args) {
  let out;
  fs[name](...args, (err, res) => { if (err) throw err; out = res; });
  return out;
}
const enc = (s) => new TextEncoder().encode(s);
const dec = (b) => new TextDecoder().decode(b);

function writeFile(fs, path, s, flags = C.O_WRONLY | C.O_CREAT | C.O_TRUNC) {
  const fd = call(fs, "open", path, flags, 0o644);
  const b = enc(s);
  call(fs, "write", fd, b, 0, b.length, null);
  call(fs, "close", fd);
}
function readFile(fs, path) {
  const fd = call(fs, "open", path, C.O_RDONLY, 0);
  const size = call(fs, "fstat", fd).size;
  const buf = new Uint8Array(size);
  call(fs, "read", fd, buf, 0, size, null);
  call(fs, "close", fd);
  return dec(buf);
}

test("write then read", () => {
  const fs = createFS();
  fs.mkdirAll("/a/b");
  writeFile(fs, "/a/b/f.txt", "hello");
  assert.equal(readFile(fs, "/a/b/f.txt"), "hello");
});

test("append and truncate", () => {
  const fs = createFS();
  writeFile(fs, "/f", "ab");
  writeFile(fs, "/f", "cd", C.O_WRONLY | C.O_APPEND);
  assert.equal(readFile(fs, "/f"), "abcd");
  writeFile(fs, "/f", "x");
  assert.equal(readFile(fs, "/f"), "x");
});

test("positional write past the end zero-fills", () => {
  const fs = createFS();
  writeFile(fs, "/f", "abcdef");
  const fd = call(fs, "open", "/f", C.O_RDWR, 0);
  call(fs, "ftruncate", fd, 1);
  call(fs, "write", fd, enc("z"), 0, 1, 3);
  call(fs, "close", fd);
  assert.deepEqual([...enc(readFile(fs, "/f"))], [0x61, 0, 0, 0x7a]);
});

test("errors carry node codes", () => {
  const fs = createFS();
  const code = (fn) => { try { fn(); } catch (e) { return e.code; } return "none"; };
  assert.equal(code(() => call(fs, "open", "/missing/f", C.O_WRONLY | C.O_CREAT, 0o644)), "ENOENT");
  writeFile(fs, "/f", "x");
  assert.equal(code(() => call(fs, "open", "/f", C.O_WRONLY | C.O_CREAT | C.O_EXCL, 0o644)), "EEXIST");
  assert.equal(code(() => call(fs, "mkdir", "/f", 0o755)), "EEXIST");
  assert.equal(code(() => call(fs, "close", 99)), "EBADF");
  fs.mkdirAll("/d/e");
  assert.equal(code(() => call(fs, "rmdir", "/d")), "ENOTEMPTY");
  assert.equal(code(() => call(fs, "unlink", "/d")), "EISDIR");
});

test("rename replaces an existing file", () => {
  const fs = createFS();
  writeFile(fs, "/config.toml", "old");
  writeFile(fs, "/config.toml.tmp", "new");
  call(fs, "rename", "/config.toml.tmp", "/config.toml");
  assert.equal(readFile(fs, "/config.toml"), "new");
  assert.deepEqual(call(fs, "readdir", "/"), ["config.toml"]);
});

test("stat says what a node is", () => {
  const fs = createFS();
  fs.mkdirAll("/d");
  writeFile(fs, "/d/f", "abc");
  const d = call(fs, "stat", "/d"), f = call(fs, "stat", "/d/f");
  assert.ok(d.isDirectory());
  assert.ok(!f.isDirectory());
  assert.equal(f.size, 3);
  assert.equal(d.mode & 0o170000, 0o040000);
  assert.equal(f.mode & 0o170000, 0o100000);
});

test("readdir is sorted; paths normalize", () => {
  const fs = createFS();
  fs.mkdirAll("/d");
  writeFile(fs, "/d/b", "");
  writeFile(fs, "/d/./a", "");
  writeFile(fs, "/d/x/../c", "x".slice(1)); // /d/x doesn't exist: still resolves to /d/c
  assert.deepEqual(call(fs, "readdir", "/d"), ["a", "b", "c"]);
});

test("stdout and stderr go to the callbacks", () => {
  const seen = [];
  const fs = createFS({ stdout: (b) => seen.push(["out", dec(b)]), stderr: (b) => seen.push(["err", dec(b)]) });
  fs.writeSync(1, enc("hi\n"));
  call(fs, "write", 2, enc("oops"), 0, 4, null);
  assert.deepEqual(seen, [["out", "hi\n"], ["err", "oops"]]);
});

test("writeFileAll makes parents", () => {
  const fs = createFS();
  fs.writeFileAll("/home/kiln/.config/kiln/worlds/fm.toml", enc("host = 1"));
  assert.equal(readFile(fs, "/home/kiln/.config/kiln/worlds/fm.toml"), "host = 1");
});
