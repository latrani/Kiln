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
  // mkdir; create, write, chmod, utimes, unlink; rmdir
  assert.deepEqual(changed, ["/d/sub", "/d/f", "/d/f", "/d/f", "/d/f", "/d/f", "/d/sub"]);
});

test("truncate by path resizes and reports", () => {
  const { fs, changed } = watched();
  writeFile(fs, "/d/../f", "abcdef");
  changed.length = 0;
  call(fs, "truncate", "/f", 2);
  assert.equal(readFile(fs, "/f"), "ab");
  call(fs, "truncate", "/./f", 4);
  assert.deepEqual([...enc(readFile(fs, "/f"))], [0x61, 0x62, 0, 0]);
  assert.deepEqual(changed, ["/f", "/f"]);
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

test("an fd on a file that's renamed over, or unlinked, reports nothing", () => {
  const { fs, changed, synced } = watched();
  writeFile(fs, "/f", "old");
  const fd = call(fs, "open", "/f", C.O_WRONLY, 0);
  writeFile(fs, "/f.tmp", "new");
  call(fs, "rename", "/f.tmp", "/f");
  writeFile(fs, "/g", "g");
  const gd = call(fs, "open", "/g", C.O_WRONLY, 0);
  call(fs, "unlink", "/g");
  changed.length = 0;
  for (const d of [fd, gd]) {
    call(fs, "write", d, enc("x"), 0, 1, null);
    call(fs, "ftruncate", d, 0);
    call(fs, "fchmod", d, 0o600);
    call(fs, "fsync", d);
  }
  assert.deepEqual(changed, []);
  assert.deepEqual(synced, []);
  assert.equal(readFile(fs, "/f"), "new");
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
