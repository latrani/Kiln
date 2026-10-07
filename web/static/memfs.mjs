// memfs: an in-memory filesystem with the Node-style callback API that
// Go's syscall package calls through globalThis.fs under GOOS=js (see
// $(go env GOROOT)/src/syscall/fs_js.go). It gives Kiln real files in the
// browser: config, known_hosts and logs, for the length of a visit.
//
// Errors carry Node's codes (ENOENT, ...): Go maps them to errnos, and
// panics on a code it doesn't know. Callbacks run synchronously, which
// Go's fsCall allows. Paths resolve lexically; the cwd is "/".
//
// memfs reports changed paths through onChange (every mutation) and onSync
// (fsync) so the page can save them; persist.mjs does that. mkdirAll,
// writeFileAll and put are for the page and stay silent.

const S_IFDIR = 0o040000;
const S_IFREG = 0o100000;

export const constants = {
  O_RDONLY: 0, O_WRONLY: 1, O_RDWR: 2, O_CREAT: 0o100, O_EXCL: 0o200,
  O_TRUNC: 0o1000, O_APPEND: 0o2000, O_DIRECTORY: 0o200000,
};

function fail(code, call, path = "") {
  const e = new Error(`${code}: ${call} ${path}`);
  e.code = code;
  return e;
}

function lineLogger(log) {
  let pending = "";
  const dec = new TextDecoder();
  return (bytes) => {
    pending += dec.decode(bytes, { stream: true });
    const nl = pending.lastIndexOf("\n");
    if (nl >= 0) {
      log(pending.slice(0, nl));
      pending = pending.slice(nl + 1);
    }
  };
}

export function createFS({ stdout = lineLogger(console.log), stderr = lineLogger(console.error),
  onChange = () => {}, onSync = () => {} } = {}) {
  let nextIno = 1;
  let nextFd = 3; // 0-2 are stdio
  const now = () => Date.now();
  const dir = (perm) => ({ kind: "dir", mode: S_IFDIR | (perm & 0o7777), ino: nextIno++, entries: new Map(), mtimeMs: now() });
  const file = (perm) => ({ kind: "file", mode: S_IFREG | (perm & 0o7777), ino: nextIno++, data: new Uint8Array(0), size: 0, mtimeMs: now() });
  const root = dir(0o755);
  const fds = new Map();

  function parts(path) {
    const out = [];
    for (const p of String(path).split("/")) {
      if (!p || p === ".") continue;
      if (p === "..") out.pop(); else out.push(p);
    }
    return out;
  }
  const norm = (path) => "/" + parts(path).join("/");
  // under lists n's path and every path below it, joined onto base.
  function under(n, base) {
    const out = [base];
    if (n.kind === "dir") for (const [name, c] of n.entries) out.push(...under(c, base === "/" ? `/${name}` : `${base}/${name}`));
    return out;
  }
  function walk(ps, call, path) {
    let n = root;
    for (const p of ps) {
      if (n.kind !== "dir") throw fail("ENOTDIR", call, path);
      n = n.entries.get(p);
      if (!n) throw fail("ENOENT", call, path);
    }
    return n;
  }
  const lookup = (path, call) => walk(parts(path), call, path);
  function parentOf(path, call) {
    const ps = parts(path);
    if (ps.length === 0) throw fail("EEXIST", call, path);
    const d = walk(ps.slice(0, -1), call, path);
    if (d.kind !== "dir") throw fail("ENOTDIR", call, path);
    return [d, ps[ps.length - 1]];
  }
  function open(fd, call) {
    const f = fds.get(fd);
    if (!f) throw fail("EBADF", call);
    return f;
  }
  function ensure(n, len) {
    if (len <= n.data.length) return;
    const d = new Uint8Array(Math.max(len, n.data.length * 2));
    d.set(n.data.subarray(0, n.size));
    n.data = d;
  }
  function resize(n, len, call) {
    if (n.kind === "dir") throw fail("EISDIR", call);
    ensure(n, len);
    if (len > n.size) n.data.fill(0, n.size, len);
    n.size = len;
    n.mtimeMs = now();
  }
  function stat(n) {
    const size = n.kind === "dir" ? 0 : n.size;
    const isDir = n.kind === "dir";
    return {
      dev: 1, ino: n.ino, mode: n.mode, nlink: 1, uid: 0, gid: 0, rdev: 0, size,
      blksize: 4096, blocks: Math.ceil(size / 512),
      atimeMs: n.mtimeMs, mtimeMs: n.mtimeMs, ctimeMs: n.mtimeMs,
      isDirectory: () => isDir,
    };
  }
  // run calls fn and hands its result or error to a Node-style callback.
  // Errors without a code are bugs, not filesystem errors: rethrow them.
  function run(callback, fn) {
    let res, err = null;
    try { res = fn(); } catch (e) { if (!e.code) throw e; err = e; }
    callback(err, res);
  }
  function stdio(fd, bytes) {
    (fd === 1 ? stdout : stderr)(bytes);
    return bytes.length;
  }

  const fs = {
    constants,

    writeSync(fd, buf) {
      if (fd === 1 || fd === 2) return stdio(fd, buf);
      let n;
      fs.write(fd, buf, 0, buf.length, null, (err, res) => { if (err) throw err; n = res; });
      return n;
    },

    open(path, flags, perm, cb) {
      run(cb, () => {
        let n, created = false;
        try {
          n = lookup(path, "open");
          if ((flags & constants.O_CREAT) && (flags & constants.O_EXCL)) throw fail("EEXIST", "open", path);
        } catch (e) {
          if (e.code !== "ENOENT" || !(flags & constants.O_CREAT)) throw e;
          const [d, name] = parentOf(path, "open");
          n = file(perm);
          created = true;
          d.entries.set(name, n);
          d.mtimeMs = now();
        }
        if (n.kind === "dir" && (flags & 3) !== constants.O_RDONLY) throw fail("EISDIR", "open", path);
        if ((flags & constants.O_DIRECTORY) && n.kind !== "dir") throw fail("ENOTDIR", "open", path);
        if ((flags & constants.O_TRUNC) && n.kind === "file") { resize(n, 0, "open"); created = true; }
        const fd = nextFd++;
        fds.set(fd, { node: n, pos: 0, append: !!(flags & constants.O_APPEND), path: norm(path) });
        if (created) onChange(norm(path));
        return fd;
      });
    },
    close(fd, cb) { run(cb, () => { if (!fds.delete(fd)) throw fail("EBADF", "close"); }); },
    fsync(fd, cb) { run(cb, () => { onSync(open(fd, "fsync").path); }); },

    read(fd, buf, offset, length, position, cb) {
      run(cb, () => {
        const f = open(fd, "read");
        if (f.node.kind === "dir") throw fail("EISDIR", "read");
        const pos = position ?? f.pos;
        const k = Math.max(0, Math.min(length, f.node.size - pos));
        buf.set(f.node.data.subarray(pos, pos + k), offset);
        if (position == null) f.pos += k;
        return k;
      });
    },
    write(fd, buf, offset, length, position, cb) {
      run(cb, () => {
        if (fd === 1 || fd === 2) return stdio(fd, buf.subarray(offset, offset + length));
        const f = open(fd, "write");
        const n = f.node;
        const pos = position ?? (f.append ? n.size : f.pos);
        if (pos + length > n.size) resize(n, pos + length, "write");
        n.data.set(buf.subarray(offset, offset + length), pos);
        n.mtimeMs = now();
        onChange(f.path);
        if (position == null) f.pos = pos + length;
        return length;
      });
    },
    ftruncate(fd, len, cb) { run(cb, () => { const f = open(fd, "ftruncate"); resize(f.node, len, "ftruncate"); onChange(f.path); }); },
    truncate(path, len, cb) { run(cb, () => { resize(lookup(path, "truncate"), len, "truncate"); onChange(norm(path)); }); },

    fstat(fd, cb) { run(cb, () => stat(open(fd, "fstat").node)); },
    stat(path, cb) { run(cb, () => stat(lookup(path, "stat"))); },
    lstat(path, cb) { run(cb, () => stat(lookup(path, "lstat"))); },

    mkdir(path, perm, cb) {
      run(cb, () => {
        const [d, name] = parentOf(path, "mkdir");
        if (d.entries.has(name)) throw fail("EEXIST", "mkdir", path);
        d.entries.set(name, dir(perm));
        d.mtimeMs = now();
        onChange(norm(path));
      });
    },
    readdir(path, cb) {
      run(cb, () => {
        const n = lookup(path, "readdir");
        if (n.kind !== "dir") throw fail("ENOTDIR", "readdir", path);
        return [...n.entries.keys()].sort();
      });
    },
    rename(from, to, cb) {
      run(cb, () => {
        const [fdir, fname] = parentOf(from, "rename");
        const n = fdir.entries.get(fname);
        if (!n) throw fail("ENOENT", "rename", from);
        const [td, tname] = parentOf(to, "rename");
        const old = td.entries.get(tname);
        if (old && old !== n) {
          if (old.kind === "dir" && n.kind !== "dir") throw fail("EISDIR", "rename", to);
          if (old.kind !== "dir" && n.kind === "dir") throw fail("ENOTDIR", "rename", to);
          if (old.kind === "dir" && old.entries.size) throw fail("ENOTEMPTY", "rename", to);
        }
        const from_ = norm(from), to_ = norm(to);
        const moved = under(n, from_);
        fdir.entries.delete(fname);
        td.entries.set(tname, n);
        fdir.mtimeMs = td.mtimeMs = now();
        for (const f of fds.values()) {
          if (f.path === from_ || f.path.startsWith(from_ + "/")) f.path = to_ + f.path.slice(from_.length);
        }
        for (const p of moved) { onChange(p); onChange(to_ + p.slice(from_.length)); }
      });
    },
    unlink(path, cb) {
      run(cb, () => {
        const [d, name] = parentOf(path, "unlink");
        const n = d.entries.get(name);
        if (!n) throw fail("ENOENT", "unlink", path);
        if (n.kind === "dir") throw fail("EISDIR", "unlink", path);
        d.entries.delete(name);
        onChange(norm(path));
      });
    },
    rmdir(path, cb) {
      run(cb, () => {
        const [d, name] = parentOf(path, "rmdir");
        const n = d.entries.get(name);
        if (!n) throw fail("ENOENT", "rmdir", path);
        if (n.kind !== "dir") throw fail("ENOTDIR", "rmdir", path);
        if (n.entries.size) throw fail("ENOTEMPTY", "rmdir", path);
        d.entries.delete(name);
        onChange(norm(path));
      });
    },

    chmod(path, mode, cb) { run(cb, () => { const n = lookup(path, "chmod"); n.mode = (n.mode & ~0o7777) | (mode & 0o7777); onChange(norm(path)); }); },
    fchmod(fd, mode, cb) { run(cb, () => { const f = open(fd, "fchmod"); const n = f.node; n.mode = (n.mode & ~0o7777) | (mode & 0o7777); onChange(f.path); }); },
    utimes(path, atime, mtime, cb) { run(cb, () => { lookup(path, "utimes").mtimeMs = mtime * 1000; onChange(norm(path)); }); },
    chown(path, uid, gid, cb) { run(cb, () => { lookup(path, "chown"); }); },
    fchown(fd, uid, gid, cb) { run(cb, () => { open(fd, "fchown"); }); },
    lchown(path, uid, gid, cb) { run(cb, () => { lookup(path, "lchown"); }); },
    link(from, to, cb) { run(cb, () => { throw fail("ENOSYS", "link"); }); },
    symlink(from, to, cb) { run(cb, () => { throw fail("ENOSYS", "symlink"); }); },
    readlink(path, cb) { run(cb, () => { throw fail("ENOSYS", "readlink"); }); },

    // The rest are for the page and test runner, not Go. None report changes.
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
    mkdirAll(path) {
      let n = root;
      for (const p of parts(path)) {
        if (!n.entries.has(p)) n.entries.set(p, dir(0o755));
        n = n.entries.get(p);
        if (n.kind !== "dir") throw fail("ENOTDIR", "mkdirAll", path);
      }
    },
    writeFileAll(path, bytes) {
      fs.mkdirAll(parts(path).slice(0, -1).join("/"));
      const [d, name] = parentOf(path, "writeFileAll");
      const n = file(0o644);
      ensure(n, bytes.length);
      n.data.set(bytes);
      n.size = bytes.length;
      d.entries.set(name, n);
    },
  };
  return fs;
}
